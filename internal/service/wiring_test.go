package service

import (
	"context"
	"sort"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

type memoryStore struct {
	requests  []domain.RunRequest
	result    domain.RunResult
	events    []domain.StageEvent
	raw       int
	market    int
	portfolio int
}

func (m *memoryStore) Begin(_ context.Context, r domain.RunRequest, _ time.Time) error {
	m.requests = append(m.requests, r)
	return nil
}
func (m *memoryStore) AppendEvent(_ context.Context, e domain.StageEvent) error {
	m.events = append(m.events, e)
	return nil
}
func (m *memoryStore) Finish(_ context.Context, r domain.RunResult) error { m.result = r; return nil }
func (m *memoryStore) SaveRaw(_ context.Context, runID string, s domain.RawSnapshot) (string, error) {
	m.raw++
	return runID + "-raw", nil
}
func (m *memoryStore) SaveMarket(_ context.Context, runID string, s domain.MarketSnapshot) (string, error) {
	m.market++
	return runID + "-market", nil
}
func (m *memoryStore) SavePortfolio(_ context.Context, runID string, s domain.PortfolioSnapshot) (string, error) {
	m.portfolio++
	return runID + "-portfolio", nil
}

func TestRunnerWithStoresRunsRealAdapterFlowAndFailsClosed(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	func TestRunnerWithInjectedDataCollectorRunsFlowAndFailsClosed(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	asOf := time.Date(2026, 10, 20, 15, 5, 0, 0, loc)
	bars := make([]domain.RawBar, 0, 21)
	for i := 20; i >= 0; i-- {
		closePrice := float64(100 + 20 - i)
		bars = append(bars, domain.RawBar{
			Symbol: "600519", TradeDate: asOf.AddDate(0, 0, -i),
			Open: closePrice * 0.995, High: closePrice * 1.01,
			Low: closePrice * 0.99, Close: closePrice,
		})
	}
	collector := fixedDataCollector{snapshot: domain.RawSnapshot{
		AsOf: asOf,
		Bars: bars,
		SourceNotes: []domain.SourceNote{{
			Dataset: "daily_bars", Symbol: "600519", QualityStatus: "partial_metadata",
			Diagnostic: "test fixture intentionally omits provider metadata",
		}},
	}}
	store := &memoryStore{}
	runner, err := NewRunnerWithDataCollector(collector, store, store)
	if err != nil {
		t.Fatal(err)
	}
	result, err := runner.Run(context.Background(), domain.RunRequest{
		RunID: "integration-test-run", TaskType: domain.TaskPostMarketScan, AsOf: asOf,
		UniverseID: "test-universe", Symbols: []string{"600519"}, PortfolioID: "unconfigured",
		MarketID: "CN-A", StrategyIDs: []string{StrategyTrendFollowing},
		StrategyConfigVersion: "strategy-v1", RiskConfigVersion: "risk-v1",
		TradingRulesVersion: "rules-v1", DataPolicyVersion: "quality-v1",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !result.Quality.CorePass {
		t.Fatalf("expected core quality gate pass, issues=%+v", result.Quality.Issues)
	}
	if result.Status != domain.RunStatusPartial {
		t.Fatalf("expected partial due incomplete source metadata, got %s", result.Status)
	}
	if len(result.Signals) != 1 {
		t.Fatalf("expected one trend candidate, got %d", len(result.Signals))
	}
	if len(result.Tradability) != 1 || result.Tradability[0].Allowed {
		t.Fatalf("missing fail-closed tradability decision: %+v", result.Tradability)
	}
	if len(result.Plans) != 0 {
		t.Fatalf("unsafe MVP must not produce a trade plan: %+v", result.Plans)
	}
	if store.raw != 1 || store.market != 1 || store.portfolio != 1 {
		t.Fatalf("expected raw/market/portfolio persistence, got %d/%d/%d", store.raw, store.market, store.portfolio)
	}
	if len(store.events) == 0 || store.result.RunID != result.RunID {
		t.Fatalf("run audit/result not persisted")
	}
	stages := make([]string, 0, len(store.events))
	for _, event := range store.events {
		stages = append(stages, event.Stage)
	}
	sort.Strings(stages)
	if !containsStage(stages, "quality_gate") || !containsStage(stages, "strategy_evaluation") || !containsStage(stages, "portfolio_planning") {
		t.Fatalf("expected key stages, got %v", stages)
	}
}
kground(), domain.RunRequest{AsOf: time.Now(), UniverseID: "u", Symbols: []string{"600519.SH"}})
	if err == nil {
		t.Fatal("expected suffixed symbol to be rejected; this adapter expects six digit A-share codes")
	}
}

type fixedDataCollector struct {
	snapshot domain.RawSnapshot
}

func (c fixedDataCollector) Collect(context.Context, domain.RunRequest, domain.UniverseSnapshot, domain.RequirementSet) (domain.RawSnapshot, error) {
	return c.snapshot, nil
}

type injectedDataCollector struct{}

func (injectedDataCollector) Collect(context.Context, domain.RunRequest, domain.UniverseSnapshot, domain.RequirementSet) (domain.RawSnapshot, error) {
	return domain.RawSnapshot{}, nil
}

func TestRunnerWithDataCollectorAcceptsRootOwnedAdapter(t *testing.T) {
	store := &memoryStore{}
	runner, err := NewRunnerWithDataCollector(injectedDataCollector{}, store, store)
	if err != nil {
		t.Fatalf("wire Root-owned DataCollector: %v", err)
	}
	if runner == nil {
		t.Fatal("NewRunnerWithDataCollector returned nil runner")
	}
}
