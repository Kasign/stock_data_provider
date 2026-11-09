package metric

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidMetric = errors.New("invalid quant metric")
	decimalStringRE = regexp.MustCompile(`^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$`)
)

type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

type Status string

const (
	StatusAvailable          Status = "available"
	StatusInsufficientData   Status = "insufficient_data"
	StatusInsufficientSample Status = "insufficient_sample"
	StatusNotApplicable      Status = "not_applicable"
	StatusUnknown            Status = "unknown"
)

// Metric 是跨 Indicator/Factor 使用的 QuantMetric 结构。
// value/bounds 的具体 JSON 类型由 metric_code 对应的 Registry Schema 决定。
type Metric struct {
	MetricCode         string          `json:"metric_code"`
	Value              json.RawMessage `json:"value"`
	Unit               string          `json:"unit"`
	Period             string          `json:"period"`
	SystemAsOf         string          `json:"system_as_of"`
	QualityStatus      QualityStatus   `json:"quality_status"`
	MetricStatus       Status          `json:"metric_status"`
	CalculationVersion string          `json:"calculation_version"`
	SampleCount        *int            `json:"sample_count,omitempty"`
	LowerBound         json.RawMessage `json:"lower_bound"`
	UpperBound         json.RawMessage `json:"upper_bound"`
}

// Validate 只验证通用结构，不猜测 metric_code 的单位、精度、数值类型或样本口径。
func (m Metric) Validate() error {
	if !nonBlank(m.MetricCode) || !nonBlank(m.Unit) || !nonBlank(m.Period) ||
		!nonBlank(m.CalculationVersion) || !canonicalUTCTimestamp(m.SystemAsOf) ||
		!validQuality(m.QualityStatus) || !validStatus(m.MetricStatus) ||
		len(m.Value) == 0 || len(m.LowerBound) == 0 || len(m.UpperBound) == 0 {
		return ErrInvalidMetric
	}
	if m.SampleCount != nil && *m.SampleCount < 0 {
		return ErrInvalidMetric
	}

	valueKind, valueNull, ok := scalarKind(m.Value)
	if !ok {
		return ErrInvalidMetric
	}
	lowerKind, lowerNull, ok := scalarKind(m.LowerBound)
	if !ok {
		return ErrInvalidMetric
	}
	upperKind, upperNull, ok := scalarKind(m.UpperBound)
	if !ok {
		return ErrInvalidMetric
	}
	if lowerNull != upperNull {
		return ErrInvalidMetric
	}
	if valueNull {
		if m.MetricStatus == StatusAvailable || !lowerNull {
			return ErrInvalidMetric
		}
	} else if m.MetricStatus != StatusAvailable {
		return ErrInvalidMetric
	} else if !lowerNull {
		if valueKind != lowerKind || valueKind != upperKind {
			return ErrInvalidMetric
		}
		lower, err := decimalValue(m.LowerBound, lowerKind)
		if err != nil {
			return ErrInvalidMetric
		}
		value, err := decimalValue(m.Value, valueKind)
		if err != nil {
			return ErrInvalidMetric
		}
		upper, err := decimalValue(m.UpperBound, upperKind)
		if err != nil || lower.Cmp(value) > 0 || value.Cmp(upper) > 0 {
			return ErrInvalidMetric
		}
	}
	if m.MetricStatus == StatusAvailable &&
		(m.QualityStatus == QualityInvalid || m.QualityStatus == QualityUnknown) {
		return ErrInvalidMetric
	}
	return nil
}

func scalarKind(raw json.RawMessage) (string, bool, bool) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return "", false, false
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return "null", true, true
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", false, false
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return "", false, false
	}
	switch value.(type) {
	case json.Number:
		return "number", false, true
	case string:
		return "string", false, true
	default:
		return "", false, false
	}
}

// decimalValue 比较数值边界时使用精确有理数，避免 float64 舍入改变边界关系。
func decimalValue(raw json.RawMessage, kind string) (*big.Rat, error) {
	if len(raw) == 0 || len(raw) > 256 {
		return nil, ErrInvalidMetric
	}
	text := string(bytes.TrimSpace(raw))
	if kind == "string" {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil || !decimalStringRE.MatchString(value) {
			return nil, ErrInvalidMetric
		}
		text = value
	}
	if index := strings.IndexAny(text, "eE"); index >= 0 {
		exponent, err := strconv.Atoi(text[index+1:])
		if err != nil || exponent < -10000 || exponent > 10000 {
			return nil, ErrInvalidMetric
		}
	}
	value, ok := new(big.Rat).SetString(text)
	if !ok {
		return nil, ErrInvalidMetric
	}
	return value, nil
}

func canonicalUTCTimestamp(value string) bool {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	return err == nil && parsed.Location() == time.UTC &&
		strings.HasSuffix(value, "Z") && parsed.UTC().Format(time.RFC3339Nano) == value
}

func nonBlank(value string) bool {
	return strings.TrimSpace(value) != ""
}

func validQuality(value QualityStatus) bool {
	switch value {
	case QualityValid, QualityDegraded, QualityInvalid, QualityUnknown:
		return true
	default:
		return false
	}
}

func validStatus(value Status) bool {
	switch value {
	case StatusAvailable, StatusInsufficientData, StatusInsufficientSample, StatusNotApplicable, StatusUnknown:
		return true
	default:
		return false
	}
}
