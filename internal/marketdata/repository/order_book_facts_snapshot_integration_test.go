package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
)

func TestMariaDBOrderBookFactsSnapshotExactSystemAsOfReuse(t *testing.T) {
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

	snapshot := validRepositoryOrderBookFactsSnapshot(t)
	if err := store.SaveOrderBookFactsSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("save immutable order-book facts snapshot: %v", err)
	}
	systemAsOf, err := time.Parse(time.RFC3339Nano, snapshot.Manifest.SystemAsOf)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetOrderBookFactsSnapshotForSystemAsOf(
		ctx, snapshot.Manifest.Symbol, snapshot.Manifest.PurposeCode, systemAsOf,
	)
	if err != nil {
		t.Fatalf("exact reuse lookup failed: %v", err)
	}
	if loaded.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef {
		t.Fatalf("reuse lookup returned %s, want %s", loaded.Manifest.SnapshotRef, snapshot.Manifest.SnapshotRef)
	}
	if _, err := store.GetOrderBookFactsSnapshotForSystemAsOf(
		ctx, snapshot.Manifest.Symbol, snapshot.Manifest.PurposeCode, systemAsOf.Add(time.Nanosecond),
	); !errors.Is(err, ErrOrderBookFactsSnapshotNotFound) {
		t.Fatalf("nearby system_as_of must not be reused, got %v", err)
	}
	if _, err := store.GetOrderBookFactsSnapshotForSystemAsOf(
		ctx, snapshot.Manifest.Symbol, "other_purpose", systemAsOf,
	); !errors.Is(err, ErrOrderBookFactsSnapshotNotFound) {
		t.Fatalf("different purpose must not be reused, got %v", err)
	}

	asOf, err := store.GetLatestOrderBookFactsSnapshotAtOrBefore(ctx, snapshot.Manifest.Symbol, "risk_review", systemAsOf)
	if err != nil || asOf.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef {
		t.Fatalf("exact as-of lookup failed: ref=%v err=%v", asOf.Manifest.SnapshotRef, err)
	}
	later, err := store.GetLatestOrderBookFactsSnapshotAtOrBefore(
		ctx, snapshot.Manifest.Symbol, "risk_review", systemAsOf.Add(30*time.Second),
	)
	if err != nil || later.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef {
		t.Fatalf("later evaluation must reuse the earlier snapshot: ref=%v err=%v", later.Manifest.SnapshotRef, err)
	}
	if _, err := store.GetLatestOrderBookFactsSnapshotAtOrBefore(
		ctx, snapshot.Manifest.Symbol, "risk_review", systemAsOf.Add(-time.Minute),
	); !errors.Is(err, ErrOrderBookFactsSnapshotNotFound) {
		t.Fatalf("earlier evaluation must not see a later snapshot, got %v", err)
	}
	if _, err := store.GetLatestOrderBookFactsSnapshotAtOrBefore(
		ctx, snapshot.Manifest.Symbol, "other_purpose", systemAsOf,
	); !errors.Is(err, ErrOrderBookFactsSnapshotNotFound) {
		t.Fatalf("as-of lookup with a different purpose must miss, got %v", err)
	}
}

// 盘口事实快照必须与 close-series / 交易状态事实共用同一张不可变存储，
// 并按 payload_kind 精确隔离。该测试只在配置真实 MariaDB 时运行。
func TestMariaDBOrderBookFactsSnapshotSharesImmutableStore(t *testing.T) {
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

	book := validRepositoryOrderBookFactsSnapshot(t)
	if err := store.SaveOrderBookFactsSnapshot(ctx, book); err != nil {
		t.Fatalf("save immutable order-book facts snapshot: %v", err)
	}
	if err := store.SaveOrderBookFactsSnapshot(ctx, book); err != nil {
		t.Fatalf("idempotent duplicate save failed: %v", err)
	}
	loaded, err := store.GetOrderBookFactsSnapshot(ctx, book.Manifest.SnapshotRef)
	if err != nil {
		t.Fatalf("read immutable order-book facts snapshot: %v", err)
	}
	if err := loaded.Verify(); err != nil {
		t.Fatalf("loaded order-book snapshot failed Verify(): %v", err)
	}
	if loaded.Payload.TradableVolumeShares != 3600 || loaded.Payload.BidLevels != 5 {
		t.Fatalf("loaded order-book fact differs from the saved content: %+v", loaded.Payload)
	}

	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, book.Manifest.SnapshotRef); !errors.Is(err, ErrDailyBarsCloseSeriesSnapshotNotFound) {
		t.Fatalf("close-series reader must not read an order-book row, got %v", err)
	}
	if _, err := store.GetSecurityTradingStatusFactsSnapshot(ctx, book.Manifest.SnapshotRef); !errors.Is(err, ErrSecurityTradingStatusFactsSnapshotNotFound) {
		t.Fatalf("trading-status reader must not read an order-book row, got %v", err)
	}
	closeSeries := validRepositoryDailyBarsCloseSeriesSnapshot(t)
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, closeSeries); err != nil {
		t.Fatalf("save close-series snapshot: %v", err)
	}
	if _, err := store.GetOrderBookFactsSnapshot(ctx, closeSeries.Manifest.SnapshotRef); !errors.Is(err, ErrOrderBookFactsSnapshotNotFound) {
		t.Fatalf("order-book reader must not read a close-series row, got %v", err)
	}

	// 直接改库改变可成交量必须被指纹与规范 JSON 复核发现。
	if _, err := store.db.ExecContext(ctx, `UPDATE market_data_daily_bars_close_series_snapshots
SET payload_json = REPLACE(payload_json, '"tradable_volume_shares":3600', '"tradable_volume_shares":9999')
WHERE snapshot_ref = ? AND payload_kind = ?`, book.Manifest.SnapshotRef, app.OrderBookFactsPayloadKind); err != nil {
		t.Fatalf("tamper persisted order-book payload: %v", err)
	}
	if _, err := store.GetOrderBookFactsSnapshot(ctx, book.Manifest.SnapshotRef); err == nil {
		t.Fatal("tampered order-book payload was accepted")
	}
}
