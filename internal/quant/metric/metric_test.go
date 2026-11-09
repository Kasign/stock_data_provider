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
