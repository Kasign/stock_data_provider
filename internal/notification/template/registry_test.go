package template

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
	"example.com/stock-analysis-flow/internal/notification/schema"
)

func testTemplateRegistry(t *testing.T, limits Limits) *Registry {
	t.Helper()
	registry, err := NewRegistry(limits)
	if err != nil {
		t.Fatalf("create template registry: %v", err)
	}
	return registry
}

func testTemplateEvent(payload string) domain.EventEnvelope {
	event := domain.EventEnvelope{
		EventID: "event-1", EventType: "quant.risk_decision.changed",
		SchemaVersion: "1.0.0", SourceModule: "quant",
		OccurredAt: "2026-11-06T09:00:00Z", PublishedAt: "2026-11-06T09:00:01Z",
		SubjectRef: "risk-subject-1", IdempotencyKey: "publish-1",
		CorrelationID: "correlation-1", Payload: json.RawMessage(payload),
	}
	event.PayloadFingerprint, _ = event.CalculatePayloadFingerprint()
	return event
}

func baseDefinition(format Format) Definition {
	return Definition{
		TemplateID: "risk-alert", Version: 1,
		EventType: "quant.risk_decision.changed", SchemaVersion: "1.0.0",
		Format: format, BodyTemplate: "{{.message}}",
		VariableBindings: map[string]string{"message": "/message"},
	}
}

func TestRegistryRendersHTMLWithAllowlistedScalarsAndEscaping(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 4096, MaxOutputBytes: 4096, MaxPayloadDepth: 8,
	})
	definition := baseDefinition(FormatHTML)
	definition.SubjectTemplate = "Risk: {{.status}}"
	definition.BodyTemplate = "<p>{{.message}}</p><span>{{.count}}</span>"
	definition.VariableBindings = map[string]string{
		"message": "/message", "count": "/count", "status": "/status",
	}
	if _, err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}

	event := testTemplateEvent(`{"message":"<script>alert(1)</script>","count":3,"status":"reject"}`)
	rendered, err := registry.Render(context.Background(), event, definition.TemplateID, definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Subject != "Risk: reject" {
		t.Fatalf("unexpected subject %q", rendered.Subject)
	}
	if strings.Contains(rendered.Body, "<script>") || !strings.Contains(rendered.Body, "&lt;script&gt;") {
		t.Fatalf("HTML body must contextually escape untrusted event values: %q", rendered.Body)
	}
	if rendered.TemplateDigest == "" || rendered.EventPayloadFingerprint != event.PayloadFingerprint {
		t.Fatalf("render result must retain immutable template and event fingerprints: %+v", rendered)
	}
}

func TestRegistryRendersPlainTextWithoutHTMLEscaping(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	if _, err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	event := testTemplateEvent(`{"message":"<b>plain text</b>"}`)
	rendered, err := registry.Render(context.Background(), event, definition.TemplateID, definition.Version)
	if err != nil {
		t.Fatal(err)
	}
	if rendered.Body != "<b>plain text</b>" {
		t.Fatalf("plain text format should preserve text content, got %q", rendered.Body)
	}
}

func TestRegistryRejectsUnallowlistedAndUnusedVariables(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	definition.BodyTemplate = "{{.secret}}"
	if _, err := registry.Register(definition); !errors.Is(err, ErrTemplateVariableNotAllowed) {
		t.Fatalf("expected unallowlisted variable error, got %v", err)
	}

	definition.BodyTemplate = "{{.message}}"
	definition.VariableBindings["unused"] = "/other"
	if _, err := registry.Register(definition); !errors.Is(err, ErrUnusedTemplateBinding) {
		t.Fatalf("expected unused binding error, got %v", err)
	}
}

func TestRegistryRejectsUnsupportedTemplateControlFlow(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	definition.BodyTemplate = "{{if .message}}visible{{end}}"
	if _, err := registry.Register(definition); !errors.Is(err, ErrUnsupportedTemplateSyntax) {
		t.Fatalf("expected control flow to be rejected, got %v", err)
	}
}

func TestRegistryRejectsMissingAndNonScalarBindings(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	if _, err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Render(context.Background(), testTemplateEvent(`{"message":"hello"}`), definition.TemplateID, definition.Version); err != nil {
		t.Fatal(err)
	}

	if _, err := registry.Render(context.Background(), testTemplateEvent(`{"other":"hello"}`), definition.TemplateID, definition.Version); !errors.Is(err, ErrTemplateBindingMissing) {
		t.Fatalf("expected missing JSON Pointer binding error, got %v", err)
	}

	definition.TemplateID = "array-template"
	definition.VariableBindings = map[string]string{"message": "/message"}
	if _, err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Render(context.Background(), testTemplateEvent(`{"message":{"nested":"not-scalar"}}`), definition.TemplateID, definition.Version); !errors.Is(err, ErrTemplateBindingNotScalar) {
		t.Fatalf("expected non-scalar binding error, got %v", err)
	}
}

