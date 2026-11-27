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

// CreateOrderBookFactsSnapshot 创建并授权一个不可变的日内盘口事实快照。
// 盘口来自 Data 的真实 Provider，不读取 PIT 日线，也不接受调用方传入的数值。
func (m *Module) CreateOrderBookFactsSnapshot(
	ctx context.Context, symbol string, systemAsOf time.Time, purpose string,
) (application.OrderBookFactsSnapshotReferences, error) {
	if m == nil || m.store == nil || m.orderBook == nil {
		return application.OrderBookFactsSnapshotReferences{}, errors.New("Market Data order-book facts Snapshot persistence is unavailable")
	}
	owner, err := application.NewDataAuthorizationOwner(m.store, m.store)
	if err != nil {
		return application.OrderBookFactsSnapshotReferences{}, err
	}
	creator, err := application.NewOrderBookFactsSnapshotCreator(m.orderBook, m.store, owner)
	if err != nil {
		return application.OrderBookFactsSnapshotReferences{}, err
	}
	return creator.Create(ctx, symbol, systemAsOf, purpose)
}

// ReadAuthorizedOrderBookFactsSnapshot 使用盘口数据类策略做授权复核，
// 与交易事实共用同一事务边界和 fail-closed 语义。
func (m *Module) ReadAuthorizedOrderBookFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.OrderBookFactsSnapshot, error) {
	if m == nil || m.store == nil {
		return app.OrderBookFactsSnapshot{}, errors.New("Market Data Snapshot persistence is unavailable")
	}
	reader, err := application.NewAuthorizedOrderBookFactsSnapshotReader(m.store)
	if err != nil {
		return app.OrderBookFactsSnapshot{}, err
	}
	return reader.Read(ctx, snapshotRef, authorizationRef, purpose)
}

// CreateLimitPoolFactsSnapshot 创建并授权一个不可变的涨跌停事实快照。
// 只接受全量池；盘中不完整会由 Provider 完整性断言与收盘后约束拒绝。
func (m *Module) CreateLimitPoolFactsSnapshot(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (application.LimitPoolFactsSnapshotReferences, error) {
	if m == nil || m.store == nil || m.limitPool == nil {
		return application.LimitPoolFactsSnapshotReferences{}, errors.New("Market Data limit-pool facts Snapshot persistence is unavailable")
	}
	owner, err := application.NewDataAuthorizationOwner(m.store, m.store)
	if err != nil {
		return application.LimitPoolFactsSnapshotReferences{}, err
	}
	creator, err := application.NewLimitPoolFactsSnapshotCreator(m.limitPool, m.store, owner)
	if err != nil {
		return application.LimitPoolFactsSnapshotReferences{}, err
	}
	return creator.Create(ctx, symbol, tradeDate, systemAsOf, purpose)
}

// PersistLimitPoolFactsSnapshot 产出并持久化涨跌停事实快照，但**不签发授权**。
//
// 这是采集任务/工具在 Run 之前预置事实的入口：授权仍由读取方在需要时签发，
// 因此采集侧不需要 Owner 级凭据。systemAsOf 为零值时以实际观测时刻为快照时点。
func (m *Module) PersistLimitPoolFactsSnapshot(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (string, error) {
	if m == nil || m.store == nil || m.limitPool == nil {
		return "", errors.New("Market Data limit-pool facts Snapshot persistence is unavailable")
	}
	collector, err := application.NewLimitPoolFactsSnapshotCollector(m.limitPool, m.store)
	if err != nil {
		return "", err
	}
	return collector.Persist(ctx, symbol, tradeDate, systemAsOf, purpose)
}

// ReadAuthorizedLimitPoolFactsSnapshot 使用涨跌停数据类策略做授权复核。
func (m *Module) ReadAuthorizedLimitPoolFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (app.LimitPoolFactsSnapshot, error) {
	if m == nil || m.store == nil {
		return app.LimitPoolFactsSnapshot{}, errors.New("Market Data Snapshot persistence is unavailable")
	}
	reader, err := application.NewAuthorizedLimitPoolFactsSnapshotReader(m.store)
	if err != nil {
		return app.LimitPoolFactsSnapshot{}, err
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
