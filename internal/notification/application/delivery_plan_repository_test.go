package application

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type deliveryPlanKey struct {
	sourceModule         string
	eventID              string
	subscriptionID       string
	subscriptionVersion  int64
	channelRef           string
	channelConfigVersion int64
	templateID           string
	templateVersion      int64
	recipientRef         string
}

type memoryDeliveryPlans struct {
	mu    sync.Mutex
	byKey map[deliveryPlanKey]domain.DeliveryPlan
}

func newMemoryDeliveryPlans() *memoryDeliveryPlans {
	return &memoryDeliveryPlans{byKey: make(map[deliveryPlanKey]domain.DeliveryPlan)}
}

func deliveryPlanKeyFor(plan domain.DeliveryPlan) deliveryPlanKey {
	return deliveryPlanKey{
		sourceModule: plan.SourceModule, eventID: plan.EventID,
		subscriptionID: plan.SubscriptionID, subscriptionVersion: plan.SubscriptionVersion,
		channelRef: plan.ChannelRef, channelConfigVersion: plan.ChannelConfigVersion,
		templateID: plan.TemplateID, templateVersion: plan.TemplateVersion,
		recipientRef: plan.RecipientRef,
	}
}

func (m *memoryDeliveryPlans) CreateOrGet(_ context.Context, plan domain.DeliveryPlan) (domain.DeliveryPlan, bool, error) {
	if err := plan.VerifyFingerprint(); err != nil {
		return domain.DeliveryPlan{}, false, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	key := deliveryPlanKeyFor(plan)
	if existing, ok := m.byKey[key]; ok {
		if err := domain.ValidateDeliveryPlanReplay(existing, plan); err != nil {
			return domain.DeliveryPlan{}, false, err
		}
		return existing, true, nil
	}
	m.byKey[key] = plan
	return plan, false, nil
}

func validPlanForRepository() domain.DeliveryPlan {
	return domain.DeliveryPlan{
		SourceModule: "quant", EventID: "event-1",
		SubscriptionID: "sub-1", SubscriptionVersion: 2,
		ChannelRef: "channel-1", ChannelConfigVersion: 3,
		TemplateID: "template-1", TemplateVersion: 4,
		RecipientRef:                "recipient-1",
		PayloadFingerprint:          strings.Repeat("a", 64),
		AuthorizationPolicyVersions: []string{"policy-a:v1", "policy-b:v2"},
		DeliveryID:                  "delivery-1", DeliveryIdempotencyKey: "delivery-key-1",
	}
}

func TestDeliveryPlanRepositoryConcurrentIdempotency(t *testing.T) {
	plan := validPlanForRepository()
	fingerprint, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	plan.PlanFingerprint = fingerprint

	repo := newMemoryDeliveryPlans()
	const workers = 32
	var created atomic.Int64
	var duplicates atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, duplicate, err := repo.CreateOrGet(context.Background(), plan)
			if err != nil {
				t.Errorf("create delivery plan: %v", err)
				return
			}
			if duplicate {
				duplicates.Add(1)
			} else {
				created.Add(1)
			}
		}()
	}
	wg.Wait()
	if created.Load() != 1 || duplicates.Load() != workers-1 {
		t.Fatalf("expected one plan and %d duplicates, got created=%d duplicates=%d", workers-1, created.Load(), duplicates.Load())
	}
	if len(repo.byKey) != 1 {
		t.Fatalf("expected one stored plan, got %d", len(repo.byKey))
	}
}

func TestDeliveryPlanRepositoryRejectsFingerprintConflict(t *testing.T) {
	plan := validPlanForRepository()
	plan.PlanFingerprint, _ = plan.CalculateFingerprint()
	repo := newMemoryDeliveryPlans()
	if _, duplicate, err := repo.CreateOrGet(context.Background(), plan); err != nil || duplicate {
		t.Fatalf("first plan should be created, duplicate=%v err=%v", duplicate, err)
	}

	conflict := plan
	conflict.AuthorizationPolicyVersions = []string{"policy-a:v9"}
	conflict.PlanFingerprint, _ = conflict.CalculateFingerprint()
	if _, _, err := repo.CreateOrGet(context.Background(), conflict); err != domain.ErrDeliveryPlanConflict {
		t.Fatalf("expected plan conflict, got %v", err)
	}
}
