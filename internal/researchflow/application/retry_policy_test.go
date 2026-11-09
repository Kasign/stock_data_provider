package application

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type fixedRetryJitter time.Duration

func (j fixedRetryJitter) Below(upper time.Duration) (time.Duration, error) {
	if time.Duration(j) < 0 || time.Duration(j) >= upper {
		return 0, ErrInvalidRetryPolicy
	}
	return time.Duration(j), nil
}

func TestCalculateRetryAtUsesAllowlistAndAttemptLimit(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	policy := template.RetryPolicy{
		MaxAttempts:          3,
		RetryableReasonCodes: []string{"provider_unavailable"},
		Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 1000, MaxDelayMS: 5000},
	}
	got, allowed, err := CalculateRetryAt(policy, 1, []string{"provider_unavailable"}, now, fixedRetryJitter(250*time.Millisecond))
	if err != nil || !allowed || !got.Equal(now.Add(250*time.Millisecond)) {
		t.Fatalf("unexpected retry: %v %v %v", got, allowed, err)
	}
	_, allowed, err = CalculateRetryAt(policy, 1, []string{"schema_invalid"}, now, fixedRetryJitter(0))
	if err != nil || allowed {
		t.Fatalf("non-allowlisted reason must not retry: allowed=%v err=%v", allowed, err)
	}
	_, allowed, err = CalculateRetryAt(policy, 3, []string{"provider_unavailable"}, now, fixedRetryJitter(0))
	if err != nil || allowed {
		t.Fatalf("max attempts includes first attempt: allowed=%v err=%v", allowed, err)
	}
}

func TestCalculateRetryAtCapsExponentialBackoff(t *testing.T) {
	now := time.Now().UTC()
	policy := template.RetryPolicy{
		MaxAttempts:          8,
		RetryableReasonCodes: []string{"temporary"},
		Backoff: template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 1000, MaxDelayMS: 3000},
	}
	got, allowed, err := CalculateRetryAt(policy, 4, []string{"temporary"}, now, fixedRetryJitter(0))
	if err != nil || !allowed || !got.Equal(now) {
		t.Fatalf("expected zero jitter under capped delay: %v %v %v", got, allowed, err)
	}
}
