package application

import (
	"context"

	"example.com/stock-analysis-flow/internal/marketdata/app"
)

// DailyBarsCloseSeriesSnapshotRepository 是 Data Application 的内部持久化 Port。
// 它不属于跨模块公开 Application API；Repository Adapter 负责实现。
type DailyBarsCloseSeriesSnapshotRepository interface {
	SaveDailyBarsCloseSeriesSnapshot(ctx context.Context, snapshot app.DailyBarsCloseSeriesSnapshot) error
	GetDailyBarsCloseSeriesSnapshot(ctx context.Context, snapshotRef string) (app.DailyBarsCloseSeriesSnapshot, error)
}
