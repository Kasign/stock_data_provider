package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/authorization"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/risk"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
)

// Risk 门禁 Stage 的端到端契约测试：Stage 参数 → Data 创建并签发事实快照 →
// 授权读取 → 门禁评估 → 决策落库 → Stage Output 引用真实 result_id。
// 除 MariaDB 外全部使用生产代码（Data 创建器/签发器/授权读取器、Quant Facade、
// Stage Adapter、两个 Validator）。
//
// 验收标准是"拒绝可追溯"，不是"能放行"：Data 已提供真实盘口事实但尚无涨跌停事实，
// 因此决策必然 reject，missing_facts 只点名 price_limit，且四段证据必须指向真实快照与真实授权。

type quantRiskResultStore struct {
	records map[string]result.Record
}

func newQuantRiskResultStore() *quantRiskResultStore {
	return &quantRiskResultStore{records: map[string]result.Record{}}
}

func (store *quantRiskResultStore) Save(_ context.Context, record result.Record) error {
	if existing, present := store.records[record.Manifest.ResultID]; present &&
		existing.Manifest.ManifestFingerprint != record.Manifest.ManifestFingerprint {
		return errors.New("conflicting risk result")
	}
	store.records[record.Manifest.ResultID] = record
	return nil
}

type quantRiskFixedDecisionIDs struct{ id string }

func (ids quantRiskFixedDecisionIDs) NewDecisionID(context.Context) (string, error) {
	return ids.id, nil
}

func newQuantRiskEndToEndAdapter(
	t *testing.T, chain quantTradingFactsChain,
) (*quantstage.RiskAdapter, *quantRiskResultStore) {
	t.Helper()
	store := newQuantRiskResultStore()
	service, err := quantapplication.NewCreateRiskService(
		chain.adapter, chain.orderBookAdapter, quantRiskFixedDecisionIDs{id: "risk_decision_e2e"}, store,
		func() time.Time { return chain.systemAsOf.Add(time.Minute) },
	)
	if err != nil {
		t.Fatalf("NewCreateRiskService() error = %v", err)
	}
	adapter, err := quantstage.NewRiskAdapter(chain.adapter, chain.orderBookAdapter, service)
	if err != nil {
		t.Fatalf("NewRiskAdapter() error = %v", err)
	}
	return adapter, store
}

