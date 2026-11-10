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

func TestHithinkFetchIndexConstituentsReportsMissingSnapshotTimestamp(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-share-index/constituents/ths-stock-list" {
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"item": []map[string]any{{"thscode": "600519.SH", "ticker": "600519", "name": "贵州茅台"}},
			},
		})
	}))
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
	got, err := p.FetchIndexConstituents(ctx, "000300.SH")
	if err != nil {
		t.Fatalf("FetchIndexConstituents: %v", err)
	}
	if got.QualityStatus != "degraded" || len(got.Items) != 1 {
		t.Fatalf("constituent data should remain available but degraded: %+v", got)
	}
	found := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Stage == validation.StageValidate && issue.Severity == validation.SeverityWarning &&
			issue.Field == "provider_time" && issue.ProviderField == "timestamp" &&
			issue.Code == "missing_or_invalid_value" && issue.RowIndex == nil {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing snapshot-level provider_time issue: %+v", reporter.Snapshot().Issues)
	}
}
