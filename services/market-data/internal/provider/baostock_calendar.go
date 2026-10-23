package provider

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

type baostockTradeCalendarClient interface {
	QueryTradeDates(context.Context, string, string) ([]baostock.TradeDate, error)
}

func (p *Baostock) FetchTradingCalendar(ctx context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error) {
	client, ok := p.client.(baostockTradeCalendarClient)
	if !ok {
		return nil, errors.New("BaoStock client does not implement trading-calendar queries")
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil {
		return nil, fmt.Errorf("invalid trading-calendar start date %q", startDate)
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil {
		return nil, fmt.Errorf("invalid trading-calendar end date %q", endDate)
	}
	if start.After(end) {
		return nil, errors.New("trading-calendar start date must not be after end date")
	}
	totalDays := int(end.Sub(start)/(24*time.Hour)) + 1
	if totalDays > 3660 {
		return nil, errors.New("trading-calendar date range must not exceed 3660 calendar days")
	}
	rows, err := client.QueryTradeDates(ctx, startDate, endDate)
	if err != nil {
		return nil, err
	}
	if len(rows) != totalDays {
		return nil, fmt.Errorf("BaoStock trading-calendar coverage incomplete: received %d of %d calendar days", len(rows), totalDays)
	}
	items := make([]model.TradingCalendarDay, 0, totalDays)
	tradingDays := 0
	for index, row := range rows {
		expectedDate := start.AddDate(0, 0, index).Format("2006-01-02")
		if row.Date != expectedDate {
			return nil, fmt.Errorf("BaoStock trading-calendar date mismatch at row %d: got %s, want %s", index, row.Date, expectedDate)
		}
		if row.IsTradingDay {
			tradingDays++
		}
		items = append(items, model.TradingCalendarDay{Date: row.Date, IsTradingDay: row.IsTradingDay})
	}
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: startDate, EndDate: endDate,
		TotalDays: totalDays, TradingDays: tradingDays, Items: items,
		Source: p.Name(), SourceVersion: "baostock-query-trade-dates-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}, nil
}
