package bootstrap

import (
	"context"
	"encoding/json"
	"fmt"

	newsprovider "example.com/stock-analysis-flow/internal/news/adapters/provider"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
)

const newsSearchStageOutputRegistryVersion = "1.0.0"

type newsSearchStageComposition struct {
	executor researchflowapplication.StageExecutor
	registry *stageoutput.Registry
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

	adapter, err := newsstage.NewSearchCandidatesAdapter(newsapp.NewSearchCandidates(providers))
	if err != nil {
		return nil, fmt.Errorf("创建 News SearchCandidates Adapter 失败: %w", err)
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
	return &newsSearchStageComposition{
		executor: newsSearchStageExecutor{adapter: adapter},
		registry: registry,
	}, nil
}

func hasConfiguredNewsSearchProvider(cfg newsconfig.Config) bool {
	return cfg.Bocha().Configured() || cfg.Tavily().Configured() ||
		cfg.Brave().Configured() || cfg.SearXNG().Configured()
}

type newsSearchStageExecutor struct {
	adapter *newsstage.SearchCandidatesAdapter
}

func (e newsSearchStageExecutor) ExecuteStage(ctx context.Context, request researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
	if request.Stage.Code != newsstage.SearchCandidatesStageCode {
		return researchflowapplication.StageExecutionResult{}, fmt.Errorf("News Stage Executor does not support stage code %q", request.Stage.Code)
	}
	if e.adapter == nil {
		return failedNewsStage("news_stage_adapter_unavailable"), nil
	}
	output, err := e.adapter.ExecuteParameters(ctx, request.Parameters)
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
				AttemptStatus: researchflowdomain.AttemptFailed,
				StageStatus:   researchflowdomain.StageRetryWait,
				ReasonCodes:   []string{"news_provider_retryable_failure"},
			}, nil
		}
		return failedNewsStage(newsFailureReasonCode(payload.FailureCode)), nil
	case "skipped":
		return failedNewsStage(newsSkippedReasonCode(payload.FailureCode)), nil
	default:
		return failedNewsStage("news_search_status_invalid"), nil
	}
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
