package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/baostock"
	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func TestBaostockAShareSymbolFiltersNonAShareRows(t *testing.T) {
	cases := []struct {
		raw      string
		symbol   string
		exchange string
		ok       bool
	}{
		{"sh.600519", "600519.SH", "SH", true},
		{"sh.688981", "688981.SH", "SH", true},
		{"sz.000001", "000001.SZ", "SZ", true},
		{"sz.300750", "300750.SZ", "SZ", true},
		{"sz.302132", "302132.SZ", "SZ", true},
		{"SH.600519", "600519.SH", "SH", true},
		{"sh.000001", "", "", false}, // 上证指数
		{"sz.399001", "", "", false}, // 深证指数
		{"sh.510300", "", "", false}, // 场内基金
		{"sh.900901", "", "", false}, // B 股
		{"sz.200011", "", "", false}, // B 股
		{"bj.430047", "", "", false}, // 北交所不在 BaoStock 目录契约内
		{"600519", "", "", false},
		{"", "", "", false},
	}
	for _, tc := range cases {
		symbol, code, exchange, ok := baostockAShareSymbol(tc.raw)
		if ok != tc.ok || symbol != tc.symbol || exchange != tc.exchange {
			t.Fatalf("baostockAShareSymbol(%q) = (%q, %q, %q, %v), want (%q, _, %q, %v)",
				tc.raw, symbol, code, exchange, ok, tc.symbol, tc.exchange, tc.ok)
		}
	}
}

func syntheticBaostockRecords(count int) []baostock.AllStockRecord {
	records := make([]baostock.AllStockRecord, 0, count+3)
	for i := 0; i < count; i++ {
		records = append(records, baostock.AllStockRecord{
			Code: fmt.Sprintf("sh.%06d", 600000+i), TradeStatus: "1", Name: fmt.Sprintf("沪市样本%04d", i),
		})
	}
	records = append(records,
		baostock.AllStockRecord{Code: "sh.000001", TradeStatus: "1", Name: "上证综合指数"},
		baostock.AllStockRecord{Code: "sz.399001", TradeStatus: "1", Name: "深证成指"},
		baostock.AllStockRecord{Code: "sh.600999", TradeStatus: "1", Name: ""},
	)
	return records
}

func TestBuildBaostockStockUniverseKeepsOnlyNamedAShares(t *testing.T) {
	records := syntheticBaostockRecords(1200)
	snapshot, err := buildBaostockStockUniverse(records, time.Date(2026, 11, 27, 7, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("buildBaostockStockUniverse() error = %v", err)
	}
	if snapshot.Total != 1200 || len(snapshot.Items) != 1200 {
		t.Fatalf("total = %d items = %d, want 1200", snapshot.Total, len(snapshot.Items))
	}
	if snapshot.Source != "baostock" || snapshot.SourceVersion != baostockUniverseVersion {
		t.Fatalf("unexpected source identity: %s/%s", snapshot.Source, snapshot.SourceVersion)
	}
	if snapshot.QualityStatus != model.QualityDegraded || !snapshot.TotalIsLowerBound {
		t.Fatalf("directory without Beijing exchange must stay degraded and lower-bound: %+v", snapshot)
	}
	if len(snapshot.Items) > 0 && snapshot.Items[0] != (model.StockSearchItem{Symbol: "600000.SH", Code: "600000", Name: "沪市样本0000", Exchange: "SH"}) {
		t.Fatalf("unexpected first item: %+v", snapshot.Items[0])
	}
	if !hasReason(snapshot.QualityReasons, "provider_universe_rows_without_name_excluded") {
		t.Fatalf("unnamed rows must be reported, got %v", snapshot.QualityReasons)
	}
}

func hasReason(reasons []string, want string) bool {
	for _, reason := range reasons {
		if reason == want {
			return true
		}
	}
	return false
}

func TestBuildBaostockStockUniverseRejectsIncompleteDirectory(t *testing.T) {
	_, err := buildBaostockStockUniverse(syntheticBaostockRecords(10), time.Now().UTC())
	if err == nil {
		t.Fatal("an under-sized directory must fail closed instead of serving a partial universe")
	}
}

func TestBuildBaostockStockUniverseRejectsDuplicateSymbol(t *testing.T) {
	records := syntheticBaostockRecords(1200)
	records = append(records, baostock.AllStockRecord{Code: "sh.600000", TradeStatus: "1", Name: "重复样本"})
	if _, err := buildBaostockStockUniverse(records, time.Now().UTC()); err == nil {
		t.Fatal("duplicate securities must be rejected")
	}
}

type fakeBaostockUniverseClient struct {
	records       []baostock.AllStockRecord
	allStockCalls int
	requested     []string
	failAllStock  error
}

func (f *fakeBaostockUniverseClient) QueryDailyKLines(context.Context, string, string, string, string) ([]baostock.DailyKLine, error) {
	return nil, errors.New("daily K-lines are not part of this test")
}

func (f *fakeBaostockUniverseClient) QueryTradeDates(_ context.Context, startDate, endDate string) ([]baostock.TradeDate, error) {
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, err
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, err
	}
	rows := make([]baostock.TradeDate, 0, 32)
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		rows = append(rows, baostock.TradeDate{Date: day.Format("2006-01-02"), IsTradingDay: day.Weekday() != time.Saturday && day.Weekday() != time.Sunday})
	}
	return rows, nil
}

