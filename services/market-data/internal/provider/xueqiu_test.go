package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func newXueqiuFixtureServer(t *testing.T, expectedSize string, rows []map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("size"); got != expectedSize {
			t.Errorf("request size=%q, want %q", got, expectedSize)
		}
		if r.URL.Path != "/v5/stock/hot_stock/list.json" {
			t.Errorf("request path=%q, want hot-stock list", r.URL.Path)
		}
		if got := r.URL.Query().Get("type"); got != "10" {
			t.Errorf("request type=%q, want 10", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": 0,
			"data": map[string]any{
				"timestamp": time.Now().Unix(),
				"items":     rows,
			},
		})
	}))
}

func xueqiuFixtureRows(count int) []map[string]any {
	rows := make([]map[string]any, 0, count)
	for i := 0; i < count; i++ {
		rows = append(rows, map[string]any{
			"symbol":      "SH" + fmt.Sprintf("%06d", 600100+i),
			"name":        "样本股票",
			"current":     10.5,
			"change":      0.2,
			"percent":     1.2,
			"value":       100,
			"rank_change": 0,
		})
	}
	return rows
}

func hasXueqiuReason(reasons []string, expected string) bool {
	for _, reason := range reasons {
		if reason == expected {
			return true
		}
	}
	return false
}

func TestXueqiuHotRankingMarksRequestLimitCap(t *testing.T) {
	server := newXueqiuFixtureServer(t, "50", xueqiuFixtureRows(50))
	defer server.Close()

	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	snapshot, err := provider.FetchHotStocks(context.Background(), "popular", 100)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 50 || len(snapshot.Items) != 50 {
		t.Fatalf("snapshot should expose the 50-row provider cap, total=%d items=%d", snapshot.Total, len(snapshot.Items))
	}
	if !hasXueqiuReason(snapshot.QualityReasons, "provider_request_limit_capped_at_50") {
		t.Fatalf("missing explicit provider cap reason: %v", snapshot.QualityReasons)
	}
}

func TestXueqiuHotRankingMarksInvalidRankChange(t *testing.T) {
	rows := []map[string]any{{
		"symbol":      "SH600519",
		"name":        "贵州茅台",
		"current":     1500.0,
		"change":      1.0,
		"percent":     0.1,
		"rank_change": "not-an-integer",
	}}
	server := newXueqiuFixtureServer(t, "10", rows)
	defer server.Close()

	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	snapshot, err := provider.FetchHotStocks(context.Background(), "popular", 10)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 1 || len(snapshot.Items) != 1 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.Items[0].RankChange != nil {
		t.Fatalf("invalid rank_change must not be exposed: %+v", snapshot.Items[0].RankChange)
	}
	const reason = "provider_rank_change_invalid_or_out_of_range"
	if !hasXueqiuReason(snapshot.QualityReasons, reason) {
		t.Fatalf("snapshot must disclose invalid rank_change: %v", snapshot.QualityReasons)
	}
	if !hasXueqiuReason(snapshot.Items[0].QualityReasons, reason) {
		t.Fatalf("item must disclose invalid rank_change: %v", snapshot.Items[0].QualityReasons)
	}
}



func TestXueqiuHotRankingRejectsMalformedErrorCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v5/stock/hot_stock/list.json" {
			t.Errorf("request path=%q, want hot-stock list", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": "not-a-number",
			"data": map[string]any{
				"timestamp": time.Now().Unix(),
				"items": []map[string]any{{
					"symbol": "SH600519", "name": "贵州茅台", "current": 1500.0,
				}},
			},
		})
	}))
	defer server.Close()

	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	if _, err := provider.FetchHotStocks(context.Background(), "popular", 10); err == nil || !strings.Contains(err.Error(), "error_code") {
		t.Fatalf("malformed source error_code must reject the response, got %v", err)
	}
}

func TestXueqiuFixtureSymbolCodesAreUnique(t *testing.T) {
	rows := xueqiuFixtureRows(50)
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		symbol, ok := row["symbol"].(string)
		if !ok {
			t.Fatalf("fixture symbol has unexpected type: %T", row["symbol"])
		}
		if _, exists := seen[symbol]; exists {
			t.Fatalf("fixture generated duplicate symbol %q", symbol)
		}
		seen[symbol] = struct{}{}
		if _, err := strconv.Atoi(strings.TrimPrefix(symbol, "SH")); err != nil {
			t.Fatalf("fixture generated invalid symbol %q: %v", symbol, err)
		}
	}
}
