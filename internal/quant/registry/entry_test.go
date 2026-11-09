package registry

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
)

func validEntry(code, algorithmVersion string) Entry {
	return Entry{
		Code:          code,
		SchemaVersion: "1.0.0",
		ValueType:     ValueTypeNumber,
		Unit:          "ratio",
		Precision: Precision{
			MaxIntegerDigits:  8,
			MaxFractionDigits: 6,
			RoundingMode:      "half_even",
			Tolerance:         json.RawMessage("0.000001"),
		},
		Range: NumericRange{
			Minimum: json.RawMessage("-1"),
			Maximum: json.RawMessage("1"),
		},
		AlgorithmVersion: algorithmVersion,
		InputRequirements: []InputRequirement{{
			Fields:           []string{"close"},
			Interval:         "1d",
			Adjustment:       "forward_adjusted",
			PIT:              true,
			Quality:          []string{"valid"},
			RequiresSnapshot: true,
		}},
		ParametersSchema: json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false}`),
		QualityPolicy: QualityPolicy{
			MinimumCoverage:       1,
			MissingHandling:       "reject",
			QualityStates:         []string{"valid"},
			UnavailableConditions: []string{"missing_input"},
		},
		SupportsInterval: false,
	}
}

func TestEntryValidateAcceptsTypedMetadata(t *testing.T) {
	if err := validEntry("momentum_20d", "1.0.0").Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEntryValidateRejectsMismatchedNumericTypes(t *testing.T) {
	entry := validEntry("momentum_20d", "1.0.0")
	entry.ValueType = ValueTypeDecimalString
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected ErrInvalidEntry for number tolerance on decimal_string, got %v", err)
	}
}

func TestEntryValidateEnforcesDeclaredPrecision(t *testing.T) {
	entry := validEntry("momentum_20d", "1.0.0")
	entry.Precision.MaxFractionDigits = 5
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected fractional precision overflow to fail, got %v", err)
	}

	entry = validEntry("momentum_20d", "1.0.0")
	entry.Range.Maximum = json.RawMessage("100000000")
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected integer digit overflow to fail, got %v", err)
	}
}

func TestEntryValidateRejectsTrailingJSONInNumericMetadata(t *testing.T) {
	entry := validEntry("momentum_20d", "1.0.0")
	entry.Precision.Tolerance = json.RawMessage("0.000001 0.000002")
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected trailing JSON to fail, got %v", err)
	}
}

func TestEntryValidateAcceptsMathematicallyIntegralJSONNumber(t *testing.T) {
	entry := validEntry("trade_count", "1.0.0")
	entry.ValueType = ValueTypeInteger
	entry.Precision.MaxFractionDigits = 0
	entry.Precision.Tolerance = json.RawMessage("1.0")
	entry.Range = NumericRange{
		Minimum: json.RawMessage("0"),
		Maximum: json.RawMessage("1e3"),
	}
	if err := entry.Validate(); err != nil {
		t.Fatalf("expected mathematically integral JSON number to pass, got %v", err)
	}
}

func TestEntryValidateNormalizesLeadingZerosAfterExponentExpansion(t *testing.T) {
	entry := validEntry("ratio_metric", "1.0.0")
	entry.Precision.MaxIntegerDigits = 1
	entry.Precision.Tolerance = json.RawMessage("0")
	entry.Range = NumericRange{
		Minimum: json.RawMessage("0"),
		Maximum: json.RawMessage("0.001e3"),
	}
	if err := entry.Validate(); err != nil {
		t.Fatalf("expected 0.001e3 to normalize to integer 1, got %v", err)
	}
}

func TestEntryValidateCountsExponentExpandedDigits(t *testing.T) {
	entry := validEntry("ratio_metric", "1.0.0")
	entry.Precision.MaxIntegerDigits = 3
	entry.Range = NumericRange{
		Minimum: json.RawMessage("0"),
		Maximum: json.RawMessage("1e4"),
	}
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected exponent-expanded integer digits to exceed precision, got %v", err)
	}
}

func TestEntryValidatePreservesExactJSONNumberPrecision(t *testing.T) {
	entry := validEntry("ratio_metric", "1.0.0")
	entry.Precision.MaxFractionDigits = 18
	entry.Precision.Tolerance = json.RawMessage("0.000000000000000001")
	entry.Range = NumericRange{
		Minimum: json.RawMessage("0"),
		Maximum: json.RawMessage("0.1234567890123456789"),
	}
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected exact 19-digit fraction to exceed precision 18, got %v", err)
	}
}

func TestEntryValidateRejectsQuotedJSONNumber(t *testing.T) {
	entry := validEntry("momentum_20d", "1.0.0")
	entry.Precision.Tolerance = json.RawMessage("\"0.000001\"")
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected quoted JSON number to fail, got %v", err)
	}
}

func TestEntryValidateRequiresIntegerScaleZero(t *testing.T) {
	entry := validEntry("trade_count", "1.0.0")
	entry.ValueType = ValueTypeInteger
	entry.Precision.MaxFractionDigits = 1
	entry.Precision.Tolerance = json.RawMessage("0")
	entry.Range = NumericRange{
		Minimum: json.RawMessage("0"),
		Maximum: json.RawMessage("100"),
	}
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected integer scale != 0 to fail, got %v", err)
	}
}

func TestEntryValidateRequiresExplicitRange(t *testing.T) {
	entry := validEntry("momentum_20d", "1.0.0")
	entry.Range = NumericRange{}
	if err := entry.Validate(); !errors.Is(err, ErrInvalidEntry) {
		t.Fatalf("expected ErrInvalidEntry for implicit range, got %v", err)
	}
	entry.Range = NumericRange{Unbounded: true}
	if err := entry.Validate(); err != nil {
		t.Fatalf("explicit unbounded range should pass, got %v", err)
	}
}

func TestRegistryRequiresExplicitSingleActiveVersion(t *testing.T) {
	r := New()
	old := validEntry("momentum_20d", "1.0.0")
	current := validEntry("momentum_20d", "2.0.0")
	if err := r.Register(old, true); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(current, false); err != nil {
		t.Fatal(err)
	}
	if err := r.ActivateVersion("momentum_20d", "1.0.0", "2.0.0"); err != nil {
		t.Fatal(err)
	}
	active, err := r.LookupActive("momentum_20d", "1.0.0")
	if err != nil {
		t.Fatalf("LookupActive() error = %v", err)
	}
	if active.AlgorithmVersion != "2.0.0" {
		t.Fatalf("active algorithm version = %q, want 2.0.0", active.AlgorithmVersion)
	}
	if _, err := r.LookupVersion("momentum_20d", "1.0.0", "1.0.0"); err != nil {
		t.Fatalf("LookupVersion() historical version error = %v", err)
	}
	if err := r.Register(validEntry("momentum_20d", "3.0.0"), true); !errors.Is(err, ErrActiveVersionExists) {
		t.Fatalf("expected ErrActiveVersionExists, got %v", err)
	}
}

func TestRegistryFreezeRequiresActiveVersion(t *testing.T) {
	r := New()
	if err := r.Register(validEntry("sma", "1.0.0"), false); err != nil {
		t.Fatal(err)
	}
	if err := r.Freeze(); !errors.Is(err, ErrNoActiveVersion) {
		t.Fatalf("expected ErrNoActiveVersion, got %v", err)
	}
	if err := r.ActivateVersion("sma", "1.0.0", "1.0.0"); err != nil {
		t.Fatal(err)
	}
	if err := r.Freeze(); err != nil {
		t.Fatalf("Freeze() error = %v", err)
	}
}

func TestRegistryRejectsDuplicateVersionAndUnknownLookup(t *testing.T) {
	r := New()
	entry := validEntry("sma", "1.0.0")
	if err := r.Register(entry, true); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(entry, false); !errors.Is(err, ErrDuplicateVersion) {
		t.Fatalf("expected ErrDuplicateVersion, got %v", err)
	}
	if _, err := r.LookupVersion("unknown", "1.0.0", "1.0.0"); !errors.Is(err, ErrUnknownEntry) {
		t.Fatalf("expected ErrUnknownEntry, got %v", err)
	}
	if _, err := r.LookupActive("unknown", "1.0.0"); !errors.Is(err, ErrNoActiveVersion) {
		t.Fatalf("expected ErrNoActiveVersion, got %v", err)
	}
}

func TestRegistryFreezeAndStableSnapshot(t *testing.T) {
	r := New()
	for _, entry := range []Entry{
		validEntry("z_metric", "1.0.0"),
		validEntry("a_metric", "1.0.0"),
	} {
		if err := r.Register(entry, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Freeze(); err != nil {
		t.Fatalf("Freeze() error = %v", err)
	}
	if !r.IsFrozen() {
		t.Fatal("registry should be frozen")
	}
	if err := r.Register(validEntry("later", "1.0.0"), true); !errors.Is(err, ErrRegistryFrozen) {
		t.Fatalf("expected ErrRegistryFrozen, got %v", err)
	}
	entries := r.Entries()
	if len(entries) != 2 || entries[0].Code != "a_metric" || entries[1].Code != "z_metric" {
		t.Fatalf("Entries() is not stable sorted: %#v", entries)
	}
	entries[0].InputRequirements[0].Fields[0] = "mutated"
	entries[0].ParametersSchema[0] = 'x'
	again, err := r.LookupActive("a_metric", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if again.InputRequirements[0].Fields[0] != "close" || string(again.ParametersSchema) == string(entries[0].ParametersSchema) {
		t.Fatal("registry exposed mutable internal state")
	}
}

func TestRegistryValidateParametersUsesExplicitAlgorithmVersion(t *testing.T) {
	registry := New()
	entry := validEntry("sma", "1.0.0")
	entry.ParametersSchema = json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"additionalProperties":false,
		"required":["window"],
		"properties":{"window":{"type":"integer","minimum":1}}
	}`)
	if err := registry.Register(entry, true); err != nil {
		t.Fatal(err)
	}
	nextVersion := validEntry("sma", "2.0.0")
	nextVersion.ParametersSchema = json.RawMessage(`{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"additionalProperties":false,
		"required":["window"],
		"properties":{"window":{"type":"integer","minimum":2}}
	}`)
	if err := registry.Register(nextVersion, false); err != nil {
		t.Fatal(err)
	}
	if err := registry.ActivateVersion("sma", "1.0.0", "2.0.0"); err != nil {
		t.Fatal(err)
	}

	if err := registry.ValidateParameters("sma", "1.0.0", "1.0.0", json.RawMessage(`{"window":1}`)); err != nil {
		t.Fatalf("historical algorithm version changed after activation: %v", err)
	}
	if err := registry.ValidateParameters("sma", "1.0.0", "2.0.0", json.RawMessage(`{"window":1}`)); !errors.Is(err, ErrInvalidParameters) {
		t.Fatalf("new active version must apply its own schema, got %v", err)
	}
	if err := registry.ValidateParameters("sma", "1.0.0", "1.0.0", json.RawMessage(`{"window":5}`)); err != nil {
		t.Fatalf("valid parameters rejected: %v", err)
	}
	if err := registry.ValidateParameters("sma", "1.0.0", "1.0.0", json.RawMessage(`{"window":0}`)); !errors.Is(err, ErrInvalidParameters) {
		t.Fatalf("expected invalid parameters error, got %v", err)
	}
	if err := registry.ValidateParameters("sma", "1.0.0", "9.9.9", json.RawMessage(`{"window":5}`)); !errors.Is(err, ErrUnknownEntry) {
		t.Fatalf("expected unknown explicit algorithm version, got %v", err)
	}
	if err := registry.ValidateParameters("sma", "1.0.0", "1.0.0", json.RawMessage(`{"window":5} trailing`)); !errors.Is(err, ErrInvalidParameters) {
		t.Fatalf("expected malformed JSON rejection, got %v", err)
	}
}

