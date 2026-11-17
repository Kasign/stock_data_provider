// Package service contains the concrete, intentionally conservative MVP wiring.
// It creates a real analysis flow around the existing Python API and persistence
// adapters. It is not a claim that the baseline strategy has predictive edge.
package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/marketdata"
	"example.com/stock-analysis-flow/internal/adapters/pythonapi"
	"example.com/stock-analysis-flow/internal/analysis/app"
	"example.com/stock-analysis-flow/internal/analysis/domain"
	"example.com/stock-analysis-flow/internal/analysis/modules"
	"example.com/stock-analysis-flow/internal/analysis/registry"
)

const (
	StrategyTrendFollowing = "trend-following"
	factorMADistance       = "trend.ma20_distance_pct"
	factorReturn5          = "trend.return_5d_pct"
	factorReturn20         = "trend.return_20d_pct"
)

type clock struct{}

func (clock) Now() time.Time { return time.Now().UTC() }

type universeResolver struct{}

func (universeResolver) Resolve(ctx context.Context, req domain.RunRequest) (domain.UniverseSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return domain.UniverseSnapshot{}, err
	}
	symbols := append([]string(nil), req.Symbols...)
	if len(symbols) == 0 && req.Parameters != nil {
		for _, s := range strings.Split(req.Parameters["symbols"], ",") {
			if strings.TrimSpace(s) != "" {
				symbols = append(symbols, strings.TrimSpace(s))
			}
		}
	}
	if len(symbols) == 0 {
		return domain.UniverseSnapshot{}, errors.New("symbols is required; provide 1..100 A-share codes")
	}
	if len(symbols) > 100 {
		return domain.UniverseSnapshot{}, errors.New("at most 100 symbols are allowed per request")
	}
	seen := make(map[string]struct{}, len(symbols))
	securities := make([]domain.Security, 0, len(symbols))
	for _, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if !validSymbol(symbol) {
			return domain.UniverseSnapshot{}, fmt.Errorf("invalid A-share symbol %q", symbol)
		}
		if _, ok := seen[symbol]; ok {
			continue
		}
		seen[symbol] = struct{}{}
		securities = append(securities, domain.Security{
			Symbol: symbol, Exchange: inferExchange(symbol), AssetType: "equity", Board: inferBoard(symbol),
			TradingState: "unknown", Attributes: map[string]string{"trade_status_source": "not_provided_by_upstream"},
		})
	}
	if len(securities) == 0 {
		return domain.UniverseSnapshot{}, errors.New("symbols list contains no valid symbols")
	}
	sort.Slice(securities, func(i, j int) bool { return securities[i].Symbol < securities[j].Symbol })
	return domain.UniverseSnapshot{ID: req.UniverseID, AsOf: req.AsOf, Version: "request-universe-v1", Securities: securities}, nil
}

