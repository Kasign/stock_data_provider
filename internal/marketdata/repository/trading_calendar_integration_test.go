package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestMariaDBTradingCalendarSnapshotImmutableAndReadable(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("MARKET_DATA_TEST_MYSQL_DSN is not configured")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	store, err := OpenMySQL(ctx, dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	if err := store.ValidateCompatibilitySchema(ctx); err != nil {
		t.Fatalf("validate physical Data schema: %v", err)
	}

	snapshot := validRepositoryTradingCalendar()
	observedAt := time.Date(2026, 10, 25, 1, 0, 1, 123456000, time.UTC)
	ref, firstObserved, err := store.SaveTradingCalendarSnapshot(ctx, snapshot, observedAt)
	if err != nil {
		t.Fatalf("save immutable calendar: %v", err)
	}
	if ref == "" || snapshot.CalendarRef != ref || snapshot.SystemObservedAt == nil {
		t.Fatalf("save must attach immutable ref and system observation: ref=%q snapshot=%+v", ref, snapshot)
	}
	if !firstObserved.Equal(observedAt) {
		t.Fatalf("unexpected first observation time: got=%s want=%s", firstObserved, observedAt)
	}

	duplicate := validRepositoryTradingCalendar()
	duplicate.FetchedAt = duplicate.FetchedAt.Add(time.Hour)
	secondRef, secondObserved, err := store.SaveTradingCalendarSnapshot(ctx, duplicate, observedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("save duplicate calendar content: %v", err)
	}
	if secondRef != ref || !secondObserved.Equal(firstObserved) {
		t.Fatalf("identical calendar content must reuse its original ref/observation: first=%s/%s second=%s/%s", ref, firstObserved, secondRef, secondObserved)
	}

	loaded, err := store.GetTradingCalendarSnapshot(ctx, ref)
	if err != nil {
		t.Fatalf("read immutable calendar: %v", err)
	}
	if loaded.CalendarRef != ref || loaded.SystemObservedAt == nil || !loaded.SystemObservedAt.Equal(firstObserved) ||
		loaded.FetchedAt.UnixMicro() != snapshot.FetchedAt.UTC().Truncate(time.Microsecond).UnixMicro() ||
		loaded.TradingDays != snapshot.TradingDays || len(loaded.Items) != len(snapshot.Items) {
		t.Fatalf("loaded calendar does not match immutable record: %+v", loaded)
	}
	if _, err := store.GetTradingCalendarSnapshot(ctx, tradingCalendarSnapshotRefPrefix+strings.Repeat("0", 64)); !errors.Is(err, ErrTradingCalendarSnapshotNotFound) {
		t.Fatalf("unknown calendar ref must return not-found, got %v", err)
	}
	if _, err := store.GetTradingCalendarSnapshot(ctx, "data_calendar:sha256:invalid"); err == nil {
		t.Fatal("malformed calendar ref must be rejected")
	}
}
