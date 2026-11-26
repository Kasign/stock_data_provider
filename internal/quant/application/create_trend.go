package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
	"example.com/stock-analysis-flow/internal/quant/factor"
	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

// Close-only 趋势状态的生产 Facade（执行清单 1.3，D1–D5 按推荐项通过）。
//
// 设计要点：
//   - 一次 Snapshot 读取产出四个精确证据 Metric，避免为每个周期各建一个 Snapshot；
//     四个 Metric 与同一 system_as_of 绑定，PIT 一致性由 Snapshot 门禁保证。
//   - momentum_60d 的精度/质量口径来自其 Registry Entry（D1 的前置要求），
//     本 Facade 断言 Entry 策略而不是在代码里重复声明 scale 与容差。
//   - 方向/阶段/强度判定全部委托 internal/quant/trend：本层不实现阈值或归一化，
//     也不把证据 Metric 的 ratio 值改写成 percent。
//   - 结果经 TrendPayload + resultPersister 落库并返回内容寻址 Manifest；
//     Run 关联仍由 Researchflow 在提交 Stage Output 时建立。
const (
	trendPurposeCode     = factorPurposeCode
	trendSchemaVersion   = result.EnvelopeSchemaVersion
	trendPayloadVersion  = "1.0.0"
	trendSnapshotSource  = "eastmoney"
	trendProviderAPI     = "eastmoney-stock-api-v1"
	trendMaxParamsBytes  = factorMaxParamsBytes
	trendInputResultType = "trend"
)

var (
	ErrInvalidCreateTrendRequest = errors.New("invalid Quant Trend request")
	ErrInvalidTrendSnapshot      = errors.New("invalid Quant Trend input snapshot")
	ErrTrendSnapshotQuality      = errors.New("Quant Trend input snapshot does not satisfy the registered quality policy")
)

type CreateTrendRequest struct {
	SnapshotRef          string
	AuthorizationRef     string
	SystemAsOf           time.Time
	Parameters           json.RawMessage
	AllowedProviderCodes []string
}

// TrendExecutionResult 是一次趋势判定的已持久化结果与输入质量摘要。
type TrendExecutionResult struct {
	Manifest             manifest.Manifest
	Result               trend.Result
	ParameterFingerprint string
	SnapshotRef          string
	Coverage             CloseSeriesCoverageEvidence
	ObservedBars         int
	// InputQualityStatus 是 Data Snapshot 的质量状态（由 Snapshot 门禁保证为 valid）；
	// 与 Result.QualityStatus（趋势判定质量）分开表达，避免下游把"输入可信"误读为"结论可用"。
	InputQualityStatus string
}

type CreateTrendService struct {
	snapshots CloseSeriesSnapshotReader
	registry  *registry.Registry
	persister *resultPersister
	now       func() time.Time
}

func NewCreateTrendService(
	snapshots CloseSeriesSnapshotReader,
	quantRegistry *registry.Registry,
	repository result.Repository,
	now func() time.Time,
) (*CreateTrendService, error) {
	if snapshots == nil || quantRegistry == nil || repository == nil || now == nil {
		return nil, ErrInvalidCreateTrendRequest
	}
	persister, err := newResultPersister(repository)
	if err != nil {
		return nil, err
	}
	return &CreateTrendService{snapshots: snapshots, registry: quantRegistry, persister: persister, now: now}, nil
}