func TestRegistryDeepCopiesEntryOnRegisterAndLookup(t *testing.T) {
	registry := New()
	source := validEntry("copy_metric", "1.0.0")
	if err := registry.Register(source, true); err != nil {
		t.Fatal(err)
	}

	// 注册后修改调用方持有的所有嵌套可变字段，不得影响 Registry。
	source.Precision.Tolerance[0] = '9'
	source.Range.Minimum[0] = '9'
	source.Range.Maximum[0] = '9'
	source.InputRequirements[0].Fields[0] = "mutated_field"
	source.InputRequirements[0].Quality[0] = "mutated_quality"
	source.ParametersSchema[0] = 'x'
	source.QualityPolicy.QualityStates[0] = "mutated_state"
	source.QualityPolicy.UnavailableConditions[0] = "mutated_condition"

	got, err := registry.LookupVersion("copy_metric", "1.0.0", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	assertOriginalEntryNestedValues(t, got)

	// Lookup 返回值同样必须与 Registry 内部状态隔离。
	got.Precision.Tolerance[0] = '8'
	got.Range.Minimum[0] = '8'
	got.Range.Maximum[0] = '8'
	got.InputRequirements[0].Fields[0] = "lookup_field"
	got.InputRequirements[0].Quality[0] = "lookup_quality"
	got.ParametersSchema[0] = 'y'
	got.QualityPolicy.QualityStates[0] = "lookup_state"
	got.QualityPolicy.UnavailableConditions[0] = "lookup_condition"

	again, err := registry.LookupActive("copy_metric", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	assertOriginalEntryNestedValues(t, again)
}

func assertOriginalEntryNestedValues(t *testing.T, entry Entry) {
	t.Helper()
	if string(entry.Precision.Tolerance) != "0.000001" {
		t.Errorf("Precision.Tolerance = %s, want 0.000001", entry.Precision.Tolerance)
	}
	if string(entry.Range.Minimum) != "-1" || string(entry.Range.Maximum) != "1" {
		t.Errorf("Range = (%s, %s), want (-1, 1)", entry.Range.Minimum, entry.Range.Maximum)
	}
	if entry.InputRequirements[0].Fields[0] != "close" {
		t.Errorf("InputRequirements.Fields = %v, want [close]", entry.InputRequirements[0].Fields)
	}
	if entry.InputRequirements[0].Quality[0] != "valid" {
		t.Errorf("InputRequirements.Quality = %v, want [valid]", entry.InputRequirements[0].Quality)
	}
	if string(entry.ParametersSchema) != `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false}` {
		t.Errorf("ParametersSchema was mutated: %s", entry.ParametersSchema)
	}
	if entry.QualityPolicy.QualityStates[0] != "valid" {
		t.Errorf("QualityPolicy.QualityStates = %v, want [valid]", entry.QualityPolicy.QualityStates)
	}
	if entry.QualityPolicy.UnavailableConditions[0] != "missing_input" {
		t.Errorf("QualityPolicy.UnavailableConditions = %v, want [missing_input]", entry.QualityPolicy.UnavailableConditions)
	}
}

func TestRegistryValidateMetricAppliesRegisteredSemantics(t *testing.T) {
	registry := New()
	entry := validEntry("sample_metric", "1.0.0")
	entry.Precision.MaxFractionDigits = 4
	entry.Range = NumericRange{Minimum: json.RawMessage("-0.5"), Maximum: json.RawMessage("0.5")}
	entry.QualityPolicy.QualityStates = []string{"valid", "degraded"}
	if err := registry.Register(entry, true); err != nil {
		t.Fatal(err)
	}

	valid := json.RawMessage(`{"metric_code":"sample_metric","value":0.25,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`)
	if err := registry.ValidateMetric("sample_metric", "1.0.0", "1.0.0", valid); err != nil {
		t.Fatalf("valid metric rejected: %v", err)
	}
	badCases := []json.RawMessage{
		json.RawMessage(`{"metric_code":"sample_metric","value":0.25,"unit":"wrong","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`),
		json.RawMessage(`{"metric_code":"sample_metric","value":0.123456,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`),
		json.RawMessage(`{"metric_code":"sample_metric","value":0.75,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`),
		json.RawMessage(`{"metric_code":"sample_metric","value":0.25,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"unknown","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`),
		json.RawMessage(`{"metric_code":"sample_metric","value":0.25,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"2.0.0","lower_bound":null,"upper_bound":null}`),
		json.RawMessage(`{"metric_code":"sample_metric","value":0.25,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":0.3,"upper_bound":0.4}`),
	}
	for i, raw := range badCases {
		if err := registry.ValidateMetric("sample_metric", "1.0.0", "1.0.0", raw); err == nil {
			t.Errorf("invalid metric case %d accepted", i)
		}
	}
	if err := registry.ValidateMetric("unknown", "1.0.0", "1.0.0", valid); !errors.Is(err, ErrUnknownEntry) {
		t.Fatalf("unknown metric code should fail lookup, got %v", err)
	}
}


func TestRegistryFreezePreventsAlgorithmActivation(t *testing.T) {
	registry := New()
	oldVersion := validEntry("momentum_20d", "1.0.0")
	newVersion := validEntry("momentum_20d", "2.0.0")
	if err := registry.Register(oldVersion, true); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(newVersion, false); err != nil {
		t.Fatal(err)
	}
	if err := registry.Freeze(); err != nil {
		t.Fatalf("Freeze() error = %v", err)
	}
	if err := registry.ActivateVersion("momentum_20d", "1.0.0", "2.0.0"); !errors.Is(err, ErrRegistryFrozen) {
		t.Fatalf("activation after Freeze must fail with ErrRegistryFrozen, got %v", err)
	}
	active, err := registry.LookupActive("momentum_20d", "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if active.AlgorithmVersion != "1.0.0" {
		t.Fatalf("frozen active version changed to %q", active.AlgorithmVersion)
	}
	// Freeze is idempotent and must not reopen a frozen Registry.
	if err := registry.Freeze(); err != nil {
		t.Fatalf("second Freeze() error = %v", err)
	}
	if err := registry.ActivateVersion("momentum_20d", "1.0.0", "2.0.0"); !errors.Is(err, ErrRegistryFrozen) {
		t.Fatalf("activation after repeated Freeze must remain blocked, got %v", err)
	}
}

func TestRegistryConcurrentVersionActivationAndLookup(t *testing.T) {
	registry := New()
	for _, entry := range []Entry{
		validEntry("momentum_20d", "1.0.0"),
		validEntry("momentum_20d", "2.0.0"),
	} {
		if err := registry.Register(entry, entry.AlgorithmVersion == "1.0.0"); err != nil {
			t.Fatal(err)
		}
	}

	const iterations = 100
	errs := make(chan error, iterations*2)
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		for i := 0; i < iterations; i++ {
			version := "1.0.0"
			if i%2 == 1 {
				version = "2.0.0"
			}
			if err := registry.ActivateVersion("momentum_20d", "1.0.0", version); err != nil {
				errs <- err
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		for i := 0; i < iterations; i++ {
			entry, err := registry.LookupActive("momentum_20d", "1.0.0")
			if err != nil {
				errs <- err
				return
			}
			if entry.AlgorithmVersion != "1.0.0" && entry.AlgorithmVersion != "2.0.0" {
				errs <- fmt.Errorf("lookup returned unregistered algorithm version %q", entry.AlgorithmVersion)
				return
			}
		}
	}()
	workers.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