func TestQuantRiskStageEndToEndProducesTraceableRejection(t *testing.T) {
	chain := newQuantTradingFactsChain(t, newQuantTradingFactsStore())
	adapter, persisted := newQuantRiskEndToEndAdapter(t, chain)

	output, quality, err := adapter.Execute(chain.context, quantstage.RiskRequest{
		Symbol: tradingFactsSymbol, TradeDate: tradingFactsDate, SystemAsOf: chain.systemAsOf,
	})
	if err != nil {
		t.Fatalf("Risk Stage Execute() error = %v", err)
	}
	// 1) 决策结论必须是拒绝，且理由只有门禁那一条。
	if output.Status != risk.DecisionReject || output.ResultStatus != "rejected" ||
		output.RuleSetVersion != risk.TradabilityRuleSetVersion {
		t.Fatalf("unexpected decision: status=%q result_status=%q rule_set=%q",
			output.Status, output.ResultStatus, output.RuleSetVersion)
	}
	if len(output.Reasons) != 1 || output.Reasons[0].Code != risk.TradabilityGateCode ||
		output.Reasons[0].Status != risk.ReasonTriggered {
		t.Fatalf("unexpected reasons: %+v", output.Reasons)
	}
	// 2) 缺项必须只点名涨跌停：可成交量已由真实盘口授权快照核验。
	wantMissing := []string{"price_limit"}
	if len(output.MissingFacts) != len(wantMissing) {
		t.Fatalf("missing facts = %v, want %v", output.MissingFacts, wantMissing)
	}
	for index, want := range wantMissing {
		if output.MissingFacts[index] != want {
			t.Fatalf("missing facts[%d] = %q, want %q", index, output.MissingFacts[index], want)
		}
	}
	if len(output.BlockingFacts) != 0 {
		t.Fatalf("blocking facts = %v, want none for a normally traded security", output.BlockingFacts)
	}
	// 3) 证据引用必须包含两份真实快照与两份授权，且 evaluation 时刻等于 Run 的 system_as_of。
	if len(output.InputRefs) != 4 {
		t.Fatalf("input refs = %v, want four evidence references", output.InputRefs)
	}
	for index, prefix := range []string{
		"data_snapshot:sha256:", "data_authorization:", "data_snapshot:sha256:", "data_authorization:",
	} {
		if !strings.HasPrefix(output.InputRefs[index], prefix) {
			t.Fatalf("input_refs[%d] = %q, want prefix %q", index, output.InputRefs[index], prefix)
		}
	}
	if output.InputRefs[0] == output.InputRefs[2] || output.InputRefs[1] == output.InputRefs[3] {
		t.Fatalf("trading-status and order-book evidence must be distinct: %v", output.InputRefs)
	}
	if _, present := chain.store.facts[output.InputRefs[0]]; !present {
		t.Fatalf("trading-status snapshot ref %q is not persisted", output.InputRefs[0])
	}
	if _, present := chain.store.orderBooks[output.InputRefs[2]]; !present {
		t.Fatalf("order-book snapshot ref %q is not persisted", output.InputRefs[2])
	}
	if _, present := chain.store.decisions[output.InputRefs[1]]; !present {
		t.Fatalf("trading-status authorization ref %q is not persisted", output.InputRefs[1])
	}
	if _, present := chain.store.decisions[output.InputRefs[3]]; !present {
		t.Fatalf("order-book authorization ref %q is not persisted", output.InputRefs[3])
	}
	if output.EvaluatedAt != chain.systemAsOf.Format(time.RFC3339Nano) ||
		output.SystemAsOf != chain.systemAsOf.Format(time.RFC3339Nano) {
		t.Fatalf("evaluated_at/system_as_of = %q/%q, want %q",
			output.EvaluatedAt, output.SystemAsOf, chain.systemAsOf.Format(time.RFC3339Nano))
	}
	// 4) Stage Output 必须引用一条真实落库的结果，且该结果与 Manifest 指纹一致。
	record, present := persisted.records[output.ResultID]
	if !present {
		t.Fatalf("Stage Output result_id %q does not resolve to a persisted result", output.ResultID)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("persisted result is not content-addressed: %v", err)
	}
	wantSnapshotRefs := []string{output.InputRefs[0], output.InputRefs[2]}
	sort.Strings(wantSnapshotRefs) // Manifest 按内容寻址规则对输入快照引用排序。
	if len(record.Manifest.InputSnapshotRefs) != len(wantSnapshotRefs) {
		t.Fatalf("persisted manifest input refs = %v, want %v", record.Manifest.InputSnapshotRefs, wantSnapshotRefs)
	}
	for index := range wantSnapshotRefs {
		if record.Manifest.InputSnapshotRefs[index] != wantSnapshotRefs[index] {
			t.Fatalf("persisted manifest input_refs[%d] = %q, want %q", index, record.Manifest.InputSnapshotRefs[index], wantSnapshotRefs[index])
		}
	}
	var decision risk.Decision
	if err := json.Unmarshal(record.Payload, &decision); err != nil {
		t.Fatalf("decode persisted risk payload: %v", err)
	}
	if decision.Status != risk.DecisionReject || decision.DecisionID != "risk_decision_e2e" {
		t.Fatalf("persisted decision = %+v", decision)
	}
	// 5) Stage Output 与 Quality Summary 必须各自通过其冻结 Validator。
	if err := quantstage.ValidateRiskStageOutputJSON(mustQuantRiskJSON(t, output)); err != nil {
		t.Fatalf("Stage Output failed its validator: %v", err)
	}
	if err := quantstage.ValidateRiskQualitySummaryJSON(mustQuantRiskJSON(t, quality)); err != nil {
		t.Fatalf("Quality Summary failed its validator: %v", err)
	}
	if quality.DecisionStatus != "reject" || quality.QualityStatus != "valid" ||
		quality.ResultStatus != "rejected" {
		t.Fatalf("unexpected quality summary: %+v", quality)
	}
}

