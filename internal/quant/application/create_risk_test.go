package application

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/risk"
)

type riskFactsReaderFake struct {
	snapshot TradingFactsSnapshot
	err      error
	calls    int
	purpose  string
}

func (fake *riskFactsReaderFake) ReadTradingFactsSnapshot(
	_ context.Context, _, _, purpose string,
) (TradingFactsSnapshot, error) {
	fake.calls++
	fake.purpose = purpose
	return fake.snapshot, fake.err
}

type riskOrderBookReaderFake struct {
	snapshot OrderBookFactsSnapshot
	err      error
	calls    int
	purpose  string
}

func (fake *riskOrderBookReaderFake) ReadOrderBookFactsSnapshot(
	_ context.Context, _, _, purpose string,
) (OrderBookFactsSnapshot, error) {
	fake.calls++
	fake.purpose = purpose
	return fake.snapshot, fake.err
}

type riskLimitPoolReaderFake struct {
	snapshot LimitPoolFactsSnapshot
	err      error
	calls    int
	purpose  string
}

func (fake *riskLimitPoolReaderFake) ReadLimitPoolFactsSnapshot(
	_ context.Context, _, _, purpose string,
) (LimitPoolFactsSnapshot, error) {
	fake.calls++
	fake.purpose = purpose
	return fake.snapshot, fake.err
}

type riskDecisionIDsFake struct {
	id    string
	err   error
	calls int
}

func (fake *riskDecisionIDsFake) NewDecisionID(context.Context) (string, error) {
	fake.calls++
	return fake.id, fake.err
}

// riskRecordingRepository 记录落库结果，同时保留读取能力供断言使用。
// 复用 result.MemoryRepository 会丢掉"决策真的落库了"这一可验证事实
// （内存实现只提供 Save，不提供读取）。
type riskRecordingRepository struct {
	inner   *result.MemoryRepository
	records map[string]result.Record
	saveErr error
}

func newRiskRecordingRepository() *riskRecordingRepository {
	return &riskRecordingRepository{inner: result.NewMemoryRepository(), records: make(map[string]result.Record)}
}

func (repository *riskRecordingRepository) Save(ctx context.Context, record result.Record) error {
	if repository.saveErr != nil {
		return repository.saveErr
	}
	if err := repository.inner.Save(ctx, record); err != nil {
		return err
	}
	repository.records[record.Manifest.ResultID] = record
	return nil
}

func (repository *riskRecordingRepository) Records() int { return len(repository.records) }

func newCreateRiskTestService(
	t *testing.T, snapshot TradingFactsSnapshot, orderBook OrderBookFactsSnapshot,
) (*CreateRiskService, *riskFactsReaderFake, *riskOrderBookReaderFake, *riskRecordingRepository) {
	t.Helper()
	return newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, validLimitPoolFactsSnapshot())
}

