package snapshotport

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
)

// ErrOrderBookFactsSnapshotNotFound 表示没有可精确复用的盘口事实快照；
// 调用方可以继续走真实 Provider，但不能把其他时点的事实当成当前事实。
var ErrOrderBookFactsSnapshotNotFound = errors.New("order-book facts snapshot not found for exact binding")

// ErrLimitPoolFactsSnapshotNotFound 表示没有可精确复用的涨跌停事实快照。
var ErrLimitPoolFactsSnapshotNotFound = errors.New("limit-pool facts snapshot not found for exact binding")

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

// OrderBookFactsSnapshotRepository 是日内盘口授权快照的持久化 Port。
// 它与前两类共用同一张不可变快照表，由 payload_kind 分流。
type OrderBookFactsSnapshotRepository interface {
	SaveOrderBookFactsSnapshot(ctx context.Context, snapshot app.OrderBookFactsSnapshot) error
	GetOrderBookFactsSnapshot(ctx context.Context, snapshotRef string) (app.OrderBookFactsSnapshot, error)
	// GetOrderBookFactsSnapshotForSystemAsOf 精确复用同一证券、用途与评估截止时刻的
	// 已授权盘口事实；不按时间范围近似匹配，避免把旧盘口伪装成当前事实。
	GetOrderBookFactsSnapshotForSystemAsOf(
		ctx context.Context, symbol, purpose string, systemAsOf time.Time,
	) (app.OrderBookFactsSnapshot, error)
}

// LimitPoolFactsSnapshotRepository 是涨跌停事实授权快照的持久化 Port。
// 它与前几类共用同一张不可变快照表，由 payload_kind 分流。
type LimitPoolFactsSnapshotRepository interface {
	SaveLimitPoolFactsSnapshot(ctx context.Context, snapshot app.LimitPoolFactsSnapshot) error
	GetLimitPoolFactsSnapshot(ctx context.Context, snapshotRef string) (app.LimitPoolFactsSnapshot, error)
	// GetLimitPoolFactsSnapshotForSystemAsOf 精确复用同一证券、交易日、用途与评估
	// 截止时刻的涨跌停事实；不按时间范围近似匹配。
	GetLimitPoolFactsSnapshotForSystemAsOf(
		ctx context.Context, symbol, tradeDate, purpose string, systemAsOf time.Time,
	) (app.LimitPoolFactsSnapshot, error)
}
