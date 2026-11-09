package trend

import (
	"encoding/json"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/metric"
)

func validResult() Result {
	return Result{
		TrendCode: "price_trend", Direction: DirectionUnknown, Phase: PhaseEmerging,
		Period: "20d", EvidenceMetrics: []metric.Metric{}, ConflictRefs: []string{},
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

func TestResultValidateStrengthAndRequiredCollections(t *testing.T) {
	for _, strength := range []float64{-0.01, 1.01} {
		result := validResult()
		result.Strength = &strength
		if err := result.Validate(); err == nil {
			t.Errorf("strength %v accepted", strength)
		}
	}
	result := validResult()
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
