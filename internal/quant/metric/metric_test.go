package metric

import (
	"encoding/json"
	"testing"
)

func TestMetricValidateAvailableAndUnavailable(t *testing.T) {
	value := Metric{
		MetricCode: "sample_metric", Value: json.RawMessage("0.25"),
		Unit: "ratio", Period: "20d", SystemAsOf: "2026-11-09T01:00:00Z",
		QualityStatus: QualityValid, MetricStatus: StatusAvailable,
		CalculationVersion: "1.0.0", LowerBound: json.RawMessage("0.2"),
		UpperBound: json.RawMessage("0.3"),
	}
	if err := value.Validate(); err != nil {
		t.Fatalf("valid metric rejected: %v", err)
	}
	value.Value = json.RawMessage("null")
	value.MetricStatus = StatusInsufficientSample
	value.LowerBound = json.RawMessage("null")
	value.UpperBound = json.RawMessage("null")
	if err := value.Validate(); err != nil {
		t.Fatalf("valid unavailable metric rejected: %v", err)
	}
}

func TestMetricValidateRejectsUnsafeCombinations(t *testing.T) {
	base := Metric{
		MetricCode: "sample_metric", Value: json.RawMessage("0.25"),
		Unit: "ratio", Period: "20d", SystemAsOf: "2026-11-09T01:00:00Z",
		QualityStatus: QualityValid, MetricStatus: StatusAvailable,
		CalculationVersion: "1.0.0", LowerBound: json.RawMessage("0.2"),
		UpperBound: json.RawMessage("0.3"),
	}
	cases := []Metric{
		func() Metric { v := base; v.QualityStatus = QualityUnknown; return v }(),
		func() Metric {
			v := base
			v.LowerBound = json.RawMessage("0.2")
			v.UpperBound = json.RawMessage("null")
			return v
		}(),
		func() Metric { v := base; v.Value = json.RawMessage("null"); return v }(),
		func() Metric { v := base; v.Value = json.RawMessage("{}"); return v }(),
		func() Metric {
			v := base
			v.SystemAsOf = "2026-11-09T01:00:00.000Z"
			return v
		}(),
		func() Metric { v := base; v.LowerBound = json.RawMessage("0.26"); return v }(),
		func() Metric { v := base; v.UpperBound = json.RawMessage("0.24"); return v }(),
		func() Metric { v := base; v.LowerBound = json.RawMessage("\"0.2\""); return v }(),
	}
	for i, value := range cases {
		if err := value.Validate(); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
}

func TestMetricValidateComparesDecimalStringBoundsExactly(t *testing.T) {
	value := Metric{
		MetricCode: "decimal_metric", Value: json.RawMessage("\"0.3000000000000000001\""),
		Unit: "ratio", Period: "20d", SystemAsOf: "2026-11-09T01:00:00Z",
		QualityStatus: QualityValid, MetricStatus: StatusAvailable,
		CalculationVersion: "1.0.0", LowerBound: json.RawMessage("\"0.3\""),
		UpperBound: json.RawMessage("\"0.3000000000000000002\""),
	}
	if err := value.Validate(); err != nil {
		t.Fatalf("valid exact decimal bounds rejected: %v", err)
	}
	value.UpperBound = json.RawMessage("\"0.3000000000000000000\"")
	if err := value.Validate(); err == nil {
		t.Fatal("upper bound below exact decimal value unexpectedly accepted")
	}
}

func TestMetricValidateAllowsUnavailableBoundsForAvailableValue(t *testing.T) {
	value := Metric{
		MetricCode: "sample_metric", Value: json.RawMessage("0.25"),
		Unit: "ratio", Period: "20d", SystemAsOf: "2026-11-09T01:00:00Z",
		QualityStatus: QualityValid, MetricStatus: StatusAvailable,
		CalculationVersion: "1.0.0", LowerBound: json.RawMessage("null"),
		UpperBound: json.RawMessage("null"),
	}
	if err := value.Validate(); err != nil {
		t.Fatalf("available metric with absent optional bounds rejected: %v", err)
	}
}

func TestMetricValidateRejectsInvalidUTF8TextAndJSONValues(t *testing.T) {
	base := Metric{
		MetricCode: "sample_metric", Value: json.RawMessage("0.25"),
		Unit: "ratio", Period: "20d", SystemAsOf: "2026-11-09T01:00:00Z",
		QualityStatus: QualityValid, MetricStatus: StatusAvailable,
		CalculationVersion: "1.0.0", LowerBound: json.RawMessage("null"),
		UpperBound: json.RawMessage("null"),
	}
	cases := []struct {
		name   string
		mutate func(*Metric)
	}{
		{"metric code", func(m *Metric) { m.MetricCode = string([]byte{0xff}) }},
		{"unit", func(m *Metric) { m.Unit = string([]byte{0xff}) }},
		{"period", func(m *Metric) { m.Period = string([]byte{0xff}) }},
		{"calculation version", func(m *Metric) { m.CalculationVersion = string([]byte{0xff}) }},
		{"value JSON", func(m *Metric) { m.Value = json.RawMessage(append([]byte{'"'}, 0xff, '"')) }},
		{"lower bound JSON", func(m *Metric) { m.LowerBound = json.RawMessage(append([]byte{'"'}, 0xff, '"')) }},
		{"upper bound JSON", func(m *Metric) { m.UpperBound = json.RawMessage(append([]byte{'"'}, 0xff, '"')) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			value := base
			tc.mutate(&value)
			if err := value.Validate(); err == nil {
				t.Fatal("invalid UTF-8 input accepted")
			}
		})
	}
}
