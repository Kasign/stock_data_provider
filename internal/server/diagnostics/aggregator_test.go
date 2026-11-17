package diagnostics

import (
	"context"
	"errors"
	"testing"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

func check(id string, required bool, status platformdiagnostics.Status) platformdiagnostics.Check {
	return platformdiagnostics.Check{
		ID: id, Name: id, Required: required,
		Run: func(context.Context) (platformdiagnostics.Observation, error) {
			observation := platformdiagnostics.Observation{
				Status: status, SafeMessage: "diagnostic result",
			}
			if status != platformdiagnostics.StatusHealthy {
				observation.ReasonCode = "test.check_not_healthy"
			}
			return observation, nil
		},
	}
}

func TestAggregatorRequiresAtLeastOneRequiredCheck(t *testing.T) {
	_, err := (Aggregator{}).Run(context.Background(), []platformdiagnostics.Check{
		check("optional", false, platformdiagnostics.StatusHealthy),
	})
	if !errors.Is(err, ErrNoRequiredChecks) {
		t.Fatalf("expected ErrNoRequiredChecks, got %v", err)
	}
}

func TestAggregatorReadinessRequiresEveryRequiredCheckHealthy(t *testing.T) {
	tests := []struct {
		name   string
		status platformdiagnostics.Status
		ready  bool
	}{
		{name: "healthy", status: platformdiagnostics.StatusHealthy, ready: true},
		{name: "degraded", status: platformdiagnostics.StatusDegraded},
		{name: "unavailable", status: platformdiagnostics.StatusUnavailable},
		{name: "failed", status: platformdiagnostics.StatusFailed},
		{name: "skipped", status: platformdiagnostics.StatusSkipped},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report, err := (Aggregator{}).Run(context.Background(), []platformdiagnostics.Check{
				check("required", true, test.status),
			})
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if report.Readiness.Ready != test.ready {
				t.Fatalf("Ready = %v, want %v", report.Readiness.Ready, test.ready)
			}
		})
	}
}

func TestOptionalDegradedCheckDoesNotBlockReadiness(t *testing.T) {
	report, err := (Aggregator{}).Run(context.Background(), []platformdiagnostics.Check{
		check("required", true, platformdiagnostics.StatusHealthy),
		check("optional", false, platformdiagnostics.StatusDegraded),
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !report.Readiness.Ready {
		t.Fatalf("optional degradation must not block readiness: %+v", report.Readiness)
	}
	if report.Diagnostics.Summary.Status != platformdiagnostics.StatusDegraded {
		t.Fatalf("diagnostic summary = %s, want degraded", report.Diagnostics.Summary.Status)
	}
}

func TestAggregatorRejectsDuplicateCheckIDs(t *testing.T) {
	_, err := (Aggregator{}).Run(context.Background(), []platformdiagnostics.Check{
		check("duplicate", true, platformdiagnostics.StatusHealthy),
		check("duplicate", false, platformdiagnostics.StatusHealthy),
	})
	if err == nil {
		t.Fatal("expected duplicate check ID error")
	}
}
