package service

import (
	"errors"
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

func TestCircuitBreakerSeparatesContractAndQualityOutcomes(t *testing.T) {
	now := time.Date(2026, 11, 10, 5, 0, 0, 0, time.UTC)
	b := newCircuitBreaker(1, time.Minute)
	b.now = func() time.Time { return now }
	keys := map[string]string{"provider:quote": "quote"}

	if !b.Allow("provider:quote") {
		t.Fatal("initial request should be admitted")
	}
	b.ContractFailure("provider:quote")
	got := b.Diagnostics(keys)[0]
	if got.Status != "healthy" || got.ConsecutiveFails != 0 ||
		got.TotalContractFailure != 1 || got.TotalFailure != 0 ||
		got.LastContractFailureAt == nil {
		t.Fatalf("contract failure must be diagnostic, not availability failure: %+v", got)
	}

	b.QualityDegraded("provider:quote")
	got = b.Diagnostics(keys)[0]
	if got.Status != "healthy" || got.ConsecutiveFails != 0 ||
		got.TotalQualityDegraded != 1 || got.TotalRequestFailure != 0 || got.TotalFailure != 0 ||
		got.LastQualityDegradedAt == nil {
		t.Fatalf("quality degradation must be diagnostic, not availability failure: %+v", got)
	}
}

func TestCircuitBreakerNonAvailabilityOutcomeReleasesHalfOpenProbe(t *testing.T) {
	now := time.Date(2026, 11, 10, 5, 0, 0, 0, time.UTC)
	b := newCircuitBreaker(1, time.Second)
	b.now = func() time.Time { return now }
	b.Failure("provider:quote")
	now = now.Add(2 * time.Second)
	if !b.Allow("provider:quote") {
		t.Fatal("cooldown should allow half-open probe")
	}
	b.ContractFailure("provider:quote")
	if b.states["provider:quote"].probeInFlight {
		t.Fatal("contract response must release half-open probe")
	}
	if !b.Allow("provider:quote") {
		t.Fatal("contract response proves availability and should close the circuit")
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

func TestProviderCallFailureClassificationControlsAvailabilityCircuit(t *testing.T) {
	b := newCircuitBreaker(1, time.Minute)
	recordProviderCallFailure(b, "provider:quote", errors.New("quote source_version is missing"))
	state := b.states["provider:quote"]
	if state == nil || state.failures != 0 || state.totalFailure != 0 || state.totalContractFailure != 1 {
		t.Fatalf("contract error must not trip availability circuit: %+v", state)
	}

	recordProviderCallFailure(b, "provider:quote", errors.New("dial tcp: connection refused"))
	state = b.states["provider:quote"]
	if state.failures != 1 || state.totalFailure != 1 || b.Allow("provider:quote") {
		t.Fatalf("network failure must count toward availability circuit: %+v", state)
	}
}

func TestProviderHTTPClientErrorsDoNotTripAvailabilityCircuit(t *testing.T) {
	b := newCircuitBreaker(1, time.Minute)
	recordProviderCallFailure(b, "provider:quote", errors.New("HTTP status 400 Bad Request"))
	state := b.states["provider:quote"]
	if state == nil || state.failures != 0 || state.totalFailure != 0 || state.totalRequestFailure != 1 {
		t.Fatalf("HTTP 4xx must be tracked as request failure, not availability failure: %+v", state)
	}
}
