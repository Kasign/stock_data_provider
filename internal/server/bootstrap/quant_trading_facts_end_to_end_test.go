package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	marketdataapp "example.com/stock-analysis-flow/internal/marketdata/app"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/snapshotport"
	"example.com/stock-analysis-flow/internal/platform/security"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/risk"
)

// 交易事实授权快照的端到端契约测试：创建 → 授权签发 → 授权读取 → 门禁判定，
// 每一步都由生产代码执行；只有 MariaDB 被替换成内存 Store。
//
// 验收标准不是"能放行"，而是"拒绝可追溯"：Data 没有涨跌停与可成交量事实，
// 因此 Verified 必然为 false、门禁必然 reject，且证据引用必须指向真实快照与真实授权。

const (
	tradingFactsSymbol   = "600519.SH"
	tradingFactsDate     = "2026-11-25"
	tradingFactsPurpose  = quantapplication.TradingFactsPurposeCode
	tradingFactsPolicyID = "facts-policy-2026-11"
)

type quantTradingFactsStore struct {
	facts      map[string]marketdataapp.SecurityTradingStatusFactsSnapshot
	orderBooks map[string]marketdataapp.OrderBookFactsSnapshot
	limitPools map[string]marketdataapp.LimitPoolFactsSnapshot
	policies   map[string]authorization.CurrentPolicy
	decisions  map[string]marketdataapp.DataAuthorizationDecision
}

func newQuantTradingFactsStore() *quantTradingFactsStore {
	return &quantTradingFactsStore{
		facts:      map[string]marketdataapp.SecurityTradingStatusFactsSnapshot{},
		orderBooks: map[string]marketdataapp.OrderBookFactsSnapshot{},
		limitPools: map[string]marketdataapp.LimitPoolFactsSnapshot{},
		policies:   map[string]authorization.CurrentPolicy{},
		decisions:  map[string]marketdataapp.DataAuthorizationDecision{},
	}
}

func (store *quantTradingFactsStore) SaveSecurityTradingStatusFactsSnapshot(
	_ context.Context, snapshot marketdataapp.SecurityTradingStatusFactsSnapshot,
) error {
	store.facts[snapshot.Manifest.SnapshotRef] = snapshot
	return nil
}

func (store *quantTradingFactsStore) GetSecurityTradingStatusFactsSnapshot(
	_ context.Context, snapshotRef string,
) (marketdataapp.SecurityTradingStatusFactsSnapshot, error) {
	snapshot, present := store.facts[snapshotRef]
	if !present {
		return marketdataapp.SecurityTradingStatusFactsSnapshot{}, errors.New("trading-status facts Snapshot not found")
	}
	return snapshot, nil
}

func (store *quantTradingFactsStore) SaveOrderBookFactsSnapshot(
	_ context.Context, snapshot marketdataapp.OrderBookFactsSnapshot,
) error {
	store.orderBooks[snapshot.Manifest.SnapshotRef] = snapshot
	return nil
}

func (store *quantTradingFactsStore) GetOrderBookFactsSnapshot(
	_ context.Context, snapshotRef string,
) (marketdataapp.OrderBookFactsSnapshot, error) {
	snapshot, present := store.orderBooks[snapshotRef]
	if !present {
		return marketdataapp.OrderBookFactsSnapshot{}, errors.New("order-book facts Snapshot not found")
	}
	return snapshot, nil
}

func (store *quantTradingFactsStore) GetOrderBookFactsSnapshotForSystemAsOf(
	_ context.Context, symbol, purpose string, systemAsOf time.Time,
) (marketdataapp.OrderBookFactsSnapshot, error) {
	wantAsOf := systemAsOf.UTC().Format(time.RFC3339Nano)
	for _, snapshot := range store.orderBooks {
		if snapshot.Manifest.Symbol == symbol && snapshot.Manifest.PurposeCode == purpose &&
			snapshot.Manifest.SystemAsOf == wantAsOf {
			return snapshot, nil
		}
	}
	return marketdataapp.OrderBookFactsSnapshot{}, snapshotport.ErrOrderBookFactsSnapshotNotFound
}

func (store *quantTradingFactsStore) SaveLimitPoolFactsSnapshot(
	_ context.Context, snapshot marketdataapp.LimitPoolFactsSnapshot,
) error {
	store.limitPools[snapshot.Manifest.SnapshotRef] = snapshot
	return nil
}

