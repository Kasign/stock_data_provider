package modules

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"example.com/stock-analysis-flow/internal/analysis/app"
	"example.com/stock-analysis-flow/internal/analysis/domain"
	"example.com/stock-analysis-flow/internal/analysis/registry"
)

// RegistryRequirementCatalog derives data requirements from the selected
// strategies and the factor dependencies they declare. Adding a strategy or
// factor does not require changes to the runner or this catalog.
type RegistryRequirementCatalog struct {
	strategies *registry.Registry[Strategy]
	factors    *registry.Registry[FactorCalculator]
	core       []domain.DataRequirement
}

func NewRegistryRequirementCatalog(
	strategies *registry.Registry[Strategy],
	factors *registry.Registry[FactorCalculator],
	core []domain.DataRequirement,
) (*RegistryRequirementCatalog, error) {
	if strategies == nil || factors == nil {
		return nil, errors.New("strategy and factor registries are required")
	}
	if !strategies.IsFrozen() || !factors.IsFrozen() {
		return nil, errors.New("strategy and factor registries must be frozen before requirement catalog construction")
	}
	for _, factor := range factors.AllSorted() {
		if strings.TrimSpace(factor.Version()) == "" {
			return nil, fmt.Errorf("factor %s has an empty version", factor.ID())
		}
	}
	for _, strategy := range strategies.AllSorted() {
		if strings.TrimSpace(strategy.Version()) == "" {
			return nil, fmt.Errorf("strategy %s has an empty version", strategy.ID())
		}
		seen := make(map[string]struct{})
		for _, factorID := range strategy.FactorIDs() {
			factorID = strings.TrimSpace(factorID)
			if factorID == "" {
				return nil, fmt.Errorf("strategy %s declares an empty factor ID", strategy.ID())
			}
			if _, duplicate := seen[factorID]; duplicate {
				return nil, fmt.Errorf("strategy %s declares duplicate factor %s", strategy.ID(), factorID)
			}
			seen[factorID] = struct{}{}
			if _, err := factors.Get(factorID); err != nil {
				return nil, fmt.Errorf("strategy %s requires unregistered factor %s: %w", strategy.ID(), factorID, err)
			}
		}
	}
	return &RegistryRequirementCatalog{strategies: strategies, factors: factors, core: cloneRequirements(core)}, nil
}

func (c *RegistryRequirementCatalog) Resolve(ctx context.Context, request domain.RunRequest) (domain.RequirementSet, error) {
	if err := ctx.Err(); err != nil {
		return domain.RequirementSet{}, err
	}
	strategyIDs := uniqueStrings(request.StrategyIDs)
	if len(strategyIDs) == 0 {
		return domain.RequirementSet{}, errors.New("cannot resolve data requirements without strategy IDs")
	}
	sort.Strings(strategyIDs)
	out := domain.RequirementSet{
		Core:       cloneRequirements(c.core),
		ByStrategy: make(map[string][]domain.DataRequirement, len(strategyIDs)),
		ByFactor:   make(map[string][]domain.DataRequirement),
	}
	factorSet := make(map[string]struct{})
	for _, strategyID := range strategyIDs {
		strategy, err := c.strategies.Get(strategyID)
		if err != nil {
			return domain.RequirementSet{}, fmt.Errorf("requested strategy %s is not registered: %w", strategyID, err)
		}
		out.ByStrategy[strategyID] = cloneRequirements(strategy.Requirements())
		for _, factorID := range strategy.FactorIDs() {
			factorSet[factorID] = struct{}{}
		}
	}
	for factorID := range factorSet {
		out.FactorIDs = append(out.FactorIDs, factorID)
	}
	sort.Strings(out.FactorIDs)
	for _, factorID := range out.FactorIDs {
		factor, err := c.factors.Get(factorID)
		if err != nil {
			return domain.RequirementSet{}, fmt.Errorf("factor %s required by selected strategy is not registered: %w", factorID, err)
		}
		out.ByFactor[factorID] = cloneRequirements(factor.Requirements())
	}
	return out, nil
}

func cloneRequirements(input []domain.DataRequirement) []domain.DataRequirement {
	out := make([]domain.DataRequirement, len(input))
	for i, requirement := range input {
		out[i] = requirement
		out[i].Fields = append([]string(nil), requirement.Fields...)
		sort.Strings(out[i].Fields)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Dataset != out[j].Dataset {
			return out[i].Dataset < out[j].Dataset
		}
		if out[i].Consumer != out[j].Consumer {
			return out[i].Consumer < out[j].Consumer
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

var _ app.RequirementCatalog = (*RegistryRequirementCatalog)(nil)
