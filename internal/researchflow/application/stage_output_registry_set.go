package application

import (
	"errors"
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
)

var (
	ErrInvalidStageOutputRegistrySet = errors.New("invalid Researchflow Stage Output Registry set")
	ErrStageOutputRegistryNotFound   = errors.New("Researchflow Stage Output Registry version not found")
)

// StageOutputRegistrySet retains immutable registries by exact version.
// A new Stage Output contract must publish a new version; it cannot mutate an existing version.
type StageOutputRegistrySet struct {
	registries map[string]StageOutputRegistry
}

func NewStageOutputRegistrySet(registries ...StageOutputRegistry) (*StageOutputRegistrySet, error) {
	if len(registries) == 0 {
		return nil, ErrInvalidStageOutputRegistrySet
	}
	set := &StageOutputRegistrySet{registries: make(map[string]StageOutputRegistry, len(registries))}
	for _, registry := range registries {
		if registry == nil || strings.TrimSpace(registry.Version()) == "" || strings.TrimSpace(registry.Fingerprint()) == "" {
			return nil, ErrInvalidStageOutputRegistrySet
		}
		version := registry.Version()
		if _, exists := set.registries[version]; exists {
			return nil, fmt.Errorf("%w: duplicate version %q", ErrInvalidStageOutputRegistrySet, version)
		}
		set.registries[version] = registry
	}
	return set, nil
}

func (set *StageOutputRegistrySet) Resolve(version string) (StageOutputRegistry, error) {
	if set == nil || strings.TrimSpace(version) == "" {
		return nil, ErrStageOutputRegistryNotFound
	}
	registry, exists := set.registries[version]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrStageOutputRegistryNotFound, version)
	}
	return registry, nil
}

var _ StageOutputRegistryResolver = (*StageOutputRegistrySet)(nil)
var _ StageOutputRegistry = (*stageoutput.Registry)(nil)
