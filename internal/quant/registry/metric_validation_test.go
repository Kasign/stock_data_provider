package registry

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestRegistryValidateMetricSupportsDecimalStringAndInteger(t *testing.T) {
	decimalRegistry := New()
	decimal := validEntry("decimal_metric", "1.0.0")
	decimal.ValueType = ValueTypeDecimalString
	decimal.Precision.MaxIntegerDigits = 2
	decimal.Precision.MaxFractionDigits = 8
	decimal.Precision.Tolerance = json.RawMessage("\"0.00000001\"")
	decimal.Range = NumericRange{Minimum: json.RawMessage("\"-1\""), Maximum: json.RawMessage("\"1\"")}
	if err := decimalRegistry.Register(decimal, true); err != nil {
		t.Fatal(err)
	}
	decimalResult := json.RawMessage(`{"metric_code":"decimal_metric","value":"0.12345678","unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := decimalRegistry.ValidateMetric("decimal_metric", "1.0.0", "1.0.0", decimalResult); err != nil {
		t.Fatalf("valid decimal-string metric rejected: %v", err)
	}
	decimalResult = json.RawMessage(`{"metric_code":"decimal_metric","value":0.12345678,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := decimalRegistry.ValidateMetric("decimal_metric", "1.0.0", "1.0.0", decimalResult); !errors.Is(err, ErrInvalidMetricResult) {
		t.Fatalf("JSON number must not satisfy decimal_string metadata, got %v", err)
	}

	integerRegistry := New()
	integer := validEntry("sample_count_metric", "1.0.0")
	integer.ValueType = ValueTypeInteger
	integer.Precision.MaxIntegerDigits = 4
	integer.Precision.MaxFractionDigits = 0
	integer.Precision.Tolerance = json.RawMessage("0")
	integer.Range = NumericRange{Minimum: json.RawMessage("0"), Maximum: json.RawMessage("1000")}
	if err := integerRegistry.Register(integer, true); err != nil {
		t.Fatal(err)
	}
	integerResult := json.RawMessage(`{"metric_code":"sample_count_metric","value":12,"unit":"count","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := integerRegistry.ValidateMetric("sample_count_metric", "1.0.0", "1.0.0", integerResult); err != nil {
		t.Fatalf("valid integer metric rejected: %v", err)
	}
	integerResult = json.RawMessage(`{"metric_code":"sample_count_metric","value":12.5,"unit":"count","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := integerRegistry.ValidateMetric("sample_count_metric", "1.0.0", "1.0.0", integerResult); !errors.Is(err, ErrInvalidMetricResult) {
		t.Fatalf("fractional value must not satisfy integer metadata, got %v", err)
	}
}

func TestRegistryValidateMetricAcceptsUnavailableNullValue(t *testing.T) {
	registry := New()
	entry := validEntry("sample_metric", "1.0.0")
	if err := registry.Register(entry, true); err != nil {
		t.Fatal(err)
	}
	result := json.RawMessage(`{"metric_code":"sample_metric","value":null,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"insufficient_sample","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := registry.ValidateMetric("sample_metric", "1.0.0", "1.0.0", result); err != nil {
		t.Fatalf("valid unavailable metric rejected: %v", err)
	}
}


func TestRegistryValidateMetricHonorsSupportsInterval(t *testing.T) {
	result := json.RawMessage(`{"metric_code":"bounded_metric","value":0.5,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":0.4,"upper_bound":0.6}`)

	withoutInterval := New()
	entry := validEntry("bounded_metric", "1.0.0")
	if err := withoutInterval.Register(entry, true); err != nil {
		t.Fatal(err)
	}
	if err := withoutInterval.ValidateMetric("bounded_metric", "1.0.0", "1.0.0", result); !errors.Is(err, ErrInvalidMetricResult) {
		t.Fatalf("interval bounds must be rejected when supports_interval is false, got %v", err)
	}

	withInterval := New()
	entry.SupportsInterval = true
	if err := withInterval.Register(entry, true); err != nil {
		t.Fatal(err)
	}
	if err := withInterval.ValidateMetric("bounded_metric", "1.0.0", "1.0.0", result); err != nil {
		t.Fatalf("interval bounds must be accepted when supports_interval is true, got %v", err)
	}
}
