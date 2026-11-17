package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// researchflowOwnerComposition is an in-process Application composition.
// It deliberately does not register HTTP routes; SRV-CTR-001 remains deferred.
type researchflowOwnerComposition struct {
	newsStage        *newsSearchStageComposition
	createRun        *researchflowapplication.CreateRunService
	getRun           *researchflowapplication.GetRunService
	cancelRun        *researchflowapplication.CancelRunService
	resourcePolicies researchflowapplication.ResourcePolicyResolver
}

func newResearchflowOwnerComposition(
	shared *serverresources.Resources,
	newsConfig newsconfig.Config,
	capacity serverconfig.ResearchflowCapacity,
	runDeadline time.Duration,
) (*researchflowOwnerComposition, error) {
	if shared == nil || shared.DB == nil || shared.HTTPClient == nil {
		return nil, errors.New("Researchflow Owner composition requires shared MariaDB and Platform HTTP Client resources")
	}
	if !capacity.Configured || runDeadline <= 0 {
		return nil, errors.New("Researchflow Owner composition requires explicit capacity and positive Run Deadline configuration")
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

	// The current News Search composition declares no hard-risk stage. A nil
	// gate registry therefore fails closed if a future Template adds one.
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
	clock, err := researchflowstorage.NewMySQLDatabaseClock(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow database clock: %w", err)
	}
	ids := researchflowIDGenerator{}
	identity := ownerSessionResearchflowIdentity{}

	createRun, err := researchflowapplication.NewCreateRunService(
		identity, clock, runDeadline, ids, templateRegistry,
		newsStage.parameterSchemas, newsStage.applicability, nil,
		policyResolver, runRepository,
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow CreateRun service: %w", err)
	}
	getRun, err := researchflowapplication.NewGetRunService(identity, runRepository, runRepository)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow GetRun service: %w", err)
	}
	cancelRun, err := researchflowapplication.NewCancelRunService(identity, runRepository, runRepository, runRepository)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow CancelRun service: %w", err)
	}

	return &researchflowOwnerComposition{
		newsStage: newsStage,
		createRun: createRun, getRun: getRun, cancelRun: cancelRun,
		resourcePolicies: policyResolver,
	}, nil
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
