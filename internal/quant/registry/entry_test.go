package registry

import (
	"encoding/json"
	"errors"
	"testing"
)

func validEntry(code, algorithmVersion string) Entry {
	return Entry{
		Code:             code,
		SchemaVersion:    "1.0.0",
		ValueType:        ValueTypeNumber,
		Unit:             "ratio",
		Precision: Precision{
			MaxIntegerDigits:   8,
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
		ParametersSchema: json.RawMessage(`{"type":"object","additionalProperties":false}`),
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
