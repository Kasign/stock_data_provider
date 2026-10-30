package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func resetMiaoXiangRateLimitForTest() {
	miaoXiangRateMu.Lock()
	miaoXiangLastRequest = time.Time{}
	miaoXiangRateMu.Unlock()
}

func newMiaoXiangTestServer(t *testing.T, response string, handler func(http.ResponseWriter, *http.Request)) (*MiaoXiang, *int, func()) {
	t.Helper()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if handler != nil {
			handler(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, response)
	}))
	resetMiaoXiangRateLimitForTest()
	p, err := NewMiaoXiang("mx-test-key-12345678", server.URL, 2*time.Second)
	if err != nil {
		server.Close()
		t.Fatalf("NewMiaoXiang: %v", err)
	}
	return p, &requests, server.Close
}

func TestMiaoXiangFetchChipDistributionParsesOfficialResponseShape(t *testing.T) {
	response := `{
		"status":0,
		"data":{"data":{"searchDataResultDTO":{"dataTableDTOList":[{
			"nameMap":{"0":"平均成本","1":"获利比例","2":"90%筹码集中度","3":"70%筹码集中度"},
			"table":{"headName":["2026-10-21"],"0":["12.50"],"1":["83.24%"],"2":["15.2%"],"3":["9.5%"]}
		}]}}}
	}`
	p, _, cleanup := newMiaoXiangTestServer(t, response, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s want POST", r.Method)
		}
		if r.Header.Get("apikey") != "mx-test-key-12345678" {
			t.Error("MX API key header missing")
		}
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode query: %v", err)
		}
		if !strings.Contains(body["toolQuery"], "600519.SH") || !strings.Contains(body["toolQuery"], "平均成本") {
			t.Errorf("unexpected query %q", body["toolQuery"])
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, response)
	})
	defer cleanup()

	got, err := p.FetchChipDistribution(context.Background(), "600519")
	if err != nil {
		t.Fatalf("FetchChipDistribution: %v", err)
	}
	if got.Symbol != "600519.SH" || got.Source != "miaoxiang" || got.SourceVersion != miaoXiangVersion {
		t.Fatalf("unexpected identity: %+v", got)
	}
	if got.AverageCost != 12.5 || got.Date != "2026-10-21" {
		t.Fatalf("unexpected snapshot: %+v", got)
	}
	if got.ProfitRatio == nil || *got.ProfitRatio < 0.83239 || *got.ProfitRatio > 0.83241 {
		t.Fatalf("profit ratio should normalize 83.24%% to 0.8324: %+v", got.ProfitRatio)
	}
	if got.Concentration90 == nil || *got.Concentration90 < 0.1519 || *got.Concentration90 > 0.1521 {
		t.Fatalf("90%% concentration wrong: %+v", got.Concentration90)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("publication-availability limitation must be visible: %+v", got)
	}
}

func TestMiaoXiangCapitalFlowRequiresCompleteWindow(t *testing.T) {
	response := `{
		"status":0,
		"data":{"dataTableDTOList":[{
			"nameMap":{"0":"主力净流入金额"},
			"table":{"headName":["2026-10-21","2026-10-20","2026-10-19","2026-10-16","2026-10-15"],"0":["1亿","2亿","-100万元","4亿","-"]}
		}]}
	}`
	p, _, cleanup := newMiaoXiangTestServer(t, response, nil)
	defer cleanup()

	got, err := p.FetchCapitalFlow(context.Background(), "600519.SH")
	if err != nil {
		t.Fatalf("FetchCapitalFlow: %v", err)
	}
	if got.MainNetInflow == nil || *got.MainNetInflow != 1e8 {
		t.Fatalf("today's inflow should normalize to CNY: %+v", got.MainNetInflow)
	}
	if got.Inflow5D != nil || got.Inflow10D != nil {
		t.Fatalf("incomplete windows must not be summed: 5d=%v 10d=%v", got.Inflow5D, got.Inflow10D)
	}
	if got.QualityStatus != model.QualityDegraded || got.Observations != 5 {
		t.Fatalf("unexpected flow quality: %+v", got)
	}
	if got.ProviderDate != "2026-10-21" {
		t.Fatalf("capital-flow provider date must reflect the latest observation: %q", got.ProviderDate)
	}
}

