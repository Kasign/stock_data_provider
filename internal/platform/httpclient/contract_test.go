package httpclient

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNewAppliesPLATCTR005Defaults(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if client.config.Timeout != 15*time.Second ||
		client.config.DialTimeout != 5*time.Second ||
		client.config.TLSHandshakeTimeout != 5*time.Second ||
		client.config.ResponseHeaderTimeout != 8*time.Second ||
		client.config.MaxResponseBytes != 2<<20 ||
		client.config.MaxRetries != 0 ||
		client.config.MaxConcurrent != 4 {
		t.Fatalf("unexpected HTTP Client defaults: %+v", client.config)
	}
	if client.transport.Proxy != nil {
		t.Fatal("environment proxy must remain disabled")
	}
	if client.transport.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Fatalf("TLS minimum version = %d, want TLS 1.2", client.transport.TLSClientConfig.MinVersion)
	}
	if cap(client.defaultSemaphore) != 4 {
		t.Fatalf("fallback concurrency = %d, want 4", cap(client.defaultSemaphore))
	}
}

func TestNewEnforcesPLATCTR005UpperBounds(t *testing.T) {
	tests := []struct {
		name   string
		config Config
	}{
		{name: "response body above 16 MiB", config: Config{MaxResponseBytes: maxResponseBytesLimit + 1}},
		{name: "negative retries", config: Config{MaxRetries: -1}},
		{name: "more than two retries", config: Config{MaxRetries: 3}},
		{name: "more than 32 fallback slots", config: Config{MaxConcurrent: 33}},
		{name: "dependency concurrency zero", config: Config{DependencyLimits: map[string]int{"news.fetch": 0}}},
		{name: "dependency concurrency above 32", config: Config{DependencyLimits: map[string]int{"news.fetch": 33}}},
		{name: "invalid dependency name", config: Config{DependencyLimits: map[string]int{"news fetch": 1}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.config); err == nil {
				t.Fatal("expected configuration to be rejected")
			}
		})
	}

	for _, config := range []Config{
		{MaxResponseBytes: maxResponseBytesLimit},
		{MaxRetries: maxRetryLimit},
		{MaxConcurrent: maxConcurrentLimit},
		{DependencyLimits: map[string]int{"news.fetch": maxConcurrentLimit}},
	} {
		if _, err := New(config); err != nil {
			t.Fatalf("valid upper boundary rejected: %+v: %v", config, err)
		}
	}
}

func TestNewCopiesDependencyLimitConfiguration(t *testing.T) {
	limits := map[string]int{"news.fetch": 2}
	client, err := New(Config{DependencyLimits: limits})
	if err != nil {
		t.Fatal(err)
	}
	limits["news.fetch"] = 32
	if got := cap(client.semaphores["news.fetch"]); got != 2 {
		t.Fatalf("dependency limit changed after caller mutation: %d", got)
	}
}

func TestClientRetriesOnlyWhenIdempotentAndBodyReplayable(t *testing.T) {
	tests := []struct {
		name       string
		idempotent bool
		body       bool
		replayable bool
		wantCalls  int32
	}{
		{name: "non idempotent", wantCalls: 1},
		{name: "idempotent without body", idempotent: true, wantCalls: 2},
		{name: "idempotent replayable body", idempotent: true, body: true, replayable: true, wantCalls: 2},
		{name: "idempotent non replayable body", idempotent: true, body: true, wantCalls: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()

			client, err := New(Config{
				Resolver:   staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
				MaxRetries: 1,
				Timeout:    2 * time.Second,
			})
			if err != nil {
				t.Fatal(err)
			}
			client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			t.Cleanup(client.CloseIdleConnections)

			request := localRequest(t, server, "/retry")
			if tt.body {
				request.Method = http.MethodPost
				request.Body = io.NopCloser(strings.NewReader("payload"))
				request.ContentLength = int64(len("payload"))
				if tt.replayable {
					request.GetBody = func() (io.ReadCloser, error) {
						return io.NopCloser(strings.NewReader("payload")), nil
					}
				}
			}
			policy := localPolicy(server)
			policy.Idempotent = tt.idempotent
			response, err := client.Do(context.Background(), request, policy)
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if got := calls.Load(); got != tt.wantCalls {
				t.Fatalf("request count = %d, want %d", got, tt.wantCalls)
			}
		})
	}
}

func TestClientDoesNotRetryRateLimitResponses(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver:   staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		MaxRetries: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)

	policy := localPolicy(server)
	policy.Idempotent = true
	response, err := client.Do(context.Background(), localRequest(t, server, "/rate-limit"), policy)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if got := calls.Load(); got != 1 {
		t.Fatalf("429 response was retried %d times", got)
	}
}
