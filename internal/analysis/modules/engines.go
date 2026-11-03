package modules

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"

	"example.com/stock-analysis-flow/internal/analysis/app"
	"example.com/stock-analysis-flow/internal/analysis/domain"
	"example.com/stock-analysis-flow/internal/analysis/registry"
)

// FactorEngine executes registered calculators in a stable order and stamps
// module identity/version onto every value so persisted results are auditable.
type FactorEngine struct {
	calculators *registry.Registry[FactorCalculator]
}

func NewFactorEngine(r *registry.Registry[FactorCalculator]) (*FactorEngine, error) {
	if r == nil {
		return nil, errors.New("factor registry is required")
	}
	if !r.IsFrozen() {
		return nil, errors.New("factor registry must be frozen before engine construction")
	}
	for _, calculator := range r.AllSorted() {
		if strings.TrimSpace(calculator.Version()) == "" {
			return nil, fmt.Errorf("factor %s has an empty version", calculator.ID())
		}
	}
	return &FactorEngine{calculators: r}, nil
}

func (e *FactorEngine) Calculate(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, snapshot domain.MarketSnapshot, market domain.MarketState, quality domain.QualityReport, requirements domain.RequirementSet) ([]domain.FactorValue, error) {
	out := make([]domain.FactorValue, 0)
	input := FactorInput{Request: request, Universe: universe, Snapshot: snapshot, Market: market, Quality: quality}
	allowedSymbols := make(map[string]struct{}, len(universe.Securities))
	for _, security := range universe.Securities {
		allowedSymbols[security.Symbol] = struct{}{}
	}
	factorIDs := uniqueStrings(requirements.FactorIDs)
	sort.Strings(factorIDs)
	seenValues := make(map[string]struct{})
	for _, factorID := range factorIDs {
		calculator, err := e.calculators.Get(factorID)
		if err != nil {
			return nil, fmt.Errorf("required factor %s is not registered: %w", factorID, err)
		}
		values, err := calculator.Calculate(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("factor %s@%s: %w", calculator.ID(), calculator.Version(), err)
		}
		for _, value := range values {
			if value.Symbol == "" {
				return nil, fmt.Errorf("factor %s returned value without symbol", calculator.ID())
			}
			if _, exists := allowedSymbols[value.Symbol]; !exists {
				return nil, fmt.Errorf("factor %s returned symbol %q outside the resolved universe", calculator.ID(), value.Symbol)
			}
			if value.Value == nil && strings.TrimSpace(value.MissingReason) == "" {
				return nil, fmt.Errorf("factor %s returned nil value without missing_reason for %s", calculator.ID(), value.Symbol)
			}
			if value.Value != nil && (math.IsNaN(*value.Value) || math.IsInf(*value.Value, 0)) {
				return nil, fmt.Errorf("factor %s returned non-finite value for %s; mark it missing with a reason instead", calculator.ID(), value.Symbol)
			}
			if strings.TrimSpace(value.InputQuality) == "" {
				return nil, fmt.Errorf("factor %s returned empty input_quality for %s", calculator.ID(), value.Symbol)
			}
			if value.FactorID != "" && value.FactorID != calculator.ID() {
				return nil, fmt.Errorf("factor %s returned mismatched factor_id %q", calculator.ID(), value.FactorID)
			}
			value.FactorID = calculator.ID()
			value.FactorVersion = calculator.Version()
			if value.AsOf.IsZero() {
				return nil, fmt.Errorf("factor %s returned missing as_of for %s", calculator.ID(), value.Symbol)
			}
			if value.AsOf.After(request.AsOf) {
				return nil, fmt.Errorf("factor %s returned future as_of for %s", calculator.ID(), value.Symbol)
			}
			key := value.Symbol + "\x00" + calculator.ID()
			if _, duplicate := seenValues[key]; duplicate {
				return nil, fmt.Errorf("factor %s returned duplicate values for %s", calculator.ID(), value.Symbol)
			}
			seenValues[key] = struct{}{}
			out = append(out, value)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Symbol != out[j].Symbol {
			return out[i].Symbol < out[j].Symbol
		}
		if out[i].FactorID != out[j].FactorID {
			return out[i].FactorID < out[j].FactorID
		}
		return out[i].AsOf.Before(out[j].AsOf)
	})
	return out, nil
}

// StrategyEngine executes only strategies explicitly requested and approved by
// the quality gate. A missing eligibility entry is a hard deny, not a default pass.
type StrategyEngine struct{ strategies *registry.Registry[Strategy] }

