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

func TestTickFlowDailyBarsReportsEnvelopeAndPayloadFailures(t *testing.T) {
	cases := []struct {
		name, body, wantCode string
	}{
		{name: "provider error", body: `{"code":429,"message":"rate limited"}`, wantCode: "provider_error"},
		{name: "missing data", body: `{"code":0}`, wantCode: "missing_data"},
		{name: "invalid envelope", body: `{not-json`, wantCode: "decode_error"},
		{name: "invalid payload", body: `{"code":0,"data":"not-an-object"}`, wantCode: "decode_error"},
		{name: "empty timestamps", body: `{"code":0,"data":{"timestamp":[]}}`, wantCode: "empty_result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			reporter := validation.NewReporter(validation.DefaultMaxIssues)
			ctx := validation.WithReporter(context.Background(), reporter)
			provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
			_, err := provider.FetchDailyBars(ctx, "600519.SH", "2026-10-20", "2026-10-21", 10, model.AdjustmentNone)
			if err == nil {
				t.Fatal("expected malformed or rejected response to fail")
			}
			found := false
			for _, issue := range reporter.Snapshot().Issues {
				if issue.Stage == validation.StageParse && issue.Code == tc.wantCode && issue.RowIndex == nil {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing snapshot-level %s issue: %+v", tc.wantCode, reporter.Snapshot().Issues)
			}
		})
	}
}

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
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, -1)
	timestamp := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"data":{"timestamp":[%d,%d],"open":[100,100],"high":[102,102],"low":[99,99],"close":[101,101],"volume":[1000,1000],"amount":[100000,100000]}}`, timestamp, timestamp)))
	}))
	defer server.Close()
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	if _, err := provider.FetchDailyBars(ctx, "600519.SH", date.Format("2006-01-02"), date.Format("2006-01-02"), 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected duplicate date error")
	}
	issues := reporter.Snapshot().Issues
	foundDuplicate := false
	for _, issue := range issues {
		if issue.Code == "duplicate_trade_date" && issue.RowIndex != nil && *issue.RowIndex == 1 {
			foundDuplicate = true
			break
		}
	}
	if !foundDuplicate {
		t.Fatalf("missing duplicate-date diagnostic for second row: %+v", issues)
	}
}

func TestTickFlowDailyBarsReportsDateOutsideRequestedRange(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
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
	if _, err := provider.FetchDailyBars(ctx, "600519.SH", day, day, 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected out-of-range date error")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Code != "date_out_of_requested_range" || issues[0].RowIndex == nil || *issues[0].RowIndex != 0 {
		t.Fatalf("unexpected out-of-range diagnostic: %+v", issues)
	}
}

func TestTickFlowDailyBarsReportsMissingOptionalAmountAndPreviousClose(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	date := time.Now().In(location).AddDate(0, 0, -1)
	timestamp := time.Date(date.Year(), date.Month(), date.Day(), 12, 0, 0, 0, location).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(fmt.Sprintf(`{"code":0,"data":{"timestamp":[%d],"open":[100],"high":[102],"low":[99],"close":[101],"volume":[1000]}}`, timestamp)))
	}))
	defer server.Close()
	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	provider := NewTickFlowConfigured(2*time.Second, server.URL, "", "test-key")
	bars, err := provider.FetchDailyBars(ctx, "600519.SH", date.Format("2006-01-02"), date.Format("2006-01-02"), 10, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("FetchDailyBars: %v", err)
	}
	if len(bars) != 1 || bars[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded bar: %+v", bars)
	}
	fields := map[string]bool{}
	for _, issue := range reporter.Snapshot().Issues {
		fields[issue.Field] = true
	}
	for _, field := range []string{"amount", "prev_close"} {
		if !fields[field] {
			t.Errorf("missing Reporter field %q: %+v", field, reporter.Snapshot().Issues)
		}
	}
}
