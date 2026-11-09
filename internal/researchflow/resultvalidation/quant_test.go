package resultvalidation

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validQuantResult() QuantResultEnvelope {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	return QuantResultEnvelope{
		SchemaVersion: "1.0.0", ResultID: "qres_1", ResultType: "indicator",
		Scope: "stock", SecurityID: "600000.SH", SystemAsOf: now,
		TradeDate: "2026-11-09", Period: "daily", AlgorithmVersion: "ma-v1",
		ParameterFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		InputSnapshotRefs: []string{"snapshot-1"}, QualityStatus: "valid",
		ResultStatus: "completed", AuthorizationRef: "auth-1", CreatedAt: now,
		Payload: json.RawMessage(`{"metric_code":"ma_5","value":"10.25"}`),
	}
}

func TestRegistryValidatesExactDiscriminatorAndStrongPayload(t *testing.T) {
	registry := NewRegistry()
	called := false
	err := registry.Register("indicator", "1.0.0", PayloadValidatorFunc(func(payload json.RawMessage) error {
		called = true
		if string(payload) != `{"metric_code":"ma_5","value":"10.25"}` {
			t.Fatalf("unexpected payload: %s", payload)
		}
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	result, err := registry.Validate(validQuantResult(), ValidationPolicy{
		ExpectedSystemAsOf: time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !called || !result.Usable || len(result.ReasonCodes) != 0 {
		t.Fatalf("unexpected validation result: %+v", result)
	}
}

func TestRegistryRejectsUnknownResultTypeAndSchemaWithoutFallback(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("indicator", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	result := validQuantResult()
	result.ResultType = "unknown-type"
	if _, err := registry.Validate(result, ValidationPolicy{}); !errors.Is(err, ErrUnsupportedResultType) {
		t.Fatalf("unknown type should be rejected, got %v", err)
	}
	result = validQuantResult()
	result.SchemaVersion = "0.9.0"
	if _, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf}); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("unknown schema must not fall back, got %v", err)
	}
}

func TestRegistryReturnsNonUsableForDegradedOrIncompleteResults(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("indicator", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	base := validQuantResult()
	for _, tc := range []struct {
		mutate func(*QuantResultEnvelope)
		policy ValidationPolicy
		reason string
	}{
		{mutate: func(v *QuantResultEnvelope) { v.QualityStatus = "degraded" }, policy: ValidationPolicy{ExpectedSystemAsOf: base.SystemAsOf}, reason: "degraded_not_allowed"},
		{mutate: func(v *QuantResultEnvelope) { v.QualityStatus = "unknown" }, policy: ValidationPolicy{ExpectedSystemAsOf: base.SystemAsOf, AllowDegraded: true}, reason: "quality_not_usable"},
		{mutate: func(v *QuantResultEnvelope) { v.ResultStatus = "insufficient_data" }, policy: ValidationPolicy{ExpectedSystemAsOf: base.SystemAsOf}, reason: "result_not_completed"},
	} {
		value := base
		tc.mutate(&value)
		result, err := registry.Validate(value, tc.policy)
		if err != nil {
			t.Fatal(err)
		}
		if result.Usable || len(result.ReasonCodes) == 0 || result.ReasonCodes[0] != tc.reason {
			t.Fatalf("unexpected non-usable result: %+v", result)
		}
	}
	degraded := base
	degraded.QualityStatus = "degraded"
	result, err := registry.Validate(degraded, ValidationPolicy{ExpectedSystemAsOf: base.SystemAsOf, AllowDegraded: true})
	if err != nil || !result.Usable {
		t.Fatalf("explicitly allowed degraded result should be usable: result=%+v err=%v", result, err)
	}
}

func TestRegistryRejectsEnvelopeAndPayloadContractViolations(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("indicator", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	base := validQuantResult()
	tests := []struct {
		name string
		mutate func(*QuantResultEnvelope)
		want error
	}{
		{"as-of mismatch", func(v *QuantResultEnvelope) { v.SystemAsOf = v.SystemAsOf.Add(time.Second) }, ErrInvalidQuantResult},
		{"missing authorization", func(v *QuantResultEnvelope) { v.AuthorizationRef = "" }, ErrInvalidQuantResult},
		{"missing input refs", func(v *QuantResultEnvelope) { v.InputSnapshotRefs = nil }, ErrInvalidQuantResult},
		{"unsorted input refs", func(v *QuantResultEnvelope) { v.InputSnapshotRefs = []string{"snapshot-2", "snapshot-1"} }, ErrInvalidQuantResult},
		{"invalid schema version", func(v *QuantResultEnvelope) { v.SchemaVersion = "latest" }, ErrUnsupportedSchema},
		{"invalid fingerprint", func(v *QuantResultEnvelope) { v.ParameterFingerprint = "not-a-hash" }, ErrInvalidQuantResult},
		{"stock without security", func(v *QuantResultEnvelope) { v.SecurityID = "" }, ErrInvalidQuantResult},
		{"invalid payload", func(v *QuantResultEnvelope) { v.Payload = json.RawMessage(`null`) }, ErrInvalidQuantPayload},
	}
	for _, tc := range tests {
		value := base
		tc.mutate(&value)
		if _, err := registry.Validate(value, ValidationPolicy{ExpectedSystemAsOf: base.SystemAsOf}); !errors.Is(err, tc.want) {
			t.Fatalf("%s: expected %v, got %v", tc.name, tc.want, err)
		}
	}
}

func TestRegistryRejectsDuplicateRegistrationAndPayloadSchemaFailure(t *testing.T) {
	registry := NewRegistry()
	validator := PayloadValidatorFunc(func(json.RawMessage) error { return errors.New("schema mismatch") })
	if err := registry.Register("indicator", "1.0.0", validator); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("indicator", "1.0.0", validator); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("duplicate registration should fail, got %v", err)
	}
	if _, err := registry.Validate(validQuantResult(), ValidationPolicy{ExpectedSystemAsOf: validQuantResult().SystemAsOf}); !errors.Is(err, ErrInvalidQuantPayload) {
		t.Fatalf("schema failure should be rejected, got %v", err)
	}
}

func TestRegistryAllowsEmptyInputsOnlyWhenTemplatePolicySaysSo(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("indicator", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	result := validQuantResult()
	result.InputSnapshotRefs = []string{}
	if _, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf}); !errors.Is(err, ErrInvalidQuantResult) {
		t.Fatalf("empty input refs should fail by default, got %v", err)
	}
	if _, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf, AllowEmptyInputs: true}); err != nil {
		t.Fatalf("template-approved pure function should accept empty inputs, got %v", err)
	}
}