func newCreateRiskTestServiceWithLimitPool(
	t *testing.T, snapshot TradingFactsSnapshot, orderBook OrderBookFactsSnapshot, limitPool LimitPoolFactsSnapshot,
) (*CreateRiskService, *riskFactsReaderFake, *riskOrderBookReaderFake, *riskRecordingRepository) {
	t.Helper()
	facts := &riskFactsReaderFake{snapshot: snapshot}
	books := &riskOrderBookReaderFake{snapshot: orderBook}
	pools := &riskLimitPoolReaderFake{snapshot: limitPool}
	repository := newRiskRecordingRepository()
	service, err := NewCreateRiskService(
		facts, books, pools, &riskDecisionIDsFake{id: "risk_decision_1"}, repository,
		func() time.Time { return time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewCreateRiskService() error = %v", err)
	}
	return service, facts, books, repository
}

func riskTestParameters(snapshot TradingFactsSnapshot) json.RawMessage {
	return json.RawMessage(`{"symbol":"` + snapshot.Symbol + `","trade_date":"` + snapshot.TradeDate + `"}`)
}

func riskTestRequest(
	snapshot TradingFactsSnapshot, orderBook OrderBookFactsSnapshot, limitPool LimitPoolFactsSnapshot,
	systemAsOf time.Time, parameters json.RawMessage,
) CreateRiskRequest {
	return CreateRiskRequest{
		SnapshotRef:               snapshot.SnapshotRef,
		AuthorizationRef:          snapshot.AuthorizationRef,
		OrderBookSnapshotRef:      orderBook.SnapshotRef,
		OrderBookAuthorizationRef: orderBook.AuthorizationRef,
		LimitPoolSnapshotRef:      limitPool.SnapshotRef,
		LimitPoolAuthorizationRef: limitPool.AuthorizationRef,
		SystemAsOf:                systemAsOf,
		Parameters:                parameters,
	}
}

func TestCreateRiskServiceProducesTraceableRejection(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	limitPool := validLimitPoolFactsSnapshot()
	service, facts, books, repository := newCreateRiskTestService(t, snapshot, orderBook)
	execution, err := service.Execute(context.Background(), CreateRiskRequest{
		SnapshotRef:               snapshot.SnapshotRef,
		AuthorizationRef:          snapshot.AuthorizationRef,
		OrderBookSnapshotRef:      orderBook.SnapshotRef,
		OrderBookAuthorizationRef: orderBook.AuthorizationRef,
		LimitPoolSnapshotRef:      limitPool.SnapshotRef,
		LimitPoolAuthorizationRef: limitPool.AuthorizationRef,
		SystemAsOf:                mustRiskTime(t, snapshot.SystemAsOf),
		Parameters:                riskTestParameters(snapshot),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if facts.purpose != riskPurposeCode || books.purpose != riskPurposeCode {
		t.Fatalf("purpose = %q/%q, want %q for both facts", facts.purpose, books.purpose, riskPurposeCode)
	}
	if facts.calls != 1 || books.calls != 1 {
		t.Fatalf("reader calls = %d/%d, want 1/1", facts.calls, books.calls)
	}
	if execution.Decision.Status != risk.DecisionReject {
		t.Fatalf("decision status = %q, want reject", execution.Decision.Status)
	}
	if execution.Manifest.ResultStatus != "rejected" {
		t.Fatalf("manifest result_status = %q, want rejected", execution.Manifest.ResultStatus)
	}
	if execution.Manifest.ResultType != "risk" || execution.Manifest.Scope != "stock" ||
		execution.Manifest.SecurityID != snapshot.Symbol || execution.Manifest.TradeDate != snapshot.TradeDate {
		t.Fatalf("unexpected manifest identity: %+v", execution.Manifest)
	}
	if execution.Manifest.QualityStatus != "valid" {
		t.Fatalf("manifest quality_status = %q, want valid", execution.Manifest.QualityStatus)
	}
	if execution.RuleSetVersion != risk.TradabilityRuleSetVersion {
		t.Fatalf("rule set version = %q", execution.RuleSetVersion)
	}
	if len(execution.MissingFacts) != 1 ||
		execution.MissingFacts[0] != risk.TradabilityFactPriceLimit {
		t.Fatalf("missing facts = %v", execution.MissingFacts)
	}
	// 证据必须覆盖两份快照及各自授权：tradable_volume 来自盘口快照，不能省略其证据。
	wantRefs := []string{
		snapshot.SnapshotRef, snapshot.AuthorizationRef,
		orderBook.SnapshotRef, orderBook.AuthorizationRef,
		limitPool.SnapshotRef, limitPool.AuthorizationRef,
	}
	if len(execution.Decision.InputRefs) != len(wantRefs) {
		t.Fatalf("input_refs = %v, want %v", execution.Decision.InputRefs, wantRefs)
	}
	for index := range wantRefs {
		if execution.Decision.InputRefs[index] != wantRefs[index] {
			t.Fatalf("input_refs[%d] = %q, want %q", index, execution.Decision.InputRefs[index], wantRefs[index])
		}
	}
	wantSnapshotRefs := []string{snapshot.SnapshotRef, orderBook.SnapshotRef, limitPool.SnapshotRef}
	if len(execution.Manifest.InputSnapshotRefs) != len(wantSnapshotRefs) {
		t.Fatalf("manifest input refs = %v, want %v", execution.Manifest.InputSnapshotRefs, wantSnapshotRefs)
	}
	for index := range wantSnapshotRefs {
		if execution.Manifest.InputSnapshotRefs[index] != wantSnapshotRefs[index] {
			t.Fatalf("manifest input_refs[%d] = %q, want %q", index, execution.Manifest.InputSnapshotRefs[index], wantSnapshotRefs[index])
		}
	}
	// 决策必须真的落库：Stage Output 只能引用已持久化结果。
	persisted, ok := repository.records[execution.Manifest.ResultID]
	if !ok {
		t.Fatalf("risk result %q was not persisted", execution.Manifest.ResultID)
	}
	var persistedDecision risk.Decision
	if err := json.Unmarshal(persisted.Payload, &persistedDecision); err != nil {
		t.Fatalf("decode persisted risk payload: %v", err)
	}
	if persistedDecision.Status != risk.DecisionReject || persistedDecision.DecisionID != "risk_decision_1" {
		t.Fatalf("persisted decision = %+v", persistedDecision)
	}
	if err := persisted.Manifest.VerifyPayload(persisted.Payload); err != nil {
		t.Fatalf("persisted manifest does not match its payload: %v", err)
	}
}

// 评估时刻必须等于 Run 的可信 system_as_of，而不是墙钟：
// 这样同一份事实在重放时得到同一 result_id。
func TestCreateRiskServiceUsesSystemAsOfAsEvaluatedAt(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	service, _, _, _ := newCreateRiskTestService(t, snapshot, orderBook)
	systemAsOf := mustRiskTime(t, snapshot.SystemAsOf)
	execution, err := service.Execute(context.Background(),
		riskTestRequest(snapshot, orderBook, validLimitPoolFactsSnapshot(), systemAsOf, riskTestParameters(snapshot)))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if execution.Decision.EvaluatedAt != snapshot.SystemAsOf {
		t.Fatalf("evaluated_at = %q, want %q", execution.Decision.EvaluatedAt, snapshot.SystemAsOf)
	}
}

// 事实快照降级时，Manifest 质量也必须降级；不得因为五项事实齐全就升成 valid。
func TestCreateRiskServiceProjectsSnapshotQualityIntoManifest(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	snapshot.QualityStatus = "degraded"
	orderBook := validOrderBookFactsSnapshot()
	service, _, _, _ := newCreateRiskTestService(t, snapshot, orderBook)
	execution, err := service.Execute(context.Background(),
		riskTestRequest(snapshot, orderBook, validLimitPoolFactsSnapshot(), mustRiskTime(t, snapshot.SystemAsOf), riskTestParameters(snapshot)))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if execution.Manifest.QualityStatus != "degraded" {
		t.Fatalf("manifest quality_status = %q, want degraded", execution.Manifest.QualityStatus)
	}
	if execution.Decision.Status != risk.DecisionReject {
		t.Fatalf("decision status = %q, want reject", execution.Decision.Status)
	}
}

func TestCreateRiskServiceRejectsSnapshotParameterMismatch(t *testing.T) {
	for name, mutate := range map[string]struct {
		parameters string
		systemAsOf string
	}{
		"symbol":            {parameters: `{"symbol":"000001.SZ","trade_date":"2026-11-25"}`, systemAsOf: "2026-11-25T15:00:00Z"},
		"trade date":        {parameters: `{"symbol":"600519.SH","trade_date":"2026-11-24"}`, systemAsOf: "2026-11-25T15:00:00Z"},
		"future trade date": {parameters: `{"symbol":"600519.SH","trade_date":"2026-11-26"}`, systemAsOf: "2026-11-25T15:00:00Z"},
		"unknown field":     {parameters: `{"symbol":"600519.SH","trade_date":"2026-11-25","window":20}`, systemAsOf: "2026-11-25T15:00:00Z"},
		"wrong instant":     {parameters: `{"symbol":"600519.SH","trade_date":"2026-11-25"}`, systemAsOf: "2026-11-25T14:00:00Z"},
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := validTradingFactsSnapshot()
			orderBook := validOrderBookFactsSnapshot()
			service, _, _, repository := newCreateRiskTestService(t, snapshot, orderBook)
			_, err := service.Execute(context.Background(), riskTestRequest(
				snapshot, orderBook, validLimitPoolFactsSnapshot(), mustRiskTime(t, mutate.systemAsOf), json.RawMessage(mutate.parameters),
			))
			if err == nil {
				t.Fatal("Execute() accepted a mismatched request")
			}
			if !errors.Is(err, ErrInvalidRiskSnapshot) && !errors.Is(err, ErrInvalidCreateRiskRequest) {
				t.Fatalf("error = %v, want invalid snapshot or invalid request", err)
			}
			if repository.Records() != 0 {
				t.Fatalf("mismatched request persisted %d records", repository.Records())
			}
		})
	}
}

func TestCreateRiskServiceRejectsTamperedSnapshot(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	snapshot.Facts[0].Value = "" // available 事实缺值 → 快照自身不自洽
	orderBook := validOrderBookFactsSnapshot()
	service, _, _, repository := newCreateRiskTestService(t, snapshot, orderBook)
	_, err := service.Execute(context.Background(),
		riskTestRequest(snapshot, orderBook, validLimitPoolFactsSnapshot(), mustRiskTime(t, snapshot.SystemAsOf), riskTestParameters(snapshot)))
	if !errors.Is(err, ErrInvalidRiskSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidRiskSnapshot", err)
	}
	if repository.Records() != 0 {
		t.Fatalf("tampered snapshot persisted %d records", repository.Records())
	}
}

func TestCreateRiskServiceFailsWhenDecisionIdentityUnavailable(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	facts := &riskFactsReaderFake{snapshot: snapshot}
	books := &riskOrderBookReaderFake{snapshot: orderBook}
	pools := &riskLimitPoolReaderFake{snapshot: validLimitPoolFactsSnapshot()}
	service, err := NewCreateRiskService(
		facts, books, pools, &riskDecisionIDsFake{err: errors.New("entropy unavailable")}, result.NewMemoryRepository(),
		func() time.Time { return time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewCreateRiskService() error = %v", err)
	}
	_, err = service.Execute(context.Background(),
		riskTestRequest(snapshot, orderBook, validLimitPoolFactsSnapshot(), mustRiskTime(t, snapshot.SystemAsOf), riskTestParameters(snapshot)))
	if err == nil {
		t.Fatal("Execute() succeeded without a decision identity")
	}
}

func mustRiskTime(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatalf("parse %q: %v", value, err)
	}
	return parsed.UTC()
}

// TestExecuteAcceptsLimitPoolPublishedBeforeTheEvaluationInstant 覆盖 as-of 语义：
// 涨跌停池按交易日固化，允许复用更早发布的快照（例如 16:00 的池用于 23:00 的评估）。
// 之前要求 system_as_of 完全相等，会让「先采集、后评估」这一合法顺序无法成立。
func TestExecuteAcceptsLimitPoolPublishedBeforeTheEvaluationInstant(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	pool := validLimitPoolFactsSnapshot()
	pool.SystemAsOf = "2026-11-25T08:00:00Z" // 比评估时点（15:00Z）早 7 小时
	service, _, _, _ := newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, pool)

	request := riskTestRequest(snapshot, orderBook, pool, mustRiskTime(t, snapshot.SystemAsOf), riskTestParameters(snapshot))
	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatalf("an older but trade-date-bound limit-pool snapshot must be usable: %v", err)
	}
}

