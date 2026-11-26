package application

import (
	"bytes"
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
)

// 首批 Close-only 比率因子的生产 Facade。
//
// 设计要点：
//   - 一个 Facade 通过封闭分派表覆盖全部已注册 Decimal 因子；新增因子只需在
//     decimalFactorComputation 增加一项并注册对应 Registry Entry，不需复制用例。
//   - 精度策略（scale/rounding/tolerance/value_type）从活动 Registry Entry 读取并断言，
//     不在 Facade 内硬编码；与计算层的 scale 一致性由测试断言守住。
//   - Snapshot 身份/PIT/质量/覆盖率门禁复用 close_series_snapshot_validation.go，
//     与 SMA20 共用同一实现，避免两处漂移。
//   - 结果经 result.FactorPayload + resultPersister 落库并返回内容寻址 Manifest（DEC-PLAT-002-058）；
//     持久化复用 SMA 用例同一写入端口，不复制 Repository 逻辑。
//   - 本层不签发 Data 授权、不建立 Run 关联、不装配 Server；Factor Stage Provider 属后续工作组。
const (
	factorSchemaVersion  = registry.ProductionFactorSchemaVersion
	factorPurposeCode    = "quant_analysis"
	factorMaxIntDigits   = 128
	factorToleranceJSON  = `"0.000000000001"`
	factorRoundingMode   = "half_even"
	factorMaxParamsBytes = 4096
)

var (
	ErrInvalidCreateFactorRequest = errors.New("invalid Quant Factor request")
	ErrInvalidFactorSnapshot      = errors.New("invalid Quant Factor input snapshot")
	ErrFactorSnapshotQuality      = errors.New("Quant Factor input snapshot does not satisfy the registered quality policy")
	ErrUnsupportedFactorEntry     = errors.New("active Factor registry entry is not supported by the decimal-factor facade")
	ErrUnsupportedFactorCode      = errors.New("unsupported Quant Factor code")
)

type decimalFactorComputation func([]string) (factor.DecimalResult, error)

// decimalFactorComputation 是封闭分派表：只接受已在 Production Registry 注册的 code。
// 分派函数与 Registry Entry 的一致性由 TestDecimalFactorComputationsMatchRegistry 守住。
func decimalFactorComputationFor(code string) (decimalFactorComputation, bool) {
	switch code {
	case registry.ProductionMADistanceCode:
		return factor.DecimalMADistance20, true
	case registry.ProductionMomentum5Code:
		return factor.DecimalMomentum5, true
	case registry.ProductionMomentum20Code:
		return factor.DecimalMomentum20, true
	default:
		return nil, false
	}
}

type CreateFactorRequest struct {
	FactorCode           string
	SnapshotRef          string
	AuthorizationRef     string
	SystemAsOf           time.Time
	Parameters           json.RawMessage
	AllowedProviderCodes []string
}

// FactorExecutionResult 是一次 Factor 生产计算的已持久化结果与输入质量摘要。
// Metric 为强类型 DecimalQuantMetric（value 是十进制字符串或 null），Manifest 为
// 内容寻址的不可变结果记录；Run 关联与 Server 装配由后续工作组消费本结果实现。
type FactorExecutionResult struct {
	Manifest             manifest.Manifest
	Metric               factor.DecimalQuantMetric
	ParameterFingerprint string
	SnapshotRef          string
	Coverage             CloseSeriesCoverageEvidence
	ObservedBars         int
}

// CreateFactorService 将 Data 已授权 Close Series Snapshot 转换为按活动 Registry
// Entry 校验过的 Decimal Factor 结果并持久化。不注入默认 Entry、不签发授权、不建 Run 关联。
type CreateFactorService struct {
	snapshots CloseSeriesSnapshotReader
	registry  *registry.Registry
	persister *resultPersister
	now       func() time.Time
}

func NewCreateFactorService(
	snapshots CloseSeriesSnapshotReader,
	quantRegistry *registry.Registry,
	repository result.Repository,
	now func() time.Time,
) (*CreateFactorService, error) {
	if snapshots == nil || quantRegistry == nil || repository == nil || now == nil {
		return nil, ErrInvalidCreateFactorRequest
	}
	persister, err := newResultPersister(repository)
	if err != nil {
		return nil, err
	}
	return &CreateFactorService{snapshots: snapshots, registry: quantRegistry, persister: persister, now: now}, nil
}

