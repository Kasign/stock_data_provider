package httpclient

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/observability"
)

type staticResolver struct {
	addresses []netip.Addr
	err       error
}

func (r staticResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	return r.addresses, r.err
}

func TestIsPublicIPRejectsNonPublicRanges(t *testing.T) {
	tests := []string{
		"0.0.0.0", "10.1.2.3", "100.64.0.1", "127.0.0.1",
		"169.254.1.1", "172.16.0.1", "192.0.2.1", "192.168.1.1",
		"198.18.0.1", "198.51.100.1", "203.0.113.1", "224.0.0.1",
		"::", "::1", "::ffff:127.0.0.1", "::ffff:8.8.8.8", "fc00::1", "fe80::1", "2001:db8::1", "3fff::1",
	}
	for _, value := range tests {
		address := netip.MustParseAddr(value)
		if isPublicIP(address) {
			t.Errorf("expected %s to be rejected", value)
		}
	}
	for _, value := range []string{"1.1.1.1", "8.8.8.8", "2606:4700:4700::1111"} {
		if !isPublicIP(netip.MustParseAddr(value)) {
			t.Errorf("expected %s to be public", value)
		}
	}
}

func TestValidateURLRequiresExactHTTPSAllowlist(t *testing.T) {
	policy := DestinationPolicy{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}}
	valid, _ := url.Parse("https://example.com/articles")
	if err := validateURL(valid, policy); err != nil {
		t.Fatalf("expected URL to pass: %v", err)
	}
	for _, raw := range []string{
		"http://example.com/articles",
		"https://user@example.com/articles",
		"https://sub.example.com/articles",
		"https://example.com:8443/articles",
		"https://127.0.0.1/articles",
	} {
		value, _ := url.Parse(raw)
		if err := validateURL(value, policy); err == nil {
			t.Errorf("expected %s to be rejected", raw)
		}
	}
}

func TestValidateURLAllowsExplicitHTTPPortOnlyWhenEnabled(t *testing.T) {
	value, _ := url.Parse("http://example.com:8080/feed")
	policy := DestinationPolicy{
		Dependency: "news.fetch", AllowedHosts: []string{"example.com"},
		AllowedPorts: []uint16{8080}, AllowHTTP: true,
	}
	if err := validateURL(value, policy); err != nil {
		t.Fatalf("expected explicitly allowed HTTP URL: %v", err)
	}
}

