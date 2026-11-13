package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	researchflowapp "example.com/stock-analysis-flow/internal/researchflow/application"
)

type researchflowNotificationReceiverStub struct {
	raw []byte
	err error
}

func (r *researchflowNotificationReceiverStub) Receive(_ context.Context, raw []byte) error {
	r.raw = append([]byte(nil), raw...)
	return r.err
}

func terminalOutboxMessage(t *testing.T, status, reason string, includeSummary bool) researchflowapp.OutboxMessage {
	t.Helper()
	var terminationReason *string
	if reason != "" {
		value := reason
		terminationReason = &value
	}
	var summaryRef *string
	if includeSummary {
		value := "summary_001"
		summaryRef = &value
	}
	payload := researchflowTerminalPayload{
		SourceEventRef:    "run_001",
		RunRef:            "run_001",
		Status:            status,
		TerminationReason: terminationReason,
		SystemAsOf:        "2026-11-05T16:00:00.000000000Z",
		FinishedAt:        "2026-11-06T04:10:00.000000000Z",
		ResultSummaryRef:  summaryRef,
		TemplateRef:       "daily_stock_analysis",
		TemplateVersion:   "1.0.0",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	createdAt, err := time.Parse(time.RFC3339Nano, payload.FinishedAt)
	if err != nil {
		t.Fatal(err)
	}
	return researchflowapp.OutboxMessage{
		Ref: "outbox_001", RunRef: "run_001",
		EventType: "researchflow.run.terminal", SchemaVersion: "1.0.0",
		IdempotencyKey: "run-terminal:run_001", CorrelationID: "corr_001",
		PayloadFingerprint: hex.EncodeToString(digest[:]),
		Payload:            string(raw), CreatedAt: createdAt,
	}
}

type emptyResearchflowOutboxStore struct {
	claimed chan struct{}
}

func (s *emptyResearchflowOutboxStore) ClaimOutbox(context.Context, string, string, int, time.Duration) ([]researchflowapp.OutboxMessage, error) {
	select {
	case s.claimed <- struct{}{}:
	default:
	}
	return nil, nil
}

func (*emptyResearchflowOutboxStore) AcknowledgeOutbox(context.Context, string, string, uint64) (bool, error) {
	return false, nil
}

func (*emptyResearchflowOutboxStore) RetryOutbox(context.Context, string, string, uint64, time.Time, string) (bool, error) {
	return false, nil
}

func TestResearchflowNotificationRelayStartStop(t *testing.T) {
	store := &emptyResearchflowOutboxStore{claimed: make(chan struct{}, 1)}
	relay, err := NewResearchflowNotificationRelay(store, &researchflowNotificationReceiverStub{}, ResearchflowNotificationConfig{
		WorkerID: "test-researchflow-notification-relay",
		BatchSize: 1, PollInterval: 10 * time.Millisecond,
		LeaseDuration: time.Second, PublishTimeout: 500 * time.Millisecond,
	}, nil)
	if err != nil {
		t.Fatalf("create Researchflow Notification Relay: %v", err)
	}
	if relay.Name() != "researchflow-notification-outbox-relay" {
		t.Fatalf("relay Name() = %q", relay.Name())
	}
	if err := relay.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() before Start() error = %v", err)
	}
	startCtx := context.Background()
	if err := relay.Start(startCtx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-store.claimed:
	case <-time.After(time.Second):
		t.Fatal("Relay did not begin polling the Outbox after Start()")
	}
	if err := relay.Start(startCtx); err == nil {
		t.Fatal("Start() called twice without Stop() should fail")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := relay.Stop(stopCtx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := relay.Stop(stopCtx); err != nil {
		t.Fatalf("repeated Stop() error = %v", err)
	}
}

func TestResearchflowNotificationPublisherBuildsFrozenEnvelope(t *testing.T) {
	receiver := &researchflowNotificationReceiverStub{}
	message := terminalOutboxMessage(t, "succeeded", "", true)
	if err := (researchflowNotificationPublisher{receiver: receiver}).Publish(context.Background(), message); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	var envelope researchflowNotificationEnvelope
	if err := json.Unmarshal(receiver.raw, &envelope); err != nil {
		t.Fatalf("decode Notification envelope: %v", err)
	}
	if envelope.EventID != message.Ref || envelope.EventType != "researchflow.run.terminal" ||
		envelope.SchemaVersion != "1.0.0" || envelope.SourceModule != "researchflow" ||
		envelope.SubjectRef != message.RunRef || envelope.IdempotencyKey != message.IdempotencyKey ||
		envelope.CorrelationID != message.CorrelationID ||
		envelope.OccurredAt != "2026-11-06T04:10:00.000000000Z" ||
		envelope.PublishedAt != "2026-11-06T04:10:00Z" || len(envelope.PayloadFingerprint) != 64 {
		t.Fatalf("Notification envelope fields are inconsistent: %+v", envelope)
	}
	var payload researchflowTerminalPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SourceEventRef != message.RunRef || payload.RunRef != message.RunRef ||
		payload.Status != "succeeded" || payload.ResultSummaryRef == nil ||
		*payload.ResultSummaryRef != "summary_001" {
		t.Fatalf("Notification terminal payload is inconsistent: %+v", payload)
	}
}

func TestResearchflowNotificationPublisherAllowsCancelledGoldenShape(t *testing.T) {
	receiver := &researchflowNotificationReceiverStub{}
	message := terminalOutboxMessage(t, "cancelled", "user_cancelled", false)
	if err := (researchflowNotificationPublisher{receiver: receiver}).Publish(context.Background(), message); err != nil {
		t.Fatalf("Publish() error = %v", err)
	}
	var envelope researchflowNotificationEnvelope
	if err := json.Unmarshal(receiver.raw, &envelope); err != nil {
		t.Fatal(err)
	}
	var payload researchflowTerminalPayload
	if err := json.Unmarshal(envelope.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != "cancelled" || payload.TerminationReason == nil ||
		*payload.TerminationReason != "user_cancelled" || payload.ResultSummaryRef != nil {
		t.Fatalf("cancelled payload does not match the frozen Golden: %+v", payload)
	}
}

func TestResearchflowNotificationPublisherRejectsPayloadFingerprintMismatch(t *testing.T) {
	receiver := &researchflowNotificationReceiverStub{}
	message := terminalOutboxMessage(t, "succeeded", "", true)
	message.PayloadFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
	err := (researchflowNotificationPublisher{receiver: receiver}).Publish(context.Background(), message)
	var coded interface{ Code() string }
	if !errors.As(err, &coded) || coded.Code() != "notification_event_invalid" {
		t.Fatalf("Publish() error = %v, want notification_event_invalid", err)
	}
	if len(receiver.raw) != 0 {
		t.Fatal("invalid Researchflow payload reached Notification EventReceiver")
	}
}

func TestResearchflowNotificationPublisherRejectsMismatchedRunReference(t *testing.T) {
	receiver := &researchflowNotificationReceiverStub{}
	message := terminalOutboxMessage(t, "succeeded", "", true)
	var payload researchflowTerminalPayload
	if err := json.Unmarshal([]byte(message.Payload), &payload); err != nil {
		t.Fatal(err)
	}
	payload.SourceEventRef = "another_run"
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	message.Payload = string(raw)
	message.PayloadFingerprint = hex.EncodeToString(digest[:])
	err = (researchflowNotificationPublisher{receiver: receiver}).Publish(context.Background(), message)
	var coded interface{ Code() string }
	if !errors.As(err, &coded) || coded.Code() != "notification_event_invalid" {
		t.Fatalf("Publish() error = %v, want notification_event_invalid", err)
	}
	if len(receiver.raw) != 0 {
		t.Fatal("inconsistent Researchflow payload reached Notification EventReceiver")
	}
}
