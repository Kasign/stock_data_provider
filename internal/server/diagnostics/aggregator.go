// Package diagnostics aggregates Platform and module checks for the Root Server.
package diagnostics

import (
	"context"
	"errors"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
)

var (
	ErrNoRequiredChecks = errors.New("at least one required readiness check is required")
	ErrRunnerRequired   = errors.New("diagnostics runner is required")
)

// Readiness is deliberately stricter than the aggregate diagnostics summary:
// every required check must be healthy before the instance accepts traffic.
type Readiness struct {
	Ready       bool   `json:"ready"`
	ReasonCode  string `json:"reason_code,omitempty"`
	SafeMessage string `json:"safe_message"`
}

// Report keeps the detailed Platform report and the traffic-readiness decision.
type Report struct {
	Diagnostics platformdiagnostics.Report `json:"diagnostics"`
	Readiness   Readiness                  `json:"readiness"`
}

// Aggregator delegates execution, limits, panic recovery and status aggregation
// to the Platform Runner. Server only applies the stricter readiness policy.
type Aggregator struct {
	Runner platformdiagnostics.Runner
}

func (a Aggregator) Run(ctx context.Context, checks []platformdiagnostics.Check) (Report, error) {
	required := 0
	for _, check := range checks {
		if check.Required {
			required++
		}
	}
	if required == 0 {
		return Report{}, ErrNoRequiredChecks
	}
	if a.Runner == nil {
		return Report{}, ErrRunnerRequired
	}

	diagnostics, err := a.Runner.Run(ctx, checks)
	if err != nil {
		return Report{}, err
	}
	readiness := evaluateReadiness(diagnostics.Results)
	return Report{Diagnostics: diagnostics, Readiness: readiness}, nil
}

func evaluateReadiness(results []platformdiagnostics.Result) Readiness {
	required := 0
	for _, result := range results {
		if !result.Required {
			continue
		}
		required++
		if result.Observation.Status != platformdiagnostics.StatusHealthy {
			return Readiness{
				Ready: false, ReasonCode: "server.readiness.required_check_unhealthy",
				SafeMessage: "One or more required checks are not healthy",
			}
		}
	}
	if required == 0 {
		return Readiness{
			Ready: false, ReasonCode: "server.readiness.no_required_checks",
			SafeMessage: "No required readiness checks are registered",
		}
	}
	return Readiness{Ready: true, SafeMessage: "Required dependencies are healthy"}
}
