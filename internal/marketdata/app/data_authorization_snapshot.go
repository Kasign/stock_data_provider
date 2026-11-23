package app

import (
	"context"

	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

// DataAuthorizedSnapshotRepository 在同一只读事务中读取授权决策、撤销状态、当前策略和 Snapshot。
type DataAuthorizedSnapshotRepository interface {
	ReadDataAuthorizedDailyBarsCloseSeriesSnapshot(
		context.Context, string, string,
	) (DataAuthorizationDecision, DailyBarsCloseSeriesSnapshot, authorization.CurrentPolicy, error)
}