func TestResolveRejectsMixedPublicAndPrivateDNSAnswers(t *testing.T) {
	client, err := New(Config{Resolver: staticResolver{addresses: []netip.Addr{
		netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("10.0.0.8"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	policy := DestinationPolicy{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}}
	_, err = client.resolveAndValidate(context.Background(), "example.com", "443", policy)
	if !errors.Is(err, errPolicyRejected) {
		t.Fatalf("expected mixed DNS answers to be rejected, got %v", err)
	}
}

func TestResolveAcceptsOnlyValidatedPublicDNSAnswers(t *testing.T) {
	client, err := New(Config{Resolver: staticResolver{addresses: []netip.Addr{
		netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("1.1.1.1"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	policy := DestinationPolicy{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}}
	addresses, err := client.resolveAndValidate(context.Background(), "example.com", "443", policy)
	if err != nil || len(addresses) != 2 {
		t.Fatalf("expected two validated addresses, got %v, %v", addresses, err)
	}
}

func TestReadLimitedBodyRejectsOversizedResponse(t *testing.T) {
	_, err := readLimitedBody(strings.NewReader("12345"), 4)
	if !errors.Is(err, errResponseTooLarge) {
		t.Fatalf("expected response size error, got %v", err)
	}
	data, err := readLimitedBody(strings.NewReader("1234"), 4)
	if err != nil || string(data) != "1234" {
		t.Fatalf("unexpected bounded body: %q, %v", data, err)
	}
}

func TestRedirectRequiresExplicitPolicy(t *testing.T) {
	originalURL, _ := url.Parse("https://example.com/start")
	nextURL, _ := url.Parse("https://example.com/next")
	ctx := context.WithValue(context.Background(), policyContextKey{}, DestinationPolicy{
		Dependency: "news.fetch", AllowedHosts: []string{"example.com"},
	})
	original := &http.Request{URL: originalURL, Header: make(http.Header)}
	next := &http.Request{URL: nextURL, Header: make(http.Header)}
	next = next.WithContext(ctx)
	if err := validateRedirect(next, []*http.Request{original}); !errors.Is(err, errRedirectRejected) {
		t.Fatalf("expected redirect to be denied by default, got %v", err)
	}
}

func TestClientDoesNotUseEnvironmentProxy(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if client.transport.Proxy != nil {
		t.Fatal("expected environment proxy to be disabled")
	}
	if client.httpClient.Timeout != 0 {
		t.Fatal("request timeout must be controlled by context")
	}
}

func TestValidateURLRejectsCustomHostHeader(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	request, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	request.Header.Set("Host", "internal.local")
	_, err = client.Do(context.Background(), request, DestinationPolicy{
		Dependency: "news.fetch", AllowedHosts: []string{"example.com"},
	})
	if err == nil {
		t.Fatal("expected custom Host header to be rejected")
	}
}

type sequenceResolver struct {
	mu        sync.Mutex
	addresses [][]netip.Addr
	index     int
}

func (r *sequenceResolver) LookupNetIP(context.Context, string, string) ([]netip.Addr, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	index := r.index
	if index >= len(r.addresses) {
		index = len(r.addresses) - 1
	}
	r.index++
	return r.addresses[index], nil
}

func TestResolveRevalidatesDNSForEachNewConnection(t *testing.T) {
	resolver := &sequenceResolver{addresses: [][]netip.Addr{
		{netip.MustParseAddr("8.8.8.8")},
		{netip.MustParseAddr("10.0.0.8")},
	}}
	client, err := New(Config{Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	policy := DestinationPolicy{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}}
	if _, err := client.resolveAndValidate(context.Background(), "example.com", "443", policy); err != nil {
		t.Fatalf("first public resolution should pass: %v", err)
	}
	if _, err := client.resolveAndValidate(context.Background(), "example.com", "443", policy); !errors.Is(err, errPolicyRejected) {
		t.Fatalf("second private resolution should be rejected: %v", err)
	}
}

func TestValidateURLRejectsMappedIPv6AndPrivateLiteral(t *testing.T) {
	for _, raw := range []string{"https://[::ffff:8.8.8.8]/", "https://127.0.0.1/"} {
		value, _ := url.Parse(raw)
		host := value.Hostname()
		policy := DestinationPolicy{Dependency: "news.fetch", AllowedHosts: []string{host}}
		if err := validateURL(value, policy); !errors.Is(err, errPolicyRejected) {
			t.Errorf("expected %s to be rejected, got %v", raw, err)
		}
	}
}

func TestRedirectRejectsHTTPSDowngrade(t *testing.T) {
	previousURL, _ := url.Parse("https://example.com/start")
	nextURL, _ := url.Parse("http://example.com/next")
	policy := DestinationPolicy{
		Dependency: "news.fetch", AllowedHosts: []string{"example.com"},
		AllowHTTP: true, AllowRedirects: true,
	}
	ctx := context.WithValue(context.Background(), policyContextKey{}, policy)
	previous := &http.Request{URL: previousURL, Header: make(http.Header)}
	next := (&http.Request{URL: nextURL, Header: make(http.Header)}).WithContext(ctx)
	if err := validateRedirect(next, []*http.Request{previous}); !errors.Is(err, errRedirectRejected) {
		t.Fatalf("expected HTTPS downgrade to be rejected, got %v", err)
	}
}

func newLocalTestClient(t *testing.T, server *httptest.Server, resolver Resolver, maxResponseBytes int64, maxRetries int) *Client {
	t.Helper()
	client, err := New(Config{
		Resolver: resolver, MaxResponseBytes: maxResponseBytes,
		MaxRetries: maxRetries, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)
	return client
}

func localRequest(t *testing.T, server *httptest.Server, path string) *http.Request {
	t.Helper()
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	requestURL := "http://example.com:" + serverURL.Port() + path
	request, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	return request
}

func localPolicy(server *httptest.Server) DestinationPolicy {
	serverURL, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(serverURL.Port())
	return DestinationPolicy{
		Dependency: "news.fetch", AllowedHosts: []string{"example.com"},
		AllowedPorts: []uint16{uint16(port)}, AllowHTTP: true,
	}
}

func TestClientExecutesRequestThroughValidatedIPDial(t *testing.T) {
	expectedHost := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Host != expectedHost {
			t.Errorf("expected Host %q, got %q", expectedHost, request.Host)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	parsedServerURL, _ := url.Parse(server.URL)
	expectedHost = "example.com:" + parsedServerURL.Port()
	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}, 1024, 0)
	var dialAddress string
	client.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		dialAddress = address
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	response, err := client.Do(context.Background(), localRequest(t, server, "/ok"), localPolicy(server))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || string(body) != "ok" {
		t.Fatalf("unexpected response body %q, err=%v", body, err)
	}
	if dialAddress != "8.8.8.8:"+parsedServerURL.Port() {
		t.Fatalf("expected dial to validated IP, got %q", dialAddress)
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("12345"))
	}))
	defer server.Close()

	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}, 4, 0)
	_, err := client.Do(context.Background(), localRequest(t, server, "/large"), localPolicy(server))
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.response_too_large" {
		t.Fatalf("expected response size fault, got %v", err)
	}
}

func TestClientDeniesRedirectUnlessExplicitlyEnabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/start" {
			http.Redirect(w, request, "/next", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("next"))
	}))
	defer server.Close()

	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}, 1024, 0)
	_, err := client.Do(context.Background(), localRequest(t, server, "/start"), localPolicy(server))
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.destination_rejected" {
		t.Fatalf("expected redirect rejection fault, got %v", err)
	}
}

