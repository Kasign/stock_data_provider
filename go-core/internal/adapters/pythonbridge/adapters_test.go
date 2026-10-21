package pythonbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestMarketDataAdapterOmitsEmptyAsOfFromLegacyRequests(t *testing.T) {
	paths := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if _, ok := body["as_of"]; ok {
			t.Errorf("empty as_of must be omitted for %s: %#v", r.URL.Path, body)
		}
		paths[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	client, err := NewClient(Config{BaseURL: server.URL, Token: "token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	adapter := NewMarketDataAdapter(client)
	if _, err := adapter.FetchStockBundle(context.Background(), "600519", 60, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.FetchMarketOverview(context.Background(), "cn", ""); err != nil {
		t.Fatal(err)
	}
	if !paths["/internal/v1/market-data/stock-bundle"] || !paths["/internal/v1/market-data/market-overview"] {
		t.Fatalf("not all routes were tested: %#v", paths)
	}
}
