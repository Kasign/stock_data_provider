package application

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestDeliveryPreflightCompletionPersistsSkipAndReleasesLease(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	service, err := NewDeliveryPreflightCompletionService(repository)
	if err != nil { t.Fatal(err) }
	at := claim.Delivery.UpdatedAt.Add(time.Second)
	err = service.Apply(context.Background(), claim, DeliveryPreflightResult{
		Action: DeliveryPreflightSkip, ReasonCode: "recipient_not_authorized",
	}, at, "preflight-proof-1")
	if err != nil { t.Fatal(err) }
	got, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found || got.Status != domain.DeliverySkipped || got.Version != claim.Delivery.Version+1 {
		t.Fatalf("expected skipped delivery: %+v found=%v err=%v", got, found, err)
	}
	if len(repository.StateEvents(got.DeliveryID)) != 2 {
		t.Fatal("preflight decision must append a state event")
	}
}

func TestDeliveryPreflightCompletionPersistsBlock(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	service, err := NewDeliveryPreflightCompletionService(repository)
	if err != nil { t.Fatal(err) }
	at := claim.Delivery.UpdatedAt.Add(time.Second)
	err = service.Apply(context.Background(), claim, DeliveryPreflightResult{
		Action: DeliveryPreflightBlock, ReasonCode: "channel_disabled",
	}, at, "preflight-proof-2")
	if err != nil { t.Fatal(err) }
	got, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found || got.Status != domain.DeliveryBlocked {
		t.Fatalf("expected blocked delivery: %+v found=%v err=%v", got, found, err)
	}
}

func TestDeliveryPreflightCompletionReadyDoesNotMutateDelivery(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	service, err := NewDeliveryPreflightCompletionService(repository)
	if err != nil { t.Fatal(err) }
	if err := service.Apply(context.Background(), claim, DeliveryPreflightResult{
		Action: DeliveryPreflightReady, ReasonCode: "preflight_passed",
	}, claim.Delivery.UpdatedAt.Add(time.Second), ""); err != nil {
		t.Fatal(err)
	}
	got, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found || got.Status != domain.DeliverySending || got.Version != claim.Delivery.Version {
		t.Fatalf("ready result must leave delivery unchanged: %+v found=%v err=%v", got, found, err)
	}
}

func TestDeliveryPreflightCompletionRequiresEvidenceAndRejectsExpiredLease(t *testing.T) {
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	service, err := NewDeliveryPreflightCompletionService(repository)
	if err != nil { t.Fatal(err) }
	result := DeliveryPreflightResult{Action: DeliveryPreflightSkip, ReasonCode: "subscription_inactive"}
	if err := service.Apply(context.Background(), claim, result, claim.Delivery.UpdatedAt.Add(time.Second), ""); err == nil {
		t.Fatal("skip without evidence must fail")
	}
	if err := service.Apply(context.Background(), claim, result, claim.Lease.ExpiresAt, "proof-expired"); err == nil {
		t.Fatal("expired lease must fail")
	}
}
