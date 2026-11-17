package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

type testClock struct{ now time.Time }

func (c testClock) Now() time.Time { return c.now }

type testStore struct {
	events []domain.StageEvent
	result *domain.RunResult
}

type testSnapshots struct{}

func (testSnapshots) SavePortfolio(_ context.Context, _ string, s domain.PortfolioSnapshot) (string, error) {
	if s.ID != "" {
		return s.ID, nil
	}
	return "portfolio-persisted", nil
}

func (testSnapshots) SaveRaw(_ context.Context, _ string, s domain.RawSnapshot) (string, error) {
	if s.ID != "" {
		return s.ID, nil
	}
	return "raw-persisted", nil
}
func (testSnapshots) SaveMarket(_ context.Context, _ string, s domain.MarketSnapshot) (string, error) {
	if s.ID != "" {
		return s.ID, nil
	}
	return "market-persisted", nil
}

func (s *testStore) Begin(context.Context, domain.RunRequest, time.Time) error { return nil }
func (s *testStore) AppendEvent(_ context.Context, e domain.StageEvent) error {
	s.events = append(s.events, e)
	return nil
}
func (s *testStore) Finish(_ context.Context, r domain.RunResult) error {
	copy := r
	s.result = &copy
	return nil
}

type testUniverse struct{}

func (testUniverse) Resolve(_ context.Context, r domain.RunRequest) (domain.UniverseSnapshot, error) {
	return domain.UniverseSnapshot{ID: r.UniverseID, AsOf: r.AsOf, Version: "v1", Securities: []domain.Security{{Symbol: "600000.SH", TradingState: "normal"}}}, nil
}

type testRequirements struct{}

func (testRequirements) Resolve(context.Context, domain.RunRequest) (domain.RequirementSet, error) {
	return domain.RequirementSet{Core: []domain.DataRequirement{{Dataset: "daily_bars", Required: true}}}, nil
}

type testData struct{}

func (testData) Collect(_ context.Context, r domain.RunRequest, _ domain.UniverseSnapshot, _ domain.RequirementSet) (domain.RawSnapshot, error) {
	return domain.RawSnapshot{ID: "raw-1", AsOf: r.AsOf}, nil
}

type testNormalizer struct{}

func (testNormalizer) Normalize(_ context.Context, raw domain.RawSnapshot, _ domain.RunRequest) (domain.MarketSnapshot, error) {
	return domain.MarketSnapshot{ID: "market-1", AsOf: raw.AsOf, Bars: map[string][]domain.Bar{"600000.SH": {{Symbol: "600000.SH", Close: 10}}}}, nil
}

type testQuality struct {
	pass          bool
	policyVersion string
}

func (q testQuality) Assess(_ context.Context, r domain.RunRequest, _ domain.UniverseSnapshot, _ domain.MarketSnapshot, _ domain.RequirementSet) (domain.QualityReport, error) {
	version := q.policyVersion
	if version == "" {
		version = "quality-v1"
	}
	return domain.QualityReport{CorePass: q.pass, StrategyEligible: map[string]bool{"trend": true}, CheckedAt: r.AsOf, PolicyVersion: version}, nil
}

type testMarket struct{ calls int }

func (m *testMarket) Analyze(_ context.Context, _ domain.RunRequest, s domain.MarketSnapshot) (domain.MarketState, error) {
	m.calls++
	return domain.MarketState{AsOf: s.AsOf, Regime: "neutral", AlgorithmVersion: "market-v1"}, nil
}

type testFactors struct{}

func (testFactors) Calculate(context.Context, domain.RunRequest, domain.UniverseSnapshot, domain.MarketSnapshot, domain.MarketState, domain.QualityReport, domain.RequirementSet) ([]domain.FactorValue, error) {
	return nil, nil
}

type testStrategies struct{}

func (testStrategies) Evaluate(_ context.Context, r domain.RunRequest, _ domain.UniverseSnapshot, s domain.MarketSnapshot, _ domain.MarketState, _ []domain.FactorValue, _ domain.QualityReport) ([]domain.CandidateSignal, error) {
	return []domain.CandidateSignal{{ID: "signal-1", Symbol: "600000.SH", StrategyID: "trend", StrategyVersion: "strategy-v1", Direction: "long", SignalAt: r.AsOf, ValidUntil: r.AsOf.Add(48 * time.Hour), EntryRule: "next session open", ExitRule: "stop or invalidation", InvalidationRule: "close below trend"}}, nil
}

type testTradability struct{}

