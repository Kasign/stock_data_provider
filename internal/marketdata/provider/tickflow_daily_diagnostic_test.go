package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestTickFlowDailyBarsReportsRawFieldParseLocation(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, -1)
	timestamp := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"data":{"timestamp":[%d],"open":[100],"high":[102],"low":[99],"close":["bad-price"],"volume":[1000],"amount":[100000],"prev_close":[100]}}`, timestamp)))
	}))
	defer server.Close()

	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	if _, err := provider.FetchDailyBars(ctx, "600519.SH", date.Format("2006-01-02"), date.Format("2006-01-02"), 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected invalid close field")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Stage != validation.StageParse || issues[0].Field != "close" || issues[0].ProviderField != "close" || issues[0].Code != "invalid_number" || issues[0].RowIndex == nil || *issues[0].RowIndex != 0 {
		t.Fatalf("unexpected TickFlow parse issue: %+v", issues)
	}
}

func TestTickFlowDailyBarsReportsArrayLengthMismatchWithoutRowIndex(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, -1)
	timestamp := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"data":{"timestamp":[%d],"open":[],"high":[102],"low":[99],"close":[101],"volume":[1000]}}`, timestamp)))
	}))
	defer server.Close()

	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	if _, err := provider.FetchDailyBars(ctx, "600519.SH", date.Format("2006-01-02"), date.Format("2006-01-02"), 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected required array length mismatch")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Field != "open" || issues[0].Code != "array_length_mismatch" || issues[0].RowIndex != nil {
		t.Fatalf("unexpected array length diagnostic: %+v", issues)
	}
}

func TestTickFlowDailyBarsReportsDuplicateTradeDate(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil { t.Fatal(err) }
	date := time.Now().In(location).AddDate(0, 0, -1)
	timestamp := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"data":{"timestamp":[%d,%d],"open":[100,100],"high":[102,102],"low":[99,99],"close":[101,101],"volume":[1000,1000],"amount":[100000,100000]}}`, timestamp, timestamp)))
	}))
	defer server.Close()
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	if _, err := provider.FetchDailyBars(ctx, "600519.SH", date.Format("2006-01-02"), date.Format("2006-01-02"), 10, model.AdjustmentNone); err == nil { t.Fatal("expected duplicate date error") }
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Code != "duplicate_trade_date" || issues[0].RowIndex == nil || *issues[0].RowIndex != 1 {
		t.Fatalf("unexpected duplicate-date diagnostic: %+v", issues)
	}
}

func TestTickFlowDailyBarsReportsDateOutsideRequestedRange(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil { t.Fatal(err) }
	requested := time.Now().In(location).AddDate(0, 0, -1)
	outside := requested.AddDate(0, 0, -1)
	timestamp := time.Date(outside.Year(), outside.Month(), outside.Day(), 12, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"data":{"timestamp":[%d],"open":[100],"high":[102],"low":[99],"close":[101],"volume":[1000],"amount":[100000]}}`, timestamp)))
	}))
	defer server.Close()
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	day := requested.Format("2006-01-02")
	if _, err := provider.FetchDailyBars(ctx, "600519.SH", day, day, 10, model.AdjustmentNone); err == nil { t.Fatal("expected out-of-range date error") }
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Code != "date_out_of_requested_range" || issues[0].RowIndex == nil || *issues[0].RowIndex != 0 {
		t.Fatalf("unexpected out-of-range diagnostic: %+v", issues)
	}
}
