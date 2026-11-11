package application

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type fanoutTestWorkItems struct {
	claim    FanoutClaim
	claimed  bool
	advanced bool
	retried  bool
	terminal bool
	cursor   string
	next     time.Time
}

func (m *fanoutTestWorkItems) ClaimDue(context.Context, string, time.Duration) (FanoutClaim, bool, error) {
	return m.claim, m.claimed, nil
}

func (m *fanoutTestWorkItems) AdvanceWithLease(_ context.Context, _ FanoutClaim, cursor string, completed bool) error {
	m.advanced, m.cursor = true, cursor
	m.terminal = completed
	return nil
}

func (m *fanoutTestWorkItems) RetryWithLease(_ context.Context, _ FanoutClaim, next time.Time, _ string, terminal bool) error {
	m.retried, m.next, m.terminal = true, next, terminal
	return nil
}

type fanoutTestSubscriptions struct {
	values []domain.Subscription
	err    error
}

func (m fanoutTestSubscriptions) ListActiveAtRevision(context.Context, string, int64, string, int) ([]domain.Subscription, error) {
	return m.values, m.err
}

type fanoutTestFactory struct {
	decisions map[string]FanoutPlanDecision
	err       error
}

func (m fanoutTestFactory) Build(_ context.Context, _ domain.EventEnvelope, subscription domain.Subscription) (FanoutPlanDecision, error) {
	if m.err != nil {
		return FanoutPlanDecision{}, m.err
	}
	return m.decisions[subscription.SubscriptionID], nil
}

type fanoutTestClock struct {
	now time.Time
}

func (c fanoutTestClock) Now() time.Time {
	return c.now
}

type fanoutTestPlans struct {
	created   int
	duplicate bool
	err       error
}

func (p *fanoutTestPlans) CreateOrGet(_ context.Context, plan domain.DeliveryPlan) (domain.DeliveryPlan, bool, error) {
	if p.err != nil {
		return domain.DeliveryPlan{}, false, p.err
	}
	return plan, p.duplicate, nil
}

func validFanoutClaim(now time.Time) FanoutClaim {
	event := domain.EventEnvelope{
		SourceModule:  "quant",
		EventID:       "event-1",
		EventType:     "quant.risk_decision.changed",
		SchemaVersion: "1.0.0",
		SubjectRef:    "stock:600000",
		OccurredAt:    "2026-11-11T09:00:00Z",
		Payload:       []byte(`{"decision":"watch"}`),
	}
	event.PayloadFingerprint, _ = event.CalculatePayloadFingerprint()
	return FanoutClaim{
		WorkItem: domain.FanoutWorkItem{
			WorkItemID:         domain.FanoutWorkItemID("quant", "event-1"),
			SourceModule:       "quant",
			EventID:            "event-1",
			EventType:          event.EventType,
			PayloadFingerprint: event.PayloadFingerprint,
			SnapshotRevision:   4,
			HasSnapshot:        true,
			Status:             domain.FanoutProcessing,
			AttemptCount:       1,
			LeaseWorkerID:      "worker-1",
			LeaseExpiresAt:     now.Add(time.Minute),
			FencingToken:       1,
			CreatedAt:          now,
			UpdatedAt:          now,
		},
		Event: event,
	}
}

func validFanoutPlan() domain.DeliveryPlan {
	plan := domain.DeliveryPlan{
		SourceModule:           "quant",
		EventID:                "event-1",
		SubscriptionID:         "sub-1",
		SubscriptionVersion:    1,
		ChannelRef:             "channel-1",
		ChannelConfigVersion:   1,
		TemplateID:             "template-1",
		TemplateVersion:        1,
		RecipientRef:           "recipient-1",
		PayloadFingerprint:     strings.Repeat("a", 64),
		DeliveryID:             "delivery-1",
		DeliveryIdempotencyKey: "delivery-key-1",
	}
	plan.PlanFingerprint, _ = plan.CalculateFingerprint()
	return plan
}

func TestFanoutDispatcherAdvancesOnlyAfterPagePlansPersist(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	claim := validFanoutClaim(now)
	work := &fanoutTestWorkItems{claim: claim, claimed: true}
	subscriptions := fanoutTestSubscriptions{
		values: []domain.Subscription{
			{SubscriptionID: "sub-1"},
			{SubscriptionID: "sub-2"},
		},
	}
	plans, _ := NewDeliveryPlanWriter(newMemoryDeliveryPlans())
	factory := fanoutTestFactory{
		decisions: map[string]FanoutPlanDecision{
			"sub-1": {Matched: true, Plan: validFanoutPlan()},
			"sub-2": {Matched: false},
		},
	}
	dispatcher, err := NewFanoutDispatcher(work, subscriptions, factory, plans, fanoutTestClock{now}, FanoutDispatcherConfig{
		PageSize:      2,
		LeaseDuration: time.Minute,
		Retry: FanoutRetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   time.Second,
			MaxDelay:    time.Minute,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := dispatcher.RunOnce(context.Background(), "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if !report.Claimed || !report.Completed || report.Processed != 2 || report.PlansCreated != 1 ||
		!work.advanced || work.cursor != "sub-2" || !work.terminal {
		t.Fatalf("unexpected fanout result: report=%+v work=%+v", report, work)
	}
}

func TestFanoutDispatcherDoesNotAdvanceCursorWhenPlanWriteFails(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	work := &fanoutTestWorkItems{claim: validFanoutClaim(now), claimed: true}
	subscriptions := fanoutTestSubscriptions{
		values: []domain.Subscription{{SubscriptionID: "sub-1"}},
	}
	plans, _ := NewDeliveryPlanWriter(&fanoutTestPlans{
		err: errors.New("transient plan persistence failure"),
	})
	factory := fanoutTestFactory{
		decisions: map[string]FanoutPlanDecision{
			"sub-1": {Matched: true, Plan: validFanoutPlan()},
		},
	}
	dispatcher, err := NewFanoutDispatcher(work, subscriptions, factory, plans, fanoutTestClock{now}, FanoutDispatcherConfig{
		PageSize:      1,
		LeaseDuration: time.Minute,
		Retry: FanoutRetryPolicy{
			MaxAttempts: 3,
			BaseDelay:   time.Second,
			MaxDelay:    time.Minute,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	report, err := dispatcher.RunOnce(context.Background(), "worker-1")
	if err != nil {
		t.Fatal(err)
	}
	if !report.RetryScheduled || work.advanced || !work.retried || work.terminal ||
		!work.next.Equal(now.Add(time.Second)) {
		t.Fatalf("failed page must retry without advancing cursor: report=%+v work=%+v", report, work)
	}
}

func TestFanoutDispatcherRejectsMissingPlanFactoryAndInvalidConfiguration(t *testing.T) {
	plans, _ := NewDeliveryPlanWriter(&memoryDeliveryPlans{})
	_, err := NewFanoutDispatcher(
		&fanoutTestWorkItems{},
		fanoutTestSubscriptions{},
		nil,
		plans,
		fanoutTestClock{time.Now().UTC()},
		FanoutDispatcherConfig{
			PageSize:      1,
			LeaseDuration: time.Minute,
			Retry: FanoutRetryPolicy{
				MaxAttempts: 1,
				BaseDelay:   time.Second,
				MaxDelay:    time.Second,
			},
		},
	)
	if !errors.Is(err, ErrFanoutConfigurationInvalid) {
		t.Fatalf("expected invalid configuration, got %v", err)
	}
}
