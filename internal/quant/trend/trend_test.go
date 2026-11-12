package trend

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/metric"
)

func validResult() Result {
	strength := 0.35
	return Result{
		TrendCode: "price_trend", Direction: DirectionUnknown, Phase: PhaseEmerging,
		Strength: &strength, Period: "20d",
		EvidenceMetrics: []metric.Metric{}, ConflictRefs: []string{},
		QualityStatus: QualityValid, SystemAsOf: "2026-11-09T01:00:00Z",
		AlgorithmVersion: "1.0.0",
	}
}

func TestResultValidateSeparatesDirectionAndPhase(t *testing.T) {
	result := validResult()
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Result
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Direction != DirectionUnknown || decoded.Phase != PhaseEmerging {
		t.Fatalf("direction/phase changed: %#v", decoded)
	}
}

func TestResultMatchesGolden(t *testing.T) {
	encoded, err := json.Marshal(validResult())
	if err != nil {
		t.Fatalf("marshal TrendResult: %v", err)
	}
	goldenPath := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "trend-result-1.0.0.golden.json")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read TrendResult Golden: %v", err)
	}
	var got, want Result
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode generated TrendResult: %v", err)
	}
	if err := json.Unmarshal(golden, &want); err != nil {
		t.Fatalf("decode TrendResult Golden: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("TrendResult Golden mismatch: got=%s want=%s", encoded, golden)
	}
}

func TestResultValidateStrengthAndRequiredCollections(t *testing.T) {
	for _, strength := range []float64{-0.01, 1.01} {
		result := validResult()
		result.Strength = &strength
		if err := result.Validate(); err == nil {
			t.Errorf("strength %v accepted", strength)
		}
	}
	result := validResult()
	result.Strength = nil
	if err := result.Validate(); err != nil {
		t.Fatalf("nullable strength rejected: %v", err)
	}
	result.EvidenceMetrics = nil
	if err := result.Validate(); err == nil {
		t.Fatal("missing evidence_metrics accepted")
	}
	result = validResult()
	result.ConflictRefs = []string{" "}
	if err := result.Validate(); err == nil {
		t.Fatal("blank conflict ref accepted")
	}
}

func TestResultValidateRequiresEvidenceMetricSystemAsOfMatch(t *testing.T) {
	result := validResult()
	result.EvidenceMetrics = []metric.Metric{{
		MetricCode:         "close_price",
		Value:              json.RawMessage("10"),
		Unit:               "CNY",
		Period:             "20d",
		SystemAsOf:         "2026-11-09T00:59:00Z",
		QualityStatus:      metric.QualityValid,
		MetricStatus:       metric.StatusAvailable,
		CalculationVersion: "1.0.0",
		LowerBound:         json.RawMessage("null"),
		UpperBound:         json.RawMessage("null"),
	}}
	if err := result.Validate(); err == nil {
		t.Fatal("TrendResult accepted an evidence Metric with a different system_as_of")
	}
	result.EvidenceMetrics[0].SystemAsOf = result.SystemAsOf
	if err := result.Validate(); err != nil {
		t.Fatalf("TrendResult rejected evidence Metric with matching system_as_of: %v", err)
	}
}

func TestResultValidateRejectsInvalidUTF8Text(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Result)
	}{
		{"trend code", func(r *Result) { r.TrendCode = string([]byte{0xff}) }},
		{"trend code padded", func(r *Result) { r.TrendCode = " price_trend " }},
		{"period", func(r *Result) { r.Period = string([]byte{0xff}) }},
		{"period padded", func(r *Result) { r.Period = " 20d " }},
		{"algorithm version", func(r *Result) { r.AlgorithmVersion = string([]byte{0xff}) }},
		{"algorithm version padded", func(r *Result) { r.AlgorithmVersion = " 1.0.0 " }},
		{"conflict ref", func(r *Result) { r.ConflictRefs = []string{string([]byte{0xff})} }},
		{"conflict ref padded", func(r *Result) { r.ConflictRefs = []string{" ref:trend "} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validResult()
			tc.mutate(&result)
			if err := result.Validate(); err == nil {
				t.Fatal("invalid UTF-8 input accepted")
			}
		})
	}
}
