package application

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type fixedExecutionRenderer struct {
	content domain.RenderedContent
	err     error
}

func (r fixedExecutionRenderer) Render(context.Context, domain.EventEnvelope, string, int64) (domain.RenderedContent, error) {
	return r.content, r.err
}

type cancelAfterSendAdapter struct {
	snapshot domain.ChannelConfigSnapshot
	cancel   context.CancelFunc
	calls    int
}

func (a *cancelAfterSendAdapter) ConfigSnapshot() domain.ChannelConfigSnapshot {
	return cloneChannelSnapshot(a.snapshot)
}

func (a *cancelAfterSendAdapter) Send(context.Context, domain.ChannelSendRequest) (domain.ChannelSendResult, error) {
	a.calls++
	a.cancel()
	return domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAccepted, ProviderMessageRef: "provider-message-after-cancel"}, nil
}

type sequenceExecutionClock struct {
	times []time.Time
	index int
}

func (c *sequenceExecutionClock) Now() time.Time {
	if c.index >= len(c.times) {
		return c.times[len(c.times)-1]
	}
	value := c.times[c.index]
	c.index++
	return value
}

func executionFixture(t *testing.T) (*DeliveryExecutionService, *memoryDeliveries, *fakeChannelAdapter, DeliveryClaim, domain.EventEnvelope) {
	t.Helper()
	repository := newMemoryDeliveries()
	delivery := validDeliveryForRepository()
	event := domain.EventEnvelope{
		EventID: delivery.EventID, EventType: "researchflow.run.terminal",
		SchemaVersion: "1.0.0", SourceModule: delivery.SourceModule,
		OccurredAt: "2026-11-09T02:00:00Z", PublishedAt: "2026-11-09T02:00:01Z",
		SubjectRef: "run:execution-1", IdempotencyKey: "event-key-1", CorrelationID: "correlation-1",
		Payload: json.RawMessage([]byte("{\"status\":\"succeeded\"}")),
	}
	fingerprint, err := event.CalculatePayloadFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	event.PayloadFingerprint = fingerprint
	delivery.PayloadFingerprint = fingerprint
	if err := repository.Create(context.Background(), delivery); err != nil {
		t.Fatal(err)
	}
	claimTime := delivery.CreatedAt.Add(time.Second)
	claim, claimed, err := repository.ClaimDue(
		context.Background(), "worker-attempt", claimTime, claimTime.Add(time.Minute),
	)
	if err != nil || !claimed {
		t.Fatalf("claim delivery: claimed=%v err=%v", claimed, err)
	}
	repository.now = claim.Lease.ExpiresAt.Add(-time.Second)

	preflight, _, _ := preflightFixture(t)
	snapshot := channelRegistryTestSnapshot()
	snapshot.ConfigVersion = claim.Delivery.ChannelConfigVersion
	adapter := &fakeChannelAdapter{
		snapshot: snapshot,
		result: domain.ChannelSendResult{
			Outcome:            domain.ChannelOutcomeAccepted,
			ProviderMessageRef: "provider-message-1",
		},
	}
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	preflight.channels = registry

	renderer := fixedExecutionRenderer{content: domain.RenderedContent{
		TemplateID:              claim.Delivery.TemplateID,
		TemplateVersion:         claim.Delivery.TemplateVersion,
		TemplateDigest:          strings.Repeat("c", 64),
		EventPayloadFingerprint: claim.Delivery.PayloadFingerprint,
		Format:                  domain.TemplateFormatText,
		Subject:                 "subject", Body: "body",
	}}
	preflightCompletion, err := NewDeliveryPreflightCompletionService(repository)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := NewDeliveryCompletionService(repository, repository, RetryPolicy{
		MaxAttempts: 3, MaxElapsed: time.Hour, PerAttemptTimeout: time.Minute,
		BaseDelay: time.Second, MaxDelay: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	clock := &sequenceExecutionClock{times: []time.Time{
		claim.Delivery.UpdatedAt.Add(time.Second),
		claim.Delivery.UpdatedAt.Add(2 * time.Second),
	}}
	service, err := NewDeliveryExecutionService(preflight, preflightCompletion, renderer, repository, completion, clock)
	if err != nil {
		t.Fatal(err)
	}
	return service, repository, adapter, claim, event
}

func TestDeliveryExecutionCallsChannelOnlyAfterAttemptStarts(t *testing.T) {
	service, repository, adapter, claim, event := executionFixture(t)

	result, err := service.Execute(
		context.Background(), claim, event, "attempt-execution-1",
		0.5, "evidence-execution-1",
	)
	if err != nil {
		t.Fatalf("execute delivery: %v", err)
	}
	if result.AttemptID != "attempt-execution-1" || result.ChannelResult == nil ||
		result.ChannelResult.Outcome != domain.ChannelOutcomeAccepted || adapter.calls != 1 {
		t.Fatalf("unexpected execution result/call count: result=%+v calls=%d", result, adapter.calls)
	}
	delivery, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found || delivery.Status != domain.DeliverySent || delivery.AttemptCount != 1 {
		t.Fatalf("delivery did not converge to sent: delivery=%+v found=%v err=%v", delivery, found, err)
	}
}

func TestDeliveryExecutionAttemptStartFailureNeverCallsChannel(t *testing.T) {
	service, repository, adapter, claim, event := executionFixture(t)
	repository.failAttemptNext = true

	_, err := service.Execute(
		context.Background(), claim, event, "attempt-execution-2",
		0.5, "evidence-execution-2",
	)
	if err == nil {
		t.Fatal("expected Attempt persistence failure")
	}
	if adapter.calls != 0 {
		t.Fatalf("channel must not be called when Attempt start fails, calls=%d", adapter.calls)
	}
	delivery, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if getErr != nil || !found || delivery.Status != domain.DeliverySending || delivery.AttemptCount != 0 {
		t.Fatalf("failed Attempt start must not mutate Delivery: delivery=%+v found=%v err=%v", delivery, found, getErr)
	}
}

func TestDeliveryExecutionAmbiguousResultConvergesToUnknown(t *testing.T) {
	service, repository, adapter, claim, event := executionFixture(t)
	adapter.result = domain.ChannelSendResult{
		Outcome:   domain.ChannelOutcomeAmbiguous,
		ErrorCode: "provider_timeout",
	}

	result, err := service.Execute(
		context.Background(), claim, event, "attempt-execution-3",
		0.5, "evidence-execution-3",
	)
	if err != nil {
		t.Fatalf("ambiguous channel result should be persisted before returning: %v", err)
	}
	if result.ChannelResult == nil || result.ChannelResult.Outcome != domain.ChannelOutcomeAmbiguous || adapter.calls != 1 {
		t.Fatalf("unexpected ambiguous result: %+v calls=%d", result, adapter.calls)
	}
	delivery, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if getErr != nil || !found || delivery.Status != domain.DeliveryUnknown {
		t.Fatalf("ambiguous send must converge to unknown: delivery=%+v found=%v err=%v", delivery, found, getErr)
	}
}

func TestDeliveryExecutionRenderFailureBlocksWithoutChannelCall(t *testing.T) {
	service, repository, adapter, claim, event := executionFixture(t)
	service.renderer = fixedExecutionRenderer{err: domain.ErrInvalidPayloadFingerprint}

	_, err := service.Execute(
		context.Background(), claim, event, "attempt-execution-4",
		0.5, "evidence-execution-4",
	)
	if err == nil {
		t.Fatal("expected template render failure")
	}
	if adapter.calls != 0 {
		t.Fatalf("render failure must not call channel, calls=%d", adapter.calls)
	}
	delivery, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if getErr != nil || !found || delivery.Status != domain.DeliveryBlocked {
		t.Fatalf("render failure must persist blocked state: delivery=%+v found=%v err=%v", delivery, found, getErr)
	}
}

func TestDeliveryExecutionRejectsEventFromAnotherDelivery(t *testing.T) {
	service, repository, adapter, claim, event := executionFixture(t)
	event.EventID = "another-event"

	_, err := service.Execute(
		context.Background(), claim, event, "attempt-execution-5",
		0.5, "evidence-execution-5",
	)
	if err == nil {
		t.Fatal("event identity mismatch must fail closed")
	}
	if adapter.calls != 0 {
		t.Fatalf("mismatched event must not call channel, calls=%d", adapter.calls)
	}
	delivery, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if getErr != nil || !found || delivery.Status != domain.DeliverySending || delivery.AttemptCount != 0 {
		t.Fatalf("mismatched event must not mutate Delivery: delivery=%+v found=%v err=%v", delivery, found, getErr)
	}
}

func TestDeliveryExecutionCancellationAfterSendLeavesAttemptForUnknownRecovery(t *testing.T) {
	service, repository, _, claim, event := executionFixture(t)
	snapshot := channelRegistryTestSnapshot()
	snapshot.ConfigVersion = claim.Delivery.ChannelConfigVersion
	ctx, cancel := context.WithCancel(context.Background())
	adapter := &cancelAfterSendAdapter{snapshot: snapshot, cancel: cancel}
	registry := NewChannelRegistry()
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	service.preflight.channels = registry

	_, err := service.Execute(
		ctx, claim, event, "attempt-execution-cancelled",
		0.5, "evidence-execution-cancelled",
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("completion must report cancellation after channel side effect, got %v", err)
	}
	if adapter.calls != 1 {
		t.Fatalf("channel must be called exactly once, calls=%d", adapter.calls)
	}
	delivery, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if getErr != nil || !found || delivery.Status != domain.DeliverySending || delivery.AttemptCount != 1 {
		t.Fatalf("cancelled completion must preserve sending state for lease recovery: delivery=%+v found=%v err=%v", delivery, found, getErr)
	}
	attempts, listErr := repository.ListByDelivery(context.Background(), claim.Delivery.DeliveryID)
	if listErr != nil || len(attempts) != 1 || attempts[0].FinishedAt != nil {
		t.Fatalf("attempt must remain unfinished for conservative recovery: attempts=%+v err=%v", attempts, listErr)
	}
}

func TestDeliveryExecutionRetryableRejectionSchedulesRetry(t *testing.T) {
	service, repository, adapter, claim, event := executionFixture(t)
	adapter.result = domain.ChannelSendResult{
		Outcome: domain.ChannelOutcomeRejected,
		ErrorCode: "rate_limited",
	}

	result, err := service.Execute(
		context.Background(), claim, event, "attempt-execution-retry",
		0.5, "evidence-execution-retry",
	)
	if err != nil {
		t.Fatalf("execute retryable rejection: %v", err)
	}
	if result.ChannelResult == nil || result.ChannelResult.Outcome != domain.ChannelOutcomeRejected || adapter.calls != 1 {
		t.Fatalf("unexpected rejection result: %+v calls=%d", result, adapter.calls)
	}
	delivery, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if getErr != nil || !found || delivery.Status != domain.DeliveryRetryWait || delivery.NextAttemptAt == nil || delivery.AttemptCount != 1 {
		t.Fatalf("retryable rejection must schedule retry without sending twice: delivery=%+v found=%v err=%v", delivery, found, getErr)
	}
}

func TestDeliveryExecutionRejectsInvalidRetrySampleBeforeChannelCall(t *testing.T) {
	for _, sample := range []float64{-0.1, 1, math.NaN(), math.Inf(1)} {
		t.Run(fmt.Sprintf("sample_%v", sample), func(t *testing.T) {
			service, repository, adapter, claim, event := executionFixture(t)
			_, err := service.Execute(
				context.Background(), claim, event, "attempt-invalid-random",
				sample, "evidence-invalid-random",
			)
			if !errors.Is(err, ErrRetryNotApplicable) {
				t.Fatalf("invalid retry sample must be rejected before execution, got %v", err)
			}
			if adapter.calls != 0 {
				t.Fatalf("invalid retry sample must not call channel, calls=%d", adapter.calls)
			}
			delivery, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
			if getErr != nil || !found || delivery.AttemptCount != 0 || delivery.Status != domain.DeliverySending {
				t.Fatalf("invalid retry sample must not mutate delivery: delivery=%+v found=%v err=%v", delivery, found, getErr)
			}
		})
	}
}