func (store *quantTradingFactsStore) GetLimitPoolFactsSnapshot(
	_ context.Context, snapshotRef string,
) (marketdataapp.LimitPoolFactsSnapshot, error) {
	snapshot, present := store.limitPools[snapshotRef]
	if !present {
		return marketdataapp.LimitPoolFactsSnapshot{}, errors.New("limit-pool facts Snapshot not found")
	}
	return snapshot, nil
}

func (store *quantTradingFactsStore) GetLatestLimitPoolFactsSnapshotAtOrBefore(
	_ context.Context, symbol, tradeDate, purpose string, systemAsOf time.Time,
) (marketdataapp.LimitPoolFactsSnapshot, error) {
	wantAsOf := systemAsOf.UTC().Format(time.RFC3339Nano)
	for _, snapshot := range store.limitPools {
		if snapshot.Manifest.Symbol == symbol && snapshot.Manifest.TradeDate == tradeDate &&
			snapshot.Manifest.PurposeCode == purpose && snapshot.Manifest.SystemAsOf == wantAsOf {
			return snapshot, nil
		}
	}
	return marketdataapp.LimitPoolFactsSnapshot{}, snapshotport.ErrLimitPoolFactsSnapshotNotFound
}

func (store *quantTradingFactsStore) ReadDataAuthorizedLimitPoolFactsSnapshot(
	_ context.Context, authorizationRef, snapshotRef string,
) (marketdataapp.DataAuthorizationDecision, marketdataapp.LimitPoolFactsSnapshot, authorization.CurrentPolicy, error) {
	decision, present := store.decisions[authorizationRef]
	if !present {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.LimitPoolFactsSnapshot{}, authorization.CurrentPolicy{}, errors.New("data authorization decision not found")
	}
	snapshot, present := store.limitPools[snapshotRef]
	if !present {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.LimitPoolFactsSnapshot{}, authorization.CurrentPolicy{}, errors.New("limit-pool facts Snapshot not found")
	}
	current, err := store.ResolveCurrentDataAuthorizationPolicyByRef(
		context.Background(), authorization.LimitPoolFactsPolicyRef,
	)
	if err != nil {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.LimitPoolFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	return decision, snapshot, current, nil
}

func (store *quantTradingFactsStore) PublishDataAuthorizationPolicyVersion(
	context.Context, string, authorization.PolicyVersion, marketdataapp.DataAuthorizationPolicyPublicationAudit,
) error {
	return errors.New("policy publication is not part of this evidence chain")
}

func (store *quantTradingFactsStore) ResolveCurrentDataAuthorizationPolicy(
	context.Context,
) (authorization.CurrentPolicy, error) {
	current, present := store.policies[authorization.DailyBarsV2PolicyRef]
	if !present {
		return authorization.CurrentPolicy{}, marketdataapp.ErrDataAuthorizationPolicyNotPublished
	}
	return current, nil
}

func (store *quantTradingFactsStore) ResolveCurrentDataAuthorizationPolicyByRef(
	_ context.Context, policyRef string,
) (authorization.CurrentPolicy, error) {
	current, present := store.policies[policyRef]
	if !present {
		return authorization.CurrentPolicy{}, marketdataapp.ErrDataAuthorizationPolicyNotPublished
	}
	return current, nil
}

func (store *quantTradingFactsStore) AppendDataAuthorizationDecision(
	_ context.Context, decision marketdataapp.DataAuthorizationDecision,
) error {
	var ref struct {
		AuthorizationRef string `json:"authorization_ref"`
	}
	if err := json.Unmarshal(decision.AuthorizationRef, &ref); err != nil {
		return err
	}
	store.decisions[ref.AuthorizationRef] = decision
	return nil
}

func (store *quantTradingFactsStore) GetDataAuthorizationDecision(
	_ context.Context, authorizationRef string,
) (marketdataapp.DataAuthorizationDecision, error) {
	decision, present := store.decisions[authorizationRef]
	if !present {
		return marketdataapp.DataAuthorizationDecision{}, errors.New("data authorization decision not found")
	}
	return decision, nil
}

func (store *quantTradingFactsStore) ReadDataAuthorizedSecurityTradingStatusFactsSnapshot(
	_ context.Context, authorizationRef, snapshotRef string,
) (marketdataapp.DataAuthorizationDecision, marketdataapp.SecurityTradingStatusFactsSnapshot, authorization.CurrentPolicy, error) {
	decision, present := store.decisions[authorizationRef]
	if !present {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, errors.New("data authorization decision not found")
	}
	snapshot, present := store.facts[snapshotRef]
	if !present {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, errors.New("trading-status facts Snapshot not found")
	}
	current, err := store.ResolveCurrentDataAuthorizationPolicyByRef(context.Background(), authorization.SecurityStatusFactsPolicyRef)
	if err != nil {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	return decision, snapshot, current, nil
}

func (store *quantTradingFactsStore) ReadDataAuthorizedOrderBookFactsSnapshot(
	_ context.Context, authorizationRef, snapshotRef string,
) (marketdataapp.DataAuthorizationDecision, marketdataapp.OrderBookFactsSnapshot, authorization.CurrentPolicy, error) {
	decision, present := store.decisions[authorizationRef]
	if !present {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, errors.New("data authorization decision not found")
	}
	snapshot, present := store.orderBooks[snapshotRef]
	if !present {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, errors.New("order-book facts Snapshot not found")
	}
	current, err := store.ResolveCurrentDataAuthorizationPolicyByRef(
		context.Background(), authorization.OrderBookFactsPolicyRef,
	)
	if err != nil {
		return marketdataapp.DataAuthorizationDecision{}, marketdataapp.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	return decision, snapshot, current, nil
}

// quantTradingFactsWrongClassStore 模拟"仓储按错误数据类返回策略"的故障。
type quantTradingFactsWrongClassStore struct{ *quantTradingFactsStore }

func (store *quantTradingFactsWrongClassStore) ResolveCurrentDataAuthorizationPolicyByRef(
	_ context.Context, _ string,
) (authorization.CurrentPolicy, error) {
	current, present := store.policies[authorization.DailyBarsV2PolicyRef]
	if !present {
		return authorization.CurrentPolicy{}, marketdataapp.ErrDataAuthorizationPolicyNotPublished
	}
	return current, nil
}

type quantTradingFactsPITReader struct {
	record     *model.SecurityTradingStatusSnapshot
	wantAsOf   time.Time
	lastCalled *time.Time
}

func (reader *quantTradingFactsPITReader) SecurityTradingStatusAsOf(
	_ context.Context, symbol, tradeDate string, asOf time.Time,
) (*model.SecurityTradingStatusSnapshot, error) {
	if symbol != tradingFactsSymbol || tradeDate != tradingFactsDate || !asOf.Equal(reader.wantAsOf) {
		return nil, errors.New("PIT identity or as_of does not match the requested facts")
	}
	reader.lastCalled = &asOf
	copy := *reader.record
	copy.QualityReasons = append([]string(nil), reader.record.QualityReasons...)
	return &copy, nil
}

// quantOrderBookProviderFake 提供真实结构但确定性的五档盘口，避免端到端测试依赖外网。
// 它不参与数值推导：值由 Data Creator 经 OrderBook.Validate 与 TradableVolumeShares 读取。
type quantOrderBookProviderFake struct {
	wantAsOf time.Time
	book     *model.OrderBook
}

func (p *quantOrderBookProviderFake) FetchOrderBook(
	_ context.Context, symbol string,
) (*model.OrderBook, error) {
	if p.book == nil || symbol != p.book.Symbol || !p.book.ObservedAt.Before(p.wantAsOf) {
		return nil, errors.New("order-book identity or observation time does not match the request")
	}
	copy := *p.book
	return &copy, nil
}

func validOrderBook(systemAsOf time.Time) *model.OrderBook {
	observed := systemAsOf.Add(-2 * time.Second)
	levels := func(base float64) []model.OrderBookLevel {
		return []model.OrderBookLevel{
			{Price: base, Volume: 1200},
			{Price: base - 0.1, Volume: 900},
			{Price: base - 0.2, Volume: 700},
			{Price: base - 0.3, Volume: 500},
			{Price: base - 0.4, Volume: 300},
		}
	}
	return &model.OrderBook{
		Symbol:           tradingFactsSymbol,
		Bids:             levels(1313.1),
		Asks:             levels(1313.2),
		ObservedAt:       observed,
		SystemObservedAt: observed,
		Source:           "eastmoney",
		QualityStatus:    model.QualityValid,
	}
}

type quantTradingFactsChain struct {
	adapter             *quantTradingFactsAdapter
	orderBookAdapter    *quantOrderBookFactsAdapter
	store               *quantTradingFactsStore
	authorized          *marketdataapplication.AuthorizedSecurityTradingStatusFactsSnapshotReader
	creator             *marketdataapplication.SecurityTradingStatusFactsSnapshotCreator
	orderBookCreator    *marketdataapplication.OrderBookFactsSnapshotCreator
	orderBookAuthorized *marketdataapplication.AuthorizedOrderBookFactsSnapshotReader
	limitPoolAdapter    *quantLimitPoolFactsAdapter
	context             context.Context
	systemAsOf          time.Time
	evaluatedAt         time.Time
	closeSeriesRef      string
}

func tradingFactsPolicy(t *testing.T, systemAsOf time.Time) authorization.CurrentPolicy {
	t.Helper()
	return tradingFactsPolicyForClass(t, authorization.SecurityStatusFactsPolicyRef, tradingFactsPolicyID, systemAsOf)
}

func tradingFactsPolicyForClass(
	t *testing.T, policyRef, policyVersion string, systemAsOf time.Time,
) authorization.CurrentPolicy {
	t.Helper()
	policy := authorization.DataAccessPolicy{
		SchemaVersion:   authorization.DataAccessPolicySchemaVersion,
		Status:          authorization.PolicyApproved,
		AllowedSubjects: []string{"principal:service:researchflow"},
		AllowedPurposes: []string{tradingFactsPurpose},
		RequiredScopes:  []string{security.ScopeDataRead},
		EffectiveAt:     systemAsOf.Add(-24 * time.Hour).UTC(),
	}
	payload, err := json.Marshal(policy)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(payload)
	return authorization.CurrentPolicy{
		Version: authorization.PolicyVersion{
			PolicyType: authorization.DataAccessPolicyType, PolicyRef: policyRef,
			PolicyVersion: policyVersion, Payload: payload,
			PayloadHash: hex.EncodeToString(sum[:]), CreatedAt: systemAsOf.Add(-24 * time.Hour).UTC(),
		},
		Policy: policy,
	}
}

func validTradingStatusRecord(systemAsOf time.Time) *model.SecurityTradingStatusSnapshot {
	observed := systemAsOf.Add(-2 * time.Hour)
	return &model.SecurityTradingStatusSnapshot{
		Symbol:             tradingFactsSymbol,
		TradeDate:          tradingFactsDate,
		RiskWarningStatus:  model.RiskWarningNormal,
		DelistingStatus:    model.DelistingStatusNormal,
		ListingDate:        "2001-08-27",
		ListingStatusKnown: true,
		IsTradingDay:       true,
		TradingDayKnown:    true,
		SuspensionStatus:   model.SuspensionStatusNoEvent,
		Source:             "tushare",
		SourceVersion:      "tushare-status-v1",
		FetchedAt:          observed,
		SystemObservedAt:   &observed,
		QualityStatus:      model.QualityValid,
	}
}

func newQuantTradingFactsChain(t *testing.T, store *quantTradingFactsStore) quantTradingFactsChain {
	t.Helper()
	systemAsOf := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	store.policies[authorization.SecurityStatusFactsPolicyRef] = tradingFactsPolicy(t, systemAsOf)
	store.policies[authorization.OrderBookFactsPolicyRef] = tradingFactsPolicyForClass(
		t, authorization.OrderBookFactsPolicyRef, "order-book-policy-2026-11", systemAsOf,
	)
	store.policies[authorization.LimitPoolFactsPolicyRef] = tradingFactsPolicyForClass(
		t, authorization.LimitPoolFactsPolicyRef, "limit-pool-policy-2026-11", systemAsOf,
	)

	owner, err := marketdataapplication.NewDataAuthorizationOwner(store, store)
	if err != nil {
		t.Fatal(err)
	}
	pit := &quantTradingFactsPITReader{record: validTradingStatusRecord(systemAsOf), wantAsOf: systemAsOf}
	creator, err := marketdataapplication.NewSecurityTradingStatusFactsSnapshotCreator(pit, store, owner)
	if err != nil {
		t.Fatal(err)
	}
	authorized, err := marketdataapplication.NewAuthorizedSecurityTradingStatusFactsSnapshotReader(store)
	if err != nil {
		t.Fatal(err)
	}
	adapter, err := newQuantTradingFactsAdapter(&quantTradingFactsDataPort{creator: creator, authorized: authorized})
	if err != nil {
		t.Fatal(err)
	}
	orderBookCreator, err := marketdataapplication.NewOrderBookFactsSnapshotCreator(
		&quantOrderBookProviderFake{wantAsOf: systemAsOf, book: validOrderBook(systemAsOf)}, store, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	orderBookAuthorized, err := marketdataapplication.NewAuthorizedOrderBookFactsSnapshotReader(store)
	if err != nil {
		t.Fatal(err)
	}
	orderBookAdapter, err := newQuantOrderBookFactsAdapter(&quantOrderBookFactsDataPort{
		creator: orderBookCreator, authorized: orderBookAuthorized,
	})
	if err != nil {
		t.Fatal(err)
	}
	limitPoolCreator, err := marketdataapplication.NewLimitPoolFactsSnapshotCreator(
		&quantLimitPoolProviderFake{up: validLimitPoolUp(systemAsOf), down: validLimitPoolDown(systemAsOf)},
		store, owner,
	)
	if err != nil {
		t.Fatal(err)
	}
	limitPoolAuthorized, err := marketdataapplication.NewAuthorizedLimitPoolFactsSnapshotReader(store)
	if err != nil {
		t.Fatal(err)
	}
	limitPoolAdapter, err := newQuantLimitPoolFactsAdapter(&quantLimitPoolFactsPortFake{
		creator: limitPoolCreator, authorized: limitPoolAuthorized,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := security.WithPrincipal(context.Background(), security.Principal{
		Kind: security.IdentityService, AuthMethod: security.AuthMethodServiceBearer,
		Subject: "researchflow", Scopes: []string{security.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	return quantTradingFactsChain{
		adapter: adapter, orderBookAdapter: orderBookAdapter, store: store,
		authorized: authorized, creator: creator,
		orderBookCreator: orderBookCreator, orderBookAuthorized: orderBookAuthorized,
		limitPoolAdapter: limitPoolAdapter,
		context:          ctx, systemAsOf: systemAsOf, evaluatedAt: systemAsOf.Add(time.Hour),
	}
}

// quantTradingFactsDataPort 等价于 marketdata/bootstrap.Module 的两行委托，只是把 MariaDB 换成内存 Store。
type quantTradingFactsDataPort struct {
	creator    *marketdataapplication.SecurityTradingStatusFactsSnapshotCreator
	authorized *marketdataapplication.AuthorizedSecurityTradingStatusFactsSnapshotReader
}

func (port *quantTradingFactsDataPort) CreateSecurityTradingStatusFactsSnapshot(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (marketdataapplication.SecurityTradingStatusFactsSnapshotReferences, error) {
	return port.creator.Create(ctx, symbol, tradeDate, systemAsOf, purpose)
}

func (port *quantTradingFactsDataPort) ReadAuthorizedSecurityTradingStatusFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (marketdataapp.SecurityTradingStatusFactsSnapshot, error) {
	return port.authorized.Read(ctx, snapshotRef, authorizationRef, purpose)
}

var _ quantTradingFactsPort = (*quantTradingFactsDataPort)(nil)

type quantOrderBookFactsDataPort struct {
	creator    *marketdataapplication.OrderBookFactsSnapshotCreator
	authorized *marketdataapplication.AuthorizedOrderBookFactsSnapshotReader
}

func (port *quantOrderBookFactsDataPort) CreateOrderBookFactsSnapshot(
	ctx context.Context, symbol string, systemAsOf time.Time, purpose string,
) (marketdataapplication.OrderBookFactsSnapshotReferences, error) {
	return port.creator.Create(ctx, symbol, systemAsOf, purpose)
}

func (port *quantOrderBookFactsDataPort) ReadAuthorizedOrderBookFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (marketdataapp.OrderBookFactsSnapshot, error) {
	return port.authorized.Read(ctx, snapshotRef, authorizationRef, purpose)
}

var _ quantOrderBookFactsPort = (*quantOrderBookFactsDataPort)(nil)

type quantLimitPoolProviderFake struct {
	up, down *model.LimitPoolSnapshot
}

func (fake *quantLimitPoolProviderFake) FetchFullLimitPool(
	_ context.Context, poolType, _ string,
) (*model.LimitPoolSnapshot, error) {
	switch poolType {
	case "up":
		return fake.up, nil
	case "down":
		return fake.down, nil
	default:
		return nil, errors.New("unsupported limit-pool type")
	}
}

// unknownLimitPoolFactsForTest 构造"池不完整"的涨跌停 DTO：用于验证缺失路径，
// 不代表真实 Provider 结果。
func unknownLimitPoolFactsForTest(systemAsOf time.Time) quantapplication.LimitPoolFactsSnapshot {
	return quantapplication.LimitPoolFactsSnapshot{
		SnapshotRef:           "data_snapshot:sha256:" + strings.Repeat("e", 64),
		AuthorizationRef:      "data_authorization:limit-pool-opaque",
		ManifestSchemaVersion: "1.0.0",
		PayloadSchemaVersion:  quantapplication.LimitPoolFactsPayloadSchemaVersion,
		PayloadKind:           quantapplication.LimitPoolFactsPayloadKind,
		CallerRef:             "principal:service:researchflow",
		PurposeCode:           tradingFactsPurpose,
		Symbol:                tradingFactsSymbol,
		TradeDate:             systemAsOf.Format("2006-01-02"),
		ObservedAt:            systemAsOf.Add(-time.Minute).Format(time.RFC3339Nano),
		SystemAsOf:            systemAsOf.Format(time.RFC3339Nano),
		LimitStatus:           quantapplication.LimitPoolStatusUnknown,
		UpPoolTotal:           10,
		UpPoolComplete:        false,
		DownPoolComplete:      true,
		Source:                "eastmoney",
		QualityStatus:         "valid",
	}
}

func validLimitPoolUp(systemAsOf time.Time) *model.LimitPoolSnapshot {
	return &model.LimitPoolSnapshot{
		PoolType: "up", TradeDate: systemAsOf.Format("2006-01-02"), Total: 0, HasData: true,
		Source: "eastmoney", FetchedAt: systemAsOf.Add(-time.Minute), QualityStatus: model.QualityValid,
	}
}

func validLimitPoolDown(systemAsOf time.Time) *model.LimitPoolSnapshot {
	return &model.LimitPoolSnapshot{
		PoolType: "down", TradeDate: systemAsOf.Format("2006-01-02"), Total: 0, HasData: true,
		Source: "eastmoney", FetchedAt: systemAsOf.Add(-time.Minute), QualityStatus: model.QualityValid,
	}
}

type quantLimitPoolFactsPortFake struct {
	creator    *marketdataapplication.LimitPoolFactsSnapshotCreator
	authorized *marketdataapplication.AuthorizedLimitPoolFactsSnapshotReader
}

func (port *quantLimitPoolFactsPortFake) CreateLimitPoolFactsSnapshot(
	ctx context.Context, symbol, tradeDate string, systemAsOf time.Time, purpose string,
) (marketdataapplication.LimitPoolFactsSnapshotReferences, error) {
	return port.creator.Create(ctx, symbol, tradeDate, systemAsOf, purpose)
}

func (port *quantLimitPoolFactsPortFake) ReadAuthorizedLimitPoolFactsSnapshot(
	ctx context.Context, snapshotRef, authorizationRef, purpose string,
) (marketdataapp.LimitPoolFactsSnapshot, error) {
	return port.authorized.Read(ctx, snapshotRef, authorizationRef, purpose)
}

var _ quantLimitPoolFactsPort = (*quantLimitPoolFactsPortFake)(nil)

func TestQuantTradingFactsEndToEndProducesTraceableRejection(t *testing.T) {
	store := newQuantTradingFactsStore()
	chain := newQuantTradingFactsChain(t, store)

	snapshotRef, authorizationRef, err := chain.adapter.CreateTradingFactsSnapshot(
		chain.context, tradingFactsSymbol, tradingFactsDate, chain.systemAsOf, tradingFactsPurpose,
	)
	if err != nil {
		t.Fatalf("creating an authorized trading-status facts Snapshot failed: %v", err)
	}
	if len(store.facts) != 1 || len(store.decisions) != 1 {
		t.Fatalf("expected exactly one persisted Snapshot and one decision, got %d/%d", len(store.facts), len(store.decisions))
	}
	snapshot, present := store.facts[snapshotRef]
	if !present {
		t.Fatalf("SnapshotRef %q does not resolve to the persisted Snapshot", snapshotRef)
	}
	if snapshot.Manifest.PurposeCode != tradingFactsPurpose || snapshot.Payload.QualityStatus != marketdataapp.QualityValid {
		t.Fatalf("persisted Snapshot has unexpected purpose/quality: %+v", snapshot.Manifest)
	}

	facts, err := chain.adapter.ReadTradingFactsSnapshot(chain.context, snapshotRef, authorizationRef, tradingFactsPurpose)
	if err != nil {
		t.Fatalf("authorized facts read failed: %v", err)
	}
	if facts.SnapshotRef != snapshotRef || facts.AuthorizationRef != authorizationRef ||
		facts.Symbol != tradingFactsSymbol || facts.TradeDate != tradingFactsDate ||
		facts.SystemAsOf != chain.systemAsOf.Format(time.RFC3339Nano) {
		t.Fatalf("facts identity does not match the requested evidence: %+v", facts)
	}
	if len(facts.Facts) != 5 || facts.Facts[0].Key != "risk_warning_status" || facts.Facts[0].Value != "normal" {
		t.Fatalf("facts payload was not transported faithfully: %+v", facts.Facts)
	}

	orderBookFacts := quantapplication.OrderBookFactsSnapshot{
		SnapshotRef:           "data_snapshot:sha256:" + strings.Repeat("b", 64),
		AuthorizationRef:      "data_authorization:order-book-opaque",
		ManifestSchemaVersion: "1.0.0",
		PayloadSchemaVersion:  quantapplication.OrderBookFactsPayloadSchemaVersion,
		PayloadKind:           quantapplication.OrderBookFactsPayloadKind,
		CallerRef:             "principal:service:researchflow",
		PurposeCode:           tradingFactsPurpose,
		Symbol:                tradingFactsSymbol,
		ObservedAt:            chain.systemAsOf.Add(-2 * time.Second).Format(time.RFC3339Nano),
		SystemAsOf:            chain.systemAsOf.Format(time.RFC3339Nano),
		TradableVolumeShares:  3600,
		BidLevels:             5,
		AskLevels:             5,
		Source:                "eastmoney",
		QualityStatus:         "valid",
	}
	evidence := quantapplication.TradabilityEvidence{
		TradingFacts: facts, OrderBookFacts: orderBookFacts,
		LimitPoolFacts: unknownLimitPoolFactsForTest(chain.systemAsOf),
	}
	assessment, err := quantapplication.AssessTradabilityFacts(evidence)
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	if assessment.Facts.Verified {
		t.Fatal("tradability must stay unverified while price_limit is absent")
	}
	wantMissing := []risk.TradabilityFact{risk.TradabilityFactPriceLimit}
	if len(assessment.Facts.MissingFacts) != len(wantMissing) {
		t.Fatalf("MissingFacts = %v, want %v", assessment.Facts.MissingFacts, wantMissing)
	}
	for index, want := range wantMissing {
		if assessment.Facts.MissingFacts[index] != want {
			t.Fatalf("MissingFacts[%d] = %q, want %q", index, assessment.Facts.MissingFacts[index], want)
		}
	}
	wantRefs := []string{
		snapshotRef, authorizationRef, orderBookFacts.SnapshotRef, orderBookFacts.AuthorizationRef,
		evidence.LimitPoolFacts.SnapshotRef, evidence.LimitPoolFacts.AuthorizationRef,
	}
	if len(assessment.Facts.EvidenceRefs) != len(wantRefs) {
		t.Fatalf("EvidenceRefs = %v, want %v", assessment.Facts.EvidenceRefs, wantRefs)
	}
	for index := range wantRefs {
		if assessment.Facts.EvidenceRefs[index] != wantRefs[index] {
			t.Fatalf("EvidenceRefs[%d] = %q, want %q", index, assessment.Facts.EvidenceRefs[index], wantRefs[index])
		}
	}
	if len(assessment.BlockingFacts) != 0 {
		t.Fatalf("BlockingFacts = %v, want none for a normally traded security", assessment.BlockingFacts)
	}

	decision, err := quantapplication.EvaluateTradabilityDecision(
		chain.context, "risk-decision-1", chain.evaluatedAt, evidence,
	)
	if err != nil {
		t.Fatalf("EvaluateTradabilityDecision() error = %v", err)
	}
	if decision.Status != risk.DecisionReject || decision.RuleSetVersion != risk.TradabilityRuleSetVersion {
		t.Fatalf("decision = %+v, want reject under %s", decision, risk.TradabilityRuleSetVersion)
	}
	if len(decision.Reasons) != 1 || decision.Reasons[0].Code != risk.TradabilityGateCode ||
		decision.Reasons[0].Status != risk.ReasonTriggered {
		t.Fatalf("decision reasons = %+v, want a triggered tradability gate", decision.Reasons)
	}
	if len(decision.InputRefs) != len(wantRefs) {
		t.Fatalf("decision InputRefs = %v, want the same evidence as the assessment", decision.InputRefs)
	}
	for index := range wantRefs {
		if decision.InputRefs[index] != wantRefs[index] {
			t.Fatalf("decision InputRefs[%d] = %q, want %q", index, decision.InputRefs[index], wantRefs[index])
		}
	}
	if err := decision.Validate(); err != nil {
		t.Fatalf("decision is not contract-valid: %v", err)
	}
}

func TestQuantTradingFactsEndToEndFailsClosedWithoutFactsPolicy(t *testing.T) {
	store := newQuantTradingFactsStore()
	chain := newQuantTradingFactsChain(t, store)
	delete(store.policies, authorization.SecurityStatusFactsPolicyRef)

	_, _, err := chain.adapter.CreateTradingFactsSnapshot(
		chain.context, tradingFactsSymbol, tradingFactsDate, chain.systemAsOf, tradingFactsPurpose,
	)
	if err == nil {
		t.Fatal("facts snapshot creation succeeded without a published trading-facts policy")
	}
	// 允许留下不可变快照，但绝不签发授权，也绝不返回可消费引用。
	if len(store.decisions) != 0 {
		t.Fatalf("no authorization decision may be persisted without a policy, got %d", len(store.decisions))
	}
}

func TestQuantTradingFactsEndToEndRejectsPolicyClassMismatch(t *testing.T) {
	// 只有日线策略被发布；按数据类解析时故意返回日线策略，模拟仓储故障。
	barsOnly := newQuantTradingFactsStore()
	barsOnly.policies[authorization.DailyBarsV2PolicyRef] = tradingFactsPolicyForClass(
		t, authorization.DailyBarsV2PolicyRef, "bars-policy-2026-11", time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
	)
	store := &quantTradingFactsWrongClassStore{quantTradingFactsStore: barsOnly}

	systemAsOf := time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC)
	owner, err := marketdataapplication.NewDataAuthorizationOwner(store, store)
	if err != nil {
		t.Fatal(err)
	}
	pit := &quantTradingFactsPITReader{record: validTradingStatusRecord(systemAsOf), wantAsOf: systemAsOf}
	creator, err := marketdataapplication.NewSecurityTradingStatusFactsSnapshotCreator(pit, store, owner)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := security.WithPrincipal(context.Background(), security.Principal{
		Kind: security.IdentityService, AuthMethod: security.AuthMethodServiceBearer,
		Subject: "researchflow", Scopes: []string{security.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := creator.Create(ctx, tradingFactsSymbol, tradingFactsDate, systemAsOf, tradingFactsPurpose); err == nil {
		t.Fatal("issuing an authorization bound to the wrong policy class succeeded")
	}
	if len(store.decisions) != 0 {
		t.Fatalf("no decision may be persisted for a policy class mismatch, got %d", len(store.decisions))
	}
}

func TestQuantTradingFactsReaderRejectsOtherPayloadKinds(t *testing.T) {
	store := newQuantTradingFactsStore()
	chain := newQuantTradingFactsChain(t, store)
	snapshotRef, authorizationRef, err := chain.adapter.CreateTradingFactsSnapshot(
		chain.context, tradingFactsSymbol, tradingFactsDate, chain.systemAsOf, tradingFactsPurpose,
	)
	if err != nil {
		t.Fatalf("creating facts Snapshot failed: %v", err)
	}
	// 把同一个引用挪到 close-series 映射中，facts 读取必须拒绝而不是串读。
	delete(store.facts, snapshotRef)
	if _, err := chain.adapter.ReadTradingFactsSnapshot(
		chain.context, snapshotRef, authorizationRef, tradingFactsPurpose,
	); err == nil {
		t.Fatal("facts reader accepted a Snapshot that is not a trading-status facts payload")
	}
}

func TestQuantTradingFactsReaderRejectsTamperedPayload(t *testing.T) {
	store := newQuantTradingFactsStore()
	chain := newQuantTradingFactsChain(t, store)
	snapshotRef, authorizationRef, err := chain.adapter.CreateTradingFactsSnapshot(
		chain.context, tradingFactsSymbol, tradingFactsDate, chain.systemAsOf, tradingFactsPurpose,
	)
	if err != nil {
		t.Fatalf("creating facts Snapshot failed: %v", err)
	}
	snapshot := store.facts[snapshotRef]
	snapshot.Payload.Facts[0].Value = "risk_warning"
	// 指纹未重算：读取端必须发现内容与指纹不一致。
	store.facts[snapshotRef] = snapshot
	if _, err := chain.adapter.ReadTradingFactsSnapshot(
		chain.context, snapshotRef, authorizationRef, tradingFactsPurpose,
	); err == nil {
		t.Fatal("facts reader accepted a tampered payload")
	}
}
