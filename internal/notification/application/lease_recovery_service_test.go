package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

type recoveryScannerFake struct {
	claims []DeliveryClaim
	limit  int
	err    error
}

func (f *recoveryScannerFake) ListExpiredLeases(_ context.Context, limit int) ([]DeliveryClaim, error) {
	f.limit = limit
	return append([]DeliveryClaim(nil), f.claims...), f.err
}

type recoveryAttemptsFake struct {
	attempts []domain.DeliveryAttempt
	err      error
}

func (f *recoveryAttemptsFake) StartWithLease(context.Context, int64, int64, domain.DeliveryAttempt) error {
	return errors.New("unexpected StartWithLease")
}
func (f *recoveryAttemptsFake) CompleteWithLease(context.Context, int64, int64, domain.DeliveryAttempt) error {
	return errors.New("unexpected CompleteWithLease")
}
func (f *recoveryAttemptsFake) ListByDelivery(context.Context, string) ([]domain.DeliveryAttempt, error) {
	return append([]domain.DeliveryAttempt(nil), f.attempts...), f.err
}

type recoveryRepositoryFake struct {
	expectedVersion int64
	fencingToken    int64
	next            domain.DeliveryRecord
	event           domain.DeliveryStateEvent
	now             time.Time
	calls           int
	err             error
}

func (f *recoveryRepositoryFake) RecoverExpiredLease(_ context.Context, version, token int64, next domain.DeliveryRecord, event domain.DeliveryStateEvent, now time.Time) error {
	f.calls++
	f.expectedVersion, f.fencingToken = version, token
	f.next, f.event, f.now = next, event, now
	return f.err
}

func recoveryServiceFixture(t *testing.T, attempts []domain.DeliveryAttempt) (*LeaseRecoveryService, DeliveryClaim, time.Time, *recoveryRepositoryFake) {
	t.Helper()
	delivery, lease, now := recoveryFixture(t)
	scanner := &recoveryScannerFake{claims: []DeliveryClaim{{Delivery: delivery, Lease: lease}}}
	attemptStore := &recoveryAttemptsFake{attempts: attempts}
	repository := &recoveryRepositoryFake{}
	service := &LeaseRecoveryService{
		Scanner: scanner, Attempts: attemptStore, Repository: repository,
		RetryPolicy: RetryPolicy{
			MaxAttempts: delivery.MaxAttempts, MaxElapsed: time.Hour,
			PerAttemptTimeout: time.Minute, BaseDelay: time.Second, MaxDelay: time.Minute,
		},
		RandomUnit: func() float64 { return 0.5 },
	}
	return service, scanner.claims[0], now, repository
}

func TestLeaseRecoveryServiceRecoversWithoutStartingAttempt(t *testing.T) {
	service, claim, now, repository := recoveryServiceFixture(t, nil)
	report, err := service.RecoverExpiredOnce(context.Background(), now, 10)
	if err != nil || report != (LeaseRecoveryReport{Scanned: 1, Recovered: 1}) {
		t.Fatalf("unexpected recovery report: %+v err=%v", report, err)
	}
	if repository.calls != 1 || repository.expectedVersion != claim.Delivery.Version ||
		repository.fencingToken != claim.Lease.FencingToken ||
		repository.next.Status != domain.DeliveryRetryWait ||
		repository.next.NextAttemptAt == nil ||
		!repository.event.ChangedAt.Equal(now) {
		t.Fatalf("safe pre-attempt recovery was not committed: %+v", repository)
	}
}

func TestLeaseRecoveryServiceFinalizesAcceptedAttempt(t *testing.T) {
	delivery, lease, now := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAccepted, true)
	delivery.AttemptCount = 1
	service, claim, now, repository := recoveryServiceFixture(t, []domain.DeliveryAttempt{attempt})
	claim.Delivery = delivery
	service.Scanner.(*recoveryScannerFake).claims[0] = claim

	report, err := service.RecoverExpiredOnce(context.Background(), now, 5)
	if err != nil || report.Recovered != 1 || repository.next.Status != domain.DeliverySent {
		t.Fatalf("accepted Attempt must finalize sent: report=%+v next=%+v err=%v", report, repository.next, err)
	}
}

func TestLeaseRecoveryServiceMarksAmbiguousAttemptUnknown(t *testing.T) {
	delivery, lease, _ := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptAmbiguous, true)
	delivery.AttemptCount = 1
	service, claim, now, repository := recoveryServiceFixture(t, []domain.DeliveryAttempt{attempt})
	claim.Delivery = delivery
	service.Scanner.(*recoveryScannerFake).claims[0] = claim

	report, err := service.RecoverExpiredOnce(context.Background(), now, 5)
	if err != nil || report.Recovered != 1 || repository.next.Status != domain.DeliveryUnknown {
		t.Fatalf("ambiguous Attempt must become unknown: report=%+v next=%+v err=%v", report, repository.next, err)
	}
	if repository.event.EvidenceRef == nil || *repository.event.EvidenceRef != attempt.AttemptID {
		t.Fatalf("unknown transition must retain Attempt evidence: %+v", repository.event)
	}
}

func TestLeaseRecoveryServiceSchedulesRetryableRejection(t *testing.T) {
	delivery, lease, _ := recoveryFixture(t)
	attempt := recoveryAttempt(t, delivery, lease, domain.DeliveryAttemptRejected, true)
	delivery.AttemptCount = 1
	service, claim, now, repository := recoveryServiceFixture(t, []domain.DeliveryAttempt{attempt})
	claim.Delivery = delivery
	service.Scanner.(*recoveryScannerFake).claims[0] = claim

	report, err := service.RecoverExpiredOnce(context.Background(), now, 5)
	if err != nil || report.Recovered != 1 || repository.next.Status != domain.DeliveryRetryWait ||
		repository.next.NextAttemptAt == nil || *repository.next.NextAttemptAt == now {
		t.Fatalf("retryable rejection must use RetryPolicy: report=%+v next=%+v err=%v", report, repository.next, err)
	}
}

func TestLeaseRecoveryServiceCountsCASConflictsAndBoundsScan(t *testing.T) {
	service, _, now, repository := recoveryServiceFixture(t, nil)
	repository.err = ErrDeliveryLeaseConflict
	scanner := service.Scanner.(*recoveryScannerFake)
	report, err := service.RecoverExpiredOnce(context.Background(), now, 7)
	if err != nil || report.Scanned != 1 || report.Conflicted != 1 || report.Recovered != 0 || scanner.limit != 7 {
		t.Fatalf("CAS conflicts should be counted without aborting the batch: %+v err=%v", report, err)
	}
}

func TestLeaseRecoveryServiceRejectsInvalidBatchAndScannerOverflow(t *testing.T) {
	service, claim, now, _ := recoveryServiceFixture(t, nil)
	if _, err := service.RecoverExpiredOnce(context.Background(), now, 0); !errors.Is(err, ErrLeaseRecoveryServiceConfig) {
		t.Fatalf("zero limit must be rejected, got %v", err)
	}
	scanner := service.Scanner.(*recoveryScannerFake)
	scanner.claims = []DeliveryClaim{claim, claim}
	if _, err := service.RecoverExpiredOnce(context.Background(), now, 1); !errors.Is(err, ErrLeaseRecoveryHistory) {
		t.Fatalf("scanner over-return must fail closed, got %v", err)
	}
}
