package snapshotport

import (
	"context"

	"example.com/stock-analysis-flow/internal/marketdata/app"
)

// DailyBarsCloseSeriesSnapshotRepository 是 Data 模块内部的持久化 Port。
// 它不属于跨模块公开 Application API。
type DailyBarsCloseSeriesSnapshotRepository interface {
	SaveDailyBarsCloseSeriesSnapshot(ctx context.Context, snapshot app.DailyBarsCloseSeriesSnapshot) error
	GetDailyBarsCloseSeriesSnapshot(ctx context.Context, snapshotRef string) (app.DailyBarsCloseSeriesSnapshot, error)
}

// SecurityTradingStatusFactsSnapshotRepository 是交易状态事实快照的持久化 Port。
// 它与 close-series 共用同一张不可变快照存储，由 payload_kind 分流；
// 两个 Port 分开定义，使任一种 Payload 的存储实现都不会被迫依赖另一种。
type SecurityTradingStatusFactsSnapshotRepository interface {
	SaveSecurityTradingStatusFactsSnapshot(ctx context.Context, snapshot app.SecurityTradingStatusFactsSnapshot) error
	GetSecurityTradingStatusFactsSnapshot(ctx context.Context, snapshotRef string) (app.SecurityTradingStatusFactsSnapshot, error)
}
