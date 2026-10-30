package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTencentStockEndpointsRejectNonStockSecuritiesBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unsupported security should be rejected before upstream request", http.StatusBadRequest)
	}))
	defer server.Close()

	p := NewTencent(2 * time.Second)
	p.quoteURL = server.URL + "/quote"
	p.klineURL = server.URL + "/kline-adjusted"
	p.noneKlineURL = server.URL + "/kline"
	ctx := context.Background()
	calls := []struct {
		name string
		call func(string) error
	}{
		{"quote", func(symbol string) error { _, err := p.FetchQuote(ctx, symbol); return err }},
		{"daily_bars", func(symbol string) error {
			_, err := p.FetchDailyBars(ctx, symbol, "2026-10-20", "2026-10-26", 10, model.AdjustmentNone)
			return err
		}},
	}
	for _, symbol := range []string{"510300.SH", "159919.SZ", "399001.SZ", "113001.SH", "900901.SH"} {
		for _, call := range calls {
			if err := call.call(symbol); err == nil {
				t.Errorf("%s must reject non-common-stock symbol %s", call.name, symbol)
			}
		}
	}
	if requests != 0 {
		t.Fatalf("unsupported securities must be rejected before Tencent upstream access; got %d request(s)", requests)
	}
}
