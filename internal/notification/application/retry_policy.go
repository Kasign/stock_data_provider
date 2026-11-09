package application

import (
	"errors"
	"math"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

var (
	ErrInvalidRetryPolicy  = errors.New("notification: invalid retry policy")
	ErrRetryNotApplicable  = errors.New("notification: retry is not applicable")
)

const (
	RetryReasonBudgetExhausted = "retry_budget_exhausted"
	RetryReasonDeadlineExceeded = "deadline_exceeded"
)

type RetryPolicy struct {
	MaxAttempts        int
	MaxElapsed         time.Duration
	PerAttemptTimeout  time.Duration
	BaseDelay          time.Duration
	MaxDelay           time.Duration
}

type RetryDecision struct {
	Retry          bool
	ReasonCode     string
	NextAttemptAt  *time.Time
	DelayCap       time.Duration
	ChosenDelay    time.Duration
	RetryAfterUsed bool
}

// Validate requires explicit, positive millisecond policy values.
func (p RetryPolicy) Validate() error {
	if p.MaxAttempts < 1 ||
		!positiveMilliseconds(p.MaxElapsed) ||
		!positiveMilliseconds(p.PerAttemptTimeout) ||
		!positiveMilliseconds(p.BaseDelay) ||
		!positiveMilliseconds(p.MaxDelay) ||
		p.MaxDelay < p.BaseDelay {
		return ErrInvalidRetryPolicy
	}
	return nil
}

// ScheduleRetry evaluates a completed, explicitly retryable rejection.
// randomUnit is a caller-supplied uniform sample in [0,1); keeping entropy
// outside the policy makes the decision deterministic and directly testable.
func (p RetryPolicy) ScheduleRetry(
	delivery domain.DeliveryRecord,
	attempts []domain.DeliveryAttempt,
	now time.Time,
	randomUnit float64,
) (RetryDecision, error) {
	if err := p.Validate(); err != nil {
		return RetryDecision{}, err
	}
	if err := delivery.Validate(); err != nil {
		return RetryDecision{}, err
	}
	if !isRecoveryUTC(now) || math.IsNaN(randomUnit) ||
		math.IsInf(randomUnit, 0) || randomUnit < 0 || randomUnit >= 1 {
		return RetryDecision{}, ErrRetryNotApplicable
	}
	if delivery.Status != domain.DeliverySending || delivery.MaxAttempts != p.MaxAttempts ||
		len(attempts) != delivery.AttemptCount || len(attempts) == 0 {
		return RetryDecision{}, ErrRetryNotApplicable
	}

	var firstAttemptAt time.Time
	var latest domain.DeliveryAttempt
	seenAttemptIDs := make(map[string]struct{}, len(attempts))
	seenFencingTokens := make(map[int64]struct{}, len(attempts))
	for i := range attempts {
		attempt := attempts[i]
		if err := attempt.Validate(); err != nil ||
			attempt.DeliveryID != delivery.DeliveryID ||
			attempt.AttemptNo != i+1 {
			return RetryDecision{}, ErrRetryNotApplicable
		}
		if _, exists := seenAttemptIDs[attempt.AttemptID]; exists {
			return RetryDecision{}, ErrRetryNotApplicable
		}
		if _, exists := seenFencingTokens[attempt.FencingToken]; exists {
			return RetryDecision{}, ErrRetryNotApplicable
		}
		seenAttemptIDs[attempt.AttemptID] = struct{}{}
		seenFencingTokens[attempt.FencingToken] = struct{}{}
		if i == 0 {
			firstAttemptAt = attempt.StartedAt
		}
		latest = attempt
	}
	if latest.FinishedAt == nil || latest.Outcome == nil ||
		*latest.Outcome != domain.DeliveryAttemptRejected ||
		!latest.Retryable || now.Before(*latest.FinishedAt) {
		return RetryDecision{}, ErrRetryNotApplicable
	}

	decision := RetryDecision{}
	if latest.AttemptNo >= p.MaxAttempts {
		decision.ReasonCode = RetryReasonBudgetExhausted
		return decision, nil
	}
	elapsedDeadline := firstAttemptAt.Add(p.MaxElapsed)
	cutoff := earlierTime(delivery.DeadlineAt, elapsedDeadline)
	if !now.Before(cutoff) {
		decision.ReasonCode = RetryReasonDeadlineExceeded
		return decision, nil
	}

	decision.DelayCap = exponentialDelayCap(p.BaseDelay, p.MaxDelay, latest.AttemptNo)
	scaledDelay := uint64(float64(uint64(decision.DelayCap)) * randomUnit)
	if scaledDelay > uint64(decision.DelayCap) {
		scaledDelay = uint64(decision.DelayCap)
	}
	decision.ChosenDelay = time.Duration(scaledDelay)
	if decision.ChosenDelay <= 0 {
		// Delivery timestamps are persisted at microsecond precision; avoid a zero-delay hot loop.
		decision.ChosenDelay = time.Microsecond
	}
	next := now.Add(decision.ChosenDelay)
	if latest.RetryAfterAt != nil {
		if !isRecoveryUTC(*latest.RetryAfterAt) {
			return RetryDecision{}, ErrRetryNotApplicable
		}
		if latest.RetryAfterAt.After(next) {
			next = *latest.RetryAfterAt
			decision.RetryAfterUsed = true
		}
	}
	if !next.Before(cutoff) {
		decision.ReasonCode = RetryReasonDeadlineExceeded
		return decision, nil
	}
	decision.Retry = true
	decision.NextAttemptAt = &next
	return decision, nil
}

// AttemptDeadline returns min(now+per_attempt_timeout, delivery deadline,
// first attempt time+max elapsed).
func (p RetryPolicy) AttemptDeadline(now, firstAttemptAt, deliveryDeadline time.Time) (time.Time, error) {
	if err := p.Validate(); err != nil {
		return time.Time{}, err
	}
	if !isRecoveryUTC(now) || !isRecoveryUTC(firstAttemptAt) ||
		!isRecoveryUTC(deliveryDeadline) || now.Before(firstAttemptAt) {
		return time.Time{}, ErrRetryNotApplicable
	}
	deadline := now.Add(p.PerAttemptTimeout)
	deadline = earlierTime(deadline, deliveryDeadline)
	deadline = earlierTime(deadline, firstAttemptAt.Add(p.MaxElapsed))
	if !deadline.After(now) {
		return time.Time{}, ErrRetryNotApplicable
	}
	return deadline, nil
}

func positiveMilliseconds(value time.Duration) bool {
	return value > 0 && value%time.Millisecond == 0
}

func exponentialDelayCap(base, maximum time.Duration, attemptNo int) time.Duration {
	cap := base
	for step := 1; step < attemptNo && cap < maximum; step++ {
		if cap > maximum/2 {
			return maximum
		}
		cap *= 2
	}
	if cap > maximum {
		return maximum
	}
	return cap
}

func earlierTime(left, right time.Time) time.Time {
	if left.Before(right) {
		return left
	}
	return right
}

func cloneRetryTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