// TestExecuteRejectsLimitPoolPublishedAfterTheEvaluationInstant 锁定 as-of 的上界方向：
// 发布晚于评估时点的快照在 PIT 上还没有发生，绝不能使用。
func TestExecuteRejectsLimitPoolPublishedAfterTheEvaluationInstant(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	pool := validLimitPoolFactsSnapshot()
	pool.SystemAsOf = "2026-11-25T16:00:00Z" // 晚于评估时点 15:00:00Z
	service, _, _, _ := newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, pool)

	request := riskTestRequest(snapshot, orderBook, pool, mustRiskTime(t, snapshot.SystemAsOf), riskTestParameters(snapshot))
	if _, err := service.Execute(context.Background(), request); err == nil {
		t.Fatal("a limit-pool snapshot published after the evaluation instant must be rejected")
	} else if !errors.Is(err, ErrInvalidRiskSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidRiskSnapshot", err)
	}
}

func TestExecuteAcceptsTradingStatusAndOrderBookPublishedBeforeEvaluation(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	pool := validLimitPoolFactsSnapshot()
	// 2026-11-25 收盘后事实：07:02Z（15:02 北京时间）发布，15:00Z 评估。
	snapshot.SystemAsOf = "2026-11-25T07:02:00Z"
	orderBook.ObservedAt = "2026-11-25T07:01:00Z"
	orderBook.SystemAsOf = "2026-11-25T07:02:00Z"
	service, _, _, _ := newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, pool)

	request := riskTestRequest(snapshot, orderBook, pool, mustRiskTime(t, "2026-11-25T15:00:00Z"), riskTestParameters(snapshot))
	if _, err := service.Execute(context.Background(), request); err != nil {
		t.Fatalf("earlier trading-status/order-book snapshots must be reusable as-of: %v", err)
	}
}

