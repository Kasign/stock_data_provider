package diagnostics

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

func healthyCheck(id string, required bool) Check {
	return Check{
		ID: id, Required: required,
		Run: func(context.Context) (Observation, error) {
			return Observation{Status: StatusHealthy, SafeMessage: "ok"}, nil
		},
	}
}

func TestRunnerContinuesAfterIndependentFailure(t *testing.T) {
	runner := Runner{MaxConcurrent: 2, TotalTimeout: time.Second}
	checks := []Check{
		{ID: "first", Required: true, Run: func(context.Context) (Observation, error) {
			return Observation{}, errors.New("sensitive upstream response")
		}},
		healthyCheck("second", true),
	}
	report, err := runner.Run(context.Background(), checks)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Status != StatusFailed || report.Summary.Counts.Healthy != 1 || report.Summary.Counts.Failed != 1 {
		t.Fatalf("unexpected summary: %+v", report.Summary)
	}
	if report.Results[0].Observation.SafeMessage != "check failed" {
		t.Fatalf("unsafe error leaked: %q", report.Results[0].Observation.SafeMessage)
	}
}

func TestRunnerRequiredCheckCannotBeSkipped(t *testing.T) {
	check := Check{ID: "required", Required: true, Run: func(context.Context) (Observation, error) {
		return Observation{Status: StatusSkipped, ReasonCode: "not_configured", SafeMessage: "not configured"}, nil
	}}
	report, err := (Runner{}).Run(context.Background(), []Check{check})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Observation.Status != StatusFailed {
		t.Fatalf("expected failed, got %s", report.Results[0].Observation.Status)
	}
}

func TestRunnerOptionalFailureDegradesSummary(t *testing.T) {
	check := Check{ID: "optional", Run: func(context.Context) (Observation, error) {
		return Observation{}, fault.New(fault.ClassUnavailable, "news.provider.unavailable", "provider unavailable", nil)
	}}
	report, err := (Runner{}).Run(context.Background(), []Check{check})
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Status != StatusDegraded {
		t.Fatalf("expected degraded, got %s", report.Summary.Status)
	}
}

func TestRunnerHonorsCheckTimeout(t *testing.T) {
	check := Check{
		ID: "slow", Required: true, Timeout: 10 * time.Millisecond,
		Run: func(ctx context.Context) (Observation, error) {
			<-ctx.Done()
			return Observation{}, ctx.Err()
		},
	}
	report, err := (Runner{TotalTimeout: time.Second}).Run(context.Background(), []Check{check})
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Observation.Status != StatusUnavailable || report.Results[0].Fault == nil {
		t.Fatalf("expected timeout fault, got %+v", report.Results[0])
	}
}

func TestRunnerRejectsDuplicateIDs(t *testing.T) {
	_, err := (Runner{}).Run(context.Background(), []Check{healthyCheck("same", true), healthyCheck("same", false)})
	if err == nil {
		t.Fatal("expected duplicate ID error")
	}
}

func TestRunnerTotalDeadlineSkipsQueuedOptionalCheck(t *testing.T) {
	checks := []Check{
		{ID: "blocking", Required: true, Run: func(ctx context.Context) (Observation, error) {
			<-ctx.Done()
			return Observation{}, ctx.Err()
		}},
		healthyCheck("queued-optional", false),
	}
	report, err := (Runner{MaxConcurrent: 1, TotalTimeout: 20 * time.Millisecond}).Run(context.Background(), checks)
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].Observation.Status != StatusUnavailable {
		t.Fatalf("expected required check unavailable, got %s", report.Results[0].Observation.Status)
	}
	if report.Results[1].Observation.Status != StatusSkipped {
		t.Fatalf("expected queued optional check skipped, got %s", report.Results[1].Observation.Status)
	}
}
