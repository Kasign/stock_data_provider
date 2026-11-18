package service

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

func TestScheduledMarketSessionBoundaries(t *testing.T) {
	cases := []struct {
		name       string
		tradingDay bool
		hour       int
		minute     int
		phase      string
		inSession  bool
		continuous bool
		auction    bool
	}{
		{name: "weekend ignores clock", tradingDay: false, hour: 10, minute: 0, phase: model.MarketSessionNonTradingDay},
		{name: "before opening auction", tradingDay: true, hour: 9, minute: 14, phase: model.MarketSessionPreOpen},
		{name: "opening auction start", tradingDay: true, hour: 9, minute: 15, phase: model.MarketSessionOpeningAuction, inSession: true, auction: true},
		{name: "opening auction end", tradingDay: true, hour: 9, minute: 25, phase: model.MarketSessionAuctionPause},
		{name: "continuous morning start", tradingDay: true, hour: 9, minute: 30, phase: model.MarketSessionContinuousAM, inSession: true, continuous: true},
		{name: "morning session end", tradingDay: true, hour: 11, minute: 30, phase: model.MarketSessionLunchBreak},
		{name: "afternoon session start", tradingDay: true, hour: 13, minute: 0, phase: model.MarketSessionContinuousPM, inSession: true, continuous: true},
		{name: "closing auction start", tradingDay: true, hour: 14, minute: 57, phase: model.MarketSessionClosingAuction, inSession: true, auction: true},
		{name: "closing auction end", tradingDay: true, hour: 15, minute: 0, phase: model.MarketSessionClosed},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			phase, inSession, continuous, auction := scheduledMarketSession(tc.tradingDay, tc.hour, tc.minute)
			if phase != tc.phase || inSession != tc.inSession || continuous != tc.continuous || auction != tc.auction {
				t.Fatalf("scheduledMarketSession(%v,%02d:%02d)=(%q,%v,%v,%v), want (%q,%v,%v,%v)",
					tc.tradingDay, tc.hour, tc.minute, phase, inSession, continuous, auction,
					tc.phase, tc.inSession, tc.continuous, tc.auction)
			}
		})
	}
}

// marketDataFixedClock 为时间相关测试提供确定的当前时刻。
type marketDataFixedClock struct {
	now time.Time
}

func (c marketDataFixedClock) Now() time.Time { return c.now }

type marketSessionCalendarProvider struct {
	snapshot *model.TradingCalendarSnapshot
}

func (*marketSessionCalendarProvider) Name() string { return "market-session-test" }

func (p *marketSessionCalendarProvider) FetchTradingCalendar(context.Context, string, string) (*model.TradingCalendarSnapshot, error) {
	return p.snapshot, nil
}

func TestMarketSessionUsesInjectedClock(t *testing.T) {
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 22, 10, 0, 0, 0, location)
	calendarProvider := &marketSessionCalendarProvider{snapshot: &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: "2026-10-22", EndDate: "2026-10-22",
		TotalDays: 1, TradingDays: 1,
		Items:  []model.TradingCalendarDay{{Date: "2026-10-22", IsTradingDay: true}},
		Source: "market-session-test", SourceVersion: "fixture-v1",
		FetchedAt: now.UTC(), QualityStatus: model.QualityValid,
	}}
	svc := NewWithCapabilitiesAndClock(
		[]provider.NamedProvider{calendarProvider}, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil,
		marketDataFixedClock{now: now},
	)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{calendarProvider})
	got, err := svc.MarketSession(context.Background(), now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.ServerTime.Equal(now.UTC()) || !got.FetchedAt.Equal(now.UTC()) {
		t.Fatalf("market-session timestamps must use injected clock: server=%s fetched=%s want=%s", got.ServerTime, got.FetchedAt, now.UTC())
	}
	if got.LocalDate != "2026-10-22" || got.LocalTime != "10:00:00" {
		t.Fatalf("market-session local time must use Asia/Shanghai: date=%s time=%s", got.LocalDate, got.LocalTime)
	}
}
