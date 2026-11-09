package diagnostics

import (
	"context"
	"errors"
	"fmt"

	platformdiagnostics "example.com/stock-analysis-flow/internal/platform/diagnostics"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
	"example.com/stock-analysis-flow/internal/server/resources"
)

// Service owns the immutable check set used by Server diagnostics requests.
// Module checks are supplied by their owning modules; Server only composes them.
type Service struct {
	aggregator Aggregator
	checks     []platformdiagnostics.Check
}

func NewService(shared *resources.Resources, moduleChecks ...[]platformdiagnostics.Check) (*Service, error) {
	return NewServiceWithMigration(shared, nil, moduleChecks...)
}

// NewServiceWithMigration includes the required read-only Schema readiness check.
func NewServiceWithMigration(shared *resources.Resources, registry *servermigration.Registry, moduleChecks ...[]platformdiagnostics.Check) (*Service, error) {
	checks := StorageChecks(shared, registry)
	for _, group := range moduleChecks {
		checks = append(checks, group...)
	}
	if len(checks) == 0 {
		return nil, ErrNoRequiredChecks
	}
	seen := make(map[string]struct{}, len(checks))
	required := false
	for _, check := range checks {
		if check.Required {
			required = true
		}
		if _, exists := seen[check.ID]; exists {
			return nil, fmt.Errorf("duplicate diagnostics check ID %q", check.ID)
		}
		seen[check.ID] = struct{}{}
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
