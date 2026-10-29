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
			fields = []string{"ts_code","name","industry","market","exchange","list_status","list_date","delist_date"}
			items = [][]any{{"600519.SH","贵州茅台","白酒","主板","SSE","L","20261020",nil}}
		case "namechange":
			fields = []string{"ts_code","name","start_date","end_date","ann_date","change_reason"}
			items = [][]any{
				{"600519.SH","贵州茅台","20010827","20261019","20010827","改名"},
				{"600519.SH","*ST贵州","20261020",nil,"20261019","实施风险警示"},
			}
		case "trade_cal":
			fields = []string{"exchange","cal_date","is_open"}
			switch {
			case request.Params["start_date"] == "20261022" && request.Params["end_date"] == "20261022":
				items = [][]any{{request.Params["exchange"],"20261022","1"}}
			case request.Params["start_date"] == "20261020" && request.Params["end_date"] == "20261022":
				items = [][]any{
					{request.Params["exchange"],"20261020","1"},
					{request.Params["exchange"],"20261021","1"},
					{request.Params["exchange"],"20261022","1"},
				}
			default:
				t.Errorf("unexpected trade_cal params: %+v", request.Params)
				items = [][]any{}
			}
		case "suspend_d":
			fields = []string{"ts_code","trade_date","suspend_timing","suspend_type"}
			items = [][]any{{"600519.SH","20261022","09:30-10:00","S"}}
		default:
			t.Errorf("unexpected Tushare api_name %q", request.APIName)
			http.Error(w, "unexpected API", http.StatusBadRequest)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": 0,
			"msg": "",
			"data": map[string]any{"fields":fields,"items":items},
		})
	}))
	defer server.Close()

	client := NewTushareConfigured(2*time.Second, server.URL, "fixture-token")
	got, err := client.FetchSecurityTradingStatus(context.Background(),"600519.SH","2026-10-22")
	if err != nil {
		t.Fatalf("FetchSecurityTradingStatus: %v",err)
	}
	if got.TradeDate!="2026-10-22" || !got.IsTradingDay || !got.TradingDayKnown {
		t.Fatalf("unexpected exchange-calendar status: %+v",got)
	}
	if got.SecurityName!="*ST贵州" || got.RiskWarningStatus!=model.RiskWarningApplied {
		t.Fatalf("historical effective name was not applied: %+v",got)
	}
	if got.NameEffectiveFrom!="2026-10-20" || got.NameEffectiveTo!="" {
		t.Fatalf("unexpected effective name interval: %+v",got)
	}
	if !got.ListingStatusKnown || got.ListingTradingDayNumber!=3 || got.ListingDate!="2026-10-20" {
		t.Fatalf("listing-day count should include IPO date and requested open date: %+v",got)
	}
	if got.SuspensionStatus!=model.SuspensionStatusIntraday || got.SuspensionTiming!="09:30-10:00" {
		t.Fatalf("intraday suspension timing was lost: %+v",got)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls["stock_basic"]!=1 || calls["namechange"]!=1 || calls["suspend_d"]!=1 || calls["trade_cal"]!=2 {
		t.Fatalf("unexpected Tushare capability calls: %+v",calls)
	}
}

func TestRiskWarningStatusFromHistoricalNameDoesNotGuessUnknownNames(t *testing.T) {
	cases:=[]struct{name string; want model.RiskWarningStatus}{
		{name:"*ST示例",want:model.RiskWarningApplied},
		{name:"ST示例",want:model.RiskWarningApplied},
		{name:"S*ST示例",want:model.RiskWarningApplied},
		{name:"SST示例",want:model.RiskWarningApplied},
		{name:"示例股份",want:model.RiskWarningNormal},
		{name:"",want:model.RiskWarningUnknown},
		{name:"PT示例",want:model.RiskWarningUnknown},
	}
	for _,tc:=range cases{
		if got:=riskWarningStatusFromHistoricalName(tc.name);got!=tc.want{
			t.Errorf("riskWarningStatusFromHistoricalName(%q)=%q, want %q",tc.name,got,tc.want)
		}
	}
}
