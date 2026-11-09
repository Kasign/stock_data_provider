package application

import (
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

// CompleteAttemptFromChannelResult converts a validated adapter result into
// the immutable Attempt outcome. Retryable is derived only from the exact
// ChannelConfigSnapshot allowlist, never from provider-supplied text/flags.
func CompleteAttemptFromChannelResult(
	attempt domain.DeliveryAttempt,
	channel domain.ChannelConfigSnapshot,
	result domain.ChannelSendResult,
	finishedAt time.Time,
	evidenceRef string,
) (domain.DeliveryAttempt, error) {
	if err := channel.Validate(); err != nil {
		return domain.DeliveryAttempt{}, err
	}
	if err := result.Validate(channel.Capabilities); err != nil {
		return domain.DeliveryAttempt{}, err
	}

	var outcome domain.DeliveryAttemptOutcome
	switch result.Outcome {
	case domain.ChannelOutcomeAccepted:
		outcome = domain.DeliveryAttemptAccepted
	case domain.ChannelOutcomeRejected:
		outcome = domain.DeliveryAttemptRejected
	case domain.ChannelOutcomeAmbiguous:
		outcome = domain.DeliveryAttemptAmbiguous
	case domain.ChannelOutcomeNotSent:
		outcome = domain.DeliveryAttemptNotSent
	default:
		return domain.DeliveryAttempt{}, domain.ErrInvalidChannelSendResult
	}
	return attempt.CompleteWithRetryAfter(
		finishedAt,
		outcome,
		result.ProviderMessageRef,
		result.ErrorCode,
		result.IsRetryable(channel),
		evidenceRef,
		result.RetryAfter,
	)
}
