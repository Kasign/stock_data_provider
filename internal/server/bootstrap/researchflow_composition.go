package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// researchflowOwnerComposition adds authenticated Owner use cases to the shared
// execution composition. Worker Runtime does not depend on this type.
type researchflowOwnerComposition struct {
	execution        *researchflowComposition
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
	execution, err := newResearchflowComposition(shared, newsConfig, capacity)
	if err != nil {
		return nil, err
	}
	return newResearchflowOwnerCompositionFromExecution(execution, runDeadline)
}

func newResearchflowOwnerCompositionFromExecution(
	execution *researchflowComposition,
	runDeadline time.Duration,
) (*researchflowOwnerComposition, error) {
	if execution == nil || runDeadline <= 0 {
		return nil, errors.New("Researchflow Owner composition requires execution dependencies and a positive Run Deadline")
	}
	identity := ownerSessionResearchflowIdentity{}
	createRun, err := researchflowapplication.NewCreateRunService(
		identity, execution.clock, runDeadline, execution.ids, execution.templateRegistry,
		execution.newsStage.parameterSchemas, execution.newsStage.applicability, nil,
		execution.resourcePolicies, execution.runRepository,
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow CreateRun service: %w", err)
	}
	getRun, err := researchflowapplication.NewGetRunService(identity, execution.runRepository, execution.runRepository)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow GetRun service: %w", err)
	}
	cancelRun, err := researchflowapplication.NewCancelRunService(identity, execution.runRepository, execution.runRepository, execution.runRepository)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow CancelRun service: %w", err)
	}
	return &researchflowOwnerComposition{
		execution: execution, newsStage: execution.newsStage,
		createRun: createRun, getRun: getRun, cancelRun: cancelRun,
		resourcePolicies: execution.resourcePolicies,
	}, nil
}

// publishWorkflowTemplate remains an Owner composition convenience for existing
// bootstrap call sites; publication belongs to the shared execution composition.
func (c *researchflowOwnerComposition) publishWorkflowTemplate(ctx context.Context) error {
	if c == nil || c.execution == nil {
		return errors.New("Researchflow execution composition is not configured")
	}
	return c.execution.publishWorkflowTemplate(ctx)
}
