package provider

import (
	"context"
	"encoding/json"
	"math"
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
	Fields  string            `json:"fields"`
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

func TestTushareDailyNormalizesAmountFromThousandCNYToCNY(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			APIName string `json:"api_name"`
			Fields  string `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "daily" {
			t.Errorf("api_name=%q, want daily", request.APIName)
		}
		writeTushareTestTable(w,
			[]string{"ts_code", "trade_date", "open", "high", "low", "close", "pct_chg", "vol", "amount"},
			[][]any{{"600519.SH", "20261026", 10.0, 11.0, 9.0, 10.5, 5.0, 100.0, 105.0}},
		)
	}))
	defer server.Close()

	p := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	bars, err := p.FetchDailyBars(context.Background(), "600519.SH", "2026-10-26", "2026-10-26", 10, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("FetchDailyBars: %v", err)
	}
	if len(bars) != 1 {
		t.Fatalf("got %d bars, want 1", len(bars))
	}
	bar := bars[0]
	if bar.Amount == nil || *bar.Amount != 105000 || bar.AmountUnit != "CNY" {
		t.Fatalf("normalized amount must be 105000 CNY, got amount=%v unit=%q", bar.Amount, bar.AmountUnit)
	}
	if bar.RawAmount == nil || *bar.RawAmount != 105 || bar.RawAmountUnit != "thousand_CNY" || bar.RawAmountField != "amount" {
		t.Fatalf("raw amount provenance was not preserved: raw_amount=%v unit=%q field=%q", bar.RawAmount, bar.RawAmountUnit, bar.RawAmountField)
	}
	if bar.Volume == nil || *bar.Volume != 10000 || bar.RawVolume == nil || *bar.RawVolume != 100 || bar.RawVolumeUnit != "hands" {
		t.Fatalf("Tushare volume normalization/provenance mismatch: volume=%v raw=%v raw_unit=%q", bar.Volume, bar.RawVolume, bar.RawVolumeUnit)
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

func TestTushareIndexConstituentsCountsUnsupportedRowsAtLatestReportDate(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location)
	date := today.Format("20060102")
	monthStart, monthEnd := tushareTestMonthWindow(today, 0)
	fields := []string{"index_code", "con_code", "trade_date", "weight"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "index_weight":
			if request.Params["index_code"] != "000300.SH" || request.Params["start_date"] != monthStart || request.Params["end_date"] != monthEnd {
				t.Errorf("unexpected index_weight request params: %#v", request.Params)
			}
			writeTushareTestTable(w, fields, [][]any{
				{"000300.SH", "510300.SH", date, 1.1},
				{"000300.SH", "600519.SH", date, 3.5},
				{"000300.SH", "600036.SH", date, 2.0},
			})
		case "stock_basic":
			writeTushareTestTable(w, []string{"ts_code", "name"}, [][]any{
				{"600519.SH", "贵州茅台"},
				{"600036.SH", "招商银行"},
			})
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
	if got.ProviderReportDate != today.Format("2006-01-02") || got.Total != 2 || got.ProviderReturned != 3 || got.ExcludedCount != 1 {
		t.Fatalf("source/accepted/excluded counts must include unsupported rows: %+v", got)
	}
	if got.QualityStatus != model.QualityDegraded || len(got.Items) != 2 {
		t.Fatalf("unsupported constituent rows must be excluded and disclosed: %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "provider_unsupported_constituent_rows_excluded" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing unsupported-row quality reason: %v", got.QualityReasons)
	}
}

func TestTushareIndexConstituentsDoesNotFallBackPastUnsupportedLatestReportDate(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location)
	currentStart, currentEnd := tushareTestMonthWindow(today, 0)
	previousStart, previousEnd := tushareTestMonthWindow(today, 1)
	previousMonthEnd := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, location).AddDate(0, 0, -1)
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
			start := request.Params["start_date"]
			weightStarts = append(weightStarts, start)
			if start == currentStart {
				rows := [][]any{{"000300.SH", "510300.SH", today.Format("20060102"), 1.0}}
				if today.Day() > 1 {
					rows = append(rows, []any{"000300.SH", "600519.SH", today.AddDate(0, 0, -1).Format("20060102"), 3.5})
				}
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, rows)
			} else if start == previousStart {
				if request.Params["end_date"] != previousEnd {
					t.Errorf("previous month end = %q, want %q", request.Params["end_date"], previousEnd)
				}
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{
					{"000300.SH", "600519.SH", previousMonthEnd.Format("20060102"), 3.0},
				})
			} else {
				t.Errorf("unexpected index_weight start date %q (current %q)", start, currentStart)
				writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{})
			}
		case "stock_basic":
			t.Errorf("unsupported-only latest snapshot should not require name enrichment")
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
		t.Fatalf("unsupported latest constituents should return an unknown snapshot for fallback: %v", err)
	}
	if len(weightStarts) != 1 || weightStarts[0] != currentStart {
		t.Fatalf("provider must not silently fall back to an older report date, requested windows=%v", weightStarts)
	}
	if got.ProviderReportDate != today.Format("2006-01-02") || got.Total != 0 || got.ProviderReturned != 1 || got.ExcludedCount != 1 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown snapshot with latest report date and excluded counts, got %+v", got)
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

func TestTushareStockBasicRejectsDuplicateRows(t *testing.T) {
	fields := []string{"ts_code", "name", "industry", "market", "exchange", "list_status", "list_date", "delist_date"}
	row := []any{"600519.SH", "贵州茅台", "白酒", "主板", "SSE", "L", "20010827", ""}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare stock_basic request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "stock_basic" || request.Params["ts_code"] != "600519.SH" {
			t.Errorf("unexpected Tushare stock_basic request: %+v", request)
		}
		writeTushareTestTable(w, fields, [][]any{row, row})
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	if _, err := provider.FetchStockBasic(context.Background(), "600519.SH"); err == nil || !strings.Contains(err.Error(), "重复返回证券") {
		t.Fatalf("duplicate stock_basic rows must be rejected, got %v", err)
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
			expectedFields := "ts_code,trade_date,pe,pe_ttm,pb,ps_ttm,turnover_rate,turnover_rate_f,volume_ratio,dv_ratio,dv_ttm"
			if request.Fields != expectedFields {
				t.Errorf("daily_basic request fields = %q, want exactly %q", request.Fields, expectedFields)
			}
			writeTushareTestTable(w, []string{
				"ts_code", "trade_date", "pe", "pe_ttm", "pb", "ps_ttm",
				"turnover_rate", "turnover_rate_f", "volume_ratio", "dv_ratio", "dv_ttm",
			}, [][]any{
				{"600519.SH", tradeDate, 22.5, 18.4, 6.1, 7.2, 1.25, 1.7, 0.95, 2.3, 2.6},
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
	if got.SourceVersion != "tushare-daily-basic-v2" || item.SourceVersion != "tushare-daily-basic-v2" {
		t.Fatalf("unexpected Tushare valuation field version: snapshot=%q item=%q", got.SourceVersion, item.SourceVersion)
	}
	if item.PEStatic == nil || *item.PEStatic != 22.5 {
		t.Fatalf("expected source-native static PE, got %+v", item.PEStatic)
	}
	if item.PEMRQ != nil {
		t.Fatalf("Tushare daily_basic pe must not be labelled pe_mrq, got %+v", item.PEMRQ)
	}
	if item.PETTM == nil || *item.PETTM != 18.4 {
		t.Fatalf("expected TTM PE to remain mapped, got %+v", item.PETTM)
	}
	if item.TurnoverRatePercent == nil || *item.TurnoverRatePercent != 1.25 ||
		item.FloatTurnoverRatePercent == nil || *item.FloatTurnoverRatePercent != 1.7 ||
		item.VolumeRatio == nil || *item.VolumeRatio != 0.95 {
		t.Fatalf("turnover and volume-ratio fields were not mapped: %+v", item)
	}
	if item.DividendYieldPercent == nil || *item.DividendYieldPercent != 2.3 ||
		item.DividendYieldTTMPercent == nil || *item.DividendYieldTTMPercent != 2.6 {
		t.Fatalf("dividend-yield fields were not mapped: %+v", item)
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

func TestTushareLimitPoolRejectsDuplicateSymbols(t *testing.T) {
	fields := strings.Split("trade_date,ts_code,industry,name,close,pct_chg,amount,limit_amount,float_mv,total_mv,turnover_ratio,fd_amount,first_time,last_time,open_times,up_stat,limit_times,limit", ",")
	row := make([]any, len(fields))
	for index, field := range fields {
		switch field {
		case "trade_date":
			row[index] = "20261027"
		case "ts_code":
			row[index] = "600519.SH"
		case "industry":
			row[index] = "白酒"
		case "name":
			row[index] = "贵州茅台"
		case "close":
			row[index] = 1500.0
		case "pct_chg":
			row[index] = 1.2
		case "limit":
			row[index] = "U"
		default:
			row[index] = nil
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare limit-pool request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "limit_list_d" {
			t.Errorf("api_name=%q, want limit_list_d", request.APIName)
		}
		writeTushareTestTable(w, fields, [][]any{row, row})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	if _, err := client.FetchLimitPool(context.Background(), "up", "2026-10-27", 30); err == nil || !strings.Contains(err.Error(), "重复证券") {
		t.Fatalf("duplicate limit-pool symbols must be rejected, got %v", err)
	}
}

func TestTushareLimitPoolEmptyResultDoesNotInventProviderReportDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare limit-pool request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.APIName != "limit_list_d" {
			t.Errorf("api_name=%q, want limit_list_d", request.APIName)
		}
		writeTushareTestTable(w,
			strings.Split("trade_date,ts_code,industry,name,close,pct_chg,amount,limit_amount,float_mv,total_mv,turnover_ratio,fd_amount,first_time,last_time,open_times,up_stat,limit_times,limit", ","),
			[][]any{},
		)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchLimitPool(context.Background(), "up", "2026-10-27", 30)
	if err != nil {
		t.Fatalf("FetchLimitPool: %v", err)
	}
	if got.QualityStatus != model.QualityUnknown || got.ProviderReportDate != "" {
		t.Fatalf("empty unknown pool must not claim a confirmed provider report date: %+v", got)
	}
}

func TestTushareLimitPoolMarksExcludedNonAShareRows(t *testing.T) {
	fields := strings.Split("trade_date,ts_code,industry,name,close,pct_chg,amount,limit_amount,float_mv,total_mv,turnover_ratio,fd_amount,first_time,last_time,open_times,up_stat,limit_times,limit", ",")
	makeRow := func(symbol, name string) []any {
		values := make([]any, len(fields))
		for index, field := range fields {
			switch field {
			case "trade_date":
				values[index] = "20261027"
			case "ts_code":
				values[index] = symbol
			case "industry":
				values[index] = "消费"
			case "name":
				values[index] = name
			case "close":
				values[index] = 1500.0
			case "pct_chg":
				values[index] = 1.2
			case "amount":
				values[index] = 100.0
			case "limit":
				values[index] = "U"
			default:
				values[index] = nil
			}
		}
		return values
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare limit-pool request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.APIName != "limit_list_d" || request.Params["trade_date"] != "20261027" || request.Params["limit_type"] != "U" {
			t.Errorf("unexpected Tushare limit-pool request: %+v", request)
		}
		writeTushareTestTable(w, fields, [][]any{
			makeRow("510300.SH", "沪深300ETF"),
			makeRow("600519.SH", "贵州茅台"),
		})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchLimitPool(context.Background(), "up", "2026-10-27", 30)
	if err != nil {
		t.Fatalf("FetchLimitPool: %v", err)
	}
	if got.Total != 1 || len(got.Items) != 1 || got.Items[0].Symbol != "600519.SH" {
		t.Fatalf("non-A-share rows must not enter the pool, got %+v", got)
	}
	if got.ProviderReturned != 2 || got.ExcludedNonAShareCount != 1 || got.TotalIsLowerBound {
		t.Fatalf("source coverage ledger mismatch: returned=%d excluded=%d lower_bound=%v", got.ProviderReturned, got.ExcludedNonAShareCount, got.TotalIsLowerBound)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("filtered provider rows must remain visible as degraded quality, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "provider_non_a_share_security_rows_excluded" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing non-A-share exclusion reason: %v", got.QualityReasons)
	}
}

func TestTushareDragonTigerMarksExcludedNonAShareRows(t *testing.T) {
	fields := strings.Split("trade_date,ts_code,name,close,pct_change,turnover_rate,amount,l_sell,l_buy,l_amount,net_amount,net_rate,amount_rate,reason", ",")
	makeRow := func(symbol, name string) []any {
		values := make([]any, len(fields))
		for index, field := range fields {
			switch field {
			case "trade_date":
				values[index] = "20261027"
			case "ts_code":
				values[index] = symbol
			case "name":
				values[index] = name
			case "close":
				values[index] = 1500.0
			case "pct_change":
				values[index] = 1.2
			case "reason":
				values[index] = "测试原因"
			default:
				values[index] = nil
			}
		}
		return values
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare dragon-tiger request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "top_list" || request.Params["trade_date"] != "20261027" {
			t.Errorf("unexpected Tushare dragon-tiger request: %+v", request)
		}
		writeTushareTestTable(w, fields, [][]any{
			makeRow("600519.SH", "贵州茅台"),
			makeRow("113001.SH", "测试可转债"),
		})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchDragonTiger(context.Background(), "2026-10-27", 1, 100)
	if err != nil {
		t.Fatalf("FetchDragonTiger: %v", err)
	}
	if got.Total != 2 || got.SourceReturned != 2 || got.Returned != 1 || got.ExcludedNonAShareCount != 1 || len(got.Items) != 1 {
		t.Fatalf("source/accepted/excluded row counts do not reconcile: %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "provider_non_a_share_security_rows_excluded" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing non-A-share exclusion reason: %v", got.QualityReasons)
	}
}

func TestTushareDragonTigerRejectsDuplicateSymbols(t *testing.T) {
	fields := strings.Split("trade_date,ts_code,name,close,pct_change,turnover_rate,amount,l_sell,l_buy,l_amount,net_amount,net_rate,amount_rate,reason", ",")
	row := []any{"20261027", "600519.SH", "贵州茅台", 1500.0, 1.2, 0.5, 100000.0, 1000.0, 900.0, 1900.0, 100.0, 5.0, 1.2, "测试原因"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare dragon-tiger request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "top_list" || request.Params["trade_date"] != "20261027" {
			t.Errorf("unexpected Tushare dragon-tiger request: %+v", request)
		}
		writeTushareTestTable(w, fields, [][]any{row, row})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	if _, err := client.FetchDragonTiger(context.Background(), "2026-10-27", 1, 100); err == nil || !strings.Contains(err.Error(), "重复证券") {
		t.Fatalf("duplicate source symbols must reject the Tushare page, got %v", err)
	}
}

func TestTushareDragonTigerEmptyResultDoesNotInventProviderReportDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare dragon-tiger request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.APIName != "top_list" {
			t.Errorf("api_name=%q, want top_list", request.APIName)
		}
		writeTushareTestTable(w,
			strings.Split("trade_date,ts_code,name,close,pct_change,turnover_rate,amount,l_sell,l_buy,l_amount,net_amount,net_rate,amount_rate,reason", ","),
			[][]any{},
		)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchDragonTiger(context.Background(), "2026-10-27", 1, 100)
	if err != nil {
		t.Fatalf("FetchDragonTiger: %v", err)
	}
	if got.QualityStatus != model.QualityUnknown || got.ProviderReportDate != "" {
		t.Fatalf("empty unknown report must not claim a confirmed provider report date: %+v", got)
	}
}

func TestTushareEventEndpointsRejectFutureTradeDatesBeforeRequest(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	for _, name := range []string{"limit-pool", "dragon-tiger"} {
		t.Run(name, func(t *testing.T) {
			called := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				http.Error(w, "should not be called for a future trade date", http.StatusBadRequest)
			}))
			defer server.Close()
			client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
			var callErr error
			if name == "limit-pool" {
				_, callErr = client.FetchLimitPool(context.Background(), "up", future, 30)
			} else {
				_, callErr = client.FetchDragonTiger(context.Background(), future, 1, 100)
			}
			if callErr == nil || called {
				t.Fatalf("future trade date should be rejected before the provider request: err=%v called=%v", callErr, called)
			}
		})
	}
}

func TestTushareIndexDailyBarsRejectsFutureEndDateBeforeRequest(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	future := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should not be called for future index date", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	_, err = client.FetchIndexDailyBars(context.Background(), "000300.SH", future, future)
	if err == nil || called {
		t.Fatalf("future index daily-bars range must be rejected before provider request: err=%v called=%v", err, called)
	}
}

func TestTushareFundamentalsMarksMissingIndicatorRevisionAtSnapshotLevel(t *testing.T) {
	const period = "20260630"
	const announcementDate = "20260830"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare financial request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		fields := strings.Split(request.Fields, ",")
		if request.APIName == "fina_indicator" {
			writeTushareTestTable(w, fields, [][]any{})
			return
		}
		values := make([]any, len(fields))
		for index, field := range fields {
			switch field {
			case "ts_code":
				values[index] = "600519.SH"
			case "ann_date", "f_ann_date":
				values[index] = announcementDate
			case "end_date":
				values[index] = period
			case "report_type":
				values[index] = request.Params["report_type"]
			default:
				values[index] = 1.0
			}
		}
		switch request.APIName {
		case "income", "balancesheet", "cashflow":
			writeTushareTestTable(w, fields, [][]any{values})
		default:
			t.Errorf("unexpected Tushare financial endpoint %q", request.APIName)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	snapshot, err := client.FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil {
		t.Fatalf("statement sections should remain available without a published indicator revision: %v", err)
	}
	if !snapshot.HasData || snapshot.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded statement snapshot, got %+v", snapshot)
	}
	found := false
	for _, reason := range snapshot.QualityReasons {
		if reason == "no_published_indicator_row_for_requested_period" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("parent snapshot must expose the missing indicator revision: %v", snapshot.QualityReasons)
	}
}

func TestTushareFundamentalsUsesStatementsWhenIndicatorEndpointFails(t *testing.T) {
	const period = "20260630"
	const announcementDate = "20260830"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare financial request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		if request.APIName == "fina_indicator" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":2002,"msg":"fina_indicator permission not available","data":null}`))
			return
		}
		fields := strings.Split(request.Fields, ",")
		values := make([]any, len(fields))
		for index, field := range fields {
			switch field {
			case "ts_code":
				values[index] = "600519.SH"
			case "ann_date", "f_ann_date":
				values[index] = announcementDate
			case "end_date":
				values[index] = period
			case "report_type":
				values[index] = request.Params["report_type"]
			default:
				values[index] = 1.0
			}
		}
		switch request.APIName {
		case "income", "balancesheet", "cashflow":
			writeTushareTestTable(w, fields, [][]any{values})
		default:
			t.Errorf("unexpected Tushare financial endpoint %q", request.APIName)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	snapshot, err := client.FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil {
		t.Fatalf("statement sections should remain available when fina_indicator fails: %v", err)
	}
	if !snapshot.HasData || snapshot.QualityStatus != model.QualityDegraded || len(snapshot.Sections) != 9 {
		t.Fatalf("expected a degraded snapshot assembled from available statements, got %+v", snapshot)
	}
	foundIndicatorError := false
	for _, reason := range snapshot.QualityReasons {
		if reason == "provider_fina_indicator_endpoint_unavailable" {
			foundIndicatorError = true
			break
		}
	}
	if !foundIndicatorError {
		t.Fatalf("parent snapshot must preserve the indicator endpoint failure reason: %v", snapshot.QualityReasons)
	}
	for _, category := range []string{"profitability", "operating", "growth", "solvency", "cash_flow", "dupont"} {
		found := false
		for _, section := range snapshot.Sections {
			if section.Category == category {
				found = !section.HasData && section.QualityStatus == model.QualityDegraded
				break
			}
		}
		if !found {
			t.Fatalf("indicator section %s must explicitly degrade when fina_indicator is unavailable", category)
		}
	}
	for _, category := range []string{"income_statement", "balance_sheet", "cash_flow_statement"} {
		found := false
		for _, section := range snapshot.Sections {
			if section.Category == category {
				found = section.HasData && section.QualityStatus == model.QualityValid
				break
			}
		}
		if !found {
			t.Fatalf("statement section %s should remain usable despite indicator endpoint failure", category)
		}
	}
}

func TestTushareStatementReportTypeForQuarterBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		apiName    string
		periodDate string
		want       string
	}{
		{name: "income first quarter", apiName: "income", periodDate: "2026-03-31", want: "1"},
		{name: "income second quarter", apiName: "income", periodDate: "2026-06-30", want: "2"},
		{name: "income third quarter", apiName: "income", periodDate: "2026-09-30", want: "2"},
		{name: "income annual report", apiName: "income", periodDate: "2026-12-31", want: "2"},
		{name: "cashflow first quarter", apiName: "cashflow", periodDate: "2026-03-31", want: "1"},
		{name: "cashflow second quarter", apiName: "cashflow", periodDate: "2026-06-30", want: "2"},
		{name: "cashflow third quarter", apiName: "cashflow", periodDate: "2026-09-30", want: "2"},
		{name: "cashflow annual report", apiName: "cashflow", periodDate: "2026-12-31", want: "2"},
		{name: "balance sheet first quarter", apiName: "balancesheet", periodDate: "2026-03-31", want: "1"},
		{name: "balance sheet second quarter", apiName: "balancesheet", periodDate: "2026-06-30", want: "1"},
		{name: "balance sheet third quarter", apiName: "balancesheet", periodDate: "2026-09-30", want: "1"},
		{name: "balance sheet annual report", apiName: "balancesheet", periodDate: "2026-12-31", want: "1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tushareStatementReportType(tt.apiName, tt.periodDate); got != tt.want {
				t.Fatalf("tushareStatementReportType(%q, %q) = %q, want %q", tt.apiName, tt.periodDate, got, tt.want)
			}
		})
	}
}

