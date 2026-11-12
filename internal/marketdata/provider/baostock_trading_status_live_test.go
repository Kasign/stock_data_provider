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
		fullDaySymbol  = "sz.002968"
		fullDayStart   = "2025-09-12"
		fullDayEnd     = "2025-09-30"
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

	rowsByDate := make(map[string]baostock.DailyKLine, len(fullDayRows))
	tradeStatusCounts := make(map[string]int)
	isSTCounts := make(map[string]int)
	for _, row := range fullDayRows {
		if row.TradeStatus != "0" && row.TradeStatus != "1" {
			t.Fatalf("BaoStock tradestatus is missing or invalid on %s: %q", row.Date, row.TradeStatus)
		}
		if row.IsST != "0" && row.IsST != "1" {
			t.Fatalf("BaoStock isST is missing or invalid on %s: %q", row.Date, row.IsST)
		}
		if _, exists := rowsByDate[row.Date]; exists {
			t.Fatalf("BaoStock returned duplicate daily row for %s", row.Date)
		}
		rowsByDate[row.Date] = row
		tradeStatusCounts[row.TradeStatus]++
		isSTCounts[row.IsST]++
	}

	const (
		suspensionStart = "2025-09-15"
		suspensionEnd   = "2025-09-28"
		resumeDate      = "2025-09-29"
	)
	suspensionTradingDays := 0
	for _, day := range calendar {
		if !day.IsTradingDay || day.Date < suspensionStart || day.Date > suspensionEnd {
			continue
		}
		row, exists := rowsByDate[day.Date]
		if !exists {
			t.Fatalf("BaoStock omitted daily row for suspended trading day %s", day.Date)
		}
		if row.TradeStatus != "0" {
			t.Fatalf("BaoStock tradestatus on official full-day suspension %s = %q, want 0", day.Date, row.TradeStatus)
		}
		suspensionTradingDays++
	}
	if suspensionTradingDays == 0 {
		t.Fatal("BaoStock calendar returned no trading days inside the official full-day suspension window")
	}
	resumeRow, exists := rowsByDate[resumeDate]
	if !exists {
		t.Fatalf("BaoStock omitted daily row on official resume date %s", resumeDate)
	}
	if resumeRow.TradeStatus != "1" {
		t.Fatalf("BaoStock tradestatus on official resume date %s = %q, want 1", resumeDate, resumeRow.TradeStatus)
	}

	intradayEvidence := make([]map[string]string, 0, len(intradayRows))
	intradayStatusFound := false
	for _, row := range intradayRows {
		if row.Date != intradayDate {
			t.Fatalf("BaoStock returned unexpected date %s for intraday event %s", row.Date, intradayDate)
		}
		if row.TradeStatus != "1" {
			t.Fatalf("BaoStock daily tradestatus on intraday-only suspension date %s = %q, want 1", intradayDate, row.TradeStatus)
		}
		if row.IsST != "0" && row.IsST != "1" {
			t.Fatalf("BaoStock isST is missing or invalid on intraday event date %s: %q", intradayDate, row.IsST)
		}
		intradayStatusFound = true
		intradayEvidence = append(intradayEvidence, map[string]string{
			"date":         row.Date,
			"trade_status": row.TradeStatus,
			"is_st":        row.IsST,
			"open":         row.Open,
			"high":         row.High,
			"low":          row.Low,
			"close":        row.Close,
			"volume":       row.Volume,
			"amount":       row.Amount,
		})
	}
	if !intradayStatusFound {
		t.Fatalf("BaoStock returned no row for intraday event date %s", intradayDate)
	}

	report := map[string]any{
		"provider": "baostock",
		"full_day_suspension": map[string]any{
			"official_url":                 "https://www.szse.cn/disclosure/notice/temp/t20250915_616077.html",
			"symbol":                       fullDaySymbol,
			"official_suspension_window":   "2025-09-15..2025-09-28",
			"suspension_trading_day_count": suspensionTradingDays,
			"resume_trade_status":          resumeRow.TradeStatus,
			"official_resume_date":         "2025-09-29",
			"calendar":                     calendar,
			"daily_rows":                   fullDayRows,
			"trade_status_counts":          tradeStatusCounts,
			"is_st_counts":                 isSTCounts,
		},
		"intraday_suspension": map[string]any{
			"official_url":            "https://www.sse.com.cn/disclosure/announcement/general/mainsuspension/c/c_20250103_10767682.shtml",
			"symbol":                  intradaySymbol,
			"event_date":              intradayDate,
			"daily_tradestatus_scope": "daily status does not encode the intraday halt interval",
			"daily_rows":              intradayEvidence,
		},
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal BaoStock trade-status evidence: %v", err)
	}
	t.Logf("LIVE_BAOSTOCK_TRADE_STATUS_REPORT %s", encoded)
}