func (testTradability) Check(_ context.Context, _ domain.RunRequest, _ domain.MarketSnapshot, signals []domain.CandidateSignal) ([]domain.SignalDecision, error) {
	out := make([]domain.SignalDecision, 0, len(signals))
	for _, signal := range signals {
		out = append(out, domain.SignalDecision{SignalID: signal.ID, Allowed: true})
	}
	return out, nil
}

type testPortfolioProvider struct{}

func (testPortfolioProvider) Load(_ context.Context, request domain.RunRequest) (domain.PortfolioSnapshot, error) {
	return domain.PortfolioSnapshot{PortfolioID: request.PortfolioID, AsOf: request.AsOf, Currency: "CNY", AvailableCash: 100000, TotalEquity: 100000, Version: "paper-v1"}, nil
}

type testRisk struct{}

func (testRisk) Assess(_ context.Context, _ domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, signals []domain.CandidateSignal, _ []domain.SignalDecision) ([]domain.SignalDecision, error) {
	out := make([]domain.SignalDecision, 0, len(signals))
	for _, signal := range signals {
		out = append(out, domain.SignalDecision{SignalID: signal.ID, Allowed: true})
	}
	return out, nil
}

type testPlanner struct{}

func (testPlanner) Plan(_ context.Context, r domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, signals []domain.CandidateSignal, _ []domain.SignalDecision, _ []domain.SignalDecision) ([]domain.TradePlan, []domain.PlanRejection, error) {
	out := make([]domain.TradePlan, 0, len(signals))
	for _, signal := range signals {
		out = append(out, domain.TradePlan{SignalID: signal.ID, Symbol: signal.Symbol, Side: "buy", Quantity: 100, ReferencePrice: 10, EarliestExecuteAt: r.AsOf.Add(24 * time.Hour), ExpireAt: r.AsOf.Add(36 * time.Hour), RuleVersion: r.TradingRulesVersion})
	}
	return out, nil, nil
}

type testExecutionAssessor struct{}

func (testExecutionAssessor) Assess(_ context.Context, _ domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, plans []domain.TradePlan) (domain.ExecutionAssessment, error) {
	orders := make([]domain.ExecutionOrderAssessment, 0, len(plans))
	for _, plan := range plans {
		orders = append(orders, domain.ExecutionOrderAssessment{SignalID: plan.SignalID, Status: domain.ExecutionPending, RequestedQuantity: plan.Quantity, RemainingQuantity: plan.Quantity})
	}
	return domain.ExecutionAssessment{ModelVersion: "plan-assessment-v1", Orders: orders, Warnings: []string{"future execution remains pending until later market data is available"}}, nil
}

func testDependencies(q testQuality, market *testMarket, store *testStore) Dependencies {
	return Dependencies{
		Clock:     testClock{now: time.Date(2026, 10, 20, 16, 0, 0, 0, time.FixedZone("CST", 8*3600))},
		Universes: testUniverse{}, Requirements: testRequirements{}, Data: testData{}, Normalizer: testNormalizer{},
		Quality: q, Market: market, Factors: testFactors{}, Strategies: testStrategies{}, Tradability: testTradability{},
		Portfolio: testPortfolioProvider{}, Risk: testRisk{}, Planner: testPlanner{}, Execution: testExecutionAssessor{}, Snapshots: testSnapshots{}, Store: store,
	}
}

func validRequest() domain.RunRequest {
	return domain.RunRequest{TaskType: domain.TaskPostMarketScan, AsOf: time.Date(2026, 10, 20, 15, 0, 0, 0, time.FixedZone("CST", 8*3600)), UniverseID: "all-a-shares", Symbols: []string{"600000"}, PortfolioID: "paper-main", StrategyIDs: []string{"trend", "trend"}, MarketID: "CN-A", StrategyConfigVersion: "strategy-v1", RiskConfigVersion: "risk-v1", TradingRulesVersion: "rules-2026-07", DataPolicyVersion: "quality-v1"}
}

