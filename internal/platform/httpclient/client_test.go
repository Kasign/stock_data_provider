package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"sync"
	"testing"
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
