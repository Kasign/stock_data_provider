package stage

import (
	"bytes"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/registry"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

// Trend Stage Output 的强类型 DTO 与运行时 Validator。
//
// 这里的 Schema 属于 QNT-CTR-003（deferred）的 L2 机器契约，只约束 close-only
// 趋势状态；不升级契约状态，也不代表 Trend 已进入完整研究 Template。
// result_id 必须由 QNT-CTR-005 Quant Result Manifest 提供，Stage 侧不得推导。
const (
	TrendStageCode            = "quant.trend"
	TrendOutputType           = "quant.trend_result"
	TrendSchemaVersion        = "1.0.0"
	TrendQualitySchemaVersion = "1.0.0"
)

var ErrInvalidTrendStageOutput = errors.New("invalid Quant Trend stage output")

var (
	trendStageResultIDPattern = regexp.MustCompile(`^quant_result:sha256:[0-9a-f]{64}$`)
	trendStageCalculationRE   = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)
	// trendStageEvidenceOrder 是 D1 周期集合的固定证据顺序，与 TrendResult 一致。
	trendStageEvidenceOrder = []struct {
		code   string
		period string
	}{
		{registry.ProductionMADistanceCode, "20d"},
		{registry.ProductionMomentum5Code, "5d"},
		{registry.ProductionMomentum20Code, "20d"},
		{registry.ProductionMomentum60Code, "60d"},
	}
)

// TrendOutput 是趋势状态的 Researchflow Stage payload。
// 内嵌 trend.Result 使 payload 与 QNT-CTR-004 TrendResult 保持同一字段语义，
// 但额外携带 Stage 层必需的结果身份与状态，不重复定义趋势字段。
type TrendOutput struct {
	SchemaVersion string `json:"schema_version"`
	ResultID      string `json:"result_id"`
	SnapshotRef   string `json:"snapshot_ref"`
	ResultStatus  string `json:"result_status"`
	trend.Result
}

// TrendQualitySummary 是趋势 Stage 的 quality_summary。
// minimum_required_bars 来自 Data CoverageEvidence；趋势窗口的样本充分性由
// 证据 Metric 的 sample_count/metric_status 表达，两者不混用。
//
// quality_status 恒为输入（Data Snapshot）质量；result_quality_status 是趋势判定
// 本身的质量，必须与 Payload 的 quality_status 一致。两者分开是因为"输入可信"
// 不等于"结论可用"（长周期证据缺失会得到 valid 输入 + degraded 判定）。
type TrendQualitySummary struct {
	SchemaVersion         string `json:"schema_version"`
	TrendCode             string `json:"trend_code"`
	QualityStatus         string `json:"quality_status"`
	ResultQualityStatus   string `json:"result_quality_status"`
	CalendarQualityStatus string `json:"calendar_quality_status"`
	ExpectedTradingDays   int    `json:"expected_trading_days"`
	ObservedBars          int    `json:"observed_bars"`
	MinimumRequiredBars   int    `json:"minimum_required_bars"`
	CoverageSufficient    bool   `json:"coverage_sufficient"`
}

func ValidateTrendStageOutputJSON(payload []byte) error {
	var output TrendOutput
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil || ensureJSONEOF(decoder) != nil {
		return ErrInvalidTrendStageOutput
	}
	if output.SchemaVersion != TrendSchemaVersion ||
		!trendStageResultIDPattern.MatchString(output.ResultID) ||
		!factorStageOpaqueRef(output.SnapshotRef) ||
		output.TrendCode != trend.CloseTrendCode ||
		output.AlgorithmVersion != trend.CloseTrendAlgorithmVersion ||
		!trendStageCalculationRE.MatchString(output.AlgorithmVersion) ||
		output.Phase != trend.PhaseUnknown || output.Strength != nil ||
		output.Period != trend.CloseTrendPeriod ||
		len(output.EvidenceMetrics) != len(trendStageEvidenceOrder) ||
		len(output.ConflictRefs) != 0 {
		return ErrInvalidTrendStageOutput
	}
	if err := output.Result.Validate(); err != nil {
		return ErrInvalidTrendStageOutput
	}
	switch output.ResultStatus {
	case "completed", "insufficient_data":
	default:
		return ErrInvalidTrendStageOutput
	}
	// 方向不可判定必须恰好对应 insufficient_data；不得把 unknown 记成 completed，
	// 也不得把可判定方向伪装成降级结果。
	if (output.Direction == trend.DirectionUnknown) != (output.ResultStatus == "insufficient_data") {
		return ErrInvalidTrendStageOutput
	}
	for index, spec := range trendStageEvidenceOrder {
		item := output.EvidenceMetrics[index]
		if item.MetricCode != spec.code || item.Period != spec.period || item.Unit != "ratio" ||
			item.SystemAsOf != output.SystemAsOf ||
			item.QualityStatus != metric.QualityValid {
			return ErrInvalidTrendStageOutput
		}
	}
	if err := validateTrendDirectionConsistency(output); err != nil {
		return err
	}
	return validateTrendQualityConsistency(output)
}

