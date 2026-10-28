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

func TestEastmoneyFetchDragonTigerRejectsFutureTradeDateWithoutCallingProvider(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	futureDate := time.Now().In(loc).AddDate(0, 0, 1).Format("2006-01-02")
	p := NewEastmoney(2*time.Second, "", "")
	if _, err := p.FetchDragonTiger(context.Background(), futureDate, 1, 100); err == nil || !strings.Contains(err.Error(), "must not be in the future") {
		t.Fatalf("future dragon-tiger date must be rejected before making a request, got %v", err)
	}
}

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
	if got.TradeDate != "2026-10-22" || got.ProviderReportDate != "2026-10-22" || got.Page != 2 || got.PageSize != 1 || got.Total != 3 || got.Pages != 3 || got.SourceReturned != 1 || got.Returned != 1 || got.ExcludedNonAShareCount != 0 || !got.HasMore {
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

func TestEastmoneyFetchDragonTigerExcludesConvertibleBondRecords(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"pages":1,"count":2,"data":[
			{"SECURITY_CODE":"600519","SECUCODE":"600519.SH","SECURITY_NAME_ABBR":"贵州茅台","TRADE_DATE":"2026-10-23 00:00:00","CLOSE_PRICE":"1313.31","CHANGE_RATE":"1.25","BILLBOARD_NET_AMT":"123456.78","BILLBOARD_BUY_AMT":"234567.89","BILLBOARD_SELL_AMT":"111111.11","BILLBOARD_DEAL_AMT":"345678.90","DEAL_AMOUNT_RATIO":"2.4","EXPLAIN":"日涨幅偏离值达到7%","SECURITY_TYPE_CODE":"058001001"},
			{"SECURITY_CODE":"113711","SECUCODE":"113711.SH","SECURITY_NAME_ABBR":"N常青转","TRADE_DATE":"2026-10-23 00:00:00","CLOSE_PRICE":"137.93","CHANGE_RATE":"37.93","BILLBOARD_NET_AMT":"7284700","BILLBOARD_BUY_AMT":"32062300","BILLBOARD_SELL_AMT":"24777600","BILLBOARD_DEAL_AMT":"90319700","DEAL_AMOUNT_RATIO":"62.93","EXPLAIN":"可转债上市首日交易公开信息","SECURITY_TYPE_CODE":"060"}
		]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchDragonTiger(context.Background(), "2026-10-23", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.Total != 2 || got.Pages != 1 || got.SourceReturned != 2 || got.Returned != 1 || got.ExcludedNonAShareCount != 1 || !got.HasData {
		t.Fatalf("provider/source row counts were not preserved: %+v", got)
	}
	if len(got.Items) != 1 || got.Items[0].Symbol != "600519.SH" {
		t.Fatalf("only the valid A-share row should remain: %+v", got.Items)
	}
	if !strings.Contains(strings.Join(got.QualityReasons, ","), "provider_non_a_share_security_rows_excluded") {
		t.Fatalf("non-A-share exclusion should be visible in quality reasons: %+v", got.QualityReasons)
	}
}

func TestEastmoneyFetchDragonTigerBondOnlyPageRemainsUnknown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"pages":1,"count":1,"data":[{"SECURITY_CODE":"113711","SECUCODE":"113711.SH","SECURITY_NAME_ABBR":"N常青转","TRADE_DATE":"2026-10-23 00:00:00","BILLBOARD_NET_AMT":"7284700","BILLBOARD_BUY_AMT":"32062300","BILLBOARD_SELL_AMT":"24777600","BILLBOARD_DEAL_AMT":"90319700","DEAL_AMOUNT_RATIO":"62.93","EXPLAIN":"可转债上市首日交易公开信息","SECURITY_TYPE_CODE":"060"}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	got, err := p.FetchDragonTiger(context.Background(), "2026-10-23", 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.HasData || got.Total != 1 || got.Pages != 1 || got.SourceReturned != 1 ||
		got.Returned != 0 || got.ExcludedNonAShareCount != 1 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("bond-only provider page must be unknown, not a confirmed empty stock report: %+v", got)
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


func TestEastmoneyFetchDragonTigerRejectsUnderfilledPage(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		fmt.Fprint(w, `{"result":{"pages":2,"count":101,"data":[{}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	_, err := p.FetchDragonTiger(context.Background(), "2026-10-22", 1, 100)
	if err == nil || !strings.Contains(err.Error(), "returned 1 raw rows, expected 100") {
		t.Fatalf("underfilled dragon-tiger page must be rejected, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("underfilled source page should fail immediately, got %d requests", requests)
	}
}

func TestEastmoneyFetchDragonTigerRejectsInconsistentPageCount(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"result":{"pages":1,"count":101,"data":[{}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	_, err := p.FetchDragonTiger(context.Background(), "2026-10-22", 1, 100)
	if err == nil || !strings.Contains(err.Error(), "disagree with count=101") {
		t.Fatalf("inconsistent source pages must be rejected, got %v", err)
	}
}


func TestEastmoneyFetchDragonTigerRejectsExplicitFailureWithRows(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":false,"code":0,"message":"数据暂不可用","result":{"pages":1,"count":1,"data":[{"SECURITY_CODE":"600519","SECUCODE":"600519.SH","SECURITY_NAME_ABBR":"贵州茅台","TRADE_DATE":"2026-10-22 00:00:00","BILLBOARD_NET_AMT":"100","BILLBOARD_BUY_AMT":"200","BILLBOARD_SELL_AMT":"100","CLOSE_PRICE":"1313.3","CHANGE_RATE":"1.2","DEAL_AMOUNT_RATIO":"2.1"}]}}`)
	}))
	defer server.Close()

	p := NewEastmoneyConfiguredWithDragonTiger(2*time.Second, "", "", "", "", "", "", "", "", server.URL, "")
	_, err := p.FetchDragonTiger(context.Background(), "2026-10-22", 1, 100)
	if err == nil || !strings.Contains(err.Error(), "success=false") {
		t.Fatalf("explicit provider failure must reject even populated result data, got %v", err)
	}
}
