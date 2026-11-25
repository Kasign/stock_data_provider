package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// researchflowComposition contains shared execution dependencies. Owner Session
// identity remains outside this composition so Worker-only processes can assemble it.
type researchflowComposition struct {
	newsStage         *newsSearchStageComposition
	quantResultReader *researchflowQuantResultReader
	quantStage        *quantSMA20StageComposition
	runRepository     *researchflowstorage.MySQLRunRepository
	templateRegistry  *researchflowstorage.MySQLTemplateRegistry
	workflowTemplates []template.WorkflowTemplate
	resourcePolicies  researchflowapplication.ResourcePolicyResolver
	parameterSchemas  *parameterschema.Validator
	applicability     template.ApplicabilityEvaluator
	outputRegistries  researchflowapplication.StageOutputRegistryResolver
	stageExecutor     researchflowapplication.StageExecutor
	clock             *researchflowstorage.MySQLDatabaseClock
	ids               researchflowapplication.IDGenerator
}

func newResearchflowComposition(
	shared *serverresources.Resources,
	newsConfig newsconfig.Config,
	capacity serverconfig.ResearchflowCapacity,
	dataModule *marketdatabootstrap.Module,
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
	newsProviderCodes, err := newsStage.providerCodes()
	if err != nil {
		return nil, fmt.Errorf("read assembled News Provider inventory: %w", err)
	}
	policyRegistry, err := resourcepolicy.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("create Researchflow ResourcePolicy Registry: %w", err)
	}
	providerInventories := map[string][]string{
		resourcepolicy.NewsSearchConfiguredProvidersRef: append([]string(nil), newsProviderCodes...),
		resourcepolicy.QuantDailyBarsProvidersRef:       {},
	}
	if len(newsProviderCodes) > 0 {
		if _, err := policyRegistry.Resolve(resourcepolicy.NewsSearchConfiguredProvidersRef, newsProviderCodes); err != nil {
			return nil, fmt.Errorf("validate assembled News Provider inventory: %w", err)
		}
	}

	var quantStage *quantSMA20StageComposition
	if dataModule != nil {
		quantStage, err = newQuantSMA20StageComposition(dataModule, shared)
		if err != nil {
			return nil, fmt.Errorf("compose Quant SMA20 Stage: %w", err)
		}
	}
	if quantStage != nil {
		providerInventories[resourcepolicy.QuantDailyBarsProvidersRef] = append([]string(nil), quantStage.providerCodes...)
	}

	workflowTemplates := make([]template.WorkflowTemplate, 0, 2)
	newsTemplate, err := buildNewsSearchWorkflowTemplate(newsStage, policyRegistry)
	if err != nil {
		return nil, fmt.Errorf("preflight News Search workflow template: %w", err)
	}
	workflowTemplates = append(workflowTemplates, newsTemplate)
	if quantStage != nil {
		quantTemplate, templateErr := buildQuantSMA20WorkflowTemplate(quantStage, policyRegistry)
		if templateErr != nil {
			return nil, fmt.Errorf("preflight Quant SMA20 workflow template: %w", templateErr)
		}
		workflowTemplates = append(workflowTemplates, quantTemplate)
	}

	policyResolver := configuredResourcePolicyResolver{
		registry: policyRegistry, providerCodesByRef: providerInventories,
	}
	parameterSchemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		return nil, fmt.Errorf("create Researchflow parameter Schema Validator: %w", err)
	}
	outputRegistriesList := []researchflowapplication.StageOutputRegistry{newsStage.registry}
	quantExecutor := researchflowapplication.StageExecutor(nil)
	if quantStage != nil {
		outputRegistriesList = append(outputRegistriesList, quantStage.registry)
		quantExecutor = quantStage.executor
	}
	outputRegistries, err := researchflowapplication.NewStageOutputRegistrySet(outputRegistriesList...)
	if err != nil {
		return nil, fmt.Errorf("create immutable Researchflow Stage Output Registry set: %w", err)
	}
	stageExecutor := researchflowStageDispatcher{
		news:  newsStage.executor,
		quant: quantExecutor,
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
	clock, err := researchflowstorage.NewMySQLDatabaseClock(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow database clock: %w", err)
	}

	var quantResultReader *researchflowQuantResultReader
	if quantStage != nil {
		policyStore, storeErr := quantstorage.NewMySQLResultAccessPolicyStore(shared.DB)
		if storeErr != nil {
			return nil, fmt.Errorf("create Quant result access policy reader: %w", storeErr)
		}
		auditor, auditErr := quantstorage.NewMySQLAccessAuditor(shared.DB)
		if auditErr != nil {
			return nil, fmt.Errorf("create Quant result access auditor: %w", auditErr)
		}
		stageOutputReader, readerErr := researchflowapplication.NewGetStageOutputForRunStage(
			runRepository, runRepository, runRepository,
		)
		if readerErr != nil {
			return nil, fmt.Errorf("create Researchflow Stage Output association reader: %w", readerErr)
		}
		associationVerifier := quantSMA20ResultAssociationVerifier{reader: stageOutputReader}
		authorizedReader, authorizedErr := quantstorage.NewMySQLAuthorizedResultReader(
			policyStore, quantStage.repository, associationVerifier,
		)
		if authorizedErr != nil {
			return nil, fmt.Errorf("create Quant authorized result reader: %w", authorizedErr)
		}
		getQuantResult, getResultErr := quantapplication.NewGetResult(
			authorizedReader, researchflowWorkerPrincipalResolver{}, auditor,
			func() time.Time { return time.Now().UTC() },
		)
		if getResultErr != nil {
			return nil, fmt.Errorf("create Quant GetResult Application Service: %w", getResultErr)
		}
		quantResultReader = &researchflowQuantResultReader{getResult: getQuantResult}
	}

	return &researchflowComposition{
		newsStage: newsStage, quantStage: quantStage, quantResultReader: quantResultReader,
		runRepository: runRepository, templateRegistry: templateRegistry,
		workflowTemplates: workflowTemplates, resourcePolicies: policyResolver,
		parameterSchemas: parameterSchemas, applicability: researchflowApplicabilityEvaluator{},
		outputRegistries: outputRegistries, stageExecutor: stageExecutor,
		clock: clock, ids: researchflowIDGenerator{},
	}, nil
}

