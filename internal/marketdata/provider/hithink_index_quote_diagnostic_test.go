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

func TestHithinkFetchIndexQuotesReportsRowLevelIssues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		items := []map[string]any{
			{
				"thscode": "000001.SH", "name": "",
				"last_price": 101.0, "price_change": 9.0, "price_change_ratio_pct": 1.0,
				"open_price": 99.0, "high_price": 102.0, "low_price": 98.0,
				"prev_price": 100.0, "turnover": -1.0,
			},
			{
				"thscode": "000001.SH", "name": "重复上证指数",
				"last_price": 101.0, "price_change": 1.0, "price_change_ratio_pct": 1.0,
				"open_price": 99.0, "high_price": 102.0, "low_price": 98.0,
				"prev_price": 100.0, "turnover": 100.0,
			},
		}
		for _, definition := range hithinkCoreIndexDefinitions[1:] {
			items = append(items, map[string]any{
				"thscode": definition.Symbol, "name": definition.Name,
				"last_price": 101.0, "price_change": 1.0, "price_change_ratio_pct": 1.0,
				"open_price": 99.0, "high_price": 102.0, "low_price": 98.0,
				"prev_price": 100.0, "turnover": 100.0,
			})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{
				"timestamp": time.Now().UTC().Add(-time.Minute).UnixMilli(),
				"item": items,
			},
		})
	}))
	defer server.Close()

	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
	got, err := provider.FetchIndexQuotes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got.QualityStatus != "degraded" {
		t.Fatalf("snapshot should be degraded, got %q", got.QualityStatus)
	}
	issues := reporter.Snapshot().Issues
	want := map[string]int{
		"provider_value_inconsistent": 0,
		"missing_value": 0,
		"missing_or_invalid_value": 0,
		"duplicate_identity": 0,
	}
	rows := map[string]int{}
	for _, issue := range issues {
		if _, ok := want[issue.Code]; ok {
			want[issue.Code]++
			if issue.RowIndex != nil {
				rows[issue.Code] = *issue.RowIndex
			}
		}
	}
	for code, count := range want {
		if count != 1 {
			t.Errorf("issue %q count = %d, want 1; all issues: %+v", code, count, issues)
		}
	}
	if rows["provider_value_inconsistent"] != 0 || rows["missing_value"] != 0 ||
		rows["missing_or_invalid_value"] != 0 || rows["duplicate_identity"] != 1 {
		t.Fatalf("unexpected row evidence: %+v", rows)
	}
}

func TestHithinkFetchIndexQuotesReportsTimestampAndCoverageGaps(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		definition := hithinkCoreIndexDefinitions[0]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"data": map[string]any{"item": []map[string]any{{
				"thscode": definition.Symbol, "name": definition.Name,
				"last_price": 101.0, "price_change": 1.0, "price_change_ratio_pct": 1.0,
				"open_price": 99.0, "high_price": 102.0, "low_price": 98.0,
				"prev_price": 100.0, "turnover": 100.0,
			}}},
		})
	}))
	defer server.Close()
	reporter := validation.NewReporter(12)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewHithinkConfigured(2*time.Second, server.URL, "unit-test-key")
	got, err := p.FetchIndexQuotes(ctx)
	if err != nil { t.Fatal(err) }
	if got.QualityStatus != "degraded" { t.Fatalf("snapshot should be degraded, got %q", got.QualityStatus) }
	foundTime, foundCoverage := false, false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Stage != validation.StageValidate || issue.Severity != validation.SeverityWarning || issue.RowIndex != nil { continue }
		switch issue.Field {
		case "provider_time": foundTime = issue.Code == "missing_or_invalid_value"
		case "index_coverage": foundCoverage = issue.Code == "allowlisted_index_unavailable" && issue.ActualPreview != ""
		}
	}
	if !foundTime || !foundCoverage { t.Fatalf("missing snapshot-level diagnostics: %+v", reporter.Snapshot().Issues) }
}