func TestClientAllowsRedirectOnlyWithinExplicitPolicy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/start" {
			http.Redirect(w, request, "/next", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte("next"))
	}))
	defer server.Close()

	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}, 1024, 0)
	policy := localPolicy(server)
	policy.AllowRedirects = true
	policy.MaxRedirects = 1
	response, err := client.Do(context.Background(), localRequest(t, server, "/start"), policy)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(response.Body)
	if string(body) != "next" {
		t.Fatalf("unexpected redirect response %q", body)
	}
}

func TestClientRevalidatesDNSAfterConnectionClose(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Connection", "close")
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	resolver := &sequenceResolver{addresses: [][]netip.Addr{
		{netip.MustParseAddr("8.8.8.8")},
		{netip.MustParseAddr("10.0.0.8")},
	}}
	client := newLocalTestClient(t, server, resolver, 1024, 0)
	policy := localPolicy(server)
	response, err := client.Do(context.Background(), localRequest(t, server, "/first"), policy)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	_, err = client.Do(context.Background(), localRequest(t, server, "/second"), policy)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.destination_rejected" {
		t.Fatalf("expected second DNS resolution to be rejected, got %v", err)
	}
}

func TestClientReleasesDependencySlotDuringRetryBackoff(t *testing.T) {
	var calls atomic.Int32
	firstAttempt := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			close(firstAttempt)
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		MaxRetries: 1, MaxConcurrent: 1, DependencyLimits: map[string]int{"news.fetch": 1},
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
	firstDone := make(chan error, 1)
	go func() {
		response, requestErr := client.Do(context.Background(), localRequest(t, server, "/retry-first"), policy)
		if requestErr == nil {
			_ = response.Body.Close()
		}
		firstDone <- requestErr
	}()
	<-firstAttempt

	secondDone := make(chan error, 1)
	go func() {
		response, requestErr := client.Do(context.Background(), localRequest(t, server, "/concurrent"), policy)
		if requestErr == nil {
			_ = response.Body.Close()
		}
		secondDone <- requestErr
	}()
	select {
	case err := <-secondDone:
		if err != nil {
			t.Fatalf("concurrent request failed during retry backoff: %v", err)
		}
	case <-time.After(80 * time.Millisecond):
		t.Fatal("concurrent request was blocked by another request's retry backoff")
	}
	if err := <-firstDone; err != nil {
		t.Fatalf("retried request failed: %v", err)
	}
	if got := calls.Load(); got != 3 {
		t.Fatalf("expected initial, concurrent, and retry requests; got %d", got)
	}
}

