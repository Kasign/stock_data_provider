package repository

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
)

func TestDailyBarsCloseSeriesSnapshotStorageDisabledFailsClosed(t *testing.T) {
	snapshot := validRepositoryDailyBarsCloseSeriesSnapshot(t)
	if err := (*Store)(nil).SaveDailyBarsCloseSeriesSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveDailyBarsCloseSeriesSnapshot() succeeded without storage")
	}
	if _, err := (*Store)(nil).GetDailyBarsCloseSeriesSnapshot(context.Background(), snapshot.Manifest.SnapshotRef); err == nil {
		t.Fatal("GetDailyBarsCloseSeriesSnapshot() succeeded without storage")
	}
	if err := (&Store{}).SaveDailyBarsCloseSeriesSnapshot(context.Background(), snapshot); err == nil {
		t.Fatal("SaveDailyBarsCloseSeriesSnapshot() succeeded with an unconfigured Store")
	}
}

func TestDailyBarsCloseSeriesSnapshotRefRejectsMalformedValues(t *testing.T) {
	for _, value := range []string{"", "data_snapshot:sha256:xyz", "data_snapshot:sha256:" + strings.Repeat("A", 64)} {
		if _, err := parseDailyBarsCloseSeriesSnapshotRef(value); err == nil {
			t.Fatalf("malformed SnapshotRef was accepted: %q", value)
		}
	}
}

func TestMariaDBDailyBarsCloseSeriesSnapshotIsImmutableAndVerifiable(t *testing.T) {
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
	if err := store.Migrate(ctx); err != nil {
		t.Fatalf("migrate Data schema: %v", err)
	}
	if err := store.ValidateCompatibilitySchema(ctx); err != nil {
		t.Fatalf("validate physical Data schema: %v", err)
	}

	snapshot := validRepositoryDailyBarsCloseSeriesSnapshot(t)
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("save immutable close-series snapshot: %v", err)
	}
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("idempotent duplicate save failed: %v", err)
	}
	loaded, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, snapshot.Manifest.SnapshotRef)
	if err != nil {
		t.Fatalf("read immutable close-series snapshot: %v", err)
	}
	if err := loaded.Verify(); err != nil {
		t.Fatalf("loaded snapshot failed Verify(): %v", err)
	}
	if loaded.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef ||
		loaded.Manifest.ManifestFingerprint != snapshot.Manifest.ManifestFingerprint ||
		loaded.PayloadFingerprint != snapshot.PayloadFingerprint ||
		loaded.Payload.Bars[0].Close != snapshot.Payload.Bars[0].Close {
		t.Fatalf("loaded Snapshot differs from the saved content: %+v", loaded)
	}

	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, "data_snapshot:sha256:"+strings.Repeat("0", 64)); !errors.Is(err, ErrDailyBarsCloseSeriesSnapshotNotFound) {
		t.Fatalf("unknown SnapshotRef must return not-found, got %v", err)
	}
	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, "data_snapshot:sha256:invalid"); err == nil {
		t.Fatal("malformed SnapshotRef must be rejected")
	}

	// Concurrent duplicate writes must converge on the same immutable record.
	const writers = 4
	start := make(chan struct{})
	errs := make(chan error, writers)
	var wait sync.WaitGroup
	for index := 0; index < writers; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			errs <- store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot)
		}()
	}
	close(start)
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent idempotent Snapshot save failed: %v", err)
		}
	}

	manifestJSON, payloadJSON, err := canonicalDailyBarsCloseSeriesSnapshotJSON(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE market_data_daily_bars_close_series_snapshots SET payload_json='{}' WHERE snapshot_ref=?", snapshot.Manifest.SnapshotRef); err != nil {
		t.Fatalf("tamper with stored Payload: %v", err)
	}
	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, snapshot.Manifest.SnapshotRef); err == nil {
		t.Fatal("tampered Payload must fail content verification")
	}
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, snapshot); err == nil {
		t.Fatal("Save must not overwrite a conflicting/tampered immutable record")
	}
	if _, err := store.db.ExecContext(ctx, "UPDATE market_data_daily_bars_close_series_snapshots SET manifest_json=?, payload_json=? WHERE snapshot_ref=?", string(manifestJSON), string(payloadJSON), snapshot.Manifest.SnapshotRef); err != nil {
		t.Fatalf("restore stored Snapshot after integrity test: %v", err)
	}
}

func validRepositoryDailyBarsCloseSeriesSnapshot(t *testing.T) app.DailyBarsCloseSeriesSnapshot {
	t.Helper()
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	observed := cutoff.Add(-time.Minute)
	fractionFour, fractionThree := 4, 3
	query := app.DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-11-17", EndDate: "2026-11-18",
		Limit: 2, AdjustmentBasis: app.AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	}
	snapshot := app.DailyBarsV2Snapshot{
		Symbol: "600036.SH", Interval: "1d", AdjustmentBasis: app.AdjustmentNone,
		QualityStatus: app.QualityValid, SystemObservedAtCutoff: &cutoff,
		TradingCalendarRef: repositoryStringPointer("data_calendar:sha256:" + strings.Repeat("a", 64)),
		CoverageEvidence: app.DailyBarCoverageEvidence{
			CalendarQualityStatus: app.QualityValid, ExpectedTradingDays: 2, MinimumRequiredBars: 2,
			CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
		Bars: []app.DailyBar{
			repositoryCloseSeriesBar("2026-11-17", "43.49", "43.4900", &fractionFour, cutoff, observed),
			repositoryCloseSeriesBar("2026-11-18", "43.12", "43.120", &fractionThree, cutoff, observed),
		},
	}
	input, err := app.BuildDailyBarsCloseSeriesInput(query, snapshot)
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesInput() error = %v", err)
	}
	result, err := app.BuildDailyBarsCloseSeriesSnapshot(input, "principal:repository-test", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() error = %v", err)
	}
	return result
}

func repositoryCloseSeriesBar(tradeDate, closeValue, sourceToken string, fractionDigits *int, cutoff, observed time.Time) app.DailyBar {
	return app.DailyBar{
		Symbol: "600036.SH", TradeDate: tradeDate, Period: "1d", AdjustmentBasis: app.AdjustmentNone,
		Source: "eastmoney", SourceVersion: "eastmoney-stock-api-v1",
		FetchedAt: cutoff.Add(-2 * time.Minute), NormalizedAt: cutoff.Add(-time.Minute),
		SystemObservedAt: &observed, NormalizationVersion: "4", QualityStatus: app.QualityValid,
		ExactValues: &app.DailyBarExactValues{
			Close: closeValue,
			CloseEvidence: &app.DailyBarDecimalEvidence{
				EvidenceKind: "observed_token", SourceToken: sourceToken, SourceFractionDigits: fractionDigits,
			},
		},
	}
}

func repositoryStringPointer(value string) *string { return &value }
