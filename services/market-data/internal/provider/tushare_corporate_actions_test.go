package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTushareFetchCorporateActionsKeepsAnnouncementAndEffectiveDatesSeparate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"fields":["ts_code","end_date","ann_date","div_proc","stk_bo_rate","stk_co_rate","cash_div","cash_div_tax","record_date","ex_date","pay_date","imp_ann_date"],"items":[["600519.SH","20251231","20260320","实施","0.05","0.05","0.50","0.60","20260615","20260616","20260616","20260610"],["600519.SH","20251231","20260401","预案",null,null,null,null,null,null,null,null]]}}`))
	}))
	defer server.Close()
	p := NewTushareConfigured(time.Second, server.URL, "test-token")
	items, err := p.FetchCorporateActions(context.Background(), "600519.SH", "2026-03-01", "2026-06-30")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("got %d actions, want 2", len(items))
	}
	got := items[0]
	if got.AnnouncementDate != "2026-03-20" || got.ImplementationAnnouncementDate != "2026-06-10" || got.ExDate != "2026-06-16" {
		t.Fatalf("announcement, implementation and ex-date must remain distinct: %+v", got)
	}
	if got.CashDividendCNY == nil || *got.CashDividendCNY != 0.5 || got.CashDividendBeforeTaxCNY == nil || *got.CashDividendBeforeTaxCNY != 0.6 {
		t.Fatalf("cash dividend tax bases were not mapped correctly: %+v", got)
	}
	if got.AvailableAt != nil || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("daily announcement date must not be promoted to exact available_at: %+v", got)
	}
	proposal := items[1]
	if proposal.ExDate != "" || proposal.EventStatus != "预案" || proposal.AnnouncementDate != "2026-04-01" {
		t.Fatalf("unimplemented proposal must remain a dated announcement without invented ex-date: %+v", proposal)
	}
}

func TestTushareFetchCorporateActionsRequiresToken(t *testing.T) {
	p := NewTushareConfigured(time.Second, "", "")
	if _, err := p.FetchCorporateActions(context.Background(), "600519.SH", "2026-01-01", "2026-10-30"); err == nil {
		t.Fatal("expected missing token error")
	}
}
