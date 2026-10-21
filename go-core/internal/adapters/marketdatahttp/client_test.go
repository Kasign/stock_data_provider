package marketdatahttp

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchStockBundlePreservesUnitsSourceAndQuality(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer market-secret" {
			t.Errorf("missing auth header")
		}
		if !strings.HasPrefix(r.URL.Path, "/api/v1/bundles/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("adjustment") != "forward_adjusted" {
			t.Errorf("unexpected adjustment=%s", r.URL.Query().Get("adjustment"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "symbol":"600519.SH","name":"贵州茅台","fetched_at":"2026-10-21T03:28:02Z",
		  "quality_status":"degraded","sources":{"daily_bars":"eastmoney"},"warnings":["quote_and_daily_bars_adjustment_basis_mismatch"],
		  "quote":{"symbol":"600519.SH","name":"贵州茅台","current_price":1313.31,"change":-18.57,"change_percent":-1.394,"open":1331.88,"high":1331.98,"low":1312,"prev_close":1331.88,"volume":3413600,"amount":4525398959,"quote_time":"2026-10-21T03:28:02Z","fetched_at":"2026-10-21T03:28:02Z","normalized_at":"2026-10-21T03:28:02Z","source":"eastmoney","volume_unit":"shares","amount_unit":"CNY","adjustment_basis":"none","quality_status":"valid"},
		  "daily_bars":[{"symbol":"600519.SH","trade_date":"2026-10-20","period":"1d","open":1321.01,"high":1335,"low":1316.14,"close":1331.88,"volume":3413600,"amount":4525398959,"change_percent":0.81,"adjustment_basis":"forward_adjusted","source":"eastmoney","volume_unit":"shares","amount_unit":"CNY","quality_status":"valid"}]
		}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL, "market-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := client.FetchStockBundle(context.Background(), "600519", 60, "2026-10-21T03:28:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Code != "600519" {
		t.Fatalf("code=%s", bundle.Code)
	}
	if bundle.Quote == nil || bundle.Quote.Volume == nil || *bundle.Quote.Volume != 3413600 {
		t.Fatalf("quote volume not mapped to shares: %+v", bundle.Quote)
	}
	if bundle.Quote.VolumeUnit != "shares" || bundle.Quote.AmountUnit != "CNY" || bundle.Quote.Source != "eastmoney" {
		t.Fatalf("quote metadata lost: %+v", bundle.Quote)
	}
	if len(bundle.DailyBars) != 1 || bundle.DailyBars[0].AdjustmentBasis != "forward_adjusted" {
		t.Fatalf("bar metadata lost: %+v", bundle.DailyBars)
	}
	if bundle.DailyBars[0].Amount != 4525398959 || bundle.DailyBars[0].Volume != 3413600 {
		t.Fatalf("bar values not mapped: %+v", bundle.DailyBars[0])
	}
	if bundle.QualityStatus != "degraded" || len(bundle.Warnings) == 0 {
		t.Fatalf("quality metadata lost: %+v", bundle)
	}
}

func TestNewClientRejectsURLCredentialsAndMissingKey(t *testing.T) {
	if _, err := NewClient("http://user:pass@localhost:8091", "x", time.Second); err == nil {
		t.Fatal("expected credentials in URL to be rejected")
	}
	if _, err := NewClient("http://localhost:8091", "", time.Second); err == nil {
		t.Fatal("expected missing API key to be rejected")
	}
}

func TestClientRejectsTrailingJSONValues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{} {}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "market-secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := client.get(context.Background(), "/invalid", &result); err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("expected multiple-JSON-values rejection, got %v", err)
	}
}

func TestClientRejectsOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"padding":"%s"}`, strings.Repeat("x", int(maxMarketDataResponseBytes)))))
	}))
	defer server.Close()
	client, err := NewClient(server.URL, "market-secret", 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := client.get(context.Background(), "/large", &result); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected response size-limit rejection, got %v", err)
	}
}
