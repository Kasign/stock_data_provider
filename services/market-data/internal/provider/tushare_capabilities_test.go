package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

type tushareIndexWeightRequest struct {
	APIName string            `json:"api_name"`
	Params  map[string]string `json:"params"`
}

func writeTushareTestTable(w http.ResponseWriter, fields []string, items [][]any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code": 0,
		"msg":  "ok",
		"data": map[string]any{"fields": fields, "items": items},
	})
}

func tushareTestMonthWindow(now time.Time, offset int) (string, string) {
	first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, -offset, 0)
	last := first.AddDate(0, 1, -1)
	return first.Format("20060102"), last.Format("20060102")
}

func TestTushareDailyRejectsEmptyOrDuplicateFieldNames(t *testing.T) {
	cases := []struct {
		name   string
		fields []string
		want   string
	}{
		{name: "empty field name", fields: []string{"ts_code", "", "trade_date"}, want: "empty field name"},
		{name: "duplicate field name", fields: []string{"ts_code", "trade_date", "ts_code"}, want: "duplicate field"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeTushareTestTable(w, tc.fields, [][]any{})
			}))
			defer server.Close()

			provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
			_, err := provider.FetchDailyBars(context.Background(), "600519.SH", "2026-10-26", "2026-10-26", 10, model.AdjustmentNone)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("invalid Tushare response field table should be rejected, got %v", err)
			}
		})
	}
}

func TestTushareIndexConstituentsQueriesOnlyNewestAvailableMonth(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	wantStart, wantEnd := tushareTestMonthWindow(now, 0)
	var weightRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "index_weight":
			weightRequests++
			if request.Params["index_code"] != "000300.SH" || request.Params["start_date"] != wantStart || request.Params["end_date"] != wantEnd {
				t.Errorf("unexpected index_weight request params: %#v", request.Params)
			}
			writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{
				{"000300.SH", "600519.SH", now.Format("20060102"), 3.5},
			})
		case "stock_basic":
			writeTushareTestTable(w, []string{"ts_code", "name"}, [][]any{{"600519.SH", "贵州茅台"}})
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected api", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := provider.FetchIndexConstituents(context.Background(), "000300.SH")
	if err != nil {
		t.Fatal(err)
	}
	if weightRequests != 1 {
		t.Fatalf("expected one monthly index_weight request, got %d", weightRequests)
	}
	if got.ProviderReportDate != now.Format("2006-01-02") || got.Total != 1 || got.ProviderReturned != 1 || got.ExcludedCount != 0 || got.Items[0].Symbol != "600519.SH" {
		t.Fatalf("unexpected index constituent snapshot: %+v", got)
	}
}

func TestTushareIndexConstituentsFallsBackToPreviousMonth(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	currentStart, currentEnd := tushareTestMonthWindow(now, 0)
	previousStart, previousEnd := tushareTestMonthWindow(now, 1)
	previousMonthEnd := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location).AddDate(0, 0, -1)
	var weightStarts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "index_weight":
			weightStarts = append(weightStarts, request.Params["start_date"])
			switch request.Params["start_date"] {
			case currentStart:
				if request.Params["end_date"] != currentEnd {
					t.Errorf("current month end = %q, want %q", request.Params["end_date"], currentEnd)
				}
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{})
			case previousStart:
				if request.Params["end_date"] != previousEnd {
					t.Errorf("previous month end = %q, want %q", request.Params["end_date"], previousEnd)
				}
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{
					{"000300.SH", "600519.SH", previousMonthEnd.Format("20060102"), 3.25},
				})
			default:
				t.Errorf("unexpected index_weight start date %q", request.Params["start_date"])
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{})
			}
		case "stock_basic":
			writeTushareTestTable(w, []string{"ts_code", "name"}, [][]any{{"600519.SH", "贵州茅台"}})
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected api", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := provider.FetchIndexConstituents(context.Background(), "000300.SH")
	if err != nil {
		t.Fatal(err)
	}
	if len(weightStarts) != 2 || weightStarts[0] != currentStart || weightStarts[1] != previousStart {
		t.Fatalf("expected newest-first monthly requests, got %v", weightStarts)
	}
	if got.ProviderReportDate != previousMonthEnd.Format("2006-01-02") || got.Total != 1 {
		t.Fatalf("unexpected previous-month constituent snapshot: %+v", got)
	}
}

func TestTushareStockBasicRejectsNonAShareBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	for _, symbol := range []string{"510300.SH", "159919.SZ", "900901.SH"} {
		if _, err := provider.FetchStockBasic(context.Background(), symbol); err == nil {
			t.Errorf("non-A-share instrument %s must be rejected", symbol)
		}
	}
	if requests != 0 {
		t.Fatalf("non-A-share requests must be rejected before upstream access; got %d request(s)", requests)
	}
}

func TestTushareStockBasicRejectsUnknownMarketInsteadOfAssumingMainBoard(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTushareTestTable(w,
			[]string{"ts_code", "name", "industry", "market", "exchange", "list_status", "list_date", "delist_date"},
			[][]any{{"600519.SH", "贵州茅台", "白酒", "", "SSE", "L", "20010827", ""}},
		)
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	if _, err := provider.FetchStockBasic(context.Background(), "600519.SH"); err == nil {
		t.Fatal("missing market classification must be rejected rather than silently mapped to main board")
	}
}

