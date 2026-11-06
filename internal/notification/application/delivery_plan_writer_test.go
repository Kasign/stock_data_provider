package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type countingDeliveryPlanRepository struct {
	calls int
}

func (r *countingDeliveryPlanRepository) CreateOrGet(_ context.Context, plan domain.DeliveryPlan) (domain.DeliveryPlan, bool, error) {
	r.calls++
	return plan, false, nil
}

func validPlanForWriter() domain.DeliveryPlan {
	plan := domain.DeliveryPlan{
		SourceModule: "quant", EventID: "event-1",
		SubscriptionID: "sub-1", SubscriptionVersion: 1,
		ChannelRef: "channel-1", ChannelConfigVersion: 1,
		TemplateID: "template-1", TemplateVersion: 1,
		RecipientRef: "recipient-1", PayloadFingerprint: strings.Repeat("a", 64),
		AuthorizationPolicyVersions: []string{"policy:v1"},
		DeliveryID: "delivery-1", DeliveryIdempotencyKey: "delivery-key-1",
	}
	plan.PlanFingerprint, _ = plan.CalculateFingerprint()
	return plan
}

func TestDeliveryPlanWriterValidatesBeforePersistence(t *testing.T) {
	repository := &countingDeliveryPlanRepository{}
	writer, err := NewDeliveryPlanWriter(repository)
	if err != nil {
		t.Fatal(err)
	}

	invalidStructure := validPlanForWriter()
	invalidStructure.RecipientRef = ""
	if _, _, err := writer.CreateOrGet(context.Background(), invalidStructure); !errors.Is(err, domain.ErrInvalidDeliveryPlan) {
		t.Fatalf("expected structural validation error, got %v", err)
	}
	if repository.calls != 0 {
		t.Fatal("repository must not be called for invalid structure")
	}

	invalidFingerprint := validPlanForWriter()
	invalidFingerprint.PlanFingerprint = strings.Repeat("0", 64)
	if _, _, err := writer.CreateOrGet(context.Background(), invalidFingerprint); !errors.Is(err, domain.ErrInvalidDeliveryPlan) {
		t.Fatalf("expected fingerprint validation error, got %v", err)
	}
	if repository.calls != 0 {
		t.Fatal("repository must not be called for invalid fingerprint")
	}

	valid := validPlanForWriter()
	if _, duplicate, err := writer.CreateOrGet(context.Background(), valid); err != nil || duplicate {
		t.Fatalf("valid plan should persist, duplicate=%v err=%v", duplicate, err)
	}
	if repository.calls != 1 {
		t.Fatalf("expected one persistence call, got %d", repository.calls)
	}
}
