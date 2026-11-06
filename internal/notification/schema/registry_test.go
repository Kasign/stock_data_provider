package schema

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func testResourceLimits() ResourceLimits {
	return ResourceLimits{MaxEnvelopeBytes: 1 << 20, MaxPayloadBytes: 768 << 10, MaxJSONDepth: 32}
}

func newTestRegistryWithLimits(t *testing.T, limits ResourceLimits) *Registry {
	t.Helper()
	registry, err := NewRegistry(os.DirFS("../../../doc/notification/schema"), limits)
	if err != nil {
		t.Fatalf("create schema registry: %v", err)
	}
	return registry
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	return newTestRegistryWithLimits(t, testResourceLimits())
}

func TestValidateAllGoldenEvents(t *testing.T) {
	registry := newTestRegistry(t)
	goldenFS := os.DirFS("../../../doc/notification/golden")
	files, err := fs.ReadDir(goldenFS, ".")
	if err != nil {
		t.Fatal(err)
	}
	validated := 0
	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".golden.json") {
			continue
		}
		raw, err := fs.ReadFile(goldenFS, file.Name())
		if err != nil {
			t.Fatal(err)
		}
		if _, err := registry.ValidateEvent(raw); err != nil {
			t.Errorf("%s: %v", file.Name(), err)
		}
		validated++
	}
	if validated != 6 {
		t.Fatalf("expected 6 Golden fixtures, got %d", validated)
	}
}

func TestRejectsFingerprintMismatch(t *testing.T) {
	registry := newTestRegistry(t)
	raw, err := fs.ReadFile(os.DirFS("../../../doc/notification/golden"), "quant-risk-decision-first-evaluation.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	event["payload_fingerprint"] = strings.Repeat("0", 64)
	raw, _ = json.Marshal(event)
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, domain.ErrPayloadFingerprintMismatch) {
		t.Fatalf("expected fingerprint mismatch, got %v", err)
	}
}

func TestRejectsUnknownSchemaVersion(t *testing.T) {
	registry := newTestRegistry(t)
	raw, err := fs.ReadFile(os.DirFS("../../../doc/notification/golden"), "quant-risk-decision-first-evaluation.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	event["schema_version"] = "2.0.0"
	raw, _ = json.Marshal(event)
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, ErrUnknownSchema) {
		t.Fatalf("expected unknown schema error, got %v", err)
	}
}

func TestRejectsNonUTCDateTime(t *testing.T) {
	registry := newTestRegistry(t)
	raw, err := fs.ReadFile(os.DirFS("../../../doc/notification/golden"), "quant-risk-decision-first-evaluation.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	event["published_at"] = "2026-11-06T12:30:01+08:00"
	raw, _ = json.Marshal(event)
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected schema validation error, got %v", err)
	}
}

func TestRejectsDuplicateEnvelopeKeys(t *testing.T) {
	registry := newTestRegistry(t)
	raw := []byte(`{"event_id":"first","event_id":"second"}`)
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, ErrInvalidEventJSON) {
		t.Fatalf("expected strict JSON error, got %v", err)
	}
}

func TestSchemaDigestIsImmutable(t *testing.T) {
	registry := newTestRegistry(t)
	key := SchemaKey{EventType: "quant.risk_decision.changed", Version: "1.0.0"}
	digest, ok := registry.SchemaDigest(key)
	if !ok || len(digest) != 64 {
		t.Fatalf("expected registered SHA-256 digest, got %q", digest)
	}
	if err := registry.registerDigest(key, "quant", strings.Repeat("0", 64), nil); !errors.Is(err, ErrSchemaImmutable) {
		t.Fatalf("expected immutable schema error, got %v", err)
	}
}

func TestRejectsPayloadThatDoesNotMatchRegisteredSchema(t *testing.T) {
	registry := newTestRegistry(t)
	raw, err := fs.ReadFile(os.DirFS("../../../doc/notification/golden"), "quant-risk-decision-first-evaluation.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var event domain.EventEnvelope
	if err := json.Unmarshal(raw, &event); err != nil {
		t.Fatal(err)
	}
	event.Payload = []byte(`{"unexpected":"field"}`)
	event.PayloadFingerprint, err = event.CalculatePayloadFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	raw, err = json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, ErrSchemaValidation) {
		t.Fatalf("expected payload schema validation error, got %v", err)
	}
}