func TestClientCancellationWhileWaitingForDependencySlotDoesNotLeak(t *testing.T) {
	entered := make(chan struct{})
	releaseFirst := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/hold" {
			close(entered)
			select {
			case <-releaseFirst:
			case <-request.Context().Done():
			}
		}
		calls.Add(1)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		MaxConcurrent: 1, DependencyLimits: map[string]int{"news.fetch": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)
	policy := localPolicy(server)

	firstDone := make(chan error, 1)
	go func() {
		response, err := client.Do(context.Background(), localRequest(t, server, "/hold"), policy)
		if err == nil {
			_ = response.Body.Close()
		}
		firstDone <- err
	}()
	<-entered

	waitingCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Do(waitingCtx, localRequest(t, server, "/waiting"), policy)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.canceled" {
		t.Fatalf("expected canceled request while waiting for slot, got %v", err)
	}

	close(releaseFirst)
	if err := <-firstDone; err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	response, err := client.Do(context.Background(), localRequest(t, server, "/after-cancel"), policy)
	if err != nil {
		t.Fatalf("slot was not available after cancellation: %v", err)
	}
	_ = response.Body.Close()
	if got := calls.Load(); got != 2 {
		t.Fatalf("canceled waiter should not reach server; got %d handler calls", got)
	}
}

func TestClientRetriesOnlyExplicitlyIdempotentRequests(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}, 1024, 1)
	policy := localPolicy(server)
	policy.Idempotent = true
	response, err := client.Do(context.Background(), localRequest(t, server, "/retry"), policy)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("expected one retry, got %d requests", calls.Load())
	}
}

func TestClientPropagatesRequestIDFromContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if got := request.Header.Get(observability.RequestIDHeader); got != "request-789" {
			t.Errorf("request ID = %q", got)
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}, 1024, 0)
	ctx := observability.WithRequestID(context.Background(), "request-789")
	response, err := client.Do(ctx, localRequest(t, server, "/request-id"), localPolicy(server))
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
}

func TestResolveRejectsMixedIPv4AndIPv6Answers(t *testing.T) {
	client, err := New(Config{Resolver: staticResolver{addresses: []netip.Addr{
		netip.MustParseAddr("8.8.8.8"),
		netip.MustParseAddr("fd00::1"),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	policy := DestinationPolicy{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}}
	if _, err := client.resolveAndValidate(context.Background(), "example.com", "443", policy); !errors.Is(err, errPolicyRejected) {
		t.Fatalf("expected mixed IPv4/IPv6 DNS answers to be rejected, got %v", err)
	}
}

func TestClientDoesNotDialWhenAnyDNSAnswerIsPrivate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("unexpected"))
	}))
	defer server.Close()

	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{
		netip.MustParseAddr("8.8.8.8"),
		netip.MustParseAddr("10.0.0.8"),
	}}, 1024, 0)
	var dialCalls atomic.Int32
	client.dial = func(context.Context, string, string) (net.Conn, error) {
		dialCalls.Add(1)
		return nil, errors.New("dial must not be called for rejected DNS answers")
	}

	_, err := client.Do(context.Background(), localRequest(t, server, "/blocked"), localPolicy(server))
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.destination_rejected" {
		t.Fatalf("expected destination rejection, got %v", err)
	}
	if dialCalls.Load() != 0 {
		t.Fatalf("expected no network dial, got %d", dialCalls.Load())
	}
}

