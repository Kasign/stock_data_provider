package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

const defaultPersistTimeout = 5 * time.Second

// Runner owns the stable daily-analysis orchestration. It does not implement
// data-source rules, factors, strategies, risk formulas, or execution models.
type Runner struct {
	deps           Dependencies
	persistTimeout time.Duration
}

func NewRunner(deps Dependencies) (*Runner, error) {
	if deps.Clock == nil || deps.Universes == nil || deps.Requirements == nil ||
		deps.Data == nil || deps.Normalizer == nil || deps.Quality == nil ||
		deps.Market == nil || deps.Factors == nil || deps.Strategies == nil ||
		deps.Tradability == nil || deps.Portfolio == nil || deps.Risk == nil || deps.Planner == nil ||
		deps.Execution == nil || deps.Snapshots == nil || deps.Store == nil {
		return nil, errors.New("analysis runner: all dependencies must be configured")
	}
	return &Runner{deps: deps, persistTimeout: defaultPersistTimeout}, nil
}

// Run returns business-blocked/rejected outcomes as a result with nil error.
// The error return is reserved for operational failures (database/API/module errors).
func (r *Runner) Run(ctx context.Context, request domain.RunRequest) (result domain.RunResult, retErr error) {
	if strings.TrimSpace(request.RunID) == "" {
		request.RunID = newRunID()
	}
	if err := validateRunID(request.RunID); err != nil {
		return domain.RunResult{
			RunID: request.RunID, Status: domain.RunStatusRejected, Request: request,
			Failure: &domain.Failure{Code: "invalid_run_id", Stage: "validate_request", Message: err.Error()},
		}, nil
	}
	startedAt := r.deps.Clock.Now()
	request.StrategyIDs = uniqueNonEmpty(request.StrategyIDs)
	request.Parameters = cloneStringMap(request.Parameters)
	result = domain.RunResult{
		RunID: request.RunID, Status: domain.RunStatusRunning,
		Request: request, StartedAt: startedAt,
	}

	if err := r.deps.Store.Begin(ctx, request, startedAt); err != nil {
		return result, fmt.Errorf("begin analysis run %s: %w", request.RunID, err)
	}

	defer func() {
		result.FinishedAt = r.deps.Clock.Now()
		if retErr != nil {
			result.Status = domain.RunStatusFailed
			if result.Failure == nil {
				result.Failure = &domain.Failure{Code: "stage_failed", Message: retErr.Error()}
			}
		} else if result.Status == domain.RunStatusRunning {
			result.Status = domain.RunStatusCompleted
		}

		// Do not let a cancelled request context prevent recording the final state.
		persistCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), r.persistTimeout)
		defer cancel()
		if err := r.deps.Store.Finish(persistCtx, result); err != nil {
			result.Status = domain.RunStatusFailed
			result.Failure = &domain.Failure{Code: "result_persistence_failed", Stage: "persist_result", Message: err.Error()}
			retErr = errors.Join(retErr, fmt.Errorf("persist analysis result %s: %w", request.RunID, err))
		}
	}()

	if err := validateRequest(request); err != nil {
		result.Status = domain.RunStatusRejected
		result.Failure = &domain.Failure{Code: "invalid_request", Stage: "validate_request", Message: err.Error()}
		event := domain.StageEvent{RunID: result.RunID, Stage: "validate_request", Status: domain.StageStatusFailed, At: r.deps.Clock.Now(), Message: err.Error()}
		result.Events = append(result.Events, event)
		if auditErr := r.deps.Store.AppendEvent(ctx, event); auditErr != nil {
			result.Status = domain.RunStatusFailed
			result.Failure = &domain.Failure{Code: "audit_write_failed", Stage: "validate_request", Message: auditErr.Error()}
			return result, fmt.Errorf("record request validation failure: %w", auditErr)
		}
		return result, nil
	}

	var requirements domain.RequirementSet
	if err := r.stage(ctx, &result, "resolve_universe", func() error {
		var err error
		result.Universe, err = r.deps.Universes.Resolve(ctx, request)
		if err != nil {
			return err
		}
		return validateUniverse(request, result.Universe)
	}); err != nil {
		return result, stageError("resolve_universe", err)
	}

	if err := r.stage(ctx, &result, "resolve_requirements", func() error {
		var err error
		requirements, err = r.deps.Requirements.Resolve(ctx, request)
		return err
	}); err != nil {
		return result, stageError("resolve_requirements", err)
	}

	var raw domain.RawSnapshot
	if err := r.stage(ctx, &result, "collect_data", func() error {
		var err error
		raw, err = r.deps.Data.Collect(ctx, request, result.Universe, requirements)
		if err != nil {
			return err
		}
		if raw.AsOf.IsZero() {
			return errors.New("raw snapshot has missing as_of timestamp")
		}

		if request.AsOfMode == domain.AsOfModeRealtimeSnapshot {
			if raw.AsOf.Before(request.AsOf) {
				return errors.New("realtime raw snapshot cutoff precedes request start time")
			}

			// 统一后续所有计算、信号及审计结果所使用的有效时点。
			request.AsOf = raw.AsOf
			result.Request = request
			result.Universe.AsOf = raw.AsOf
		} else if raw.AsOf.After(request.AsOf) {
			return errors.New("raw snapshot has future as_of timestamp")
		}
		return nil
	}); err != nil {
		return result, stageError("collect_data", err)
	}

	if err := r.stage(ctx, &result, "persist_raw_snapshot", func() error {
		var err error
		var snapshotID string
		snapshotID, err = r.deps.Snapshots.SaveRaw(ctx, request.RunID, raw)
		if err != nil {
			return err
		}
		if strings.TrimSpace(snapshotID) == "" {
			return errors.New("snapshot store returned empty raw snapshot id")
		}
		raw.ID = snapshotID
		result.RawSnapshotID = snapshotID
		return nil
	}); err != nil {
		return result, stageError("persist_raw_snapshot", err)
	}

	var snapshot domain.MarketSnapshot
	if err := r.stage(ctx, &result, "normalize_data", func() error {
		var err error
		snapshot, err = r.deps.Normalizer.Normalize(ctx, raw, request)
		if err != nil {
			return err
		}
		if snapshot.AsOf.IsZero() || snapshot.AsOf.After(request.AsOf) {
			return errors.New("normalized snapshot has missing or future as_of timestamp")
		}
		return nil
	}); err != nil {
		return result, stageError("normalize_data", err)
	}

	if err := r.stage(ctx, &result, "persist_market_snapshot", func() error {
		var err error
		var snapshotID string
		snapshotID, err = r.deps.Snapshots.SaveMarket(ctx, request.RunID, snapshot)
		if err != nil {
			return err
		}
		if strings.TrimSpace(snapshotID) == "" {
			return errors.New("snapshot store returned empty market snapshot id")
		}
		snapshot.ID = snapshotID
		result.MarketSnapshotID = snapshotID
		return nil
	}); err != nil {
		return result, stageError("persist_market_snapshot", err)
	}

	if err := r.stage(ctx, &result, "quality_gate", func() error {
		var err error
		result.Quality, err = r.deps.Quality.Assess(ctx, request, result.Universe, snapshot, requirements)
		if err != nil {
			return err
		}
		if result.Quality.PolicyVersion != request.DataPolicyVersion {
			return fmt.Errorf("quality policy version %q does not match frozen request version %q", result.Quality.PolicyVersion, request.DataPolicyVersion)
		}
		if result.Quality.CheckedAt.IsZero() {
			return errors.New("quality report checked_at timestamp is required")
		}
		return nil
	}); err != nil {
		return result, stageError("quality_gate", err)
	}
	if !result.Quality.CorePass {
		result.Status = domain.RunStatusBlocked
		result.Warnings = append(result.Warnings, "核心数据质量门禁未通过；未生成有效策略信号")
		return result, nil
	}
	eligibleStrategyIDs := make([]string, 0, len(request.StrategyIDs))
	partial := result.Quality.Partial
	if result.Quality.Partial {
		result.Warnings = append(result.Warnings, "质量报告标记为部分可用：存在辅助数据缺失或质量降级")
	}
	for _, strategyID := range request.StrategyIDs {
		eligible, found := result.Quality.StrategyEligible[strategyID]
		if !found || !eligible {
			partial = true
			reason := "质量报告未判定该策略，按 fail-closed 规则跳过"
			if found {
				reason = "策略必需数据不满足，按质量门禁跳过"
			}
			if blockers := result.Quality.StrategyBlockers[strategyID]; len(blockers) > 0 {
				reason += ": " + strings.Join(blockers, "; ")
			}
			result.Warnings = append(result.Warnings, fmt.Sprintf("策略 %s：%s", strategyID, reason))
			continue
		}
		eligibleStrategyIDs = append(eligibleStrategyIDs, strategyID)
	}

	if err := r.stage(ctx, &result, "market_analysis", func() error {
		var err error
		result.MarketState, err = r.deps.Market.Analyze(ctx, request, snapshot)
		if err != nil {
			return err
		}
		if result.MarketState.AsOf.IsZero() || result.MarketState.AsOf.After(request.AsOf) {
			return errors.New("market state has missing or future as_of timestamp")
		}
		if strings.TrimSpace(result.MarketState.AlgorithmVersion) == "" {
			return errors.New("market state algorithm_version is required")
		}
		if strings.TrimSpace(result.MarketState.Regime) == "" {
			return errors.New("market state regime is required")
		}
		for metric, value := range result.MarketState.Metrics {
			if strings.TrimSpace(metric) == "" || !finite(value) {
				return fmt.Errorf("market state contains empty metric name or non-finite value for %q", metric)
			}
		}
		return nil
	}); err != nil {
		return result, stageError("market_analysis", err)
	}

	if len(eligibleStrategyIDs) == 0 {
		result.Status = domain.RunStatusPartial
		for _, stageName := range []string{"resolve_active_requirements", "factor_calculation", "strategy_evaluation", "tradability_check", "risk_assessment", "portfolio_planning", "execution_assessment"} {
			if err := r.skipStage(ctx, &result, stageName, "没有通过策略数据门禁的策略，保留市场分析结果但不进入交易决策阶段"); err != nil {
				return result, stageError(stageName, err)
			}
		}
		return result, nil
	}

	strategyRequest := request
	strategyRequest.StrategyIDs = eligibleStrategyIDs
	var activeRequirements domain.RequirementSet
	if err := r.stage(ctx, &result, "resolve_active_requirements", func() error {
		var err error
		activeRequirements, err = r.deps.Requirements.Resolve(ctx, strategyRequest)
		return err
	}); err != nil {
		return result, stageError("resolve_active_requirements", err)
	}

	if err := r.stage(ctx, &result, "factor_calculation", func() error {
		var err error
		result.Factors, err = r.deps.Factors.Calculate(ctx, strategyRequest, result.Universe, snapshot, result.MarketState, result.Quality, activeRequirements)
		return err
	}); err != nil {
		return result, stageError("factor_calculation", err)
	}

	if err := r.stage(ctx, &result, "strategy_evaluation", func() error {
		var err error
		result.Signals, err = r.deps.Strategies.Evaluate(ctx, strategyRequest, result.Universe, snapshot, result.MarketState, result.Factors, result.Quality)
		if err != nil {
			return err
		}
		return validateCandidateSignals(request, strategyRequest.StrategyIDs, result.Universe, result.Signals)
	}); err != nil {
		return result, stageError("strategy_evaluation", err)
	}

	if err := r.stage(ctx, &result, "tradability_check", func() error {
		var err error
		result.Tradability, err = r.deps.Tradability.Check(ctx, request, snapshot, result.Signals)
		if err != nil {
			return err
		}
		return validateSignalDecisions(result.Signals, result.Tradability, "tradability")
	}); err != nil {
		return result, stageError("tradability_check", err)
	}

	var portfolio domain.PortfolioSnapshot
	if err := r.stage(ctx, &result, "load_portfolio", func() error {
		var err error
		portfolio, err = r.deps.Portfolio.Load(ctx, request)
		if err != nil {
			return err
		}
		return validatePortfolio(request, portfolio)
	}); err != nil {
		return result, stageError("load_portfolio", err)
	}

	if err := r.stage(ctx, &result, "persist_portfolio_snapshot", func() error {
		var err error
		var snapshotID string
		snapshotID, err = r.deps.Snapshots.SavePortfolio(ctx, request.RunID, portfolio)
		if err != nil {
			return err
		}
		if strings.TrimSpace(snapshotID) == "" {
			return errors.New("snapshot store returned empty portfolio snapshot id")
		}
		portfolio.ID = snapshotID
		result.PortfolioSnapshotID = snapshotID
		result.Portfolio = &portfolio
		return nil
	}); err != nil {
		return result, stageError("persist_portfolio_snapshot", err)
	}

	if err := r.stage(ctx, &result, "risk_assessment", func() error {
		var err error
		result.Risk, err = r.deps.Risk.Assess(ctx, request, snapshot, portfolio, result.Signals, result.Tradability)
		if err != nil {
			return err
		}
		return validateSignalDecisions(result.Signals, result.Risk, "risk")
	}); err != nil {
		return result, stageError("risk_assessment", err)
	}

	approvedSignals, gateRejections := approvedForPlanning(result.Signals, result.Tradability, result.Risk)
	result.PlanningRejections = append(result.PlanningRejections, gateRejections...)
	if err := r.stage(ctx, &result, "portfolio_planning", func() error {
		var err error
		var plannerRejections []domain.PlanRejection
		result.Plans, plannerRejections, err = r.deps.Planner.Plan(ctx, request, snapshot, portfolio, approvedSignals, result.Tradability, result.Risk)
		result.PlanningRejections = append(result.PlanningRejections, plannerRejections...)
		if err != nil {
			return err
		}
		return validateTradePlans(request, approvedSignals, result.Plans, plannerRejections)
	}); err != nil {
		return result, stageError("portfolio_planning", err)
	}

	if err := r.stage(ctx, &result, "execution_assessment", func() error {
		var err error
		result.Execution, err = r.deps.Execution.Assess(ctx, request, snapshot, portfolio, result.Plans)
		if err != nil {
			return err
		}
		return validateExecutionAssessment(request, result.Plans, result.Execution)
	}); err != nil {
		return result, stageError("execution_assessment", err)
	}

	if partial {
		result.Status = domain.RunStatusPartial
	} else {
		result.Status = domain.RunStatusCompleted
	}
	return result, nil
}

