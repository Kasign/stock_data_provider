package factor

import (
	"strings"
	"time"
)

// DecimalQuantMetric 是首批 Close-only 比率因子的强类型 QNT-CTR-004 公开指标对象。
// 与 QuantMetric（float64）并存：Value/上下界以十进制字符串承载，序列化为 JSON string；
// 样本不足时 Value 为 nil，序列化为显式 JSON null（GOV-CTR-001 空值语义）。
// 字符串值由 decimal_factor.go 的 big.Rat 精确中间运算、固定 scale=12 与 half-even 舍入生成，
// 不经 float64 转换，也不接受外部传入的格式化 float64。
type DecimalQuantMetric struct {
	MetricCode         string        `json:"metric_code"`
	Value              *string       `json:"value"`
	Unit               string        `json:"unit"`
	Period             string        `json:"period"`
	SystemAsOf         string        `json:"system_as_of"`
	QualityStatus      QualityStatus `json:"quality_status"`
	MetricStatus       Status        `json:"metric_status"`
	CalculationVersion string        `json:"calculation_version"`
	SampleCount        int           `json:"sample_count"`
	LowerBound         *string       `json:"lower_bound"`
	UpperBound         *string       `json:"upper_bound"`
}

// NewDecimalQuantMetric 将精确因子计算结果转换为固定字段、固定类型的公开指标对象。
// systemAsOf 必须由可信调用上下文提供；本函数不会推导当前时间或输入快照质量。
// 结果值必须是 decimal_factor.go 生成的十进制字符串语法；不接受空串、指数、NaN 或前导零。
func NewDecimalQuantMetric(result DecimalResult, systemAsOf time.Time, quality QualityStatus, calculationVersion string) (DecimalQuantMetric, error) {
	if systemAsOf.IsZero() || !validQualityStatus(quality) || !semanticVersionPattern.MatchString(calculationVersion) {
		return DecimalQuantMetric{}, ErrInvalidMetric
	}
	period, minimumSamples, ok := factorPeriod(result.Code)
	if !ok || result.SampleCount < 0 {
		return DecimalQuantMetric{}, ErrInvalidMetric
	}

	metric := DecimalQuantMetric{
		MetricCode:         result.Code,
		Unit:               "ratio",
		Period:             period,
		SystemAsOf:         systemAsOf.UTC().Format(time.RFC3339Nano),
		QualityStatus:      quality,
		MetricStatus:       result.Status,
		CalculationVersion: calculationVersion,
		SampleCount:        result.SampleCount,
	}
	switch result.Status {
	case StatusAvailable:
		if result.SampleCount < minimumSamples || !decimalFactorClosePattern.MatchString(result.Value) {
			return DecimalQuantMetric{}, ErrInvalidMetric
		}
		if quality == QualityInvalid || quality == QualityUnknown {
			return DecimalQuantMetric{}, ErrInvalidMetric
		}
		value := result.Value
		metric.Value = &value
	case StatusInsufficientData:
		if result.SampleCount >= minimumSamples || result.Value != "" {
			return DecimalQuantMetric{}, ErrInvalidMetric
		}
		// 零值不是缺失值的替代；Value 保持 nil，序列化为显式 JSON null。
	default:
		return DecimalQuantMetric{}, ErrInvalidMetric
	}
	if err := metric.Validate(); err != nil {
		return DecimalQuantMetric{}, err
	}
	return metric, nil
}

// Validate 校验一个已反序列化的 DecimalQuantMetric 是否满足 QNT-CTR-004 首批 Decimal
// 因子的字段级约束：metric_code→period 映射、十进制字符串语法、semver、样本边界、
// 质量/状态一致性与规范 UTC 时间。NewDecimalQuantMetric 亦在末尾复用同一校验，
// 确保"构造时"与"从持久化 Payload 恢复时"使用同一份规则，避免两处漂移。
func (metric DecimalQuantMetric) Validate() error {
	period, minimumSamples, ok := factorPeriod(metric.MetricCode)
	if !ok || metric.Unit != "ratio" || metric.Period != period ||
		!semanticVersionPattern.MatchString(metric.CalculationVersion) ||
		!validQualityStatus(metric.QualityStatus) || metric.SampleCount < 0 ||
		!canonicalDecimalMetricTime(metric.SystemAsOf) ||
		metric.LowerBound != nil || metric.UpperBound != nil {
		return ErrInvalidMetric
	}
	switch metric.MetricStatus {
	case StatusAvailable:
		if metric.SampleCount < minimumSamples || metric.Value == nil ||
			!decimalFactorClosePattern.MatchString(*metric.Value) ||
			metric.QualityStatus == QualityInvalid || metric.QualityStatus == QualityUnknown {
			return ErrInvalidMetric
		}
	case StatusInsufficientData:
		if metric.SampleCount >= minimumSamples || metric.Value != nil {
			return ErrInvalidMetric
		}
	default:
		return ErrInvalidMetric
	}
	return nil
}

func canonicalDecimalMetricTime(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC &&
		strings.HasSuffix(value, "Z") && parsed.UTC().Format(time.RFC3339Nano) == value
}
