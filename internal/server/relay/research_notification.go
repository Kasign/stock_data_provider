// Package relay contains Server-owned cross-module delivery components.
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	notificationdomain "example.com/stock-analysis-flow/internal/notification/domain"
	notificationschema "example.com/stock-analysis-flow/internal/notification/schema"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
)

const (
	rejectInvalidEventReason = "notification_event_invalid"
	retryUnavailableReason   = "notification_receive_unavailable"
)

type Outbox interface {
	Claim(context.Context, string, int, time.Duration) ([]researchapp.NotificationEventEnvelope, error)
	Confirm(context.Context, string, string, string) error
	Retry(context.Context, string, string, string, time.Duration, string) error
	Reject(context.Context, string, string, string, string) error
}

type EventReceiver interface {
	Receive(context.Context, []byte) error
}

type Config struct {
	WorkerID      string
	BatchSize     int
	PollInterval  time.Duration
	LeaseDuration time.Duration
	RetryDelay    time.Duration
}

func (c Config) Validate() error {
	if c.WorkerID == "" || len(c.WorkerID) > 128 {
		return errors.New("Research Outbox relay worker ID is invalid")
	}
	for _, r := range c.WorkerID {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return errors.New("Research Outbox relay worker ID is invalid")
		}
	}
	if c.BatchSize < 1 || c.BatchSize > 100 {
		return errors.New("Research Outbox relay batch size must be between 1 and 100")
	}
	if c.PollInterval < 10*time.Millisecond || c.PollInterval > time.Minute {
		return errors.New("Research Outbox relay poll interval must be between 10ms and 1m")
	}
	if c.LeaseDuration < time.Second || c.LeaseDuration > 5*time.Minute {
		return errors.New("Research Outbox relay lease duration must be between 1s and 5m")
	}
	if c.RetryDelay <= 0 || c.RetryDelay > 24*time.Hour {
		return errors.New("Research Outbox relay retry delay must be positive and no greater than 24h")
	}
	return nil
}

type ResearchOutboxRelay struct {
	outbox   Outbox
	receiver EventReceiver
	config   Config
	logger   *slog.Logger

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
}

func NewResearchOutboxRelay(outbox Outbox, receiver EventReceiver, config Config, logger *slog.Logger) (*ResearchOutboxRelay, error) {
	if outbox == nil || receiver == nil {
		return nil, errors.New("Research Outbox relay dependencies are required")
	}
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &ResearchOutboxRelay{outbox: outbox, receiver: receiver, config: config, logger: logger}, nil
}

func (r *ResearchOutboxRelay) Name() string { return "research-notification-outbox-relay" }

func (r *ResearchOutboxRelay) Start(ctx context.Context) error {
	if r == nil || ctx == nil {
		return errors.New("Research Outbox relay and start context are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cancel != nil || r.done != nil {
		return errors.New("Research Outbox relay has already been started")
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

func (r *ResearchOutboxRelay) Stop(ctx context.Context) error {
	if r == nil {
		return nil
	}
	if ctx == nil {
		return errors.New("Research Outbox relay stop context is required")
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

func (r *ResearchOutboxRelay) run(ctx context.Context) {
	ticker := time.NewTicker(r.config.PollInterval)
	defer ticker.Stop()
	for {
		if err := r.ProcessOnce(ctx); err != nil && !errors.Is(err, context.Canceled) {
			r.logger.Error("Research Outbox relay batch failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// ProcessOnce claims one bounded batch and handles every still-owned lease.
// It is exported for deterministic integration and operational tests.
func (r *ResearchOutboxRelay) ProcessOnce(ctx context.Context) error {
	if r == nil || r.outbox == nil || r.receiver == nil || ctx == nil {
		return errors.New("Research Outbox relay is not configured")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	events, err := r.outbox.Claim(ctx, r.config.WorkerID, r.config.BatchSize, r.config.LeaseDuration)
	if err != nil {
		return fmt.Errorf("claim Research Outbox: %w", err)
	}
	var wait sync.WaitGroup
	errorsCh := make(chan error, len(events))
	for _, event := range events {
		if err := ctx.Err(); err != nil {
			break
		}
		event := event
		wait.Add(1)
		go func() {
			defer wait.Done()
			if err := r.deliver(ctx, event); err != nil {
				errorsCh <- fmt.Errorf("deliver Research Outbox event %q: %w", event.EventID, err)
			}
		}()
	}
	wait.Wait()
	close(errorsCh)
	var result error
	for err := range errorsCh {
		result = errors.Join(result, err)
	}
	if err := ctx.Err(); err != nil {
		result = errors.Join(result, err)
	}
	return result
}

func (r *ResearchOutboxRelay) deliver(ctx context.Context, event researchapp.NotificationEventEnvelope) error {
	raw, err := json.Marshal(event)
	if err != nil {
		if rejectErr := r.outbox.Reject(ctx, event.EventID, r.config.WorkerID, event.LeaseToken, rejectInvalidEventReason); rejectErr != nil {
			return errors.Join(fmt.Errorf("encode Research Outbox envelope: %w", err), fmt.Errorf("reject invalid envelope: %w", rejectErr))
		}
		return nil
	}
	if err := r.receiver.Receive(ctx, raw); err != nil {
		if isPermanentEventError(err) {
			if rejectErr := r.outbox.Reject(ctx, event.EventID, r.config.WorkerID, event.LeaseToken, rejectInvalidEventReason); rejectErr != nil {
				return fmt.Errorf("reject invalid Notification event: %w", rejectErr)
			}
			r.logger.Error("Research Outbox event rejected by Notification", "event_id", event.EventID, "reason_code", rejectInvalidEventReason)
			return nil
		}
		if retryErr := r.outbox.Retry(ctx, event.EventID, r.config.WorkerID, event.LeaseToken, r.config.RetryDelay, retryUnavailableReason); retryErr != nil {
			return fmt.Errorf("schedule Research Outbox retry after Notification receive failure: %w", retryErr)
		}
		r.logger.Warn("Notification Inbox temporarily unavailable; Research Outbox retry scheduled", "event_id", event.EventID, "reason_code", retryUnavailableReason)
		return nil
	}
	// If confirmation fails after Inbox acceptance, lease expiry will replay the
	// same immutable Envelope; Notification Inbox idempotency makes that safe.
	if err := r.outbox.Confirm(ctx, event.EventID, r.config.WorkerID, event.LeaseToken); err != nil {
		return fmt.Errorf("confirm Research Outbox delivery: %w", err)
	}
	return nil
}

func isPermanentEventError(err error) bool {
	return errors.Is(err, notificationschema.ErrUnknownSchema) ||
		errors.Is(err, notificationschema.ErrSourceModuleMismatch) ||
		errors.Is(err, notificationschema.ErrSchemaValidation) ||
		errors.Is(err, notificationschema.ErrInvalidEventJSON) ||
		errors.Is(err, notificationschema.ErrEventResourceLimitExceeded) ||
		errors.Is(err, notificationdomain.ErrInvalidPayloadFingerprint) ||
		errors.Is(err, notificationdomain.ErrPayloadFingerprintMismatch) ||
		errors.Is(err, notificationdomain.ErrInboxIdempotencyConflict)
}
