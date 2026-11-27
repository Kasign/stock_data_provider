package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/llm"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	newsplatform "example.com/stock-analysis-flow/internal/news/adapters/platform"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantresult "example.com/stock-analysis-flow/internal/quant/result"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	"example.com/stock-analysis-flow/internal/server/llmtransport"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// researchflowComposition contains shared execution dependencies. Owner Session
// identity remains outside this composition so Worker-only processes can assemble it.
type researchflowComposition struct {
	newsStage          *newsSearchStageComposition
	newsFeedStage      *newsFeedStageComposition
	quantResultReader  *researchflowQuantResultReader
	newsEvidenceReader *researchflowNewsEvidenceReader
	// stageOutputReader 与 quantResultRead 是阶段 2.3 结果读取的窄端口。
	// 二者复用 2.2 已装配的同一对象，不新建平行读取路径（AGENTS.md 规则 15）。
	stageOutputReader quantStageOutputReader
	quantResultRead   quantResultReadUseCase
	quantStage        *quantSMA20StageComposition
	quantFactorStage  *quantFactorStageComposition
	quantTrendStage   *quantTrendStageComposition
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
	llmProvider       llm.Provider
	llmModel          string
	llmCapabilities   llm.CapabilityState
}

func newResearchflowComposition(
	ctx context.Context,
	shared *serverresources.Resources,
	newsRepository *newsstorage.MySQLRepository,
	newsConfig newsconfig.Config,
	capacity serverconfig.ResearchflowCapacity,
	dataModule *marketdatabootstrap.Module,
	llmConfigs ...serverconfig.LLM,
) (*researchflowComposition, error) {
	if ctx == nil || shared == nil || shared.DB == nil || shared.HTTPClient == nil || newsRepository == nil {
		return nil, errors.New("Researchflow composition requires shared MariaDB and Platform HTTP Client resources")
	}
	if !capacity.Configured {
		return nil, errors.New("Researchflow composition requires explicit database-wide capacity configuration")
	}
	if len(llmConfigs) > 1 {
		return nil, errors.New("Researchflow composition accepts at most one LLM configuration")
	}
	var (
		llmProvider     llm.Provider
		llmModel        string
		llmCapabilities llm.CapabilityState
	)
	if len(llmConfigs) == 1 && llmConfigs[0].Configured {
		provider, err := llmtransport.NewResponsesAdapter(
			shared, llmConfigs[0].Provider, llmConfigs[0].APIKey,
			llmConfigs[0].Capabilities, llm.SystemClock{}, llmConfigs[0].Policy,
		)
		if err != nil {
			return nil, fmt.Errorf("compose LLM Provider: %w", err)
		}
		llmProvider = provider
		llmModel = llmConfigs[0].Provider.DefaultModel
		llmCapabilities = llmConfigs[0].Capabilities
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
	var quantTrendStage *quantTrendStageComposition
	var quantRiskStage *quantRiskStageComposition
	if dataModule != nil {
		quantStage, err = newQuantSMA20StageComposition(dataModule, shared)
		if err != nil {
			return nil, fmt.Errorf("compose Quant SMA20 Stage: %w", err)
		}
		quantFactorStage, err = newQuantFactorStageComposition(dataModule, shared)
		if err != nil {
			return nil, fmt.Errorf("compose Quant Factor Stage: %w", err)
		}
		quantTrendStage, err = newQuantTrendStageComposition(dataModule, shared)
		if err != nil {
			return nil, fmt.Errorf("compose Quant Trend Stage: %w", err)
		}
		quantRiskStage, err = newQuantRiskStageComposition(dataModule, shared)
		if err != nil {
			return nil, fmt.Errorf("compose Quant Risk Stage: %w", err)
		}
	}
	if quantStage != nil || quantFactorStage != nil || quantTrendStage != nil {
		var smaProviders, factorProviders, trendProviders []string
		if quantStage != nil {
			smaProviders = quantStage.providerCodes
		}
		if quantFactorStage != nil {
			factorProviders = quantFactorStage.providerCodes
		}
		if quantTrendStage != nil {
			trendProviders = quantTrendStage.providerCodes
		}
		providerInventories[resourcepolicy.QuantDailyBarsProvidersRef] = mergeQuantProviderCodes(smaProviders, factorProviders, trendProviders)
	}

	workflowTemplates := make([]template.WorkflowTemplate, 0, 6)
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
	if quantTrendStage != nil {
		trendTemplate, templateErr := buildQuantTrendWorkflowTemplate(quantTrendStage, policyRegistry)
		if templateErr != nil {
			return nil, fmt.Errorf("preflight Quant Trend workflow template: %w", templateErr)
		}
		workflowTemplates = append(workflowTemplates, trendTemplate)
	}
	if quantRiskStage != nil {
		riskTemplate, templateErr := buildQuantRiskWorkflowTemplate(quantRiskStage, policyRegistry)
		if templateErr != nil {
			return nil, fmt.Errorf("preflight Quant Risk workflow template: %w", templateErr)
		}
		workflowTemplates = append(workflowTemplates, riskTemplate)
	}
	// The four-Stage parallel template is only assembled when every Quant Stage
	// is configured; otherwise its combined Registry would reference missing
	// entries. Same gate as the single-Stage Quant templates.
	var quantResearchRegistry *stageoutput.Registry
	if quantStage != nil && quantFactorStage != nil && quantTrendStage != nil && quantRiskStage != nil {
		researchTemplate, combined, templateErr := buildQuantResearchWorkflowTemplate(
			quantStage.registry, quantFactorStage.registry, quantTrendStage.registry, quantRiskStage.registry,
			policyRegistry,
		)
		if templateErr != nil {
			return nil, fmt.Errorf("preflight Quant Research workflow template: %w", templateErr)
		}
		workflowTemplates = append(workflowTemplates, researchTemplate)
		quantResearchRegistry = combined
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
	quantTrendExecutor := researchflowapplication.StageExecutor(nil)
	if quantTrendStage != nil {
		outputRegistriesList = append(outputRegistriesList, quantTrendStage.registry)
		quantTrendExecutor = quantTrendStage.executor
	}
	quantRiskExecutor := researchflowapplication.StageExecutor(nil)
	if quantRiskStage != nil {
		outputRegistriesList = append(outputRegistriesList, quantRiskStage.registry)
		quantRiskExecutor = quantRiskStage.executor
	}
	// 合并 Registry 必须与 quant.research 模板同进同退：模板引用 1.6.0，
	// 没有注册该版本时 Worker 在写 Stage Output 时会因解析失败而无法提交。
	if quantResearchRegistry != nil {
		outputRegistriesList = append(outputRegistriesList, quantResearchRegistry)
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
		quantTrend:  quantTrendExecutor,
		quantRisk:   quantRiskExecutor,
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
	// Stage Output 身份读取同时供 Quant Result 与 News Evidence 的关联校验复用：
	// 只构造一次，避免两套平行读取路径（AGENTS.md 规则 2/6）。
	stageOutputReader, err := researchflowapplication.NewGetStageOutputForRunStage(
		runRepository, runRepository, runRepository,
	)
	if err != nil {
		return nil, fmt.Errorf("create Researchflow Stage Output association reader: %w", err)
	}
	// 任一已装配的 Quant Stage 都要求授权读取路径：只按 quant.sma20 判断会漏掉
	// 仅含其它 Quant Stage 的装配。四个 Stage 共用同一张 quant_results 表与同一套
	// 授权读取链，因此任一已装配 Stage 的 repository 都足以建立 GetResult。
	var quantResultRepository *quantstorage.MySQLRepository
	if quantStage != nil {
		quantResultRepository = quantStage.repository
	}
	if quantResultRepository == nil && quantFactorStage != nil {
		quantResultRepository = quantFactorStage.repository
	}
	if quantResultRepository == nil && quantTrendStage != nil {
		quantResultRepository = quantTrendStage.repository
	}
	if quantResultRepository == nil && quantRiskStage != nil {
		quantResultRepository = quantRiskStage.repository
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

	// News Evidence 侧与 Quant 同构的关联校验 + 授权读取。GetEvidence 复用组合根
	// 已持有的 newsRepository，不改 News 授权读取内核，也不改 bootstrap.go 的装配顺序。
	// 与 Quant 分开：身份模型（NewsID 对内容寻址 result_id）、策略锁表与授权主体都不同。
	evidenceAuthorization, err := newsapp.NewEvaluateNewsEvidenceReadAuthorization(
		newsplatform.NewAuthorizationFingerprintVerifier(), newsRepository,
	)
	if err != nil {
		return nil, fmt.Errorf("create News Evidence current authorization reviewer: %w", err)
	}
	getNewsEvidence, err := newsapp.NewGetEvidence(newsRepository, newsRepository, evidenceAuthorization)
	if err != nil {
		return nil, fmt.Errorf("create News GetEvidence Application Service: %w", err)
	}
	newsEvidenceReader := &researchflowNewsEvidenceReader{
		verifier: newsEvidenceAssociationVerifier{runs: runRepository, outputs: stageOutputReader},
		read:     getNewsEvidence,
	}

	return &researchflowComposition{
		newsStage: newsStage, newsFeedStage: newsFeedStage,
		quantStage: quantStage, quantFactorStage: quantFactorStage, quantTrendStage: quantTrendStage,
		quantResultReader: quantResultReader, newsEvidenceReader: newsEvidenceReader,
		stageOutputReader: stageOutputReader, quantResultRead: quantResultReader,
		runRepository: runRepository, templateRegistry: templateRegistry,
		workflowTemplates: workflowTemplates, resourcePolicies: policyResolver,
		parameterSchemas: parameterSchemas, applicability: researchflowApplicabilityEvaluator{},
		outputRegistries: outputRegistries, stageExecutor: stageExecutor,
		clock: clock, ids: researchflowIDGenerator{},
		llmProvider: llmProvider, llmModel: llmModel, llmCapabilities: llmCapabilities,
	}, nil
}

// readQuantResultForRun is the single domain entry point for post-terminal
// "result read-back verification". It goes exclusively through
// researchflowQuantResultReader.Read so the trusted in-process Worker identity
// is injected at that boundary only: HTTP input cannot construct the marker and
// cannot bypass the Run/Stage association check or the current-policy guard.
func (composition *researchflowComposition) readQuantResultForRun(
	ctx context.Context,
	runRef, resultID string,
) (quantresult.Envelope, error) {
	if composition == nil || composition.quantResultReader == nil {
		return quantresult.Envelope{}, errResearchflowWorkerPrincipalUnavailable
	}
	return composition.quantResultReader.Read(ctx, runRef, resultID)
}

// readNewsEvidenceForRun is the News-side counterpart of readQuantResultForRun.
// The reader verifies the Run↔Evidence association before delegating to the
// existing News GetEvidence use case; runRef is supplied by the trusted
// in-process execution boundary, never by HTTP input.
func (composition *researchflowComposition) readNewsEvidenceForRun(
	ctx context.Context,
	runRef string,
	request newsapp.GetEvidenceRequest,
) (newsapp.ResearchEvidenceRecord, error) {
	if composition == nil || composition.newsEvidenceReader == nil {
		return newsapp.ResearchEvidenceRecord{}, newsapp.ErrEvidenceAssociationUnavailable
	}
	return composition.newsEvidenceReader.Read(ctx, runRef, request)
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
