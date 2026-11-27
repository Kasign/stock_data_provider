package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
)

func validRepositoryLimitPoolFactsSnapshot(t *testing.T) app.LimitPoolFactsSnapshot {
	t.Helper()
	fetchedAt := time.Date(2026, 11, 27, 7, 1, 0, 0, time.UTC)
	up := &model.LimitPoolSnapshot{
		PoolType: "up", TradeDate: "2026-11-27", Total: 0, HasData: true,
		Source: "eastmoney", FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}
	down := &model.LimitPoolSnapshot{
		PoolType: "down", TradeDate: "2026-11-27", Total: 0, HasData: true,
		Source: "eastmoney", FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}
	payload, err := app.BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatalf("build limit-pool payload: %v", err)
	}
	sources := []app.DailyBarsCloseSeriesSource{{SourceRef: "eastmoney"}}
	snapshot, err := app.BuildLimitPoolFactsSnapshot(
		payload, sources, "service:quant", "risk_review", fetchedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("build limit-pool snapshot: %v", err)
	}
	return snapshot
}

func TestMariaDBLimitPoolFactsSnapshotRoundTripAndExactReuse(t *testing.T) {
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

	snapshot := validRepositoryLimitPoolFactsSnapshot(t)
	if err := store.SaveLimitPoolFactsSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("save immutable limit-pool facts snapshot: %v", err)
	}
	loaded, err := store.GetLimitPoolFactsSnapshot(ctx, snapshot.Manifest.SnapshotRef)
	if err != nil {
		t.Fatalf("read back limit-pool facts snapshot: %v", err)
	}
	if loaded.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef ||
		loaded.Payload.TradeDate != "2026-11-27" || loaded.Payload.Symbol != "600036.SH" {
		t.Fatalf("round trip mismatch: %+v", loaded.Manifest)
	}
	if err := loaded.Verify(); err != nil {
		t.Fatalf("round-tripped snapshot failed verification: %v", err)
	}

	systemAsOf, err := time.Parse(time.RFC3339Nano, snapshot.Manifest.SystemAsOf)
	if err != nil {
		t.Fatal(err)
	}
	reused, err := store.GetLimitPoolFactsSnapshotForSystemAsOf(
		ctx, "600036.SH", "2026-11-27", "risk_review", systemAsOf,
	)
	if err != nil {
		t.Fatalf("exact reuse lookup failed: %v", err)
	}
	if reused.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef {
		t.Fatalf("reuse lookup returned %s, want %s", reused.Manifest.SnapshotRef, snapshot.Manifest.SnapshotRef)
	}
	if _, err := store.GetLimitPoolFactsSnapshotForSystemAsOf(
		ctx, "600036.SH", "2026-11-26", "risk_review", systemAsOf,
	); !errors.Is(err, snapshotport.ErrLimitPoolFactsSnapshotNotFound) {
		t.Fatalf("different trade_date must not be reused, got %v", err)
	}
	if _, err := store.GetLimitPoolFactsSnapshotForSystemAsOf(
		ctx, "600036.SH", "2026-11-27", "risk_review", systemAsOf.Add(time.Nanosecond),
	); !errors.Is(err, snapshotport.ErrLimitPoolFactsSnapshotNotFound) {
		t.Fatalf("nearby system_as_of must not be reused, got %v", err)
	}

	// 跨数据类隔离：同一张共享表按 payload_kind 分流，order_book 读取不得看到 limit_pool。
	if _, err := store.GetOrderBookFactsSnapshot(ctx, snapshot.Manifest.SnapshotRef); !errors.Is(err, ErrOrderBookFactsSnapshotNotFound) {
		t.Fatalf("order-book read of a limit-pool snapshot must fail closed, got %v", err)
	}
}
