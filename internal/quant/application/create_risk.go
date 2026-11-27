package application

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/markettime"
	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/risk"
)

// Risk 硬门禁切片的第一个生产 Facade（执行清单 1.4 / QNT-010）。
//
// 设计要点：
//   - 只消费已授权的交易状态事实与真实盘口事实；不重新解释事实，也不推导涨跌停。
//   - tradable_volume 只取自 Data 授权盘口快照中的真实买盘五档挂单量之和。
//   - 决策的 evaluated_at 取 Run 的可信 system_as_of 而不是墙钟：决策是"某个 PIT 时刻的
//     判断"，取墙钟会让同一份事实在重放时得到不同 result_id，破坏内容寻址的可复现性；
//     真实执行时刻由 Researchflow 的 Attempt 时间戳承载，信息不丢失。
//   - 决策结论落在 payload 里，Manifest.result_status 与之同步（reject → rejected）。
//     Stage 是否成功是另一层语义：评估完成即成功，结论是拒绝。
//   - price_limit 由独立 limit_pool 授权快照支撑：涨跌停时进入 BlockingFacts（不可交易），
//     池不完整只能记缺失；不得把 unknown 当成可交易，也不得推导涨跌幅比例。
const (
	riskPurposeCode      = TradingFactsPurposeCode
	riskSchemaVersion    = result.EnvelopeSchemaVersion
	riskPayloadVersion   = "1.0.0"
	riskInputResultType  = "risk"
	riskPeriod           = "1d"
	riskAlgorithmVersion = "1.0.0"
	riskMaxParamsBytes   = 4096

	// orderBookIntradayMaxAge 只约束盘中评估：盘口是日内事实，超过 15 分钟不能
	// 再当作当前可成交量。收盘后的评估改用交易日身份 + 收盘时刻约束。
	orderBookIntradayMaxAge = 15 * time.Minute
)

var (
	ErrInvalidCreateRiskRequest = errors.New("invalid Quant Risk request")
	ErrInvalidRiskSnapshot      = errors.New("invalid Quant Risk input snapshot")
)

// RiskDecisionIDGenerator 生成决策身份。
// 决策内容含评估时刻，identity 必须由外部生成，不能与 payload 指纹互相推导。
type RiskDecisionIDGenerator interface {
	NewDecisionID(context.Context) (string, error)
}

type CreateRiskRequest struct {
	SnapshotRef               string
	AuthorizationRef          string
	OrderBookSnapshotRef      string
	OrderBookAuthorizationRef string
	LimitPoolSnapshotRef      string
	LimitPoolAuthorizationRef string
	SystemAsOf                time.Time
	Parameters                json.RawMessage
}

// RiskExecutionResult 是一次可交易性判定已持久化的结果。
// MissingFacts / BlockingFacts 直接来自事实转换，供 Stage 层写 Quality Summary，
// 使"为什么拒绝"在 Stage Output 之外也有机器可读出口。
type RiskExecutionResult struct {
	Manifest           manifest.Manifest
	Decision           risk.Decision
	SnapshotRef        string
	MissingFacts       []risk.TradabilityFact
	BlockingFacts      []string
	RuleSetVersion     string
	InputQualityStatus string
}

type CreateRiskService struct {
	facts      TradingFactsReader
	orderBooks OrderBookFactsReader
	limitPools LimitPoolFactsReader
	decisions  RiskDecisionIDGenerator
	persister  *resultPersister
	now        func() time.Time
}

func NewCreateRiskService(
	facts TradingFactsReader,
	orderBooks OrderBookFactsReader,
	limitPools LimitPoolFactsReader,
	decisions RiskDecisionIDGenerator,
	repository result.Repository,
	now func() time.Time,
) (*CreateRiskService, error) {
	if facts == nil || orderBooks == nil || limitPools == nil || decisions == nil || repository == nil || now == nil {
		return nil, ErrInvalidCreateRiskRequest
	}
	persister, err := newResultPersister(repository)
	if err != nil {
		return nil, err
	}
	return &CreateRiskService{
		facts: facts, orderBooks: orderBooks, limitPools: limitPools,
		decisions: decisions, persister: persister, now: now,
	}, nil
}

// riskParameters 是风险 Stage 传给 Facade 的参数。
// 它们不参与门禁判定（门禁没有可调阈值），只用于把参数与事实快照绑定，
// 防止"参数说 A、快照是 B"的错配进入落库结果。
type riskParameters struct {
	Symbol    string `json:"symbol"`
	TradeDate string `json:"trade_date"`
}

