package stageoutput

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func validIdentity() ExecutionIdentity {
	return ExecutionIdentity{
		RunRef: "run:1", StageRef: "stage:1", AttemptRef: "attempt:1",
		StageCode: "news.collect", TemplateVersion: "1.0.0", RegistryVersion: "1.0.0",
	}
}

func validRegistry(t *testing.T) *Registry {
	t.Helper()
	registry, err := NewRegistry("1.0.0", []Entry{{
		StageCode: "news.collect", OutputType: "news.collection", SchemaVersion: "1.0.0",
		PayloadSchemaRef:        "schema://news.collection/1.0.0",
		QualitySummarySchemaRef: "schema://news.collection.quality/1.0.0",
		PayloadValidator: ValidatorFunc(func(value json.RawMessage) error {
			var payload struct {\n\t\t\t\tCount int `json:"count"`\n\t\t\t}
			return json.Unmarshal(value, &payload)
		}),
		QualitySummaryValidator: ValidatorFunc(func(value json.RawMessage) error {
			var summary struct {\n\t\t\t\tStatus string `json:"status"`\n\t\t\t}
			if err := json.Unmarshal(value, &summary); err != nil {\n\t\t\t\treturn err\n\t\t\t}
			if summary.Status == "" {\n\t\t\t\treturn errors.New("status is required")\n\t\t\t}
			return nil
		}),
	}})
	if err != nil {\n\t\tt.Fatal(err)\n\t}
	return registry
}

func validEnvelope(t *testing.T) (*Registry, Envelope, ValidationContext) {
	t.Helper()
	registry := validRegistry(t)
	identity := validIdentity()
	envelope, err := BuildEnvelope(identity, ProviderResult{
		OutputType: "news.collection", SchemaVersion: "1.0.0",
		Payload:        json.RawMessage(`{"count":2}`),
		QualitySummary: json.RawMessage(`{"status":"complete"}`),
		DiagnosticRefs: []string{"diag:1"},
	}, time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC))
	if err != nil {\n\t\tt.Fatal(err)\n\t}
	return registry, envelope, ValidationContext{Identity: identity}
}

func TestBuildEnvelopeInjectsOrchestratorIdentityAndCopiesProviderData(t *testing.T) {
	identity := validIdentity()
	payload := json.RawMessage(`{"count":2}`)
	refs := []string{"diag:1"}
	envelope, err := BuildEnvelope(identity, ProviderResult{
		OutputType: "news.collection", SchemaVersion: "1.0.0",
		Payload: payload, QualitySummary: json.RawMessage(`{"status":"complete"}`), DiagnosticRefs: refs,
	}, time.Date(2026, 11, 16, 8, 0, 0, 0, time.UTC))
	if err != nil {\n\t\tt.Fatal(err)\n\t}
	payload[2] = 'X'
	refs[0] = "tampered"
	if envelope.RunRef != identity.RunRef || string(envelope.Payload) != `{"count":2}` || envelope.DiagnosticRefs[0] != "diag:1" {
		t.Fatalf("provider mutated envelope-owned data: %+v", envelope)
	}
}

func TestRegistryValidatesExactEntryAndSchemas(t *testing.T) {
	registry, envelope, context := validEnvelope(t)
	if err := registry.Validate(envelope, context); err != nil {\n\t\tt.Fatal(err)\n\t}
	envelope.OutputType = "unknown.output"
	if !errors.Is(registry.Validate(envelope, context), ErrUnknownOutput) {
		t.Fatal("unknown output discriminator must fail closed")
	}
}

func TestRegistryRejectsIdentityAndVersionTampering(t *testing.T) {
	registry, envelope, context := validEnvelope(t)
	envelope.StageRef = "stage:other"
	if !errors.Is(registry.Validate(envelope, context), ErrInvalidEnvelope) {
		t.Fatal("provider must not override orchestrator stage identity")
	}
	_, envelope, context = validEnvelope(t)
	context.Identity.RegistryVersion = "2.0.0"
	if !errors.Is(registry.Validate(envelope, context), ErrInvalidEnvelope) {
		t.Fatal("registry version mismatch must fail closed")
	}
}

func TestRegistryRejectsDuplicateJSONKeysAndInvalidQualitySummary(t *testing.T) {
	registry, envelope, context := validEnvelope(t)
	envelope.Payload = json.RawMessage(`{"count":1,"count":2}`)
	if !errors.Is(registry.Validate(envelope, context), ErrInvalidEnvelope) {
		t.Fatal("duplicate JSON keys must be rejected")
	}
	_, envelope, context = validEnvelope(t)
	envelope.QualitySummary = json.RawMessage(`{"status":""}`)
	if !errors.Is(registry.Validate(envelope, context), ErrInvalidEnvelope) {
		t.Fatal("invalid typed quality summary must be rejected")
	}
}

func TestRegistryIsDeterministicAndRejectsDuplicateEntries(t *testing.T) {
	one := Entry{
		StageCode: "news.collect", OutputType: "news.collection", SchemaVersion: "1.0.0",
		PayloadSchemaRef:        "schema://payload/1", QualitySummarySchemaRef: "schema://quality/1", 
		PayloadValidator:        ValidatorFunc(func(json.RawMessage) error { return nil }),
		QualitySummaryValidator: ValidatorFunc(func(json.RawMessage) error { return nil }),
	}
	two := one
	two.StageCode = "news.publish"
	a, err := NewRegistry("1.0.0", []Entry{one, two})
	if err != nil {\n\t\tt.Fatal(err)\n\t}
	b, err := NewRegistry("1.0.0", []Entry{two, one})
	if err != nil {\n\t\tt.Fatal(err)\n\t}
	if a.Fingerprint() != b.Fingerprint() {\n\t\tt.Fatal("registry fingerprint must not depend on registration order")\n\t}
	if _, err := NewRegistry("1.0.0", []Entry{one, one}); !errors.Is(err, ErrInvalidRegistry) {
		t.Fatal("duplicate registry entry must be rejected")
	}
}

func TestRegistryRequiresStrongValidatorsAndUTCCompletionTime(t *testing.T) {
	identity := validIdentity()
	if _, err := BuildEnvelope(identity, ProviderResult{
		OutputType: "news.collection", SchemaVersion: "1.0.0",
		Payload: json.RawMessage(`{}`), QualitySummary: json.RawMessage(`{}`),
	}, time.Date(2026, 11, 16, 16, 0, 0, 0, time.FixedZone("offset", 8*60*60))); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatal("non-UTC location must be rejected")
	}
	if _, err := NewRegistry("1.0.0", []Entry{{
		StageCode: "news.collect", OutputType: "news.collection", SchemaVersion: "1.0.0",
		PayloadSchemaRef:        "schema://payload/1", QualitySummarySchemaRef: "schema://quality/1", 
	}}); !errors.Is(err, ErrInvalidRegistry) {
		t.Fatal("missing validators must be rejected")
	}
}
