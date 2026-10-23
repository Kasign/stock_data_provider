package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

type domesticMarketRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper domesticMarketRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return roundTripper(req)
}

func TestEastmoneyMarketBreadthDefaultsToDistributionEndpoint(t *testing.T) {
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", "", "", "")
	if p.marketBreadthURL != "https://push2ex.eastmoney.com/getTopicZDFenBu" {
		t.Fatalf("market breadth default endpoint = %q", p.marketBreadthURL)
	}
}

func TestEastmoneyFetchStockBoardsMapsBoardItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/slist/get" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		for key, want := range map[string]string{
			"secid": "1.600519", "pz": "1000", "pi": "0", "forcect": "1", "spt": "3",
			"fields": "f1,f12,f152,f3,f14,f128,f136", "fid": "f3", "fid0": "f4003",
		} {
			if got := query.Get(key); got != want {
				t.Errorf("%s=%q want %q", key, got, want)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{
			"total": 2, "diff": map[string]any{
				"0": map[string]any{"f12": "BK0477", "f14": "酿酒行业", "f3": 1.25},
				"1": map[string]any{"f12": "BK0896", "f14": "白酒", "f3": 2.5},
			},
		}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithStockBoards(2*time.Second, "", "", "", "", "", "", "", server.URL+"/slist/get", "")
	got, err := p.FetchStockBoards(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if got.Symbol != "600519.SH" || !got.HasData || got.Total != 2 || len(got.Items) != 2 {
		t.Fatalf("unexpected stock boards snapshot: %+v", got)
	}
	if got.Items[0].Code != "BK0477" || got.Items[0].Name != "酿酒行业" || got.Items[0].ChangePercent == nil || *got.Items[0].ChangePercent != 1.25 {
		t.Fatalf("unexpected first board: %+v", got.Items[0])
	}
	if got.QualityStatus != model.QualityDegraded || !strings.Contains(strings.Join(got.QualityReasons, ","), "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("timestamp quality downgrade missing: %+v", got)
	}
}

func TestDecodeEastmoneyBoardRowsSupportsArrayPayload(t *testing.T) {
	rows, err := decodeEastmoneyBoardRows(json.RawMessage(`[{"f12":"BK0477"},{"f12":"BK0896"}]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || parseEastmoneyBoardString(rows[0], "f12") != "BK0477" || parseEastmoneyBoardString(rows[1], "f12") != "BK0896" {
		t.Fatalf("unexpected decoded rows: %+v", rows)
	}
}

func TestEastmoneyFetchStockBoardsKeepsEmptyResultUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{"total": 0, "diff": []any{}}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithStockBoards(2*time.Second, "", "", "", "", "", "", "", server.URL+"/slist/get", "")
	got, err := p.FetchStockBoards(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if got.HasData || got.Total != 0 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("empty board list must remain unknown: %+v", got)
	}
}

func TestEastmoneyFetchMarketBreadthAggregatesDistributionBuckets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/getTopicZDFenBu" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("dpt") != "wz.ztzt" {
			t.Errorf("dpt=%q", r.URL.Query().Get("dpt"))
		}
		if r.URL.Query().Get("ut") == "" {
			t.Error("expected public Eastmoney token")
		}
		payload := map[string]any{"rc": 0, "data": map[string]any{
			"qdate": 20261023,
			"fenbu": []map[string]int{
				{"-11": 2}, {"-2": 3}, {"-1": 10}, {"0": 5}, {"1": 20}, {"11": 4},
			},
		}}
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", server.URL+"/getTopicZDFenBu", "", "")
	got, err := p.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.ProviderReportDate != "2026-10-23" {
		t.Fatalf("provider report date = %q", got.ProviderReportDate)
	}
	if got.TotalSecurities != 44 || got.CoveredSecurities != 44 || got.AdvancingCount != 24 || got.DecliningCount != 15 || got.UnchangedCount != 5 || got.UnclassifiedCount != 0 {
		t.Fatalf("unexpected distribution counts: %+v", got)
	}
	if got.TotalAmountCNY != nil || got.AmountCoverageCount != 0 {
		t.Fatalf("distribution endpoint must not invent turnover: %+v", got)
	}
	if got.QualityStatus != model.QualityDegraded ||
		!strings.Contains(strings.Join(got.QualityReasons, ","), "provider_snapshot_timestamp_not_exposed") ||
		!strings.Contains(strings.Join(got.QualityReasons, ","), "provider_market_amount_unavailable_from_distribution_endpoint") {
		t.Fatalf("missing conservative quality metadata: %+v", got)
	}
}

func TestEastmoneyFetchMarketBreadthRejectsInvalidDistribution(t *testing.T) {
	cases := map[string]string{
		"missing-data": `{"rc":0,"data":{"qdate":20261023}}`,
		"empty-buckets": `{"rc":0,"data":{"qdate":20261023,"fenbu":[]}}`,
		"bad-date": `{"rc":0,"data":{"qdate":"today","fenbu":[{"1":3}]}}`,
		"bad-key": `{"rc":0,"data":{"qdate":20261023,"fenbu":[{"12":3}]}}`,
		"negative-count": `{"rc":0,"data":{"qdate":20261023,"fenbu":[{"1":-3}]}}`,
		"duplicate-bucket": `{"rc":0,"data":{"qdate":20261023,"fenbu":[{"1":3},{"1":4}]}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.WriteString(w, body)
			}))
			defer server.Close()
			p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", server.URL+"/getTopicZDFenBu", "", "")
			if _, err := p.FetchMarketBreadth(context.Background()); err == nil {
				t.Fatalf("expected invalid distribution %s to be rejected", name)
			}
		})
	}
}

