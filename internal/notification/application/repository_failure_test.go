package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var errInjectedRepositoryFailure = errors.New("notification: injected repository failure")

func TestInboxRepositoryFailureDoesNotPartiallyAccept(t *testing.T) {
	now := fixedClock{now: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	event := domain.EventEnvelope{
		SourceModule: "quant", EventID: "event-failure",
		IdempotencyKey: "publish-failure", PayloadFingerprint: "fingerprint",
	}
	repository := newMemoryInbox()
	repository.failNextAccept = true
	receiver, err := NewEventReceiver(staticValidator{event: event}, repository, now)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := receiver.Receive(context.Background(), []byte("{}")); !errors.Is(err, errInjectedRepositoryFailure) {
		t.Fatalf("expected injected inbox failure, got %v", err)
	}
	if len(repository.byEvent) != 0 || len(repository.byIdempotency) != 0 || repository.fanoutWorkItems != 0 {
		t.Fatal("failed inbox accept must not leave a partial event or fan-out item")
	}

	result, err := receiver.Receive(context.Background(), []byte("{}"))
	if err != nil || result.Duplicate {
		t.Fatalf("retry after transient failure should accept once, result=%+v err=%v", result, err)
	}
	if len(repository.byEvent) != 1 || repository.fanoutWorkItems != 1 {
		t.Fatalf("expected one accepted event and one fan-out item, got events=%d fanout=%d", len(repository.byEvent), repository.fanoutWorkItems)
	}
}

func TestSubscriptionRepositoryFailureDoesNotPartiallyCreateOrAppend(t *testing.T) {
	repository := newMemorySubscriptions()
	ctx := context.Background()
	initial := validApplicationSubscription()

	repository.failNextCreate = true
	if err := repository.Create(ctx, initial); !errors.Is(err, errInjectedRepositoryFailure) {
		t.Fatalf("expected injected create failure, got %v", err)
	}
	if _, found, err := repository.GetLatest(ctx, initial.SubscriptionID); err != nil || found {
		t.Fatalf("failed create must not persist a partial subscription, found=%v err=%v", found, err)
	}
	if err := repository.Create(ctx, initial); err != nil {
		t.Fatalf("create retry should succeed: %v", err)
	}

	next := nextSubscriptionVersion(initial)
	repository.failNextAppend = true
	if err := repository.AppendVersion(ctx, 1, next); !errors.Is(err, errInjectedRepositoryFailure) {
		t.Fatalf("expected injected append failure, got %v", err)
	}
	latest, found, err := repository.GetLatest(ctx, initial.SubscriptionID)
	if err != nil || !found || latest.Version != 1 {
		t.Fatalf("failed append must preserve version 1, latest=%+v found=%v err=%v", latest, found, err)
	}
	if err := repository.AppendVersion(ctx, 1, next); err != nil {
		t.Fatalf("append retry should succeed: %v", err)
	}
	latest, found, err = repository.GetLatest(ctx, initial.SubscriptionID)
	if err != nil || !found || latest.Version != 2 {
		t.Fatalf("expected successful version-2 retry, latest=%+v found=%v err=%v", latest, found, err)
	}
}

func TestDeliveryPlanRepositoryFailureDoesNotPartiallyCreate(t *testing.T) {
	repository := newMemoryDeliveryPlans()
	repository.failNextCreate = true
	writer, err := NewDeliveryPlanWriter(repository)
	if err != nil {
		t.Fatal(err)
	}
	plan := domain.DeliveryPlan{
		SourceModule: "quant", EventID: "event-failure",
		SubscriptionID: "sub-failure", SubscriptionVersion: 1,
		ChannelRef: "channel-failure", ChannelConfigVersion: 1,
		TemplateID: "template-failure", TemplateVersion: 1,
		RecipientRef: "recipient-failure", PayloadFingerprint: strings.Repeat("a", 64),
		AuthorizationPolicyVersions: []string{"policy:v1"},
		DeliveryID:                  "delivery-failure", DeliveryIdempotencyKey: "delivery-key-failure",
	}
	plan.PlanFingerprint, err = plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := writer.CreateOrGet(context.Background(), plan); !errors.Is(err, errInjectedRepositoryFailure) {
		t.Fatalf("expected injected plan persistence failure, got %v", err)
	}
	if len(repository.byKey) != 0 {
		t.Fatal("failed plan create must not leave a partial delivery plan")
	}
	if _, duplicate, err := writer.CreateOrGet(context.Background(), plan); err != nil || duplicate {
		t.Fatalf("retry after transient failure should create the plan, duplicate=%v err=%v", duplicate, err)
	}
	if len(repository.byKey) != 1 {
		t.Fatalf("expected one persisted delivery plan, got %d", len(repository.byKey))
	}
}
