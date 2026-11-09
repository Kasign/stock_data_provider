package application

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func channelResultFixture() (domain.DeliveryAttempt, domain.ChannelConfigSnapshot, time.Time) {
	startedAt := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	attempt := domain.DeliveryAttempt{
		AttemptID: "attempt-channel-1", DeliveryID: "delivery-channel-1",
		AttemptNo: 1, WorkerID: "worker-channel-1", FencingToken: 1,
		LeaseExpiresAt: startedAt.Add(time.Minute), StartedAt: startedAt,
	}
	channel := domain.ChannelConfigSnapshot{
		ChannelRef: "channel-1", ConfigVersion: 1, ChannelKind: "test",
		Enabled: true, SecretRef: "secret://channel-1/v1",
		Capabilities: domain.ChannelCapabilities{
			SupportsIdempotency:    domain.CapabilityUnknown,
			SupportsDeliveryLookup: domain.CapabilityUnknown,
			SupportsRetryAfter:     domain.CapabilitySupported,
			MaxPayloadBytes:        4096,
		},
		RetryableErrorCodes: []string{"rate_limited"},
	}
	return attempt, channel, startedAt.Add(time.Second)
}

func TestCompleteAttemptFromChannelResultUsesChannelRetryableAllowlist(t *testing.T) {
	attempt, channel, finishedAt := channelResultFixture()
	retryAfter := finishedAt.Add(30 * time.Second)
	result := domain.ChannelSendResult{
		Outcome:    domain.ChannelOutcomeRejected,
		ErrorCode:  "rate_limited",
		RetryAfter: &retryAfter,
	}
	completed, err := CompleteAttemptFromChannelResult(attempt, channel, result, finishedAt, "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Outcome == nil || *completed.Outcome != domain.DeliveryAttemptRejected ||
		!completed.Retryable || completed.RetryAfterAt == nil ||
		!completed.RetryAfterAt.Equal(retryAfter) {
		t.Fatalf("whitelisted rejection and Retry-After must be retained: %+v", completed)
	}

	result.ErrorCode = "invalid_payload"
	completed, err = CompleteAttemptFromChannelResult(attempt, channel, result, finishedAt, "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Retryable {
		t.Fatalf("error outside channel retryable allowlist must not be retried: %+v", completed)
	}
}

func TestCompleteAttemptFromChannelResultNeverRetriesAmbiguousAndRequiresNotSentEvidence(t *testing.T) {
	attempt, channel, finishedAt := channelResultFixture()
	ambiguous := domain.ChannelSendResult{
		Outcome:   domain.ChannelOutcomeAmbiguous,
		ErrorCode: "request_timeout",
	}
	completed, err := CompleteAttemptFromChannelResult(attempt, channel, ambiguous, finishedAt, "")
	if err != nil {
		t.Fatal(err)
	}
	if completed.Retryable {
		t.Fatalf("ambiguous result must not become retryable: %+v", completed)
	}

	notSent := domain.ChannelSendResult{
		Outcome:   domain.ChannelOutcomeNotSent,
		ErrorCode: "connection_not_opened",
	}
	if _, err := CompleteAttemptFromChannelResult(attempt, channel, notSent, finishedAt, ""); err == nil {
		t.Fatal("not_sent requires an evidence reference")
	}
	completed, err = CompleteAttemptFromChannelResult(attempt, channel, notSent, finishedAt, "evidence-not-sent")
	if err != nil || completed.EvidenceRef == nil || *completed.EvidenceRef != "evidence-not-sent" {
		t.Fatalf("not_sent proof must be retained: %+v err=%v", completed, err)
	}
}

func TestCompleteAttemptFromChannelResultRejectsUnsupportedRetryAfter(t *testing.T) {
	attempt, channel, finishedAt := channelResultFixture()
	channel.Capabilities.SupportsRetryAfter = domain.CapabilityUnsupported
	retryAfter := finishedAt.Add(time.Second)
	result := domain.ChannelSendResult{
		Outcome:    domain.ChannelOutcomeRejected,
		ErrorCode:  "rate_limited",
		RetryAfter: &retryAfter,
	}
	if _, err := CompleteAttemptFromChannelResult(attempt, channel, result, finishedAt, ""); err == nil {
		t.Fatal("unverified Retry-After capability must fail closed")
	}
}