func TestRegistryEventTypeLookup(t *testing.T) {
	registry := newTestRegistry(t)
	for _, eventType := range []string{
		"researchflow.run.terminal",
		"quant.risk_decision.changed",
		"research.event.lifecycle_changed",
	} {
		if !registry.HasEventType(eventType) {
			t.Fatalf("expected registered event type %q", eventType)
		}
	}
	if registry.HasEventType("unknown.event") {
		t.Fatal("unknown event type must not be registered")
	}
}

func TestRegistryResourceLimitBoundaries(t *testing.T) {
	goldenFS := os.DirFS("../../../doc/notification/golden")
	raw, err := fs.ReadFile(goldenFS, "quant-risk-decision-first-evaluation.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	payload := fields["payload"]

	registry := newTestRegistryWithLimits(t, ResourceLimits{
		MaxEnvelopeBytes: len(raw), MaxPayloadBytes: len(payload), MaxJSONDepth: 32,
	})
	if _, err := registry.ValidateEvent(raw); err != nil {
		t.Fatalf("input exactly at configured byte limits should pass: %v", err)
	}

	registry = newTestRegistryWithLimits(t, ResourceLimits{
		MaxEnvelopeBytes: len(raw) - 1, MaxPayloadBytes: len(payload), MaxJSONDepth: 32,
	})
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, ErrEventResourceLimitExceeded) {
		t.Fatalf("expected envelope byte limit error, got %v", err)
	}

	registry = newTestRegistryWithLimits(t, ResourceLimits{
		MaxEnvelopeBytes: len(raw), MaxPayloadBytes: len(payload) - 1, MaxJSONDepth: 32,
	})
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, ErrEventResourceLimitExceeded) {
		t.Fatalf("expected payload byte limit error, got %v", err)
	}
}

func TestRegistryRequiresExplicitPositiveResourceLimits(t *testing.T) {
	_, err := NewRegistry(os.DirFS("../../../doc/notification/schema"), ResourceLimits{})
	if !errors.Is(err, ErrInvalidResourceLimits) {
		t.Fatalf("expected invalid resource limits, got %v", err)
	}
	if err := (ResourceLimits{MaxEnvelopeBytes: 10, MaxPayloadBytes: 11, MaxJSONDepth: 1}).Validate(); !errors.Is(err, ErrInvalidResourceLimits) {
		t.Fatalf("payload limit larger than envelope limit must fail, got %v", err)
	}
}

func TestStrictJSONRejectsDuplicateKeysAndInvalidUnicode(t *testing.T) {
	cases := [][]byte{
		[]byte(`{"outer":{"value":1,"value":2}}`),
		[]byte(`{"value":"\uD800"}`),
		[]byte(`{"value":"\uDC00"}`),
		[]byte(`{"value":"\uD800\u0041"}`),
		{'{', '"', 'v', '"', ':', '"', 0xff, '"', '}'},
	}
	for i, raw := range cases {
		if err := validateStrictJSON(raw, 32); err == nil {
			t.Errorf("case %d: expected strict JSON rejection", i)
		}
	}
	if err := validateStrictJSON([]byte(`{"value":"\uD83D\uDE00"}`), 32); err != nil {
		t.Fatalf("valid surrogate pair should pass: %v", err)
	}
}

func TestStrictJSONEnforcesContainerDepth(t *testing.T) {
	raw := []byte(`{"a":{"b":[1]}}`)
	if err := validateStrictJSON(raw, 3); err != nil {
		t.Fatalf("depth exactly at limit should pass: %v", err)
	}
	if err := validateStrictJSON(raw, 2); !errors.Is(err, ErrEventResourceLimitExceeded) {
		t.Fatalf("expected depth limit error, got %v", err)
	}
}

func TestRegistryRejectsJSONDepthLimit(t *testing.T) {
	goldenFS := os.DirFS("../../../doc/notification/golden")
	raw, err := fs.ReadFile(goldenFS, "quant-risk-decision-first-evaluation.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	registry := newTestRegistryWithLimits(t, ResourceLimits{
		MaxEnvelopeBytes: 1 << 20, MaxPayloadBytes: 768 << 10, MaxJSONDepth: 1,
	})
	if _, err := registry.ValidateEvent(raw); !errors.Is(err, ErrEventResourceLimitExceeded) {
		t.Fatalf("expected JSON depth limit error, got %v", err)
	}
}
