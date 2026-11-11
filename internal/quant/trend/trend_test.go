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