func (r *Runner) skipStage(ctx context.Context, result *domain.RunResult, name, message string) error {
	event := domain.StageEvent{RunID: result.RunID, Stage: name, Status: domain.StageStatusSkipped, At: r.deps.Clock.Now(), Message: message}
	result.Events = append(result.Events, event)
	return r.deps.Store.AppendEvent(ctx, event)
}

func (r *Runner) stage(ctx context.Context, result *domain.RunResult, name string, fn func() error) error {
	started := r.deps.Clock.Now()
	startEvent := domain.StageEvent{RunID: result.RunID, Stage: name, Status: domain.StageStatusStarted, At: started}
	result.Events = append(result.Events, startEvent)
	slog.Info("分析步骤开始",
		"service", "stock-analysis-api", "run_id", result.RunID, "stage", name,
		"status", domain.StageStatusStarted)
	if err := r.deps.Store.AppendEvent(ctx, startEvent); err != nil {
		result.Failure = &domain.Failure{Code: "audit_write_failed", Stage: name, Message: err.Error()}
		slog.Error("分析步骤状态写入失败",
			"service", "stock-analysis-api", "run_id", result.RunID, "stage", name,
			"status", "audit_write_failed", "error", boundedStageError(err.Error(), 300))
		return fmt.Errorf("write stage-start event: %w", err)
	}

	stageErr := fn()
	finished := r.deps.Clock.Now()
	duration := finished.Sub(started).Milliseconds()
	status := domain.StageStatusSucceeded
	message := ""
	if stageErr != nil {
		status = domain.StageStatusFailed
		message = stageErr.Error()
		result.Failure = &domain.Failure{Code: "stage_failed", Stage: name, Message: message}
		slog.Error("分析步骤失败",
			"service", "stock-analysis-api", "run_id", result.RunID, "stage", name,
			"status", status, "duration_ms", duration,
			"error_type", fmt.Sprintf("%T", stageErr), "error", boundedStageError(message, 300))
	} else {
		slog.Info("分析步骤完成",
			"service", "stock-analysis-api", "run_id", result.RunID, "stage", name,
			"status", status, "duration_ms", duration)
	}
	endEvent := domain.StageEvent{
		RunID: result.RunID, Stage: name, Status: status, At: finished,
		DurationMS: duration, Message: message,
	}
	result.Events = append(result.Events, endEvent)
	if err := r.deps.Store.AppendEvent(ctx, endEvent); err != nil {
		result.Failure = &domain.Failure{Code: "audit_write_failed", Stage: name, Message: err.Error()}
		slog.Error("分析步骤完成状态写入失败",
			"service", "stock-analysis-api", "run_id", result.RunID, "stage", name,
			"duration_ms", duration, "error", boundedStageError(err.Error(), 300))
		return errors.Join(stageErr, fmt.Errorf("write stage-finish event: %w", err))
	}
	return stageErr
}

