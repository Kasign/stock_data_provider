package provider

import (
	"context"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestEastmoneyStockSpecificEndpointsRejectNonStockSecuritiesBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "non-stock security should be rejected before request", http.StatusBadRequest)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithStockBoards(
		2*time.Second,
		server.URL, server.URL, server.URL, server.URL, server.URL,
		server.URL, server.URL, server.URL, "",
	).WithCapitalFlowURL(server.URL)
	ctx := context.Background()
	calls := []struct {
		name string
		call func(string) error
	}{
		{"quote", func(symbol string) error { _, err := p.FetchQuote(ctx, symbol); return err }},
		{"daily_bars", func(symbol string) error { _, err := p.FetchDailyBars(ctx, symbol, "2026-10-20", "2026-10-26", 10, model.AdjustmentNone); return err }},
		{"stock_basic", func(symbol string) error { _, err := p.FetchStockBasic(ctx, symbol); return err }},
		{"stock_boards", func(symbol string) error { _, err := p.FetchStockBoards(ctx, symbol); return err }},
		{"chip_distribution", func(symbol string) error { _, err := p.FetchChipDistribution(ctx, symbol); return err }},
		{"capital_flow", func(symbol string) error { _, err := p.FetchCapitalFlow(ctx, symbol); return err }},
	}
	for _, symbol := range []string{"510300.SH", "159919.SZ", "000300.SH", "113001.SH", "900901.SH"} {
		for _, item := range calls {
			if err := item.call(symbol); err == nil {
				t.Errorf("%s endpoint must reject non-common-stock symbol %s", item.name, symbol)
			}
		}
	}
	if requests != 0 {
		t.Fatalf("non-stock security requests must be rejected before upstream access; got %d request(s)", requests)
	}
}
