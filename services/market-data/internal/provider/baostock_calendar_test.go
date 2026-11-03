package provider

import (
	"context"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

type fakeBaostockCalendarClient struct {
	rows []baostock.TradeDate
	err  error
}

func (f fakeBaostockCalendarClient) QueryDailyKLines(context.Context, string, string, string, string) ([]baostock.DailyKLine, error) {
	return nil, nil
}
func (f fakeBaostockCalendarClient) QueryTradeDates(context.Context, string, string) ([]baostock.TradeDate, error) {
	return f.rows, f.err
}

func TestBaostockTradingCalendarBuildsCompleteInclusiveRange(t *testing.T) {
	p := NewBaostock(fakeBaostockCalendarClient{rows: []baostock.TradeDate{
		{Date: "2026-10-22", IsTradingDay: true},
		{Date: "2026-10-23", IsTradingDay: true},
		{Date: "2026-10-24", IsTradingDay: false},
	}})
	got, err := p.FetchTradingCalendar(context.Background(), "2026-10-22", "2026-10-24")
	if err != nil {
		t.Fatal(err)
	}
	if got.Market != "cn_a_share" || got.TotalDays != 3 || got.TradingDays != 2 || len(got.Items) != 3 {
		t.Fatalf("unexpected trading calendar: %+v", got)
	}
	if got.QualityStatus != model.QualityDegraded || got.Items[2].IsTradingDay {
		t.Fatalf("missing conservative quality flag or non-trading day: %+v", got)
	}
}

func TestBaostockTradingCalendarRejectsIncompleteCoverage(t *testing.T) {
	p := NewBaostock(fakeBaostockCalendarClient{rows: []baostock.TradeDate{
		{Date: "2026-10-22", IsTradingDay: true},
		{Date: "2026-10-24", IsTradingDay: false},
	}})
	if _, err := p.FetchTradingCalendar(context.Background(), "2026-10-22", "2026-10-24"); err == nil {
		t.Fatal("missing 2026-10-23 must not be accepted as a holiday")
	}
}