func TestEastmoneyFetchMarketBreadthRetriesTransientHTTP502(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{
			"qdate": 20261023, "fenbu": []map[string]int{{"-1": 2}, {"0": 1}, {"1": 3}},
		}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", server.URL+"/getTopicZDFenBu", "", "")
	got, err := p.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected one retry after transient 502, got %d requests", requests.Load())
	}
	if got.CoveredSecurities != 6 || got.AdvancingCount != 3 || got.DecliningCount != 2 || got.UnchangedCount != 1 {
		t.Fatalf("unexpected retry result: %+v", got)
	}
}

func TestEastmoneyFetchLimitPoolTypeDateAndFields(t *testing.T) {
	for _, tc := range []struct{ poolType, path, sort string }{
		{"up", "/getTopicZTPool", "fbt:asc"},
		{"down", "/getTopicDTPool", "fund:asc"},
		{"broken", "/getTopicZBPool", "fbt:asc"},
	} {
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
				if got := r.URL.Query().Get("sort"); got != tc.sort {
					t.Errorf("sort=%s, want %s", got, tc.sort)
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

func TestEastmoneyFetchLimitPoolMapsBeijing920CodeToBJ(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{
			"tc": 1, "qdate": 20261023, "pool": []map[string]any{{
				"c": "920285", "m": 0, "n": "N远大", "zdp": 20.0, "lbc": 1,
			}},
		}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchLimitPool(context.Background(), "up", "2026-10-23", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 1 || got.Items[0].Symbol != "920285.BJ" {
		t.Fatalf("Beijing stock with 920 prefix mapped incorrectly: %+v", got.Items)
	}
}

func TestEastmoneyFetchLimitPoolRetriesInconsistentPartialPayload(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{"tc": 9, "qdate": 20261023, "pool": []any{}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{
			"tc": 1, "qdate": 20261023, "pool": []map[string]any{{
				"c": "600519", "m": 1, "n": "贵州茅台", "zdp": 10.0, "lbc": 1,
			}},
		}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchLimitPool(context.Background(), "down", "2026-10-23", 10)
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected one semantic retry, got %d requests", requests.Load())
	}
	if !got.HasData || got.Total != 1 || len(got.Items) != 1 || got.Items[0].Symbol != "600519.SH" {
		t.Fatalf("unexpected retried pool: %+v", got)
	}
}

func TestEastmoneyFetchLimitPoolRejectsRepeatedInconsistentPartialPayload(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{"tc": 9, "qdate": 20261023, "pool": []any{}}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", "", server.URL, "")
	_, err := p.FetchLimitPool(context.Background(), "down", "2026-10-23", 10)
	if err == nil || !strings.Contains(err.Error(), "mismatch after retry") {
		t.Fatalf("expected persistent partial-response error, got %v", err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected exactly one retry, got %d requests", requests.Load())
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