func TestTushareFundamentalsMapsThreeFinancialStatements(t *testing.T) {
	const period = "20260630"
	const announcementDate = "20260830"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare financial request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		expectedReportType := "2"
		if request.APIName == "balancesheet" {
			expectedReportType = "1"
		}
		if request.Params["report_type"] != expectedReportType {
			t.Errorf("%s report_type=%q, want %q", request.APIName, request.Params["report_type"], expectedReportType)
		}
		fields := strings.Split(request.Fields, ",")
		values := make([]any, len(fields))
		for i, field := range fields {
			switch field {
			case "ts_code":
				values[i] = "600519.SH"
			case "ann_date", "f_ann_date":
				values[i] = announcementDate
			case "end_date":
				values[i] = period
			case "report_type":
				values[i] = request.Params["report_type"]
			case "rd_exp", "inventories", "finan_exp":
				// 这些是按行业/报表结构可缺失的明细科目；缺失不能让完整关键汇总项整体降级。
				values[i] = nil
			default:
				values[i] = 1.0
			}
		}
		switch request.APIName {
		case "fina_indicator", "income", "balancesheet", "cashflow":
			writeTushareTestTable(w, fields, [][]any{values})
		default:
			t.Errorf("unexpected Tushare financial endpoint %q", request.APIName)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	snapshot, err := client.FetchFundamentals(nil, "600519.SH", 2026, 2)
	if err != nil {
		t.Fatalf("FetchFundamentals: %v", err)
	}
	if len(snapshot.Sections) != 9 {
		t.Fatalf("expected six indicator groups plus three statements, got %d sections", len(snapshot.Sections))
	}
	for _, category := range []string{"income_statement", "balance_sheet", "cash_flow_statement"} {
		found := false
		for _, section := range snapshot.Sections {
			if section.Category == category {
				found = section.HasData && section.QualityStatus == model.QualityValid && section.PublicationDate == "2026-08-30"
				if !found {
					t.Fatalf("statement section %s missing expected values or publication date: %+v", category, section)
				}
				break
			}
		}
		if !found {
			t.Fatalf("expected usable statement section %q", category)
		}
	}
	// 字段清单来自 Tushare 三张报表接口；fixture 对每个请求字段返回数值，验证扩展字段没有丢失。
	for _, expectedMetric := range []struct {
		category string
		key      string
	}{
		{category: "income_statement", key: "ebitda"},
		{category: "income_statement", key: "commission_income"},
		{category: "balance_sheet", key: "total_liabilities_and_equity"},
		{category: "balance_sheet", key: "right_of_use_assets"},
		{category: "cash_flow_statement", key: "free_cash_flow"},
		{category: "cash_flow_statement", key: "cash_received_tax_refunds"},
		{category: "cash_flow_statement", key: "other_financing_cash_paid"},
	} {
		found := false
		for _, section := range snapshot.Sections {
			if section.Category != expectedMetric.category {
				continue
			}
			for _, metric := range section.Metrics {
				if metric.Key == expectedMetric.key && metric.SourceField != "" {
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("expected extended statement metric %s.%s to be mapped", expectedMetric.category, expectedMetric.key)
		}
	}
}

func TestTushareIndexDailyBarsDegradesMissingProviderTimestamp(t *testing.T) {
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
			[][]any{{"000300.SH", "20261020", 4000.0, 4050.0, 3950.0, 4020.0, 0.5, 100000.0, 120000000.0}},
		)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchIndexDailyBars(context.Background(), "000300.SH", "2026-10-20", "2026-10-20")
	if err != nil {
		t.Fatalf("FetchIndexDailyBars: %v", err)
	}
	if got.QualityStatus != model.QualityDegraded || !containsTushareReason(got.QualityReasons, "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("index daily-bars snapshot without provider timestamp must be degraded: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].QualityStatus != model.QualityDegraded ||
		!containsTushareReason(got.Items[0].QualityReasons, "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("index daily bar without provider timestamp must preserve the quality reason: %+v", got.Items)
	}
	bar := got.Items[0]
	if bar.Amount == nil || *bar.Amount != 120000000000 || bar.AmountUnit != "CNY" {
		t.Fatalf("Tushare index_daily amount must convert thousand CNY to CNY, got amount=%v unit=%q", bar.Amount, bar.AmountUnit)
	}
	if bar.Volume == nil || *bar.Volume != 100000 || bar.VolumeUnit != "hands" {
		t.Fatalf("Tushare index_daily volume must preserve hand unit, got volume=%v unit=%q", bar.Volume, bar.VolumeUnit)
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

func TestTushareDailyDegradesMissingChangePercent(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location).Format("20060102")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTushareTestTable(w,
			[]string{"ts_code", "trade_date", "open", "high", "low", "close", "pct_chg", "vol", "amount"},
			[][]any{{"600519.SH", today, 100.0, 102.0, 99.0, 101.0, nil, 1200.0, 456.0}},
		)
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	bars, err := provider.FetchDailyBars(context.Background(), "600519.SH", time.Now().In(location).Format("2006-01-02"), time.Now().In(location).Format("2006-01-02"), 10, model.AdjustmentNone)
	if err != nil {
		t.Fatal(err)
	}
	if len(bars) != 1 {
		t.Fatalf("expected one daily bar, got %+v", bars)
	}
	if bars[0].ChangePercent != nil || bars[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("missing pct_chg must remain nil and degrade quality: %+v", bars[0])
	}
	found := false
	for _, reason := range bars[0].QualityReasons {
		if reason == "provider_change_percent_unavailable" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing pct_chg degradation reason: %+v", bars[0].QualityReasons)
	}
}

func TestTushareDailyRejectsFractionalVolumeInHands(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location)
	date := today.Format("20060102")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeTushareTestTable(w,
			[]string{"ts_code", "trade_date", "open", "high", "low", "close", "pct_chg", "vol", "amount"},
			[][]any{{"600519.SH", date, 100.0, 102.0, 99.0, 101.0, 0.0, 1.5, 456.0}},
		)
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	dateISO := today.Format("2006-01-02")
	_, err = provider.FetchDailyBars(context.Background(), "600519.SH", dateISO, dateISO, 10, model.AdjustmentNone)
	if err == nil || !strings.Contains(err.Error(), "fractional volume") {
		t.Fatalf("fractional volume in hands must not be rounded into shares, got %v", err)
	}
}

func TestTushareFundamentalsIgnoresFutureAnnouncementVersion(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	today := time.Now().In(location)
	futureAnnouncement := today.AddDate(0, 0, 5).Format("20060102")
	knownAnnouncement := today.AddDate(0, 0, -30).Format("20060102")
	periodEnd := "20260630"
	if knownAnnouncement < periodEnd {
		t.Skip("test date is before the selected report period")
	}

	fields := strings.Split("ts_code,ann_date,end_date,eps,dt_eps,roe,roe_waa,roe_dt,roa,netprofit_margin,grossprofit_margin,assets_turn,inv_turn,ar_turn,ca_turn,fa_turn,netprofit_yoy,or_yoy,assets_yoy,eqt_yoy,current_ratio,quick_ratio,cash_ratio,debt_to_assets,ocf_to_or,ocf_to_profit,ocf_to_debt,dupont_roe,dupont_asset_turn,dupont_pnitoni,dupont_tax_burden,dupont_intburden", ",")
	makeRow := func(announcementDate string, eps float64) []any {
		values := make([]any, len(fields))
		for i, field := range fields {
			switch field {
			case "ts_code":
				values[i] = "600519.SH"
			case "ann_date":
				values[i] = announcementDate
			case "end_date":
				values[i] = periodEnd
			case "eps":
				values[i] = eps
			default:
				values[i] = nil
			}
		}
		return values
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare fundamentals request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "fina_indicator" {
			// 三张财务报表在本用例中返回空表，重点验证公告日期门禁，不把其当成接口故障。
			if request.APIName == "income" || request.APIName == "balancesheet" || request.APIName == "cashflow" {
				writeTushareTestTable(w, strings.Split(request.Fields, ","), [][]any{})
				return
			}
			t.Errorf("unexpected Tushare fundamentals request: %+v", request)
			return
		}
		if request.Params["ts_code"] != "600519.SH" || request.Params["period"] != periodEnd {
			t.Errorf("unexpected Tushare fundamentals request: %+v", request)
		}
		// 将未来公告放在第一行，确保适配器不会直接选择它。
		writeTushareTestTable(w, fields, [][]any{
			makeRow(futureAnnouncement, 9.99),
			makeRow(knownAnnouncement, 1.23),
		})
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	snapshot, err := provider.FetchFundamentals(context.Background(), "600519.SH", 2026, 2)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.QualityStatus == model.QualityValid {
		t.Fatal("sparse indicator fixture should not be considered a complete valid snapshot")
	}
	for _, reason := range snapshot.QualityReasons {
		if reason == "provider_snapshot_timestamp_not_exposed" {
			t.Fatal("fundamentals must not inherit the index daily-bars timestamp quality reason")
		}
	}
	for _, section := range snapshot.Sections {
		for _, metric := range section.Metrics {
			if metric.Key == "eps" {
				if metric.Value != 1.23 || section.PublicationDate != time.Date(today.Year(), today.Month(), today.Day()-30, 0, 0, 0, 0, location).Format("2006-01-02") {
					t.Fatalf("future-announced version was selected: metric=%+v section=%+v", metric, section)
				}
				return
			}
		}
	}
	t.Fatal("expected a selected EPS metric from the already-published version")
}

func TestTushareFundamentalsRejectsMissingAnnouncementDate(t *testing.T) {
	fields := strings.Split("ts_code,ann_date,end_date,eps,dt_eps,roe,roe_waa,roe_dt,roa,netprofit_margin,grossprofit_margin,assets_turn,inv_turn,ar_turn,ca_turn,fa_turn,netprofit_yoy,or_yoy,assets_yoy,eqt_yoy,current_ratio,quick_ratio,cash_ratio,debt_to_assets,ocf_to_or,ocf_to_profit,ocf_to_debt,dupont_roe,dupont_asset_turn,dupont_pnitoni,dupont_tax_burden,dupont_intburden", ",")
	periodEnd := "20251231"
	values := make([]any, len(fields))
	for index, field := range fields {
		switch field {
		case "ts_code":
			values[index] = "600519.SH"
		case "ann_date":
			values[index] = ""
		case "end_date":
			values[index] = periodEnd
		case "eps":
			values[index] = 0.88
		default:
			values[index] = nil
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare fundamentals request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName == "income" || request.APIName == "balancesheet" || request.APIName == "cashflow" {
			// 此用例只验证指标修订缺少公告日期；独立报表端点返回空表，不额外制造测试错误。
			writeTushareTestTable(w, strings.Split(request.Fields, ","), [][]any{})
			return
		}
		if request.APIName != "fina_indicator" || request.Params["ts_code"] != "600519.SH" || request.Params["period"] != periodEnd {
			t.Errorf("unexpected Tushare fundamentals request: %+v", request)
		}
		writeTushareTestTable(w, fields, [][]any{values})
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	_, err := provider.FetchFundamentals(context.Background(), "600519.SH", 2025, 4)
	if err == nil || !strings.Contains(err.Error(), "未返回") {
		t.Fatalf("financial data without an announcement date must be excluded, got %v", err)
	}
}

func TestTushareDailyRejectsNonAShareBeforeRequest(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		http.Error(w, "unexpected request", http.StatusBadRequest)
	}))
	defer server.Close()

	provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	for _, symbol := range []string{"510300.SH", "159919.SZ", "000300.SH", "900901.SH"} {
		if _, err := provider.FetchDailyBars(context.Background(), symbol, "2026-10-01", "2026-10-26", 10, model.AdjustmentNone); err == nil {
			t.Errorf("non-common-A-share symbol %s must be rejected before querying Tushare daily", symbol)
		}
	}
	if requests != 0 {
		t.Fatalf("unsupported security types must be rejected before upstream access; got %d request(s)", requests)
	}
}

func TestTushareAnnouncementsRejectsNonStockSecurities(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		http.Error(w, "should reject non-stock symbols before request", http.StatusBadRequest)
	}))
	defer server.Close()
	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	for _, symbol := range []string{"510300.SH", "000300.SH", "113001.SH", "900901.SH", "159919.SZ"} {
		if _, err := client.FetchStockAnnouncements(context.Background(), symbol, "2026-10-20", "2026-10-26", 1, 20); err == nil {
			t.Errorf("non-common-A-share symbol %s must be rejected", symbol)
		}
	}
	if called {
		t.Fatal("non-stock announcement requests must be rejected before contacting Tushare")
	}
}

