package service

import (
	"context"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
)

func TestTradingCalendarPersistsImmutableSnapshotAcrossCacheHits(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	}
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	if err := store.ValidateCompatibilitySchema(ctx); err != nil {
		t.Fatalf("validate Data schema: %v", err)
	}

	calendarProvider := &testTradingCalendarProvider{snapshot: validTestTradingCalendar()}
	calendarCache := &tradingCalendarTestCache{items: map[string][]byte{}}
	svc := NewWithCapabilities([]provider.NamedProvider{calendarProvider}, nil, nil,
		calendarCache, store, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{calendarProvider})

	first, cacheHit, err := svc.TradingCalendar(ctx, "2026-10-22", "2026-10-24")
	if err != nil || cacheHit {
		t.Fatalf("first calendar fetch failed: cache_hit=%v err=%v", cacheHit, err)
	}
	if first.CalendarRef == "" || first.SystemObservedAt == nil {
		t.Fatalf("first calendar fetch must return internal immutable reference: %+v", first)
	}
	loaded, err := store.GetTradingCalendarSnapshot(ctx, first.CalendarRef)
	if err != nil || loaded == nil || loaded.CalendarRef != first.CalendarRef {
		t.Fatalf("persisted calendar cannot be resolved by exact reference: loaded=%+v err=%v", loaded, err)
	}

	second, cacheHit, err := svc.TradingCalendar(ctx, "2026-10-22", "2026-10-24")
	if err != nil || !cacheHit {
		t.Fatalf("second calendar read should use cache: cache_hit=%v err=%v", cacheHit, err)
	}
	if second.CalendarRef != first.CalendarRef || second.SystemObservedAt == nil ||
		!second.SystemObservedAt.Equal(*first.SystemObservedAt) || calendarProvider.calls != 1 {
		t.Fatalf("cache hit must resolve the same immutable calendar without refetching: first=%+v second=%+v calls=%d", first, second, calendarProvider.calls)
	}
	coverageOK, coverageWarnings, coverageEvidence := svc.dailyBarCoverageEvidence(ctx, "2026-10-22", "2026-10-24", 2, 2)
	if !coverageOK || coverageEvidence.CalendarRef != first.CalendarRef ||
		coverageEvidence.CoverageBasis != "exchange_calendar" || coverageEvidence.ExpectedTradingDays != first.TradingDays ||
		coverageEvidence.MinimumRequiredBars != 2 || len(coverageWarnings) != len(coverageEvidence.Warnings) {
		t.Fatalf("coverage evidence must expose the exact calendar used for the denominator: ok=%v warnings=%v evidence=%+v calendar=%+v", coverageOK, coverageWarnings, coverageEvidence, first)
	}
}

func TestTradingCalendarAsOfUsesObservedRepositoryVersionWithoutLiveProvider(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	}
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := repository.OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}

	asOf := time.Now().UTC().Truncate(time.Microsecond).Add(-time.Minute)
	first := validTestTradingCalendar()
	first.StartDate, first.EndDate = "1990-01-02", "1990-01-04"
	first.Items = []model.TradingCalendarDay{
		{Date: "1990-01-02", IsTradingDay: true},
		{Date: "1990-01-03", IsTradingDay: true},
		{Date: "1990-01-04", IsTradingDay: false},
	}
	first.TotalDays, first.TradingDays = 3, 2
	first.FetchedAt = asOf.Add(-2 * time.Minute)
	firstObserved := asOf.Add(-time.Minute)
	firstRef, _, err := store.SaveTradingCalendarSnapshot(ctx, first, firstObserved)
	if err != nil {
		t.Fatalf("save calendar visible at as_of: %v", err)
	}
	second := validTestTradingCalendar()
	second.StartDate, second.EndDate = first.StartDate, first.EndDate
	second.Items = append([]model.TradingCalendarDay(nil), first.Items...)
	second.Items[1].IsTradingDay = false
	second.TradingDays = 1
	second.QualityReasons = append(second.QualityReasons, "future_calendar_revision")
	second.FetchedAt = asOf.Add(-30 * time.Second)
	secondObserved := asOf.Add(time.Second)
	if _, _, err := store.SaveTradingCalendarSnapshot(ctx, second, secondObserved); err != nil {
		t.Fatalf("save calendar newer than as_of: %v", err)
	}

	liveProvider := &testTradingCalendarProvider{snapshot: second}
	svc := NewWithCapabilities([]provider.NamedProvider{liveProvider}, nil, nil,
		&tradingCalendarTestCache{items: map[string][]byte{}}, store, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetTradingCalendarProviders([]provider.TradingCalendarProvider{liveProvider})

	got, err := svc.TradingCalendarAsOf(ctx, first.StartDate, first.EndDate, asOf)
	if err != nil {
		t.Fatalf("read calendar as of historical cutoff: %v", err)
	}
	if got.CalendarRef != firstRef || got.TradingDays != 2 {
		t.Fatalf("service must return the repository version visible at as_of: got=%+v want_ref=%s", got, firstRef)
	}
	if liveProvider.calls != 0 {
		t.Fatalf("TradingCalendarAsOf must not consult live Providers, calls=%d", liveProvider.calls)
	}
	coverageOK, coverageWarnings, coverageEvidence := svc.dailyBarCoverageEvidenceAsOf(ctx, first.StartDate, first.EndDate, 1, 1, asOf)
	if !coverageOK || coverageEvidence.CalendarRef != firstRef ||
		coverageEvidence.ExpectedTradingDays != first.TradingDays ||
		coverageEvidence.CoverageBasis != "exchange_calendar" ||
		len(coverageWarnings) != len(coverageEvidence.Warnings) {
		t.Fatalf("PIT coverage evidence must preserve the exact calendar visible at as_of: ok=%v warnings=%v evidence=%+v expected_ref=%s", coverageOK, coverageWarnings, coverageEvidence, firstRef)
	}
	if liveProvider.calls != 0 {
		t.Fatalf("PIT coverage evaluation must not consult live Providers, calls=%d", liveProvider.calls)
	}
}
