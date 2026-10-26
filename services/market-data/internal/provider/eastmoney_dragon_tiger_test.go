package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestEastmoneyFetchDragonTigerPreservesUnitsDateAndPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		for key, want := range map[string]string{
			"reportName":  "RPT_DAILYBILLBOARD_DETAILSNEW",
			"pageNumber":  "2",
			"pageSize":    "1",
			"sortColumns": "BILLBOARD_NET_AMT",
			"sortTypes":   "-1",
			"source":      "WEB",
			"client":      "WEB",
			"filter":      "(TRADE_DATE='2026-10-22')",
		} {
			if got := q.Get(key); got != want {
				t.Errorf("%s=%q want %q", key, got, want)
			}
		}
		fmt.Fprint(w, `{"result":{"pages":3,"count":3,"data":[{"SECURITY_CODE":"600519","SECUCODE":"600519.SH","SECURITY_NAME_ABBR":"贵州茅台","TRADE_DATE":"2026-10-22 00:00:00","CLOSE_PRICE":"1313.31","CHANGE_RATE":"1.25","BILLBOARD_NET_AMT":"123456.78","BILLBOARD_BUY_AMT":"234567.89","BILLBOARD_SELL_AMT":"111111.11","BILLBOARD_DEAL_AMT":"345678.90","DEAL_AMOUNT_RATIO":"2.4","EXPLAIN":"日涨幅偏离值达到7%","EXPLANATION":"日涨幅偏离值达到7%","SECURITY_TYPE_CODE":"001001"}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchDragonTiger(context.Background(), "2026-10-22", 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got.TradeDate != "2026-10-22" || got.ProviderReportDate != "2026-10-22" || got.Page != 2 || got.PageSize != 1 || got.Total != 3 || got.Pages != 3 || got.Returned != 1 || !got.HasMore {
		t.Fatalf("unexpected dragon-tiger snapshot: %+v", got)
	}
	if got.QualityStatus != model.QualityDegraded || !strings.Contains(strings.Join(got.QualityReasons, ","), "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("report must stay degraded without provider snapshot time: %+v", got)
	}
	item := got.Items[0]
	if item.Symbol != "600519.SH" || item.Code != "600519" || item.Name != "贵州茅台" || item.TradeDate != "2026-10-22" {
		t.Fatalf("wrong row identity: %+v", item)
	}
	if item.NetBuyAmountCNY == nil || *item.NetBuyAmountCNY != 123456.78 || item.BuyAmountCNY == nil || *item.BuyAmountCNY != 234567.89 || item.SellAmountCNY == nil || *item.SellAmountCNY != 111111.11 || item.DealAmountCNY == nil || *item.DealAmountCNY != 345678.90 {
		t.Fatalf("amounts must be retained as CNY values: %+v", item)
	}
	if item.ChangePercent == nil || *item.ChangePercent != 1.25 || item.DealAmountRatio == nil || *item.DealAmountRatio != 2.4 {
		t.Fatalf("percentage fields must remain percentage points: %+v", item)
	}
}

func TestEastmoneyFetchDragonTigerKeepsEmptyReportUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"pages":0,"count":0,"data":[]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchDragonTiger(context.Background(), "2026-10-22", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasData || got.Total != 0 || got.Returned != 0 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("empty report must remain unknown: %+v", got)
	}
}

func TestEastmoneyFetchDragonTigerRejectsRowsForDifferentTradeDate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"pages":1,"count":1,"data":[{"SECURITY_CODE":"600519","SECUCODE":"600519.SH","SECURITY_NAME_ABBR":"贵州茅台","TRADE_DATE":"2026-10-21","BILLBOARD_NET_AMT":"100"}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	if _, err := p.FetchDragonTiger(context.Background(), "2026-10-22", 1, 100); err == nil {
		t.Fatal("provider must reject rows for a different trade date")
	}
}

func TestEastmoneyFetchDragonTigerTreatsCode9201AsUnknownEmptyReport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":null,"result":null,"success":false,"message":"返回数据为空","code":9201}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchDragonTiger(context.Background(), "2026-10-25", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasData || got.Total != 0 || got.Returned != 0 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("code 9201 must remain an unknown empty report: %+v", got)
	}
	if !strings.Contains(strings.Join(got.QualityReasons, ","), "provider_empty_response_code_9201") {
		t.Fatalf("response code should be retained in quality reasons: %+v", got.QualityReasons)
	}
}

