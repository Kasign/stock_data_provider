package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/validation"
)

func TestHithinkDailyBarParseIssuesIncludeRowAndProviderField(t *testing.T) {
	payload := `{"code":0,"data":{"item":[{"date_ms":1792454400000,"open_price":"bad","high_price":11,"low_price":9,"close_price":10,"volume":1000,"turnover":10000}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-api-key") != "test-key" {
			t.Errorf("missing API key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	if _, err := provider.FetchDailyBars(ctx, "600036.SH", "2026-10-20", "2026-10-21", 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected invalid open price to reject the response")
	}
	found := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "open" && issue.ProviderField == "open_price" && issue.Code == "type_mismatch" && issue.RowIndex != nil && *issue.RowIndex == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected structured open_price issue at row 0, got %+v", reporter.Snapshot().Issues)
	}
}