func TestTushareStockBasicMapsKnownMarketClassification(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTushareTestTable(w,
			[]string{"ts_code", "name", "industry", "market", "exchange", "list_status", "list_date", "delist_date"},
			[][]any{{"600519.SH", "贵州茅台", "白酒", "主板", "SSE", "L", "20010827", ""}},
		)
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := provider.FetchStockBasic(context.Background(), "600519.SH")
	if err != nil {
		t.Fatal(err)
	}
	if got.SecurityType != 1 || got.SecurityTypeLabel != "主板股票" {
		t.Fatalf("known main-board classification mapped incorrectly: %+v", got)
	}
}

func TestTushareValuationKeepsStaticPESeparateFromMRQ(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	reference := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, location)
	if now.Hour() < 18 {
		reference = reference.AddDate(0, 0, -1)
	}
	tradeDate := reference.Format("20060102")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "trade_cal":
			writeTushareTestTable(w, []string{"exchange", "cal_date", "is_open"}, [][]any{{"SSE", tradeDate, 1}})
		case "daily_basic":
			if request.Params["trade_date"] != tradeDate {
				t.Errorf("daily_basic trade_date = %q, want %q", request.Params["trade_date"], tradeDate)
			}
			writeTushareTestTable(w, []string{"ts_code", "trade_date", "pe", "pe_ttm", "pb", "ps_ttm"}, [][]any{
				{"600519.SH", tradeDate, 22.5, 18.4, 6.1, 7.2},
			})
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected api", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := provider.FetchValuations(context.Background(), []string{"600519.SH"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || len(got.Items) != 1 {
		t.Fatalf("unexpected valuation snapshot: %+v", got)
	}
	item := got.Items[0]
	if item.PEStatic == nil || *item.PEStatic != 22.5 {
		t.Fatalf("expected source-native static PE, got %+v", item.PEStatic)
	}
	if item.PEMRQ != nil {
		t.Fatalf("Tushare daily_basic pe must not be labelled pe_mrq, got %+v", item.PEMRQ)
	}
	if item.PETTM == nil || *item.PETTM != 18.4 {
		t.Fatalf("expected TTM PE to remain mapped, got %+v", item.PETTM)
	}
	if !containsTushareReason(item.QualityReasons, "provider_pe_mrq_not_supported_by_tushare_daily_basic") {
		t.Fatalf("expected missing-MRQ quality reason, got %+v", item.QualityReasons)
	}
}

func containsTushareReason(reasons []string, target string) bool {
	for _, reason := range reasons {
		if reason == target {
			return true
		}
	}
	return false
}


func TestTushareDailyCanonicalizesEndDateBeforeFutureDateCheck(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location).Format("2006-01-02")
	tradeDate := strings.ReplaceAll(today, "-", "")
	var requestedEnd string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare daily request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.APIName != "daily" {
			t.Errorf("api_name=%q, want daily", request.APIName)
		}
		requestedEnd = request.Params["end_date"]
		writeTushareTestTable(w,
			[]string{"ts_code", "trade_date", "open", "high", "low", "close", "pct_chg", "vol", "amount"},
			[][]any{{"600519.SH", tradeDate, 100.0, 101.0, 99.0, 100.5, 0.5, 100.0, 1000.0}},
		)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	bars, err := client.FetchDailyBars(context.Background(), "600519.SH", today, today+" ", 10, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("today's end date with surrounding whitespace should be normalized, got %v", err)
	}
	if requestedEnd != tradeDate {
		t.Fatalf("request end_date=%q, want canonical %q", requestedEnd, tradeDate)
	}
	if len(bars) != 1 || bars[0].TradeDate != today {
		t.Fatalf("unexpected daily bars result: %+v", bars)
	}
}


func TestTushareIndexDailyBarsRejectsInt64VolumeOverflow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare index daily request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.APIName != "index_daily" {
			t.Errorf("api_name=%q, want index_daily", request.APIName)
		}
		writeTushareTestTable(w,
			[]string{"ts_code", "trade_date", "open", "high", "low", "close", "pct_chg", "vol", "amount"},
			[][]any{{"000300.SH", "20261020", 4000.0, 4050.0, 3950.0, 4020.0, 0.5, "9223372036854775808", 120000000.0}},
		)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchIndexDailyBars(context.Background(), "000300.SH", "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("FetchIndexDailyBars: %v", err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("expected one index daily bar, got %+v", got.Items)
	}
	bar := got.Items[0]
	if bar.Volume != nil || bar.VolumeUnit != "unknown" {
		t.Fatalf("out-of-range volume must be withheld, got volume=%v unit=%q", bar.Volume, bar.VolumeUnit)
	}
	if bar.QualityStatus != model.QualityDegraded || !containsTushareReason(bar.QualityReasons, "provider_index_volume_missing_or_invalid") {
		t.Fatalf("out-of-range volume must degrade the item and record its reason: %+v", bar)
	}
}
