package bootstrap

import (
	"context"
	"errors"
	"fmt"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// researchflowComposition contains the shared execution dependencies. It has no
// Owner Session identity and can therefore be assembled by Worker-only processes.
type researchflowComposition struct {
	newsStage        *newsSearchStageComposition
	runRepository    *researchflowstorage.MySQLRunRepository
	templateRegistry *researchflowstorage.MySQLTemplateRegistry
	workflowTemplate template.WorkflowTemplate
	resourcePolicies researchflowapplication.ResourcePolicyResolver
	clock            *researchflowstorage.MySQLDatabaseClock
	ids              researchflowapplication.IDGenerator
}

func newResearchflowComposition(
	shared *serverresources.Resources,
	newsConfig newsconfig.Config,
	capacity serverconfig.ResearchflowCapacity,
) (*researchflowComposition, error) {
	if shared == nil || shared.DB == nil || shared.HTTPClient == nil {
		return nil, errors.New("Researchflow composition requires shared MariaDB and Platform HTTP Client resources")
	}
	if !capacity.Configured {
		return nil, errors.New("Researchflow composition requires explicit database-wide capacity configuration")
	}

	newsStage, err := newNewsSearchStageComposition(newsConfig, shared.HTTPClient)
	if err != nil {
		return nil, fmt.Errorf("compose News Search Stage: %w", err)
	}
	providerCodes, err := newsStage.providerCodes()
	if err != nil {
		return nil, fmt.Errorf("read assembled News Provider inventory: %w", err)
	}
	policyRegistry, err := resourcepolicy.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("create Researchflow ResourcePolicy Registry: %w", err)
	}
	if len(providerCodes) > 0 {
		if _, err := policyRegistry.Resolve(resourcepolicy.NewsSearchConfiguredProvidersRef, providerCodes); err != nil {
			return nil, fmt.Errorf("validate assembled News Provider inventory: %w", err)
		}
	}
	policyResolver := configuredNewsResourcePolicyResolver{
		registry: policyRegistry, providerCodes: append([]string(nil), providerCodes...),
	}

	runRepository, err := researchflowstorage.NewMySQLRunRepository(shared.DB, researchflowstorage.CapacityLimits{
		MaxQueuedRuns: capacity.MaxQueuedRuns, MaxWorkers: capacity.MaxWorkers,
	})
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Run Repository: %w", err)
	}
	templateRegistry, err := researchflowstorage.NewMySQLTemplateRegistry(shared.DB, nil)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Template Registry: %w", err)
	}
	workflowTemplate, err := buildNewsSearchWorkflowTemplate(newsStage, policyRegistry)
	if err != nil {
		return nil, fmt.Errorf("preflight News Search workflow template: %w", err)
	}
	clock, err := researchflowstorage.NewMySQLDatabaseClock(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow database clock: %w", err)
	}

	return &researchflowComposition{
		newsStage: newsStage, runRepository: runRepository,
		templateRegistry: templateRegistry, workflowTemplate: workflowTemplate,
		resourcePolicies: policyResolver, clock: clock,
		ids: researchflowIDGenerator{},
	}, nil
}

// publishWorkflowTemplate publishes the preflighted immutable definition. Existing
// identical versions are idempotent; conflicting content aborts Server startup.
func (c *researchflowComposition) publishWorkflowTemplate(ctx context.Context) error {
	if c == nil || c.templateRegistry == nil {
		return errors.New("Researchflow workflow template registry is not configured")
	}
	if ctx == nil {
		return errors.New("Researchflow workflow template publication context is required")
	}
	if err := c.templateRegistry.Publish(ctx, c.workflowTemplate); err != nil {
		return fmt.Errorf("publish Researchflow workflow template %s@%s: %w", c.workflowTemplate.Ref, c.workflowTemplate.Version, err)
	}
	return nil
}

// configuredNewsResourcePolicyResolver snapshots only the Provider instances
// successfully constructed by the Server composition root.
type configuredNewsResourcePolicyResolver struct {
	registry      *resourcepolicy.Registry
	providerCodes []string
}

func (r configuredNewsResourcePolicyResolver) Resolve(ctx context.Context, ref string) (resourcepolicy.Snapshot, error) {
	if ctx == nil || r.registry == nil {
		return resourcepolicy.Snapshot{}, errors.New("Researchflow ResourcePolicy resolver is not configured")
	}
	if err := ctx.Err(); err != nil {
		return resourcepolicy.Snapshot{}, err
	}
	return r.registry.Resolve(ref, append([]string(nil), r.providerCodes...))
}

var _ researchflowapplication.ResourcePolicyResolver = configuredNewsResourcePolicyResolver{}