// publishWorkflowTemplate publishes each preflighted immutable definition.
// Existing identical versions are idempotent; conflicting content aborts Server startup.
func (c *researchflowComposition) publishWorkflowTemplate(ctx context.Context) error {
	if c == nil || c.templateRegistry == nil || len(c.workflowTemplates) == 0 {
		return errors.New("Researchflow workflow template registry is not configured")
	}
	if ctx == nil {
		return errors.New("Researchflow workflow template publication context is required")
	}
	for _, workflowTemplate := range c.workflowTemplates {
		if err := c.templateRegistry.Publish(ctx, workflowTemplate); err != nil {
			return fmt.Errorf("publish Researchflow workflow template %s@%s: %w", workflowTemplate.Ref, workflowTemplate.Version, err)
		}
	}
	return nil
}

// configuredResourcePolicyResolver snapshots only provider instances assembled by Server.
type configuredResourcePolicyResolver struct {
	registry           *resourcepolicy.Registry
	providerCodesByRef map[string][]string
}

func (r configuredResourcePolicyResolver) Resolve(ctx context.Context, ref string) (resourcepolicy.Snapshot, error) {
	if ctx == nil || r.registry == nil {
		return resourcepolicy.Snapshot{}, errors.New("Researchflow ResourcePolicy resolver is not configured")
	}
	if err := ctx.Err(); err != nil {
		return resourcepolicy.Snapshot{}, err
	}
	codes, exists := r.providerCodesByRef[ref]
	if !exists {
		return resourcepolicy.Snapshot{}, fmt.Errorf("%w: %s", resourcepolicy.ErrPolicyNotFound, ref)
	}
	return r.registry.Resolve(ref, append([]string(nil), codes...))
}
