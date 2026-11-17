package application

import (
	"context"
	"testing"
	"time"
)

type outboxDeliveryRepoFake struct {
	claimed                                                      []NotificationEventEnvelope
	claimWorker                                                  string
	claimLimit                                                   int
	leaseDuration                                                time.Duration
	confirmedEvent, confirmedToken, confirmedWorker              string
	retriedEvent, retriedToken, retriedWorker                    string
	retryDelay                                                   time.Duration
	retryReason                                                  string
	rejectedEvent, rejectedToken, rejectedWorker, rejectedReason string
	err                                                          error
}

func (f *outboxDeliveryRepoFake) ClaimOutbox(_ context.Context, worker string, limit int, lease time.Duration) ([]NotificationEventEnvelope, error) {
	f.claimWorker, f.claimLimit, f.leaseDuration = worker, limit, lease
	return f.claimed, f.err
}

func (f *outboxDeliveryRepoFake) MarkOutboxPublished(_ context.Context, event, worker, token string) error {
	f.confirmedEvent, f.confirmedWorker, f.confirmedToken = event, worker, token
	return f.err
}

func (f *outboxDeliveryRepoFake) ScheduleOutboxRetry(_ context.Context, event, worker, token string, delay time.Duration, reason string) error {
	f.retriedEvent, f.retriedWorker, f.retriedToken = event, worker, token
	f.retryDelay, f.retryReason = delay, reason
	return f.err
}

func (f *outboxDeliveryRepoFake) RejectOutbox(_ context.Context, event, worker, token, reason string) error {
	f.rejectedEvent, f.rejectedWorker, f.rejectedToken, f.rejectedReason = event, worker, token, reason
	return f.err
}

func TestOutboxServiceUsesStableLeaseAndDeliveryCommands(t *testing.T) {
	repo := &outboxDeliveryRepoFake{}
	service, err := NewOutboxService(repo)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Claim(context.Background(), "worker_1", 10, 30*time.Second); err != nil {
		t.Fatal(err)
	}
	if repo.claimWorker != "worker_1" || repo.claimLimit != 10 || repo.leaseDuration != 30*time.Second {
		t.Fatalf("unexpected claim request: %+v", repo)
	}

	token := "0123456789abcdef0123456789abcdef"
	if err := service.Confirm(context.Background(), "nte_1", "worker_1", token); err != nil {
		t.Fatal(err)
	}
	if repo.confirmedEvent != "nte_1" || repo.confirmedToken != token {
		t.Fatalf("unexpected confirmation: %+v", repo)
	}
	if err := service.Retry(context.Background(), "nte_2", "worker_1", token, time.Minute, "notification_unavailable"); err != nil {
		t.Fatal(err)
	}
	if repo.retryReason != "notification_unavailable" || repo.retriedToken != token || repo.retryDelay != time.Minute {
		t.Fatalf("unexpected retry request: %+v", repo)
	}
	if err := service.Reject(context.Background(), "nte_3", "worker_1", token, "notification_schema_rejected"); err != nil {
		t.Fatal(err)
	}
	if repo.rejectedEvent != "nte_3" || repo.rejectedToken != token || repo.rejectedReason != "notification_schema_rejected" {
		t.Fatalf("unexpected rejection request: %+v", repo)
	}
}

func TestOutboxServiceRejectsUnsafeClaimAndRetryParameters(t *testing.T) {
	service, err := NewOutboxService(&outboxDeliveryRepoFake{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Claim(context.Background(), "bad worker", 10, time.Minute); err == nil {
		t.Fatal("unsafe worker ID must be rejected")
	}
	if _, err := service.Claim(context.Background(), "worker_1", 101, time.Minute); err == nil {
		t.Fatal("oversized batch must be rejected")
	}
	if _, err := service.Claim(context.Background(), "worker_1", 10, 0); err == nil {
		t.Fatal("zero lease must be rejected")
	}
	token := "0123456789abcdef0123456789abcdef"
	if err := service.Retry(context.Background(), "nte_1", "worker_1", token, time.Minute+time.Nanosecond, "notification_unavailable"); err == nil {
		t.Fatal("sub-microsecond retry delay must be rejected")
	}
	if err := service.Retry(context.Background(), "nte_1", "worker_1", token, time.Minute, "invalid reason"); err == nil {
		t.Fatal("unsafe retry reason must be rejected")
	}
}
