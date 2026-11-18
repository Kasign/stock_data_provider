package resultvalidation

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	quantresult "example.com/stock-analysis-flow/internal/quant/result"
)

const validTrendPayload = `{"trend_code":"price_trend","direction":"unknown","phase":"emerging","strength":null,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T08:00:00Z","algorithm_version":"trend-algorithm.v1"}`

func validQuantResult() QuantResultEnvelope {
	now := time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	return QuantResultEnvelope{
		SchemaVersion: quantresult.EnvelopeSchemaVersion, PayloadSchemaVersion: "1.0.0",
		ResultID: "quant_result:sha256:" + strings.Repeat("a", 64), ResultType: "trend",
		Scope: "stock", SecurityID: "600000.SH", SystemAsOf: now,
		TradeDate: "2026-11-09", Period: "20d", AlgorithmVersion: "trend-algorithm.v1",
		ParameterFingerprint: "sha256:parameter-fingerprint",
		InputSnapshotRefs:    []string{"snapshot-1"}, QualityStatus: "valid",
		ResultStatus: "completed", AuthorizationRef: "auth-1",
		Payload: json.RawMessage(validTrendPayload),
	}
}

func TestRegistryValidatesExactDiscriminatorAndStrongPayload(t *testing.T) {
	registry := NewRegistry()
	called := false
	err := registry.Register("trend", "1.0.0", PayloadValidatorFunc(func(payload json.RawMessage) error {
		called = true
		if string(payload) != validTrendPayload {
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

func TestRegistryUsesPayloadSchemaVersionNotEnvelopeSchemaVersion(t *testing.T) {
	registry := NewRegistry()
	called := false
	if err := registry.Register("trend", "2.0.0", PayloadValidatorFunc(func(json.RawMessage) error {
		called = true
		return nil
	})); err != nil {
		t.Fatal(err)
	}
	result := validQuantResult()
	result.PayloadSchemaVersion = "2.0.0"
	validated, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf})
	if err != nil {
		t.Fatal(err)
	}
	if !called || !validated.Usable {
		t.Fatalf("payload schema version was not used as registry key: called=%v result=%+v", called, validated)
	}
}

func TestRegistryRejectsUnknownResultTypeAndSchemaWithoutFallback(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("trend", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	result := validQuantResult()
	result.ResultType = "indicator"
	if _, err := registry.Validate(result, ValidationPolicy{}); !errors.Is(err, ErrUnsupportedResultType) {
		t.Fatalf("result type outside Quant Envelope union should be rejected, got %v", err)
	}
	result = validQuantResult()
	result.SchemaVersion = "0.9.0"
	if _, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf}); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("unknown envelope schema must not fall back, got %v", err)
	}
	result = validQuantResult()
	result.PayloadSchemaVersion = "2.0.0"
	if _, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf}); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("unknown payload schema must not fall back, got %v", err)
	}
}

func TestNewQuantPayloadRegistryUsesQuantOwnedTypedPayloadValidators(t *testing.T) {
	registry, err := NewQuantPayloadRegistry()
	if err != nil {
		t.Fatal(err)
	}
	result := validQuantResult()
	validated, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf})
	if err != nil || !validated.Usable {
		t.Fatalf("Quant-owned trend payload should validate: result=%+v err=%v", validated, err)
	}
	result.Payload = json.RawMessage(`{"trend_code":"bad","unknown_field":true}`)
	if _, err := registry.Validate(result, ValidationPolicy{ExpectedSystemAsOf: result.SystemAsOf}); !errors.Is(err, ErrInvalidQuantPayload) {
		t.Fatalf("Quant strict decoder must reject unknown payload fields, got %v", err)
	}
}

func TestFromQuantEnvelopePreservesSeparateSchemaVersions(t *testing.T) {
	payload, err := quantresult.DecodePayload("trend", "1.0.0", json.RawMessage(validTrendPayload))
	if err != nil {
		t.Fatal(err)
	}
	base := validQuantResult()
	envelope := quantresult.Envelope{
		SchemaVersion: quantresult.EnvelopeSchemaVersion,
		ResultID:      base.ResultID, ResultType: "trend", Scope: "stock",
		SecurityID: "600000.SH", SystemAsOf: "2026-11-09T08:00:00Z",
		Period: "20d", AlgorithmVersion: "trend-algorithm.v1",
		ParameterFingerprint: "sha256:parameter-fingerprint",
		InputSnapshotRefs:    []string{"snapshot-1"}, QualityStatus: "valid",
		ResultStatus: "completed", AuthorizationRef: "auth-1",
		PayloadSchemaVersion: "1.0.0", Payload: payload,
	}
	got, err := FromQuantEnvelope(envelope)
	if err != nil {
		t.Fatal(err)
	}
	if got.SchemaVersion != quantresult.EnvelopeSchemaVersion || got.PayloadSchemaVersion != "1.0.0" {
		t.Fatalf("envelope and payload schema versions were not preserved separately: %+v", got)
	}
	if !got.SystemAsOf.Equal(base.SystemAsOf) {
		t.Fatalf("system_as_of changed during Quant envelope adaptation: %v", got.SystemAsOf)
	}
}

func TestRegistryReturnsNonUsableForDegradedOrIncompleteResults(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("trend", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
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
	if err := registry.Register("trend", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
		t.Fatal(err)
	}
	base := validQuantResult()
	tests := []struct {
		name   string
		mutate func(*QuantResultEnvelope)
		want   error
	}{
		{"as-of mismatch", func(v *QuantResultEnvelope) { v.SystemAsOf = v.SystemAsOf.Add(time.Second) }, ErrInvalidQuantResult},
		{"zero-offset non-UTC location", func(v *QuantResultEnvelope) { v.SystemAsOf = time.Date(2026, 11, 9, 8, 0, 0, 0, time.FixedZone("UTC", 0)) }, ErrInvalidQuantResult},
		{"missing authorization", func(v *QuantResultEnvelope) { v.AuthorizationRef = "" }, ErrInvalidQuantResult},
		{"missing input refs", func(v *QuantResultEnvelope) { v.InputSnapshotRefs = nil }, ErrInvalidQuantResult},
		{"unsorted input refs", func(v *QuantResultEnvelope) { v.InputSnapshotRefs = []string{"snapshot-2", "snapshot-1"} }, ErrInvalidQuantResult},
		{"invalid envelope schema", func(v *QuantResultEnvelope) { v.SchemaVersion = "latest" }, ErrUnsupportedSchema},
		{"invalid payload schema", func(v *QuantResultEnvelope) { v.PayloadSchemaVersion = "latest" }, ErrUnsupportedSchema},
		{"invalid result id", func(v *QuantResultEnvelope) { v.ResultID = "qres-1" }, ErrInvalidQuantResult},
		{"padded fingerprint", func(v *QuantResultEnvelope) { v.ParameterFingerprint = " padded " }, ErrInvalidQuantResult},
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
	if err := registry.Register("trend", "1.0.0", validator); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register("trend", "1.0.0", validator); !errors.Is(err, ErrUnsupportedSchema) {
		t.Fatalf("duplicate registration should fail, got %v", err)
	}
	if _, err := registry.Validate(validQuantResult(), ValidationPolicy{ExpectedSystemAsOf: validQuantResult().SystemAsOf}); !errors.Is(err, ErrInvalidQuantPayload) {
		t.Fatalf("schema failure should be rejected, got %v", err)
	}
}

func TestRegistryAllowsEmptyInputsOnlyWhenTemplatePolicySaysSo(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("trend", "1.0.0", PayloadValidatorFunc(func(json.RawMessage) error { return nil })); err != nil {
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
