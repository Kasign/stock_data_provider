package service

import (
	"context"
	"os"
	"testing"
	"time"

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
}