// 未发布交易事实策略时必须 fail closed：不产出决策、不落库。
func TestQuantRiskStageEndToEndFailsClosedWithoutFactsPolicy(t *testing.T) {
	store := newQuantTradingFactsStore()
	chain := newQuantTradingFactsChain(t, store)
	adapter, persisted := newQuantRiskEndToEndAdapter(t, chain)
	for ref := range store.policies {
		delete(store.policies, ref)
	}
	if _, _, err := adapter.Execute(chain.context, quantstage.RiskRequest{
		Symbol: tradingFactsSymbol, TradeDate: tradingFactsDate, SystemAsOf: chain.systemAsOf,
	}); err == nil {
		t.Fatal("Risk Stage produced a decision without a published trading-facts policy")
	}
	if len(persisted.records) != 0 {
		t.Fatalf("no risk result may be persisted without an authorization, got %d", len(persisted.records))
	}
}

// 未发布盘口数据类策略时必须 fail closed：不得用交易事实策略替代盘口授权。
func TestQuantRiskStageEndToEndFailsClosedWithoutOrderBookPolicy(t *testing.T) {
	store := newQuantTradingFactsStore()
	chain := newQuantTradingFactsChain(t, store)
	adapter, persisted := newQuantRiskEndToEndAdapter(t, chain)
	delete(store.policies, authorization.OrderBookFactsPolicyRef)
	if _, _, err := adapter.Execute(chain.context, quantstage.RiskRequest{
		Symbol: tradingFactsSymbol, TradeDate: tradingFactsDate, SystemAsOf: chain.systemAsOf,
	}); err == nil {
		t.Fatal("Risk Stage produced a decision without a published order-book policy")
	}
	if len(persisted.records) != 0 {
		t.Fatalf("no risk result may be persisted without order-book authorization, got %d", len(persisted.records))
	}
}

// 未来交易日必须前置拒绝：否则会用未来事实做今天的判断。
func TestQuantRiskStageEndToEndRejectsFutureTradeDate(t *testing.T) {
	chain := newQuantTradingFactsChain(t, newQuantTradingFactsStore())
	adapter, persisted := newQuantRiskEndToEndAdapter(t, chain)
	future := chain.systemAsOf.AddDate(0, 0, 1).Format("2006-01-02")
	if _, _, err := adapter.Execute(chain.context, quantstage.RiskRequest{
		Symbol: tradingFactsSymbol, TradeDate: future, SystemAsOf: chain.systemAsOf,
	}); !errors.Is(err, quantstage.ErrInvalidRiskStageRequest) {
		t.Fatalf("error = %v, want ErrInvalidRiskStageRequest", err)
	}
	if len(persisted.records) != 0 {
		t.Fatalf("no risk result may be persisted for a future trade date, got %d", len(persisted.records))
	}
}

// 事实快照与请求身份错配（PIT 返回另一只证券）必须拒绝，且不落库。
func TestQuantRiskStageEndToEndRejectsSnapshotIdentityMismatch(t *testing.T) {
	store := newQuantTradingFactsStore()
	chain := newQuantTradingFactsChain(t, store)
	adapter, persisted := newQuantRiskEndToEndAdapter(t, chain)
	if _, _, err := adapter.Execute(chain.context, quantstage.RiskRequest{
		Symbol: "000001.SZ", TradeDate: tradingFactsDate, SystemAsOf: chain.systemAsOf,
	}); err == nil {
		t.Fatal("Risk Stage accepted a Snapshot created for a different security")
	}
	if len(persisted.records) != 0 {
		t.Fatalf("no risk result may be persisted for a mismatched Snapshot, got %d", len(persisted.records))
	}
}

func mustQuantRiskJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func quantRiskInputSnapshotRef(t *testing.T, output quantstage.RiskOutput) string {
	t.Helper()
	if len(output.InputRefs) != 4 {
		t.Fatalf("input refs = %v", output.InputRefs)
	}
	return output.InputRefs[0]
}

func quantRiskInputAuthorizationRef(t *testing.T, output quantstage.RiskOutput) string {
	t.Helper()
	if len(output.InputRefs) != 4 {
		t.Fatalf("input refs = %v", output.InputRefs)
	}
	return output.InputRefs[1]
}