func TestRunnerCompletesAndPersistsTrace(t *testing.T) {
	market, store := &testMarket{}, &testStore{}
	runner, err := NewRunner(testDependencies(testQuality{pass: true}, market, store))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.RunStatusCompleted {
		t.Fatalf("status = %s", result.Status)
	}
	if len(result.Signals) != 1 || len(result.Plans) != 1 {
		t.Fatalf("expected signal and plan, got %d and %d", len(result.Signals), len(result.Plans))
	}
	if market.calls != 1 {
		t.Fatalf("market analysis calls = %d", market.calls)
	}
	if store.result == nil || store.result.Status != domain.RunStatusCompleted {
		t.Fatal("final result was not persisted")
	}
	if len(store.events) == 0 {
		t.Fatal("stage trace was not persisted")
	}
	if len(result.Request.StrategyIDs) != 1 {
		t.Fatalf("strategy IDs should be deduplicated, got %#v", result.Request.StrategyIDs)
	}
	if result.PortfolioSnapshotID == "" || result.Portfolio == nil {
		t.Fatal("portfolio snapshot should be persisted and referenced in the result")
	}
	if len(result.Execution.Orders) != 1 || result.Execution.Orders[0].Status != domain.ExecutionPending {
		t.Fatalf("new plan should remain pending until future market data arrives: %#v", result.Execution.Orders)
	}
	if !result.Plans[0].EarliestExecuteAt.After(result.Request.AsOf) {
		t.Fatal("plan execution must be strictly after the analysis cutoff")
	}
}

func TestRunnerBlocksOnCoreDataQuality(t *testing.T) {
	market, store := &testMarket{}, &testStore{}
	runner, err := NewRunner(testDependencies(testQuality{pass: false}, market, store))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.RunStatusBlocked {
		t.Fatalf("status = %s", result.Status)
	}
	if len(result.Signals) != 0 {
		t.Fatalf("blocked run must not emit signals: %#v", result.Signals)
	}
	if market.calls != 0 {
		t.Fatal("market analysis must not run after failed core quality gate")
	}
	if store.result == nil || store.result.Status != domain.RunStatusBlocked {
		t.Fatal("blocked status was not persisted")
	}
}

func TestRunnerRejectsInvalidRequest(t *testing.T) {
	market, store := &testMarket{}, &testStore{}
	runner, err := NewRunner(testDependencies(testQuality{pass: true}, market, store))
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest()
	request.StrategyIDs = nil
	result, err := runner.Run(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != domain.RunStatusRejected {
		t.Fatalf("status = %s", result.Status)
	}
	if result.Failure == nil || result.Failure.Code != "invalid_request" {
		t.Fatalf("unexpected failure: %#v", result.Failure)
	}
}

func TestRunnerRejectsMissingOrMalformedSymbols(t *testing.T) {
	for _, tc := range []struct {
		name    string
		symbols []string
	}{
		{name: "missing symbols"},
		{name: "symbol with exchange suffix", symbols: []string{"600519.SH"}},
		{name: "non-numeric symbol", symbols: []string{"60AB19"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			market, store := &testMarket{}, &testStore{}
			runner, err := NewRunner(testDependencies(testQuality{pass: true}, market, store))
			if err != nil {
				t.Fatal(err)
			}
			request := validRequest()
			request.Symbols = tc.symbols
			result, err := runner.Run(context.Background(), request)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != domain.RunStatusRejected {
				t.Fatalf("status = %s, want %s", result.Status, domain.RunStatusRejected)
			}
			if result.Failure == nil || result.Failure.Code != "invalid_request" {
				t.Fatalf("unexpected failure: %#v", result.Failure)
			}
			if market.calls != 0 {
				t.Fatal("request validation must reject symbols before market analysis")
			}
		})
	}
}

func TestRunnerReportsOperationalFailure(t *testing.T) {
	market, store := &testMarket{}, &testStore{}
	deps := testDependencies(testQuality{pass: true}, market, store)
	deps.Data = failingData{err: errors.New("source timeout")}
	runner, err := NewRunner(deps)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), validRequest())
	if err == nil {
		t.Fatal("expected operational error")
	}
	if result.Status != domain.RunStatusFailed {
		t.Fatalf("status = %s", result.Status)
	}
	if store.result == nil || store.result.Status != domain.RunStatusFailed {
		t.Fatal("failed status was not persisted")
	}
}

func TestRunnerFailsClosedWhenQualityPolicyVersionDiffers(t *testing.T) {
	market, store := &testMarket{}, &testStore{}
	runner, err := NewRunner(testDependencies(testQuality{pass: true, policyVersion: "unfrozen-quality-v2"}, market, store))
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), validRequest())
	if err == nil {
		t.Fatal("expected policy version mismatch to fail the run")
	}
	if result.Status != domain.RunStatusFailed || len(result.Signals) != 0 {
		t.Fatalf("unexpected result after policy version mismatch: status=%s signals=%d", result.Status, len(result.Signals))
	}
}