// Execute 读取已授权 Snapshot，按 D1 的 5/20/60 周期集合精确计算四个证据 Metric，
// 再交由 trend.Assess 判定方向并落库。两类降级都不视为错误：
//   - 长周期证据不足但方向可判定 → quality_status=degraded，result_status=completed；
//   - 方向不可判定 → quality_status=unknown，result_status=insufficient_data。
//
// 两种情形都必须落库，否则 Stage Output 与授权读取拿不到可信 result_id。
func (service *CreateTrendService) Execute(ctx context.Context, request CreateTrendRequest) (TrendExecutionResult, error) {
	if service == nil || service.snapshots == nil || service.registry == nil || service.persister == nil || service.now == nil ||
		ctx == nil ||
		strings.TrimSpace(request.SnapshotRef) == "" || strings.TrimSpace(request.AuthorizationRef) == "" ||
		request.SystemAsOf.IsZero() || request.SystemAsOf.Location() != time.UTC ||
		len(request.Parameters) == 0 || len(request.Parameters) > trendMaxParamsBytes ||
		!json.Valid(request.Parameters) || !containsSortedUnique(request.AllowedProviderCodes, trendSnapshotSource) {
		return TrendExecutionResult{}, ErrInvalidCreateTrendRequest
	}
	if err := ctx.Err(); err != nil {
		return TrendExecutionResult{}, err
	}

	// momentum_60d 是最长窗口：它的 Entry 同时定义趋势输入的最小样本与覆盖率门禁。
	longEntry, err := service.registry.LookupActive(registry.ProductionMomentum60Code, factorSchemaVersion)
	if err != nil {
		return TrendExecutionResult{}, fmt.Errorf("lookup active momentum_60d registry entry: %w", err)
	}
	if err := validateDecimalFactorRegistryEntry(longEntry); err != nil {
		return TrendExecutionResult{}, err
	}
	if err := service.registry.ValidateParameters(longEntry.Code, longEntry.SchemaVersion, longEntry.AlgorithmVersion, request.Parameters); err != nil {
		return TrendExecutionResult{}, fmt.Errorf("%w: parameters: %v", ErrInvalidCreateTrendRequest, err)
	}
	if err := validateFactorParameters(request.Parameters); err != nil {
		return TrendExecutionResult{}, err
	}
	parameterFingerprint, err := manifest.FingerprintPayload(request.Parameters)
	if err != nil {
		return TrendExecutionResult{}, fmt.Errorf("%w: parameter fingerprint: %v", ErrInvalidCreateTrendRequest, err)
	}

	snapshot, err := service.snapshots.ReadAuthorizedCloseSeriesSnapshot(
		ctx, request.SnapshotRef, request.AuthorizationRef, trendPurposeCode,
	)
	if err != nil {
		return TrendExecutionResult{}, fmt.Errorf("read authorized Data Snapshot: %w", err)
	}
	trustedNow := service.now()
	policy, err := closeSeriesValidationPolicyFromEntry(longEntry, trendPurposeCode, trendSnapshotSource, trendProviderAPI)
	if err != nil {
		return TrendExecutionResult{}, ErrInvalidTrendSnapshot
	}
	systemAsOf, err := validateAuthorizedCloseSeriesSnapshot(
		snapshot, request.SnapshotRef, request.SystemAsOf, request.AllowedProviderCodes, policy, trustedNow,
	)
	if err != nil {
		return TrendExecutionResult{}, classifyCloseSeriesValidationError(err, ErrTrendSnapshotQuality, ErrInvalidTrendSnapshot)
	}

	closes := make([]string, len(snapshot.Bars))
	for index := range snapshot.Bars {
		closes[index] = snapshot.Bars[index].Close
	}
	tradeDate := snapshot.Bars[len(snapshot.Bars)-1].TradeDate
	calculationContext := domain.CalculationContext{
		SystemAsOf: systemAsOf, TradeDate: tradeDate,
		InputSnapshotRefs: []string{snapshot.SnapshotRef},
		AdjustmentBasis:   domain.AdjustmentBasisNone,
		QualityPolicy:     domain.QualityPolicyStrict,
	}
	if err := calculationContext.Validate(trustedNow, true); err != nil {
		return TrendExecutionResult{}, fmt.Errorf("%w: calculation context: %v", ErrInvalidTrendSnapshot, err)
	}

	evidence, err := trendEvidenceMetrics(closes, systemAsOf, registry.ProductionFactorAlgorithmVersion)
	if err != nil {
		return TrendExecutionResult{}, err
	}
	// 长周期证据必须与它的活动 Registry Entry 一致：Entry 是 momentum_60d 精度、
	// 单位、样本口径的唯一来源，趋势层不得自行放宽或改写。
	longEvidenceJSON, err := json.Marshal(evidence[3])
	if err != nil {
		return TrendExecutionResult{}, fmt.Errorf("encode momentum_60d evidence: %w", err)
	}
	if err := service.registry.ValidateMetric(longEntry.Code, longEntry.SchemaVersion, longEntry.AlgorithmVersion, longEvidenceJSON); err != nil {
		return TrendExecutionResult{}, fmt.Errorf("validate momentum_60d evidence against its registry entry: %w", err)
	}
	assessed, err := trend.Assess(trend.AssessmentInput{SystemAsOf: systemAsOf, Evidence: evidence})
	if err != nil {
		return TrendExecutionResult{}, fmt.Errorf("assess trend direction: %w", err)
	}

	resultStatus := "completed"
	if assessed.Direction == trend.DirectionUnknown {
		resultStatus = "insufficient_data"
	}
	// QNT-CTR-005 要求 Trend payload 的 quality_status 与 Manifest 完全一致；
	// 趋势结果的 quality 描述的是判定本身（valid/degraded/unknown），
	// 不是输入 Snapshot 的质量，因此这里取判定结果而非 snapshot.QualityStatus。
	input := manifest.Input{
		ResultType: trendInputResultType, Scope: "stock", SecurityID: snapshot.Symbol,
		TradeDate: tradeDate, Period: assessed.Period,
		QualityStatus: string(assessed.QualityStatus), ResultStatus: resultStatus,
		SchemaVersion: trendSchemaVersion, PayloadSchemaVersion: trendPayloadVersion,
		AlgorithmVersion: trend.CloseTrendAlgorithmVersion, ParameterFingerprint: parameterFingerprint,
		SystemAsOf: systemAsOf, InputSnapshotRefs: []string{snapshot.SnapshotRef},
	}
	payload, err := result.NewTrendPayload(assessed)
	if err != nil {
		return TrendExecutionResult{}, err
	}
	persisted, err := service.persister.persist(ctx, input, payload)
	if err != nil {
		return TrendExecutionResult{}, err
	}
	return TrendExecutionResult{
		Manifest:             persisted,
		Result:               assessed,
		ParameterFingerprint: parameterFingerprint,
		SnapshotRef:          snapshot.SnapshotRef,
		Coverage:             snapshot.Coverage,
		ObservedBars:         len(snapshot.Bars),
		InputQualityStatus:   snapshot.QualityStatus,
	}, nil
}