func TestTushareStockAnnouncementsMarksDuplicateRows(t *testing.T) {
	fields := []string{"ann_date", "ts_code", "name", "title", "url", "rec_time"}
	row := []any{"20261023", "600519.SH", "贵州茅台", "测试公告", "https://example.test/notice/1", "2026-10-23 18:30:00"}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare announcement request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "anns_d" {
			t.Errorf("api_name=%q, want anns_d", request.APIName)
		}
		writeTushareTestTable(w, fields, [][]any{row, row})
	}))
	defer server.Close()

	p := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := p.FetchStockAnnouncements(context.Background(), "600519.SH", "2026-10-20", "2026-10-26", 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 1 || got.Returned != 1 || len(got.Items) != 1 {
		t.Fatalf("duplicate source rows should produce one unique announcement, got %+v", got)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("deduplicated source rows must be disclosed as degraded quality, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "provider_duplicate_announcement_rows_deduplicated" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("missing duplicate-row quality reason: %v", got.QualityReasons)
	}
}

func TestTushareDailyAppliesAdjustmentFactors(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	end := time.Now().In(location).AddDate(0, 0, -1)
	start := end.AddDate(0, 0, -2)
	startDate, endDate := start.Format("2006-01-02"), end.Format("2006-01-02")
	startCompact, endCompact := start.Format("20060102"), end.Format("20060102")

	for _, tc := range []struct {
		name           string
		basis          model.AdjustmentBasis
		wantMethod     string
		wantFirstOpen  float64
		wantFirstHigh  float64
		wantFirstLow   float64
		wantFirstClose float64
		wantLastOpen   float64
		wantLastHigh   float64
		wantLastLow    float64
		wantLastClose  float64
		wantReference  string
	}{
		{
			name: "forward adjusted", basis: model.AdjustmentForward,
			wantMethod: "tushare_adj_factor_qfq_v1", wantFirstOpen: 5, wantFirstHigh: 6, wantFirstLow: 4.5, wantFirstClose: 5.5,
			wantLastOpen: 20, wantLastHigh: 22, wantLastLow: 18, wantLastClose: 21, wantReference: endDate,
		},
		{
			name: "backward adjusted", basis: model.AdjustmentBackward,
			wantMethod: "tushare_adj_factor_hfq_v1", wantFirstOpen: 20, wantFirstHigh: 24, wantFirstLow: 18, wantFirstClose: 22,
			wantLastOpen: 80, wantLastHigh: 88, wantLastLow: 72, wantLastClose: 84,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dailyRequests, factorRequests := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					APIName string            `json:"api_name"`
					Params  map[string]string `json:"params"`
					Fields  string            `json:"fields"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode Tushare request: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				switch request.APIName {
				case "daily":
					dailyRequests++
					if request.Params["ts_code"] != "600519.SH" || request.Params["start_date"] != startCompact || request.Params["end_date"] != endCompact {
						t.Errorf("unexpected daily request params: %#v", request.Params)
					}
					writeTushareTestTable(w,
						[]string{"ts_code", "trade_date", "open", "high", "low", "close", "pct_chg", "vol", "amount"},
						[][]any{
							{"600519.SH", startCompact, 10.0, 12.0, 9.0, 11.0, 10.0, 100.0, 100.0},
							{"600519.SH", endCompact, 20.0, 22.0, 18.0, 21.0, 90.0, 200.0, 200.0},
						},
					)
				case "adj_factor":
					factorRequests++
					if request.Params["ts_code"] != "600519.SH" || request.Params["start_date"] != startCompact || request.Params["end_date"] != endCompact {
						t.Errorf("unexpected adj_factor request params: %#v", request.Params)
					}
					// 返回顺序故意倒置，换算逻辑不能依赖供应商行顺序。
					writeTushareTestTable(w, []string{"ts_code", "trade_date", "adj_factor"}, [][]any{
						{"600519.SH", endCompact, 4.0},
						{"600519.SH", startCompact, 2.0},
					})
				default:
					t.Errorf("unexpected Tushare API %q", request.APIName)
					http.Error(w, "unexpected API", http.StatusBadRequest)
				}
			}))
			defer server.Close()

			provider := NewTushareConfigured(2*time.Second, server.URL, "test-token")
			bars, err := provider.FetchDailyBars(context.Background(), "600519.SH", startDate, endDate, 10, tc.basis)
			if err != nil {
				t.Fatalf("FetchDailyBars: %v", err)
			}
			if dailyRequests != 1 || factorRequests != 1 {
				t.Fatalf("requests daily=%d adj_factor=%d, want one each", dailyRequests, factorRequests)
			}
			if len(bars) != 2 {
				t.Fatalf("got %d bars, want 2", len(bars))
			}
			first, last := bars[0], bars[1]
			if first.TradeDate != startDate || last.TradeDate != endDate {
				t.Fatalf("unexpected sorted dates: %s, %s", first.TradeDate, last.TradeDate)
			}
			if first.Open != tc.wantFirstOpen || first.High != tc.wantFirstHigh || first.Low != tc.wantFirstLow || first.Close != tc.wantFirstClose ||
				last.Open != tc.wantLastOpen || last.High != tc.wantLastHigh || last.Low != tc.wantLastLow || last.Close != tc.wantLastClose {
				t.Fatalf("adjusted OHLC mismatch: first=(%.4f, %.4f, %.4f, %.4f) last=(%.4f, %.4f, %.4f, %.4f)",
					first.Open, first.High, first.Low, first.Close, last.Open, last.High, last.Low, last.Close)
			}
			for _, bar := range bars {
				if bar.AdjustmentBasis != tc.basis || bar.AdjustmentMethod != tc.wantMethod {
					t.Fatalf("adjustment metadata mismatch: %+v", bar)
				}
				if bar.Volume == nil || bar.Amount == nil || bar.RawVolume == nil || bar.RawAmount == nil {
					t.Fatalf("volume/amount/raw provenance must remain intact: %+v", bar)
				}
				if bar.TradeDate == startDate && (*bar.Amount != 100000 || *bar.Volume != 10000 || *bar.RawAmount != 100 || *bar.RawVolume != 100) {
					t.Fatalf("first bar volume/amount must not be adjusted: %+v", bar)
				}
				if bar.TradeDate == endDate && (*bar.Amount != 200000 || *bar.Volume != 20000 || *bar.RawAmount != 200 || *bar.RawVolume != 200) {
					t.Fatalf("last bar volume/amount must not be adjusted: %+v", bar)
				}
			}
			if first.AdjustmentRefDate != tc.wantReference || last.AdjustmentRefDate != tc.wantReference {
				t.Fatalf("unexpected adjustment reference dates: %q, %q; want %q", first.AdjustmentRefDate, last.AdjustmentRefDate, tc.wantReference)
			}
		})
	}
}

func TestApplyTushareAdjustmentRejectsMissingOrInvalidFactors(t *testing.T) {
	bar := model.DailyBar{TradeDate: "2026-10-26", Open: 10, High: 11, Low: 9, Close: 10}
	if _, err := applyTushareAdjustment([]model.DailyBar{bar}, map[string]float64{}, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "requires daily bars and adjustment factors") {
		t.Fatalf("empty factors should be rejected explicitly, got %v", err)
	}
	if _, err := applyTushareAdjustment([]model.DailyBar{bar}, map[string]float64{"2026-10-26": 2}, model.AdjustmentForward); err != nil {
		t.Fatalf("valid factor should succeed: %v", err)
	}
	if _, err := applyTushareAdjustment([]model.DailyBar{bar}, map[string]float64{"2026-10-27": 2}, model.AdjustmentForward); err == nil || !strings.Contains(err.Error(), "missing for daily bar date") {
		t.Fatalf("missing daily factor should be rejected, got %v", err)
	}
	if _, err := applyTushareAdjustment([]model.DailyBar{bar}, map[string]float64{"2026-10-26": math.NaN()}, model.AdjustmentForward); err == nil {
		t.Fatal("non-finite reference factor should be rejected")
	}
}

func TestTushareIndexConstituentsAsOfUsesReportOnRequestedDateAndAllListingStatuses(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	asOfTime := currentMonthStart.AddDate(0, 0, -1)
	asOfDate := asOfTime.Format("2006-01-02")
	wantStart := time.Date(asOfTime.Year(), asOfTime.Month(), 1, 0, 0, 0, 0, location).Format("20060102")
	wantEnd := asOfTime.Format("20060102")
	var gotStockBasicParams map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "index_weight":
			if request.Params["index_code"] != "000300.SH" || request.Params["start_date"] != wantStart || request.Params["end_date"] != wantEnd {
				t.Errorf("unexpected historical index_weight window: %#v", request.Params)
			}
			writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{
				{"000300.SH", "600519.SH", asOfTime.AddDate(0, 0, -1).Format("20060102"), 2.5},
				{"000300.SH", "600036.SH", asOfTime.Format("20060102"), 3.0},
			})
		case "stock_basic":
			gotStockBasicParams = request.Params
			writeTushareTestTable(w, []string{"ts_code", "name"}, [][]any{{"600036.SH", "招商银行"}})
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected api", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	p := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := p.FetchIndexConstituentsAsOf(context.Background(), "000300.SH", asOfDate)
	if err != nil {
		t.Fatal(err)
	}
	if got.AsOfDate != asOfDate || got.ProviderReportDate != asOfDate || got.Total != 1 ||
		got.ProviderReturned != 1 || got.Items[0].Symbol != "600036.SH" {
		t.Fatalf("unexpected as-of constituent snapshot: %+v", got)
	}
	if _, hasStatusFilter := gotStockBasicParams["list_status"]; hasStatusFilter || len(gotStockBasicParams) != 0 {
		t.Fatalf("historical name enrichment must include delisted securities, params=%#v", gotStockBasicParams)
	}
	foundBoundaryReason := false
	for _, reason := range got.QualityReasons {
		if reason == "provider_as_of_uses_latest_index_weight_report_not_effective_membership_interval" {
			foundBoundaryReason = true
		}
	}
	if !foundBoundaryReason {
		t.Fatalf("historical snapshot must disclose report-date semantics: %v", got.QualityReasons)
	}
}

func TestTushareIndexConstituentsAsOfRejectsRowsAfterRequestedDate(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().In(location)
	currentMonthStart := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, location)
	asOfTime := currentMonthStart.AddDate(0, 0, -1)
	asOfDate := asOfTime.Format("2006-01-02")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "index_weight" {
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected api", http.StatusBadRequest)
			return
		}
		writeTushareTestTable(w, []string{"index_code", "con_code", "trade_date", "weight"}, [][]any{
			{"000300.SH", "600519.SH", asOfTime.AddDate(0, 0, 1).Format("20060102"), 2.5},
		})
	}))
	defer server.Close()

	p := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	if _, err := p.FetchIndexConstituentsAsOf(context.Background(), "000300.SH", asOfDate); err == nil || !strings.Contains(err.Error(), "日期无效或超出查询月份") {
		t.Fatalf("provider over-returning a report after as_of must be rejected, got %v", err)
	}
}

func TestTushareLimitPoolDefaultsToServiceLimit(t *testing.T) {
	fields := strings.Split("trade_date,ts_code,industry,name,close,pct_chg,amount,limit_amount,float_mv,total_mv,turnover_ratio,fd_amount,first_time,last_time,open_times,up_stat,limit_times,limit", ",")
	rows := make([][]any, 35)
	for index := range rows {
		values := make([]any, len(fields))
		for column, field := range fields {
			switch field {
			case "trade_date":
				values[column] = "20261027"
			case "ts_code":
				values[column] = fmt.Sprintf("%06d.SH", 600000+index)
			case "industry":
				values[column] = "银行"
			case "name":
				values[column] = fmt.Sprintf("测试股票%d", index+1)
			case "close":
				values[column] = 10.0
			case "pct_chg":
				values[column] = 1.2
			case "limit":
				values[column] = "U"
			default:
				values[column] = nil
			}
		}
		rows[index] = values
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare limit-pool request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "limit_list_d" {
			t.Errorf("api_name=%q, want limit_list_d", request.APIName)
		}
		writeTushareTestTable(w, fields, rows)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchLimitPool(context.Background(), "up", "2026-10-27", 0)
	if err != nil {
		t.Fatalf("FetchLimitPool with default limit: %v", err)
	}
	if got.Total != len(rows) || len(got.Items) != len(rows) {
		t.Fatalf("default Tushare limit should match the service's 100-row default, got total=%d items=%d want=%d", got.Total, len(got.Items), len(rows))
	}
	if got.ProviderReturned != len(rows) || got.TotalIsLowerBound || got.ExcludedNonAShareCount != 0 {
		t.Fatalf("unexpected source coverage ledger: returned=%d excluded=%d lower_bound=%v", got.ProviderReturned, got.ExcludedNonAShareCount, got.TotalIsLowerBound)
	}
}

func TestTushareLimitPoolMarks2500RowBoundaryAsLowerBound(t *testing.T) {
	fields := strings.Split("trade_date,ts_code,industry,name,close,pct_chg,amount,limit_amount,float_mv,total_mv,turnover_ratio,fd_amount,first_time,last_time,open_times,up_stat,limit_times,limit", ",")
	prefixes := []string{"600", "601", "603"}
	rows := make([][]any, 2500)
	for index := range rows {
		prefixIndex := index / 1000
		suffix := index % 1000
		prefix := prefixes[prefixIndex]
		values := make([]any, len(fields))
		for column, field := range fields {
			switch field {
			case "trade_date":
				values[column] = "20261027"
			case "ts_code":
				values[column] = fmt.Sprintf("%s%03d.SH", prefix, suffix)
			case "industry":
				values[column] = "测试行业"
			case "name":
				values[column] = fmt.Sprintf("测试股票%d", index+1)
			case "close":
				values[column] = 10.0
			case "pct_chg":
				values[column] = 1.2
			case "limit":
				values[column] = "U"
			default:
				values[column] = nil
			}
		}
		rows[index] = values
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare limit-pool request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "limit_list_d" {
			t.Errorf("api_name=%q, want limit_list_d", request.APIName)
		}
		writeTushareTestTable(w, fields, rows)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchLimitPool(context.Background(), "up", "2026-10-27", 100)
	if err != nil {
		t.Fatalf("FetchLimitPool: %v", err)
	}
	if got.Total != 2500 || len(got.Items) != 100 || got.ProviderReturned != 2500 {
		t.Fatalf("unexpected source counts at cap boundary: total=%d items=%d returned=%d", got.Total, len(got.Items), got.ProviderReturned)
	}
	if !got.TotalIsLowerBound {
		t.Fatal("a 2500-row response must not be presented as an authoritative full total")
	}
	for _, reason := range []string{"provider_response_may_be_capped_at_2500_rows", "provider_limit_pool_total_is_lower_bound"} {
		if !containsTushareReason(got.QualityReasons, reason) {
			t.Errorf("missing lower-bound quality reason %q: %v", reason, got.QualityReasons)
		}
	}
}

func TestTushareDragonTigerMarks10000RowBoundaryAsLowerBound(t *testing.T) {
	fields := strings.Split("trade_date,ts_code,name,close,pct_change,turnover_rate,amount,l_sell,l_buy,l_amount,net_amount,net_rate,amount_rate,reason", ",")
	prefixes := []struct {
		prefix   string
		exchange string
	}{
		{prefix: "600", exchange: "SH"},
		{prefix: "601", exchange: "SH"},
		{prefix: "603", exchange: "SH"},
		{prefix: "605", exchange: "SH"},
		{prefix: "688", exchange: "SH"},
		{prefix: "689", exchange: "SH"},
		{prefix: "000", exchange: "SZ"},
		{prefix: "001", exchange: "SZ"},
		{prefix: "002", exchange: "SZ"},
		{prefix: "003", exchange: "SZ"},
	}
	rows := make([][]any, 10000)
	for index := range rows {
		group := index / 1000
		suffix := index % 1000
		values := make([]any, len(fields))
		for column, field := range fields {
			switch field {
			case "trade_date":
				values[column] = "20261027"
			case "ts_code":
				values[column] = fmt.Sprintf("%s%03d.%s", prefixes[group].prefix, suffix, prefixes[group].exchange)
			case "name":
				values[column] = fmt.Sprintf("测试股票%d", index+1)
			case "close":
				values[column] = 10.0
			case "pct_change":
				values[column] = 1.2
			case "reason":
				values[column] = "测试原因"
			default:
				values[column] = nil
			}
		}
		rows[index] = values
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare dragon-tiger request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if request.APIName != "top_list" {
			t.Errorf("api_name=%q, want top_list", request.APIName)
		}
		writeTushareTestTable(w, fields, rows)
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchDragonTiger(context.Background(), "2026-10-27", 1, 100)
	if err != nil {
		t.Fatalf("FetchDragonTiger: %v", err)
	}
	if got.Total != 10000 || got.Pages != 100 || got.SourceReturned != 100 || len(got.Items) != 100 {
		t.Fatalf("unexpected dragon-tiger pagination at source cap: total=%d pages=%d source_returned=%d items=%d", got.Total, got.Pages, got.SourceReturned, len(got.Items))
	}
	if !got.TotalIsLowerBound {
		t.Fatal("a 10000-row response must not be presented as an authoritative full total")
	}
	for _, reason := range []string{"provider_response_may_be_capped_at_10000_rows", "provider_dragon_tiger_total_is_lower_bound"} {
		if !containsTushareReason(got.QualityReasons, reason) {
			t.Errorf("missing lower-bound quality reason %q: %v", reason, got.QualityReasons)
		}
	}
}

func TestTushareIndexConstituentsMarks5000RowBoundaryAsLowerBound(t *testing.T) {
	indexFields := []string{"index_code", "con_code", "trade_date", "weight"}
	basicFields := []string{"ts_code", "name"}
	prefixes := []struct {
		prefix   string
		exchange string
	}{
		{prefix: "600", exchange: "SH"},
		{prefix: "601", exchange: "SH"},
		{prefix: "603", exchange: "SH"},
		{prefix: "605", exchange: "SH"},
		{prefix: "688", exchange: "SH"},
		{prefix: "689", exchange: "SH"},
		{prefix: "000", exchange: "SZ"},
		{prefix: "001", exchange: "SZ"},
		{prefix: "002", exchange: "SZ"},
		{prefix: "003", exchange: "SZ"},
	}
	weightRows := make([][]any, 5000)
	basicRows := make([][]any, 5000)
	for index := range weightRows {
		prefix := prefixes[index/500]
		code := fmt.Sprintf("%s%03d.%s", prefix.prefix, index%500, prefix.exchange)
		weightRows[index] = []any{"000300.SH", code, "20261015", 0.02}
		basicRows[index] = []any{code, fmt.Sprintf("测试成分股%d", index+1)}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request tushareIndexWeightRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode Tushare index-constituent request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		switch request.APIName {
		case "index_weight":
			writeTushareTestTable(w, indexFields, weightRows)
		case "stock_basic":
			writeTushareTestTable(w, basicFields, basicRows)
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected API", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "test-token")
	got, err := client.FetchIndexConstituentsAsOf(context.Background(), "000300.SH", "2026-10-20")
	if err != nil {
		t.Fatalf("FetchIndexConstituentsAsOf: %v", err)
	}
	if got.ProviderReportDate != "2026-10-15" || got.ProviderReturned != 5000 ||
		got.Total != 5000 || len(got.Items) != 5000 {
		t.Fatalf("unexpected index-weight coverage at source cap: report=%s returned=%d total=%d items=%d",
			got.ProviderReportDate, got.ProviderReturned, got.Total, len(got.Items))
	}
	if !got.TotalIsLowerBound {
		t.Fatal("a 5000-row monthly index_weight response must not be treated as authoritative full membership")
	}
	if !containsTushareReason(got.QualityReasons, "provider_monthly_index_weight_result_is_large_and_may_be_truncated") {
		t.Fatalf("missing source-cap reason: %v", got.QualityReasons)
	}
}
