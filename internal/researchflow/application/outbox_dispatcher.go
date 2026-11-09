package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// OutboxMessage is the application-facing delivery contract. The payload and
// idempotency key are immutable; lease metadata fences concurrent publishers.
type OutboxMessage struct {
	Ref               string
	RunRef             string
	EventType          string
	SchemaVersion      string
	IdempotencyKey     string
	CorrelationID      string
	PayloadFingerprint string
	Payload            string
	CreatedAt          time.Time
	AttemptCount       uint32
	LeaseGeneration    uint64
}

type OutboxStore interface {
	ClaimOutbox(context.Context, string, string, int, time.Duration) ([]OutboxMessage, error)
	AcknowledgeOutbox(context.Context, string, string, uint64) (bool, error)
	RetryOutbox(context.Context, string, string, uint64, time.Time, string) (bool, error)
}

type OutboxPublisher interface {
	Publish(context.Context, OutboxMessage) error
}

type OutboxErrorCoder interface {
	Code() string
}

type OutboxRetryPolicy interface {
	NextAttemptAt(attemptCount uint32, now time.Time) time.Time
}

type ExponentialOutboxRetryPolicy struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

func (p ExponentialOutboxRetryPolicy) NextAttemptAt(attemptCount uint32, now time.Time) time.Time {
	delay := p.BaseDelay
	if delay <= 0 {
		delay = time.Second
	}
	maxDelay := p.MaxDelay
	if maxDelay <= 0 {
		maxDelay = 5 * time.Minute
	}
	if delay > maxDelay {
		delay = maxDelay
	}
	for i := uint32(1); i < attemptCount && delay < maxDelay; i++ {
		if delay > maxDelay/2 {
			delay = maxDelay
			break
		}
		delay *= 2
	}
	return now.UTC().Add(delay)
}

type OutboxDispatcherConfig struct {
	WorkerID       string
	BatchSize      int
	MaxConcurrency int
	LeaseDuration  time.Duration
	PublishTimeout time.Duration
	RetryPolicy    OutboxRetryPolicy
}

type OutboxDispatchResult struct {
	Claimed   int
	Published int
	Retried   int
	Stale     int
}

type OutboxDispatcher struct {
	store     OutboxStore
	publisher OutboxPublisher
	config    OutboxDispatcherConfig
	now       func() time.Time
}

func NewOutboxDispatcher(
	store OutboxStore,
	publisher OutboxPublisher,
	config OutboxDispatcherConfig,
) (*OutboxDispatcher, error) {
	if store == nil || publisher == nil ||
		strings.TrimSpace(config.WorkerID) == "" || len(config.WorkerID) > 128 ||
		config.BatchSize <= 0 || config.BatchSize > 500 ||
		config.MaxConcurrency <= 0 || config.MaxConcurrency > 100 ||
		config.LeaseDuration <= 0 || config.PublishTimeout <= 0 ||
		config.PublishTimeout >= config.LeaseDuration {
		return nil, errors.New("Researchflow Outbox dispatcher dependencies or configuration are invalid")
	}
	if config.RetryPolicy == nil {
		config.RetryPolicy = ExponentialOutboxRetryPolicy{}
	}
	return &OutboxDispatcher{store: store, publisher: publisher, config: config, now: time.Now}, nil
}

// DispatchBatch provides at-least-once delivery. Publishers must honor the
// stable IdempotencyKey because a process can crash after external publish but
// before the local acknowledgement is committed.
func (d *OutboxDispatcher) DispatchBatch(ctx context.Context) (OutboxDispatchResult, error) {
	if d == nil || d.store == nil || d.publisher == nil {
		return OutboxDispatchResult{}, errors.New("Researchflow Outbox dispatcher is not configured")
	}
	claimLimit := d.config.BatchSize
	if claimLimit > d.config.MaxConcurrency {
		claimLimit = d.config.MaxConcurrency
	}
	messages, err := d.store.ClaimOutbox(ctx, d.config.WorkerID, "", claimLimit, d.config.LeaseDuration)
	if err != nil {
		return OutboxDispatchResult{}, fmt.Errorf("claim Researchflow Outbox batch: %w", err)
	}
	result := OutboxDispatchResult{Claimed: len(messages)}
	var mu sync.Mutex
	var failures []error
	var workers sync.WaitGroup
	for _, message := range messages {
		message := message
		workers.Add(1)
		go func() {
			defer workers.Done()
			publishCtx, cancel := context.WithTimeout(ctx, d.config.PublishTimeout)
			publishErr := d.publisher.Publish(publishCtx, message)
			cancel()

			mu.Lock()
			defer mu.Unlock()
			if publishErr != nil {
				errorCode := outboxErrorCode(publishErr)
				nextAttempt := d.config.RetryPolicy.NextAttemptAt(message.AttemptCount, d.now())
				retried, retryErr := d.store.RetryOutbox(ctx, message.Ref, d.config.WorkerID,
					message.LeaseGeneration, nextAttempt, errorCode)
				if retryErr != nil {
					failures = append(failures, fmt.Errorf("persist Outbox retry %q: %w", message.Ref, retryErr))
					return
				}
				if !retried {
					result.Stale++
					return
				}
				result.Retried++
				return
			}
			acknowledged, ackErr := d.store.AcknowledgeOutbox(ctx, message.Ref,
				d.config.WorkerID, message.LeaseGeneration)
			if ackErr != nil {
				failures = append(failures, fmt.Errorf("acknowledge Outbox %q: %w", message.Ref, ackErr))
				return
			}
			if !acknowledged {
				result.Stale++
				return
			}
			result.Published++
		}()
	}
	workers.Wait()
	if len(failures) > 0 {
		return result, errors.Join(failures...)
	}
	return result, nil
}

func outboxErrorCode(err error) string {
	var coded OutboxErrorCoder
	if errors.As(err, &coded) {
		code := strings.TrimSpace(coded.Code())
		if code != "" && len(code) <= 128 {
			return code
		}
	}
	return "publish_failed"
}
