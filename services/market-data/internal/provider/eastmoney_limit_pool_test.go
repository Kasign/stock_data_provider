package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEastmoneyFetchLimitPoolRejectsFutureTradeDateWithoutCallingProvider(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	futureDate := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "future trade date must be rejected before request", http.StatusBadRequest)
	}))
	defer server.Close()

	provider := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	_, err = provider.FetchLimitPool(context.Background(), "up", futureDate, 30)
	if err == nil || !strings.Contains(err.Error(), "must not be in the future") || called {
		t.Fatalf("future limit-pool date must be rejected before request: err=%v called=%v", err, called)
	}
}

func TestEastmoneyFetchLimitPoolMarksPartialCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/getTopicZTPool") {
			t.Errorf("path=%q, want getTopicZTPool", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"rc": 0,
			"data": {
				"tc": 2,
				"qdate": "20261027",
				"pool": [{"c":"600519","n":"贵州茅台","m":"1","zdp":1.2}]
			}
		}`))
	}))
	defer server.Close()

	provider := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	got, err := provider.FetchLimitPool(context.Background(), "up", "2026-10-27", 1)
	if err != nil {
		t.Fatalf("FetchLimitPool: %v", err)
	}
	if got.Total != 2 || len(got.Items) != 1 {
		t.Fatalf("fixture should contain total=2 but one returned row, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "provider_total_exceeds_returned_rows" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("partial limit-pool coverage must be explicit: %v", got.QualityReasons)
	}
}

