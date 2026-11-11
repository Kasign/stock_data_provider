package application

import (
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var ErrInvalidRetryPolicy = errors.New("invalid Researchflow retry policy")

// RetryJitterSource returns a duration in [0, upperExclusive).
type RetryJitterSource interface {
	Below(upperExclusive time.Duration) (time.Duration, error)
}

type CryptoRetryJitter struct{}

func (CryptoRetryJitter) Below(upperExclusive time.Duration) (time.Duration, error) {
	if upperExclusive <= 0 {
		return 0, errors.New("retry jitter upper bound must be positive")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(int64(upperExclusive)))
	if err != nil {
		return 0, err
	}
	return time.Duration(n.Int64()), nil
}

// CalculateRetryAt enforces exact reason-code allowlisting and max_attempts
// (including the first attempt), then applies exponential full jitter.
func CalculateRetryAt(policy template.RetryPolicy, attemptNo int, reasonCodes []string,
	now time.Time, jitter RetryJitterSource) (time.Time, bool, error) {
	if attemptNo <= 0 || policy.MaxAttempts <= 0 || policy.Backoff.Algorithm != "exponential_full_jitter_v1" ||
		policy.Backoff.BaseDelayMS <= 0 || policy.Backoff.MaxDelayMS < policy.Backoff.BaseDelayMS ||
		jitter == nil || now.IsZero() {
		return time.Time{}, false, ErrInvalidRetryPolicy
	}
	if attemptNo >= policy.MaxAttempts || !hasRetryableReason(reasonCodes, policy.RetryableReasonCodes) {
		return time.Time{}, false, nil
	}
	capMS := policy.Backoff.BaseDelayMS
	for i := 1; i < attemptNo && capMS < policy.Backoff.MaxDelayMS; i++ {
		if capMS > policy.Backoff.MaxDelayMS/2 {
			capMS = policy.Backoff.MaxDelayMS
			break
		}
		capMS *= 2
	}
	if capMS > policy.Backoff.MaxDelayMS {
		capMS = policy.Backoff.MaxDelayMS
	}
	capDuration := time.Duration(capMS) * time.Millisecond
	if capDuration <= 0 || int64(capDuration/time.Millisecond) != capMS {
		return time.Time{}, false, ErrInvalidRetryPolicy
	}
	delay, err := jitter.Below(capDuration)
	if err != nil {
		return time.Time{}, false, err
	}
	if delay < 0 || delay >= capDuration {
		return time.Time{}, false, ErrInvalidRetryPolicy
	}
	return now.UTC().Add(delay), true, nil
}

func hasRetryableReason(actual, allowed []string) bool {
	allowlist := make(map[string]struct{}, len(allowed))
	for _, code := range allowed {
		code = strings.TrimSpace(code)
		if code != "" {
			allowlist[code] = struct{}{}
		}
	}
	for _, code := range actual {
		if _, ok := allowlist[code]; ok {
			return true
		}
	}
	return false
}
