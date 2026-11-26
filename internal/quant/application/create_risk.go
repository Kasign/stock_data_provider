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

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/risk"
)

// Risk 硬门禁切片的第一个生产 Facade（执行清单 1.4 / QNT-010）。
//
// 设计要点：
//   - 只消费已授权的交易事实快照；不重新解释事实，也不推导涨跌停与可成交量。
//   - 决策的 evaluated_at 取 Run 的可信 system_as_of 而不是墙钟：决策是"某个 PIT 时刻的
//     判断"，取墙钟会让同一份事实在重放时得到不同 result_id，破坏内容寻址的可复现性；
//     真实执行时刻由 Researchflow 的 Attempt 时间戳承载，信息不丢失。
//   - 决策结论落在 payload 里，Manifest.result_status 与之同步（reject → rejected）。
//     Stage 是否成功是另一层语义：评估完成即成功，结论是拒绝。
//   - 当前事实边界下 Verified 恒为 false（缺涨跌停与可成交量），因此决策恒为 reject。
//     这是正确结论，不是缺陷；任何"补值"都会把推导伪装成事实。
const (
	riskPurposeCode      = TradingFactsPurposeCode
	riskSchemaVersion    = result.EnvelopeSchemaVersion
	riskPayloadVersion   = "1.0.0"
	riskInputResultType  = "risk"
	riskPeriod           = "1d"
	riskAlgorithmVersion = "1.0.0"
	riskMaxParamsBytes   = 4096
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
	SnapshotRef      string
	AuthorizationRef string
	SystemAsOf       time.Time
	Parameters       json.RawMessage
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
	facts     TradingFactsReader
	decisions RiskDecisionIDGenerator
	persister *resultPersister
	now       func() time.Time
}

func NewCreateRiskService(
	facts TradingFactsReader,
	decisions RiskDecisionIDGenerator,
	repository result.Repository,
	now func() time.Time,
) (*CreateRiskService, error) {
	if facts == nil || decisions == nil || repository == nil || now == nil {
		return nil, ErrInvalidCreateRiskRequest
	}
	persister, err := newResultPersister(repository)
	if err != nil {
		return nil, err
	}
	return &CreateRiskService{facts: facts, decisions: decisions, persister: persister, now: now}, nil
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
	if service == nil || service.facts == nil || service.decisions == nil || service.persister == nil ||
		service.now == nil || ctx == nil ||
		strings.TrimSpace(request.SnapshotRef) == "" || strings.TrimSpace(request.AuthorizationRef) == "" ||
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

	decisionID, err := service.decisions.NewDecisionID(ctx)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("generate risk decision identity: %w", err)
	}
	assessment, err := AssessTradabilityFacts(snapshot)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("%w: %v", ErrInvalidRiskSnapshot, err)
	}
	decision, err := EvaluateTradabilityDecision(ctx, decisionID, request.SystemAsOf, snapshot)
	if err != nil {
		return RiskExecutionResult{}, fmt.Errorf("evaluate tradability gate: %w", err)
	}

	input := manifest.Input{
		ResultType: riskInputResultType, Scope: "stock", SecurityID: snapshot.Symbol,
		TradeDate: snapshot.TradeDate, Period: riskPeriod,
		// quality_status 描述证据质量（事实快照质量），不是结论；结论在 decision.Status。
		QualityStatus:        snapshot.QualityStatus,
		ResultStatus:         riskResultStatus(decision.Status),
		SchemaVersion:        riskSchemaVersion,
		PayloadSchemaVersion: riskPayloadVersion,
		AlgorithmVersion:     riskAlgorithmVersion, ParameterFingerprint: parameterFingerprint,
		SystemAsOf: request.SystemAsOf, InputSnapshotRefs: []string{snapshot.SnapshotRef},
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
		InputQualityStatus: snapshot.QualityStatus,
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
	if snapshot.SystemAsOf != request.SystemAsOf.Format(time.RFC3339Nano) {
		return fmt.Errorf("%w: Snapshot system_as_of differs from the requested evaluation instant", ErrInvalidRiskSnapshot)
	}
	if trustedNow.IsZero() || snapshot.TradeDate > request.SystemAsOf.UTC().Format("2006-01-02") {
		return fmt.Errorf("%w: trade_date is later than the evaluation instant", ErrInvalidRiskSnapshot)
	}
	return nil
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
