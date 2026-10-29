package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestTushareFetchSecurityTradingStatusUsesEffectiveNameAndSuspensionRecords(t *testing.T) {
	var mu sync.Mutex
	calls := map[string]int{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			APIName string            `json:"api_name"`
			Params  map[string]string `json:"params"`
			Fields  string            `json:"fields"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		mu.Lock()
		calls[request.APIName]++
		mu.Unlock()

		var fields []string
		var items [][]any
		switch request.APIName {
		case "stock_basic":
			fields = []string{"ts_code", "name", "industry", "market", "exchange", "list_status", "list_date", "delist_date"}
			items = [][]any{{"600519.SH", "贵州茅台", "白酒", "主板", "SSE", "L", "20261020", nil}}
		case "stock_st":
			fields = []string{"ts_code", "name", "trade_date", "type", "type_name"}
			items = [][]any{{"600519.SH", "*ST贵州", "20261022", "ST", "风险警示板"}}
		case "namechange":
			fields = []string{"ts_code", "name", "start_date", "end_date", "ann_date", "change_reason"}
			items = [][]any{
				{"600519.SH", "贵州茅台", "20010827", "20261019", "20010827", "改名"},
				{"600519.SH", "*ST贵州", "20261020", nil, "20261019", "实施风险警示"},
			}
		case "trade_cal":
			fields = []string{"exchange", "cal_date", "is_open"}
			switch {
			case request.Params["start_date"] == "20261022" && request.Params["end_date"] == "20261022":
				items = [][]any{{request.Params["exchange"], "20261022", "1"}}
			case request.Params["start_date"] == "20261020" && request.Params["end_date"] == "20261022":
				items = [][]any{
					{request.Params["exchange"], "20261020", "1"},
					{request.Params["exchange"], "20261021", "1"},
					{request.Params["exchange"], "20261022", "1"},
				}
			default:
				t.Errorf("unexpected trade_cal params: %+v", request.Params)
				items = [][]any{}
			}
		case "suspend_d":
			fields = []string{"ts_code", "trade_date", "suspend_timing", "suspend_type"}
			items = [][]any{{"600519.SH", "20261022", "09:30-10:00", "S"}}
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected API", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "",
			"data": map[string]any{"fields": fields, "items": items},
		})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "fixture-token")
	got, err := client.FetchSecurityTradingStatus(context.Background(), "600519.SH", "2026-10-22")
	if err != nil {
		t.Fatalf("FetchSecurityTradingStatus: %v", err)
	}
	if got.TradeDate != "2026-10-22" || !got.IsTradingDay || !got.TradingDayKnown {
		t.Fatalf("unexpected exchange-calendar status: %+v", got)
	}
	if got.SecurityName != "*ST贵州" || got.RiskWarningStatus != model.RiskWarningApplied {
		t.Fatalf("historical effective name was not applied: %+v", got)
	}
	if got.NameEffectiveFrom != "2026-10-20" || got.NameEffectiveTo != "" {
		t.Fatalf("unexpected effective name interval: %+v", got)
	}
	if !got.ListingStatusKnown || got.ListingTradingDayNumber != 3 || got.ListingDate != "2026-10-20" {
		t.Fatalf("listing-day count should include IPO date and requested open date: %+v", got)
	}
	if got.SuspensionStatus != model.SuspensionStatusIntraday || got.SuspensionTiming != "09:30-10:00" {
		t.Fatalf("intraday suspension timing was lost: %+v", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["stock_basic"] != 1 || calls["namechange"] != 1 || calls["suspend_d"] != 1 || calls["trade_cal"] != 2 || calls["stock_st"] != 1 {
		t.Fatalf("unexpected Tushare capability calls: %+v", calls)
	}
}

func TestRiskWarningStatusFromHistoricalNameDoesNotGuessUnknownNames(t *testing.T) {
	cases := []struct {
		name string
		want model.RiskWarningStatus
	}{
		{name: "*ST示例", want: model.RiskWarningApplied},
		{name: "ST示例", want: model.RiskWarningApplied},
		{name: "S*ST示例", want: model.RiskWarningApplied},
		{name: "SST示例", want: model.RiskWarningApplied},
		{name: "示例股份", want: model.RiskWarningNormal},
		{name: "", want: model.RiskWarningUnknown},
		{name: "PT示例", want: model.RiskWarningUnknown},
	}
	for _, tc := range cases {
		if got := riskWarningStatusFromHistoricalName(tc.name); got != tc.want {
			t.Errorf("riskWarningStatusFromHistoricalName(%q)=%q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDelistingStatusFromHistoricalName(t *testing.T) {
	cases := []struct {
		name string
		want model.DelistingStatus
	}{
		{name: "贵州茅台", want: model.DelistingStatusUnknown},
		{name: "*ST贵州", want: model.DelistingStatusUnknown},
		{name: "退市整理某股", want: model.DelistingStatusUnknown},
		{name: "退市某股", want: model.DelistingStatusUnknown},
		{name: "某股摘牌", want: model.DelistingStatusUnknown},
		{name: "", want: model.DelistingStatusUnknown},
		{name: "PT某股", want: model.DelistingStatusUnknown},
	}
	for _, tc := range cases {
		if got := delistingStatusFromHistoricalName(tc.name); got != tc.want {
			t.Errorf("delistingStatusFromHistoricalName(%q)=%q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTushareFetchSecurityTradingStatusUsesSSECalendarForBeijingExchange(t *testing.T) {
	tradeCalendarCalls := 0
	tradeCalendarExchange := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			APIName string            `json:"api_name"`
			Params  map[string]string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}

		var fields []string
		var items [][]any
		switch request.APIName {
		case "stock_basic":
			fields = []string{"ts_code", "name", "industry", "market", "exchange", "list_status", "list_date", "delist_date"}
			items = [][]any{{"920001.BJ", "北证示例", "综合", "北交所", "BSE", "L", "20200101", nil}}
		case "stock_st":
			fields = []string{"ts_code", "name", "trade_date", "type", "type_name"}
			items = [][]any{}
		case "namechange":
			fields = []string{"ts_code", "name", "start_date", "end_date", "ann_date", "change_reason"}
			items = [][]any{{"920001.BJ", "北证示例", "20200101", nil, "20200101", "上市"}}
		case "trade_cal":
			tradeCalendarCalls++
			tradeCalendarExchange = request.Params["exchange"]
			fields = []string{"exchange", "cal_date", "is_open"}
			if request.Params["start_date"] != "20261022" || request.Params["end_date"] != "20261022" {
				t.Errorf("unexpected trade_cal date range: %+v", request.Params)
			}
			items = [][]any{{"SSE", "20261022", "1"}}
		case "suspend_d":
			fields = []string{"ts_code", "trade_date", "suspend_timing", "suspend_type"}
			items = [][]any{}
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected API", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "",
			"data": map[string]any{"fields": fields, "items": items},
		})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "fixture-token")
	got, err := client.FetchSecurityTradingStatus(context.Background(), "920001.BJ", "2026-10-22")
	if err != nil {
		t.Fatalf("FetchSecurityTradingStatus: %v", err)
	}
	if tradeCalendarExchange != "SSE" || tradeCalendarCalls != 1 {
		t.Fatalf("Beijing Exchange must use the documented SSE calendar proxy only for market-day status, exchange=%q calls=%d", tradeCalendarExchange, tradeCalendarCalls)
	}
	if !got.TradingDayKnown || !got.IsTradingDay {
		t.Fatalf("market trading-day status should be backed by the documented calendar: %+v", got)
	}
	if got.ListingStatusKnown || got.ListingTradingDayNumber != 0 {
		t.Fatalf("Beijing Exchange listing-trading-day number must remain unknown rather than estimated: %+v", got)
	}
	foundReason := false
	for _, reason := range got.QualityReasons {
		if reason == "listing_trading_day_number_not_verified_for_beijing_exchange" {
			foundReason = true
			break
		}
	}
	if !foundReason {
		t.Fatalf("expected explicit reason for unknown Beijing Exchange listing-day number: %+v", got.QualityReasons)
	}
}

func TestTushareFetchSecurityTradingStatusFindsDelistedSecurityBasicRow(t *testing.T) {
	var stockBasicStatuses []string
	var tradeCalendarExchange string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			APIName string            `json:"api_name"`
			Params  map[string]string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var fields []string
		var items [][]any
		switch request.APIName {
		case "stock_basic":
			status := request.Params["list_status"]
			stockBasicStatuses = append(stockBasicStatuses, status)
			fields = []string{"ts_code", "list_status", "list_date", "market"}
			switch status {
			case "L":
				items = [][]any{}
			case "D":
				items = [][]any{{"920001.BJ", "D", "20200101", "北交所"}}
			case "P":
				items = [][]any{}
			default:
				t.Errorf("unexpected stock_basic list_status %q", status)
			}
		case "stock_st":
			fields = []string{"ts_code", "name", "trade_date", "type", "type_name"}
			items = [][]any{}
		case "namechange":
			fields = []string{"ts_code", "name", "start_date", "end_date", "ann_date", "change_reason"}
			items = [][]any{
				{"920001.BJ", "北证示例", "20200101", "20261019", "20200101", "上市"},
				{"920001.BJ", "退市整理示例", "20261020", nil, "20261019", "退市整理"},
			}
		case "trade_cal":
			tradeCalendarExchange = request.Params["exchange"]
			fields = []string{"exchange", "cal_date", "is_open"}
			items = [][]any{{"SSE", "20261022", "1"}}
		case "suspend_d":
			fields = []string{"ts_code", "trade_date", "suspend_timing", "suspend_type"}
			items = [][]any{}
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected API", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg":  "",
			"data": map[string]any{"fields": fields, "items": items},
		})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "fixture-token")
	got, err := client.FetchSecurityTradingStatus(context.Background(), "920001.BJ", "2026-10-22")
	if err != nil {
		t.Fatalf("FetchSecurityTradingStatus for delisted symbol: %v", err)
	}
	if len(stockBasicStatuses) != 2 || stockBasicStatuses[0] != "L" || stockBasicStatuses[1] != "D" {
		t.Fatalf("expected L then D status lookup to find the historical security: %v", stockBasicStatuses)
	}
	if got.ListingDate != "2020-01-01" {
		t.Fatalf("expected original listing date for delisted security, got %+v", got)
	}
	if got.DelistingStatus != model.DelistingStatusDelistingPeriod || got.RiskWarningStatus != model.RiskWarningUnknown {
		t.Fatalf("expected delisting-period classification without treating it as normal ST status: %+v", got)
	}
	if tradeCalendarExchange != "SSE" || !got.TradingDayKnown {
		t.Fatalf("expected documented shared market calendar for Beijing Exchange, got exchange=%q status=%+v", tradeCalendarExchange, got)
	}
}

func TestFetchHistoricalRiskWarningStatusUsesExactDailyStockSTList(t *testing.T) {
	cases := []struct {
		name    string
		fields  []string
		items   [][]any
		want    model.RiskWarningStatus
		wantErr bool
	}{
		{name: "not in ST list means normal", items: [][]any{}, want: model.RiskWarningNormal},
		{name: "empty response without schema is unknown", fields: []string{}, items: [][]any{}, want: model.RiskWarningUnknown, wantErr: true},
		{name: "matching ST row", items: [][]any{{"600519.SH", "*ST贵州", "20261022", "ST", "风险警示板"}}, want: model.RiskWarningApplied},
		{name: "wrong symbol", items: [][]any{{"600000.SH", "ST浦发", "20261022", "ST", "风险警示板"}}, want: model.RiskWarningUnknown, wantErr: true},
		{name: "wrong date", items: [][]any{{"600519.SH", "*ST贵州", "20261021", "ST", "风险警示板"}}, want: model.RiskWarningUnknown, wantErr: true},
		{name: "duplicate rows", items: [][]any{{"600519.SH", "*ST贵州", "20261022", "ST", "风险警示板"}, {"600519.SH", "*ST贵州", "20261022", "ST", "风险警示板"}}, want: model.RiskWarningUnknown, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					APIName string            `json:"api_name"`
					Params  map[string]string `json:"params"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Errorf("decode request: %v", err)
					http.Error(w, "bad request", http.StatusBadRequest)
					return
				}
				if request.APIName != "stock_st" || request.Params["ts_code"] != "600519.SH" || request.Params["trade_date"] != "20261022" {
					t.Errorf("unexpected stock_st request: %+v", request)
				}
				fields := tc.fields
				if fields == nil {
					fields = []string{"ts_code", "name", "trade_date", "type", "type_name"}
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"code": 0, "msg": "",
					"data": map[string]any{"fields": fields, "items": tc.items},
				})
			}))
			defer server.Close()
			client := NewTushareConfigured(2*time.Second, server.URL, "fixture-token")
			got, err := client.fetchHistoricalRiskWarningStatus(context.Background(), "600519.SH", "20261022")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error=%v, wantErr=%v", err, tc.wantErr)
			}
			if got != tc.want {
				t.Fatalf("status=%q, want %q", got, tc.want)
			}
		})
	}
}

