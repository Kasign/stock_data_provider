package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHithinkHotRankingReportsDuplicateExclusionAndLowerBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-share/special-data/hot-stock-list" {
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-api-key") != "unit-test-key" {
			t.Error("missing Hithink API key")
		}
		now := time.Now().UTC().UnixMilli()
		payload := map[string]any{
			"code": 0,
			"data": map[string]any{
				"timestamp": now,
				"item": []map[string]any{
					{"thscode": "600519.SH", "ticker": "600519", "name": "贵州茅台", "rank": 1},
					{"thscode": "600519.SH", "ticker": "600519", "name": "重复贵州茅台", "rank": 2},
				},
			},
		}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Errorf("encode fixture: %v", err)
		}
	}))
	defer server.Close()

	provider := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
	got, err := provider.FetchHotStocks(context.Background(), "popular", 10)
	if err != nil {
		t.Fatalf("FetchHotStocks: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.ProviderReturned != 2 ||
		got.ExcludedRows != 1 || !got.TotalIsLowerBound {
		t.Fatalf("unexpected hot-ranking coverage ledger: total=%d items=%d returned=%d excluded=%d lower_bound=%v",
			got.Total, len(got.Items), got.ProviderReturned, got.ExcludedRows, got.TotalIsLowerBound)
	}
}