// Execute 读取已授权事实快照，评估可交易性并落库。
// 决策为 reject 不是错误：返回已持久化的结果，由调用方读 decision.Status。
func (service *CreateRiskService) Execute(ctx context.Context, request CreateRiskRequest) (RiskExecutionResult, error) {
	if service == nil || service.facts == nil || service.orderBooks == nil || service.limitPools == nil ||
		service.decisions == nil ||
		service.persister == nil || service.now == nil || ctx == nil ||
		strings.TrimSpace(request.SnapshotRef) == "" || strings.TrimSpace(request.AuthorizationRef) == "" ||
		strings.TrimSpace(request.OrderBookSnapshotRef) == "" ||
		strings.TrimSpace(request.OrderBookAuthorizationRef) == "" ||
		strings.TrimSpace(request.LimitPoolSnapshotRef) == "" ||
		strings.TrimSpace(request.LimitPoolAuthorizationRef) == "" ||
		request.SystemAsOf.IsZero() || request.SystemAsOf.Location() != time.UTC ||
		len(request.Parameters) == 0 || len(request.Parameters) > riskMaxParamsBytes ||
		!json.Valid(request.Parameters) {
		return RiskExecutionResult{}, ErrInvalidCreateRiskRequest
	}
	if err := ctx.Err(); err != nil {
		return RiskExecutionResult{}, err
	}
	parameters, err := decodeRiskParameters(request.Parameters)
	if err != nil {
		return RiskExecutionResult{}, err
	}
	parameterFingerprint, err := manifest.FingerprintPayload(request.Parameters)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("%w: parameter fingerprint: %v", ErrInvalidCreateRiskRequest, err)
	}

	snapshot, err := service.facts.ReadTradingFactsSnapshot(
		ctx, request.SnapshotRef, request.AuthorizationRef, riskPurposeCode,
	)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("read authorized trading-status facts Snapshot: %w", err)
	}
	if err := validateRiskSnapshotBinding(snapshot, request, parameters, service.now()); err != nil {
		return RiskExecutionResult{}, err
	}
	orderBook, err := service.orderBooks.ReadOrderBookFactsSnapshot(
		ctx, request.OrderBookSnapshotRef, request.OrderBookAuthorizationRef, riskPurposeCode,
	)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("read authorized order-book facts Snapshot: %w", err)
	}
	if err := validateRiskOrderBookBinding(orderBook, request, parameters, snapshot); err != nil {
		return RiskExecutionResult{}, err
	}

	limitPool, err := service.limitPools.ReadLimitPoolFactsSnapshot(
		ctx, request.LimitPoolSnapshotRef, request.LimitPoolAuthorizationRef, riskPurposeCode,
	)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("read authorized limit-pool facts Snapshot: %w", err)
	}
	if err := validateRiskLimitPoolBinding(limitPool, request, parameters, snapshot); err != nil {
		return RiskExecutionResult{}, err
	}

	evidence := TradabilityEvidence{
		TradingFacts: snapshot, OrderBookFacts: orderBook, LimitPoolFacts: limitPool,
	}
	decisionID, err := service.decisions.NewDecisionID(ctx)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("generate risk decision identity: %w", err)
	}
	assessment, err := AssessTradabilityFacts(evidence)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("%w: %v", ErrInvalidRiskSnapshot, err)
	}
	decision, err := EvaluateTradabilityDecision(ctx, decisionID, request.SystemAsOf, request.SystemAsOf, evidence)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("evaluate tradability gate: %w", err)
	}

	qualityStatus := riskInputQualityStatus(snapshot.QualityStatus, orderBook.QualityStatus, limitPool.QualityStatus)
	input := manifest.Input{
		ResultType: riskInputResultType, Scope: "stock", SecurityID: snapshot.Symbol,
		TradeDate: snapshot.TradeDate, Period: riskPeriod,
		// quality_status 描述证据质量（两份快照中较差者），不是结论；结论在 decision.Status。
		QualityStatus:        qualityStatus,
		ResultStatus:         riskResultStatus(decision.Status),
		SchemaVersion:        riskSchemaVersion,
		PayloadSchemaVersion: riskPayloadVersion,
		AlgorithmVersion:     riskAlgorithmVersion, ParameterFingerprint: parameterFingerprint,
		SystemAsOf:        request.SystemAsOf,
		InputSnapshotRefs: []string{snapshot.SnapshotRef, orderBook.SnapshotRef, limitPool.SnapshotRef},
	}
	payload, err := result.NewRiskPayload(decision)
	if err != nil {
		return RiskExecutionResult{}, err
	}
	persisted, err := service.persister.persist(ctx, input, payload)
	if err != nil {
		return RiskExecutionResult{}, err
	}
	return RiskExecutionResult{
		Manifest:           persisted,
		Decision:           decision,
		SnapshotRef:        snapshot.SnapshotRef,
		MissingFacts:       append([]risk.TradabilityFact{}, assessment.Facts.MissingFacts...),
		BlockingFacts:      append([]string{}, assessment.BlockingFacts...),
		RuleSetVersion:     decision.RuleSetVersion,
		InputQualityStatus: qualityStatus,
	}, nil
}