func validSymbol(s string) bool {
	if len(s) != 6 {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
func inferExchange(s string) string {
	switch s[0] {
	case '5', '6', '9':
		return "SSE"
	case '4', '8':
		return "BSE"
	default:
		return "SZSE"
	}
}
func inferBoard(s string) string {
	if s[0] == '4' || s[0] == '8' {
		return "beijing"
	}
	if s[0] == '3' {
		return "chinext"
	}
	if s[0] == '6' && strings.HasPrefix(s, "688") {
		return "star"
	}
	return "unknown"
}

type normalizer struct{}

func (normalizer) Normalize(ctx context.Context, raw domain.RawSnapshot, req domain.RunRequest) (domain.MarketSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return domain.MarketSnapshot{}, err
	}
	out := domain.MarketSnapshot{AsOf: raw.AsOf, Quotes: make(map[string]domain.Quote), Bars: make(map[string][]domain.Bar), IndexBars: make(map[string][]domain.Bar), Sources: append([]domain.SourceNote(nil), raw.SourceNotes...)}
	for _, r := range raw.Quotes {
		if r.Symbol == "" || r.CurrentPrice <= 0 || math.IsNaN(r.CurrentPrice) || math.IsInf(r.CurrentPrice, 0) {
			continue
		}
		out.Quotes[r.Symbol] = domain.Quote{Symbol: r.Symbol, Name: r.Name, CurrentPrice: r.CurrentPrice, Change: r.Change, ChangePercent: r.ChangePercent, Open: r.Open, High: r.High, Low: r.Low, PrevClose: r.PrevClose, Volume: r.Volume, Amount: r.Amount, QuoteTime: r.QuoteTime, Source: r.Source, FetchedAt: r.FetchedAt, AvailableAt: r.AvailableAt}
	}
	for _, r := range raw.Bars {
		if !validRawBar(r, req.AsOf) {
			continue
		}
		out.Bars[r.Symbol] = append(out.Bars[r.Symbol], rawToBar(r))
	}
	for _, r := range raw.IndexBars {
		if !validRawBar(r, req.AsOf) {
			continue
		}
		out.IndexBars[r.Symbol] = append(out.IndexBars[r.Symbol], rawToBar(r))
	}
	for symbol := range out.Bars {
		sort.Slice(out.Bars[symbol], func(i, j int) bool { return out.Bars[symbol][i].TradeDate.Before(out.Bars[symbol][j].TradeDate) })
	}
	for symbol := range out.IndexBars {
		sort.Slice(out.IndexBars[symbol], func(i, j int) bool {
			return out.IndexBars[symbol][i].TradeDate.Before(out.IndexBars[symbol][j].TradeDate)
		})
	}
	return out, nil
}
func validRawBar(b domain.RawBar, asOf time.Time) bool {
	return b.Symbol != "" && !b.TradeDate.IsZero() && !b.TradeDate.After(asOf) && b.Open > 0 && b.High >= b.Low && b.Low > 0 && b.Close > 0 && b.High >= b.Open && b.High >= b.Close && b.Low <= b.Open && b.Low <= b.Close
}
func rawToBar(r domain.RawBar) domain.Bar {
	return domain.Bar{Symbol: r.Symbol, TradeDate: r.TradeDate, Open: r.Open, High: r.High, Low: r.Low, Close: r.Close, Volume: r.Volume, Amount: r.Amount, Source: r.Source, FetchedAt: r.FetchedAt, AvailableAt: r.AvailableAt, Basis: r.AdjustBasis, MissingFields: append([]string(nil), r.MissingFields...)}
}

type qualityAssessor struct{}

func (qualityAssessor) Assess(ctx context.Context, req domain.RunRequest, universe domain.UniverseSnapshot, snapshot domain.MarketSnapshot, _ domain.RequirementSet) (domain.QualityReport, error) {
	if err := ctx.Err(); err != nil {
		return domain.QualityReport{}, err
	}
	eligible := make(map[string]bool, len(req.StrategyIDs))
	blockers := make(map[string][]string)
	goodSymbols := 0
	for _, sec := range universe.Securities {
		if len(snapshot.Bars[sec.Symbol]) >= 21 {
			goodSymbols++
		}
	}
	corePass := goodSymbols > 0
	partial := false
	issues := make([]domain.DataIssue, 0)
	if !corePass {
		issues = append(issues, domain.DataIssue{Code: "insufficient_daily_history", Dataset: "daily_bars", Severity: "critical", Message: "没有任何自选股具备至少 21 根有效日 K，无法计算 20 日趋势基线"})
	}
	for _, note := range snapshot.Sources {
		if note.QualityStatus == "unavailable" || note.QualityStatus == "partial_metadata" {
			partial = true
			severity := "warning"
			if note.QualityStatus == "unavailable" {
				severity = "error"
			}
			issues = append(issues, domain.DataIssue{Code: "upstream_data_quality", Dataset: note.Dataset, Symbol: note.Symbol, Severity: severity, Message: note.Diagnostic})
		}
		if len(note.MissingFields) > 0 {
			partial = true
		}
	}
	if req.TaskType == domain.TaskHistoricalRun {
		// The Python endpoint does not expose provider publication time or price-adjustment basis.
		// Historical replay must not claim point-in-time validity without those fields.
		corePass = false
		issues = append(issues, domain.DataIssue{Code: "historical_point_in_time_unverified", Severity: "critical", Message: "上游接口没有逐条 available_at、provider 和复权口径，历史回放按规则阻断"})
	}
	if len(snapshot.IndexBars) == 0 {
		partial = true
		issues = append(issues, domain.DataIssue{Code: "index_data_missing", Dataset: "index_bars", Severity: "warning", Message: "没有取得指数日 K，市场环境只使用自选股横截面，不能视为完整市场广度分析"})
	}
	for _, id := range req.StrategyIDs {
		eligible[id] = corePass && goodSymbols > 0
		if !eligible[id] {
			blockers[id] = []string{"至少需要一只股票具备 21 根有效日 K，且必须通过核心数据门禁"}
		}
	}
	return domain.QualityReport{CorePass: corePass, Partial: partial, Issues: issues, StrategyEligible: eligible, StrategyBlockers: blockers, CheckedAt: time.Now().UTC(), PolicyVersion: req.DataPolicyVersion}, nil
}

type marketAnalyzer struct{}

func (marketAnalyzer) Analyze(ctx context.Context, req domain.RunRequest, s domain.MarketSnapshot) (domain.MarketState, error) {
	if err := ctx.Err(); err != nil {
		return domain.MarketState{}, err
	}
	above, total := 0, 0
	for _, bars := range s.Bars {
		if len(bars) < 20 {
			continue
		}
		total++
		var sum float64
		for _, b := range bars[len(bars)-20:] {
			sum += b.Close
		}
		if bars[len(bars)-1].Close > sum/20 {
			above++
		}
	}
	metrics := map[string]float64{}
	evidence := []string{}
	regime := "insufficient_cross_section"
	if total > 0 {
		breadth := float64(above) / float64(total) * 100
		metrics["watchlist_above_ma20_pct"] = breadth
		metrics["watchlist_symbols_with_20_bars"] = float64(total)
		evidence = append(evidence, fmt.Sprintf("自选股中 %d/%d 只收盘价高于自身 20 日均线", above, total))
		if breadth >= 65 {
			regime = "watchlist_trend_positive"
		} else if breadth <= 35 {
			regime = "watchlist_trend_weak"
		} else {
			regime = "watchlist_mixed"
		}
	}
	indexCount := 0
	for symbol, bars := range s.IndexBars {
		if len(bars) < 21 {
			continue
		}
		base, last := bars[len(bars)-21].Close, bars[len(bars)-1].Close
		if base > 0 {
			metrics["index_20d_return_pct"] = (last/base - 1) * 100
			indexCount++
			evidence = append(evidence, fmt.Sprintf("指数 %s 的 20 个交易间隔收益为 %.2f%%", symbol, metrics["index_20d_return_pct"]))
		}
	}
	if indexCount == 0 {
		evidence = append(evidence, "缺少可计算的指数历史行情；本结果不是完整大盘趋势判断")
	}
	return domain.MarketState{AsOf: req.AsOf, Regime: regime, Metrics: metrics, Evidence: evidence, AlgorithmVersion: "mvp-watchlist-market-v1"}, nil
}

type trendFactor struct {
	id     string
	window int
	kind   string
}

func (f trendFactor) ID() string      { return f.id }
func (f trendFactor) Version() string { return "v1" }
func (f trendFactor) Requirements() []domain.DataRequirement {
	return []domain.DataRequirement{{Dataset: "daily_bars", Fields: []string{"close", "trade_date"}, Required: true, Consumer: f.id}}
}
func (f trendFactor) Calculate(ctx context.Context, input modules.FactorInput) ([]domain.FactorValue, error) {
	out := make([]domain.FactorValue, 0, len(input.Universe.Securities))
	for _, sec := range input.Universe.Securities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		bars := input.Snapshot.Bars[sec.Symbol]
		value := domain.FactorValue{Symbol: sec.Symbol, FactorID: f.id, AsOf: input.Request.AsOf, InputQuality: "partial_metadata"}
		if f.kind == "ma_distance" && len(bars) >= 20 {
			var sum float64
			for _, b := range bars[len(bars)-20:] {
				sum += b.Close
			}
			ma := sum / 20
			if ma > 0 {
				v := (bars[len(bars)-1].Close/ma - 1) * 100
				value.Value = &v
				value.Unit = "percent"
			}
		} else if f.kind == "return" && len(bars) > f.window {
			base, last := bars[len(bars)-1-f.window].Close, bars[len(bars)-1].Close
			if base > 0 {
				v := (last/base - 1) * 100
				value.Value = &v
				value.Unit = "percent"
			}
		}
		if value.Value == nil {
			value.MissingReason = fmt.Sprintf("至少需要 %d 根日 K；当前 %d 根", maxInt(20, f.window+1), len(bars))
		}
		if len(bars) > 0 {
			value.AsOf = bars[len(bars)-1].TradeDate
		}
		out = append(out, value)
	}
	return out, nil
}

