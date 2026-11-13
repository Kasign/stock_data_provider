// Package relay contains Server-owned cross-module delivery components.
package relay

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gowebpki/jcs"

	researchflowapp "example.com/stock-analysis-flow/internal/researchflow/application"
)

type ResearchflowNotificationConfig struct {
	WorkerID       string
	BatchSize      int
	PollInterval   time.Duration
	LeaseDuration  time.Duration
	PublishTimeout time.Duration
}

type researchflowEventReceiver interface {
	Receive(context.Context, []byte) error
}

type researchflowTerminalPayload struct {
	SourceEventRef    string  `json:"source_event_ref"`
	RunRef            string  `json:"run_ref"`
	Status            string  `json:"status"`
	TerminationReason *string `json:"termination_reason"`
	SystemAsOf        string  `json:"system_as_of"`
	FinishedAt        string  `json:"finished_at"`
	ResultSummaryRef  *string `json:"result_summary_ref"`
	TemplateRef       string  `json:"template_ref"`
	TemplateVersion   string  `json:"template_version"`
}

type researchflowNotificationEnvelope struct {
	EventID            string          `json:"event_id"`
	EventType          string          `json:"event_type"`
	SchemaVersion      string          `json:"schema_version"`
	SourceModule       string          `json:"source_module"`
	OccurredAt         string          `json:"occurred_at"`
	PublishedAt        string          `json:"published_at"`
	SubjectRef         string          `json:"subject_ref"`
	IdempotencyKey     string          `json:"idempotency_key"`
	CorrelationID      string          `json:"correlation_id"`
	PayloadFingerprint string          `json:"payload_fingerprint"`
	Payload            json.RawMessage `json:"payload"`
}

type researchflowFingerprintInput struct {
	EventType     string          `json:"event_type"`
	SchemaVersion string          `json:"schema_version"`
	SourceModule  string          `json:"source_module"`
	SubjectRef    string          `json:"subject_ref"`
	OccurredAt    string          `json:"occurred_at"`
	Payload       json.RawMessage `json:"payload"`
}

type researchflowNotificationPublisher struct {
	receiver researchflowEventReceiver
}

type researchflowPublishError struct {
	code string
	err  error
}

func (e *researchflowPublishError) Error() string {
	if e == nil || e.err == nil {
		return "Researchflow Notification publish failed"
	}
	return e.err.Error()
}

func (e *researchflowPublishError) Unwrap() error { return e.err }
func (e *researchflowPublishError) Code() string  { return e.code }

func (p researchflowNotificationPublisher) Publish(ctx context.Context, message researchflowapp.OutboxMessage) error {
	if p.receiver == nil || ctx == nil {
		return &researchflowPublishError{code: "notification_receive_unavailable", err: errors.New("Notification EventReceiver is not configured")}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if message.EventType != "researchflow.run.terminal" || message.SchemaVersion != "1.0.0" ||
		strings.TrimSpace(message.Ref) == "" || strings.TrimSpace(message.RunRef) == "" ||
		strings.TrimSpace(message.IdempotencyKey) == "" || strings.TrimSpace(message.CorrelationID) == "" ||
		strings.TrimSpace(message.PayloadFingerprint) == "" || message.CreatedAt.IsZero() {
		return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("Researchflow Outbox metadata does not match the frozen Notification event contract")}
	}

	sourcePayload := []byte(message.Payload)
	sourceDigest := sha256.Sum256(sourcePayload)
	if hex.EncodeToString(sourceDigest[:]) != message.PayloadFingerprint {
		return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("Researchflow Outbox payload fingerprint does not match its immutable payload")}
	}
	var payload researchflowTerminalPayload
	decoder := json.NewDecoder(strings.NewReader(message.Payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return &researchflowPublishError{code: "notification_event_invalid", err: fmt.Errorf("decode Researchflow terminal payload: %w", err)}
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("Researchflow terminal payload contains trailing JSON data")}
	}
	if payload.SourceEventRef == "" || payload.SourceEventRef != message.RunRef ||
		payload.RunRef != message.RunRef || payload.TemplateRef == "" || payload.TemplateVersion == "" {
		return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("Researchflow terminal payload identity is inconsistent with its Outbox row")}
	}
	switch payload.Status {
	case "succeeded", "partial", "failed", "cancelled":
	default:
		return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("Researchflow terminal payload status is not terminal")}
	}
	if payload.Status == "cancelled" {
		if payload.TerminationReason == nil || *payload.TerminationReason != "user_cancelled" || payload.ResultSummaryRef != nil {
			return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("cancelled Researchflow event does not match the frozen payload contract")}
		}
	} else {
		if payload.ResultSummaryRef == nil || strings.TrimSpace(*payload.ResultSummaryRef) == "" {
			return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("terminal Researchflow event is missing its ResultSummary reference")}
		}
		if payload.TerminationReason != nil && *payload.TerminationReason != "deadline_exceeded" {
			return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("terminal Researchflow event has an unsupported termination reason")}
		}
	}
	systemAsOf, err := parseTerminalUTC(payload.SystemAsOf)
	if err != nil {
		return &researchflowPublishError{code: "notification_event_invalid", err: fmt.Errorf("invalid Researchflow system_as_of: %w", err)}
	}
	finishedAt, err := parseTerminalUTC(payload.FinishedAt)
	if err != nil {
		return &researchflowPublishError{code: "notification_event_invalid", err: fmt.Errorf("invalid Researchflow finished_at: %w", err)}
	}
	if !systemAsOf.Before(finishedAt) && !systemAsOf.Equal(finishedAt) {
		return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("Researchflow system_as_of is later than finished_at")}
	}
	publishedAt := message.CreatedAt.UTC()
	if !finishedAt.Equal(publishedAt) {
		return &researchflowPublishError{code: "notification_event_invalid", err: errors.New("Researchflow Outbox created_at differs from terminal finished_at")}
	}

	fingerprintInput := researchflowFingerprintInput{
		EventType: "researchflow.run.terminal", SchemaVersion: "1.0.0",
		SourceModule: "researchflow", SubjectRef: message.RunRef,
		OccurredAt:  payload.FinishedAt, Payload:            json.RawMessage(sourcePayload),
	}
	fingerprintJSON, err := json.Marshal(fingerprintInput)
	if err != nil {
		return &researchflowPublishError{code: "notification_event_invalid", err: fmt.Errorf("encode Notification fingerprint input: %w", err)}
	}
	canonical, err := jcs.Transform(fingerprintJSON)
	if err != nil {
		return &researchflowPublishError{code: "notification_event_invalid", err: fmt.Errorf("canonicalize Notification fingerprint input: %w", err)}
	}
	fingerprint := sha256.Sum256(canonical)
	envelope := researchflowNotificationEnvelope{
		EventID: message.Ref, EventType: "researchflow.run.terminal",
		SchemaVersion: "1.0.0", SourceModule: "researchflow",
		OccurredAt:  payload.FinishedAt,
		PublishedAt: publishedAt.Format(time.RFC3339Nano),
		SubjectRef:  message.RunRef, IdempotencyKey: message.IdempotencyKey,
		CorrelationID:      message.CorrelationID,
		PayloadFingerprint: hex.EncodeToString(fingerprint[:]),
		Payload: json.RawMessage(sourcePayload),
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		return &researchflowPublishError{code: "notification_event_invalid", err: fmt.Errorf("encode Notification Event Envelope: %w", err)}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.receiver.Receive(ctx, raw); err != nil {
		return &researchflowPublishError{code: "notification_receive_unavailable", err: fmt.Errorf("Notification EventReceiver rejected Researchflow terminal event: %w", err)}
	}
	return nil
}