func TestClientRevalidatesDNSForRedirectTargetBeforeDial(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		if request.URL.Path == "/start" {
			http.Redirect(w, request, "http://redirect.example:"+request.URL.Query().Get("port")+"/private", http.StatusFound)
			return
		}
		t.Fatal("redirect target must not be reached")
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	resolver := &sequenceResolver{addresses: [][]netip.Addr{
		{netip.MustParseAddr("8.8.8.8")},
		{netip.MustParseAddr("10.0.0.8")},
	}}
	client := newLocalTestClient(t, server, resolver, 1024, 0)
	var dialCalls atomic.Int32
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		dialCalls.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}

	request, err := http.NewRequest(http.MethodGet, "http://example.com:"+serverURL.Port()+"/start?port="+serverURL.Port(), nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := localPolicy(server)
	policy.AllowedHosts = []string{"example.com", "redirect.example"}
	policy.AllowRedirects = true
	policy.MaxRedirects = 1

	_, err = client.Do(context.Background(), request, policy)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.destination_rejected" {
		t.Fatalf("expected redirect target DNS rejection, got %v", err)
	}
	if requests.Load() != 1 {
		t.Fatalf("expected only the initial request to reach the server, got %d", requests.Load())
	}
	if dialCalls.Load() != 1 {
		t.Fatalf("expected only the initial validated connection to dial, got %d", dialCalls.Load())
	}
	if resolver.index != 2 {
		t.Fatalf("expected DNS to be resolved for both hops, got %d lookups", resolver.index)
	}
}

func TestClientRejectsRedirectToHostOutsideAllowlist(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		http.Redirect(w, request, "http://not-allowed.example:"+request.URL.Query().Get("port")+"/private", http.StatusFound)
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{
		netip.MustParseAddr("8.8.8.8"),
	}}, 1024, 0)
	var dialCalls atomic.Int32
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		dialCalls.Add(1)
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}

	request, err := http.NewRequest(http.MethodGet, "http://example.com:"+serverURL.Port()+"/start?port="+serverURL.Port(), nil)
	if err != nil {
		t.Fatal(err)
	}
	policy := localPolicy(server)
	policy.AllowRedirects = true
	policy.MaxRedirects = 1
	_, err = client.Do(context.Background(), request, policy)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.destination_rejected" {
		t.Fatalf("expected unlisted redirect host to be rejected, got %v", err)
	}
	if requests.Load() != 1 || dialCalls.Load() != 1 {
		t.Fatalf("unexpected follow-up network activity: requests=%d dials=%d", requests.Load(), dialCalls.Load())
	}
}

func TestValidatePolicyRejectsWildcardAndInvalidPorts(t *testing.T) {
	for _, policy := range []DestinationPolicy{
		{Dependency: "news.fetch", AllowedHosts: []string{"*.example.com"}},
		{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}, AllowedPorts: []uint16{0}},
		{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}, MaxRedirects: 1},
		{Dependency: "news.fetch", AllowedHosts: []string{"example.com"}, AllowRedirects: true, MaxRedirects: 4},
	} {
		if err := validatePolicy(policy); !errors.Is(err, errPolicyRejected) {
			t.Errorf("expected policy rejection for %+v, got %v", policy, err)
		}
	}
}

func TestDialContextRequiresRequestPolicyContext(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	var dialCalls atomic.Int32
	client.dial = func(context.Context, string, string) (net.Conn, error) {
		dialCalls.Add(1)
		return nil, errors.New("dial must not be called")
	}
	_, err = client.dialContext(context.Background(), "tcp", "example.com:443")
	if !errors.Is(err, errPolicyRejected) {
		t.Fatalf("expected missing policy to be rejected, got %v", err)
	}
	if dialCalls.Load() != 0 {
		t.Fatalf("expected no dial, got %d", dialCalls.Load())
	}
}

