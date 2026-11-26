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
	t *testing.T, snapshot TradingFactsSnapshot,
) (*CreateRiskService, *riskFactsReaderFake, *riskRecordingRepository) {
	t.Helper()
	facts := &riskFactsReaderFake{snapshot: snapshot}
	repository := newRiskRecordingRepository()
	service, err := NewCreateRiskService(
		facts, &riskDecisionIDsFake{id: "risk_decision_1"}, repository,
		func() time.Time { return time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewCreateRiskService() error = %v", err)
	}
	return service, facts, repository
}

func riskTestParameters(snapshot TradingFactsSnapshot) json.RawMessage {
	return json.RawMessage(`{"symbol":"` + snapshot.Symbol + `","trade_date":"` + snapshot.TradeDate + `"}`)
}

func TestCreateRiskServiceProducesTraceableRejection(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	service, facts, repository := newCreateRiskTestService(t, snapshot)
	execution, err := service.Execute(context.Background(), CreateRiskRequest{
		SnapshotRef:      snapshot.SnapshotRef,
		AuthorizationRef: snapshot.AuthorizationRef,
		SystemAsOf:       mustRiskTime(t, snapshot.SystemAsOf),
		Parameters:       riskTestParameters(snapshot),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if facts.purpose != riskPurposeCode {
		t.Fatalf("facts purpose = %q, want %q", facts.purpose, riskPurposeCode)
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
	if len(execution.MissingFacts) != 2 ||
		execution.MissingFacts[0] != risk.TradabilityFactPriceLimit ||
		execution.MissingFacts[1] != risk.TradabilityFactTradableVolume {
		t.Fatalf("missing facts = %v", execution.MissingFacts)
	}
	// 证据必须同时包含快照与授权：只有快照无法证明"按什么授权读到"。
	wantRefs := []string{snapshot.SnapshotRef, snapshot.AuthorizationRef}
	if len(execution.Decision.InputRefs) != len(wantRefs) {
		t.Fatalf("input_refs = %v, want %v", execution.Decision.InputRefs, wantRefs)
	}
	for index := range wantRefs {
		if execution.Decision.InputRefs[index] != wantRefs[index] {
			t.Fatalf("input_refs[%d] = %q, want %q", index, execution.Decision.InputRefs[index], wantRefs[index])
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
	service, _, _ := newCreateRiskTestService(t, snapshot)
	systemAsOf := mustRiskTime(t, snapshot.SystemAsOf)
	execution, err := service.Execute(context.Background(), CreateRiskRequest{
		SnapshotRef: snapshot.SnapshotRef, AuthorizationRef: snapshot.AuthorizationRef,
		SystemAsOf: systemAsOf, Parameters: riskTestParameters(snapshot),
	})
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
	service, _, _ := newCreateRiskTestService(t, snapshot)
	execution, err := service.Execute(context.Background(), CreateRiskRequest{
		SnapshotRef: snapshot.SnapshotRef, AuthorizationRef: snapshot.AuthorizationRef,
		SystemAsOf: mustRiskTime(t, snapshot.SystemAsOf), Parameters: riskTestParameters(snapshot),
	})
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
			service, _, repository := newCreateRiskTestService(t, snapshot)
			_, err := service.Execute(context.Background(), CreateRiskRequest{
				SnapshotRef: snapshot.SnapshotRef, AuthorizationRef: snapshot.AuthorizationRef,
				SystemAsOf: mustRiskTime(t, mutate.systemAsOf), Parameters: json.RawMessage(mutate.parameters),
			})
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
	service, _, repository := newCreateRiskTestService(t, snapshot)
	_, err := service.Execute(context.Background(), CreateRiskRequest{
		SnapshotRef: snapshot.SnapshotRef, AuthorizationRef: snapshot.AuthorizationRef,
		SystemAsOf: mustRiskTime(t, snapshot.SystemAsOf), Parameters: riskTestParameters(snapshot),
	})
	if !errors.Is(err, ErrInvalidRiskSnapshot) {
		t.Fatalf("error = %v, want ErrInvalidRiskSnapshot", err)
	}
	if repository.Records() != 0 {
		t.Fatalf("tampered snapshot persisted %d records", repository.Records())
	}
}

func TestCreateRiskServiceFailsWhenDecisionIdentityUnavailable(t *testing.T) {
	snapshot := validTradingFactsSnapshot()
	facts := &riskFactsReaderFake{snapshot: snapshot}
	service, err := NewCreateRiskService(
		facts, &riskDecisionIDsFake{err: errors.New("entropy unavailable")}, result.NewMemoryRepository(),
		func() time.Time { return time.Date(2026, 11, 25, 16, 0, 0, 0, time.UTC) },
	)
	if err != nil {
		t.Fatalf("NewCreateRiskService() error = %v", err)
	}
	_, err = service.Execute(context.Background(), CreateRiskRequest{
		SnapshotRef: snapshot.SnapshotRef, AuthorizationRef: snapshot.AuthorizationRef,
		SystemAsOf: mustRiskTime(t, snapshot.SystemAsOf), Parameters: riskTestParameters(snapshot),
	})
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
