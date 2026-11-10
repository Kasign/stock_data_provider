package trend

import (
	"errors"
	"math"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/quant/metric"
)

var ErrInvalidTrend = errors.New("invalid trend result")

type Direction string

const (
	DirectionBullish  Direction = "bullish"
	DirectionBearish  Direction = "bearish"
	DirectionSideways Direction = "sideways"
	DirectionUnknown  Direction = "unknown"
)

type Phase string

const (
	PhaseEmerging     Phase = "emerging"
	PhaseAccelerating Phase = "accelerating"
	PhaseMature       Phase = "mature"
	PhaseWeakening    Phase = "weakening"
	PhaseReversing    Phase = "reversing"
	PhaseUnknown      Phase = "unknown"
)

type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

// Result 是 QNT-CTR-004 TrendResult 的强类型 DTO。
type Result struct {
	TrendCode        string          `json:"trend_code"`
	Direction        Direction       `json:"direction"`
	Phase            Phase           `json:"phase"`
	Strength         *float64        `json:"strength"`
	Period           string          `json:"period"`
	EvidenceMetrics  []metric.Metric `json:"evidence_metrics"`
	ConflictRefs     []string        `json:"conflict_refs"`
	QualityStatus    QualityStatus   `json:"quality_status"`
	SystemAsOf       string          `json:"system_as_of"`
	AlgorithmVersion string          `json:"algorithm_version"`
}

// Validate 只校验 DTO 结构；趋势推断、强度归一化和质量策略由上层用例负责。
func (r Result) Validate() error {
	if !nonBlank(r.TrendCode) || !validDirection(r.Direction) ||
		!validPhase(r.Phase) || !nonBlank(r.Period) ||
		!validQuality(r.QualityStatus) || !canonicalUTCTimestamp(r.SystemAsOf) ||
		!nonBlank(r.AlgorithmVersion) || r.EvidenceMetrics == nil || r.ConflictRefs == nil {
		return ErrInvalidTrend
	}
	if r.Strength != nil &&
		(math.IsNaN(*r.Strength) || math.IsInf(*r.Strength, 0) || *r.Strength < 0 || *r.Strength > 1) {
		return ErrInvalidTrend
	}
	for _, item := range r.EvidenceMetrics {
		if item.Validate() != nil {
			return ErrInvalidTrend
		}
	}
	for _, ref := range r.ConflictRefs {
		if !nonBlank(ref) {
			return ErrInvalidTrend
		}
	}
	return nil
}

func nonBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

func canonicalUTCTimestamp(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC &&
		strings.HasSuffix(value, "Z") && parsed.UTC().Format(time.RFC3339Nano) == value
}

func validDirection(value Direction) bool {
	switch value {
	case DirectionBullish, DirectionBearish, DirectionSideways, DirectionUnknown:
		return true
	default:
		return false
	}
}

func validPhase(value Phase) bool {
	switch value {
	case PhaseEmerging, PhaseAccelerating, PhaseMature, PhaseWeakening, PhaseReversing, PhaseUnknown:
		return true
	default:
		return false
	}
}

func validQuality(value QualityStatus) bool {
	switch value {
	case QualityValid, QualityDegraded, QualityInvalid, QualityUnknown:
		return true
	default:
		return false
	}
}
