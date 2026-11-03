package app

import (
	"context"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

// Ports are application boundaries. Implementations live in adapters/modules,
// not in the runner. Interfaces should change only when the domain contract changes.

type Clock interface {
	Now() time.Time
}

type UniverseResolver interface {
	Resolve(ctx context.Context, request domain.RunRequest) (domain.UniverseSnapshot, error)
}

type RequirementCatalog interface {
	Resolve(ctx context.Context, request domain.RunRequest) (domain.RequirementSet, error)
}

type DataCollector interface {
	Collect(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, requirements domain.RequirementSet) (domain.RawSnapshot, error)
}

type Normalizer interface {
	Normalize(ctx context.Context, raw domain.RawSnapshot, request domain.RunRequest) (domain.MarketSnapshot, error)
}

type QualityAssessor interface {
	Assess(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, snapshot domain.MarketSnapshot, requirements domain.RequirementSet) (domain.QualityReport, error)
}

type MarketAnalyzer interface {
	Analyze(ctx context.Context, request domain.RunRequest, snapshot domain.MarketSnapshot) (domain.MarketState, error)
}

type FactorEngine interface {
	Calculate(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, snapshot domain.MarketSnapshot, market domain.MarketState, quality domain.QualityReport, requirements domain.RequirementSet) ([]domain.FactorValue, error)
}

type StrategyEngine interface {
	Evaluate(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, snapshot domain.MarketSnapshot, market domain.MarketState, factors []domain.FactorValue, quality domain.QualityReport) ([]domain.CandidateSignal, error)
}

type TradabilityChecker interface {
	Check(ctx context.Context, request domain.RunRequest, snapshot domain.MarketSnapshot, signals []domain.CandidateSignal) ([]domain.SignalDecision, error)
}

type PortfolioProvider interface {
	Load(ctx context.Context, request domain.RunRequest) (domain.PortfolioSnapshot, error)
}

type RiskEngine interface {
	Assess(ctx context.Context, request domain.RunRequest, snapshot domain.MarketSnapshot, portfolio domain.PortfolioSnapshot, signals []domain.CandidateSignal, tradability []domain.SignalDecision) ([]domain.SignalDecision, error)
}

type PositionPlanner interface {
	Plan(ctx context.Context, request domain.RunRequest, snapshot domain.MarketSnapshot, portfolio domain.PortfolioSnapshot, signals []domain.CandidateSignal, tradability []domain.SignalDecision, risk []domain.SignalDecision) ([]domain.TradePlan, []domain.PlanRejection, error)
}

type ExecutionAssessor interface {
	Assess(ctx context.Context, request domain.RunRequest, snapshot domain.MarketSnapshot, portfolio domain.PortfolioSnapshot, plans []domain.TradePlan) (domain.ExecutionAssessment, error)
}

// SnapshotStore persists immutable inputs and normalized datasets. IDs returned
// here are durable application snapshot IDs, not provider request IDs.
type SnapshotStore interface {
	// runID enables idempotent writes if a worker retries the same run.
	SaveRaw(ctx context.Context, runID string, snapshot domain.RawSnapshot) (string, error)
	SaveMarket(ctx context.Context, runID string, snapshot domain.MarketSnapshot) (string, error)
	SavePortfolio(ctx context.Context, runID string, snapshot domain.PortfolioSnapshot) (string, error)
}

type RunStore interface {
	Begin(ctx context.Context, request domain.RunRequest, startedAt time.Time) error
	AppendEvent(ctx context.Context, event domain.StageEvent) error
	Finish(ctx context.Context, result domain.RunResult) error
}

type Dependencies struct {
	Clock        Clock
	Universes    UniverseResolver
	Requirements RequirementCatalog
	Data         DataCollector
	Normalizer   Normalizer
	Quality      QualityAssessor
	Market       MarketAnalyzer
	Factors      FactorEngine
	Strategies   StrategyEngine
	Tradability  TradabilityChecker
	Portfolio    PortfolioProvider
	Risk         RiskEngine
	Planner      PositionPlanner
	Execution    ExecutionAssessor
	Snapshots    SnapshotStore
	Store        RunStore
}
