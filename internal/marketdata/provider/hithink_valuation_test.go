package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/validation"
)

func TestHithinkValuationsWithoutReportDateRemainDegraded(t *testing.T) {
	timestamp := time.Now().UTC().UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/a-share/valuations/snapshot" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-api-key") != "unit-test-key" {
			http.Error(w, "missing API key header", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"timestamp":%d,"total":1,"item":[{"thscode":"600519.SH","ticker":"600519","name":"贵州茅台","pe_ttm":25.0,"pe_mrq":24.0,"pb_mrq":8.0,"ps_ttm":10.0,"pcf_ttm":20.0}]}}`, timestamp)
	}))
	t.Cleanup(server.Close)

	p := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key")
	snapshot, err := p.FetchValuations(context.Background(), []string{"600519.SH"})
	if err != nil {
		t.Fatalf("FetchValuations returned error: %v", err)
	}
	if snapshot.ProviderTimestamp == nil || snapshot.ProviderReportDate != "" {
		t.Fatalf("expected exact timestamp and absent report date to remain distinct, got timestamp=%v report_date=%q",
			snapshot.ProviderTimestamp, snapshot.ProviderReportDate)
	}
	if snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing report date must prevent valid quality, got %q", snapshot.QualityStatus)
	}
	found := false
	for _, reason := range snapshot.QualityReasons {
		if reason == "provider_valuation_report_date_not_exposed" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing report-date evidence reason not recorded: %v", snapshot.QualityReasons)
	}
}


func TestHithinkValuationsDistinguishesMissingAndInvalidMetrics(t *testing.T) {
	timestamp := time.Now().UTC().UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"timestamp":%d,"total":1,"item":[{"thscode":"600519.SH","ticker":"600519","name":"贵州茅台","pe_ttm":-12.5,"pe_mrq":0,"pb_mrq":null,"ps_ttm":"3.5","pcf_ttm":"N/A"}]}}`, timestamp)
	}))
	t.Cleanup(server.Close)

	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	snapshot, err := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key").
		FetchValuations(ctx, []string{"600519.SH"})
	if err != nil {
		t.Fatalf("FetchValuations returned error: %v", err)
	}
	if len(snapshot.Items) != 1 {
		t.Fatalf("expected one valuation item, got %d", len(snapshot.Items))
	}
	item := snapshot.Items[0]
	if item.PETTM == nil || *item.PETTM != -12.5 {
		t.Fatalf("negative PE must be preserved, got %v", item.PETTM)
	}
	if item.PEMRQ == nil || *item.PEMRQ != 0 {
		t.Fatalf("numeric zero must not be treated as missing, got %v", item.PEMRQ)
	}
	if item.PSTTM == nil || *item.PSTTM != 3.5 {
		t.Fatalf("numeric string must be parsed, got %v", item.PSTTM)
	}
	if item.PBMRQ != nil || item.PCFTTM != nil {
		t.Fatalf("null and invalid metrics must remain nil, got PB=%v PCF=%v", item.PBMRQ, item.PCFTTM)
	}
	for _, expected := range []string{"valuation_metric_missing:pb_mrq", "valuation_metric_invalid:pcf_ttm"} {
		found := false
		for _, reason := range item.QualityReasons {
			if reason == expected {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected field-specific reason %q, got %v", expected, item.QualityReasons)
		}
	}
	if item.QualityStatus != model.QualityDegraded || snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing/invalid metrics must degrade item and snapshot, got item=%q snapshot=%q",
			item.QualityStatus, snapshot.QualityStatus)
	}
	issues := reporter.Snapshot().Issues
	wantIssues := map[string]string{"pb_mrq": "missing_value", "pcf_ttm": "invalid_number"}
	for field, code := range wantIssues {
		found := false
		for _, issue := range issues {
			if issue.Field == field && issue.ProviderField == field && issue.Code == code &&
				issue.Stage == validation.StageParse && issue.Symbol == "600519.SH" &&
				issue.RowIndex != nil && *issue.RowIndex == 0 {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected valuation parse issue %s/%s, got %+v", field, code, issues)
		}
	}
}


func TestHithinkValuationsRejectsTimestampBeyondServiceFutureSkew(t *testing.T) {
	timestamp := time.Now().UTC().Add(2 * time.Minute).UnixMilli()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"timestamp":%d,"total":1,"item":[{"thscode":"600519.SH","ticker":"600519","pe_ttm":25,"pe_mrq":24,"pb_mrq":8,"ps_ttm":10,"pcf_ttm":20}]}}`, timestamp)
	}))
	t.Cleanup(server.Close)

	snapshot, err := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key").
		FetchValuations(context.Background(), []string{"600519.SH"})
	if err != nil {
		t.Fatalf("FetchValuations returned error: %v", err)
	}
	if snapshot.ProviderTimestamp != nil {
		t.Fatalf("provider timestamp beyond service's one-minute future skew must be rejected: %v", snapshot.ProviderTimestamp)
	}
	if snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("missing/invalid provider timestamp must degrade snapshot: %+v", snapshot)
	}
	if !containsHithinkReason(snapshot.QualityReasons, "provider_valuation_timestamp_missing_or_invalid") {
		t.Fatalf("invalid timestamp reason missing: %+v", snapshot.QualityReasons)
	}
}


func TestHithinkValuationsReportsInvalidProviderTimestampWithoutDroppingRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"code":0,"data":{"timestamp":"not-a-timestamp","total":1,"item":[{"thscode":"600519.SH","ticker":"600519","name":"贵州茅台","pe_ttm":25.0,"pe_mrq":24.0,"pb_mrq":8.0,"ps_ttm":10.0,"pcf_ttm":20.0}]}}`)
	}))
	defer server.Close()

	reporter := validation.NewReporter(validation.DefaultMaxIssues)
	ctx := validation.WithReporter(context.Background(), reporter)
	snapshot, err := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key").
		FetchValuations(ctx, []string{"600519.SH"})
	if err != nil {
		t.Fatalf("invalid optional provider timestamp should not discard valid valuation rows: %v", err)
	}
	if snapshot.ProviderTimestamp != nil || len(snapshot.Items) != 1 || snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("invalid provider timestamp must remain explicit and degrade snapshot: %+v", snapshot)
	}
	found := false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "provider_timestamp" && issue.ProviderField == "timestamp" &&
			issue.Code == "invalid_timestamp" && issue.ActualType == "string" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected provider timestamp parse issue, got %+v", reporter.Snapshot().Issues)
	}
}


