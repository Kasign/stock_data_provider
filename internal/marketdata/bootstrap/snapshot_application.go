package bootstrap

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/application"
)

// CreateDailyBarsCloseSeriesSnapshot 创建并授权一个不可变的首批 SMA 输入 Snapshot。
// Module 未配置持久化 Store 时明确失败，不会降级成内存 Snapshot。
func (m *Module) CreateDailyBarsCloseSeriesSnapshot(
	ctx context.Context, query app.DailyBarsQuery, purpose string,
) (application.DailyBarsCloseSeriesSnapshotReferences, error) {
	if m == nil || m.store == nil || m.application == nil {
		return application.DailyBarsCloseSeriesSnapshotReferences{}, errors.New("Market Data Snapshot persistence is unavailable")
	}
	reader, ok := m.application.(app.DailyBarsV2Reader)
	if !ok {
		return application.DailyBarsCloseSeriesSnapshotReferences{}, errors.New("Market Data V2 Reader is unavailable")
	}
	owner, err := application.NewDataAuthorizationOwner(m.store, m.store)
	if err != nil {
		return application.DailyBarsCloseSeriesSnapshotReferences{}, err
	}
	creator, err := application.NewDailyBarsCloseSeriesSnapshotCreator(
		reader, m.store, owner,
	)
	if err != nil {
		return application.DailyBarsCloseSeriesSnapshotReferences{}, err
	}
	return creator.Create(ctx, query, purpose)
}

// ReadAuthorizedDailyBarsCloseSeriesSnapshot 仅在授权、撤销、当前策略和 Snapshot
// 同一数据库一致性事务检查通过后返回 Payload。
func (m *Module) ReadAuthorizedDailyBarsCloseSeriesSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.DailyBarsCloseSeriesSnapshot, error) {
	if m == nil || m.store == nil {
		return app.DailyBarsCloseSeriesSnapshot{}, errors.New("Market Data Snapshot persistence is unavailable")
	}
	reader, err := application.NewAuthorizedDailyBarsCloseSeriesSnapshotReader(m.store)
	if err != nil {
		return app.DailyBarsCloseSeriesSnapshot{}, err
	}
	return reader.Read(ctx, snapshotRef, authorizationRef, purpose)
}

// CreateSecurityTradingStatusFactsSnapshot 创建并授权一个不可变的交易状态事实快照。
// PIT 来源缺失或持久化未配置时明确失败，不会降级成实时抓取或内存快照。
func (m *Module) CreateSecurityTradingStatusFactsSnapshot(
	ctx context.Context,
	symbol, tradeDate string,
	systemAsOf time.Time,
	purpose string,
) (application.SecurityTradingStatusFactsSnapshotReferences, error) {
	if m == nil || m.store == nil || m.securityTradingStatus == nil {
		return application.SecurityTradingStatusFactsSnapshotReferences{}, errors.New("Market Data trading-status facts Snapshot persistence is unavailable")
	}
	owner, err := application.NewDataAuthorizationOwner(m.store, m.store)
	if err != nil {
		return application.SecurityTradingStatusFactsSnapshotReferences{}, err
	}
	creator, err := application.NewSecurityTradingStatusFactsSnapshotCreator(
		m.securityTradingStatus, m.store, owner,
	)
	if err != nil {
		return application.SecurityTradingStatusFactsSnapshotReferences{}, err
	}
	return creator.Create(ctx, symbol, tradeDate, systemAsOf, purpose)
}

// ReadAuthorizedSecurityTradingStatusFactsSnapshot 使用交易事实数据类的策略做授权复核，
// 与 close-series 共用同一事务边界与 fail-closed 语义。
func (m *Module) ReadAuthorizedSecurityTradingStatusFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.SecurityTradingStatusFactsSnapshot, error) {
	if m == nil || m.store == nil {
		return app.SecurityTradingStatusFactsSnapshot{}, errors.New("Market Data Snapshot persistence is unavailable")
	}
	reader, err := application.NewAuthorizedSecurityTradingStatusFactsSnapshotReader(m.store)
	if err != nil {
		return app.SecurityTradingStatusFactsSnapshot{}, err
	}
	return reader.Read(ctx, snapshotRef, authorizationRef, purpose)
}

// DataAuthorizationPolicyOwner 返回 Data 自有的 Owner 策略管理 Application。
// Root Server 只持有该 Application Port，不直接操作策略表或复制授权规则。
func (m *Module) DataAuthorizationPolicyOwner() (*application.DataAuthorizationPolicyOwner, error) {
	if m == nil || m.store == nil {
		return nil, errors.New("Market Data authorization policy storage is unavailable")
	}
	return application.NewDataAuthorizationPolicyOwner(m.store)
}
