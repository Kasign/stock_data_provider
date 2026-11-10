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
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/marketdata/validation"
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

func TestXueqiuHotRankingReportsInvalidNumericField(t *testing.T) {
	rows := []map[string]any{{
		"symbol":  "SH600519",
		"name":    "贵州茅台",
		"current": "not-a-number",
	}}
	server := newXueqiuFixtureServer(t, "10", rows)
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	if _, err := provider.FetchHotStocks(ctx, "popular", 10); err == nil {
		t.Fatal("invalid numeric field must reject the ranking response")
	}
	report := reporter.Snapshot()
	if report.WarningCount != 1 || report.ErrorCount != 0 || len(report.Issues) != 1 {
		t.Fatalf("expected one parse warning and no errors, got %+v", report)
	}
	issue := report.Issues[0]
	if issue.Field != "latest_price" || issue.ProviderField != "current" || issue.Code != "invalid_number" || issue.RowIndex == nil || *issue.RowIndex != 0 {
		t.Fatalf("unexpected row-level numeric diagnostic: %+v", issue)
	}
}

func TestXueqiuHotRankingReportsInvalidRankChangeWarning(t *testing.T) {
	rows := []map[string]any{{
		"symbol":      "SH600519",
		"name":        "贵州茅台",
		"current":     1500.0,
		"rank_change": "not-an-integer",
	}}
	server := newXueqiuFixtureServer(t, "10", rows)
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	snapshot, err := provider.FetchHotStocks(ctx, "popular", 10)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Items[0].RankChange != nil {
		t.Fatalf("invalid rank_change must remain omitted: %+v", snapshot.Items[0])
	}
	report := reporter.Snapshot()
	if report.WarningCount != 1 || report.ErrorCount != 0 {
		t.Fatalf("expected one warning and no errors, got %+v", report)
	}
	issue := report.Issues[0]
	if issue.Field != "rank_change" || issue.ProviderField != "rank_change" || issue.Code != "invalid_optional_integer" || issue.RowIndex == nil || *issue.RowIndex != 0 {
		t.Fatalf("unexpected optional integer diagnostic: %+v", issue)
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

func TestXueqiuHotRankingReportsSnapshotAndLimitDegradation(t *testing.T) {
	rows := xueqiuFixtureRows(3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("size"); got != "50" {
			t.Errorf("request size=%q, want 50", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error_code": 0,
			"data":       map[string]any{"items": rows},
		})
	}))
	defer server.Close()
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	snapshot, err := provider.FetchHotStocks(ctx, "popular", 100)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Total != 3 {
		t.Fatalf("unexpected ranking total: %d", snapshot.Total)
	}
	found := map[string]bool{"provider_time": false, "request_limit": false}
	for _, issue := range reporter.Snapshot().Issues {
		if _, ok := found[issue.Field]; ok && issue.Severity == validation.SeverityWarning {
			found[issue.Field] = true
		}
	}
	for field, ok := range found {
		if !ok {
			t.Errorf("missing snapshot-level Reporter evidence for %s: %+v", field, reporter.Snapshot().Issues)
		}
	}
}

func TestXueqiuHotRankingReportsProviderExcessRows(t *testing.T) {
	rows := xueqiuFixtureRows(3)
	server := newXueqiuFixtureServer(t, "2", rows)
	defer server.Close()
	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	if _, err := provider.FetchHotStocks(ctx, "popular", 2); err != nil {
		t.Fatal(err)
	}
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "row_count" && issue.Code == "provider_returned_excess_rows" && issue.Severity == validation.SeverityWarning {
			return
		}
	}
	t.Fatalf("expected excess row diagnostic, got %+v", reporter.Snapshot().Issues)
}

func TestXueqiuErrorMessageTruncationPreservesUTF8(t *testing.T) {
	message := strings.Repeat("a", 199) + "中" + strings.Repeat("b", 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"error_code": 1001, "error_description": message})
	}))
	defer server.Close()
	provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
	_, err := provider.FetchHotStocks(context.Background(), "popular", 10)
	if err == nil {
		t.Fatal("provider error response should be rejected")
	}
	got := err.Error()
	if !utf8.ValidString(got) {
		t.Fatalf("error message is invalid UTF-8: %q", got)
	}
	want := "雪球榜单接口错误 code=1001: " + strings.Repeat("a", 199)
	if got != want {
		t.Fatalf("error summary should stop before a split multibyte rune: got %q want %q", got, want)
	}
}

func TestXueqiuHotRankingReportsResponseRejections(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantCode string
		status   int
	}{
		{name: "HTTP status", body: "upstream unavailable", status: http.StatusBadGateway, wantCode: "http_status_error"},
		{name: "invalid JSON", body: "{", status: http.StatusOK, wantCode: "decode_error"},
		{name: "provider error", body: `{"error_code":1001,"error_description":"quota exceeded"}`, status: http.StatusOK, wantCode: "provider_error"},
		{name: "invalid error code", body: `{"error_code":"bad","data":{"items":[]}}`, status: http.StatusOK, wantCode: "invalid_provider_error_code"},
		{name: "missing data", body: `{"error_code":0}`, status: http.StatusOK, wantCode: "missing_data"},
		{name: "invalid data shape", body: `{"error_code":0,"data":[]}`, status: http.StatusOK, wantCode: "decode_error"},
		{name: "empty rows", body: `{"error_code":0,"data":{"items":[]}}`, status: http.StatusOK, wantCode: "empty_result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()

			reporter := validation.NewReporter(10)
			ctx := validation.WithReporter(context.Background(), reporter)
			provider := NewXueqiuConfigured(2*time.Second, server.URL, "")
			snapshot, err := provider.FetchHotStocks(ctx, "popular", 10)
			if tc.wantCode == "empty_result" {
				if err != nil {
					t.Fatalf("empty ranking preserves successful empty-snapshot semantics: %v", err)
				}
				if snapshot == nil || snapshot.Total != 0 {
					t.Fatalf("expected empty snapshot, got %+v", snapshot)
				}
			} else if err == nil {
				t.Fatal("expected rejected response")
			}

			for _, issue := range reporter.Snapshot().Issues {
				if issue.Code == tc.wantCode {
					if issue.RowIndex != nil {
						t.Fatalf("response-level issue must not invent row index: %+v", issue)
					}
					return
				}
			}
			t.Fatalf("missing Reporter code %q: %+v", tc.wantCode, reporter.Snapshot().Issues)
		})
	}
}
