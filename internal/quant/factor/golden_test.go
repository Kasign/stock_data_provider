package factor

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type factorGolden struct {
	FixtureType      string `json:"fixture_type"`
	SchemaVersion    string `json:"schema_version"`
	FactorCode       string `json:"factor_code"`
	AlgorithmVersion string `json:"algorithm_version"`
	Input            struct {
		Close []float64 `json:"close"`
	} `json:"input"`
	Expected struct {
		Value       float64 `json:"value"`
		SampleCount int     `json:"sample_count"`
	} `json:"expected"`
	Tolerance float64 `json:"tolerance"`
}

func TestFactorGoldenFixtures(t *testing.T) {
	codes := []string{"momentum_20d", "momentum_60d", "volatility_20d"}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			path := filepath.Join("../../../doc/quant/golden/factor", code+".golden.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden fixture: %v", err)
			}
			var fixture factorGolden
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("decode golden fixture: %v", err)
			}
			if fixture.FixtureType != "synthetic_fixture" || fixture.SchemaVersion != "1.0.0" ||
				fixture.AlgorithmVersion != "1.0.0" || fixture.FactorCode != code {
				t.Fatalf("unexpected fixture metadata: %+v", fixture)
			}
			if fixture.Tolerance <= 0 || math.IsNaN(fixture.Tolerance) || math.IsInf(fixture.Tolerance, 0) {
				t.Fatalf("invalid tolerance: %v", fixture.Tolerance)
			}

			var got Result
			switch code {
			case "momentum_20d":
				got, err = Momentum20(fixture.Input.Close)
			case "momentum_60d":
				got, err = Momentum60(fixture.Input.Close)
			case "volatility_20d":
				got, err = Volatility20(fixture.Input.Close)
			default:
				t.Fatalf("unsupported factor code %q", code)
			}
			if err != nil {
				t.Fatal(err)
			}
			if got.Code != code || got.Status != StatusAvailable || got.Unit != "ratio" {
				t.Fatalf("unexpected result metadata: %+v", got)
			}
			if got.SampleCount != fixture.Expected.SampleCount {
				t.Fatalf("sample_count: got %d, want %d", got.SampleCount, fixture.Expected.SampleCount)
			}
			if math.IsNaN(got.Value) || math.IsInf(got.Value, 0) || math.Abs(got.Value-fixture.Expected.Value) > fixture.Tolerance {
				t.Fatalf("value: got %.15g, want %.15g (tolerance %.3g)", got.Value, fixture.Expected.Value, fixture.Tolerance)
			}
		})
	}
}
