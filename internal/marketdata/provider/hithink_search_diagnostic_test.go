package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestHithinkSearchStocksReportsInvalidDuplicateAndMismatchedIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/meta/tickers/search" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("X-api-key"); got != "test-key" {
			t.Errorf("unexpected API key header %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"item":[
			{"thscode":"sh.600519","ticker":"600519","name":"贵州茅台","exchange":"SH","asset_type":"a-share"},
			{"thscode":"sh.600519","ticker":"600519","name":"贵州茅台重复","exchange":"SH","asset_type":"a-share"},
			{"thscode":"sz.000001","ticker":"600519","name":"平安银行","exchange":"SZ","asset_type":"a-share"},
			{"thscode":"invalid","ticker":"000002","name":"无效证券","exchange":"SZ","asset_type":"a-share"}
		]}}`)
	}))
	defer server.Close()

	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewHithinkConfiguredWithHTTPDoer(time.Second, server.URL, "test-key", server.Client())
	snapshot, err := provider.SearchStocks(ctx, "贵州茅台", 10)
	if err != nil {
		t.Fatalf("SearchStocks() error = %v", err)
	}
	if snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("QualityStatus = %q, want degraded", snapshot.QualityStatus)
	}
	if snapshot.Total != 1 || len(snapshot.Items) != 1 || snapshot.Items[0].Symbol != "600519.SH" {
		t.Fatalf("unexpected retained stock search items: %+v", snapshot.Items)
	}

	report := reporter.Snapshot()
	if report.TotalCount != 3 || report.WarningCount != 3 {
		t.Fatalf("unexpected diagnostic counts: %+v", report)
	}
	got := map[string]bool{}
	for _, issue := range report.Issues {
		got[issue.Code] = true
		if issue.RowIndex == nil {
			t.Fatalf("issue must retain source row index: %+v", issue)
		}
		if issue.ProviderField == "" || issue.ActualPreview == "" {
			t.Fatalf("issue must retain provider field evidence: %+v", issue)
		}
	}
	for _, code := range []string{"duplicate_identity", "symbol_code_mismatch", "invalid_symbol"} {
		if !got[code] {
			t.Errorf("missing diagnostic code %q in %+v", code, report.Issues)
		}
	}
}
