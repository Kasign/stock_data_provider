package application

import (
	"context"
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

func executionFixture(t *testing.T) (*DeliveryExecutionService, *memoryDeliveries, *fakeChannelAdapter, DeliveryClaim) {
	t.Helper()
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)

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
	return service, repository, adapter, claim
}

func TestDeliveryExecutionCallsChannelOnlyAfterAttemptStarts(t *testing.T) {
	service, repository, adapter, claim := executionFixture(t)
	repository.now = claim.Lease.ExpiresAt.Add(-time.Second)

	result, err := service.Execute(
		context.Background(), claim, domain.EventEnvelope{}, "attempt-execution-1",
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
	service, repository, adapter, claim := executionFixture(t)
	repository.failAttemptNext = true
	repository.now = claim.Lease.ExpiresAt.Add(-time.Second)

	_, err := service.Execute(
		context.Background(), claim, domain.EventEnvelope{}, "attempt-execution-2",
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
