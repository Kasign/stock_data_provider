package diagnostics

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestRunnerRejectsContractLimitViolations(t *testing.T) {
	checks := make([]Check, 129)
	for i := range checks {
		checks[i] = healthyCheck("check-"+strconv.Itoa(i), false)
	}
	if _, err := (Runner{}).Run(context.Background(), checks); err == nil {
		t.Fatal("expected more than 128 checks to be rejected")
	}
	if _, err := (Runner{MaxConcurrent: maxConcurrentLimit + 1}).Run(context.Background(), []Check{healthyCheck("one", true)}); err == nil {
		t.Fatal("expected max concurrent above contract limit to be rejected")
	}
	if _, err := (Runner{}).Run(context.Background(), []Check{{ID: "missing-run"}}); err == nil {
		t.Fatal("expected check without Run function to be rejected")
	}
	if _, err := (Runner{}).Run(context.Background(), []Check{{
		ID: "negative-timeout",
		Timeout: -time.Second,
		Run: func(context.Context) (Observation, error) {
			return Observation{Status: StatusHealthy, SafeMessage: "ok"}, nil
		},
	}}); err == nil {
		t.Fatal("expected negative check timeout to be rejected")
	}
}

func TestRunnerMarksChecksUnavailableWhenParentAlreadyCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	check := Check{ID: "not-started", Run: func(context.Context) (Observation, error) {
		called = true
		return Observation{Status: StatusHealthy, SafeMessage: "ok"}, nil
	}}

	report, err := (Runner{}).Run(ctx, []Check{check})
	if err != nil {
		t.Fatal(err)
	}
	if called {
		t.Fatal("check must not run after parent context cancellation")
	}
	got := report.Results[0].Observation
	if got.Status != StatusUnavailable || got.ReasonCode != "platform.diagnostics.canceled" {
		t.Fatalf("unexpected canceled observation: %+v", got)
	}
}

func TestRunnerHonorsConfiguredConcurrencyLimit(t *testing.T) {
	var active atomic.Int32
	var maximum atomic.Int32
	checks := make([]Check, 8)
	for i := range checks {
		checks[i] = Check{ID: "concurrent-" + strconv.Itoa(i), Run: func(context.Context) (Observation, error) {
			current := active.Add(1)
			for previous := maximum.Load(); current > previous; previous = maximum.Load() {
				if maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
			return Observation{Status: StatusHealthy, SafeMessage: "ok"}, nil
		}}
	}

	report, err := (Runner{MaxConcurrent: 2}).Run(context.Background(), checks)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Status != StatusHealthy {
		t.Fatalf("summary status = %s", report.Summary.Status)
	}
	if got := maximum.Load(); got == 0 || got > 2 {
		t.Fatalf("maximum concurrent checks = %d, want 1..2", got)
	}
}
