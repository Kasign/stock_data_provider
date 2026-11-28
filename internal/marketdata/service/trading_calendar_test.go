package service

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
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

func TestDailyBarCoverageHistoricalTimestampOnlyCalendarGapIsInformational(t *testing.T) {
	p := &testTradingCalendarProvider{snapshot: validTestTradingCalendar()}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{p})

	ok, warnings, evidence := svc.dailyBarCoverageEvidence(context.Background(), "2026-10-22", "2026-10-24", 2, 2)
	if !ok {
		t.Fatalf("complete historical date coverage should pass: warnings=%v", warnings)
	}
	if evidence.CalendarRef != "" || evidence.CoverageBasis != "exchange_calendar_unreferenced" ||
		evidence.CalendarQualityStatus != model.QualityDegraded {
		t.Fatalf("calendar without durable reference must be represented as unreferenced evidence: %+v", evidence)
	}
	for _, warning := range warnings {
		if warning == "daily_bar_coverage_uses_degraded_trading_calendar" {
			t.Fatalf("missing historical publication timestamp alone must not degrade complete calendar dates: %v", warnings)
		}
	}
}

// 历史交易日历日期是稳定事实：当唯一质量缺口是上游未暴露发布时间戳、且快照已有
// 不可变内容引用时，覆盖证据必须把日历质量视为可信，否则 Close Series 准入会因为
// CalendarQualityStatus != valid 而永久拒绝，即使日期覆盖和 bar 质量都合格。
func TestDailyBarCoverageHistoricalTimestampOnlyCalendarWithReferenceIsValid(t *testing.T) {
	calendar := validTestTradingCalendar()
	calendar.CalendarRef = "data_calendar:sha256:0000000000000000000000000000000000000000000000000000000000000000"
	p := &testTradingCalendarProvider{snapshot: calendar}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{p})

	ok, warnings, evidence := svc.dailyBarCoverageEvidence(context.Background(), "2026-10-22", "2026-10-24", 2, 2)
	if !ok {
		t.Fatalf("complete historical coverage should pass: warnings=%v", warnings)
	}
	if evidence.CalendarQualityStatus != model.QualityValid {
		t.Fatalf("historical timestamp-only gap with a durable reference must yield valid calendar coverage, got %q: %+v",
			evidence.CalendarQualityStatus, evidence)
	}
	if evidence.CoverageBasis != "exchange_calendar" || evidence.CalendarRef == "" {
		t.Fatalf("durable reference must be preserved as exchange_calendar evidence: %+v", evidence)
	}
	if !containsTestWarning(warnings, "daily_bar_coverage_calendar_snapshot_time_unverified") {
		t.Fatalf("timestamp gap must remain visible as a context warning: %v", warnings)
	}
	if containsTestWarning(warnings, "daily_bar_coverage_uses_degraded_trading_calendar") {
		t.Fatalf("timestamp-only gap must not degrade calendar coverage: %v", warnings)
	}
	bars := []model.DailyBar{{QualityStatus: model.QualityValid}}
	applyDailyBarCoverageWarnings(bars, warnings)
	if bars[0].QualityStatus != model.QualityValid {
		t.Fatalf("timestamp-only historical gap must not degrade bar quality: %+v", bars[0])
	}
}

// 除时间戳之外还有别的降级原因时，历史日历不能被豁免。
func TestDailyBarCoverageHistoricalCalendarWithOtherReasonStaysDegraded(t *testing.T) {
	calendar := validTestTradingCalendar()
	calendar.CalendarRef = "data_calendar:sha256:0000000000000000000000000000000000000000000000000000000000000000"
	calendar.QualityReasons = []string{"provider_trading_calendar_sources_disagree"}
	p := &testTradingCalendarProvider{snapshot: calendar}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{p})

	_, warnings, evidence := svc.dailyBarCoverageEvidence(context.Background(), "2026-10-22", "2026-10-24", 2, 2)
	if evidence.CalendarQualityStatus != model.QualityDegraded {
		t.Fatalf("non-timestamp degradation must stay degraded, got %q: %+v", evidence.CalendarQualityStatus, evidence)
	}
	if !containsTestWarning(warnings, "daily_bar_coverage_uses_degraded_trading_calendar") {
		t.Fatalf("non-timestamp degradation must keep the degraded warning: %v", warnings)
	}
}

func TestDailyBarCoverageAsOfDoesNotConsultLiveCalendarProvider(t *testing.T) {
	p := &testTradingCalendarProvider{snapshot: validTestTradingCalendar()}
	svc := NewWithCapabilities([]provider.NamedProvider{p}, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{p})

	asOf := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	ok, warnings := svc.dailyBarCoverageSufficientAsOf(context.Background(), "2026-10-22", "2026-10-24", 2, 2, asOf)
	if !ok {
		t.Fatalf("weekday estimate should preserve the existing coverage calculation: warnings=%v", warnings)
	}
	if p.calls != 0 {
		t.Fatalf("PIT coverage must not call current calendar Providers, calls=%d", p.calls)
	}
	found := false
	for _, warning := range warnings {
		if warning == "daily_bar_coverage_calendar_not_observed_by_as_of" {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing explicit as-of calendar gap warning: %v", warnings)
	}
	bars := []model.DailyBar{{QualityStatus: model.QualityValid}}
	applyDailyBarCoverageWarnings(bars, warnings)
	if bars[0].QualityStatus != model.QualityDegraded ||
		!dailyBarsPITContainsQualityReason(bars[0].QualityReasons, "daily_bar_coverage_calendar_not_observed_by_as_of") {
		t.Fatalf("missing as-of calendar must degrade the PIT bar series: %+v", bars[0])
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

func TestTradingCalendarRejectsMismatchedProviderSourceAndContinues(t *testing.T) {
	mismatchedSnapshot := validTestTradingCalendar()
	mismatchedSnapshot.Source = "spoofed-source"
	fallbackSnapshot := validTestTradingCalendar()
	fallbackSnapshot.Source = "calendar-fallback"
	preferred := &testTradingCalendarProvider{name: "calendar-preferred", snapshot: mismatchedSnapshot}
	fallback := &testTradingCalendarProvider{name: "calendar-fallback", snapshot: fallbackSnapshot}
	svc := NewWithCapabilities([]provider.NamedProvider{preferred, fallback}, nil, nil,
		cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{preferred, fallback})

	got, hit, err := svc.TradingCalendar(context.Background(), "2026-10-22", "2026-10-24")
	if err != nil || hit || got == nil || got.Source != "calendar-fallback" {
		t.Fatalf("mismatched trading-calendar source must be rejected: hit=%v got=%+v err=%v", hit, got, err)
	}
	if preferred.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected both configured providers to be queried, calls=%d/%d", preferred.calls, fallback.calls)
	}
}
