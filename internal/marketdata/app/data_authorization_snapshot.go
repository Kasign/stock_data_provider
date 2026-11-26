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

// DataAuthorizedTradingFactsRepository 在同一只读事务中读取授权决策、撤销状态、
// 交易事实数据类的当前策略和交易状态事实快照。
//
// 与 close-series 分开定义，使任一 Payload 的读取实现都不必被迫实现另一种；
// 也避免"只实现了日线读取"的仓储被误用于交易事实授权。
type DataAuthorizedTradingFactsRepository interface {
	ReadDataAuthorizedSecurityTradingStatusFactsSnapshot(
		context.Context, string, string,
	) (DataAuthorizationDecision, SecurityTradingStatusFactsSnapshot, authorization.CurrentPolicy, error)
}