func TestHithinkValuationsReportsMissingNameAndRequestedSymbol(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"code":0,"data":{"timestamp":%d,"total":1,"item":[{"thscode":"600519.SH","ticker":"600519","name":"","pe_ttm":25.0,"pe_mrq":24.0,"pb_mrq":8.0,"ps_ttm":10.0,"pcf_ttm":20.0}]}}`, time.Now().UTC().UnixMilli())
	}))
	defer server.Close()

	reporter := validation.NewReporter(10)
	ctx := validation.WithReporter(context.Background(), reporter)
	snapshot, err := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key").
		FetchValuations(ctx, []string{"600519.SH", "600036.SH"})
	if err != nil {
		t.Fatalf("FetchValuations: %v", err)
	}
	if len(snapshot.Items) != 1 || len(snapshot.MissingSymbols) != 1 || snapshot.MissingSymbols[0] != "600036.SH" {
		t.Fatalf("unexpected valuation coverage: %+v", snapshot)
	}
	foundName, foundMissing := false, false
	for _, issue := range reporter.Snapshot().Issues {
		if issue.Field == "name" && issue.ProviderField == "name" && issue.Code == "missing_value" &&
			issue.RowIndex != nil && *issue.RowIndex == 0 {
			foundName = true
		}
		if issue.Field == "symbol" && issue.ProviderField == "thscode" && issue.Code == "missing_requested_row" &&
			issue.Symbol == "600036.SH" && issue.RowIndex == nil {
			foundMissing = true
		}
	}
	if !foundName || !foundMissing {
		t.Fatalf("missing valuation coverage Reporter evidence: name=%v missing_symbol=%v issues=%+v",
			foundName, foundMissing, reporter.Snapshot().Issues)
	}
}

func TestHithinkValuationsFailFastPathsReportStructuredIssues(t *testing.T) {
	valid := `{"thscode":"600519.SH","ticker":"600519","name":"贵州茅台","pe_ttm":25,"pe_mrq":24,"pb_mrq":8,"ps_ttm":10,"pcf_ttm":20}`
	cases := []struct {
		name, total, rows, field, providerField, code string
		rowIndex int
	}{
		{name: "total mismatch", total: "2", rows: valid, field: "row_count", providerField: "total/item", code: "provider_count_mismatch", rowIndex: -1},
		{name: "invalid symbol", total: "1", rows: `{"thscode":"not-a-symbol","ticker":"x"}`, field: "symbol", providerField: "thscode", code: "invalid_symbol", rowIndex: 0},
		{name: "unrequested symbol", total: "1", rows: strings.Replace(valid, "600519.SH", "600036.SH", 1), field: "symbol", providerField: "thscode", code: "unrequested_identity", rowIndex: 0},
		{name: "duplicate symbol", total: "2", rows: valid + "," + valid, field: "symbol", providerField: "thscode", code: "duplicate_identity", rowIndex: 1},
		{name: "ticker mismatch", total: "1", rows: strings.Replace(valid, `"ticker":"600519"`, `"ticker":"000001"`, 1), field: "code", providerField: "ticker", code: "symbol_code_mismatch", rowIndex: 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"code":0,"data":{"timestamp":` + fmt.Sprint(time.Now().UTC().UnixMilli()) + `,"total":` + tc.total + `,"item":[` + tc.rows + `]}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprint(w, payload)
			}))
			defer server.Close()
			reporter := validation.NewReporter(20)
			ctx := validation.WithReporter(context.Background(), reporter)
			_, err := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key").FetchValuations(ctx, []string{"600519.SH"})
			if err == nil { t.Fatal("expected fail-fast validation error") }
			found := false
			for _, issue := range reporter.Snapshot().Issues {
				if issue.Stage != validation.StageValidate || issue.Field != tc.field || issue.ProviderField != tc.providerField || issue.Code != tc.code { continue }
				if tc.rowIndex < 0 && issue.RowIndex == nil { found = true }
				if tc.rowIndex >= 0 && issue.RowIndex != nil && *issue.RowIndex == tc.rowIndex { found = true }
			}
			if !found { t.Fatalf("expected Reporter issue %s/%s/%s at row %d, got %+v", tc.field, tc.providerField, tc.code, tc.rowIndex, reporter.Snapshot().Issues) }
		})
	}
}
