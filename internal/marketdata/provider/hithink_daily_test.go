package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestHithinkDailyBarParseIssuesIncludeRowAndProviderField(t *testing.T) {
	payload := `{"code":0,"data":{"item":[{"date_ms":1792454400000,"open_price":"bad","high_price":11,"low_price":9,"close_price":10,"volume":1000,"turnover":10000}]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-api-key") != "test-key" {
			t.Errorf("missing API key header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	if _, err := provider.FetchDailyBars(ctx, "600036.SH", "2026-10-20", "2026-10-21", 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected invalid open price to reject the response")
	}
	found := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "open" && issue.ProviderField == "open_price" && issue.Code == "type_mismatch" && issue.RowIndex != nil && *issue.RowIndex == 0 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected structured open_price issue at row 0, got %+v", reporter.Snapshot().Issues)
	}
}

func TestHithinkDailyBarsFetchesSingleResponseWithoutOffset(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	dateMS := func(day int) int64 {
		return time.Date(2026, time.October, day, 0, 0, 0, 0, loc).UnixMilli()
	}
	row := func(day int) string {
		return fmt.Sprintf("{\"date_ms\":%d,\"open_price\":10,\"high_price\":11,\"low_price\":9,\"close_price\":10,\"volume\":1000,\"turnover\":10000}", dateMS(day))
	}
	payload := "{\"code\":0,\"data\":{\"item\":[" + strings.Join([]string{row(7), row(8), row(9)}, ",") + "]}}"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/a-share/prices/historical" {
			t.Errorf("unexpected request path %q", r.URL.Path)
		}
		if _, exists := r.URL.Query()["offset"]; exists {
			t.Errorf("historical K-line endpoint does not support offset pagination")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")
	got, err := provider.FetchDailyBars(context.Background(), "600036.SH", "2026-10-07", "2026-10-09", 10, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("FetchDailyBars: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected exactly one historical request, got %d", requests)
	}
	if len(got) != 3 || got[0].TradeDate != "2026-10-07" || got[2].TradeDate != "2026-10-09" {
		t.Fatalf("expected all three rows from one response, got %+v", got)
	}
}

func TestHithinkDailyResponseRejectsDuplicateDatesWithinSingleResponse(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	dateMS := func(day int) int64 {
		return time.Date(2026, time.October, day, 0, 0, 0, 0, loc).UnixMilli()
	}
	row := func(day int) string {
		return fmt.Sprintf("{\"date_ms\":%d,\"open_price\":10,\"high_price\":11,\"low_price\":9,\"close_price\":10,\"volume\":1000,\"turnover\":10000}", dateMS(day))
	}
	payload := "{\"code\":0,\"data\":{\"item\":[" + strings.Join([]string{row(7), row(8), row(8)}, ",") + "]}}"
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if _, exists := r.URL.Query()["offset"]; exists {
			t.Errorf("historical K-line endpoint does not support offset pagination")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(payload))
	}))
	defer server.Close()

	provider := NewHithinkConfigured(time.Second, server.URL, "test-key")
	reporter := validation.NewReporter(20)
	ctx := validation.WithReporter(context.Background(), reporter)
	_, err := provider.FetchDailyBars(ctx, "600036.SH", "2026-10-07", "2026-10-09", 10, model.AdjustmentNone)
	if err == nil || !strings.Contains(err.Error(), "重复交易日 2026-10-08") ||
		!strings.Contains(err.Error(), "first_row=1 duplicate_row=2 returned_rows=3") {
		t.Fatalf("expected single-response duplicate diagnostic, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected exactly one historical request, got %d", requests)
	}
	found := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Stage == validation.StageParse && issue.Field == "trade_date" &&
			issue.ProviderField == "date_ms" && issue.Code == "duplicate_trade_date" &&
			issue.RowIndex != nil && *issue.RowIndex == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected structured duplicate trade-date issue at row 2, got %+v", reporter.Snapshot().Issues)
	}
}

func TestHithinkIndexDailyBarsReportsOptionalFieldIssues(t *testing.T) {
	loc := time.FixedZone("CST", 8*60*60)
	dateMS := time.Date(2026, time.November, 6, 0, 0, 0, 0, loc).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-share-index/prices/historical" {
			t.Errorf("unexpected request path %q", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		payload := map[string]any{
			"code": 0,
			"data": map[string]any{
				"item": []map[string]any{{
					"date_ms": dateMS, "open_price": 10.0, "high_price": 11.0,
					"low_price": 9.0, "close_price": 10.5, "volume": -1.0, "turnover": -2.0,
				}},
			},
		}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewHithinkConfigured(time.Second, server.URL, "test-key")
	got, err := p.FetchIndexDailyBars(ctx, "000001.SH", "2026-11-06", "2026-11-06")
	if err != nil {
		t.Fatalf("FetchIndexDailyBars: %v", err)
	}
	if len(got.Items) != 1 || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected one degraded index bar, got %+v", got)
	}
	seen := map[string]bool{}
	for _, issue := range reporter.Snapshot().Issues {
		seen[issue.Field+":"+issue.ProviderField+":"+issue.Code] = true
		if issue.Field != "provider_time" && (issue.RowIndex == nil || *issue.RowIndex != 0) {
			t.Errorf("row-level issue must point to row 0: %+v", issue)
		}
	}
	for _, key := range []string{
		"provider_time:timestamp:missing_or_invalid_value",
		"volume:volume:missing_or_invalid_value",
		"amount:turnover:missing_or_invalid_value",
	} {
		if !seen[key] {
			t.Errorf("missing Reporter issue %q; got %+v", key, reporter.Snapshot().Issues)
		}
	}
}

func TestHithinkDailyResponseFailuresAreReportedAtSnapshotLevel(t *testing.T) {
	cases := []struct {
		name     string
		payload  string
		wantCode string
	}{
		{name: "provider rejection", payload: `{"code":1001,"message":"quota exceeded"}`, wantCode: "provider_response_error"},
		{name: "missing data", payload: `{"code":0}`, wantCode: "provider_response_error"},
		{name: "empty result", payload: `{"code":0,"data":{"item":[]}}`, wantCode: "empty_result"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.payload))
			}))
			defer server.Close()
			reporter := validation.NewReporter(10)
			ctx := validation.WithReporter(context.Background(), reporter)
			p := NewHithinkConfigured(time.Second, server.URL, "test-key")
			if _, err := p.FetchDailyBars(ctx, "600036.SH", "2026-10-20", "2026-10-21", 10, model.AdjustmentNone); err == nil {
				t.Fatal("expected response to be rejected")
			}
			issues := reporter.Snapshot().Issues
			if len(issues) != 1 || issues[0].Stage != validation.StageParse || issues[0].Code != tc.wantCode || issues[0].RowIndex != nil {
				t.Fatalf("expected one snapshot-level %s issue, got %+v", tc.wantCode, issues)
			}
		})
	}
}

func TestHithinkDailyRequestFailureIsReportedAtSnapshotLevel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	p := NewHithinkConfigured(time.Second, server.URL, "test-key")
	if _, err := p.FetchDailyBars(ctx, "600036.SH", "2026-10-20", "2026-10-21", 10, model.AdjustmentNone); err == nil {
		t.Fatal("expected HTTP failure")
	}
	issues := reporter.Snapshot().Issues
	if len(issues) != 1 || issues[0].Code != "request_error" || issues[0].RowIndex != nil {
		t.Fatalf("expected one snapshot-level request_error issue, got %+v", issues)
	}
}
