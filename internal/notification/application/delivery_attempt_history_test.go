package application

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestValidateDeliveryAttemptHistoryAcceptsRunningLastAttempt(t *testing.T) {
	delivery, lease, _ := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, false)
	if err := ValidateDeliveryAttemptHistory(delivery.DeliveryID, 1, []domain.DeliveryAttempt{attempt}); err != nil {
		t.Fatalf("running final attempt is valid history: %v", err)
	}
}

func TestValidateDeliveryAttemptHistoryRejectsImpossibleHistory(t *testing.T) {
	delivery, lease, _ := recoveryFixture(t)
	first := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, true)
	secondLease := DeliveryLease{WorkerID: "worker-2", FencingToken: 2, ExpiresAt: lease.ExpiresAt.Add(time.Minute)}
	second := domain.DeliveryAttempt{
		AttemptID: "attempt-2", DeliveryID: delivery.DeliveryID, AttemptNo: 2,
		WorkerID: secondLease.WorkerID, FencingToken: secondLease.FencingToken,
		LeaseExpiresAt: secondLease.ExpiresAt, StartedAt: first.FinishedAt.Add(time.Second),
	}
	cases := []struct {
		name string
		list []domain.DeliveryAttempt
	}{
		{name: "attempt count mismatch", list: []domain.DeliveryAttempt{first}},
		{name: "terminal outcome followed by another attempt", list: []domain.DeliveryAttempt{first, second}},
		{name: "duplicate attempt id", list: []domain.DeliveryAttempt{first, func() domain.DeliveryAttempt { value := second; value.AttemptID = first.AttemptID; return value }()}},
		{name: "non-increasing fencing token", list: []domain.DeliveryAttempt{first, func() domain.DeliveryAttempt { value := second; value.FencingToken = first.FencingToken; return value }()}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := ValidateDeliveryAttemptHistory(delivery.DeliveryID, 2, tc.list); !errors.Is(err, ErrDeliveryAttemptHistory) {
				t.Fatalf("expected history rejection, got %v", err)
			}
		})
	}
}

func TestValidateDeliveryAttemptHistoryForDeliveryRejectsOutsideLifecycle(t *testing.T) {
	delivery, lease, _ := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, true)
	if err := ValidateDeliveryAttemptHistoryForDelivery(delivery, []domain.DeliveryAttempt{attempt}); err != nil {
		t.Fatalf("valid attempt history rejected: %v", err)
	}

	beforeCreation := attempt
	beforeCreation.StartedAt = delivery.CreatedAt.Add(-time.Second)
	beforeCreation.LeaseExpiresAt = delivery.CreatedAt.Add(time.Minute)
	if err := ValidateDeliveryAttemptHistoryForDelivery(delivery, []domain.DeliveryAttempt{beforeCreation}); !errors.Is(err, ErrDeliveryAttemptHistory) {
		t.Fatalf("attempt before Delivery creation must fail closed: %v", err)
	}

	afterDeadline := attempt
	afterDeadline.StartedAt = delivery.DeadlineAt
	afterDeadline.LeaseExpiresAt = delivery.DeadlineAt.Add(time.Minute)
	if err := ValidateDeliveryAttemptHistoryForDelivery(delivery, []domain.DeliveryAttempt{afterDeadline}); !errors.Is(err, ErrDeliveryAttemptHistory) {
		t.Fatalf("attempt at/after Delivery deadline must fail closed: %v", err)
	}

	leaseBeyondDeadline := attempt
	leaseBeyondDeadline.LeaseExpiresAt = delivery.DeadlineAt.Add(time.Second)
	if err := ValidateDeliveryAttemptHistoryForDelivery(delivery, []domain.DeliveryAttempt{leaseBeyondDeadline}); !errors.Is(err, ErrDeliveryAttemptHistory) {
		t.Fatalf("attempt lease beyond Delivery deadline must fail closed: %v", err)
	}
}