func TestExecuteRejectsTradingStatusPublishedAfterEvaluation(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	pool := validLimitPoolFactsSnapshot()
	snapshot.SystemAsOf = "2026-11-25T16:00:00Z"
	service, _, _, _ := newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, pool)

	request := riskTestRequest(snapshot, orderBook, pool, mustRiskTime(t, "2026-11-25T15:00:00Z"), riskTestParameters(snapshot))
	if _, err := service.Execute(context.Background(), request); err == nil {
		t.Fatal("a trading-status snapshot published after the evaluation instant must be rejected")
	} else if !errors.Is(err, ErrInvalidRiskSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidRiskSnapshot", err)
	}
}

func TestExecuteRejectsOrderBookPublishedAfterEvaluation(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	pool := validLimitPoolFactsSnapshot()
	orderBook.ObservedAt = "2026-11-25T15:59:00Z"
	orderBook.SystemAsOf = "2026-11-25T16:00:00Z"
	service, _, _, _ := newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, pool)

	request := riskTestRequest(snapshot, orderBook, pool, mustRiskTime(t, snapshot.SystemAsOf), riskTestParameters(snapshot))
	if _, err := service.Execute(context.Background(), request); err == nil {
		t.Fatal("an order-book snapshot published after the evaluation instant must be rejected")
	} else if !errors.Is(err, ErrInvalidRiskSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidRiskSnapshot", err)
	}
}

