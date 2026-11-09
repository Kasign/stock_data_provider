package diagnostics

import (
	"context"
	"errors"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	"example.com/stock-analysis-flow/internal/server/resources"
)

// Service owns the immutable check set used by Server diagnostics requests.
// Module checks are supplied by their owning modules; Server only composes them.
type Service struct {
	aggregator Aggregator
	checks     []platformdiagnostics.Check
}

func NewService(shared *resources.Resources, moduleChecks ...[]platformdiagnostics.Check) (*Service, error) {
	checks := StorageChecks(shared)
	for _, group := range moduleChecks {
		checks = append(checks, group...)
	}
	if len(checks) == 0 {
		return nil, ErrNoRequiredChecks
	}
	required := false
	for _, check := range checks {
		if check.Required {
			required = true
			break
		}
	}
	if !required {
		return nil, ErrNoRequiredChecks
	}
	return &Service{
		aggregator: Aggregator{Runner: platformdiagnostics.Runner{}},
		checks:     append([]platformdiagnostics.Check(nil), checks...),
	}, nil
}

// Run executes a fresh diagnostic pass; it never caches readiness between requests.
func (s *Service) Run(ctx context.Context) (Report, error) {
	if s == nil {
		return Report{}, errors.New("diagnostics service is required")
	}
	return s.aggregator.Run(ctx, s.checks)
}