type trendStrategy struct{}

func (trendStrategy) ID() string      { return StrategyTrendFollowing }
func (trendStrategy) Version() string { return "mvp-v1" }
func (trendStrategy) FactorIDs() []string {
	return []string{factorMADistance, factorReturn5, factorReturn20}
}
func (trendStrategy) Requirements() []domain.DataRequirement {
	return []domain.DataRequirement{{Dataset: "daily_bars", Fields: []string{"close", "trade_date"}, Required: true, Reason: "趋势基线至少需要 21 根日 K"}}
}
func (trendStrategy) Evaluate(ctx context.Context, in modules.StrategyInput) ([]domain.CandidateSignal, error) {
	values := make(map[string]map[string]float64)
	for _, f := range in.Factors {
		if f.Value == nil {
			continue
		}
		if values[f.Symbol] == nil {
			values[f.Symbol] = map[string]float64{}
		}
		values[f.Symbol][f.FactorID] = *f.Value
	}
	out := make([]domain.CandidateSignal, 0)
	for _, sec := range in.Universe.Securities {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		v, ok := values[sec.Symbol]
		if !ok {
			continue
		}
		ma, ok1 := v[factorMADistance]
		r5, ok2 := v[factorReturn5]
		r20, ok3 := v[factorReturn20]
		if !ok1 || !ok2 || !ok3 || ma <= 0 || r5 <= 0 || r20 <= 0 {
			continue
		}
		bars := in.Snapshot.Bars[sec.Symbol]
		if len(bars) == 0 {
			continue
		}
		price := bars[len(bars)-1].Close
		out = append(out, domain.CandidateSignal{ID: fmt.Sprintf("%s-%s-%s", in.Request.RunID, sec.Symbol, in.Request.AsOf.Format("20060102")), Symbol: sec.Symbol, StrategyID: StrategyTrendFollowing, StrategyVersion: "mvp-v1", Direction: "long_candidate", SignalAt: in.Request.AsOf, ValidUntil: in.Request.AsOf.Add(24 * time.Hour), EntryRule: "研究候选：仅在后续交易日重新验证行情、涨跌停、停牌和流动性后评估；本系统当前不自动下单", ExitRule: "研究规则：若收盘价跌破 20 日均线，视为趋势条件失效并重新评估", InvalidationRule: "20 日均线距离或 5 日/20 日收益任一项不再为正，候选失效", ExpectedHoldDays: 5, ReferencePrice: &price, EvidenceRefs: []string{factorMADistance, factorReturn5, factorReturn20}, Attributes: map[string]string{"ma20_distance_pct": strconv.FormatFloat(ma, 'f', 4, 64), "return_5d_pct": strconv.FormatFloat(r5, 'f', 4, 64), "return_20d_pct": strconv.FormatFloat(r20, 'f', 4, 64)}})
	}
	return out, nil
}

