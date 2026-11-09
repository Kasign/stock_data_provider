package registry

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"example.com/stock-analysis-flow/internal/quant/metric"
)

var ErrInvalidMetricResult = errors.New("invalid quant metric result")

// ValidateMetric 按明确的 Registry 版本校验实际指标结果。
// sample_count 是否必需取决于指标专属 Schema；当前 Entry 没有该字段时不在此处推断。
func (r *Registry) ValidateMetric(code, schemaVersion, algorithmVersion string, raw json.RawMessage) error {
	entry, err := r.LookupVersion(code, schemaVersion, algorithmVersion)
	if err != nil {
		return err
	}
	if len(raw) == 0 || !json.Valid(raw) {
		return fmt.Errorf("%w: invalid JSON", ErrInvalidMetricResult)
	}
	if err := validateUniqueJSONKeys(raw); err != nil {
		return fmt.Errorf("%w: duplicate JSON keys: %v", ErrInvalidMetricResult, err)
	}

	var result metric.Metric
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&result); err != nil {
		return fmt.Errorf("%w: decode: %v", ErrInvalidMetricResult, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: trailing JSON", ErrInvalidMetricResult)
	}
	if err := result.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidMetricResult, err)
	}
	if result.MetricCode != entry.Code || result.Unit != entry.Unit ||
		result.CalculationVersion != entry.AlgorithmVersion {
		return ErrInvalidMetricResult
	}
	if !contains(entry.QualityPolicy.QualityStates, string(result.QualityStatus)) {
		return ErrInvalidMetricResult
	}
	if result.MetricStatus != metric.StatusAvailable {
		return nil
	}

	value, err := parseNumeric(result.Value, entry.ValueType, entry.Precision)
	if err != nil {
		return fmt.Errorf("%w: value: %v", ErrInvalidMetricResult, err)
	}
	if !entry.Range.Unbounded {
		minimum, err := parseNumeric(entry.Range.Minimum, entry.ValueType, entry.Precision)
		if err != nil {
			return fmt.Errorf("%w: registered minimum: %v", ErrInvalidEntry, err)
		}
		maximum, err := parseNumeric(entry.Range.Maximum, entry.ValueType, entry.Precision)
		if err != nil {
			return fmt.Errorf("%w: registered maximum: %v", ErrInvalidEntry, err)
		}
		if value.Cmp(minimum) < 0 || value.Cmp(maximum) > 0 {
			return ErrInvalidMetricResult
		}
	}
	for _, bound := range []json.RawMessage{result.LowerBound, result.UpperBound} {
		if bytes.Equal(bytes.TrimSpace(bound), []byte("null")) {
			continue
		}
		if _, err := parseNumeric(bound, entry.ValueType, entry.Precision); err != nil {
			return fmt.Errorf("%w: bound: %v", ErrInvalidMetricResult, err)
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
