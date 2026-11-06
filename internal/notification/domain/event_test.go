package domain

import (
	"errors"
	"testing"
)

func TestCalculatePayloadFingerprintExcludesTransportMetadata(t *testing.T) {
	event := EventEnvelope{
		EventID: "event-a", EventType: "quant.risk_decision.changed",
		SchemaVersion: "1.0.0", SourceModule: "quant",
		OccurredAt: "2026-11-06T04:30:00Z", PublishedAt: "2026-11-06T04:30:01Z",
		SubjectRef: "security_600000_SH", IdempotencyKey: "publish-a",
		CorrelationID: "corr-a", Payload: []byte(`{"status":"reject","version":2}`),
	}
	first, err := event.CalculatePayloadFingerprint()
	if err != nil {
		t.Fatal(err)
	}

	event.EventID = "event-b"
	event.PublishedAt = "2026-11-06T05:00:00Z"
	event.IdempotencyKey = "publish-b"
	event.CorrelationID = "corr-b"
	second, err := event.CalculatePayloadFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("transport metadata changed payload fingerprint: %s != %s", first, second)
	}

	event.SubjectRef = "security_000001_SH"
	third, err := event.CalculatePayloadFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first == third {
		t.Fatal("subject_ref change did not change payload fingerprint")
	}
}

func TestCalculatePayloadFingerprintRejectsDuplicateObjectKeys(t *testing.T) {
	event := EventEnvelope{
		EventType: "test.event", SchemaVersion: "1.0.0",
		SourceModule: "test", SubjectRef: "subject",
		OccurredAt: "2026-11-06T00:00:00Z",
		Payload: []byte(`{"value":1,"value":2}`),
	}
	if _, err := event.CalculatePayloadFingerprint(); err == nil {
		t.Fatal("expected duplicate JSON property rejection")
	}
}

func TestVerifyPayloadFingerprint(t *testing.T) {
	event := EventEnvelope{
		EventType: "test.event", SchemaVersion: "1.0.0",
		SourceModule: "test", SubjectRef: "subject",
		OccurredAt: "2026-11-06T00:00:00Z", Payload: []byte(`{"value":1}`),
	}
	fingerprint, err := event.CalculatePayloadFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	event.PayloadFingerprint = fingerprint
	if err := event.VerifyPayloadFingerprint(); err != nil {
		t.Fatal(err)
	}
	event.PayloadFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := event.VerifyPayloadFingerprint(); !errors.Is(err, ErrPayloadFingerprintMismatch) {
		t.Fatalf("expected mismatch error, got %v", err)
	}
}
