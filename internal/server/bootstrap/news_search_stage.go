package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	newsprovider "example.com/stock-analysis-flow/internal/news/adapters/provider"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

const newsSearchStageOutputRegistryVersion = "1.0.0"

type newsSearchStageComposition struct {
	providers        newsapp.ProviderSet
	executor         researchflowapplication.StageExecutor
	registry         *stageoutput.Registry
	parameterSchemas *parameterschema.Validator
	applicability    template.ApplicabilityEvaluator
}

func newNewsSearchStageComposition(cfg newsconfig.Config, client *platformhttpclient.Client) (*newsSearchStageComposition, error) {
	providers := newsapp.ProviderSet(newsapp.StaticProviderSet{})
	if hasConfiguredNewsSearchProvider(cfg) {
		if client == nil {
			return nil, fmt.Errorf("News Provider 已配置但 Server 共享 Platform HTTP Client 不可用")
		}
		var err error
		providers, err = newsprovider.NewPlatformProviderSet(cfg, client)
		if err != nil {
			return nil, fmt.Errorf("创建 News Platform ProviderSet 失败: %w", err)
		}
	}

	return newNewsSearchStageCompositionWithProviders(providers)
}

func newNewsSearchStageCompositionWithProviders(providers newsapp.ProviderSet) (*newsSearchStageComposition, error) {
	if providers == nil {
		return nil, fmt.Errorf("News ProviderSet 不可用")
	}

	registry, err := stageoutput.NewRegistry(newsSearchStageOutputRegistryVersion, []stageoutput.Entry{{
		StageCode: newsstage.SearchCandidatesStageCode, OutputType: newsstage.SearchCandidatesOutputType,
		SchemaVersion:           newsstage.SearchCandidatesSchemaVersion,
		PayloadSchemaRef:        newsstage.SearchCandidatesPayloadSchemaRef,
		QualitySummarySchemaRef: newsstage.SearchCandidatesQualitySummarySchemaRef,
		PayloadValidator:        stageoutput.ValidatorFunc(newsstage.ValidatePayloadJSON),
		QualitySummaryValidator: stageoutput.ValidatorFunc(newsstage.ValidateQualitySummaryJSON),
	}})
	if err != nil {
		return nil, fmt.Errorf("创建 News Stage Output Registry 失败: %w", err)
	}
	parameterSchemas, err := newNewsSearchParameterSchemaValidator()
	if err != nil {
		return nil, fmt.Errorf("创建 News Search 参数 Schema Validator 失败: %w", err)
	}
	return &newsSearchStageComposition{
		providers:        providers,
		executor:         newsSearchStageExecutor{providers: providers},
		registry:         registry,
		parameterSchemas: parameterSchemas,
		applicability:    newsSearchApplicabilityEvaluator{},
	}, nil
}


