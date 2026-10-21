// Command demo runs the analysis pipeline using in-memory mock adapters.
// It is intended for local smoke testing only and never fetches real market data.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/app"
	"example.com/stock-analysis-flow/internal/analysis/domain"
)

const demoSymbol = "600000.SH"

// All adapters below return deterministic fake data. Replace them with real adapters
// only after the source, persistence, quality, strategy, and risk rules are implemented.
type fixedClock struct{ now time.Time }

func (c fixedClock) Now() time.Time { return c.now }

type universeResolver struct{}

func (universeResolver) Resolve(_ context.Context, r domain.RunRequest) (domain.UniverseSnapshot, error) {
	return domain.UniverseSnapshot{ID: r.UniverseID, AsOf: r.AsOf, Version: "demo-universe-v1", Securities: []domain.Security{{Symbol: demoSymbol, Exchange: "SSE", AssetType: "equity", Board: "main", TradingState: "normal"}}}, nil
}

type requirementCatalog struct{}

func (requirementCatalog) Resolve(_ context.Context, r domain.RunRequest) (domain.RequirementSet, error) {
	byStrategy := make(map[string][]domain.DataRequirement, len(r.StrategyIDs))
	for _, id := range r.StrategyIDs {
		byStrategy[id] = []domain.DataRequirement{{Dataset: "daily_bars", Fields: []string{"open", "high", "low", "close", "volume", "amount"}, Required: true, Consumer: id}}
	}
	return domain.RequirementSet{Core: []domain.DataRequirement{{Dataset: "daily_bars", Required: true}}, ByStrategy: byStrategy}, nil
}

type dataCollector struct{}

func (dataCollector) Collect(_ context.Context, r domain.RunRequest, _ domain.UniverseSnapshot, _ domain.RequirementSet) (domain.RawSnapshot, error) {
	fetchedAt := r.AsOf.Add(-time.Minute)
	volume, amount := 1200000.0, 12000000.0
	return domain.RawSnapshot{
		AsOf:        r.AsOf,
		SourceNotes: []domain.SourceNote{{Dataset: "daily_bars", Source: "demo-mock", FetchedAt: fetchedAt, AvailableAt: r.AsOf, QualityStatus: "mock", AdjustmentBasis: "unadjusted"}},
		Bars:        []domain.RawBar{{Symbol: demoSymbol, TradeDate: r.AsOf, Open: 9.90, High: 10.20, Low: 9.80, Close: 10.00, Volume: &volume, Amount: &amount, Source: "demo-mock", FetchedAt: fetchedAt, AvailableAt: r.AsOf, AdjustBasis: "unadjusted"}},
	}, nil
}

type normalizer struct{}

func (normalizer) Normalize(_ context.Context, raw domain.RawSnapshot, _ domain.RunRequest) (domain.MarketSnapshot, error) {
	bars := make(map[string][]domain.Bar)
	for _, b := range raw.Bars {
		bars[b.Symbol] = append(bars[b.Symbol], domain.Bar{Symbol: b.Symbol, TradeDate: b.TradeDate, Open: b.Open, High: b.High, Low: b.Low, Close: b.Close, Volume: b.Volume, Amount: b.Amount, Source: b.Source, FetchedAt: b.FetchedAt, AvailableAt: b.AvailableAt, Basis: b.AdjustBasis})
	}
	return domain.MarketSnapshot{AsOf: raw.AsOf, Bars: bars, Sources: raw.SourceNotes}, nil
}

type qualityAssessor struct{}

func (qualityAssessor) Assess(_ context.Context, r domain.RunRequest, _ domain.UniverseSnapshot, _ domain.MarketSnapshot, _ domain.RequirementSet) (domain.QualityReport, error) {
	eligible := make(map[string]bool, len(r.StrategyIDs))
	for _, id := range r.StrategyIDs {
		eligible[id] = true
	}
	return domain.QualityReport{CorePass: true, Partial: true, StrategyEligible: eligible, CheckedAt: r.AsOf, PolicyVersion: r.DataPolicyVersion, Issues: []domain.DataIssue{{Code: "demo_data_only", Severity: "warning", Message: "本次运行使用内存模拟数据，不代表真实行情质量"}}}, nil
}