// Execute 读取已授权 Snapshot，按活动 Registry Entry 校验参数与精度，精确计算并返回强类型 Metric。
// 不足样本返回 metric_status=insufficient_data、value=null，不视为错误；该结果仍按
// result_status=insufficient_data 落库，以便 Stage Output 与授权读取持有可信 result_id。
func (service *CreateFactorService) Execute(ctx context.Context, request CreateFactorRequest) (FactorExecutionResult, error) {
	if service == nil || service.snapshots == nil || service.registry == nil || service.persister == nil || service.now == nil ||
		ctx == nil || strings.TrimSpace(request.FactorCode) == "" ||
		strings.TrimSpace(request.SnapshotRef) == "" || strings.TrimSpace(request.AuthorizationRef) == "" ||
		request.SystemAsOf.IsZero() || request.SystemAsOf.Location() != time.UTC ||
		len(request.Parameters) == 0 || len(request.Parameters) > factorMaxParamsBytes ||
		!json.Valid(request.Parameters) || !containsSortedUnique(request.AllowedProviderCodes, "eastmoney") {
		return FactorExecutionResult{}, ErrInvalidCreateFactorRequest
	}
	compute, ok := decimalFactorComputationFor(request.FactorCode)
	if !ok {
		return FactorExecutionResult{}, fmt.Errorf("%w: %s", ErrUnsupportedFactorCode, request.FactorCode)
	}
	if err := ctx.Err(); err != nil {
		return FactorExecutionResult{}, err
	}

	entry, err := service.registry.LookupActive(request.FactorCode, factorSchemaVersion)
	if err != nil {
		return FactorExecutionResult{}, fmt.Errorf("lookup active Factor registry entry: %w", err)
	}
	if err := validateDecimalFactorRegistryEntry(entry); err != nil {
		return FactorExecutionResult{}, err
	}
	if err := service.registry.ValidateParameters(entry.Code, entry.SchemaVersion, entry.AlgorithmVersion, request.Parameters); err != nil {
		return FactorExecutionResult{}, fmt.Errorf("%w: parameters: %v", ErrInvalidCreateFactorRequest, err)
	}
	if err := validateFactorParameters(request.Parameters); err != nil {
		return FactorExecutionResult{}, err
	}
	parameterFingerprint, err := manifest.FingerprintPayload(request.Parameters)
	if err != nil {
		return FactorExecutionResult{}, fmt.Errorf("%w: parameter fingerprint: %v", ErrInvalidCreateFactorRequest, err)
	}

	snapshot, err := service.snapshots.ReadAuthorizedCloseSeriesSnapshot(
		ctx, request.SnapshotRef, request.AuthorizationRef, factorPurposeCode,
	)
	if err != nil {
		return FactorExecutionResult{}, fmt.Errorf("read authorized Data Snapshot: %w", err)
	}
	trustedNow := service.now()
	policy, err := closeSeriesValidationPolicyFromEntry(entry, factorPurposeCode, "eastmoney", "eastmoney-stock-api-v1")
	if err != nil {
		return FactorExecutionResult{}, ErrInvalidFactorSnapshot
	}
	systemAsOf, err := validateAuthorizedCloseSeriesSnapshot(
		snapshot, request.SnapshotRef, request.SystemAsOf, request.AllowedProviderCodes, policy, trustedNow,
	)
	if err != nil {
		return FactorExecutionResult{}, classifyCloseSeriesValidationError(err, ErrFactorSnapshotQuality, ErrInvalidFactorSnapshot)
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
	// Decimal 因子当前没有 *WithContext 包装，必须在 Facade 显式校验 PIT/复权/质量边界。
	if err := calculationContext.Validate(trustedNow, true); err != nil {
		return FactorExecutionResult{}, fmt.Errorf("%w: calculation context: %v", ErrInvalidFactorSnapshot, err)
	}

	calculated, err := compute(closes)
	if err != nil {
		return FactorExecutionResult{}, fmt.Errorf("calculate exact Factor: %w", err)
	}
	if calculated.Code != entry.Code {
		return FactorExecutionResult{}, fmt.Errorf("%w: computation returned code %s, want %s", ErrUnsupportedFactorEntry, calculated.Code, entry.Code)
	}
	decimalMetric, err := factor.NewDecimalQuantMetric(calculated, systemAsOf, factor.QualityValid, entry.AlgorithmVersion)
	if err != nil {
		return FactorExecutionResult{}, fmt.Errorf("%w: build DecimalQuantMetric: %v", ErrUnsupportedFactorEntry, err)
	}
	registryMetric, err := registryMetricFromDecimalMetric(decimalMetric, calculated.SampleCount)
	if err != nil {
		return FactorExecutionResult{}, err
	}
	registryMetricJSON, err := json.Marshal(registryMetric)
	if err != nil {
		return FactorExecutionResult{}, fmt.Errorf("encode Factor QuantMetric: %w", err)
	}
	if err := service.registry.ValidateMetric(entry.Code, entry.SchemaVersion, entry.AlgorithmVersion, registryMetricJSON); err != nil {
		return FactorExecutionResult{}, fmt.Errorf("validate Factor QuantMetric against active Registry Entry: %w", err)
	}

	resultStatus := "completed"
	if calculated.Status == factor.StatusInsufficientData {
		resultStatus = "insufficient_data"
	}
	input := manifest.Input{
		ResultType: "factor", Scope: "stock", SecurityID: snapshot.Symbol,
		TradeDate: tradeDate, Period: decimalMetric.Period,
		QualityStatus: snapshot.QualityStatus, ResultStatus: resultStatus,
		SchemaVersion: result.EnvelopeSchemaVersion, PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion: entry.AlgorithmVersion, ParameterFingerprint: parameterFingerprint,
		SystemAsOf: systemAsOf, InputSnapshotRefs: []string{snapshot.SnapshotRef},
	}
	payload, err := result.NewFactorPayload(decimalMetric)
	if err != nil {
		return FactorExecutionResult{}, err
	}
	persisted, err := service.persister.persist(ctx, input, payload)
	if err != nil {
		return FactorExecutionResult{}, err
	}
	return FactorExecutionResult{
		Manifest:             persisted,
		Metric:               decimalMetric,
		ParameterFingerprint: parameterFingerprint,
		SnapshotRef:          snapshot.SnapshotRef,
		Coverage:             snapshot.Coverage,
		ObservedBars:         len(snapshot.Bars),
	}, nil
}

// validateDecimalFactorRegistryEntry 断言活动 Entry 就是本 Facade 支持的 Decimal 精度策略。
// 任一字段漂移都拒绝执行，避免"Registry 改精度但计算层不变"的静默不一致。
func validateDecimalFactorRegistryEntry(entry registry.Entry) error {
	if entry.SchemaVersion != factorSchemaVersion ||
		entry.ValueType != registry.ValueTypeDecimalString ||
		entry.Unit != registry.ProductionFactorUnit ||
		entry.Precision.MaxIntegerDigits != factorMaxIntDigits ||
		entry.Precision.MaxFractionDigits != factor.DecimalFactorScale ||
		entry.Precision.RoundingMode != factorRoundingMode ||
		!bytes.Equal(bytes.TrimSpace(entry.Precision.Tolerance), []byte(factorToleranceJSON)) ||
		!entry.Range.Unbounded ||
		!entry.SampleCount.RequiredInMetric || entry.SampleCount.Unit != "bar" ||
		strings.TrimSpace(entry.SampleCount.CountingRule) == "" ||
		entry.SupportsInterval ||
		entry.QualityPolicy.MissingHandling != "reject" ||
		!containsString(entry.QualityPolicy.UnavailableConditions, "insufficient_data") {
		return ErrUnsupportedFactorEntry
	}
	return nil
}

// validateFactorParameters 要求参数为严格空对象 {}：Registry 的 parameters_schema
// 声明 maxProperties=0，本函数补齐 Go 侧严格解码（拒绝尾随 JSON 与非对象）。
func validateFactorParameters(raw json.RawMessage) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) < 2 || trimmed[0] != '{' || trimmed[len(trimmed)-1] != '}' {
		return ErrInvalidCreateFactorRequest
	}
	var parameters map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil {
		return ErrInvalidCreateFactorRequest
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return ErrInvalidCreateFactorRequest
	}
	if parameters == nil || len(parameters) != 0 {
		return ErrInvalidCreateFactorRequest
	}
	return nil
}

// registryMetricFromDecimalMetric 把强类型 DecimalQuantMetric 投影为 ValidateMetric
// 接受的通用 metric.Metric；value 以 JSON string 承载十进制结果，不足样本为 JSON null。
func registryMetricFromDecimalMetric(source factor.DecimalQuantMetric, sampleCount int) (metric.Metric, error) {
	if sampleCount < 0 {
		return metric.Metric{}, ErrUnsupportedFactorEntry
	}
	value := json.RawMessage("null")
	if source.Value != nil {
		encoded, err := json.Marshal(*source.Value)
		if err != nil {
			return metric.Metric{}, fmt.Errorf("encode decimal Factor value: %w", err)
		}
		value = encoded
	}
	return metric.Metric{
		MetricCode:         source.MetricCode,
		Value:              value,
		Unit:               source.Unit,
		Period:             source.Period,
		SystemAsOf:         source.SystemAsOf,
		QualityStatus:      metric.QualityStatus(source.QualityStatus),
		MetricStatus:       metric.Status(source.MetricStatus),
		CalculationVersion: source.CalculationVersion,
		SampleCount:        &sampleCount,
		LowerBound:         json.RawMessage("null"),
		UpperBound:         json.RawMessage("null"),
	}, nil
}