func (c *newsSearchStageComposition) providerCodes() ([]string, error) {
	if c == nil || c.providers == nil {
		return nil, fmt.Errorf("News ProviderSet is not configured")
	}
	providers := c.providers.All()
	codes := make([]string, 0, len(providers))
	seen := make(map[string]struct{}, len(providers))
	for _, provider := range providers {
		if provider == nil || provider.Code() == "" {
			return nil, fmt.Errorf("News Provider inventory contains an invalid Provider")
		}
		code := provider.Code()
		if _, exists := seen[code]; exists {
			return nil, fmt.Errorf("News Provider inventory contains duplicate Provider Code %q", code)
		}
		seen[code] = struct{}{}
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes, nil
}

func hasConfiguredNewsSearchProvider(cfg newsconfig.Config) bool {
	return cfg.Bocha().Configured() || cfg.Tavily().Configured() ||
		cfg.Brave().Configured() || cfg.SearXNG().Configured()
}

type newsSearchStageExecutor struct {
	providers newsapp.ProviderSet
}

type unavailableNewsProvider struct {
	code string
}

func (p unavailableNewsProvider) Code() string { return p.code }
func (p unavailableNewsProvider) Search(context.Context, newsdomain.SearchRequest) ([]newsdomain.Candidate, error) {
	return nil, &newsapp.ProviderError{Code: newsapp.ProviderErrorNotConfigured, Skipped: true}
}

type restrictedNewsProviderSet struct {
	providers []newsapp.Provider
}

func (s restrictedNewsProviderSet) All() []newsapp.Provider {
	return append([]newsapp.Provider(nil), s.providers...)
}

func resolveNewsExecutionProviders(inventory newsapp.ProviderSet, snapshot resourcepolicy.Snapshot) (newsapp.ProviderSet, error) {
	registry, err := resourcepolicy.NewRegistry()
	if err != nil {
		return nil, err
	}
	if err := registry.ValidateSnapshot(snapshot); err != nil {
		return nil, fmt.Errorf("News ResourcePolicy snapshot is invalid: %w", err)
	}
	available := make(map[string]newsapp.Provider)
	if inventory != nil {
		for _, provider := range inventory.All() {
			if provider == nil || provider.Code() == "" {
				return nil, fmt.Errorf("News Provider inventory contains an invalid Provider")
			}
			if _, exists := available[provider.Code()]; exists {
				return nil, fmt.Errorf("News Provider inventory contains duplicate Provider Code %q", provider.Code())
			}
			available[provider.Code()] = provider
		}
	}
	allowed := make([]newsapp.Provider, 0, len(snapshot.AllowedProviderCodes))
	for _, code := range snapshot.AllowedProviderCodes {
		provider, exists := available[code]
		if !exists {
			// 历史 Run 的授权不随当前配置缩小；缺失资源必须形成明确 Outcome，
			// 不能静默删除或以其他 Provider 替代。
			provider = unavailableNewsProvider{code: code}
		}
		allowed = append(allowed, provider)
	}
	return restrictedNewsProviderSet{providers: allowed}, nil
}

func (e newsSearchStageExecutor) ExecuteStage(ctx context.Context, request researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
	if request.Stage.Code != newsstage.SearchCandidatesStageCode {
		return researchflowapplication.StageExecutionResult{}, fmt.Errorf("News Stage Executor does not support stage code %q", request.Stage.Code)
	}
	var adapter *newsstage.SearchCandidatesAdapter
	restricted, restrictErr := resolveNewsExecutionProviders(e.providers, request.ResourcePolicy)
	if restrictErr != nil {
		return failedNewsStage("news_resource_policy_invalid"), nil
	}
	adapter, err := newsstage.NewSearchCandidatesAdapter(newsapp.NewSearchCandidates(restricted))
	if err != nil || adapter == nil {
		return failedNewsStage("news_stage_adapter_unavailable"), nil
	}
	output, err := adapter.ExecuteParameters(ctx, request.Parameters)
	if err != nil {
		return failedNewsStage("news_search_invalid_parameters"), nil
	}

	var payload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(output.Payload, &payload); err != nil {
		return failedNewsStage("news_search_output_invalid"), nil
	}
	providerResult := &stageoutput.ProviderResult{
		OutputType: output.OutputType, SchemaVersion: output.SchemaVersion,
		Payload: output.Payload, QualitySummary: output.QualitySummary,
		DiagnosticRefs: []string{},
	}
	switch payload.Status {
	case "success", "empty":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptSucceeded,
			StageStatus:    researchflowdomain.StageSucceeded,
			ProviderResult: providerResult,
		}, nil
	case "partial":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptDegraded,
			StageStatus:    researchflowdomain.StageDegraded,
			ProviderResult: providerResult,
			ReasonCodes:    []string{"news_provider_partial_failure"},
		}, nil
	case "failed":
		if hasRetryableNewsProviderOutcome(payload.Outcomes) {
			return researchflowapplication.StageExecutionResult{
				AttemptStatus:  researchflowdomain.AttemptFailed,
				StageStatus:    researchflowdomain.StageRetryWait,
				ProviderResult: providerResult,
				ReasonCodes:    []string{"news_provider_retryable_failure"},
			}, nil
		}
		return failedNewsStage(newsFailureReasonCode(payload.FailureCode)), nil
	case "skipped":
		return failedNewsStageWithProviderResult(newsSkippedReasonCode(payload.FailureCode), providerResult), nil
	default:
		return failedNewsStage("news_search_status_invalid"), nil
	}
}

func failedNewsStageWithProviderResult(reasonCode string, providerResult *stageoutput.ProviderResult) researchflowapplication.StageExecutionResult {
	result := failedNewsStage(reasonCode)
	result.ProviderResult = providerResult
	return result
}

func failedNewsStage(reasonCode string) researchflowapplication.StageExecutionResult {
	return researchflowapplication.StageExecutionResult{
		AttemptStatus: researchflowdomain.AttemptFailed,
		StageStatus:   researchflowdomain.StageFailed,
		ReasonCodes:   []string{reasonCode},
	}
}

func hasRetryableNewsProviderOutcome(outcomes []newsstage.ProviderOutcome) bool {
	for _, outcome := range outcomes {
		if outcome.Retryable {
			return true
		}
	}
	return false
}

func newsFailureReasonCode(failureCode string) string {
	switch failureCode {
	case "invalid_request":
		return "news_search_invalid_request"
	case "provider_registry_invalid":
		return "news_provider_registry_invalid"
	case "all_provider_requests_failed":
		return "news_all_provider_requests_failed"
	default:
		return "news_search_failed"
	}
}

func newsSkippedReasonCode(failureCode string) string {
	switch failureCode {
	case "no_provider_configured", "provider_set_not_configured":
		return "news_provider_not_configured"
	case "all_providers_skipped":
		return "news_all_providers_skipped"
	default:
		return "news_search_skipped"
	}
}
