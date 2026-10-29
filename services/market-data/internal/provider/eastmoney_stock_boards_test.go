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

func TestEastmoneyStockBoardsMarksSourceCapAsLowerBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("unexpected HTTP method %s", r.Method)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.Query().Get("pz") != "1000" {
			t.Errorf("expected source page size 1000, query=%s", r.URL.RawQuery)
		}
		rows := make([]map[string]any, 1000)
		for i := range rows {
			rows[i] = map[string]any{
				"f12": fmt.Sprintf("BK%04d", i+1),
				"f14": fmt.Sprintf("测试板块%d", i+1),
				"f3":  1.5,
			}
		}
		payload := map[string]any{"rc": 0, "data": map[string]any{"total": 1000, "diff": rows}}
		if err := json.NewEncoder(w).Encode(payload); err != nil {
			t.Errorf("encode fixture response: %v", err)
		}
	}))
	defer server.Close()

	client := NewEastmoneyConfiguredWithStockBoards(
		2*time.Second, server.URL, server.URL, server.URL, server.URL,
		server.URL, server.URL, server.URL, server.URL, "",
	)
	got, err := client.FetchStockBoards(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchStockBoards: %v", err)
	}
	if got.ProviderReturned != 1000 || got.ExcludedRows != 0 || got.Total != 1000 || len(got.Items) != 1000 {
		t.Fatalf("unexpected stock-board coverage: returned=%d excluded=%d total=%d items=%d",
			got.ProviderReturned, got.ExcludedRows, got.Total, len(got.Items))
	}
	if !got.TotalIsLowerBound {
		t.Fatal("a response at the 1000-row source boundary must be marked as a lower bound")
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("source cap must remain degraded, got %s", got.QualityStatus)
	}
	for _, reason := range got.QualityReasons {
		if reason == "provider_stock_boards_response_may_be_capped_at_1000_rows" {
			return
		}
	}
	t.Fatalf("source-cap quality reason missing: %v", got.QualityReasons)
}