type hardRiskRule struct{}

func (hardRiskRule) ID() string      { return "unverified-tradability-block" }
func (hardRiskRule) Version() string { return "v1" }
func (hardRiskRule) Priority() int   { return 10 }
func (hardRiskRule) Evaluate(ctx context.Context, input modules.RiskInput) ([]domain.SignalDecision, error) {
	out := make([]domain.SignalDecision, 0, len(input.Signals))
	for _, s := range input.Signals {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		out = append(out, domain.SignalDecision{SignalID: s.ID, Allowed: false, Reasons: []string{"当前 MVP 未验证 ST/停牌/涨跌停、可成交量、组合仓位与账户权益，硬风险门禁阻断交易计划"}})
	}
	return out, nil
}

type tradabilityChecker struct{}

func (tradabilityChecker) Check(ctx context.Context, _ domain.RunRequest, snapshot domain.MarketSnapshot, signals []domain.CandidateSignal) ([]domain.SignalDecision, error) {
	out := make([]domain.SignalDecision, 0, len(signals))
	for _, s := range signals {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		_, quoteOK := snapshot.Quotes[s.Symbol]
		reason := "上游未提供可信的 ST、停牌、涨跌停和可成交量字段，无法确认可交易性"
		if !quoteOK {
			reason += "；未取得可用的实时行情"
		}
		out = append(out, domain.SignalDecision{SignalID: s.ID, Allowed: false, Reasons: []string{reason}})
	}
	return out, nil
}

type portfolioProvider struct{}

func (portfolioProvider) Load(ctx context.Context, req domain.RunRequest) (domain.PortfolioSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return domain.PortfolioSnapshot{}, err
	}
	return domain.PortfolioSnapshot{PortfolioID: req.PortfolioID, AsOf: req.AsOf, Currency: "CNY", AvailableCash: 0, TotalEquity: 0, Version: "unconfigured-no-paper-ledger-v1"}, nil
}

type positionPlanner struct{}

func (positionPlanner) Plan(ctx context.Context, req domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, signals []domain.CandidateSignal, _ []domain.SignalDecision, _ []domain.SignalDecision) ([]domain.TradePlan, []domain.PlanRejection, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	out := make([]domain.PlanRejection, 0, len(signals))
	for _, s := range signals {
		out = append(out, domain.PlanRejection{SignalID: s.ID, Reasons: []string{"MVP 未完成真实组合账本与 A 股交易制度校验，不生成仓位/执行计划"}})
	}
	return nil, out, nil
}

type executionAssessor struct{}

