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

func TestHithinkLimitPoolReportsDuplicateSymbol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-share/special-data/limit-up-pool" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		now := time.Now().UTC().UnixMilli()
		row := map[string]any{
			"thscode": "600519.SH", "ticker": "600519", "name": "贵州茅台",
			"price_change_ratio_pct": 10.0, "limit_up_time": "09:30:00",
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"timestamp": now,
				"pagination": map[string]any{"total": 2, "page": 1, "size": 2},
				"item": []map[string]any{row, row},
			},
		})
	}))
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
	got, err := p.FetchLimitPool(ctx, "up", time.Now().In(time.FixedZone("CST", 8*3600)).Format("2006-01-02"), 10)
	if err != nil {
		t.Fatalf("FetchLimitPool: %v", err)
	}
	if len(got.Items) != 1 || got.ExcludedRows != 1 {
		t.Fatalf("unexpected duplicate exclusion: items=%d excluded=%d", len(got.Items), got.ExcludedRows)
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Code != "duplicate_identity" || issues[0].RowIndex == nil || *issues[0].RowIndex != 1 {
		t.Fatalf("unexpected Reporter issues: %+v", issues)
	}
}
