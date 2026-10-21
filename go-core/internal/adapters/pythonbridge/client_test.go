package pythonbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientSendsTokenAndDecodesJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Python-Bridge-Token") != "test-token" {
			t.Errorf("missing bridge token")
		}
		if r.URL.Path != "/internal/v1/test" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Token: "test-token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]string
	if err := client.Post(context.Background(), "/internal/v1/test", map[string]string{"input": "test"}, &out); err != nil {
		t.Fatal(err)
	}
	if out["status"] != "ok" {
		t.Fatalf("unexpected response: %#v", out)
	}
}

func TestClientRequiresToken(t *testing.T) {
	if _, err := NewClient(Config{BaseURL: "http://localhost:8090"}); err == nil {
		t.Fatal("expected token validation error")
	}
}

func TestClientRejectsCredentialsAndQueryInBaseURL(t *testing.T) {
	for _, baseURL := range []string{
		"http://user:pass@localhost:8090",
		"http://localhost:8090?token=secret",
		"ftp://localhost:8090",
		"http:///missing-host",
	} {
		if _, err := NewClient(Config{BaseURL: baseURL, Token: "token"}); err == nil {
			t.Errorf("expected invalid base URL to be rejected: %q", baseURL)
		}
	}
}

func TestClientDoesNotFollowRedirectsWithBridgeToken(t *testing.T) {
	var redirected bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected = true
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()
	redirector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Python-Bridge-Token") != "token" {
			t.Errorf("initial request missing token")
		}
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer redirector.Close()
	client, err := NewClient(Config{BaseURL: redirector.URL, Token: "token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Get(context.Background(), "/healthz", nil)
	if err == nil {
		t.Fatal("expected redirect response to be returned as an error")
	}
	if redirected {
		t.Fatal("client followed redirect; bridge token may have leaked")
	}
}

func TestClientConvertsHTTPFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"code":"provider_failed","message":"upstream unavailable"}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Token: "token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Post(context.Background(), "/bad", map[string]string{}, nil)
	if _, ok := err.(*APIError); !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
}

func TestClientParsesFastAPIErrorDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"detail":{"code":"stock_news_search_failed","message":"news search failed: TimeoutError"}}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Token: "token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	err = client.Post(context.Background(), "/bad", map[string]string{}, nil)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T: %v", err, err)
	}
	if apiErr.Code != "stock_news_search_failed" || apiErr.Message != "news search failed: TimeoutError" {
		t.Fatalf("FastAPI error detail was lost: %#v", apiErr)
	}
}

func TestMarketReviewNotificationTreatsNoSuccessfulChannelAsFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/notification/market-review" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sent":false}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Token: "token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewNotificationAdapter(client)
	if err := adapter.NotifyMarketReview(context.Background(), "report", "cn", "query-1"); err == nil {
		t.Fatal("notification adapter must report failure when no channel accepted the notification")
	}
}