// trendEvidenceMetrics 按 D1 的固定顺序（MA20 距离、5d、20d、60d）计算四个精确证据。
// 顺序即契约：TrendResult.evidence_metrics 的排列不随调用方变化。
func trendEvidenceMetrics(closes []string, systemAsOf time.Time, calculationVersion string) ([]metric.Metric, error) {
	computations := []func([]string) (factor.DecimalResult, error){
		factor.DecimalMADistance20,
		factor.DecimalMomentum5,
		factor.DecimalMomentum20,
		factor.DecimalMomentum60,
	}
	evidence := make([]metric.Metric, 0, len(computations))
	for _, compute := range computations {
		calculated, err := compute(closes)
		if err != nil {
			return nil, fmt.Errorf("calculate trend evidence metric: %w", err)
		}
		decimalMetric, err := factor.NewDecimalQuantMetric(calculated, systemAsOf, factor.QualityValid, calculationVersion)
		if err != nil {
			return nil, fmt.Errorf("%w: build DecimalQuantMetric: %v", ErrInvalidCreateTrendRequest, err)
		}
		registryMetric, err := registryMetricFromDecimalMetric(decimalMetric, calculated.SampleCount)
		if err != nil {
			return nil, err
		}
		evidence = append(evidence, registryMetric)
	}
	return evidence, nil
}