func (f *fakeBaostockUniverseClient) QueryAllStocks(_ context.Context, tradeDate string) ([]baostock.AllStockRecord, error) {
	f.allStockCalls++
	f.requested = append(f.requested, tradeDate)
	if f.failAllStock != nil {
		return nil, f.failAllStock
	}
	return f.records, nil
}

func TestBaostockListStockUniverseUsesLatestTradingDayAndCaches(t *testing.T) {
	client := &fakeBaostockUniverseClient{records: syntheticBaostockRecords(1200)}
	provider := NewBaostock(client)

	snapshot, err := provider.ListStockUniverse(context.Background())
	if err != nil {
		t.Fatalf("ListStockUniverse() error = %v", err)
	}
	if snapshot.Source != baostockProviderName || snapshot.Total != 1200 {
		t.Fatalf("unexpected snapshot: source=%s total=%d", snapshot.Source, snapshot.Total)
	}
	if client.allStockCalls != 1 {
		t.Fatalf("directory calls = %d, want 1", client.allStockCalls)
	}
	requested, err := time.Parse("2006-01-02", client.requested[0])
	if err != nil {
		t.Fatalf("requested date %q is not a date: %v", client.requested[0], err)
	}
	if requested.After(providerNowUTC().In(providerMarketLocation())) {
		t.Fatalf("requested date %s is in the future in Asia/Shanghai", client.requested[0])
	}
	if weekday := requested.Weekday(); weekday == time.Saturday || weekday == time.Sunday {
		t.Fatalf("requested date %s is a weekend", client.requested[0])
	}

	if _, err := provider.ListStockUniverse(context.Background()); err != nil {
		t.Fatalf("second ListStockUniverse() error = %v", err)
	}
	if client.allStockCalls != 1 {
		t.Fatalf("directory calls after cache hit = %d, want 1", client.allStockCalls)
	}
}

func TestBaostockListStockUniverseSurfacesDirectoryFailure(t *testing.T) {
	client := &fakeBaostockUniverseClient{records: syntheticBaostockRecords(1200), failAllStock: errors.New("directory unavailable")}
	provider := NewBaostock(client)

	_, err := provider.ListStockUniverse(context.Background())
	if err == nil || !strings.Contains(err.Error(), "directory unavailable") {
		t.Fatalf("ListStockUniverse() error = %v, want the underlying directory failure", err)
	}
}

func TestBaostockListStockUniverseRejectsClientWithoutDirectoryQueries(t *testing.T) {
	provider := NewBaostock(dailyOnlyBaostockClient{})
	if _, err := provider.ListStockUniverse(context.Background()); err == nil {
		t.Fatal("a client without query_all_stock support must not silently produce an empty universe")
	}
}

type dailyOnlyBaostockClient struct{}

func (dailyOnlyBaostockClient) QueryDailyKLines(context.Context, string, string, string, string) ([]baostock.DailyKLine, error) {
	return nil, errors.New("daily K-lines are not part of this test")
}
