package bootstrap

import (
	"context"
	"errors"

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

// DataAuthorizationPolicyOwner 返回 Data 自有的 Owner 策略管理 Application。
// Root Server 只持有该 Application Port，不直接操作策略表或复制授权规则。
func (m *Module) DataAuthorizationPolicyOwner() (*application.DataAuthorizationPolicyOwner, error) {
	if m == nil || m.store == nil {
		return nil, errors.New("Market Data authorization policy storage is unavailable")
	}
	return application.NewDataAuthorizationPolicyOwner(m.store)
}
