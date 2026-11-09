package service

import (
	"testing"
	"time"
)

func TestCircuitBreakerDiagnosticsTracksRealOutcomes(t *testing.T) {
	now := time.Date(2026, 10, 30, 10, 0, 0, 0, time.UTC)
	b := newCircuitBreaker(2, time.Minute)
	b.now = func() time.Time { return now }

	keys := map[string]string{"sina:quote": "quote"}
	got := b.Diagnostics(keys)
	if len(got) != 1 || got[0].Status != "not_observed" {
		t.Fatalf("initial diagnostic = %#v, want not_observed", got)
	}

	b.Failure("sina:quote")
	got = b.Diagnostics(keys)
	if got[0].Status != "degraded" || got[0].ConsecutiveFails != 1 || got[0].TotalFailure != 1 || got[0].LastFailureAt == nil {
		t.Fatalf("after first failure = %#v", got[0])
	}

	now = now.Add(time.Second)
	b.Failure("sina:quote")
	got = b.Diagnostics(keys)
	if got[0].Status != "open" || got[0].ConsecutiveFails != 2 || got[0].OpenUntil == nil || !got[0].OpenUntil.Equal(now.Add(time.Minute)) {
		t.Fatalf("after threshold failures = %#v", got[0])
	}

	now = now.Add(61 * time.Second)
	if !b.Allow("sina:quote") {
		t.Fatal("expected one half-open probe to be admitted")
	}
	got = b.Diagnostics(keys)
	if got[0].Status != "half_open" {
		t.Fatalf("after cooldown = %#v, want half_open", got[0])
	}

	b.Success("sina:quote")
	got = b.Diagnostics(keys)
	if got[0].Status != "healthy" || got[0].ConsecutiveFails != 0 || got[0].TotalSuccess != 1 || got[0].TotalFailure != 2 || got[0].LastSuccessAt == nil || got[0].OpenUntil != nil {
		t.Fatalf("after successful probe = %#v", got[0])
	}
}

func TestCircuitBreakerDiagnosticsKeepsCapabilitiesIndependent(t *testing.T) {
	b := newCircuitBreaker(1, time.Minute)
	b.Failure("provider:quote")
	got := b.Diagnostics(map[string]string{
		"provider:quote":      "quote",
		"provider:daily_bars": "daily_bars",
	})
	if len(got) != 2 {
		t.Fatalf("diagnostics count = %d", len(got))
	}
	states := map[string]string{}
	for _, item := range got {
		states[item.Capability] = item.Status
	}
	if states["quote"] != "open" || states["daily_bars"] != "not_observed" {
		t.Fatalf("capability states = %#v", states)
	}
}
