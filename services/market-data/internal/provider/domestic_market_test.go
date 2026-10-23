package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEastmoneyFetchMarketBreadthAggregatesFullReturnedUniverse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/clist/get" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("pz") != "5000" {
			t.Errorf("pz=%s", r.URL.Query().Get("pz"))
		}
		payload := map[string]any{"rc": 0, "data": map[string]any{
			"total": 3, "diff": []map[string]any{
				{"f12": "600519", "f13": 1, "f14": "贵州茅台", "f3": 1.5, "f6": 100.5},
				{"f12": "000001", "f13": 0, "f14": "平安银行", "f3": -0.5, "f6": 200.0},
				{"f12": "601398", "f13": 1, "f14": "工商银行", "f3": 0.0, "f6": 300.0},
			},
		}}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", server.URL+"/clist/get", server.URL, "")
	got, err := p.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalSecurities != 3 || got.CoveredSecurities != 3 || got.AdvancingCount != 1 || got.DecliningCount != 1 || got.UnchangedCount != 1 || got.UnclassifiedCount != 0 {
		t.Fatalf("unexpected counts: %+v", got)
	}
	if got.TotalAmountCNY == nil || *got.TotalAmountCNY != 600.5 || got.AmountCoverageCount != 3 {
		t.Fatalf("unexpected amount coverage: %+v", got)
	}
	if got.QualityStatus != "degraded" || !strings.Contains(strings.Join(got.QualityReasons, ","), "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("missing timestamp downgrade: %+v", got)
	}
}

func TestEastmoneyFetchMarketBreadthRejectsIncompleteCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{"total": 3, "diff": []map[string]any{
			{"f12": "600519", "f13": 1, "f14": "贵州茅台", "f3": 1.0, "f6": 100.0},
			{"f12": "000001", "f13": 0, "f14": "平安银行", "f3": -1.0, "f6": 200.0},
		}}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", server.URL+"/clist/get", server.URL, "")
	_, err := p.FetchMarketBreadth(context.Background())
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("expected incomplete coverage error, got %v", err)
	}
}

func TestEastmoneyFetchLimitPoolTypeDateAndFields(t *testing.T) {
	for _, tc := range []struct{ poolType, path string }{{"up", "/getTopicZTPool"}, {"down", "/getTopicDTPool"}, {"broken", "/getTopicZBPool"}} {
		t.Run(tc.poolType, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path=%s want %s", r.URL.Path, tc.path)
				}
				if r.URL.Query().Get("date") != "20261022" {
					t.Errorf("date=%s", r.URL.Query().Get("date"))
				}
				if r.URL.Query().Get("pagesize") != "10" {
					t.Errorf("pagesize=%s", r.URL.Query().Get("pagesize"))
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{
					"tc": 1, "qdate": 20261023, "pool": []map[string]any{{
						"c": "600519", "m": 1, "n": "贵州茅台", "zdp": 10.0, "lbc": 2, "fbt": 93025, "lbt": 145500, "zbc": 1, "hybk": "白酒",
					}},
				}})
			}))
			defer server.Close()
			p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", "", server.URL, "")
			got, err := p.FetchLimitPool(context.Background(), tc.poolType, "2026-10-22", 10)
			if err != nil {
				t.Fatal(err)
			}
			if got.PoolType != tc.poolType || got.TradeDate != "2026-10-22" || got.ProviderReportDate != "2026-10-23" || !got.HasData || got.Total != 1 || len(got.Items) != 1 {
				t.Fatalf("unexpected snapshot: %+v", got)
			}
			item := got.Items[0]
			if item.Symbol != "600519.SH" || item.FirstTouchTime != "09:30:25" || item.LastTouchTime != "14:55:00" || item.ConsecutiveBoards == nil || *item.ConsecutiveBoards != 2 || item.BrokenCount == nil || *item.BrokenCount != 1 {
				t.Fatalf("unexpected item: %+v", item)
			}
		})
	}
}

func TestEastmoneyFetchLimitPoolDoesNotClaimEmptyResponseIsZeroEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{"tc": 0, "qdate": 20261023, "pool": []any{}}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchLimitPool(context.Background(), "down", "2026-10-23", 10)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasData || got.Total != 0 || got.QualityStatus != "unknown" {
		t.Fatalf("empty response must remain unknown: %+v", got)
	}
}

func TestEastmoneyFetchLimitPoolRejectsInvalidPoolType(t *testing.T) {
	p := NewEastmoney(2*time.Second, "", "")
	_, err := p.FetchLimitPool(context.Background(), "all", "2026-10-23", 10)
	if err == nil || !strings.Contains(err.Error(), "unsupported limit-pool type") {
		t.Fatalf("got %v", err)
	}
}
