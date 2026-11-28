package baostock

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestParseAllStockPageReadsDocumentedFields(t *testing.T) {
	body := strings.Join([]string{
		"0", "success", "query_all_stock", "anonymous", "1", "10000",
		`{"record":[["sh.600519","1","贵州茅台"],["sz.000001","1","平安银行"]]}`,
		"2026-11-27", "code,tradeStatus,code_name",
	}, messageSplit)

	page, err := parseAllStockPage(body)
	if err != nil {
		t.Fatalf("parseAllStockPage() error = %v", err)
	}
	if page.errorCode != "0" || page.pageSize != 10000 || page.tradeDate != "2026-11-27" {
		t.Fatalf("unexpected page header: %+v", page)
	}
	if len(page.rows) != 2 {
		t.Fatalf("rows = %d, want 2", len(page.rows))
	}
	if page.rows[0] != (AllStockRecord{Code: "sh.600519", TradeStatus: "1", Name: "贵州茅台"}) {
		t.Fatalf("unexpected first row: %+v", page.rows[0])
	}
}

func TestParseAllStockPageNormalizesUpperCaseCode(t *testing.T) {
	body := strings.Join([]string{
		"0", "success", "query_all_stock", "anonymous", "1", "10000",
		`{"record":[["SH.600519","1","贵州茅台"]]}`,
		"2026-11-27", "code,tradeStatus,code_name",
	}, messageSplit)

	page, err := parseAllStockPage(body)
	if err != nil {
		t.Fatalf("parseAllStockPage() error = %v", err)
	}
	if page.rows[0].Code != "sh.600519" {
		t.Fatalf("code = %q, want lowercase sh.600519", page.rows[0].Code)
	}
}

func TestParseAllStockPageReturnsEmptyLastPage(t *testing.T) {
	body := strings.Join([]string{
		"0", "success", "query_all_stock", "anonymous", "2", "10000", "",
		"2026-11-27", "code,tradeStatus,code_name",
	}, messageSplit)

	page, err := parseAllStockPage(body)
	if err != nil {
		t.Fatalf("parseAllStockPage() error = %v", err)
	}
	if len(page.rows) != 0 {
		t.Fatalf("rows = %d, want 0", len(page.rows))
	}
}

func TestParseAllStockPageRejectsContractViolations(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"fewer than two status fields", "0"},
		{"missing structured fields", strings.Join([]string{"0", "success", "query_all_stock"}, messageSplit)},
		{"unexpected field list", strings.Join([]string{"0", "success", "query_all_stock", "anonymous", "1", "10000", `{"record":[]}`, "2026-11-27", "code,name"}, messageSplit)},
		{"invalid trade date", strings.Join([]string{"0", "success", "query_all_stock", "anonymous", "1", "10000", `{"record":[]}`, "20261127", "code,tradeStatus,code_name"}, messageSplit)},
		{"row with two columns", strings.Join([]string{"0", "success", "query_all_stock", "anonymous", "1", "10000", `{"record":[["sh.600519","1"]]}`, "2026-11-27", "code,tradeStatus,code_name"}, messageSplit)},
		{"code without exchange prefix", strings.Join([]string{"0", "success", "query_all_stock", "anonymous", "1", "10000", `{"record":[["600519","1","贵州茅台"]]}`, "2026-11-27", "code,tradeStatus,code_name"}, messageSplit)},
		{"undecodable record payload", strings.Join([]string{"0", "success", "query_all_stock", "anonymous", "1", "10000", `{"record":`, "2026-11-27", "code,tradeStatus,code_name"}, messageSplit)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := parseAllStockPage(tc.body); err == nil {
				t.Fatal("parseAllStockPage() accepted a response that violates the all-stock contract")
			}
		})
	}
}

func TestParseAllStockPageSurfacesRejectedQuery(t *testing.T) {
	body := strings.Join([]string{"10004006", "请求参数个数不正确"}, messageSplit)
	page, err := parseAllStockPage(body)
	if err != nil {
		t.Fatalf("parseAllStockPage() error = %v", err)
	}
	if page.errorCode != "10004006" {
		t.Fatalf("errorCode = %q, want 10004006", page.errorCode)
	}
}

func TestQueryAllStocksLiveDirectory(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_BAOSTOCK_UNIVERSE_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_BAOSTOCK_UNIVERSE_TESTS=1 to query the live BaoStock security directory")
	}
	client := NewClient(Config{
		Host: "public-api.baostock.com", Port: 10030,
		UserID: "anonymous", Password: "123456",
		ConnectTimeout: 8 * time.Second, RequestTimeout: 40 * time.Second,
	})
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	rows, err := client.QueryAllStocks(ctx, "2026-11-27")
	if err != nil {
		t.Fatalf("QueryAllStocks() error = %v", err)
	}
	if len(rows) < 5000 {
		t.Fatalf("all-stock rows = %d, want at least 5000", len(rows))
	}
	for _, row := range rows {
		if len(row.Code) != 9 || !strings.Contains(row.Code, ".") {
			t.Fatalf("row %+v does not use the sh./sz. exchange prefix contract", row)
		}
	}
}
