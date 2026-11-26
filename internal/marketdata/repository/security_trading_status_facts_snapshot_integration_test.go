package repository

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

// 交易事实快照必须与 close-series 共用同一张不可变存储，并且按 payload_kind 精确隔离。
// 该测试只在配置了真实 MariaDB 时运行；迁移与物理 Schema 校验都在同一条路径上。
func TestMariaDBSecurityTradingStatusFactsSnapshotSharesImmutableStore(t *testing.T) {
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
	// ValidateSchema 必须能发现 payload_kind 列缺失：否则读取会退化为跨类型串读。
	if err := store.ValidateSchema(ctx); err != nil {
		t.Fatalf("validate physical Data schema: %v", err)
	}

	facts := validRepositoryTradingFactsSnapshot(t)
	if err := store.SaveSecurityTradingStatusFactsSnapshot(ctx, facts); err != nil {
		t.Fatalf("save immutable trading-status facts snapshot: %v", err)
	}
	if err := store.SaveSecurityTradingStatusFactsSnapshot(ctx, facts); err != nil {
		t.Fatalf("idempotent duplicate save failed: %v", err)
	}
	loaded, err := store.GetSecurityTradingStatusFactsSnapshot(ctx, facts.Manifest.SnapshotRef)
	if err != nil {
		t.Fatalf("read immutable trading-status facts snapshot: %v", err)
	}
	if err := loaded.Verify(); err != nil {
		t.Fatalf("loaded facts snapshot failed Verify(): %v", err)
	}
	if loaded.Manifest.SnapshotRef != facts.Manifest.SnapshotRef ||
		loaded.PayloadFingerprint != facts.PayloadFingerprint ||
		loaded.Payload.Facts[0].Value != facts.Payload.Facts[0].Value {
		t.Fatalf("loaded facts Snapshot differs from the saved content: %+v", loaded)
	}

	// 同一引用在另一种类型的读取器下必须是 not found，而不是"读到了但解析失败"。
	if _, err := store.GetDailyBarsCloseSeriesSnapshot(ctx, facts.Manifest.SnapshotRef); !errors.Is(err, ErrDailyBarsCloseSeriesSnapshotNotFound) {
		t.Fatalf("close-series reader must not read a facts row, got %v", err)
	}
	closeSeries := validRepositoryDailyBarsCloseSeriesSnapshot(t)
	if err := store.SaveDailyBarsCloseSeriesSnapshot(ctx, closeSeries); err != nil {
		t.Fatalf("save close-series snapshot: %v", err)
	}
	if _, err := store.GetSecurityTradingStatusFactsSnapshot(ctx, closeSeries.Manifest.SnapshotRef); !errors.Is(err, ErrSecurityTradingStatusFactsSnapshotNotFound) {
		t.Fatalf("facts reader must not read a close-series row, got %v", err)
	}

	// 直接改库也要被发现：读取端重新计算指纹并比对规范 JSON。
	if _, err := store.db.ExecContext(ctx, `UPDATE market_data_daily_bars_close_series_snapshots
SET payload_json = REPLACE(payload_json, '"normal"', '"risk_warning"')
WHERE snapshot_ref = ? AND payload_kind = ?`, facts.Manifest.SnapshotRef, "security_trading_status_facts"); err != nil {
		t.Fatalf("tamper persisted payload: %v", err)
	}
	if _, err := store.GetSecurityTradingStatusFactsSnapshot(ctx, facts.Manifest.SnapshotRef); err == nil {
		t.Fatal("tampered facts payload was accepted")
	}
}
