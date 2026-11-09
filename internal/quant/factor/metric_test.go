package factor

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestNewQuantMetricMatchesFactorResultGolden(t *testing.T) {
	tests := []struct {
		name   string
		result Result
		golden string
	}{
		{
			name: "momentum available",
			result: Result{Code: "momentum_20d", Value: 0.2, Unit: "ratio", Status: StatusAvailable, SampleCount: 21},
			golden: "momentum_20d-available.golden.json",
		},
		{
			name: "momentum insufficient",
			result: Result{Code: "momentum_20d", Unit: "ratio", Status: StatusInsufficientData, SampleCount: 20},
			golden: "momentum_20d-insufficient.golden.json",
		},
		{
			name: "momentum 60d available",
			result: Result{Code: "momentum_60d", Value: 0.6, Unit: "ratio", Status: StatusAvailable, SampleCount: 61},
			golden: "momentum_60d-available.golden.json",
		},
		{
			name: "momentum 60d insufficient",
			result: Result{Code: "momentum_60d", Unit: "ratio", Status: StatusInsufficientData, SampleCount: 60},
			golden: "momentum_60d-insufficient.golden.json",
		},
		{
			name: "volatility available",
			result: Result{Code: "volatility_20d", Value: 0.01, Unit: "ratio", Status: StatusAvailable, SampleCount: 20},
			golden: "volatility_20d-available.golden.json",
		},
		{
			name: "volatility insufficient",
			result: Result{Code: "volatility_20d", Unit: "ratio", Status: StatusInsufficientData, SampleCount: 19},
			golden: "volatility_20d-insufficient.golden.json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NewQuantMetric(tc.result, time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC), QualityValid, "1.0.0")
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			raw, err := os.ReadFile(filepath.Join("../../../doc/quant/golden/result/factor", tc.golden))
			if err != nil {
				t.Fatal(err)
			}
			var want QuantMetric
			if err := json.Unmarshal(raw, &want); err != nil {
				t.Fatal(err)
			}
			var decoded QuantMetric
			if err := json.Unmarshal(encoded, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded, want) {
				t.Fatalf("serialized metric mismatch\n got: %s\nwant: %s", encoded, raw)
			}
			if tc.result.Status == StatusInsufficientData && string(encoded) == "" {
				t.Fatal("unreachable empty JSON encoding")
			}
		})
	}
}

func TestNewQuantMetricAllowsDegradedStructureButRejectsInvalidAvailability(t *testing.T) {
	result := Result{Code: "momentum_20d", Value: 0.2, Unit: "ratio", Status: StatusAvailable, SampleCount: 21}
	got, err := NewQuantMetric(result, time.Date(2026, 11, 9, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)), QualityDegraded, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if got.SystemAsOf != "2026-11-09T00:00:00Z" {
		t.Fatalf("system_as_of was not normalized to UTC: %s", got.SystemAsOf)
	}
	if _, err := NewQuantMetric(result, time.Now(), QualityInvalid, "1.0.0"); !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("invalid quality should reject available result, got %v", err)
	}
	if _, err := NewQuantMetric(result, time.Now(), QualityUnknown, "1.0.0"); !errors.Is(err, ErrInvalidMetric) {
		t.Fatalf("unknown quality should reject available result, got %v", err)
	}
}

func TestNewQuantMetricRejectsInvalidMetadataAndNonFiniteValues(t *testing.T) {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	valid := Result{Code: "momentum_20d", Value: 0.2, Unit: "ratio", Status: StatusAvailable, SampleCount: 21}
	cases := []struct {
		name   string
		result Result
		asOf   time.Time
		quality QualityStatus
		version string
	}{
		{name: "unknown metric code", result: Result{Code: "made_up", Unit: "ratio", Status: StatusAvailable, Value: 1, SampleCount: 1}, asOf: now, quality: QualityValid, version: "1.0.0"},
		{name: "wrong unit", result: Result{Code: "momentum_20d", Unit: "percent", Status: StatusAvailable, Value: 1, SampleCount: 21}, asOf: now, quality: QualityValid, version: "1.0.0"},
		{name: "missing as of", result: valid, quality: QualityValid, version: "1.0.0"},
		{name: "invalid version", result: valid, asOf: now, quality: QualityValid, version: "v1"},
		{name: "unknown quality", result: valid, asOf: now, quality: "not_known", version: "1.0.0"},
		{name: "negative sample count", result: Result{Code: "momentum_20d", Unit: "ratio", Status: StatusInsufficientData, SampleCount: -1}, asOf: now, quality: QualityValid, version: "1.0.0"},
		{name: "non finite value", result: Result{Code: "momentum_20d", Unit: "ratio", Status: StatusAvailable, Value: 1.0/0.0, SampleCount: 21}, asOf: now, quality: QualityValid, version: "1.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewQuantMetric(tc.result, tc.asOf, tc.quality, tc.version); !errors.Is(err, ErrInvalidMetric) {
				t.Fatalf("expected ErrInvalidMetric, got %v", err)
			}
		})
	}
}