func TestClientReturnsTimeoutWhenRetryBackoffExceedsDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver: staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		Timeout:  20 * time.Millisecond, MaxRetries: 1,
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
	_, err = client.Do(context.Background(), localRequest(t, server, "/retry-timeout"), policy)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.timeout" {
		t.Fatalf("expected timeout fault after retry deadline, got %v", err)
	}
}


func TestRedirectRejectsCrossOriginEvenWhenBothHostsAreAllowed(t *testing.T) {
	previousURL, _ := url.Parse("https://api.example.com/start")
	nextURL, _ := url.Parse("https://cdn.example.com/next")
	policy := DestinationPolicy{
		Dependency: "news.fetch",
		AllowedHosts: []string{"api.example.com", "cdn.example.com"},
		AllowRedirects: true,
		MaxRedirects: 1,
	}
	ctx := context.WithValue(context.Background(), policyContextKey{}, policy)
	previous := &http.Request{URL: previousURL, Header: make(http.Header)}
	next := (&http.Request{URL: nextURL, Header: make(http.Header)}).WithContext(ctx)
	if err := validateRedirect(next, []*http.Request{previous}); !errors.Is(err, errRedirectRejected) {
		t.Fatalf("expected cross-origin redirect rejection, got %v", err)
	}
}

func TestRedirectAllowsSameOriginPathChange(t *testing.T) {
	previousURL, _ := url.Parse("https://api.example.com/start")
	nextURL, _ := url.Parse("https://api.example.com/next")
	policy := DestinationPolicy{
		Dependency: "news.fetch",
		AllowedHosts: []string{"api.example.com"},
		AllowRedirects: true,
		MaxRedirects: 1,
	}
	ctx := context.WithValue(context.Background(), policyContextKey{}, policy)
	previous := &http.Request{URL: previousURL, Header: make(http.Header)}
	next := (&http.Request{URL: nextURL, Header: make(http.Header)}).WithContext(ctx)
	if err := validateRedirect(next, []*http.Request{previous}); err != nil {
		t.Fatalf("expected same-origin redirect to pass, got %v", err)
	}
}

func TestClientUsesBoundedSemaphorePoolsForUnknownDependencies(t *testing.T) {
	client, err := New(Config{
		MaxConcurrent: 2,
		DependencyLimits: map[string]int{
			"news.provider.bocha": 1,
			"news.provider.brave": 2,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	bocha := client.semaphore("news.provider.bocha")
	brave := client.semaphore("news.provider.brave")
	unknownA := client.semaphore("request.user.1001")
	unknownB := client.semaphore("request.user.1002")

	if bocha == brave || bocha == unknownA || brave == unknownA {
		t.Fatal("configured dependencies must have dedicated semaphore pools")
	}
	if unknownA != unknownB {
		t.Fatal("unconfigured dependencies must share the bounded fallback pool")
	}
	if got, want := len(client.semaphores), 2; got != want {
		t.Fatalf("configured semaphore pool count = %d, want %d", got, want)
	}
	if got, want := cap(client.defaultSemaphore), 2; got != want {
		t.Fatalf("fallback semaphore capacity = %d, want %d", got, want)
	}
}

func TestClientHoldsDependencySlotWhileReadingResponseBody(t *testing.T) {
	bodyStarted := make(chan struct{})
	releaseBody := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseBody) })
	}
	defer release()
	secondReached := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/first" {
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(bodyStarted)
			<-releaseBody
			_, _ = w.Write([]byte("first"))
			return
		}
		secondReached <- struct{}{}
		_, _ = w.Write([]byte("second"))
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver:         staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		MaxResponseBytes: 1024,
		DependencyLimits: map[string]int{"news.provider.bocha": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)

	policy := localPolicy(server)
	policy.Dependency = "news.provider.bocha"
	firstRequest := localRequest(t, server, "/first")
	firstDone := make(chan error, 1)
	go func() {
		response, requestErr := client.Do(context.Background(), firstRequest, policy)
		if requestErr == nil {
			_ = response.Body.Close()
		}
		firstDone <- requestErr
	}()
	select {
	case <-bodyStarted:
	case <-time.After(time.Second):
		t.Fatal("first response body did not start")
	}

	secondRequest := localRequest(t, server, "/second")
	secondDone := make(chan error, 1)
	go func() {
		response, requestErr := client.Do(context.Background(), secondRequest, policy)
		if requestErr == nil {
			_ = response.Body.Close()
		}
		secondDone <- requestErr
	}()
	select {
	case <-secondReached:
		t.Fatal("second request reached the server before the first response body was consumed")
	case requestErr := <-secondDone:
		t.Fatalf("second request should wait for the dependency slot, got %v", requestErr)
	case <-time.After(80 * time.Millisecond):
	}

	release()
	if err := <-firstDone; err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	select {
	case <-secondReached:
	case <-time.After(time.Second):
		t.Fatal("second request did not proceed after the first response body completed")
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("second request failed: %v", err)
	}
}