func (executionAssessor) Assess(ctx context.Context, _ domain.RunRequest, _ domain.MarketSnapshot, _ domain.PortfolioSnapshot, plans []domain.TradePlan) (domain.ExecutionAssessment, error) {
	if err := ctx.Err(); err != nil {
		return domain.ExecutionAssessment{}, err
	}
	orders := make([]domain.ExecutionOrderAssessment, 0, len(plans))
	for _, p := range plans {
		orders = append(orders, domain.ExecutionOrderAssessment{SignalID: p.SignalID, Status: domain.ExecutionPending, RequestedQuantity: p.Quantity, RemainingQuantity: p.Quantity, Reason: "仅记录待评估计划，当前不模拟成交"})
	}
	return domain.ExecutionAssessment{ModelVersion: "no-fill-mvp-v1", Orders: orders, Warnings: []string{"当前不执行真实下单，也不生成模拟成交"}}, nil
}

// NewRunnerWithStores constructs the same runner with durable storage injected.
func NewRunnerWithStores(pyCfg pythonapi.Config, store app.RunStore, snapshots app.SnapshotStore) (*app.Runner, error) {
	client, err := pythonapi.NewClient(pyCfg)
	if err != nil {
		return nil, err
	}
	collector, err := pythonapi.NewCollector(client, pyCfg)
	if err != nil {
		return nil, err
	}
	return newRunnerWithCollector(collector, store, snapshots)
}

// NewRunnerWithMarketDataStores builds the main API runner around the native Go
// market-data service. It deliberately has no Python fallback.
func NewRunnerWithMarketDataStores(cfg marketdata.Config, store app.RunStore, snapshots app.SnapshotStore) (*app.Runner, error) {
	client, err := marketdata.NewClient(cfg)
	if err != nil {
		return nil, err
	}
	collector, err := marketdata.NewCollector(client, cfg)
	if err != nil {
		return nil, err
	}
	return newRunnerWithCollector(collector, store, snapshots)
}

func newRunnerWithCollector(collector app.DataCollector, store app.RunStore, snapshots app.SnapshotStore) (*app.Runner, error) {
	factorRegistry := registry.New[modules.FactorCalculator]()
	for _, f := range []modules.FactorCalculator{trendFactor{id: factorMADistance, window: 20, kind: "ma_distance"}, trendFactor{id: factorReturn5, window: 5, kind: "return"}, trendFactor{id: factorReturn20, window: 20, kind: "return"}} {
		if err := factorRegistry.Register(f); err != nil {
			return nil, err
		}
	}
	factorRegistry.Freeze()
	strategyRegistry := registry.New[modules.Strategy]()
	if err := strategyRegistry.Register(trendStrategy{}); err != nil {
		return nil, err
	}
	strategyRegistry.Freeze()
	riskRegistry := registry.New[modules.RiskRule]()
	if err := riskRegistry.Register(hardRiskRule{}); err != nil {
		return nil, err
	}
	riskRegistry.Freeze()
	factorEngine, err := modules.NewFactorEngine(factorRegistry)
	if err != nil {
		return nil, err
	}
	strategyEngine, err := modules.NewStrategyEngine(strategyRegistry)
	if err != nil {
		return nil, err
	}
	riskEngine, err := modules.NewRiskEngine(riskRegistry)
	if err != nil {
		return nil, err
	}
	requirements, err := modules.NewRegistryRequirementCatalog(strategyRegistry, factorRegistry, []domain.DataRequirement{{Dataset: "daily_bars", Fields: []string{"trade_date", "open", "high", "low", "close"}, Required: true, Reason: "核心行情数据质量门禁"}, {Dataset: "realtime_quote", Fields: []string{"current_price", "update_time"}, Required: false, Reason: "时点辅助"}, {Dataset: "index_bars", Fields: []string{"trade_date", "close"}, Required: false, Reason: "市场环境辅助"}})
	if err != nil {
		return nil, err
	}
	if store == nil || snapshots == nil {
		return nil, errors.New("run store and snapshot store are required")
	}
	return app.NewRunner(app.Dependencies{Clock: clock{}, Universes: universeResolver{}, Requirements: requirements, Data: collector, Normalizer: normalizer{}, Quality: qualityAssessor{}, Market: marketAnalyzer{}, Factors: factorEngine, Strategies: strategyEngine, Tradability: tradabilityChecker{}, Portfolio: portfolioProvider{}, Risk: riskEngine, Planner: positionPlanner{}, Execution: executionAssessor{}, Store: store, Snapshots: snapshots})
}

func EnvDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration such as 3m", key)
	}
	return d, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
