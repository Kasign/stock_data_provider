package modules

import (
	"context"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

// These are plugin contracts implemented by individual data-source/factor/strategy/risk modules.
// Composite engines adapt their registries to app.FactorEngine, app.StrategyEngine,
// and app.RiskEngine. The main runner never imports concrete modules.

type DataRequest struct {
	RunRequest   domain.RunRequest
	Universe     domain.UniverseSnapshot
	Requirements domain.RequirementSet
}

// DataSource adapters expose their supported datasets and source identity. The
// DataCollector/source-router owns precedence, per-field fallback, freshness,
// timeouts, rate limits, and provenance; the runner must not know source names.
type DataSource interface {
	ID() string
	Capabilities() []string
	Fetch(ctx context.Context, request DataRequest) (domain.RawSnapshot, error)
}

type FactorInput struct {
	Request  domain.RunRequest
	Universe domain.UniverseSnapshot
	Snapshot domain.MarketSnapshot
	Market   domain.MarketState
	Quality  domain.QualityReport
}

type FactorCalculator interface {
	ID() string
	Version() string
	Requirements() []domain.DataRequirement
	Calculate(ctx context.Context, input FactorInput) ([]domain.FactorValue, error)
}

type StrategyInput struct {
	Request  domain.RunRequest
	Universe domain.UniverseSnapshot
	Snapshot domain.MarketSnapshot
	Market   domain.MarketState
	Factors  []domain.FactorValue
	Quality  domain.QualityReport
}

type Strategy interface {
	ID() string
	Version() string
	// FactorIDs declares the factor modules this strategy consumes. The
	// RequirementCatalog unions these IDs before the factor engine runs.
	FactorIDs() []string
	Requirements() []domain.DataRequirement
	Evaluate(ctx context.Context, input StrategyInput) ([]domain.CandidateSignal, error)
}

type RiskInput struct {
	Request     domain.RunRequest
	Snapshot    domain.MarketSnapshot
	Portfolio   domain.PortfolioSnapshot
	Signals     []domain.CandidateSignal
	Tradability []domain.SignalDecision
}

// Each RiskRule must return exactly one decision for every supplied signal.
// Missing decisions are a rule implementation error; they are never interpreted
// as permission. A denied decision must include at least one human-auditable reason.
type RiskRule interface {
	ID() string
	Version() string
	Priority() int
	Evaluate(ctx context.Context, input RiskInput) ([]domain.SignalDecision, error)
}
