package httpclient

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"testing"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

func TestBindRejectsInvalidDestinationPolicy(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Bind(DestinationPolicy{Dependency: "news.provider.bocha"}); err == nil {
		t.Fatal("expected empty host allowlist to be rejected")
	}
}

func TestBoundDoerKeepsDestinationPolicyAndUsesRequestContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	client := newLocalTestClient(
		t,
		server,
		staticResolver{addresses: []netip.Addr{netip.MustParseAddr("8.8.8.8")}},
		1024,
		0,
	)
	policy := localPolicy(server)
	bound, err := client.Bind(policy)
	if err != nil {
		t.Fatal(err)
	}

	// Mutating the caller's slices must not widen or replace the bound policy.
	policy.AllowedHosts[0] = "attacker.example"

	request := localRequest(t, server, "/ok")
	response, err := bound.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()

	attackerRequest, err := http.NewRequest(http.MethodGet, "http://attacker.example:"+request.URL.Port()+"/denied", nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = bound.Do(attackerRequest)
	var platformFault *fault.Fault
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.destination_rejected" {
		t.Fatalf("expected bound host policy rejection, got %v", err)
	}

	canceledContext, cancel := context.WithCancel(context.Background())
	cancel()
	canceledRequest := localRequest(t, server, "/canceled").WithContext(canceledContext)
	_, err = bound.Do(canceledRequest)
	if !errors.As(err, &platformFault) || platformFault.Code != "platform.httpclient.canceled" {
		t.Fatalf("expected request context cancellation, got %v", err)
	}
}

func TestBoundDoerRejectsNilRequest(t *testing.T) {
	client, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := client.Bind(DestinationPolicy{
		Dependency:   "news.provider.bocha",
		AllowedHosts: []string{"api.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bound.Do(nil); err == nil {
		t.Fatal("expected nil request to be rejected")
	}
}

var _ interface {
	Do(*http.Request) (*http.Response, error)
} = (*BoundDoer)(nil)
