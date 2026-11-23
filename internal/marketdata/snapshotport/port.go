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