func NewStrategyEngine(r *registry.Registry[Strategy]) (*StrategyEngine, error) {
	if r == nil {
		return nil, errors.New("strategy registry is required")
	}
	if !r.IsFrozen() {
		return nil, errors.New("strategy registry must be frozen before engine construction")
	}
	for _, strategy := range r.AllSorted() {
		if strings.TrimSpace(strategy.Version()) == "" {
			return nil, fmt.Errorf("strategy %s has an empty version", strategy.ID())
		}
	}
	return &StrategyEngine{strategies: r}, nil
}

func (e *StrategyEngine) Evaluate(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, snapshot domain.MarketSnapshot, market domain.MarketState, factors []domain.FactorValue, quality domain.QualityReport) ([]domain.CandidateSignal, error) {
	ids := append([]string(nil), request.StrategyIDs...)
	sort.Strings(ids)
	out := make([]domain.CandidateSignal, 0)
	allowedSymbols := make(map[string]struct{}, len(universe.Securities))
	for _, security := range universe.Securities {
		allowedSymbols[security.Symbol] = struct{}{}
	}
	for _, id := range ids {
		strategy, err := e.strategies.Get(id)
		if err != nil {
			return nil, fmt.Errorf("requested strategy %s is not registered: %w", id, err)
		}
		eligible, found := quality.StrategyEligible[id]
		if !found || !eligible {
			continue
		}
		input := StrategyInput{Request: request, Universe: universe, Snapshot: snapshot, Market: market, Factors: factors, Quality: quality}
		signals, err := strategy.Evaluate(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("strategy %s@%s: %w", strategy.ID(), strategy.Version(), err)
		}
		for _, signal := range signals {
			if signal.Symbol == "" || signal.ID == "" {
				return nil, fmt.Errorf("strategy %s returned signal missing id or symbol", id)
			}
			if _, exists := allowedSymbols[signal.Symbol]; !exists {
				return nil, fmt.Errorf("strategy %s returned symbol %q outside the resolved universe", id, signal.Symbol)
			}
			if signal.SignalAt.IsZero() || signal.SignalAt.After(request.AsOf) {
				return nil, fmt.Errorf("strategy %s returned invalid/future signal_at for %q", id, signal.ID)
			}
			if signal.ValidUntil.IsZero() || !signal.ValidUntil.After(request.AsOf) {
				return nil, fmt.Errorf("strategy %s returned expired or missing valid_until for %q", id, signal.ID)
			}
			if signal.ValidUntil.Before(signal.SignalAt) {
				return nil, fmt.Errorf("strategy %s returned valid_until before signal_at for %q", id, signal.ID)
			}
			if signal.ExpectedHoldDays < 0 {
				return nil, fmt.Errorf("strategy %s returned negative expected_hold_days for %q", id, signal.ID)
			}
			if signal.ReferencePrice != nil && (*signal.ReferencePrice <= 0 || math.IsNaN(*signal.ReferencePrice) || math.IsInf(*signal.ReferencePrice, 0)) {
				return nil, fmt.Errorf("strategy %s returned invalid reference_price for %q", id, signal.ID)
			}
			if strings.TrimSpace(signal.Direction) == "" || strings.TrimSpace(signal.EntryRule) == "" || strings.TrimSpace(signal.ExitRule) == "" || strings.TrimSpace(signal.InvalidationRule) == "" {
				return nil, fmt.Errorf("strategy %s returned incomplete trade signal %q (direction/entry/exit/invalidation required)", id, signal.ID)
			}
			if signal.StrategyID != "" && signal.StrategyID != strategy.ID() {
				return nil, fmt.Errorf("strategy %s returned mismatched strategy_id %q", id, signal.StrategyID)
			}
			signal.StrategyID = strategy.ID()
			signal.StrategyVersion = strategy.Version()
			out = append(out, signal)
		}
	}
	seen := make(map[string]struct{}, len(out))
	for _, signal := range out {
		if _, exists := seen[signal.ID]; exists {
			return nil, fmt.Errorf("duplicate signal id %q", signal.ID)
		}
		seen[signal.ID] = struct{}{}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].StrategyID != out[j].StrategyID {
			return out[i].StrategyID < out[j].StrategyID
		}
		if out[i].Symbol != out[j].Symbol {
			return out[i].Symbol < out[j].Symbol
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// RiskEngine combines risk-rule output using AND semantics. A denial from any
// rule remains a denial; a later rule can never re-approve the same signal.
type RiskEngine struct{ rules *registry.Registry[RiskRule] }

func NewRiskEngine(r *registry.Registry[RiskRule]) (*RiskEngine, error) {
	if r == nil {
		return nil, errors.New("risk rule registry is required")
	}
	if !r.IsFrozen() {
		return nil, errors.New("risk rule registry must be frozen before engine construction")
	}
	if r.Len() == 0 {
		return nil, errors.New("at least one risk rule is required")
	}
	for _, rule := range r.AllSorted() {
		if strings.TrimSpace(rule.Version()) == "" {
			return nil, fmt.Errorf("risk rule %s has an empty version", rule.ID())
		}
	}
	return &RiskEngine{rules: r}, nil
}

func (e *RiskEngine) Assess(ctx context.Context, request domain.RunRequest, snapshot domain.MarketSnapshot, portfolio domain.PortfolioSnapshot, signals []domain.CandidateSignal, tradability []domain.SignalDecision) ([]domain.SignalDecision, error) {
	rules := e.rules.AllSorted()
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority() != rules[j].Priority() {
			return rules[i].Priority() < rules[j].Priority()
		}
		return rules[i].ID() < rules[j].ID()
	})
	decisions := make(map[string]domain.SignalDecision, len(signals))
	for _, signal := range signals {
		decisions[signal.ID] = domain.SignalDecision{SignalID: signal.ID, Allowed: true}
	}
	input := RiskInput{Request: request, Snapshot: snapshot, Portfolio: portfolio, Signals: signals, Tradability: tradability}
	for _, rule := range rules {
		results, err := rule.Evaluate(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("risk rule %s: %w", rule.ID(), err)
		}
		seenByRule := make(map[string]struct{}, len(results))
		for _, candidate := range results {
			decision, exists := decisions[candidate.SignalID]
			if !exists {
				return nil, fmt.Errorf("risk rule %s returned unknown signal id %q", rule.ID(), candidate.SignalID)
			}
			if _, duplicate := seenByRule[candidate.SignalID]; duplicate {
				return nil, fmt.Errorf("risk rule %s returned duplicate decision for signal %q", rule.ID(), candidate.SignalID)
			}
			seenByRule[candidate.SignalID] = struct{}{}
			if !candidate.Allowed {
				if len(uniqueStrings(candidate.Reasons)) == 0 {
					return nil, fmt.Errorf("risk rule %s denied signal %q without a reason", rule.ID(), candidate.SignalID)
				}
				decision.Allowed = false
			}
			decision.Reasons = uniqueStrings(append(decision.Reasons, candidate.Reasons...))
			// Attribution is stamped by the composite engine, not trusted from
			// plugin-supplied RuleIDs. This binds every evaluation to a version.
			decision.RuleIDs = uniqueStrings(append(decision.RuleIDs, rule.ID()))
			decision.RuleRefs = mergeRuleRefs(decision.RuleRefs, []domain.RuleRef{{ID: rule.ID(), Version: rule.Version()}})
			decisions[candidate.SignalID] = decision
		}
		if len(seenByRule) != len(signals) {
			return nil, fmt.Errorf("risk rule %s returned %d decisions for %d signals; every signal must be explicitly checked", rule.ID(), len(seenByRule), len(signals))
		}
	}
	out := make([]domain.SignalDecision, 0, len(decisions))
	for _, signal := range signals {
		out = append(out, decisions[signal.ID])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SignalID < out[j].SignalID })
	return out, nil
}

func mergeRuleRefs(existing, incoming []domain.RuleRef) []domain.RuleRef {
	seen := make(map[string]struct{}, len(existing)+len(incoming))
	out := make([]domain.RuleRef, 0, len(existing)+len(incoming))
	for _, ref := range append(append([]domain.RuleRef(nil), existing...), incoming...) {
		ref.ID = strings.TrimSpace(ref.ID)
		ref.Version = strings.TrimSpace(ref.Version)
		if ref.ID == "" || ref.Version == "" {
			continue
		}
		key := ref.ID + "\x00" + ref.Version
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ID != out[j].ID {
			return out[i].ID < out[j].ID
		}
		return out[i].Version < out[j].Version
	})
	return out
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

var _ app.FactorEngine = (*FactorEngine)(nil)
var _ app.StrategyEngine = (*StrategyEngine)(nil)
var _ app.RiskEngine = (*RiskEngine)(nil)
