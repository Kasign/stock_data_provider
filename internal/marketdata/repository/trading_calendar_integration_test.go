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
		dsn = os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	}
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
	if err := applyMigrationsForTest(ctx, store); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	if err := store.ValidateSchema(ctx); err != nil {
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

	// Concurrent duplicate writes must converge on the same immutable resource.
	const writers = 4
	refs := make(chan string, writers)
	errs := make(chan error, writers)
	start := make(chan struct{})
	for i := 0; i < writers; i++ {
		go func() {
			<-start
			copy := validRepositoryTradingCalendar()
			ref, _, saveErr := store.SaveTradingCalendarSnapshot(ctx, copy, observedAt.Add(2*time.Hour))
			refs <- ref
			errs <- saveErr
		}()
	}
	close(start)
	for i := 0; i < writers; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent immutable calendar save failed: %v", err)
		}
		if got := <-refs; got != ref {
			t.Fatalf("concurrent duplicate write returned a different ref: got=%s want=%s", got, ref)
		}
	}

	// A payload changed without changing its content address must fail closed.
	_, canonical, _, err := buildTradingCalendarSnapshotPayload(validRepositoryTradingCalendar())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE market_data_trading_calendar_snapshots SET payload_json='{}' WHERE calendar_ref=?", ref); err != nil {
		t.Fatalf("tamper with calendar payload for integrity test: %v", err)
	}
	if _, err := store.GetTradingCalendarSnapshot(ctx, ref); err == nil {
		t.Fatal("tampered calendar payload must fail content verification")
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE market_data_trading_calendar_snapshots SET payload_json=? WHERE calendar_ref=?", string(canonical), ref); err != nil {
		t.Fatalf("restore calendar payload after integrity test: %v", err)
	}
}

func TestMariaDBTradingCalendarSnapshotAsOfNeverReturnsFutureCalendar(t *testing.T) {
	dsn := os.Getenv("MARKET_DATA_TEST_MYSQL_DSN")
	if dsn == "" {
		dsn = os.Getenv("SERVER_RELAY_TEST_MYSQL_DSN")
	}
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
	if err := applyMigrationsForTest(ctx, store); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}

	first := validRepositoryTradingCalendar()
	firstObserved := time.Date(2026, 10, 25, 1, 0, 1, 123456000, time.UTC)
	firstRef, _, err := store.SaveTradingCalendarSnapshot(ctx, first, firstObserved)
	if err != nil {
		t.Fatalf("save first calendar version: %v", err)
	}
	second := validRepositoryTradingCalendar()
	second.Items[1].IsTradingDay = false
	second.TradingDays = 1
	second.QualityReasons = append(second.QualityReasons, "calendar_revision_v2")
	secondObserved := firstObserved.Add(time.Hour)
	secondRef, _, err := store.SaveTradingCalendarSnapshot(ctx, second, secondObserved)
	if err != nil {
		t.Fatalf("save second calendar version: %v", err)
	}
	if firstRef == secondRef {
		t.Fatal("changed calendar facts must have different immutable references")
	}

	before, err := store.GetTradingCalendarSnapshotAsOf(ctx, "cn_a_share", first.StartDate, first.EndDate, firstObserved.Add(-time.Second))
	if !errors.Is(err, ErrTradingCalendarSnapshotNotFound) || before != nil {
		t.Fatalf("calendar not yet observed must be unavailable, snapshot=%+v err=%v", before, err)
	}
	between, err := store.GetTradingCalendarSnapshotAsOf(ctx, "cn_a_share", first.StartDate, first.EndDate, firstObserved.Add(time.Minute))
	if err != nil || between.CalendarRef != firstRef || between.TradingDays != 2 {
		t.Fatalf("as_of between revisions must return first version: snapshot=%+v err=%v", between, err)
	}
	after, err := store.GetTradingCalendarSnapshotAsOf(ctx, "cn_a_share", first.StartDate, first.EndDate, secondObserved.Add(time.Minute))
	if err != nil || after.CalendarRef != secondRef || after.TradingDays != 1 {
		t.Fatalf("as_of after second revision must return second version: snapshot=%+v err=%v", after, err)
	}
}