type marketAnalyzer struct{}

func (marketAnalyzer) Analyze(_ context.Context, r domain.RunRequest, _ domain.MarketSnapshot) (domain.MarketState, error) {
	return domain.MarketState{AsOf: r.AsOf, Regime: "demo_neutral", Metrics: map[string]float64{"demo_market_score": 50}, Evidence: []string{"mock market state"}, AlgorithmVersion: "demo-market-v1"}, nil
}

type factorEngine struct{}

func (factorEngine) Calculate(_ context.Context, r domain.RunRequest, u domain.UniverseSnapshot, s domain.MarketSnapshot, _ domain.MarketState, _ domain.QualityReport, _ domain.RequirementSet) ([]domain.FactorValue, error) {
	var out []domain.FactorValue
	for _, security := range u.Securities {
		bars := s.Bars[security.Symbol]
		if len(bars) == 0 {
			continue
		}
		value := bars[len(bars)-1].Close
		out = append(out, domain.FactorValue{Symbol: security.Symbol, FactorID: "demo.close", FactorVersion: "demo-factor-v1", Value: &value, Unit: "CNY", AsOf: r.AsOf, InputQuality: "mock"})
	}
	return out, nil
}

type strategyEngine struct{}

func (strategyEngine) Evaluate(_ context.Context, r domain.RunRequest, u domain.UniverseSnapshot, _ domain.MarketSnapshot, _ domain.MarketState, _ []domain.FactorValue, _ domain.QualityReport) ([]domain.CandidateSignal, error) {
	if len(r.StrategyIDs) == 0 || len(u.Securities) == 0 {
		return nil, nil
	}
	validUntil := r.AsOf.Add(48 * time.Hour)
	return []domain.CandidateSignal{{ID: "demo-signal-001", Symbol: demoSymbol, StrategyID: r.StrategyIDs[0], StrategyVersion: "demo-strategy-v1", Direction: "long", SignalAt: r.AsOf, ValidUntil: validUntil, EntryRule: "示例：下一交易日开盘后满足条件才考虑入场", ExitRule: "示例：止损或策略退出条件触发", InvalidationRule: "示例：趋势条件失效", ExpectedHoldDays: 3}}, nil
}

type tradabilityChecker struct{}

func (tradabilityChecker) Check(_ context.Context, _ domain.RunRequest, _ domain.MarketSnapshot, signals []domain.CandidateSignal) ([]domain.SignalDecision, error) {
	out := make([]domain.SignalDecision, 0, len(signals))
	for _, s := range signals {
		out = append(out, domain.SignalDecision{SignalID: s.ID, Allowed: true, Reasons: []string{"仅为演示门禁，不代表真实可交易性已验证"}})
	}
	return out, nil
}

type portfolioProvider struct{}

func (portfolioProvider) Load(_ context.Context, r domain.RunRequest) (domain.PortfolioSnapshot, error) {
	return domain.PortfolioSnapshot{PortfolioID: r.PortfolioID, AsOf: r.AsOf, Currency: "CNY", AvailableCash: 100000, TotalEquity: 100000, Version: "demo-paper-v1"}, nil
}

type riskEngine struct{}

func (riskEngine) Assess(_ context.Context, _ domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, signals []domain.CandidateSignal, _ []domain.SignalDecision) ([]domain.SignalDecision, error) {
	out := make([]domain.SignalDecision, 0, len(signals))
	for _, s := range signals {
		out = append(out, domain.SignalDecision{SignalID: s.ID, Allowed: true, RuleIDs: []string{"demo-risk-only"}, RuleRefs: []domain.RuleRef{{ID: "demo-risk-only", Version: "v1"}}})
	}
	return out, nil
}

type positionPlanner struct{}

