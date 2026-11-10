package application

import (
	"context"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func completionChannel() domain.ChannelConfigSnapshot {
	return domain.ChannelConfigSnapshot{
		ChannelRef: "channel-1", ConfigVersion: 1, ChannelKind: "test", Enabled: true,
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilityUnsupported,
			SupportsDeliveryLookup: domain.CapabilityUnknown,
			SupportsRetryAfter:     domain.CapabilitySupported,
			MaxPayloadBytes:        4096,
		},
		RetryableErrorCodes: []string{"rate_limited"},
	}
}

func startCompletionAttempt(t *testing.T, outcome domain.ChannelSendOutcome) (*memoryDeliveries, DeliveryClaim, domain.DeliveryAttempt, time.Time) {
	t.Helper()
	repository := newMemoryDeliveries()
	claim := claimDeliveryForAttempt(t, repository)
	attempt := attemptForClaim(claim, 1)
	if err := repository.StartWithLease(context.Background(), claim.Delivery.Version, claim.Lease.FencingToken, attempt); err != nil {
		t.Fatal(err)
	}
	finishedAt := attempt.StartedAt.Add(time.Second)
	repository.now = finishedAt.Add(time.Second)
	return repository, claim, attempt, finishedAt
}

func TestDeliveryCompletionAcceptedConvergesToSent(t *testing.T) {
	repository, claim, attempt, finishedAt := startCompletionAttempt(t, domain.ChannelOutcomeAccepted)
	service, err := NewDeliveryCompletionService(repository, repository, RetryPolicy{
		MaxAttempts: 3, MaxElapsed: time.Hour, PerAttemptTimeout: time.Minute,
		BaseDelay: time.Second, MaxDelay: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = service.Complete(context.Background(), claim, attempt, completionChannel(),
		domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAccepted, ProviderMessageRef: "provider-message-1"},
		finishedAt, 0.5, "")
	if err != nil {
		t.Fatalf("complete accepted attempt: %v", err)
	}
	got, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found || got.Status != domain.DeliverySent || got.Version != claim.Delivery.Version+1 {
		t.Fatalf("expected sent delivery, got=%+v found=%v err=%v", got, found, err)
	}
	history, err := repository.ListByDelivery(context.Background(), got.DeliveryID)
	if err != nil || len(history) != 1 || history[0].Outcome == nil || *history[0].Outcome != domain.DeliveryAttemptAccepted {
		t.Fatalf("attempt history not completed: %+v err=%v", history, err)
	}
}

func TestDeliveryCompletionAmbiguousAndNotSentAreConservative(t *testing.T) {
	for _, test := range []struct {
		name     string
		result   domain.ChannelSendResult
		evidence string
		want     domain.DeliveryStatus
	}{
		{name: "ambiguous", result: domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAmbiguous, ErrorCode: "provider_timeout"}, want: domain.DeliveryUnknown},
		{name: "not_sent", result: domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "request_not_sent"}, evidence: "proof-1", want: domain.DeliverySkipped},
	} {
		t.Run(test.name, func(t *testing.T) {
			repository, claim, attempt, finishedAt := startCompletionAttempt(t, test.result.Outcome)
			service, err := NewDeliveryCompletionService(repository, repository, RetryPolicy{
				MaxAttempts: 3, MaxElapsed: time.Hour, PerAttemptTimeout: time.Minute,
				BaseDelay: time.Second, MaxDelay: time.Minute,
			})
			if err != nil {
				t.Fatal(err)
			}
			err = service.Complete(context.Background(), claim, attempt, completionChannel(),
				test.result, finishedAt, 0.5, test.evidence)
			if err != nil {
				t.Fatalf("complete %s attempt: %v", test.name, err)
			}
			got, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
			if err != nil || !found || got.Status != test.want {
				t.Fatalf("expected %s, got=%+v found=%v err=%v", test.want, got, found, err)
			}
		})
	}
}

