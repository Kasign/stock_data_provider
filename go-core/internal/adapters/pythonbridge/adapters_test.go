package pythonbridge

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/adapters/marketdatahttp"
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

func TestGoMarketDataAdapterFallsBackWhenNativeOverviewIsUnavailable(t *testing.T) {
	nativeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "native component unavailable", http.StatusBadGateway)
	}))
	defer nativeServer.Close()
	bridgeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/internal/v1/market-data/market-overview" {
			t.Errorf("unexpected bridge request: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"region":"cn","date":"2026-10-27","indices":[{"code":"000001","name":"上证指数","current":3250,"change":5,"change_pct":0.15}],"warnings":[]}`))
	}))
	defer bridgeServer.Close()

	native, err := marketdatahttp.NewClient(nativeServer.URL, "market-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := NewClient(Config{BaseURL: bridgeServer.URL, Token: "bridge-token", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := NewGoMarketDataAdapter(native, bridge)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.FetchMarketOverview(context.Background(), "cn", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Indices) != 1 || got.Indices[0].Code != "000001" {
		t.Fatalf("expected compatibility result, got %+v", got)
	}
	found := false
	for _, warning := range got.Warnings {
		if warning == "native_market_overview_unavailable_legacy_data_used" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fallback must be explicit in warnings: %+v", got.Warnings)
	}
}