func (positionPlanner) Plan(_ context.Context, r domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, signals []domain.CandidateSignal, _ []domain.SignalDecision, _ []domain.SignalDecision) ([]domain.TradePlan, []domain.PlanRejection, error) {
	out := make([]domain.TradePlan, 0, len(signals))
	for _, s := range signals {
		executeAt := time.Date(r.AsOf.Year(), r.AsOf.Month(), r.AsOf.Day()+1, 9, 30, 0, 0, r.AsOf.Location())
		expireAt := executeAt.Add(5*time.Hour + 30*time.Minute)
		stop, target := 9.50, 11.00
		out = append(out, domain.TradePlan{SignalID: s.ID, Symbol: s.Symbol, Side: "buy", Quantity: 100, ReferencePrice: 10.00, StopPrice: &stop, TargetPrice: &target, EarliestExecuteAt: executeAt, ExpireAt: expireAt, RiskBudget: 100, CostEstimate: 5, RuleVersion: r.TradingRulesVersion})
	}
	return out, nil, nil
}

type executionAssessor struct{}

func (executionAssessor) Assess(_ context.Context, _ domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, plans []domain.TradePlan) (domain.ExecutionAssessment, error) {
	orders := make([]domain.ExecutionOrderAssessment, 0, len(plans))
	for _, p := range plans {
		orders = append(orders, domain.ExecutionOrderAssessment{SignalID: p.SignalID, Status: domain.ExecutionPending, RequestedQuantity: p.Quantity, RemainingQuantity: p.Quantity, Reason: "等待未来行情；演示程序不模拟真实成交"})
	}
	return domain.ExecutionAssessment{ModelVersion: "demo-execution-v1", Orders: orders, Warnings: []string{"全部数据和判定均为 mock，仅验证流程串联"}}, nil
}

type snapshotStore struct{}

func (snapshotStore) SaveRaw(_ context.Context, runID string, _ domain.RawSnapshot) (string, error) {
	return runID + "-raw", nil
}
func (snapshotStore) SaveMarket(_ context.Context, runID string, _ domain.MarketSnapshot) (string, error) {
	return runID + "-market", nil
}
func (snapshotStore) SavePortfolio(_ context.Context, runID string, _ domain.PortfolioSnapshot) (string, error) {
	return runID + "-portfolio", nil
}

type runStore struct{}

func (runStore) Begin(context.Context, domain.RunRequest, time.Time) error { return nil }
func (runStore) AppendEvent(context.Context, domain.StageEvent) error      { return nil }
func (runStore) Finish(context.Context, domain.RunResult) error            { return nil }

func main() {
	loc := time.FixedZone("Asia/Shanghai", 8*60*60)
	asOf := time.Date(2026, 10, 20, 15, 0, 0, 0, loc)
	req := domain.RunRequest{
		RunID: "demo-run-001", TaskType: domain.TaskPostMarketScan, AsOf: asOf,
		UniverseID: "demo-universe", Symbols: []string{"600000"}, PortfolioID: "demo-paper", StrategyIDs: []string{"demo-trend"}, MarketID: "CN-A",
		StrategyConfigVersion: "demo-strategy-v1", RiskConfigVersion: "demo-risk-v1", TradingRulesVersion: "demo-rules-v1", DataPolicyVersion: "demo-quality-v1",
	}
	deps := app.Dependencies{
		Clock: fixedClock{now: asOf.Add(time.Hour)}, Universes: universeResolver{}, Requirements: requirementCatalog{}, Data: dataCollector{},
		Normalizer: normalizer{}, Quality: qualityAssessor{}, Market: marketAnalyzer{}, Factors: factorEngine{}, Strategies: strategyEngine{},
		Tradability: tradabilityChecker{}, Portfolio: portfolioProvider{}, Risk: riskEngine{}, Planner: positionPlanner{}, Execution: executionAssessor{},
		Snapshots: snapshotStore{}, Store: runStore{},
	}
	runner, err := app.NewRunner(deps)
	if err != nil {
		fail(err)
	}
	result, err := runner.Run(context.Background(), req)
	if err != nil {
		fail(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(result); err != nil {
		fail(err)
	}
	fmt.Fprintf(os.Stderr, "\n演示完成：status=%s，signals=%d，plans=%d。数据全部为模拟值，不能用于投资决策。\n", result.Status, len(result.Signals), len(result.Plans))
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "demo failed:", err)
	os.Exit(1)
}