func boundedStageError(value string, limit int) string {
	if len(value) > limit {
		return value[:limit]
	}
	return value
}

func validateRunID(runID string) error {
	if strings.TrimSpace(runID) == "" || len(runID) > 64 {
		return errors.New("run_id must contain 1..64 characters")
	}
	for _, r := range runID {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return errors.New("run_id may contain only ASCII letters, digits, hyphen and underscore")
		}
	}
	return nil
}

func validateRequest(request domain.RunRequest) error {
	symbols := append([]string(nil), request.Symbols...)
	if len(symbols) == 0 && request.Parameters != nil {
		for _, symbol := range strings.Split(request.Parameters["symbols"], ",") {
			if strings.TrimSpace(symbol) != "" {
				symbols = append(symbols, strings.TrimSpace(symbol))
			}
		}
	}
	if len(symbols) == 0 {
		return errors.New("symbols is required; provide 1..100 six-digit A-share codes")
	}
	if len(symbols) > 100 {
		return errors.New("at most 100 symbols are allowed per request")
	}
	for _, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if len(symbol) != 6 {
			return fmt.Errorf("invalid A-share symbol %q: expected six digits", symbol)
		}
		for _, r := range symbol {
			if r < '0' || r > '9' {
				return fmt.Errorf("invalid A-share symbol %q: expected six digits", symbol)
			}
		}
	}
	if request.AsOf.IsZero() {
		return errors.New("as_of is required")
	}
	if strings.TrimSpace(request.UniverseID) == "" {
		return errors.New("universe_id is required")
	}
	if strings.TrimSpace(request.MarketID) == "" {
		return errors.New("market_id is required")
	}
	if strings.TrimSpace(request.PortfolioID) == "" {
		return errors.New("portfolio_id is required")
	}
	if strings.TrimSpace(request.StrategyConfigVersion) == "" {
		return errors.New("strategy_config_version is required")
	}
	if strings.TrimSpace(request.RiskConfigVersion) == "" {
		return errors.New("risk_config_version is required")
	}
	if strings.TrimSpace(request.TradingRulesVersion) == "" {
		return errors.New("trading_rules_version is required")
	}
	if strings.TrimSpace(request.DataPolicyVersion) == "" {
		return errors.New("data_policy_version is required")
	}
	if len(request.StrategyIDs) == 0 {
		return errors.New("at least one strategy_id is required")
	}
	switch request.TaskType {
	case domain.TaskPostMarketScan, domain.TaskWatchlistScan, domain.TaskPositionReview, domain.TaskHistoricalRun:
	default:
		return fmt.Errorf("unsupported task_type %q", request.TaskType)
	}
	return nil
}

