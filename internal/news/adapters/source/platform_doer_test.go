package source

import (
	"errors"
	"net/http"
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

func TestSourceDestinationPolicyIsExactAndRetainsFeedQuery(t *testing.T) {
	policy, err := sourceDestinationPolicy("https://feeds.example.com:9443/rss?channel=market", "news.source.market.rss")
	if err != nil {
		t.Fatalf("sourceDestinationPolicy() error = %v", err)
	}
	if policy.Dependency != "news.source.market.rss" {
		t.Fatalf("Dependency = %q", policy.Dependency)
	}
	if len(policy.AllowedHosts) != 1 || policy.AllowedHosts[0] != "feeds.example.com" {
		t.Fatalf("AllowedHosts = %#v", policy.AllowedHosts)
	}
	if len(policy.AllowedPorts) != 1 || policy.AllowedPorts[0] != 9443 {
		t.Fatalf("AllowedPorts = %#v", policy.AllowedPorts)
	}
	if policy.AllowHTTP || policy.AllowRedirects || policy.MaxRedirects != 0 || policy.Idempotent {
		t.Fatalf("unsafe Source policy: %#v", policy)
	}
}

func TestSourceDestinationPolicyRejectsUnsafeEndpoint(t *testing.T) {
	for _, endpoint := range []string{
		"",
		"/relative/path",
		"file:///etc/passwd",
		"https://user:pass@feeds.example.com/rss",
		"https://feeds.example.com/rss#fragment",
		"https://feeds.example.com:/rss",
		"https://feeds.example.com:70000/rss",
	} {
		if _, err := sourceDestinationPolicy(endpoint, "news.source.test"); !errors.Is(err, app.ErrProviderConfig) {
			t.Errorf("sourceDestinationPolicy(%q) error = %v, want ErrProviderConfig", endpoint, err)
		}
	}
}

func TestPlatformSourceDoerRejectsRequestOutsideBoundOrigin(t *testing.T) {
	client, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatalf("httpclient.New() error = %v", err)
	}
	defer client.CloseIdleConnections()

	doer, err := NewPlatformHTTPDoer(client, "https://feeds.example.com/rss", "news.source.test")
	if err != nil {
		t.Fatalf("NewPlatformHTTPDoer() error = %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "https://attacker.example/rss", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	_, err = doer.Do(request)
	var transportErr *sourceTransportError
	if !errors.As(err, &transportErr) {
		t.Fatalf("Do() error = %T (%v), want source transport error", err, err)
	}
	if transportErr.Retryable() {
		t.Fatal("destination policy rejection must not be retryable")
	}
}

func TestSourceTransportErrorPreservesPlatformRetryClassification(t *testing.T) {
	for _, retryable := range []bool{false, true} {
		platformErr := &fault.Fault{
			Class: fault.ClassUnavailable, Code: "platform.httpclient.dns_failed",
			SafeMessage: "destination DNS resolution failed", Retryable: retryable,
		}
		mapped := sourceTransportErrorFromPlatform(platformErr)
		var transportErr *sourceTransportError
		if !errors.As(mapped, &transportErr) {
			t.Fatalf("mapped error type = %T, want *sourceTransportError", mapped)
		}
		if transportErr.Retryable() != retryable {
			t.Errorf("Retryable() = %v, want %v", transportErr.Retryable(), retryable)
		}
	}
}
