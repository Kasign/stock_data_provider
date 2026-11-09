package storage

import (
	"bytes"
	"encoding/json"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gowebpki/jcs"
	"github.com/santhosh-tekuri/jsonschema/v6"

	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

type lifecycleFingerprintInput struct {
	SourceModule string          `json:"source_module"`
	EventType string             `json:"event_type"`
	SchemaVersion string         `json:"schema_version"`
	SubjectRef string            `json:"subject_ref"`
	OccurredAt string            `json:"occurred_at"`
	Payload json.RawMessage      `json:"payload"`
}

func compileResearchNotificationContract(t *testing.T) *jsonschema.Schema {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "notification", "research-event-lifecycle-envelope-1.0.0.schema.json"))
	if err != nil { t.Fatalf("read Research Notification contract schema: %v", err) }
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil { t.Fatalf("parse Research Notification contract schema: %v", err) }
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("research-notification-envelope.schema.json", document); err != nil {
		t.Fatalf("register Research Notification contract schema: %v", err)
	}
	compiled, err := compiler.Compile("research-notification-envelope.schema.json")
	if err != nil { t.Fatalf("compile Research Notification contract schema: %v", err) }
	return compiled
}

func validateResearchNotificationEnvelope(t *testing.T, schema *jsonschema.Schema, raw []byte) {
	t.Helper()
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil { t.Fatalf("parse Notification Envelope: %v", err) }
	if err := schema.Validate(instance); err != nil { t.Fatalf("Notification Envelope violates INT-CTR-001: %v", err) }
}

func verifyResearchNotificationFingerprint(t *testing.T, raw []byte) {
	t.Helper()
	var envelope struct {
		SourceModule string          `json:"source_module"`
		EventType string             `json:"event_type"`
		SchemaVersion string         `json:"schema_version"`
		SubjectRef string            `json:"subject_ref"`
		OccurredAt string            `json:"occurred_at"`
		Payload json.RawMessage      `json:"payload"`
		PayloadFingerprint string    `json:"payload_fingerprint"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil { t.Fatal(err) }
	input := lifecycleFingerprintInput{envelope.SourceModule, envelope.EventType, envelope.SchemaVersion, envelope.SubjectRef, envelope.OccurredAt, envelope.Payload}
	encoded, err := json.Marshal(input)
	if err != nil { t.Fatal(err) }
	canonical, err := jcs.Transform(encoded)
	if err != nil { t.Fatal(err) }
	if got := sha256Hex(canonical); got != envelope.PayloadFingerprint {
		t.Fatalf("payload fingerprint mismatch: got %s, want %s", got, envelope.PayloadFingerprint)
	}
}

func sha256Hex(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func TestResearchNotificationGoldenEnvelopesSatisfyContract(t *testing.T) {
	schema := compileResearchNotificationContract(t)
	for _, name := range []string{"research-event-lifecycle-created.golden.json", "research-event-lifecycle-invalidated.golden.json"} {
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("testdata", "notification", name))
			if err != nil { t.Fatalf("read Notification golden %s: %v", name, err) }
			validateResearchNotificationEnvelope(t, schema, raw)
			verifyResearchNotificationFingerprint(t, raw)
		})
	}
}

func TestResearchLifecycleProducerOutputSatisfiesNotificationContract(t *testing.T) {
	schema := compileResearchNotificationContract(t)
	now := time.Date(2026, 11, 9, 4, 5, 6, 123456789, time.UTC)
	created := researchevent.ResearchEvent{
		EventID: "evt_contract", EventVersion: 1, EventType: "earnings_guidance",
		Title: "业绩指引", Status: researchevent.StatusDetected,
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	envelope, _, err := buildResearchLifecycleCreatedEnvelope(created, nil, "corr_parent_123", now)
	if err != nil { t.Fatal(err) }
	raw, err := json.Marshal(envelope)
	if err != nil { t.Fatal(err) }
	validateResearchNotificationEnvelope(t, schema, raw)
	verifyResearchNotificationFingerprint(t, raw)

	next := created
	next.EventVersion = 2
	next.Status = researchevent.StatusConsolidating
	next.SystemAsOf = now.Add(time.Second)
	transition, err := researchevent.NewLifecycleTransition(created, next, now.Add(2*time.Second))
	if err != nil { t.Fatal(err) }
	updated, _, err := buildResearchLifecycleEnvelope(created, next, *transition, nil, "corr_parent_123", now.Add(2*time.Second))
	if err != nil { t.Fatal(err) }
	raw, err = json.Marshal(updated)
	if err != nil { t.Fatal(err) }
	validateResearchNotificationEnvelope(t, schema, raw)
	verifyResearchNotificationFingerprint(t, raw)
}

func TestResearchNotificationContractRejectsUnexpectedPayloadField(t *testing.T) {
	schema := compileResearchNotificationContract(t)
	raw, err := os.ReadFile(filepath.Join("testdata", "notification", "research-event-lifecycle-created.golden.json"))
	if err != nil { t.Fatal(err) }
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil { t.Fatal(err) }
	payload := value["payload"].(map[string]any)
	payload["unreviewed_field"] = true
	invalid, err := json.Marshal(value)
	if err != nil { t.Fatal(err) }
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(invalid))
	if err != nil { t.Fatal(err) }
	if err := schema.Validate(instance); err == nil {
		t.Fatal("unknown payload field unexpectedly passed the closed Notification contract")
	}
}