func TestRegistryEnforcesTemplateOutputAndPayloadDepthLimits(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 4, MaxPayloadDepth: 2,
	})
	definition := baseDefinition(FormatText)
	if _, err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Render(context.Background(), testTemplateEvent(`{"message":"1234"}`), definition.TemplateID, definition.Version); err != nil {
		t.Fatalf("output exactly at byte limit should pass: %v", err)
	}
	if _, err := registry.Render(context.Background(), testTemplateEvent(`{"message":"12345"}`), definition.TemplateID, definition.Version); !errors.Is(err, ErrTemplateResourceLimit) {
		t.Fatalf("expected output byte limit error, got %v", err)
	}

	depthLimited := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 4, MaxPayloadDepth: 1,
	})
	if _, err := depthLimited.Register(definition); err != nil {
		t.Fatal(err)
	}
	deep := testTemplateEvent(`{"nested":{"message":"x"}}`)
	if _, err := depthLimited.Render(context.Background(), deep, definition.TemplateID, definition.Version); !errors.Is(err, schema.ErrEventResourceLimitExceeded) {
		t.Fatalf("expected payload depth limit error, got %v", err)
	}
}

func TestRegistryRequiresExplicitLimitsAndEnforcesTemplateImmutability(t *testing.T) {
	if _, err := NewRegistry(Limits{}); !errors.Is(err, ErrInvalidTemplateLimits) {
		t.Fatalf("expected explicit positive limits, got %v", err)
	}
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	digest, err := registry.Register(definition)
	if err != nil {
		t.Fatal(err)
	}
	if repeated, err := registry.Register(definition); err != nil || repeated != digest {
		t.Fatalf("identical immutable registration should be idempotent, digest=%q err=%v", repeated, err)
	}
	changed := definition
	changed.BodyTemplate = "changed {{.message}}"
	if _, err := registry.Register(changed); !errors.Is(err, ErrTemplateImmutable) {
		t.Fatalf("same template version with changed content must conflict, got %v", err)
	}
	changed.Version = 2
	if _, err := registry.Register(changed); err != nil {
		t.Fatalf("new template version should be accepted: %v", err)
	}
}

func TestRegistryRejectsEventSchemaAndFingerprintMismatch(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	if _, err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	event := testTemplateEvent(`{"message":"hello"}`)
	event.SchemaVersion = "2.0.0"
	if _, err := registry.Render(context.Background(), event, definition.TemplateID, definition.Version); !errors.Is(err, ErrTemplateEventMismatch) {
		t.Fatalf("expected exact event schema binding, got %v", err)
	}
	event = testTemplateEvent(`{"message":"hello"}`)
	event.Payload = json.RawMessage(`{"message":"changed"}`)
	if _, err := registry.Render(context.Background(), event, definition.TemplateID, definition.Version); !errors.Is(err, domain.ErrPayloadFingerprintMismatch) {
		t.Fatalf("expected payload fingerprint mismatch, got %v", err)
	}
}

func TestRegistryRejectsSubjectHeaderInjection(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 1024, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	definition.SubjectTemplate = "{{.subject}}"
	definition.VariableBindings["subject"] = "/subject"
	if _, err := registry.Register(definition); err != nil {
		t.Fatal(err)
	}
	event := testTemplateEvent(`{"message":"hello","subject":"hello\r\nBcc:attacker@example.com"}`)
	if _, err := registry.Render(context.Background(), event, definition.TemplateID, definition.Version); !errors.Is(err, ErrInvalidTemplateSubject) {
		t.Fatalf("expected CRLF subject injection to be rejected, got %v", err)
	}
}

func TestRegistryEnforcesTemplateSourceLimit(t *testing.T) {
	registry := testTemplateRegistry(t, Limits{
		MaxTemplateBytes: 16, MaxOutputBytes: 1024, MaxPayloadDepth: 4,
	})
	definition := baseDefinition(FormatText)
	if _, err := registry.Register(definition); !errors.Is(err, ErrTemplateResourceLimit) {
		t.Fatalf("expected template source byte limit error, got %v", err)
	}
}
