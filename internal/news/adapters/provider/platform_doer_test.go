package provider

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

func TestProviderDestinationPolicyIsExactAndFailClosed(t *testing.T) {
	policy, err := providerDestinationPolicy("https://api.example.com:9443/v1/search", "news.provider.bocha")
	if err != nil {
		t.Fatalf("providerDestinationPolicy() error = %v", err)
	}
	if policy.Dependency != "news.provider.bocha" {
		t.Fatalf("Dependency = %q", policy.Dependency)
	}
	if len(policy.AllowedHosts) != 1 || policy.AllowedHosts[0] != "api.example.com" {
		t.Fatalf("AllowedHosts = %#v", policy.AllowedHosts)
	}
	if len(policy.AllowedPorts) != 1 || policy.AllowedPorts[0] != 9443 {
		t.Fatalf("AllowedPorts = %#v", policy.AllowedPorts)
	}
	if policy.AllowHTTP || policy.AllowRedirects || policy.MaxRedirects != 0 || policy.Idempotent {
		t.Fatalf("unsafe Provider policy: %#v", policy)
	}
}

func TestProviderDestinationPolicyUsesSchemeDefaultPort(t *testing.T) {
	tests := []struct {
		endpoint  string
		port      uint16
		allowHTTP bool
	}{
		{endpoint: "https://api.example.com/search", port: 443},
		{endpoint: "http://search.example.com/search", port: 80, allowHTTP: true},
	}
	for _, test := range tests {
		policy, err := providerDestinationPolicy(test.endpoint, "news.provider.test")
		if err != nil {
			t.Fatalf("providerDestinationPolicy(%q) error = %v", test.endpoint, err)
		}
		if len(policy.AllowedPorts) != 1 || policy.AllowedPorts[0] != test.port {
			t.Errorf("%q AllowedPorts = %#v, want %d", test.endpoint, policy.AllowedPorts, test.port)
		}
		if policy.AllowHTTP != test.allowHTTP {
			t.Errorf("%q AllowHTTP = %v, want %v", test.endpoint, policy.AllowHTTP, test.allowHTTP)
		}
	}
}

func TestProviderDestinationPolicyRejectsUnsafeEndpoint(t *testing.T) {
	tests := []string{
		"",
		"/relative/path",
		"file:///etc/passwd",
		"https://user:pass@api.example.com/search",
		"https://api.example.com/search?token=secret",
		"https://api.example.com/search#fragment",
		"https://api.example.com:/search",
		"https://api.example.com:70000/search",
	}
	for _, endpoint := range tests {
		t.Run(endpoint, func(t *testing.T) {
			if _, err := providerDestinationPolicy(endpoint, "news.provider.test"); !errors.Is(err, app.ErrProviderConfig) {
				t.Fatalf("providerDestinationPolicy() error = %v, want ErrProviderConfig", err)
			}
		})
	}
}

func TestProviderDestinationPolicyRequiresValidDependency(t *testing.T) {
	for _, dependency := range []string{" ", "bad dependency", strings.Repeat("a", 129)} {
		if _, err := providerDestinationPolicy("https://api.example.com/search", dependency); !errors.Is(err, app.ErrProviderConfig) {
			t.Errorf("providerDestinationPolicy(%q) error = %v, want ErrProviderConfig", dependency, err)
		}
	}
}

func TestPlatformDoerRejectsRequestOutsideBoundOrigin(t *testing.T) {
	client, err := httpclient.New(httpclient.Config{})
	if err != nil {
		t.Fatalf("httpclient.New() error = %v", err)
	}
	defer client.CloseIdleConnections()

	doer, err := NewPlatformHTTPDoer(client, "https://api.example.com/search", "news.provider.test")
	if err != nil {
		t.Fatalf("NewPlatformHTTPDoer() error = %v", err)
	}
	request, err := http.NewRequest(http.MethodGet, "https://attacker.example/search", nil)
	if err != nil {
		t.Fatalf("http.NewRequest() error = %v", err)
	}
	_, err = doer.Do(request)
	var providerErr *app.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("Do() error = %T (%v), want *app.ProviderError", err, err)
	}
	if providerErr.Code != "platform.httpclient.destination_rejected" {
		t.Fatalf("ProviderError.Code = %q", providerErr.Code)
	}
}

func TestProviderErrorMapsPlatformFaultWithoutExposingCause(t *testing.T) {
	cause := errors.New("sensitive upstream response and credential")
	platformErr := &fault.Fault{
		Class: fault.ClassUnavailable, Code: "platform.httpclient.dns_failed",
		SafeMessage: "destination DNS resolution failed", Retryable: true, Cause: cause,
	}
	mapped := providerErrorFromPlatform(platformErr)

	var providerErr *app.ProviderError
	if !errors.As(mapped, &providerErr) {
		t.Fatalf("mapped error type = %T, want *app.ProviderError", mapped)
	}
	if providerErr.Code != "platform.httpclient.dns_failed" || !providerErr.Retryable {
		t.Fatalf("mapped provider error = %#v", providerErr)
	}
	if errors.Is(mapped, cause) {
		t.Fatal("platform cause must not cross the Provider boundary")
	}
	if got := mapped.Error(); got == "" || strings.Contains(got, "sensitive") || strings.Contains(got, "credential") || strings.Contains(got, "upstream response") {
		t.Fatalf("mapped error exposed unsafe details: %q", got)
	}
}