func TestDeliveryCompletionRetryableRejectionUsesRetryPolicy(t *testing.T) {
	repository, claim, attempt, finishedAt := startCompletionAttempt(t, domain.ChannelOutcomeRejected)
	service, err := NewDeliveryCompletionService(repository, repository, RetryPolicy{
		MaxAttempts: 3, MaxElapsed: time.Hour, PerAttemptTimeout: time.Minute,
		BaseDelay: time.Second, MaxDelay: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = service.Complete(context.Background(), claim, attempt, completionChannel(),
		domain.ChannelSendResult{Outcome: domain.ChannelOutcomeRejected, ErrorCode: "rate_limited"},
		finishedAt, 0.5, "")
	if err != nil {
		t.Fatalf("complete retryable rejection: %v", err)
	}
	got, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found || got.Status != domain.DeliveryRetryWait || got.NextAttemptAt == nil {
		t.Fatalf("expected retry_wait, got=%+v found=%v err=%v", got, found, err)
	}
}

func TestDeliveryCompletionRejectsMismatchedClaimAndMissingNotSentEvidence(t *testing.T) {
	repository, claim, attempt, finishedAt := startCompletionAttempt(t, domain.ChannelOutcomeNotSent)
	service, err := NewDeliveryCompletionService(repository, repository, RetryPolicy{
		MaxAttempts: 3, MaxElapsed: time.Hour, PerAttemptTimeout: time.Minute,
		BaseDelay: time.Second, MaxDelay: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Complete(context.Background(), claim, attempt, completionChannel(),
		domain.ChannelSendResult{Outcome: domain.ChannelOutcomeNotSent, ErrorCode: "request_not_sent"},
		finishedAt, 0.5, ""); err == nil {
		t.Fatal("not_sent without proof must be rejected")
	}
	wrongClaim := claim
	wrongClaim.Lease.WorkerID = "another-worker"
	if err := service.Complete(context.Background(), wrongClaim, attempt, completionChannel(),
		domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAccepted, ProviderMessageRef: "provider-message-1"},
		finishedAt, 0.5, ""); err == nil {
		t.Fatal("mismatched worker must be rejected")
	}
}

func TestDeliveryCompletionRejectsWrongChannelVersion(t *testing.T) {
	repository, claim, attempt, finishedAt := startCompletionAttempt(t, domain.ChannelOutcomeAccepted)
	service, err := NewDeliveryCompletionService(repository, repository, RetryPolicy{
		MaxAttempts: 3, MaxElapsed: time.Hour, PerAttemptTimeout: time.Minute,
		BaseDelay: time.Second, MaxDelay: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	wrongChannel := completionChannel()
	wrongChannel.ConfigVersion++
	err = service.Complete(context.Background(), claim, attempt, wrongChannel,
		domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAccepted, ProviderMessageRef: "provider-message-1"},
		finishedAt, 0.5, "")
	if err == nil {
		t.Fatal("completion with a different channel config version must be rejected")
	}
	got, found, getErr := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if getErr != nil || !found || got.Status != domain.DeliverySending {
		t.Fatalf("mismatched channel must not mutate delivery, got=%+v found=%v err=%v", got, found, getErr)
	}
	history, historyErr := repository.ListByDelivery(context.Background(), claim.Delivery.DeliveryID)
	if historyErr != nil || len(history) != 1 || history[0].FinishedAt != nil {
		t.Fatalf("mismatched channel must not complete attempt, history=%+v err=%v", history, historyErr)
	}
}

func TestDeliveryCompletionDeliveryCASFailureIsRecoverableFromCompletedAttempt(t *testing.T) {
	repository, claim, attempt, finishedAt := startCompletionAttempt(t, domain.ChannelOutcomeAccepted)
	service, err := NewDeliveryCompletionService(repository, repository, RetryPolicy{
		MaxAttempts: 3, MaxElapsed: time.Hour, PerAttemptTimeout: time.Minute,
		BaseDelay: time.Second, MaxDelay: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Attempt 完成写入成功后，模拟 Delivery CAS 失败。
	repository.failNext = true
	err = service.Complete(context.Background(), claim, attempt, completionChannel(),
		domain.ChannelSendResult{Outcome: domain.ChannelOutcomeAccepted, ProviderMessageRef: "provider-message-persisted"},
		finishedAt, 0.5, "")
	if err == nil {
		t.Fatal("expected Delivery CAS failure")
	}

	current, found, err := repository.Get(context.Background(), claim.Delivery.DeliveryID)
	if err != nil || !found || current.Status != domain.DeliverySending || current.AttemptCount != 1 {
		t.Fatalf("failed Delivery CAS must leave sending snapshot and started attempt count: delivery=%+v found=%v err=%v", current, found, err)
	}
	history, err := repository.ListByDelivery(context.Background(), current.DeliveryID)
	if err != nil || len(history) != 1 || history[0].FinishedAt == nil ||
		history[0].Outcome == nil || *history[0].Outcome != domain.DeliveryAttemptAccepted {
		t.Fatalf("completed accepted Attempt must remain durable for recovery: history=%+v err=%v", history, err)
	}

	assessment, err := AssessExpiredDeliveryLease(current, claim.Lease, history, claim.Lease.ExpiresAt)
	if err != nil || assessment.Action != LeaseRecoveryFinalizeSent ||
		assessment.ProviderMessageRef != "provider-message-persisted" {
		t.Fatalf("recovery must finalize sent from immutable Attempt without resending: assessment=%+v err=%v", assessment, err)
	}
}
