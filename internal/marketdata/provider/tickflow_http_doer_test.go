package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type recordingTickFlowHTTPDoer struct {
	status int
	calls  int
	method string
	url    string
	header http.Header
}

func (d *recordingTickFlowHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	d.calls++
	d.method = request.Method
	d.url = request.URL.String()
	d.header = request.Header.Clone()
	return &http.Response{
		StatusCode: d.status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader("upstream unavailable")),
		Request:    request,
	}, nil
}

func TestTickFlowUsesSeparateAuthenticatedAndPublicHTTPDoers(t *testing.T) {
	const (
		apiURL     = "https://api.tickflow.example"
		freeAPIURL = "https://free.tickflow.example"
		apiKey     = "test-tickflow-key"
	)
	authenticated := &recordingTickFlowHTTPDoer{status: http.StatusBadGateway}
	public := &recordingTickFlowHTTPDoer{status: http.StatusBadGateway}
	provider := NewTickFlowConfiguredWithHTTPDoers(time.Second, apiURL, freeAPIURL, apiKey, authenticated, public)

	_, err := provider.getJSON(context.Background(), apiURL, "/v1/quotes", nil, true)
	if err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("authenticated getJSON() error = %v, want HTTP 502", err)
	}
	if authenticated.calls != 1 || public.calls != 0 {
		t.Fatalf("authenticated calls = %d, public calls = %d; want 1 and 0", authenticated.calls, public.calls)
	}
	if authenticated.method != http.MethodGet || authenticated.url != apiURL+"/v1/quotes" {
		t.Fatalf("authenticated request = method:%q url:%q", authenticated.method, authenticated.url)
	}
	if got := authenticated.header.Get("x-api-key"); got != apiKey {
		t.Fatalf("authenticated x-api-key = %q, want configured key", got)
	}

	_, err = provider.getJSON(context.Background(), freeAPIURL, "/v1/klines", nil, false)
	if err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("public getJSON() error = %v, want HTTP 502", err)
	}
	if authenticated.calls != 1 || public.calls != 1 {
		t.Fatalf("authenticated calls = %d, public calls = %d; want 1 and 1", authenticated.calls, public.calls)
	}
	if public.method != http.MethodGet || public.url != freeAPIURL+"/v1/klines" {
		t.Fatalf("public request = method:%q url:%q", public.method, public.url)
	}
	if got := public.header.Get("x-api-key"); got != "" {
		t.Fatalf("public request unexpectedly contains x-api-key: %q", got)
	}
	if got := public.header.Get("Accept"); got != "application/json" {
		t.Fatalf("public Accept = %q, want application/json", got)
	}
}

func TestTickFlowPostUsesAuthenticatedHTTPDoer(t *testing.T) {
	const (
		apiURL = "https://api.tickflow.example"
		apiKey = "test-tickflow-key"
	)
	authenticated := &recordingTickFlowHTTPDoer{status: http.StatusBadGateway}
	public := &recordingTickFlowHTTPDoer{status: http.StatusBadGateway}
	provider := NewTickFlowConfiguredWithHTTPDoers(time.Second, apiURL, "https://free.tickflow.example", apiKey, authenticated, public)

	_, err := provider.postJSON(context.Background(), apiURL, "/v1/universes/batch", map[string]any{"ids": []string{"x"}}, true)
	if err == nil || !strings.Contains(err.Error(), "HTTP status 502") {
		t.Fatalf("postJSON() error = %v, want HTTP status 502", err)
	}
	if authenticated.calls != 1 || public.calls != 0 {
		t.Fatalf("authenticated calls = %d, public calls = %d; want 1 and 0", authenticated.calls, public.calls)
	}
	if authenticated.method != http.MethodPost || authenticated.header.Get("x-api-key") != apiKey {
		t.Fatalf("authenticated POST request = method:%q headers:%v", authenticated.method, authenticated.header)
	}
	if got := authenticated.header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
}

func TestTickFlowConfiguredKeepsDefaultHTTPClientCompatibility(t *testing.T) {
	provider := NewTickFlowConfigured(time.Second, "https://api.tickflow.example", "https://free.tickflow.example", "key")
	if provider.client == nil || provider.authenticatedHTTPDoer == nil || provider.publicHTTPDoer == nil {
		t.Fatal("default HTTP client and both fallback executors must be configured")
	}
}
