package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/validation"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestTushareFetchAdjustmentFactorsRequiresToken(t *testing.T) {
	p := NewTushareConfigured(time.Second, "", "")
	if _, err := p.FetchAdjustmentFactors(context.Background(), "600519.SH", "2026-10-01", "2026-10-30"); err == nil {
		t.Fatal("expected missing token error")
	}
}

func TestTushareAdjustmentFactorModelKeepsAvailabilityUnknown(t *testing.T) {
	value := model.AdjustmentFactor{Source: "tushare", SourceVersion: tushareVersion, FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded}
	if value.AvailableAt != nil {
		t.Fatal("Tushare adj_factor does not expose first-publication timestamp; available_at must remain unknown")
	}
}

func TestTushareAdjustmentFactorsReportInvalidRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"fields":["ts_code","trade_date","adj_factor"],"items":[["600519.SH","20250711",0]]}}`)
	}))
	defer server.Close()
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewTushareConfigured(time.Second, server.URL, "test-token")
	_, err := p.fetchAdjustmentFactors(ctx, "600519.SH", "2025-01-01", "2025-12-31")
	if err == nil {
		t.Fatal("zero adjustment factor must be rejected")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 {
		t.Fatalf("expected one field issue, got %+v", issues)
	}
	issue := issues[0]
	if issue.Stage != validation.StageParse || issue.Field != "adjustment_factor" ||
		issue.ProviderField != "adj_factor" || issue.Code != "invalid_factor" ||
		issue.RowIndex == nil || *issue.RowIndex != 0 || issue.Symbol != "" {
		t.Fatalf("unexpected Tushare factor issue: %+v", issue)
	}
}
