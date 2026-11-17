package service

import (
	"testing"
	"time"
)

func TestCircuitBreakerCooldownAllowsOnlyOneProbe(t *testing.T) {
	now := time.Date(2026, 10, 21, 4, 0, 0, 0, time.UTC)
	b := newCircuitBreaker(1, time.Second)
	b.now = func() time.Time { return now }
	if !b.Allow("eastmoney:quote") {
		t.Fatal("closed circuit should allow calls")
	}
	b.Failure("eastmoney:quote")
	if b.Allow("eastmoney:quote") {
		t.Fatal("open circuit should block before cooldown")
	}
	now = now.Add(2 * time.Second)
	if !b.Allow("eastmoney:quote") {
		t.Fatal("cooldown should permit one probe")
	}
	if b.Allow("eastmoney:quote") {
		t.Fatal("half-open circuit should not permit concurrent probes")
	}
	b.Failure("eastmoney:quote")
	if b.Allow("eastmoney:quote") {
		t.Fatal("failed probe should reopen circuit")
	}
	now = now.Add(2 * time.Second)
	if !b.Allow("eastmoney:quote") {
		t.Fatal("second cooldown should permit probe")
	}
	b.Success("eastmoney:quote")
	if !b.Allow("eastmoney:quote") {
		t.Fatal("successful probe should close circuit")
	}
}

func TestQuoteAndBarCircuitsAreIndependent(t *testing.T) {
	b := newCircuitBreaker(1, time.Second)
	b.Failure("eastmoney:quote")
	if b.Allow("eastmoney:quote") {
		t.Fatal("quote circuit should be open")
	}
	if !b.Allow("eastmoney:daily_bars") {
		t.Fatal("quote failure must not disable daily-bar capability")
	}
}

func TestCircuitBreakerAbandonReleasesHalfOpenProbe(t *testing.T) {
	b := newCircuitBreaker(1, time.Second)
	now := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	b.now = func() time.Time { return now }
	b.Failure("eastmoney:quote")
	now = now.Add(2 * time.Second)
	if !b.Allow("eastmoney:quote") {
		t.Fatal("expected one half-open probe after cooldown")
	}
	if !b.states["eastmoney:quote"].probeInFlight {
		t.Fatal("expected probe to be marked in flight")
	}
	b.Abandon("eastmoney:quote")
	if b.states["eastmoney:quote"].probeInFlight {
		t.Fatal("abandon must release half-open state")
	}
	if !b.Allow("eastmoney:quote") {
		t.Fatal("a later request must be able to retry the abandoned probe")
	}
}
