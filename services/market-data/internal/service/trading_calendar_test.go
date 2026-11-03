package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type tradingCalendarTestCache struct {
	items map[string][]byte
}

func (c *tradingCalendarTestCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, ok := c.items[key]
	return value, ok, nil
}

func (c *tradingCalendarTestCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	if c.items == nil {
		c.items = map[string][]byte{}
	}
	c.items[key] = append([]byte(nil), value...)
	return nil
}

func (*tradingCalendarTestCache) Ping(context.Context) error { return nil }

type testTradingCalendarProvider struct {
	name     string
	snapshot *model.TradingCalendarSnapshot
	err      error
	calls    int
}

func (p *testTradingCalendarProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "calendar-test"
}

func (p *testTradingCalendarProvider) FetchTradingCalendar(context.Context, string, string) (*model.TradingCalendarSnapshot, error) {
	p.calls++
	return p.snapshot, p.err
}

func validTestTradingCalendar() *model.TradingCalendarSnapshot {
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: "2026-10-22", EndDate: "2026-10-24",
		TotalDays: 3, TradingDays: 2,
		Items: []model.TradingCalendarDay{
			{Date: "2026-10-22", IsTradingDay: true},
			{Date: "2026-10-23", IsTradingDay: true},
			{Date: "2026-10-24", IsTradingDay: false},
		},
		Source: "calendar-test", SourceVersion: "fixture-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
}

func TestTradingCalendarRejectsMissingSourceVersion(t *testing.T) {
	snapshot := validTestTradingCalendar()
	snapshot.SourceVersion = ""
	if err := validateTradingCalendarSnapshot(snapshot, snapshot.StartDate, snapshot.EndDate); err == nil {
		t.Fatal("trading-calendar snapshot without source version must be rejected")
	}
}

func TestTradingCalendarRequiresCompleteExplicitDateCoverage(t *testing.T) {
	p := &testTradingCalendarProvider{snapshot: validTestTradingCalendar()}
	testCache := &tradingCalendarTestCache{items: map[string][]byte{}}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, testCache, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{p})
	got, hit, err := svc.TradingCalendar(context.Background(), "2026-10-22", "2026-10-24")
	if err != nil || hit {
		t.Fatalf("calendar failed on first fetch: hit=%v err=%v", hit, err)
	}
	if got.TotalDays != 3 || got.TradingDays != 2 || len(got.Items) != 3 {
		t.Fatalf("unexpected calendar: %+v", got)
	}
	_, hit, err = svc.TradingCalendar(context.Background(), "2026-10-22", "2026-10-24")
	if err != nil || !hit || p.calls != 1 {
		t.Fatalf("expected cache hit, hit=%v calls=%d err=%v", hit, p.calls, err)
	}
}

func TestTradingCalendarRejectsMissingCalendarDate(t *testing.T) {
	snapshot := validTestTradingCalendar()
	snapshot.Items = []model.TradingCalendarDay{snapshot.Items[0], snapshot.Items[2]}
	p := &testTradingCalendarProvider{snapshot: snapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{p})
	if _, _, err := svc.TradingCalendar(context.Background(), "2026-10-22", "2026-10-24"); err == nil {
		t.Fatal("missing date must not be interpreted as a non-trading day")
	}
}

func TestTradingCalendarRejectsExcessiveOrReversedRange(t *testing.T) {
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	for _, tc := range []struct{ start, end string }{
		{"2026-10-25", "2026-10-22"},
		{"2000-01-01", "2026-10-23"},
		{"not-a-date", "2026-10-23"},
	} {
		if _, _, err := svc.TradingCalendar(context.Background(), tc.start, tc.end); err == nil {
			t.Fatalf("expected range %q..%q to fail", tc.start, tc.end)
		}
	}
}

func TestTradingCalendarPrefersExplicitFullDateSource(t *testing.T) {
	inferred := validTestTradingCalendar()
	inferred.Source = "inferred-calendar"
	inferred.QualityReasons = []string{"provider_returns_trading_days_only_non_trading_dates_inferred"}
	complete := validTestTradingCalendar()
	complete.Source = "complete-calendar"
	complete.QualityReasons = []string{"provider_snapshot_timestamp_not_exposed"}
	first := &testTradingCalendarProvider{name: "inferred-calendar", snapshot: inferred}
	second := &testTradingCalendarProvider{name: "complete-calendar", snapshot: complete}
	svc := NewWithCapabilities([]provider.NamedProvider{first, second}, nil, nil,
		cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{first, second})

	got, hit, err := svc.TradingCalendar(context.Background(), "2026-10-22", "2026-10-24")
	if err != nil || hit {
		t.Fatalf("calendar selection failed: hit=%v err=%v", hit, err)
	}
	if got.Source != "complete-calendar" {
		t.Fatalf("expected explicit full-date source, got %q", got.Source)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected to compare configured sources, calls=%d/%d", first.calls, second.calls)
	}
}

func TestTradingCalendarMarksConflictingSourcesDegraded(t *testing.T) {
	firstSnapshot := validTestTradingCalendar()
	firstSnapshot.Source = "calendar-a"
	secondSnapshot := validTestTradingCalendar()
	secondSnapshot.Source = "calendar-b"
	secondSnapshot.Items[1].IsTradingDay = false
	secondSnapshot.TradingDays = 1
	first := &testTradingCalendarProvider{name: "calendar-a", snapshot: firstSnapshot}
	second := &testTradingCalendarProvider{name: "calendar-b", snapshot: secondSnapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{first, second}, nil, nil,
		cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{first, second})

	got, hit, err := svc.TradingCalendar(context.Background(), "2026-10-22", "2026-10-24")
	if err != nil || hit {
		t.Fatalf("calendar conflict flow failed: hit=%v err=%v", hit, err)
	}
	if got.QualityStatus != model.QualityDegraded {
		t.Fatalf("conflicting sources must be degraded, got %s", got.QualityStatus)
	}
	for _, reason := range got.QualityReasons {
		if reason == "provider_trading_calendar_sources_disagree" {
			return
		}
	}
	t.Fatalf("conflict reason is missing: %v", got.QualityReasons)
}