func parseTerminalUTC(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) != raw || !strings.HasSuffix(raw, "Z") {
		return time.Time{}, errors.New("timestamp must be RFC3339Nano UTC with Z suffix")
	}
	value, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil || value.Location() != time.UTC {
		return time.Time{}, errors.New("timestamp must be RFC3339Nano UTC")
	}
	return value, nil
}

type ResearchflowNotificationRelay struct {
	dispatcher *researchflowapp.OutboxDispatcher
	config     ResearchflowNotificationConfig
	logger     *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewResearchflowNotificationRelay(
	store researchflowapp.OutboxStore,
	receiver researchflowEventReceiver,
	config ResearchflowNotificationConfig,
	logger *slog.Logger,
) (*ResearchflowNotificationRelay, error) {
	if store == nil || receiver == nil {
		return nil, errors.New("Researchflow Notification relay dependencies are required")
	}
	if strings.TrimSpace(config.WorkerID) == "" || len(config.WorkerID) > 128 ||
		config.BatchSize < 1 || config.BatchSize > 100 ||
		config.PollInterval < 10*time.Millisecond || config.PollInterval > time.Minute ||
		config.LeaseDuration < time.Second || config.LeaseDuration > 5*time.Minute ||
		config.PublishTimeout <= 0 || config.PublishTimeout >= config.LeaseDuration {
		return nil, errors.New("Researchflow Notification relay configuration is invalid")
	}
	dispatcher, err := researchflowapp.NewOutboxDispatcher(store, researchflowNotificationPublisher{receiver: receiver}, researchflowapp.OutboxDispatcherConfig{
		WorkerID: config.WorkerID, BatchSize: config.BatchSize,
		MaxConcurrency: config.BatchSize, LeaseDuration: config.LeaseDuration,
		PublishTimeout: config.PublishTimeout,
	})
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ResearchflowNotificationRelay{dispatcher: dispatcher, config: config, logger: logger}, nil
}

func (r *ResearchflowNotificationRelay) Name() string {
	return "researchflow-notification-outbox-relay"
}

func (r *ResearchflowNotificationRelay) Start(ctx context.Context) error {
	if r == nil || ctx == nil {
		return errors.New("Researchflow Notification relay and start context are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil || r.done != nil {
		return errors.New("Researchflow Notification relay has already been started")
	}
	workerCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	r.cancel = cancel
	r.done = make(chan struct{})
	go func() {
		defer close(r.done)
		r.run(workerCtx)
	}()
	return nil
}

func (r *ResearchflowNotificationRelay) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("Researchflow Notification relay stop context is required")
	}
	r.mu.Lock()
	cancel, done := r.cancel, r.done
	r.mu.Unlock()
	if cancel == nil || done == nil {
		return nil
	}
	cancel()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *ResearchflowNotificationRelay) run(ctx context.Context) {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		if _, err := r.ProcessOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.Error("Researchflow Notification relay batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (r *ResearchflowNotificationRelay) ProcessOnce(ctx context.Context) (researchflowapp.OutboxDispatchResult, error) {
	if r == nil || r.dispatcher == nil || ctx == nil {
		return researchflowapp.OutboxDispatchResult{}, errors.New("Researchflow Notification relay is not configured")
	}
	return r.dispatcher.DispatchBatch(ctx)
}