func TestValidateCandidateSignalsRejectsExpiredSignals(t *testing.T) {
	asOf := time.Date(2026, 10, 20, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
	universe := domain.UniverseSnapshot{Securities: []domain.Security{{Symbol: "600000.SH"}}}
	signal := domain.CandidateSignal{ID: "s1", Symbol: "600000.SH", StrategyID: "trend", StrategyVersion: "v1", Direction: "long", SignalAt: asOf.Add(-24 * time.Hour), ValidUntil: asOf, EntryRule: "next open", ExitRule: "stop", InvalidationRule: "trend broken"}
	if err := validateCandidateSignals(domain.RunRequest{AsOf: asOf}, []string{"trend"}, universe, []domain.CandidateSignal{signal}); err == nil {
		t.Fatal("expected signal expiring at the analysis cutoff to be rejected")
	}
}

func TestValidateSignalDecisionsRequiresExactCoverage(t *testing.T) {
	signals := []domain.CandidateSignal{{ID: "s1"}, {ID: "s2"}}
	if err := validateSignalDecisions(signals, []domain.SignalDecision{{SignalID: "s1", Allowed: true}}, "risk"); err == nil {
		t.Fatal("expected missing decision to fail")
	}
	if err := validateSignalDecisions(signals, []domain.SignalDecision{{SignalID: "s1", Allowed: true}, {SignalID: "s1", Allowed: true}}, "risk"); err == nil {
		t.Fatal("expected duplicate decision to fail")
	}
	if err := validateSignalDecisions(signals, []domain.SignalDecision{{SignalID: "s1", Allowed: true}, {SignalID: "other", Allowed: true}}, "risk"); err == nil {
		t.Fatal("expected unknown signal decision to fail")
	}
	if err := validateSignalDecisions(signals, []domain.SignalDecision{{SignalID: "s1", Allowed: true}, {SignalID: "s2", Allowed: false}}, "risk"); err == nil {
		t.Fatal("expected denial without reason to fail")
	}
}

func TestValidateExecutionAssessmentRequiresExplicitPendingOutcomeForFuturePlan(t *testing.T) {
	asOf := time.Date(2026, 10, 20, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
	plans := []domain.TradePlan{{SignalID: "s1", Symbol: "600000.SH", Side: "buy", Quantity: 100, ReferencePrice: 10, EarliestExecuteAt: asOf.Add(time.Hour)}}
	request := domain.RunRequest{AsOf: asOf}
	good := domain.ExecutionAssessment{ModelVersion: "plan-assessment-v1", Orders: []domain.ExecutionOrderAssessment{{SignalID: "s1", Status: domain.ExecutionPending, RequestedQuantity: 100, RemainingQuantity: 100}}}
	if err := validateExecutionAssessment(request, plans, good); err != nil {
		t.Fatalf("expected explicit pending outcome to pass: %v", err)
	}
	bad := domain.ExecutionAssessment{ModelVersion: "plan-assessment-v1", Orders: []domain.ExecutionOrderAssessment{{SignalID: "s1", Status: domain.ExecutionFilled, RequestedQuantity: 100, FilledQuantity: 100}}}
	if err := validateExecutionAssessment(request, plans, bad); err == nil {
		t.Fatal("expected future fill claim to fail point-in-time validation")
	}
	missing := domain.ExecutionAssessment{ModelVersion: "plan-assessment-v1"}
	if err := validateExecutionAssessment(request, plans, missing); err == nil {
		t.Fatal("expected omitted plan outcome to fail")
	}
}

type failingData struct{ err error }

func (f failingData) Collect(context.Context, domain.RunRequest, domain.UniverseSnapshot, domain.RequirementSet) (domain.RawSnapshot, error) {
	return domain.RawSnapshot{}, f.err
}

func TestApprovedForPlanningFailsClosed(t *testing.T) {
	signals := []domain.CandidateSignal{{ID: "s1", Symbol: "600000.SH"}, {ID: "s2", Symbol: "000001.SZ"}}
	tradability := []domain.SignalDecision{{SignalID: "s1", Allowed: true}, {SignalID: "s2", Allowed: true}}
	risk := []domain.SignalDecision{{SignalID: "s1", Allowed: true}, {SignalID: "s2", Allowed: false, Reasons: []string{"industry concentration"}}}
	approved, rejected := approvedForPlanning(signals, tradability, risk)
	if len(approved) != 1 || approved[0].ID != "s1" {
		t.Fatalf("unexpected approved signals: %#v", approved)
	}
	if len(rejected) != 1 || rejected[0].SignalID != "s2" {
		t.Fatalf("unexpected rejections: %#v", rejected)
	}

	approved, rejected = approvedForPlanning(signals[:1], tradability[:1], nil)
	if len(approved) != 0 || len(rejected) != 1 {
		t.Fatalf("missing risk decision must reject: approved=%#v rejected=%#v", approved, rejected)
	}
}