func TestMiaoXiangCapitalFlowSumsCompleteRows(t *testing.T) {
	response := `{
		"status":0,
		"data":{"dataTableDTOList":[{
			"nameMap":{"0":"主力净流入资金"},
			"table":{"headName":["2026-10-21","2026-10-20","2026-10-19","2026-10-16","2026-10-15","2026-10-14","2026-10-13","2026-10-12","2026-10-09","2026-10-08"],"0":["1亿","2亿","3亿","4亿","5亿","6亿","7亿","8亿","9亿","10亿"]}
		}]}
	}`
	p, _, cleanup := newMiaoXiangTestServer(t, response, nil)
	defer cleanup()

	got, err := p.FetchCapitalFlow(context.Background(), "600519")
	if err != nil {
		t.Fatalf("FetchCapitalFlow: %v", err)
	}
	if got.Inflow5D == nil || *got.Inflow5D != 15e8 {
		t.Fatalf("wrong 5-day total: %+v", got.Inflow5D)
	}
	if got.Inflow10D == nil || *got.Inflow10D != 55e8 {
		t.Fatalf("wrong 10-day total: %+v", got.Inflow10D)
	}
}

func TestMiaoXiangRejectsETFAndBadHTTPStatus(t *testing.T) {
	p, _, cleanup := newMiaoXiangTestServer(t, "", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream unavailable", http.StatusServiceUnavailable)
	})
	defer cleanup()

	if _, err := p.FetchChipDistribution(context.Background(), "510050"); err == nil {
		t.Fatal("ETF chip distribution must be rejected")
	}
	if _, err := p.FetchCapitalFlow(context.Background(), "600519"); err == nil {
		t.Fatal("provider HTTP failure must be returned to fallback logic")
	}
}

func TestMiaoXiangCacheAvoidsDuplicateExternalRequests(t *testing.T) {
	response := `{
		"status":0,
		"data":{"dataTableDTOList":[{
			"nameMap":{"0":"平均成本"},
			"table":{"headName":["2026-10-21"],"0":["12.50"]}
		}]}
	}`
	p, requests, cleanup := newMiaoXiangTestServer(t, response, nil)
	defer cleanup()
	if _, err := p.FetchChipDistribution(context.Background(), "600519"); err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if _, err := p.FetchChipDistribution(context.Background(), "600519"); err != nil {
		t.Fatalf("cached fetch: %v", err)
	}
	if *requests != 1 {
		t.Fatalf("expected one HTTP request, got %d", *requests)
	}
}

func TestNewMiaoXiangRejectsPlaceholderKeyAndInvalidEndpoint(t *testing.T) {
	if _, err := NewMiaoXiang("replace-with-your-mx-api-key", "", time.Second); err == nil {
		t.Fatal("placeholder key must be rejected")
	}
	if _, err := NewMiaoXiang("mx-test-key-12345678", "file:///tmp/mx", time.Second); err == nil {
		t.Fatal("non-HTTP(S) endpoint must be rejected")
	}
}


func TestMiaoXiangCapitalFlowRejectsInvalidObservationDates(t *testing.T) {
	cases := []struct {
		name  string
		dates string
		want  string
	}{
		{name: "missing date", dates: `["2026-10-21",""]`, want: "invalid observation date"},
		{name: "duplicate date", dates: `["2026-10-21","2026-10-21"]`, want: "duplicate date"},
		{name: "future date", dates: `["2099-10-21"]`, want: "in the future"},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			response := `{"status":0,"data":{"dataTableDTOList":[{"nameMap":{"0":"主力净流入金额"},"table":{"headName":` + item.dates + `,"0":["1亿","2亿"]}}]}}`
			p, _, cleanup := newMiaoXiangTestServer(t, response, nil)
			defer cleanup()
			if _, err := p.FetchCapitalFlow(context.Background(), "600519"); err == nil || !strings.Contains(err.Error(), item.want) {
				t.Fatalf("invalid observation dates must be rejected with %q, got %v", item.want, err)
			}
		})
	}
}
