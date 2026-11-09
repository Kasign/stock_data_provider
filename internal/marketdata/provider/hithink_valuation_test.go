package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
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

	snapshot, err := NewHithinkConfigured(3*time.Second, server.URL, "unit-test-key").
		FetchValuations(context.Background(), []string{"600519.SH"})
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
}
