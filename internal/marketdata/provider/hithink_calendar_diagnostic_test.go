package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestHithinkTradingCalendarReportsMissingProviderTimestamp(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"item": []map[string]any{{"date": today}}},
		})
	}))
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
	got, err := p.FetchTradingCalendar(ctx, today, today)
	if err != nil {
		t.Fatalf("FetchTradingCalendar: %v", err)
	}
	if got.QualityStatus != "degraded" {
		t.Fatalf("missing provider timestamp must keep calendar degraded, got %q", got.QualityStatus)
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Field != "provider_time" || issues[0].ProviderField != "timestamp" ||
		issues[0].Code != "missing_or_invalid_value" || issues[0].RowIndex != nil {
		t.Fatalf("unexpected calendar timestamp issue: %+v", issues)
	}
}