func stageError(stage string, err error) error {
	return fmt.Errorf("analysis stage %s: %w", stage, err)
}

func uniqueNonEmpty(values []string) []string {
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

func newRunID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}
	return "run-" + hex.EncodeToString(b[:])
}

// approvedForPlanning fails closed: a signal must have an explicit passing
// tradability decision and an explicit passing risk decision. Missing decisions
// or any denial prevent it from reaching the position planner.
func approvedForPlanning(signals []domain.CandidateSignal, tradability, risk []domain.SignalDecision) ([]domain.CandidateSignal, []domain.PlanRejection) {
	tradabilityByID := mergeDecisions(tradability)
	riskByID := mergeDecisions(risk)
	approved := make([]domain.CandidateSignal, 0, len(signals))
	rejections := make([]domain.PlanRejection, 0)
	for _, signal := range signals {
		var reasons []string
		tradabilityDecision, tradabilityFound := tradabilityByID[signal.ID]
		if !tradabilityFound {
			reasons = append(reasons, "缺少可交易性判定，按 fail-closed 规则拒绝")
		} else if !tradabilityDecision.Allowed {
			reasons = append(reasons, append([]string{"可交易性检查未放行"}, tradabilityDecision.Reasons...)...)
		}
		riskDecision, riskFound := riskByID[signal.ID]
		if !riskFound {
			reasons = append(reasons, "缺少风险判定，按 fail-closed 规则拒绝")
		} else if !riskDecision.Allowed {
			reasons = append(reasons, append([]string{"风险检查未放行"}, riskDecision.Reasons...)...)
		}
		if len(reasons) > 0 {
			rejections = append(rejections, domain.PlanRejection{SignalID: signal.ID, Reasons: uniqueNonEmpty(reasons)})
			continue
		}
		approved = append(approved, signal)
	}
	return approved, rejections
}

