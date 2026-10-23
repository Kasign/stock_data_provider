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
)

type domesticMarketRoundTripper func(*http.Request) (*http.Response, error)

func (roundTripper domesticMarketRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return roundTripper(req)
}

func TestEastmoneyFetchMarketBreadthAggregatesFullReturnedUniverse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/clist/get" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("pz") != "5000" {
			t.Errorf("pz=%s", r.URL.Query().Get("pz"))
		}
		if r.URL.Query().Get("fid") != "f12" {
			t.Errorf("fid=%s, want stable sort by security code", r.URL.Query().Get("fid"))
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

func TestEastmoneyFetchMarketBreadthFallsBackToDelayHostOnHTTP502(t *testing.T) {
	var primaryCalls, delayCalls atomic.Int32
	p := NewEastmoneyConfiguredWithDomesticMarketData(
		2*time.Second, "", "", "", "", "",
		"https://push2.eastmoney.com/api/qt/clist/get", "", "",
	)
	p.client.Transport = domesticMarketRoundTripper(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Hostname() {
		case "push2.eastmoney.com":
			primaryCalls.Add(1)
			return &http.Response{Status: "502 Bad Gateway", StatusCode: http.StatusBadGateway, Header: make(http.Header), Body: http.NoBody, Request: req}, nil
		case "push2delay.eastmoney.com":
			delayCalls.Add(1)
			body := `{"rc":0,"data":{"total":1,"diff":[{"f12":"600519","f13":1,"f14":"贵州茅台","f3":1.2,"f6":120.0}]}}`
			return &http.Response{
				Status: "200 OK", StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}},
				Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: req,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected host %q", req.URL.Hostname())
		}
	})
	got, err := p.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if primaryCalls.Load() != 3 || delayCalls.Load() != 1 {
		t.Fatalf("expected three primary attempts and one delay fallback; primary=%d delay=%d", primaryCalls.Load(), delayCalls.Load())
	}
	if got.CoveredSecurities != 1 || got.AdvancingCount != 1 {
		t.Fatalf("unexpected market-breadth fallback snapshot: %+v", got)
	}
}

func TestEastmoneyFetchMarketBreadthDoesNotExposePartialAmountTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"rc": 0, "data": map[string]any{
			"total": 2,
			"diff": []map[string]any{
				{"f12": "600519", "f13": 1, "f14": "贵州茅台", "f3": 1.2, "f6": 120.0},
				{"f12": "000001", "f13": 0, "f14": "平安银行", "f3": -0.2},
			},
		}})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", server.URL+"/clist/get", server.URL, "")
	got, err := p.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.AmountCoverageCount != 1 || got.TotalAmountCNY != nil {
		t.Fatalf("partial market amount must not be summed as a total: %+v", got)
	}
	if !strings.Contains(strings.Join(got.QualityReasons, ","), "provider_market_amount_incomplete") {
		t.Fatalf("missing amount coverage quality reason: %+v", got.QualityReasons)
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

func TestEastmoneyFetchMarketBreadthRetriesTransientHTTP502(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"rc": 0,
			"data": map[string]any{
				"total": 1,
				"diff":  []map[string]any{{"f12": "600519", "f13": 1, "f14": "贵州茅台", "f3": 1.2, "f6": 12.0}},
			},
		})
	}))
	defer server.Close()
	p := NewEastmoneyConfiguredWithDomesticMarketData(2*time.Second, "", "", "", "", "", server.URL+"/clist/get", server.URL, "")
	got, err := p.FetchMarketBreadth(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 2 {
		t.Fatalf("expected one retry after transient 502, got %d requests", requests.Load())
	}
	if got.CoveredSecurities != 1 || got.AdvancingCount != 1 {
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
