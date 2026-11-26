package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
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
	newsFeedStage     *newsFeedStageComposition
	quantResultReader *researchflowQuantResultReader
	quantStage        *quantSMA20StageComposition
	quantFactorStage  *quantFactorStageComposition
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
	ctx context.Context,
	shared *serverresources.Resources,
	newsRepository *newsstorage.MySQLRepository,
	newsConfig newsconfig.Config,
	capacity serverconfig.ResearchflowCapacity,
	dataModule *marketdatabootstrap.Module,
) (*researchflowComposition, error) {
	if ctx == nil || shared == nil || shared.DB == nil || shared.HTTPClient == nil || newsRepository == nil {
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
	newsFeedStage, err := newNewsFeedStageComposition(ctx, newsConfig, shared.HTTPClient, newsRepository)
	if err != nil {
		return nil, fmt.Errorf("compose News Feed Collection Stage: %w", err)
	}
	if newsFeedStage != nil {
		combinedRegistry, registryErr := newNewsStageOutputRegistry(newsFeedStageOutputRegistryVersion, true)
		if registryErr != nil {
			return nil, fmt.Errorf("compose combined News Stage Output Registry: %w", registryErr)
		}
		newsStage.registry = combinedRegistry
		newsFeedStage.registry = combinedRegistry
	}
	policyRegistry, err := resourcepolicy.NewRegistry()
	if err != nil {
		return nil, fmt.Errorf("create Researchflow ResourcePolicy Registry: %w", err)
	}
	providerInventories := map[string][]string{
		resourcepolicy.NewsSearchConfiguredProvidersRef: append([]string(nil), newsProviderCodes...),
		resourcepolicy.QuantDailyBarsProvidersRef:       {},
	}
	sourceEndpointInventories := make(map[string][]string)
	if newsFeedStage != nil {
		sourceEndpointInventories[resourcepolicy.NewsFeedConfiguredSourcesRef] = append([]string(nil), newsFeedStage.resourceRefs...)
		if _, err := policyRegistry.ResolveSourceEndpoints(resourcepolicy.NewsFeedConfiguredSourcesRef, newsFeedStage.resourceRefs); err != nil {
			return nil, fmt.Errorf("validate assembled News Feed Source/Endpoint inventory: %w", err)
		}
	}
	if len(newsProviderCodes) > 0 {
		if _, err := policyRegistry.Resolve(resourcepolicy.NewsSearchConfiguredProvidersRef, newsProviderCodes); err != nil {
			return nil, fmt.Errorf("validate assembled News Provider inventory: %w", err)
		}
	}

	var quantStage *quantSMA20StageComposition
	var quantFactorStage *quantFactorStageComposition
	if dataModule != nil {
		quantStage, err = newQuantSMA20StageComposition(dataModule, shared)
		if err != nil {
			return nil, fmt.Errorf("compose Quant SMA20 Stage: %w", err)
		}
		quantFactorStage, err = newQuantFactorStageComposition(dataModule, shared)
		if err != nil {
			return nil, fmt.Errorf("compose Quant Factor Stage: %w", err)
		}
	}
	if quantStage != nil || quantFactorStage != nil {
		var smaProviders, factorProviders []string
		if quantStage != nil {
			smaProviders = quantStage.providerCodes
		}
		if quantFactorStage != nil {
			factorProviders = quantFactorStage.providerCodes
		}
		providerInventories[resourcepolicy.QuantDailyBarsProvidersRef] = mergeQuantProviderCodes(smaProviders, factorProviders)
	}

	workflowTemplates := make([]template.WorkflowTemplate, 0, 4)
	newsTemplate, err := buildNewsSearchWorkflowTemplate(newsStage, policyRegistry)
	if err != nil {
		return nil, fmt.Errorf("preflight News Search workflow template: %w", err)
	}
	workflowTemplates = append(workflowTemplates, newsTemplate)
	if newsFeedStage != nil {
		feedTemplate, templateErr := buildNewsFeedWorkflowTemplate(newsFeedStage, policyRegistry)
		if templateErr != nil {
			return nil, fmt.Errorf("preflight News Feed workflow template: %w", templateErr)
		}
		workflowTemplates = append(workflowTemplates, feedTemplate)
	}
	if quantStage != nil {
		quantTemplate, templateErr := buildQuantSMA20WorkflowTemplate(quantStage, policyRegistry)
		if templateErr != nil {
			return nil, fmt.Errorf("preflight Quant SMA20 workflow template: %w", templateErr)
		}
		workflowTemplates = append(workflowTemplates, quantTemplate)
	}
	if quantFactorStage != nil {
		factorTemplate, templateErr := buildQuantFactorWorkflowTemplate(quantFactorStage, policyRegistry)
		if templateErr != nil {
			return nil, fmt.Errorf("preflight Quant Factor workflow template: %w", templateErr)
		}
		workflowTemplates = append(workflowTemplates, factorTemplate)
	}

	policyResolver := configuredResourcePolicyResolver{
		registry: policyRegistry, providerCodesByRef: providerInventories,
		sourceEndpointRefsByRef: sourceEndpointInventories,
	}
	parameterSchemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		return nil, fmt.Errorf("create Researchflow parameter Schema Validator: %w", err)
	}
	outputRegistriesList := []researchflowapplication.StageOutputRegistry{newsStage.registry}
	feedExecutor := researchflowapplication.StageExecutor(nil)
	if newsFeedStage != nil {
		feedExecutor = newsFeedStage.executor
	}
	quantExecutor := researchflowapplication.StageExecutor(nil)
	if quantStage != nil {
		outputRegistriesList = append(outputRegistriesList, quantStage.registry)
		quantExecutor = quantStage.executor
	}
	quantFactorExecutor := researchflowapplication.StageExecutor(nil)
	if quantFactorStage != nil {
		outputRegistriesList = append(outputRegistriesList, quantFactorStage.registry)
		quantFactorExecutor = quantFactorStage.executor
	}
	outputRegistries, err := researchflowapplication.NewStageOutputRegistrySet(outputRegistriesList...)
	if err != nil {
		return nil, fmt.Errorf("create immutable Researchflow Stage Output Registry set: %w", err)
	}
	stageExecutor := researchflowStageDispatcher{
		news:        newsStage.executor,
		newsFeed:    feedExecutor,
		quant:       quantExecutor,
		quantFactor: quantFactorExecutor,
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
	// 任一已装配的 Quant Stage 都要求授权读取路径：只按 quant.sma20 判断会漏掉仅含 quant.factor 的装配。
	var quantResultRepository *quantstorage.MySQLRepository
	if quantStage != nil {
		quantResultRepository = quantStage.repository
	}
	if quantResultRepository == nil && quantFactorStage != nil {
		quantResultRepository = quantFactorStage.repository
	}
	var quantResultReader *researchflowQuantResultReader
	if quantResultRepository != nil {
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
		associationVerifier := quantResultAssociationVerifier{runs: runRepository, outputs: stageOutputReader}
		authorizedReader, authorizedErr := quantstorage.NewMySQLAuthorizedResultReader(
			policyStore, quantResultRepository, associationVerifier,
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
	if newsFeedStage != nil {
		if err := publishNewsFeedSourcePolicies(ctx, newsFeedStage.sourcePolicies, newsRepository); err != nil {
			return nil, fmt.Errorf("publish News Feed Source Policy snapshots: %w", err)
		}
	}

	return &researchflowComposition{
		newsStage: newsStage, newsFeedStage: newsFeedStage,
		quantStage: quantStage, quantFactorStage: quantFactorStage, quantResultReader: quantResultReader,
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

// configuredResourcePolicyResolver snapshots only resources assembled by Server.
type configuredResourcePolicyResolver struct {
	registry                *resourcepolicy.Registry
	providerCodesByRef      map[string][]string
	sourceEndpointRefsByRef map[string][]string
}

func (r configuredResourcePolicyResolver) Resolve(ctx context.Context, ref string) (resourcepolicy.Snapshot, error) {
	if ctx == nil || r.registry == nil {
		return resourcepolicy.Snapshot{}, errors.New("Researchflow ResourcePolicy resolver is not configured")
	}
	if err := ctx.Err(); err != nil {
		return resourcepolicy.Snapshot{}, err
	}
	if ref == resourcepolicy.NewsFeedConfiguredSourcesRef {
		refs, exists := r.sourceEndpointRefsByRef[ref]
		if !exists {
			return resourcepolicy.Snapshot{}, fmt.Errorf("%w: %s", resourcepolicy.ErrPolicyNotFound, ref)
		}
		return r.registry.ResolveSourceEndpoints(ref, append([]string(nil), refs...))
	}
	codes, exists := r.providerCodesByRef[ref]
	if !exists {
		return resourcepolicy.Snapshot{}, fmt.Errorf("%w: %s", resourcepolicy.ErrPolicyNotFound, ref)
	}
	return r.registry.Resolve(ref, append([]string(nil), codes...))
}
