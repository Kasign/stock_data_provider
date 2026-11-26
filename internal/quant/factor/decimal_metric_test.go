package factor

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestNewDecimalQuantMetricAvailableEmitsStringValue(t *testing.T) {
	asOf := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	result := DecimalResult{Code: "momentum_5d", Value: "0.052631578947", Status: StatusAvailable, SampleCount: 6}
	metric, err := NewDecimalQuantMetric(result, asOf, QualityValid, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if metric.Value == nil || *metric.Value != result.Value || metric.Unit != "ratio" || metric.Period != "5d" {
		t.Fatalf("unexpected metric: %+v", metric)
	}
	encoded, err := json.Marshal(metric)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"value":"0.052631578947"`) {
		t.Fatalf("value must serialize as JSON string, got %s", encoded)
	}
}

func TestNewDecimalQuantMetricInsufficientEmitsNullValue(t *testing.T) {
	asOf := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	result := DecimalResult{Code: "ma20_distance", Status: StatusInsufficientData, SampleCount: 19}
	metric, err := NewDecimalQuantMetric(result, asOf, QualityValid, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if metric.Value != nil {
		t.Fatalf("insufficient value must be nil, got %q", *metric.Value)
	}
	encoded, err := json.Marshal(metric)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"value":null`) {
		t.Fatalf("insufficient value must serialize as explicit JSON null, got %s", encoded)
	}
}

func TestNewDecimalQuantMetricRejectsInvalidInput(t *testing.T) {
	asOf := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		result  DecimalResult
		quality QualityStatus
		version string
		asOf    time.Time
	}{
		{"unknown code", DecimalResult{Code: "unknown_factor", Value: "0.1", Status: StatusAvailable, SampleCount: 6}, QualityValid, "1.0.0", asOf},
		{"available too few samples", DecimalResult{Code: "momentum_5d", Value: "0.1", Status: StatusAvailable, SampleCount: 5}, QualityValid, "1.0.0", asOf},
		{"available non decimal value", DecimalResult{Code: "momentum_5d", Value: "NaN", Status: StatusAvailable, SampleCount: 6}, QualityValid, "1.0.0", asOf},
		{"available float formatted value", DecimalResult{Code: "momentum_5d", Value: "0.1 ", Status: StatusAvailable, SampleCount: 6}, QualityValid, "1.0.0", asOf},
		{"available invalid quality", DecimalResult{Code: "momentum_5d", Value: "0.1", Status: StatusAvailable, SampleCount: 6}, QualityInvalid, "1.0.0", asOf},
		{"insufficient with value", DecimalResult{Code: "momentum_5d", Value: "0.1", Status: StatusInsufficientData, SampleCount: 5}, QualityValid, "1.0.0", asOf},
		{"insufficient not short", DecimalResult{Code: "momentum_5d", Status: StatusInsufficientData, SampleCount: 6}, QualityValid, "1.0.0", asOf},
		{"zero time", DecimalResult{Code: "momentum_5d", Value: "0.1", Status: StatusAvailable, SampleCount: 6}, QualityValid, "1.0.0", time.Time{}},
		{"bad version", DecimalResult{Code: "momentum_5d", Value: "0.1", Status: StatusAvailable, SampleCount: 6}, QualityValid, "v1", asOf},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewDecimalQuantMetric(tc.result, tc.asOf, tc.quality, tc.version); err != ErrInvalidMetric {
				t.Fatalf("error = %v, want ErrInvalidMetric", err)
			}
		})
	}
}