// validateTrendDirectionConsistency 用与生产同一份纯符号规则复算方向，
// 防止 payload 被改写后"证据指向一个方向、结论写成另一个方向"。
func validateTrendDirectionConsistency(output TrendOutput) error {
	var maDistance, momentum20 metric.Metric
	for _, item := range output.EvidenceMetrics {
		switch item.MetricCode {
		case registry.ProductionMADistanceCode:
			maDistance = item
		case registry.ProductionMomentum20Code:
			momentum20 = item
		}
	}
	if maDistance.MetricStatus != metric.StatusAvailable || momentum20.MetricStatus != metric.StatusAvailable {
		if output.Direction != trend.DirectionUnknown {
			return ErrInvalidTrendStageOutput
		}
		return nil
	}
	maValue, maErr := trendDecimalString(maDistance.Value)
	momentumValue, momentumErr := trendDecimalString(momentum20.Value)
	if maErr != nil || momentumErr != nil {
		return ErrInvalidTrendStageOutput
	}
	expected, err := trend.DirectionOf(maValue, momentumValue)
	if err != nil || expected != output.Direction {
		return ErrInvalidTrendStageOutput
	}
	return nil
}

// validateTrendQualityConsistency 固定 quality_status 的封闭映射：
// unknown 方向必须配 unknown 质量；方向可判定时只允许 valid 或 degraded。
func validateTrendQualityConsistency(output TrendOutput) error {
	switch output.QualityStatus {
	case trend.QualityValid:
		if output.Direction == trend.DirectionUnknown {
			return ErrInvalidTrendStageOutput
		}
		for _, item := range output.EvidenceMetrics {
			if item.MetricStatus != metric.StatusAvailable {
				return ErrInvalidTrendStageOutput
			}
		}
	case trend.QualityDegraded:
		if output.Direction == trend.DirectionUnknown {
			return ErrInvalidTrendStageOutput
		}
	case trend.QualityUnknown:
		if output.Direction != trend.DirectionUnknown {
			return ErrInvalidTrendStageOutput
		}
	default:
		return ErrInvalidTrendStageOutput
	}
	return nil
}

func ValidateTrendQualitySummaryJSON(payload []byte) error {
	var summary TrendQualitySummary
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil || ensureJSONEOF(decoder) != nil {
		return ErrInvalidTrendStageOutput
	}
	if summary.SchemaVersion != TrendQualitySchemaVersion ||
		summary.TrendCode != trend.CloseTrendCode ||
		summary.QualityStatus != "valid" || summary.CalendarQualityStatus != "valid" ||
		!validTrendResultQuality(summary.ResultQualityStatus) ||
		summary.ExpectedTradingDays <= 0 || summary.ObservedBars <= 0 ||
		summary.MinimumRequiredBars <= 0 || summary.MinimumRequiredBars > summary.ObservedBars ||
		summary.ObservedBars > summary.ExpectedTradingDays || !summary.CoverageSufficient {
		return ErrInvalidTrendStageOutput
	}
	return nil
}

func validTrendResultQuality(value string) bool {
	switch trend.QualityStatus(value) {
	case trend.QualityValid, trend.QualityDegraded, trend.QualityUnknown:
		return true
	default:
		return false
	}
}

func trendDecimalString(value json.RawMessage) (string, error) {
	var raw string
	if err := json.Unmarshal(value, &raw); err != nil || strings.TrimSpace(raw) != raw || raw == "" {
		return "", ErrInvalidTrendStageOutput
	}
	return raw, nil
}
