package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/baostock"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

// 这些测试锁定 Provider 请求参数与返回 metadata 的对应关系，
// 不把请求参数本身当成上游独立回显的复权证据。
func TestEastmoneyDailyBarAdjustmentRequestAndMetadata(t *testing.T) {
	cases := []struct {
		basis model.AdjustmentBasis
		fqt   string
	}{
		{model.AdjustmentNone, "0"},
		{model.AdjustmentForward, "1"},
		{model.AdjustmentBackward, "2"},
	}
	for _, tc := range cases {
		t.Run(string(tc.basis), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("fqt"); got != tc.fqt {
					t.Errorf("fqt=%q, want %q", got, tc.fqt)
				}
				fmt.Fprint(w, `{"rc":0,"data":{"code":"600519","name":"贵州茅台","klines":["2026-10-20,10,10.2,10.3,9.9,100,102000,1.42,0.81,10.87,0.23"]}}`)
			}))
			defer server.Close()
			p := NewEastmoney(2*time.Second, server.URL, server.URL)
			bars, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-20", "2026-10-20", 10, tc.basis)
			if err != nil {
				t.Fatal(err)
			}
			if len(bars) != 1 {
				t.Fatalf("bars=%d, want 1", len(bars))
			}
			if bars[0].AdjustmentBasis != tc.basis || bars[0].AdjustmentMethod != "eastmoney_fqt_"+tc.fqt {
				t.Fatalf("unexpected adjustment metadata: basis=%q method=%q", bars[0].AdjustmentBasis, bars[0].AdjustmentMethod)
			}
		})
	}
}

func TestBaoStockDailyBarAdjustmentFlagsAndMetadata(t *testing.T) {
	cases := []struct {
		basis  model.AdjustmentBasis
		flag   string
		method string
	}{
		{model.AdjustmentNone, "3", "none"},
		{model.AdjustmentForward, "2", "baostock_pct_change"},
		{model.AdjustmentBackward, "1", "baostock_pct_change"},
	}
	for _, tc := range cases {
		t.Run(string(tc.basis), func(t *testing.T) {
			client := &fakeBaostockClient{rows: []baostock.DailyKLine{{
				Date: "2026-10-21", Code: "sh.600519", Open: "10", High: "10.2",
				Low: "9.8", Close: "10.1", Volume: "1000", Amount: "10000",
				AdjustFlag: tc.flag, TradeStatus: "1",
			}}}
			bars, err := NewBaostock(client).FetchDailyBars(context.Background(), "600519.SH", "2026-10-21", "2026-10-21", 10, tc.basis)
			if err != nil {
				t.Fatal(err)
			}
			if client.gotAdjust != tc.flag {
				t.Fatalf("adjustFlag=%q, want %q", client.gotAdjust, tc.flag)
			}
			if len(bars) != 1 || bars[0].AdjustmentBasis != tc.basis || bars[0].AdjustmentMethod != tc.method {
				t.Fatalf("unexpected BaoStock adjustment result: %+v", bars)
			}
		})
	}
}

func TestHithinkDailyBarAdjustmentRequestAndMetadata(t *testing.T) {
	cases := []struct {
		basis  model.AdjustmentBasis
		adjust string
		method string
	}{
		{model.AdjustmentNone, "none", "hithink_api_none"},
		{model.AdjustmentForward, "forward", "hithink_api_forward"},
		{model.AdjustmentBackward, "backward", "hithink_api_backward"},
	}
	loc := time.FixedZone("CST", 8*60*60)
	dateMS := time.Date(2026, 10, 21, 0, 0, 0, 0, loc).UnixMilli()
	for _, tc := range cases {
		t.Run(string(tc.basis), func(t *testing.T) {
			requestCount := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requestCount++
				if got := r.URL.Query().Get("adjust"); got != tc.adjust {
					t.Errorf("adjust=%q, want %q", got, tc.adjust)
				}
				if requestCount == 1 {
					fmt.Fprintf(w, `{"code":0,"data":{"item":[{"date_ms":%d,"open_price":10,"high_price":10.2,"low_price":9.8,"close_price":10.1,"volume":1000,"turnover":10000}]}}`, dateMS)
					return
				}
				fmt.Fprint(w, `{"code":0,"data":{"item":[]}}`)
			}))
			defer server.Close()
			p := NewHithinkConfigured(2*time.Second, server.URL, "test-key")
			bars, err := p.FetchDailyBars(context.Background(), "600036.SH", "2026-10-21", "2026-10-21", 10, tc.basis)
			if err != nil {
				t.Fatal(err)
			}
			if len(bars) != 1 || bars[0].AdjustmentBasis != tc.basis || bars[0].AdjustmentMethod != tc.method {
				t.Fatalf("unexpected HiThink adjustment result: %+v", bars)
			}
			if requestCount < 1 {
				t.Fatal("expected historical K-line request")
			}
		})
	}
}

func TestTickFlowDailyBarAdjustmentRequestAndMetadata(t *testing.T) {
	cases := []struct {
		basis  model.AdjustmentBasis
		adjust string
		method string
	}{
		{model.AdjustmentNone, "none", "tickflow_api_none"},
		{model.AdjustmentForward, "forward", "tickflow_api_forward"},
		{model.AdjustmentBackward, "backward", "tickflow_api_backward"},
	}
	loc := time.FixedZone("CST", 8*60*60)
	timestamp := time.Date(2026, 10, 21, 15, 0, 0, 0, loc).UnixMilli()
	for _, tc := range cases {
		t.Run(string(tc.basis), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/klines" {
					t.Errorf("path=%q, want /v1/klines", r.URL.Path)
				}
				if got := r.URL.Query().Get("adjust"); got != tc.adjust {
					t.Errorf("adjust=%q, want %q", got, tc.adjust)
				}
				fmt.Fprintf(w, `{"code":0,"data":{"timestamp":[%d],"open":[10],"high":[10.2],"low":[9.8],"close":[10.1],"volume":[1000],"amount":[10000],"prev_close":[10]}}`, timestamp)
			}))
			defer server.Close()
			p := NewTickFlowConfigured(2*time.Second, server.URL, server.URL, "test-key")
			bars, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-21", "2026-10-21", 10, tc.basis)
			if err != nil {
				t.Fatal(err)
			}
			if len(bars) != 1 || bars[0].AdjustmentBasis != tc.basis || bars[0].AdjustmentMethod != tc.method {
				t.Fatalf("unexpected TickFlow adjustment result: %+v", bars)
			}
		})
	}
}

func TestHithinkAdjustmentBasisMetadataMapping(t *testing.T) {
	cases := map[string]model.AdjustmentBasis{
		"none":     model.AdjustmentNone,
		"forward":  model.AdjustmentForward,
		"backward": model.AdjustmentBackward,
	}
	for input, want := range cases {
		if got := model.AdjustmentBasis(hithinkAdjustmentBasis(input)); got != want {
			t.Errorf("hithinkAdjustmentBasis(%q)=%q, want %q", input, got, want)
		}
	}
}