func TestFetchHistoricalRiskWarningStatusDoesNotAssumePre2000Normal(t *testing.T) {
	client := NewTushareConfigured(2*time.Second, "http://127.0.0.1:1", "fixture-token")
	got, err := client.fetchHistoricalRiskWarningStatus(context.Background(), "600519.SH", "19991231")
	if err == nil || got != model.RiskWarningUnknown {
		t.Fatalf("pre-2000 status must remain unknown without an upstream request: status=%q err=%v", got, err)
	}
}

func TestTushareFetchSecurityTradingStatusDoesNotInferNormalSTStateOnNonTradingDate(t *testing.T) {
	stockSTCalls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			APIName string            `json:"api_name"`
			Params  map[string]string `json:"params"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		var fields []string
		var items [][]any
		switch request.APIName {
		case "stock_basic":
			fields = []string{"ts_code", "list_status", "list_date", "market"}
			items = [][]any{{"920001.BJ", "L", "20200101", "北交所"}}
		case "namechange":
			fields = []string{"ts_code", "name", "start_date", "end_date", "ann_date", "change_reason"}
			items = [][]any{{"920001.BJ", "北证示例", "20200101", nil, "20200101", "上市"}}
		case "trade_cal":
			fields = []string{"exchange", "cal_date", "is_open"}
			items = [][]any{{"SSE", "20261024", "0"}}
		case "suspend_d":
			fields = []string{"ts_code", "trade_date", "suspend_timing", "suspend_type"}
			items = [][]any{}
		case "stock_st":
			stockSTCalls++
			t.Errorf("stock_st must not be queried for a non-trading date")
			fields = []string{"ts_code", "name", "trade_date", "type", "type_name"}
			items = [][]any{}
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected API", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0, "msg": "",
			"data": map[string]any{"fields": fields, "items": items},
		})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "fixture-token")
	got, err := client.FetchSecurityTradingStatus(context.Background(), "920001.BJ", "2026-10-24")
	if err != nil {
		t.Fatalf("FetchSecurityTradingStatus: %v", err)
	}
	if got.IsTradingDay || !got.TradingDayKnown {
		t.Fatalf("expected a verified non-trading date, got %+v", got)
	}
	if got.RiskWarningStatus != model.RiskWarningUnknown {
		t.Fatalf("risk warning status must remain unknown without a daily ST-list snapshot: %+v", got)
	}
	if stockSTCalls != 0 {
		t.Fatalf("unexpected stock_st calls: %d", stockSTCalls)
	}
	for _, reason := range got.QualityReasons {
		if reason == "authoritative_st_list_requires_trading_date" {
			return
		}
	}
	t.Fatalf("expected explicit non-trading-date quality reason: %v", got.QualityReasons)
}
