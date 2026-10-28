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
		{name: "income annual quarter", apiName: "income", periodDate: "2026-12-31", want: "2"},
		{name: "cashflow first quarter", apiName: "cashflow", periodDate: "2026-03-31", want: "1"},
		{name: "cashflow second quarter", apiName: "cashflow", periodDate: "2026-06-30", want: "2"},
		{name: "balance sheet first quarter", apiName: "balancesheet", periodDate: "2026-03-31", want: "1"},
		{name: "balance sheet annual", apiName: "balancesheet", periodDate: "2026-12-31", want: "1"},
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
			{category: "balance_sheet", key: "total_liabilities_and_equity"},
			{category: "cash_flow_statement", key: "free_cash_flow"},
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
