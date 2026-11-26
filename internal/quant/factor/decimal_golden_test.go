package factor

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type decimalFactorGolden struct {
	FixtureType      string `json:"fixture_type"`
	SchemaVersion    string `json:"schema_version"`
	FactorCode       string `json:"factor_code"`
	AlgorithmVersion string `json:"algorithm_version"`
	ValueType        string `json:"value_type"`
	Scale            int    `json:"scale"`
	RoundingMode     string `json:"rounding_mode"`
	Input            struct {
		Close []string `json:"close"`
	} `json:"input"`
	Expected struct {
		Value       string `json:"value"`
		SampleCount int    `json:"sample_count"`
	} `json:"expected"`
}

func TestDecimalFactorGoldenFixtures(t *testing.T) {
	cases := []struct {
		code             string
		algorithmVersion string
		fn               func([]string) (DecimalResult, error)
	}{
		{"ma20_distance", "1.0.0", DecimalMADistance20},
		{"momentum_5d", "1.0.0", DecimalMomentum5},
		{"momentum_20d", "1.0.0", DecimalMomentum20},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			path := filepath.Join("../../../doc/quant/golden/factor/decimal", tc.code+".golden.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read decimal golden fixture: %v", err)
			}
			var fixture decimalFactorGolden
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("decode decimal golden fixture: %v", err)
			}
			if fixture.FixtureType != "synthetic_fixture" || fixture.SchemaVersion != "1.0.0" ||
				fixture.AlgorithmVersion != tc.algorithmVersion || fixture.FactorCode != tc.code {
				t.Fatalf("unexpected fixture metadata: %+v", fixture)
			}
			if fixture.ValueType != "decimal_string" || fixture.Scale != DecimalFactorScale || fixture.RoundingMode != "half_even" {
				t.Fatalf("unexpected precision metadata: value_type=%s scale=%d rounding=%s", fixture.ValueType, fixture.Scale, fixture.RoundingMode)
			}
			if !decimalFactorClosePattern.MatchString(fixture.Expected.Value) {
				t.Fatalf("golden expected value is not a decimal string: %q", fixture.Expected.Value)
			}

			got, err := tc.fn(fixture.Input.Close)
			if err != nil {
				t.Fatal(err)
			}
			if got.Code != tc.code || got.Status != StatusAvailable {
				t.Fatalf("unexpected result metadata: %+v", got)
			}
			if got.SampleCount != fixture.Expected.SampleCount {
				t.Fatalf("sample_count: got %d, want %d", got.SampleCount, fixture.Expected.SampleCount)
			}
			// 精确字符串比较：十进制结果不允许浮点容差比较。
			if got.Value != fixture.Expected.Value {
				t.Fatalf("value: got %q, want %q", got.Value, fixture.Expected.Value)
			}
		})
	}
}
