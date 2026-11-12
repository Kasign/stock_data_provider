package provider

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/baostock"
)

// TestLiveBaoStockTradeStatusOnOfficialSuspensionWindow 读取 BaoStock 原始 tradestatus/isST 字段，
// 用交易所公告的全天停牌窗口和盘中临停日期验证这些字段是否可作为 SecurityTradingStatus 证据。
func TestLiveBaoStockTradeStatusOnOfficialSuspensionWindow(t *testing.T) {
	if os.Getenv("MARKET_DATA_LIVE_BAOSTOCK_TRADE_STATUS_TESTS") != "1" {
		t.Skip("set MARKET_DATA_LIVE_BAOSTOCK_TRADE_STATUS_TESTS=1 to query BaoStock trade-status fields")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	client := baostock.NewClient(baostock.Config{
		ConnectTimeout: 8 * time.Second,
		RequestTimeout: 20 * time.Second,
	})
	defer func() {
		if err := client.Close(); err != nil {
			t.Logf("close BaoStock client: %v", err)
		}
	}()

	const (
		fullDaySymbol = "sz.002968"
		fullDayStart  = "2025-09-12"
		fullDayEnd    = "2025-09-30"
		intradaySymbol = "sh.603072"
		intradayDate   = "2025-01-03"
	)
	calendar, err := client.QueryTradeDates(ctx, fullDayStart, fullDayEnd)
	if err != nil {
		t.Fatalf("BaoStock QueryTradeDates %s..%s: %v", fullDayStart, fullDayEnd, err)
	}
	fullDayRows, err := client.QueryDailyKLines(ctx, fullDaySymbol, fullDayStart, fullDayEnd, "3")
	if err != nil {
		t.Fatalf("BaoStock QueryDailyKLines %s %s..%s: %v", fullDaySymbol, fullDayStart, fullDayEnd, err)
	}
	intradayRows, err := client.QueryDailyKLines(ctx, intradaySymbol, intradayDate, intradayDate, "3")
	if err != nil {
		t.Fatalf("BaoStock QueryDailyKLines %s %s: %v", intradaySymbol, intradayDate, err)
	}
	if len(calendar) == 0 {
		t.Fatalf("BaoStock returned no trading-calendar rows for %s..%s", fullDayStart, fullDayEnd)
	}
	if len(fullDayRows) == 0 {
		t.Fatalf("BaoStock returned no daily K-line rows for %s %s..%s", fullDaySymbol, fullDayStart, fullDayEnd)
	}
	if len(intradayRows) == 0 {
		t.Fatalf("BaoStock returned no daily K-line row for intraday suspension sample %s %s", intradaySymbol, intradayDate)
	}

	tradeStatusCounts := make(map[string]int)
	isSTCounts := make(map[string]int)
	for _, row := range fullDayRows {
		tradeStatusCounts[row.TradeStatus]++
		isSTCounts[row.IsST]++
	}
	intradayEvidence := make([]map[string]string, 0, len(intradayRows))
	for _, row := range intradayRows {
		intradayEvidence = append(intradayEvidence, map[string]string{
			"date": row.Date,
			"trade_status": row.TradeStatus,
			"is_st": row.IsST,
			"open": row.Open,
			"high": row.High,
			"low": row.Low,
			"close": row.Close,
			"volume": row.Volume,
			"amount": row.Amount,
		})
	}

	report := map[string]any{
		"provider": "baostock",
		"full_day_suspension": map[string]any{
			"official_url": "https://www.szse.cn/disclosure/notice/temp/t20250915_616077.html",
			"symbol": fullDaySymbol,
			"official_suspension_window": "2025-09-15..2025-09-28",
			"official_resume_date": "2025-09-29",
			"calendar": calendar,
			"daily_rows": fullDayRows,
			"trade_status_counts": tradeStatusCounts,
			"is_st_counts": isSTCounts,
		},
		"intraday_suspension": map[string]any{
			"official_url": "https://www.sse.com.cn/disclosure/announcement/general/mainsuspension/c/c_20250103_10767682.shtml",
			"symbol": intradaySymbol,
			"event_date": intradayDate,
			"daily_rows": intradayEvidence,
		},
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal BaoStock trade-status evidence: %v", err)
	}
	t.Logf("LIVE_BAOSTOCK_TRADE_STATUS_REPORT %s", encoded)
}