func TestExecuteRejectsOrderBookFromDifferentTradeDate(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	pool := validLimitPoolFactsSnapshot()
	orderBook.ObservedAt = "2026-11-24T07:01:00Z"
	orderBook.SystemAsOf = "2026-11-25T07:02:00Z"
	service, _, _, _ := newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, pool)

	request := riskTestRequest(snapshot, orderBook, pool, mustRiskTime(t, snapshot.SystemAsOf), riskTestParameters(snapshot))
	if _, err := service.Execute(context.Background(), request); err == nil {
		t.Fatal("an order-book observation from another trade date must be rejected")
	} else if !errors.Is(err, ErrInvalidRiskSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidRiskSnapshot", err)
	}
}

func TestExecuteRejectsStaleIntradayOrderBook(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	orderBook := validOrderBookFactsSnapshot()
	pool := validLimitPoolFactsSnapshot()
	// 14:00 观测、14:30 评估（北京时间），超过 15 分钟盘中新鲜度。
	snapshot.SystemAsOf = "2026-11-25T06:01:00Z"
	orderBook.ObservedAt = "2026-11-25T06:00:00Z"
	orderBook.SystemAsOf = "2026-11-25T06:01:00Z"
	pool.ObservedAt = "2026-11-25T05:58:00Z"
	pool.SystemAsOf = "2026-11-25T05:59:00Z"
	service, _, _, _ := newCreateRiskTestServiceWithLimitPool(t, snapshot, orderBook, pool)

	request := riskTestRequest(snapshot, orderBook, pool, mustRiskTime(t, "2026-11-25T06:30:00Z"), riskTestParameters(snapshot))
	if _, err := service.Execute(context.Background(), request); err == nil {
		t.Fatal("an intraday order book older than the freshness bound must be rejected")
	} else if !errors.Is(err, ErrInvalidRiskSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidRiskSnapshot", err)
	}
}
