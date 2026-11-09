package indicator

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type atrBoundaryGolden struct {
	FixtureType      string `json:"fixture_type"`
	SchemaVersion    string `json:"schema_version"`
	IndicatorCode    string `json:"indicator_code"`
	AlgorithmVersion string `json:"algorithm_version"`
	Parameters       struct {
		Window          int    `json:"window"`
		SmoothingMethod string `json:"smoothing_method"`
	} `json:"parameters"`
	Input struct {
		High          []float64 `json:"high"`
		Low           []float64 `json:"low"`
		Close         []float64 `json:"close"`
		PreviousClose float64   `json:"previous_close"`
	} `json:"input"`
	Expected struct {
		Value       float64 `json:"value"`
		SampleCount int     `json:"sample_count"`
	} `json:"expected"`
	BoundaryAssertion struct {
		AlternatePreviousClose float64 `json:"alternate_previous_close"`
		ExpectedValue          float64 `json:"expected_value"`
	} `json:"boundary_assertion"`
	Tolerance float64 `json:"tolerance"`
}

func TestATRPreviousCloseBoundaryGolden(t *testing.T) {
	path := filepath.Join("../../../doc/quant/golden/indicator", "atr-previous-close-boundary.golden.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read ATR boundary golden: %v", err)
	}
	var fixture atrBoundaryGolden
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode ATR boundary golden: %v", err)
	}
	if fixture.FixtureType != "synthetic_fixture" || fixture.SchemaVersion != "1.0.0" ||
		fixture.IndicatorCode != "atr" || fixture.AlgorithmVersion != "1.0.0" {
		t.Fatalf("unexpected ATR boundary fixture metadata: %+v", fixture)
	}
	if fixture.Parameters.Window < 1 || fixture.Parameters.SmoothingMethod != "wilder" ||
		len(fixture.Input.High) != fixture.Parameters.Window ||
		len(fixture.Input.Low) != fixture.Parameters.Window ||
		len(fixture.Input.Close) != fixture.Parameters.Window ||
		fixture.Expected.SampleCount != fixture.Parameters.Window ||
		fixture.Tolerance <= 0 || math.IsNaN(fixture.Tolerance) || math.IsInf(fixture.Tolerance, 0) {
		t.Fatalf("invalid ATR boundary fixture shape: %+v", fixture)
	}
	assertValue := func(name string, got, want float64) {
		t.Helper()
		if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > fixture.Tolerance {
			t.Fatalf("%s: got %.15g, want %.15g (tolerance %.3g)", name, got, want, fixture.Tolerance)
		}
	}
	params := ATRParams{Window: fixture.Parameters.Window, SmoothingMethod: fixture.Parameters.SmoothingMethod}
	got, err := ATR(fixture.Input.High, fixture.Input.Low, fixture.Input.Close, fixture.Input.PreviousClose, params)
	if err != nil {
		t.Fatalf("ATR with fixture previous_close: %v", err)
	}
	if got.Status != StatusAvailable || got.SampleCount != fixture.Expected.SampleCount {
		t.Fatalf("unexpected ATR result metadata: %+v", got)
	}
	assertValue("value", got.Value, fixture.Expected.Value)

	alternate, err := ATR(fixture.Input.High, fixture.Input.Low, fixture.Input.Close, fixture.BoundaryAssertion.AlternatePreviousClose, params)
	if err != nil {
		t.Fatalf("ATR with alternate previous_close: %v", err)
	}
	assertValue("alternate previous_close value", alternate.Value, fixture.BoundaryAssertion.ExpectedValue)
	if math.Abs(got.Value-alternate.Value) <= fixture.Tolerance {
		t.Fatal("changing the preceding close must affect the first true range")
	}
}
