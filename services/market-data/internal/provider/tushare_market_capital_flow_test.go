package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTushareMarketCapitalFlowParsesAndSortsDailyRows(t *testing.T) {
	fields := []string{"trade_date", "close_sh", "pct_change_sh", "close_sz", "pct_change_sz", "net_amount", "net_amount_rate", "buy_elg_amount", "buy_elg_amount_rate", "buy_lg_amount", "buy_lg_amount_rate", "buy_md_amount", "buy_md_amount_rate", "buy_sm_amount", "buy_sm_amount_rate"}
	row := func(date string, net string) []any {
		return []any{date, 3260.5, 0.8, 10500.2, 1.1, net, 2.5, 1000, 0.3, 2000, 0.4, -500, -0.1, -2500, -0.6}
	}
	payload, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"fields": fields, "items": [][]any{row("20261029", "123456789"), row("20261028", "-23456789")}}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		_ = json.NewDecoder(r.Body).Decode(&request)
		if request["api_name"] != "moneyflow_mkt_dc" {
			t.Errorf("api_name = %v", request["api_name"])
		}
		fmt.Fprint(w, string(payload))
	}))
	defer server.Close()

	p := NewTushareConfigured(3*time.Second, server.URL, "test-token")
	got, err := p.FetchMarketCapitalFlow(context.Background(), "2026-10-28", "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if got.Source != "tushare" || got.Total != 2 || len(got.Items) != 2 {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	if got.Items[0].TradeDate != "2026-10-28" || got.Items[1].TradeDate != "2026-10-29" {
		t.Fatalf("rows are not sorted ascending: %+v", got.Items)
	}
	if got.Items[1].NetAmountCNY == nil || *got.Items[1].NetAmountCNY != 123456789 {
		t.Fatalf("net amount was not parsed in CNY: %+v", got.Items[1])
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing precise provider timestamp must remain degraded: %s", got.QualityStatus)
	}
}

func TestTushareMarketCapitalFlowPreservesNullAsMissing(t *testing.T) {
	fields := []string{"trade_date", "close_sh", "pct_change_sh", "close_sz", "pct_change_sz", "net_amount", "net_amount_rate", "buy_elg_amount", "buy_elg_amount_rate", "buy_lg_amount", "buy_lg_amount_rate", "buy_md_amount", "buy_md_amount_rate", "buy_sm_amount", "buy_sm_amount_rate"}
	items := [][]any{{"20261029", 3260.5, 0.8, 10500.2, 1.1, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil}}
	payload, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"fields": fields, "items": items}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	got, err := NewTushareConfigured(3*time.Second, server.URL, "test-token").FetchMarketCapitalFlow(context.Background(), "2026-10-29", "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if got.Items[0].NetAmountCNY != nil || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("null market flow must remain null/degraded: %+v", got)
	}
}

func TestTushareMarketCapitalFlowRejectsDuplicateDates(t *testing.T) {
	fields := []string{"trade_date", "net_amount"}
	payload, _ := json.Marshal(map[string]any{"code": 0, "data": map[string]any{"fields": fields, "items": [][]any{{"20261029", 1.0}, {"20261029", 2.0}}}})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(payload) }))
	defer server.Close()
	_, err := NewTushareConfigured(3*time.Second, server.URL, "test-token").FetchMarketCapitalFlow(context.Background(), "2026-10-29", "2026-10-29")
	if err == nil {
		t.Fatal("duplicate trade dates must be rejected")
	}
}
