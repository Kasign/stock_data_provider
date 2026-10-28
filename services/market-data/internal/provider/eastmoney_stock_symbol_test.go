package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
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

func TestEastmoneyLimitPoolRejectsUnsupportedSecuritySymbols(t *testing.T) {
	tests := []struct {
		name         string
		code         string
		securityName string
		market       string
		wantError    bool
		wantSymbol   string
	}{
		{name: "ETF", code: "510300", securityName: "沪深300ETF", market: "1", wantError: true},
		{name: "index", code: "000300", securityName: "沪深300", market: "1", wantError: true},
		{name: "B-share", code: "900901", securityName: "B股", market: "1", wantError: true},
		{name: "exchange-mismatch", code: "600519", securityName: "贵州茅台", market: "0", wantError: true},
		{name: "ordinary-SH-stock", code: "600519", securityName: "贵州茅台", market: "1", wantSymbol: "600519.SH"},
		{name: "ordinary-BJ-stock", code: "830799", securityName: "北交所股票", market: "0", wantSymbol: "830799.BJ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				fmt.Fprintf(w, `{"rc":0,"data":{"tc":1,"qdate":"20261026","pool":[{"c":"%s","n":"%s","m":"%s","zdp":10}]}}`,
					tt.code, tt.securityName, tt.market)
			}))
			defer server.Close()

			p := NewEastmoney(2*time.Second, server.URL, server.URL)
			p.limitPoolBaseURL = server.URL
			got, err := p.FetchLimitPool(context.Background(), "up", "2026-10-26", 10)
			if tt.wantError {
				if err == nil {
					t.Fatalf("unsupported security %s must be rejected, got snapshot %+v", tt.code, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ordinary A-share should be accepted: %v", err)
			}
			if got == nil || len(got.Items) != 1 || got.Items[0].Symbol != tt.wantSymbol {
				t.Fatalf("unexpected limit-pool items: %+v, want symbol %s", got, tt.wantSymbol)
			}
		})
	}
}


func TestEastmoneyStockSearchDropsNonAshareAndMarketMismatchRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":{"total":7,"diff":[
			{"f12":"600519","f13":1,"f14":"贵州茅台"},
			{"f12":"000858","f13":0,"f14":"五粮液"},
			{"f12":"830799","f13":0,"f14":"北交所样例"},
			{"f12":"510300","f13":1,"f14":"沪深300ETF"},
			{"f12":"000300","f13":1,"f14":"沪深300指数"},
			{"f12":"900901","f13":1,"f14":"B股"},
			{"f12":"600519","f13":0,"f14":"市场字段不一致"}
		]}}`)
	}))
	defer server.Close()

	p := &Eastmoney{client: server.Client(), stockListURL: server.URL, ut: "test-token"}
	items, quality, reasons, err := p.fetchStockSearchIndex(context.Background())
	if err != nil {
		t.Fatalf("stock search directory should tolerate and mark rejected non-A-share rows: %v", err)
	}
	got := make(map[string]bool, len(items))
	for _, item := range items {
		got[item.Symbol] = true
	}
	for _, symbol := range []string{"600519.SH", "000858.SZ", "830799.BJ"} {
		if !got[symbol] {
			t.Errorf("expected supported A-share %s in directory, got %v", symbol, got)
		}
	}
	if len(items) != 3 {
		t.Fatalf("non-A-share and mismatched-market rows must be dropped; got %d items: %+v", len(items), items)
	}
	if quality != model.QualityDegraded {
		t.Fatalf("dropping unsupported securities must mark directory degraded, got %s", quality)
	}
	if !strings.Contains(strings.Join(reasons, ","), "provider_non_a_share_security_rows_dropped") {
		t.Fatalf("quality reasons should disclose filtered non-A-share rows, got %v", reasons)
	}
}


func TestParseEastmoneyLimitTimeValidatesClockRange(t *testing.T) {
	for _, tt := range []struct {
		raw  string
		want string
	}{
		{raw: "93000", want: "09:30:00"},
		{raw: "093001", want: "09:30:01"},
		{raw: "240000", want: ""},
		{raw: "126000", want: ""},
		{raw: "125960", want: ""},
		{raw: "999999", want: ""},
		{raw: "abc123", want: ""},
	} {
		if got := parseEastmoneyLimitTime(tt.raw); got != tt.want {
			t.Errorf("parseEastmoneyLimitTime(%q) = %q, want %q", tt.raw, got, tt.want)
		}
	}
}

func TestEastmoneyLimitPoolMarksInvalidTouchTimesAsDegraded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"rc":0,"data":{"tc":1,"qdate":"20261026","pool":[{"c":"600519","n":"贵州茅台","m":"1","zdp":10,"fbt":"246000","lbt":"093000"}]}}`)
	}))
	defer server.Close()

	p := NewEastmoney(2*time.Second, server.URL, server.URL)
	p.limitPoolBaseURL = server.URL
	got, err := p.FetchLimitPool(context.Background(), "up", "2026-10-26", 10)
	if err != nil {
		t.Fatalf("FetchLimitPool returned error: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].FirstTouchTime != "" || got.Items[0].LastTouchTime != "09:30:00" {
		t.Fatalf("invalid touch time should be omitted without damaging the valid one: %+v", got.Items)
	}
	if !strings.Contains(strings.Join(got.Items[0].QualityReasons, ","), "provider_limit_pool_touch_time_invalid") {
		t.Fatalf("item quality should disclose invalid touch time: %+v", got.Items[0].QualityReasons)
	}
	if !strings.Contains(strings.Join(got.QualityReasons, ","), "provider_limit_pool_touch_time_invalid") {
		t.Fatalf("snapshot quality should disclose invalid touch time: %+v", got.QualityReasons)
	}
}
