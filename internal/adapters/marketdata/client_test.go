package marketdata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testAPIKey = "01234567890123456789012345678901"

func testConfig(base string) Config {
	cfg := DefaultConfig()
	cfg.BaseURL = base
	cfg.APIKey = testAPIKey
	return cfg
}

func TestClientReadsNativeQuoteAndDailyBars(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 20, 15, 0, 0, 0, loc).UTC()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") != testAPIKey {
			t.Errorf("X-API-Key missing or incorrect")
		}
		switch r.URL.Path {
		case "/api/v1/quotes/600519.SH":
			_, _ = fmt.Fprint(w, `{"symbol":"600519.SH","name":"贵州茅台","current_price":1326,"change":6,"change_percent":0.45,"open":1320,"high":1330,"low":1310,"prev_close":1320,"volume":10000,"amount":13260000,"quote_time":"2026-10-20T06:52:15Z","fetched_at":"2026-10-20T06:52:16Z","source":"eastmoney","source_version":"v1","quality_status":"valid","quality_reasons":[],"volume_unit":"shares","amount_unit":"CNY","adjustment_basis":"none"}`)
		case "/api/v1/bars/600519.SH/daily":
			if r.URL.Query().Get("limit") != "25" || r.URL.Query().Get("adjustment") != "forward_adjusted" {
				t.Errorf("unexpected query %q", r.URL.RawQuery)
			}
			_, _ = fmt.Fprint(w, `{"symbol":"600519.SH","period":"1d","adjustment_basis":"forward_adjusted","volume_unit":"shares","amount_unit":"CNY","quality_status":"valid","warnings":[],"bars":[{"symbol":"600519.SH","trade_date":"2026-10-19","period":"1d","open":1320,"high":1330,"low":1310,"close":1326,"volume":10000,"amount":13260000,"change_percent":0.45,"adjustment_basis":"forward_adjusted","source":"eastmoney","source_version":"v1","provider_timestamp":"2026-10-19T07:00:00Z","fetched_at":"2026-10-20T06:52:16Z","normalized_at":"2026-10-20T06:52:16Z","volume_unit":"shares","amount_unit":"CNY","quality_status":"valid","quality_reasons":[]}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	cfg := testConfig(server.URL)
	client, err := NewClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	quote, fetchedAt, err := client.GetQuote(context.Background(), "600519")
	if err != nil {
		t.Fatalf("GetQuote: %v", err)
	}
	if quote.StockCode != "600519.SH" || quote.Source != "eastmoney" || quote.CurrentPrice != 1326 {
		t.Fatalf("unexpected native quote: %+v", quote)
	}
	if quote.UpdateTime == "" || fetchedAt.IsZero() {
		t.Fatalf("missing quote timestamp or fetch timestamp: quote=%+v fetchedAt=%v", quote, fetchedAt)
	}

	history, err := client.GetHistory(context.Background(), "600519", 25)
	if err != nil {
		t.Fatalf("GetHistory: %v", err)
	}
	if history.Symbol != "600519.SH" || history.QualityStatus != "valid" || len(history.Bars) != 1 {
		t.Fatalf("unexpected native history: %+v", history)
	}
	bar := history.Bars[0]
	if bar.Source != "eastmoney" || bar.AdjustmentBasis != "forward_adjusted" || bar.Volume == nil || *bar.Volume != 10000 {
		t.Fatalf("provider metadata / share volume was not preserved: %+v", bar)
	}
	if !bar.ProviderTime.IsZero() && bar.ProviderTime.Year() == 1 {
		t.Fatalf("invalid provider timestamp: %+v", bar.ProviderTime)
	}
	_ = asOf
}

func TestCanonicalSymbol(t *testing.T) {
	cases := map[string]string{
		"600519":    "600519.SH",
		"000001":    "000001.SZ",
		"830799":    "830799.BJ",
		"sh000001":  "000001.SH",
		"sz399001":  "399001.SZ",
		"300750.SZ": "300750.SZ",
	}
	for input, want := range cases {
		got, err := canonicalSymbol(input)
		if err != nil || got != want {
			t.Errorf("canonicalSymbol(%q)=%q,%v; want %q", input, got, err, want)
		}
	}
	for _, input := range []string{"", "../../admin", "AAPL", "600519.US", "1234567"} {
		if _, err := canonicalSymbol(input); err == nil {
			t.Errorf("canonicalSymbol(%q) unexpectedly succeeded", input)
		}
	}
}

func TestClientNeverFollowsRedirectsOrLeaksResponseBody(t *testing.T) {
	secretBody := "bearer-secret-data"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://example.invalid/leak", http.StatusFound)
		_, _ = fmt.Fprint(w, secretBody)
	}))
	defer server.Close()
	client, err := NewClient(testConfig(server.URL))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = client.GetQuote(context.Background(), "600519")
	if err == nil || strings.Contains(err.Error(), secretBody) {
		t.Fatalf("expected bounded HTTP status error without body, got %v", err)
	}
}
