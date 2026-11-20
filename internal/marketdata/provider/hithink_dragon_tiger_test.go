package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestHithinkDragonTigerPaginatesUniqueSecuritiesNotEventRows(t *testing.T) {
	today := providerNowUTC().In(providerMarketLocation()).Format("2006-01-02")
	timestamp := providerNowUTC().UnixMilli()
	payload := fmt.Sprintf(`{"code":0,"data":{"timestamp":%d,"trade_date":%q,"stock_count":2,"stock_items":[
		{"thscode":"600036.SH","ticker":"600036","name":"招商银行","change":1.2,"limit_reason":"涨幅偏离"},
		{"thscode":"000001.SZ","ticker":"000001","name":"平安银行","change":-0.5,"limit_reason":"换手率"},
		{"thscode":"600036.SH","ticker":"600036","name":"招商银行","change":1.2,"limit_reason":"连续三个交易日"}
	]}}`, timestamp, today)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-share/special-data/dragon-tiger-list" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, payload)
	}))
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")

	first, err := provider.FetchDragonTiger(ctx, today, 1, 1)
	if err != nil {
		t.Fatalf("FetchDragonTiger(page 1): %v", err)
	}
	if first.Total != 2 || first.SourceReturned != 3 || first.Returned != 1 ||
		first.Pages != 2 || !first.HasMore || first.Items[0].Symbol != "600036.SH" ||
		!containsHithinkDragonTigerReason(first.QualityReasons, "duplicate_dragon_tiger_symbol_dropped") {
		t.Fatalf("first page did not use unique securities for pagination: %+v", first)
	}

	second, err := provider.FetchDragonTiger(ctx, today, 2, 1)
	if err != nil {
		t.Fatalf("FetchDragonTiger(page 2): %v", err)
	}
	if second.Total != 2 || second.SourceReturned != 3 || second.Returned != 1 ||
		second.Pages != 2 || second.HasMore || second.Items[0].Symbol != "000001.SZ" {
		t.Fatalf("second page did not preserve unique security order: %+v", second)
	}

	foundDuplicateIssue := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Code == "duplicate_identity" && issue.ProviderField == "thscode" &&
			issue.RowIndex != nil && *issue.RowIndex == 2 {
			foundDuplicateIssue = true
		}
	}
	if !foundDuplicateIssue {
		t.Fatalf("duplicate event row must be retained as diagnostic evidence: %+v", reporter.Snapshot())
	}
}

func containsHithinkDragonTigerReason(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestHithinkDragonTigerRejectsCountMismatchAfterDeduplication(t *testing.T) {
	today := providerNowUTC().In(providerMarketLocation()).Format("2006-01-02")
	timestamp := providerNowUTC().UnixMilli()
	payload := fmt.Sprintf(`{"code":0,"data":{"timestamp":%d,"trade_date":%q,"stock_count":3,"stock_items":[
		{"thscode":"600036.SH","ticker":"600036","name":"招商银行","change":1.2},
		{"thscode":"000001.SZ","ticker":"000001","name":"平安银行","change":-0.5},
		{"thscode":"600036.SH","ticker":"600036","name":"招商银行","change":1.2}
	]}}`, timestamp, today)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, payload)
	}))
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")
	if _, err := provider.FetchDragonTiger(ctx, today, 1, 10); err == nil ||
		!strings.Contains(err.Error(), "declared=3 unique=2 source_rows=3") {
		t.Fatalf("declared unique count mismatch error = %v", err)
	}
	foundCountIssue := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Code == "count_inconsistent" && issue.ProviderField == "stock_count/stock_items" {
			foundCountIssue = true
		}
	}
	if !foundCountIssue {
		t.Fatalf("count mismatch must retain structured provider evidence: %+v", reporter.Snapshot())
	}
}