func mergeDecisions(decisions []domain.SignalDecision) map[string]domain.SignalDecision {
	out := make(map[string]domain.SignalDecision, len(decisions))
	for _, decision := range decisions {
		if strings.TrimSpace(decision.SignalID) == "" {
			continue
		}
		existing, found := out[decision.SignalID]
		if !found {
			decision.Reasons = uniqueNonEmpty(decision.Reasons)
			decision.RuleIDs = uniqueNonEmpty(decision.RuleIDs)
			decision.RuleRefs = uniqueRuleRefs(decision.RuleRefs)
			out[decision.SignalID] = decision
			continue
		}
		if !decision.Allowed {
			existing.Allowed = false
		}
		existing.Reasons = uniqueNonEmpty(append(existing.Reasons, decision.Reasons...))
		existing.RuleIDs = uniqueNonEmpty(append(existing.RuleIDs, decision.RuleIDs...))
		existing.RuleRefs = uniqueRuleRefs(append(existing.RuleRefs, decision.RuleRefs...))
		out[decision.SignalID] = existing
	}
	return out
}

func uniqueRuleRefs(values []domain.RuleRef) []domain.RuleRef {
	seen := make(map[string]struct{}, len(values))
	out := make([]domain.RuleRef, 0, len(values))
	for _, value := range values {
		value.ID = strings.TrimSpace(value.ID)
		value.Version = strings.TrimSpace(value.Version)
		if value.ID == "" || value.Version == "" {
			continue
		}
		key := value.ID + "\x00" + value.Version
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func validateUniverse(request domain.RunRequest, universe domain.UniverseSnapshot) error {
	if strings.TrimSpace(universe.ID) == "" || universe.ID != request.UniverseID {
		return fmt.Errorf("resolved universe id %q does not match requested universe %q", universe.ID, request.UniverseID)
	}
	if universe.AsOf.IsZero() || universe.AsOf.After(request.AsOf) {
		return errors.New("resolved universe has missing or future as_of timestamp")
	}
	if strings.TrimSpace(universe.Version) == "" {
		return errors.New("resolved universe version is required for point-in-time audit")
	}
	if len(universe.Securities) == 0 {
		return errors.New("resolved universe contains no securities")
	}
	seen := make(map[string]struct{}, len(universe.Securities))
	for _, security := range universe.Securities {
		if strings.TrimSpace(security.Symbol) == "" {
			return errors.New("resolved universe contains empty security symbol")
		}
		if _, exists := seen[security.Symbol]; exists {
			return fmt.Errorf("resolved universe contains duplicate symbol %q", security.Symbol)
		}
		seen[security.Symbol] = struct{}{}
	}
	return nil
}

func validateCandidateSignals(request domain.RunRequest, strategyIDs []string, universe domain.UniverseSnapshot, signals []domain.CandidateSignal) error {
	allowedStrategies := make(map[string]struct{}, len(strategyIDs))
	for _, strategyID := range strategyIDs {
		allowedStrategies[strategyID] = struct{}{}
	}
	allowedSymbols := make(map[string]struct{}, len(universe.Securities))
	for _, security := range universe.Securities {
		allowedSymbols[security.Symbol] = struct{}{}
	}
	seen := make(map[string]struct{}, len(signals))
	for _, signal := range signals {
		if strings.TrimSpace(signal.ID) == "" || strings.TrimSpace(signal.Symbol) == "" {
			return errors.New("strategy output contains signal without id or symbol")
		}
		if _, duplicate := seen[signal.ID]; duplicate {
			return fmt.Errorf("strategy output contains duplicate signal id %q", signal.ID)
		}
		seen[signal.ID] = struct{}{}
		if _, exists := allowedSymbols[signal.Symbol]; !exists {
			return fmt.Errorf("signal %q references symbol %q outside the resolved universe", signal.ID, signal.Symbol)
		}
		if _, exists := allowedStrategies[signal.StrategyID]; !exists {
			return fmt.Errorf("signal %q references unselected strategy %q", signal.ID, signal.StrategyID)
		}
		if strings.TrimSpace(signal.StrategyVersion) == "" {
			return fmt.Errorf("signal %q is missing strategy version", signal.ID)
		}
		if signal.SignalAt.IsZero() || signal.SignalAt.After(request.AsOf) {
			return fmt.Errorf("signal %q has missing or future signal_at", signal.ID)
		}
		if signal.ValidUntil.IsZero() || !signal.ValidUntil.After(request.AsOf) {
			return fmt.Errorf("signal %q must have a valid_until strictly after the analysis as_of", signal.ID)
		}
		if signal.ValidUntil.Before(signal.SignalAt) {
			return fmt.Errorf("signal %q has valid_until before signal_at", signal.ID)
		}
		if strings.TrimSpace(signal.Direction) == "" || strings.TrimSpace(signal.EntryRule) == "" || strings.TrimSpace(signal.ExitRule) == "" || strings.TrimSpace(signal.InvalidationRule) == "" {
			return fmt.Errorf("signal %q is missing direction, entry, exit, or invalidation rule", signal.ID)
		}
		if signal.ExpectedHoldDays < 0 {
			return fmt.Errorf("signal %q has negative expected_hold_days", signal.ID)
		}
		if signal.ReferencePrice != nil && (!finite(*signal.ReferencePrice) || *signal.ReferencePrice <= 0) {
			return fmt.Errorf("signal %q has invalid reference_price", signal.ID)
		}
	}
	return nil
}

func validateTradePlans(request domain.RunRequest, approvedSignals []domain.CandidateSignal, plans []domain.TradePlan, rejections []domain.PlanRejection) error {
	approved := make(map[string]domain.CandidateSignal, len(approvedSignals))
	for _, signal := range approvedSignals {
		approved[signal.ID] = signal
	}
	resolved := make(map[string]string, len(approvedSignals))
	for _, plan := range plans {
		signal, ok := approved[plan.SignalID]
		if !ok {
			return fmt.Errorf("planner returned plan for unapproved or unknown signal %q", plan.SignalID)
		}
		if _, exists := resolved[plan.SignalID]; exists {
			return fmt.Errorf("planner returned duplicate outcome for signal %q", plan.SignalID)
		}
		resolved[plan.SignalID] = "plan"
		if plan.Symbol != signal.Symbol {
			return fmt.Errorf("plan symbol %q does not match signal symbol %q", plan.Symbol, signal.Symbol)
		}
		if plan.Quantity <= 0 {
			return fmt.Errorf("plan for signal %q has non-positive quantity", plan.SignalID)
		}
		if !finite(plan.ReferencePrice) || plan.ReferencePrice <= 0 {
			return fmt.Errorf("plan for signal %q has non-finite or non-positive reference price", plan.SignalID)
		}
		if !finite(plan.RiskBudget) || plan.RiskBudget < 0 || !finite(plan.CostEstimate) || plan.CostEstimate < 0 {
			return fmt.Errorf("plan for signal %q has invalid risk budget or cost estimate", plan.SignalID)
		}
		if plan.StopPrice != nil && (!finite(*plan.StopPrice) || *plan.StopPrice <= 0) {
			return fmt.Errorf("plan for signal %q has invalid stop price", plan.SignalID)
		}
		if plan.TargetPrice != nil && (!finite(*plan.TargetPrice) || *plan.TargetPrice <= 0) {
			return fmt.Errorf("plan for signal %q has invalid target price", plan.SignalID)
		}
		side := strings.ToLower(strings.TrimSpace(plan.Side))
		if side != "buy" && side != "sell" {
			return fmt.Errorf("plan for signal %q has unsupported side %q; expected buy or sell", plan.SignalID, plan.Side)
		}
		if plan.EarliestExecuteAt.IsZero() || !plan.EarliestExecuteAt.After(signal.SignalAt) {
			return fmt.Errorf("plan for signal %q must have an execution time strictly after signal_at", plan.SignalID)
		}
		if !plan.EarliestExecuteAt.After(request.AsOf) {
			return fmt.Errorf("plan for signal %q must execute after analysis as_of; this run cannot use future market data", plan.SignalID)
		}
		if plan.EarliestExecuteAt.After(signal.ValidUntil) {
			return fmt.Errorf("plan for signal %q executes after signal validity expires", plan.SignalID)
		}
		if plan.ExpireAt.IsZero() {
			return fmt.Errorf("plan for signal %q must have an explicit expiry", plan.SignalID)
		}
		if !plan.ExpireAt.After(plan.EarliestExecuteAt) {
			return fmt.Errorf("plan for signal %q expires before or at earliest execution", plan.SignalID)
		}
		if plan.ExpireAt.After(signal.ValidUntil) {
			return fmt.Errorf("plan for signal %q expires after its signal validity", plan.SignalID)
		}
		if plan.RuleVersion == "" || plan.RuleVersion != request.TradingRulesVersion {
			return fmt.Errorf("plan for signal %q uses trading rule version %q, expected %q", plan.SignalID, plan.RuleVersion, request.TradingRulesVersion)
		}
	}
	for _, rejection := range rejections {
		if _, ok := approved[rejection.SignalID]; !ok {
			return fmt.Errorf("planner returned rejection for unapproved or unknown signal %q", rejection.SignalID)
		}
		if _, exists := resolved[rejection.SignalID]; exists {
			return fmt.Errorf("planner returned multiple outcomes for signal %q", rejection.SignalID)
		}
		if len(uniqueNonEmpty(rejection.Reasons)) == 0 {
			return fmt.Errorf("planner rejected signal %q without a reason", rejection.SignalID)
		}
		resolved[rejection.SignalID] = "rejected"
	}
	for signalID := range approved {
		if _, ok := resolved[signalID]; !ok {
			return fmt.Errorf("planner returned neither a trade plan nor a rejection for approved signal %q", signalID)
		}
	}
	return nil
}

func validateSignalDecisions(signals []domain.CandidateSignal, decisions []domain.SignalDecision, stage string) error {
	expected := make(map[string]struct{}, len(signals))
	for _, signal := range signals {
		if strings.TrimSpace(signal.ID) == "" {
			return fmt.Errorf("%s validation received a signal with empty id", stage)
		}
		if _, duplicate := expected[signal.ID]; duplicate {
			return fmt.Errorf("%s validation received duplicate signal id %q", stage, signal.ID)
		}
		expected[signal.ID] = struct{}{}
	}
	seen := make(map[string]struct{}, len(decisions))
	for _, decision := range decisions {
		if strings.TrimSpace(decision.SignalID) == "" {
			return fmt.Errorf("%s decision has empty signal_id", stage)
		}
		if _, ok := expected[decision.SignalID]; !ok {
			return fmt.Errorf("%s decision references unknown signal %q", stage, decision.SignalID)
		}
		if _, duplicate := seen[decision.SignalID]; duplicate {
			return fmt.Errorf("%s returned duplicate decision for signal %q", stage, decision.SignalID)
		}
		seen[decision.SignalID] = struct{}{}
		if !decision.Allowed && len(uniqueNonEmpty(decision.Reasons)) == 0 {
			return fmt.Errorf("%s denied signal %q without an auditable reason", stage, decision.SignalID)
		}
	}
	if len(seen) != len(expected) {
		return fmt.Errorf("%s returned %d decisions for %d signals; every signal must be explicitly checked", stage, len(seen), len(expected))
	}
	return nil
}

func validateExecutionAssessment(request domain.RunRequest, plans []domain.TradePlan, simulation domain.ExecutionAssessment) error {
	if strings.TrimSpace(simulation.ModelVersion) == "" {
		return errors.New("execution assessment model_version is required")
	}
	plansBySignal := make(map[string]domain.TradePlan, len(plans))
	for _, plan := range plans {
		plansBySignal[plan.SignalID] = plan
	}
	seen := make(map[string]struct{}, len(simulation.Orders))
	for _, order := range simulation.Orders {
		plan, exists := plansBySignal[order.SignalID]
		if !exists {
			return fmt.Errorf("execution assessor returned outcome for unknown signal %q", order.SignalID)
		}
		if _, duplicate := seen[order.SignalID]; duplicate {
			return fmt.Errorf("execution assessor returned duplicate outcome for signal %q", order.SignalID)
		}
		seen[order.SignalID] = struct{}{}
		if order.RequestedQuantity != plan.Quantity || order.FilledQuantity < 0 || order.RemainingQuantity < 0 {
			return fmt.Errorf("execution outcome for signal %q has invalid requested/filled/remaining quantities", order.SignalID)
		}
		if order.FilledQuantity+order.RemainingQuantity != order.RequestedQuantity {
			return fmt.Errorf("execution outcome for signal %q does not reconcile requested quantity", order.SignalID)
		}
		switch order.Status {
		case domain.ExecutionPending:
			if order.FilledQuantity != 0 || order.RemainingQuantity != plan.Quantity {
				return fmt.Errorf("pending execution outcome for signal %q must have zero fills and full remaining quantity", order.SignalID)
			}
		case domain.ExecutionFilled:
			if order.FilledQuantity != plan.Quantity || order.RemainingQuantity != 0 {
				return fmt.Errorf("filled execution outcome for signal %q does not reconcile", order.SignalID)
			}
		case domain.ExecutionPartiallyFilled:
			if order.FilledQuantity <= 0 || order.FilledQuantity >= plan.Quantity || order.RemainingQuantity <= 0 {
				return fmt.Errorf("partially filled execution outcome for signal %q is inconsistent", order.SignalID)
			}
		case domain.ExecutionUnfilled, domain.ExecutionBlocked, domain.ExecutionExpired, domain.ExecutionRejected:
			if order.FilledQuantity != 0 || order.RemainingQuantity != plan.Quantity {
				return fmt.Errorf("non-filled execution outcome for signal %q has inconsistent quantities", order.SignalID)
			}
			if strings.TrimSpace(order.Reason) == "" {
				return fmt.Errorf("execution outcome %q for signal %q requires a reason", order.Status, order.SignalID)
			}
		default:
			return fmt.Errorf("execution outcome for signal %q has unsupported status %q", order.SignalID, order.Status)
		}
		// This runner has access only to data available at request.AsOf. A newly
		// generated plan is required to execute after that cutoff; its initial
		// outcome cannot claim a fill based on unseen future data.
		if plan.EarliestExecuteAt.After(request.AsOf) && (order.Status == domain.ExecutionFilled || order.Status == domain.ExecutionPartiallyFilled) {
			return fmt.Errorf("execution outcome for future plan %q cannot claim fills beyond available data", order.SignalID)
		}
	}
	if len(seen) != len(plansBySignal) {
		return fmt.Errorf("execution assessor returned %d outcomes for %d plans; every plan must have an explicit outcome", len(seen), len(plansBySignal))
	}
	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func validatePortfolio(request domain.RunRequest, portfolio domain.PortfolioSnapshot) error {
	if strings.TrimSpace(portfolio.PortfolioID) == "" || portfolio.PortfolioID != request.PortfolioID {
		return fmt.Errorf("loaded portfolio id %q does not match requested portfolio %q", portfolio.PortfolioID, request.PortfolioID)
	}
	if portfolio.AsOf.IsZero() || portfolio.AsOf.After(request.AsOf) {
		return errors.New("portfolio snapshot has missing or future as_of timestamp")
	}
	if strings.TrimSpace(portfolio.Version) == "" {
		return errors.New("portfolio snapshot version is required")
	}
	if strings.TrimSpace(portfolio.Currency) == "" {
		return errors.New("portfolio currency is required")
	}
	if !finite(portfolio.AvailableCash) || !finite(portfolio.TotalEquity) || portfolio.AvailableCash < 0 || portfolio.TotalEquity < 0 {
		return errors.New("portfolio cash/equity must be finite and non-negative in this cash-account model")
	}
	positions := make(map[string]struct{}, len(portfolio.Positions))
	for _, position := range portfolio.Positions {
		if strings.TrimSpace(position.Symbol) == "" {
			return errors.New("portfolio contains position with empty symbol")
		}
		if _, exists := positions[position.Symbol]; exists {
			return fmt.Errorf("portfolio contains duplicate position %q", position.Symbol)
		}
		positions[position.Symbol] = struct{}{}
		if position.Quantity < 0 || position.SellableQuantity < 0 || position.SellableQuantity > position.Quantity {
			return fmt.Errorf("portfolio position %s has invalid or inconsistent quantities", position.Symbol)
		}
		if !finite(position.AverageCost) || !finite(position.MarketValue) || position.AverageCost < 0 || position.MarketValue < 0 {
			return fmt.Errorf("portfolio position %s has invalid cost or market value", position.Symbol)
		}
	}
	orders := make(map[string]struct{}, len(portfolio.OpenOrders))
	for _, order := range portfolio.OpenOrders {
		if strings.TrimSpace(order.OrderID) == "" || strings.TrimSpace(order.Symbol) == "" {
			return errors.New("portfolio contains open order without order_id or symbol")
		}
		if _, exists := orders[order.OrderID]; exists {
			return fmt.Errorf("portfolio contains duplicate open order %q", order.OrderID)
		}
		orders[order.OrderID] = struct{}{}
		if order.RemainingQuantity < 0 || !finite(order.LimitPrice) || order.LimitPrice < 0 {
			return fmt.Errorf("portfolio open order %s has invalid quantity or limit price", order.OrderID)
		}
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
