package bootstrap

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	marketdataapp "example.com/stock-analysis-flow/internal/marketdata/app"
	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	"example.com/stock-analysis-flow/internal/marketdata/model"
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
	facts     map[string]marketdataapp.SecurityTradingStatusFactsSnapshot
	policies  map[string]authorization.CurrentPolicy
	decisions map[string]marketdataapp.DataAuthorizationDecision
}

func newQuantTradingFactsStore() *quantTradingFactsStore {
	return &quantTradingFactsStore{
		facts:     map[string]marketdataapp.SecurityTradingStatusFactsSnapshot{},
		policies:  map[string]authorization.CurrentPolicy{},
		decisions: map[string]marketdataapp.DataAuthorizationDecision{},
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

type quantTradingFactsChain struct {
	adapter        *quantTradingFactsAdapter
	store          *quantTradingFactsStore
	authorized     *marketdataapplication.AuthorizedSecurityTradingStatusFactsSnapshotReader
	creator        *marketdataapplication.SecurityTradingStatusFactsSnapshotCreator
	context        context.Context
	systemAsOf     time.Time
	evaluatedAt    time.Time
	closeSeriesRef string
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
	ctx, err := security.WithPrincipal(context.Background(), security.Principal{
		Kind: security.IdentityService, AuthMethod: security.AuthMethodServiceBearer,
		Subject: "researchflow", Scopes: []string{security.ScopeDataRead},
	})
	if err != nil {
		t.Fatal(err)
	}
	return quantTradingFactsChain{
		adapter: adapter, store: store, authorized: authorized, creator: creator,
		context: ctx, systemAsOf: systemAsOf, evaluatedAt: systemAsOf.Add(time.Hour),
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

	assessment, err := quantapplication.AssessTradabilityFacts(facts)
	if err != nil {
		t.Fatalf("AssessTradabilityFacts() error = %v", err)
	}
	if assessment.Facts.Verified {
		t.Fatal("tradability must stay unverified while price_limit and tradable_volume are absent")
	}
	wantMissing := []risk.TradabilityFact{risk.TradabilityFactPriceLimit, risk.TradabilityFactTradableVolume}
	if len(assessment.Facts.MissingFacts) != len(wantMissing) {
		t.Fatalf("MissingFacts = %v, want %v", assessment.Facts.MissingFacts, wantMissing)
	}
	for index, want := range wantMissing {
		if assessment.Facts.MissingFacts[index] != want {
			t.Fatalf("MissingFacts[%d] = %q, want %q", index, assessment.Facts.MissingFacts[index], want)
		}
	}
	if len(assessment.Facts.EvidenceRefs) != 2 ||
		assessment.Facts.EvidenceRefs[0] != snapshotRef || assessment.Facts.EvidenceRefs[1] != authorizationRef {
		t.Fatalf("EvidenceRefs = %v, want [snapshot_ref, authorization_ref]", assessment.Facts.EvidenceRefs)
	}
	if len(assessment.BlockingFacts) != 0 {
		t.Fatalf("BlockingFacts = %v, want none for a normally traded security", assessment.BlockingFacts)
	}

	decision, err := quantapplication.EvaluateTradabilityDecision(
		chain.context, "risk-decision-1", chain.evaluatedAt, facts,
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
	if len(decision.InputRefs) != 2 || decision.InputRefs[0] != snapshotRef || decision.InputRefs[1] != authorizationRef {
		t.Fatalf("decision InputRefs = %v, want the same evidence as the assessment", decision.InputRefs)
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
