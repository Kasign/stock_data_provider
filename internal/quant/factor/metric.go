package factor

import (
	"errors"
	"math"
	"regexp"
	"time"
)

var ErrInvalidMetric = errors.New("invalid factor QuantMetric")

type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

// QuantMetric 是首批 Factor 的强类型 QNT-CTR-004 子对象。
// 是否允许 degraded 结果作为可用事实，仍由上层 Registry quality_policy 决定。
type QuantMetric struct {
	MetricCode        string        `json:"metric_code"`
	Value             *float64      `json:"value"`
	Unit              string        `json:"unit"`
	Period            string        `json:"period"`
	SystemAsOf        string        `json:"system_as_of"`
	QualityStatus     QualityStatus `json:"quality_status"`
	MetricStatus      Status        `json:"metric_status"`
	CalculationVersion string       `json:"calculation_version"`
	SampleCount       int           `json:"sample_count"`
	LowerBound        *float64      `json:"lower_bound"`
	UpperBound        *float64      `json:"upper_bound"`
}

var semanticVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$`)

// NewQuantMetric 将 Factor 纯计算结果转换为固定字段、固定类型的公开指标对象。
// systemAsOf 必须由可信调用上下文提供；本函数不会推导当前时间或输入快照质量。
func NewQuantMetric(result Result, systemAsOf time.Time, quality QualityStatus, calculationVersion string) (QuantMetric, error) {
	if systemAsOf.IsZero() || !validQualityStatus(quality) || !semanticVersionPattern.MatchString(calculationVersion) {
		return QuantMetric{}, ErrInvalidMetric
	}
	period, minimumSamples, ok := factorPeriod(result.Code)
	if !ok || result.Unit != "ratio" || result.SampleCount < 0 {
		return QuantMetric{}, ErrInvalidMetric
	}

	metric := QuantMetric{
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
		if result.SampleCount < minimumSamples {
			return QuantMetric{}, ErrInvalidMetric
		}
		if quality == QualityInvalid || quality == QualityUnknown ||
			math.IsNaN(result.Value) || math.IsInf(result.Value, 0) {
			return QuantMetric{}, ErrInvalidMetric
		}
		value := result.Value
		metric.Value = &value
	case StatusInsufficientData:
		if result.SampleCount >= minimumSamples {
			return QuantMetric{}, ErrInvalidMetric
		}
		// 零值不是缺失值的替代；Value 保持 nil，序列化为显式 JSON null。
	default:
		return QuantMetric{}, ErrInvalidMetric
	}
	return metric, nil
}

func factorPeriod(code string) (string, int, bool) {
	switch code {
	case "momentum_20d":
		return "20d", 21, true
	case "momentum_60d":
		return "60d", 61, true
	case "volatility_20d":
		return "20d", 20, true
	default:
		return "", 0, false
	}
}

func validQualityStatus(status QualityStatus) bool {
	switch status {
	case QualityValid, QualityDegraded, QualityInvalid, QualityUnknown:
		return true
	default:
		return false
	}
}