func TestClientCloseIdleConnectionsDoesNotCancelActiveOrFutureRequests(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	var held atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/hold" && held.CompareAndSwap(false, true) {
			close(entered)
			<-release
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newLocalTestClient(t, server, staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}}, 1024, 0)
	firstDone := make(chan error, 1)
	go func() {
		response, err := client.Do(context.Background(), localRequest(t, server, "/hold"), localPolicy(server))
		if err == nil {
			_ = response.Body.Close()
		}
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("active request did not reach the server")
	}

	client.CloseIdleConnections()
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("closing idle connections canceled an active request: %v", err)
	}

	response, err := client.Do(context.Background(), localRequest(t, server, "/after-close"), localPolicy(server))
	if err != nil {
		t.Fatalf("client could not issue a request after CloseIdleConnections: %v", err)
	}
	_ = response.Body.Close()
}

func TestClientReleasesDependencySlotAfterResponseReadFailure(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path == "/large" {
			_, _ = w.Write([]byte("12345"))
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver:         staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		MaxResponseBytes: 4,
		DependencyLimits: map[string]int{"news.provider.bocha": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)

	policy := localPolicy(server)
	policy.Dependency = "news.provider.bocha"
	_, err = client.Do(context.Background(), localRequest(t, server, "/large"), policy)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.response_too_large" {
		t.Fatalf("expected response size fault, got %v", err)
	}

	response, err := client.Do(context.Background(), localRequest(t, server, "/after-large"), policy)
	if err != nil {
		t.Fatalf("dependency slot was not released after response read failure: %v", err)
	}
	_ = response.Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("expected both requests to reach the server, got %d", calls.Load())
	}
}

func TestClientReleasesDependencySlotAfterResponseReadCancellation(t *testing.T) {
	bodyStarted := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Path == "/cancel" {
			w.WriteHeader(http.StatusOK)
			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}
			close(bodyStarted)
			<-request.Context().Done()
			return
		}
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client, err := New(Config{
		Resolver:         staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		Timeout:          2 * time.Second,
		DependencyLimits: map[string]int{"news.provider.bocha": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.dial = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
	}
	t.Cleanup(client.CloseIdleConnections)

	policy := localPolicy(server)
	policy.Dependency = "news.provider.bocha"
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancelRequest := localRequest(t, server, "/cancel")
	cancelDone := make(chan error, 1)
	go func() {
		_, requestErr := client.Do(ctx, cancelRequest, policy)
		cancelDone <- requestErr
	}()
	select {
	case <-bodyStarted:
	case <-time.After(time.Second):
		t.Fatal("cancelable response body did not start")
	}
	cancel()

	select {
	case requestErr := <-cancelDone:
		if requestErr == nil {
			t.Fatal("expected response body read to fail after cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled response body read did not return")
	}

	response, err := client.Do(context.Background(), localRequest(t, server, "/after-cancel"), policy)
	if err != nil {
		t.Fatalf("dependency slot was not released after response read cancellation: %v", err)
	}
	_ = response.Body.Close()
	if calls.Load() != 2 {
		t.Fatalf("expected canceled and follow-up requests to reach the server, got %d", calls.Load())
	}
}
