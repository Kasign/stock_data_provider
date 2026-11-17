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

func TestEastmoneyFetchLimitPoolPaginatesTruncatedPages(t *testing.T) {
	const total = 65
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	tradeDate := time.Now().In(loc).AddDate(0, 0, -1)
	tradeDateISO, tradeDateCompact := tradeDate.Format("2006-01-02"), tradeDate.Format("20060102")
	pageCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCalls++
		query := r.URL.Query()
		page, err := strconv.Atoi(query.Get("Pageindex"))
		if err != nil {
			t.Errorf("invalid Pageindex: %v", err)
			http.Error(w, "bad page index", http.StatusBadRequest)
			return
		}
		if got := query.Get("pagesize"); got != "30" {
			t.Errorf("pagesize=%q, want 30 for paginated requests", got)
		}
		if got := query.Get("date"); got != tradeDateCompact {
			t.Errorf("date=%q, want %s", got, tradeDateCompact)
		}
		start := page * 30
		end := start + 30
		if end > total {
			end = total
		}
		if start >= total {
			end = start
		}
		rows := make([]map[string]any, 0, end-start)
		for i := start; i < end; i++ {
			rows = append(rows, map[string]any{
				"c": fmt.Sprintf("%06d", 600001+i), "n": fmt.Sprintf("测试股票%d", i+1),
				"m": "1", "zdp": 10.0,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rc": 0, "data": map[string]any{"tc": total, "qdate": tradeDateCompact, "pool": rows},
		})
	}))
	defer server.Close()

	provider := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	got, err := provider.FetchLimitPool(context.Background(), "up", tradeDateISO, 100)
	if err != nil {
		t.Fatalf("FetchLimitPool: %v", err)
	}
	if pageCalls != 3 {
		t.Fatalf("page calls=%d, want 3", pageCalls)
	}
	if got.Total != total || len(got.Items) != total {
		t.Fatalf("limit pool should return all %d rows after pagination, total=%d items=%d", total, got.Total, len(got.Items))
	}
	if got.ProviderReturned != total {
		t.Fatalf("source coverage ledger should count the raw rows returned across pages, got %d want %d", got.ProviderReturned, total)
	}
	if got.Items[0].Code != "600001" || got.Items[len(got.Items)-1].Code != "600065" {
		t.Fatalf("pagination order/range mismatch: first=%+v last=%+v", got.Items[0], got.Items[len(got.Items)-1])
	}
	for _, reason := range got.QualityReasons {
		if reason == "provider_total_exceeds_returned_rows" {
			t.Fatalf("complete pagination must not report partial coverage: %v", got.QualityReasons)
		}
	}
}

func TestEastmoneyFetchLimitPoolRejectsRowsAboveSourceTotal(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	tradeDate := time.Now().In(loc).AddDate(0, 0, -1).Format("2006-01-02")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("pagesize"); got != "30" {
			t.Errorf("pagesize=%q, want 30", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"rc": 0,
			"data": {
				"tc": 2,
				"qdate": "` + strings.ReplaceAll(tradeDate, "-", "") + `",
				"pool": [
					{"c":"600519","n":"贵州茅台","m":"1","zdp":1.2},
					{"c":"600036","n":"招商银行","m":"1","zdp":0.5},
					{"c":"601318","n":"中国平安","m":"1","zdp":0.3}
				]
			}
		}`))
	}))
	defer server.Close()

	provider := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "", "", server.URL, "",
	)
	got, err := provider.FetchLimitPool(context.Background(), "up", tradeDate, 30)
	if err == nil || !strings.Contains(err.Error(), "coverage mismatch") {
		t.Fatalf("source total smaller than returned rows must be rejected: snapshot=%+v err=%v", got, err)
	}
}