// riskResultStatus 把决策结论映射为 QNT-CTR-005 的 result_status 封闭枚举。
// unknown（规则执行失败/证据非法）必须落在 failed，不能伪装成 completed 或 rejected。
func riskResultStatus(status risk.DecisionStatus) string {
	switch status {
	case risk.DecisionPass:
		return "completed"
	case risk.DecisionReject:
		return "rejected"
	default:
		return "failed"
	}
}

// validateRiskSnapshotBinding 把参数、请求与事实快照三者钉在一起。
// 任何一项不一致都拒绝：错配的快照会让"拒绝"看起来像是关于另一只证券的结论。
func validateRiskSnapshotBinding(
	snapshot TradingFactsSnapshot, request CreateRiskRequest, parameters riskParameters, trustedNow time.Time,
) error {
	if err := snapshot.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRiskSnapshot, err)
	}
	if snapshot.SnapshotRef != request.SnapshotRef || snapshot.PurposeCode != riskPurposeCode {
		return fmt.Errorf("%w: Snapshot identity or purpose does not match the request", ErrInvalidRiskSnapshot)
	}
	if snapshot.Symbol != parameters.Symbol || snapshot.TradeDate != parameters.TradeDate {
		return fmt.Errorf("%w: parameters do not describe the authorized Snapshot", ErrInvalidRiskSnapshot)
	}
	// as-of 语义：交易状态按 trade_date 精确约束（上方已核对），允许复用更早发布的快照；
	// 发布晚于评估时点的快照在 PIT 上还不存在，必须拒绝。
	snapshotAsOf, parseErr := time.Parse(time.RFC3339Nano, snapshot.SystemAsOf)
	if parseErr != nil || snapshotAsOf.After(request.SystemAsOf.UTC()) {
		return fmt.Errorf("%w: trading-status Snapshot was published after the evaluation instant", ErrInvalidRiskSnapshot)
	}
	if trustedNow.IsZero() || snapshot.TradeDate > request.SystemAsOf.UTC().Format("2006-01-02") {
		return fmt.Errorf("%w: trade_date is later than the evaluation instant", ErrInvalidRiskSnapshot)
	}
	return nil
}

