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
