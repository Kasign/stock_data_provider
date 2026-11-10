package modules

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
	"example.com/stock-analysis-flow/internal/analysis/registry"
)

type testFactor struct {
	id      string
	version string
	calls   int
}

func (f *testFactor) ID() string      { return f.id }
func (f *testFactor) Version() string { return f.version }
func (f *testFactor) Requirements() []domain.DataRequirement {
	return []domain.DataRequirement{{Dataset: "daily_bars", Required: true, Consumer: f.id}}
}
func (f *testFactor) Calculate(_ context.Context, input FactorInput) ([]domain.FactorValue, error) {
	f.calls++
	value := 12.5
	return []domain.FactorValue{{Symbol: "600000.SH", Value: &value, Unit: "score", InputQuality: "good", AsOf: input.Request.AsOf}}, nil
}

type testStrategyModule struct {
	id      string
	version string
	factors []string
	calls   int
}

func (s *testStrategyModule) ID() string          { return s.id }
func (s *testStrategyModule) Version() string     { return s.version }
func (s *testStrategyModule) FactorIDs() []string { return append([]string(nil), s.factors...) }
func (s *testStrategyModule) Requirements() []domain.DataRequirement {
	return []domain.DataRequirement{{Dataset: "daily_bars", Fields: []string{"close"}, Required: true, Consumer: s.id}}
}
func (s *testStrategyModule) Evaluate(_ context.Context, input StrategyInput) ([]domain.CandidateSignal, error) {
	s.calls++
	return []domain.CandidateSignal{{ID: "sig-" + s.id, Symbol: "600000.SH", Direction: "long", SignalAt: input.Request.AsOf, EntryRule: "next session open", ExitRule: "risk exit", InvalidationRule: "trend broken"}}, nil
}

type testRiskRule struct {
	id       string
	version  string
	priority int
	allow    bool
	omit     bool
}

func (r testRiskRule) ID() string { return r.id }
func (r testRiskRule) Version() string {
	if r.version != "" {
		return r.version
	}
	return "v1"
}
func (r testRiskRule) Priority() int { return r.priority }
func (r testRiskRule) Evaluate(_ context.Context, input RiskInput) ([]domain.SignalDecision, error) {
	out := make([]domain.SignalDecision, 0, len(input.Signals))
	for i, signal := range input.Signals {
		if r.omit && i > 0 {
			continue
		}
		decision := domain.SignalDecision{SignalID: signal.ID, Allowed: r.allow, RuleIDs: []string{r.id}}
		if !r.allow {
			decision.Reasons = []string{"blocked by " + r.id}
		}
		out = append(out, decision)
	}
	return out, nil
}

func TestFactorEngineRunsOnlyDeclaredFactorDependencies(t *testing.T) {
	unused := &testFactor{id: "unused_factor", version: "1"}
	needed := &testFactor{id: "needed_factor", version: "2"}
	factors := registry.New[FactorCalculator]()
	if err := factors.Register(unused); err != nil {
		t.Fatal(err)
	}
	if err := factors.Register(needed); err != nil {
		t.Fatal(err)
	}
	factors.Freeze()
	engine, err := NewFactorEngine(factors)
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 20, 15, 0, 0, 0, time.FixedZone("CST", 8*3600))
	values, err := engine.Calculate(context.Background(), domain.RunRequest{AsOf: asOf}, domain.UniverseSnapshot{Securities: []domain.Security{{Symbol: "600000.SH"}}}, domain.MarketSnapshot{AsOf: asOf}, domain.MarketState{}, domain.QualityReport{}, domain.RequirementSet{FactorIDs: []string{"needed_factor"}})
	if err != nil {
		t.Fatal(err)
	}
	if unused.calls != 0 || needed.calls != 1 {
		t.Fatalf("unexpected calls: unused=%d needed=%d", unused.calls, needed.calls)
	}
	if len(values) != 1 || values[0].FactorID != "needed_factor" || values[0].FactorVersion != "2" {
		t.Fatalf("unexpected factor values: %#v", values)
	}
}

func TestRequirementCatalogBuildsOnlySelectedStrategyDependencies(t *testing.T) {
	factorA := &testFactor{id: "factor_a", version: "1"}
	factorB := &testFactor{id: "factor_b", version: "1"}
	factorRegistry := registry.New[FactorCalculator]()
	if err := factorRegistry.Register(factorA); err != nil {
		t.Fatal(err)
	}
	if err := factorRegistry.Register(factorB); err != nil {
		t.Fatal(err)
	}
	factorRegistry.Freeze()

	strategyA := &testStrategyModule{id: "strategy_a", version: "3", factors: []string{"factor_a"}}
	strategyB := &testStrategyModule{id: "strategy_b", version: "4", factors: []string{"factor_b", "factor_a"}}
	strategyRegistry := registry.New[Strategy]()
	if err := strategyRegistry.Register(strategyA); err != nil {
		t.Fatal(err)
	}
	if err := strategyRegistry.Register(strategyB); err != nil {
		t.Fatal(err)
	}
	strategyRegistry.Freeze()

	catalog, err := NewRegistryRequirementCatalog(strategyRegistry, factorRegistry, []domain.DataRequirement{{Dataset: "trading_calendar", Required: true}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := catalog.Resolve(context.Background(), domain.RunRequest{StrategyIDs: []string{"strategy_b", "strategy_a", "strategy_a"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.FactorIDs) != 2 || got.FactorIDs[0] != "factor_a" || got.FactorIDs[1] != "factor_b" {
		t.Fatalf("unexpected factor dependency set: %#v", got.FactorIDs)
	}
	if len(got.ByStrategy) != 2 || len(got.ByFactor) != 2 {
		t.Fatalf("missing strategy/factor requirements: %#v", got)
	}
	if len(got.Core) != 1 || got.Core[0].Dataset != "trading_calendar" {
		t.Fatalf("unexpected core requirements: %#v", got.Core)
	}
}

func TestRiskEngineDenialCannotBeOverriddenAndMissingDecisionFails(t *testing.T) {
	signals := []domain.CandidateSignal{{ID: "s1", Symbol: "600000.SH"}, {ID: "s2", Symbol: "000001.SZ"}}
	goodRegistry := registry.New[RiskRule]()
	if err := goodRegistry.Register(testRiskRule{id: "deny_rule", version: "risk-1", priority: 1, allow: false}); err != nil {
		t.Fatal(err)
	}
	if err := goodRegistry.Register(testRiskRule{id: "later_allow", version: "risk-2", priority: 2, allow: true}); err != nil {
		t.Fatal(err)
	}
	goodRegistry.Freeze()
	engine, err := NewRiskEngine(goodRegistry)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := engine.Assess(context.Background(), domain.RunRequest{}, domain.MarketSnapshot{}, domain.PortfolioSnapshot{}, signals, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, decision := range decisions {
		if decision.Allowed {
			t.Fatalf("risk denial was overridden: %#v", decision)
		}
		if len(decision.RuleRefs) != 2 {
			t.Fatalf("expected versioned rule attribution: %#v", decision.RuleRefs)
		}
	}

	badRegistry := registry.New[RiskRule]()
	if err := badRegistry.Register(testRiskRule{id: "incomplete_rule", version: "risk-1", priority: 1, allow: true, omit: true}); err != nil {
		t.Fatal(err)
	}
	badRegistry.Freeze()
	badEngine, err := NewRiskEngine(badRegistry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := badEngine.Assess(context.Background(), domain.RunRequest{}, domain.MarketSnapshot{}, domain.PortfolioSnapshot{}, signals, nil); err == nil {
		t.Fatal("expected missing per-signal decision to fail")
	}
}