// validateRiskOrderBookBinding 把盘口授权快照钉在请求、参数与交易状态快照上。
// 盘口是日内事实：发布时点不得晚于评估时点；盘中要求 15 分钟内观测，收盘后要求
// 观测日等于 trade_date 且不早于当日收盘，避免把隔日盘口当成当日当前事实。
func validateRiskOrderBookBinding(
	orderBook OrderBookFactsSnapshot,
	request CreateRiskRequest,
	parameters riskParameters,
	trading TradingFactsSnapshot,
) error {
	if err := orderBook.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRiskSnapshot, err)
	}
	if orderBook.SnapshotRef != request.OrderBookSnapshotRef || orderBook.PurposeCode != riskPurposeCode {
		return fmt.Errorf("%w: order-book Snapshot identity or purpose does not match the request", ErrInvalidRiskSnapshot)
	}
	if orderBook.Symbol != parameters.Symbol || orderBook.Symbol != trading.Symbol {
		return fmt.Errorf("%w: order-book Snapshot does not describe the authorized security", ErrInvalidRiskSnapshot)
	}
	orderBookAsOf, parseErr := time.Parse(time.RFC3339Nano, orderBook.SystemAsOf)
	if parseErr != nil || orderBookAsOf.After(request.SystemAsOf.UTC()) {
		return fmt.Errorf("%w: order-book Snapshot was published after the evaluation instant", ErrInvalidRiskSnapshot)
	}
	observedAt, parseErr := time.Parse(time.RFC3339Nano, orderBook.ObservedAt)
	if parseErr != nil {
		return fmt.Errorf("%w: order-book observed_at is not canonical", ErrInvalidRiskSnapshot)
	}
	location := markettime.Location()
	tradeDay, err := time.ParseInLocation("2006-01-02", parameters.TradeDate, location)
	if err != nil {
		return fmt.Errorf("%w: trade_date is not an ISO date", ErrInvalidRiskSnapshot)
	}
	if observedAt.In(location).Format("2006-01-02") != parameters.TradeDate {
		return fmt.Errorf("%w: order-book observation is from a different trade date", ErrInvalidRiskSnapshot)
	}
	closeAt := time.Date(tradeDay.Year(), tradeDay.Month(), tradeDay.Day(), 15, 0, 0, 0, location)
	if !request.SystemAsOf.Before(closeAt) {
		if observedAt.Before(closeAt) {
			return fmt.Errorf("%w: order-book observation predates the trade date close", ErrInvalidRiskSnapshot)
		}
		return nil
	}
	if request.SystemAsOf.Sub(observedAt) > orderBookIntradayMaxAge {
		return fmt.Errorf("%w: order-book observation is too old for an intraday evaluation", ErrInvalidRiskSnapshot)
	}
	return nil
}

func validateRiskLimitPoolBinding(
	limitPool LimitPoolFactsSnapshot,
	request CreateRiskRequest,
	parameters riskParameters,
	trading TradingFactsSnapshot,
) error {
	if err := limitPool.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidRiskSnapshot, err)
	}
	if limitPool.SnapshotRef != request.LimitPoolSnapshotRef || limitPool.PurposeCode != riskPurposeCode {
		return fmt.Errorf("%w: limit-pool Snapshot identity or purpose does not match the request", ErrInvalidRiskSnapshot)
	}
	if limitPool.Symbol != parameters.Symbol || limitPool.Symbol != trading.Symbol {
		return fmt.Errorf("%w: limit-pool Snapshot does not describe the authorized security", ErrInvalidRiskSnapshot)
	}
	if limitPool.TradeDate != parameters.TradeDate || limitPool.TradeDate != trading.TradeDate {
		return fmt.Errorf("%w: limit-pool trade_date differs from the request", ErrInvalidRiskSnapshot)
	}
	// as-of 语义：涨跌停池按交易日收盘后固化，允许复用更早发布的快照，但绝不能使用
	// 发布晚于评估时点的快照——那在 PIT 上还没有发生。交易日身份已由上面的
	// trade_date 精确约束，因此「更早」不会跨交易日取到过期事实。
	limitPoolAsOf, parseErr := time.Parse(time.RFC3339Nano, limitPool.SystemAsOf)
	if parseErr != nil || limitPoolAsOf.After(request.SystemAsOf.UTC()) {
		return fmt.Errorf("%w: limit-pool Snapshot was published after the evaluation instant", ErrInvalidRiskSnapshot)
	}
	return nil
}

// riskInputQualityStatus 取三份证据中较差的质量；degraded 不会被升级为 valid。
func riskInputQualityStatus(tradingQuality, orderBookQuality, limitPoolQuality string) string {
	if tradingQuality == "degraded" || orderBookQuality == "degraded" || limitPoolQuality == "degraded" {
		return "degraded"
	}
	return "valid"
}

func decodeRiskParameters(raw json.RawMessage) (riskParameters, error) {
	var parameters riskParameters
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil {
		return riskParameters{}, fmt.Errorf("%w: parameters: %v", ErrInvalidCreateRiskRequest, err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return riskParameters{}, fmt.Errorf("%w: parameters must contain exactly one object", ErrInvalidCreateRiskRequest)
	}
	if !nonBlankTradingFacts(parameters.Symbol) || !nonBlankTradingFacts(parameters.TradeDate) {
		return riskParameters{}, fmt.Errorf("%w: symbol and trade_date are required", ErrInvalidCreateRiskRequest)
	}
	if _, err := time.Parse("2006-01-02", parameters.TradeDate); err != nil {
		return riskParameters{}, fmt.Errorf("%w: trade_date is not an ISO date", ErrInvalidCreateRiskRequest)
	}
	return parameters, nil
}
