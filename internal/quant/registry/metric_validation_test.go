package registry

import (
	"encoding/json"
	"errors"
	"strings"
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

	invalidDecimalValues := []string{
		`"1e-2"`, `"01"`, `"+1"`, `".5"`, `"1."`,
		`" 0.1"`, `"0.1 "`, `"NaN"`, `1.25`,
	}
	for _, value := range invalidDecimalValues {
		t.Run("invalid_metric_value_"+strings.Trim(value, "\\\""), func(t *testing.T) {
			candidate := strings.Replace(string(decimalResult), `"0.12345678"`, value, 1)
			if err := decimalRegistry.ValidateMetric("decimal_metric", "1.0.0", "1.0.0", json.RawMessage(candidate)); !errors.Is(err, ErrInvalidMetricResult) {
				t.Fatalf("invalid decimal-string value %s should be rejected, got %v", value, err)
			}
		})
	}
	decimalResult = json.RawMessage(`{"metric_code":"decimal_metric","value":0.12345678,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := decimalRegistry.ValidateMetric("decimal_metric", "1.0.0", "1.0.0", decimalResult); !errors.Is(err, ErrInvalidMetricResult) {
		t.Fatalf("JSON number must not satisfy decimal_string metadata, got %v", err)
	}

	integerRegistry := New()
	integer := validEntry("sample_count_metric", "1.0.0")
	integer.ValueType = ValueTypeInteger
	integer.Unit = "count"
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

func TestRegistryValidateMetricEnforcesSampleCountRequiredness(t *testing.T) {
	registry := New()
	entry := validEntry("required_sample_metric", "1.0.0")
	entry.SampleCount.RequiredInMetric = true
	if err := registry.Register(entry, true); err != nil {
		t.Fatal(err)
	}

	missing := json.RawMessage(`{"metric_code":"required_sample_metric","value":0.25,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := registry.ValidateMetric("required_sample_metric", "1.0.0", "1.0.0", missing); !errors.Is(err, ErrInvalidMetricResult) {
		t.Fatalf("missing required sample_count accepted: %v", err)
	}

	present := json.RawMessage(`{"metric_code":"required_sample_metric","value":0.25,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","sample_count":0,"lower_bound":null,"upper_bound":null}`)
	if err := registry.ValidateMetric("required_sample_metric", "1.0.0", "1.0.0", present); err != nil {
		t.Fatalf("explicit zero sample_count rejected: %v", err)
	}

	explicitNull := json.RawMessage(`{"metric_code":"required_sample_metric","value":null,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"unknown","metric_status":"unknown","calculation_version":"1.0.0","sample_count":null,"lower_bound":null,"upper_bound":null}`)
	if err := registry.ValidateMetric("required_sample_metric", "1.0.0", "1.0.0", explicitNull); err != nil {
		t.Fatalf("explicit null sample_count should remain distinct from missing: %v", err)
	}

	negative := json.RawMessage(`{"metric_code":"required_sample_metric","value":0.25,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","sample_count":-1,"lower_bound":null,"upper_bound":null}`)
	if err := registry.ValidateMetric("required_sample_metric", "1.0.0", "1.0.0", negative); !errors.Is(err, ErrInvalidMetricResult) {
		t.Fatalf("negative sample_count accepted: %v", err)
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

func TestRegistryValidateMetricRejectsInvalidUTF8BeforeJSONDecode(t *testing.T) {
	registry := New()
	entry := validEntry("utf8_metric", "1.0.0")
	if err := registry.Register(entry, true); err != nil {
		t.Fatal(err)
	}

	raw := append([]byte(`{"metric_code":"utf8_metric","value":0.25,"unit":"ratio","period":"`), 0xff)
	raw = append(raw, []byte(`","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)...)
	if err := registry.ValidateMetric("utf8_metric", "1.0.0", "1.0.0", json.RawMessage(raw)); !errors.Is(err, ErrInvalidMetricResult) {
		t.Fatalf("ValidateMetric() error = %v, want ErrInvalidMetricResult", err)
	}
}
