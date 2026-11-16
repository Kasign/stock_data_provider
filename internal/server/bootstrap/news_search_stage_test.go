package bootstrap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
)

type newsStageTestProvider struct {
	code       string
	candidates []newsdomain.Candidate
	err        error
}

func (p newsStageTestProvider) Code() string { return p.code }
func (p newsStageTestProvider) Search(context.Context, newsdomain.SearchRequest) ([]newsdomain.Candidate, error) {
	return p.candidates, p.err
}

func TestNewsSearchStageCompositionRegistersExactOutputAndUsesPlatformClientOnlyWhenConfigured(t *testing.T) {
	unconfigured, err := newsconfig.Load(platformconfig.NewEnvWithLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	composition, err := newNewsSearchStageComposition(unconfigured)
	if err != nil {
		t.Fatal(err)
	}
	if composition.httpClient != nil {
		t.Fatal("unconfigured providers must not create an outbound HTTP client")
	}
	if composition.registry.Version() != newsSearchStageOutputRegistryVersion {
		t.Fatalf("unexpected registry version: %s", composition.registry.Version())
	}
	entry, ok := composition.registry.Entry(
		newsstage.SearchCandidatesStageCode, newsstage.SearchCandidatesOutputType, newsstage.SearchCandidatesSchemaVersion,
	)
	if !ok {
		t.Fatal("News SearchCandidates output is not registered")
	}
	if entry.PayloadSchemaRef != newsstage.SearchCandidatesPayloadSchemaRef ||
		entry.QualitySummarySchemaRef != newsstage.SearchCandidatesQualitySummarySchemaRef {
		t.Fatalf("registry schema refs do not match News contract: %+v", entry)
	}
	result, err := composition.executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: newsstage.SearchCandidatesStageCode},
		Parameters: json.RawMessage(`{"query":"market","limit":10}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed ||
		len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "news_provider_not_configured" {
		t.Fatalf("missing providers must fail closed: %+v", result)
	}
	if err := researchflowapplication.ValidateStageExecutionResult(result); err != nil {
		t.Fatalf("failed result is not a valid Researchflow outcome: %v", err)
	}

	configured, err := newsconfig.Load(platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		if key == newsconfig.SearXNGEndpointEnv {
			return "https://search.example.test", true
		}
		return "", false
	}))
	if err != nil {
		t.Fatal(err)
	}
	withProvider, err := newNewsSearchStageComposition(configured)
	if err != nil {
		t.Fatal(err)
	}
	defer withProvider.Close()
	if withProvider.httpClient == nil {
		t.Fatal("configured provider must use the shared Platform HTTP Client")
	}
	if _, ok := withProvider.registry.Entry(
		newsstage.SearchCandidatesStageCode, newsstage.SearchCandidatesOutputType, newsstage.SearchCandidatesSchemaVersion,
	); !ok {
		t.Fatal("configured composition lost the exact output registry entry")
	}
}

func TestNewsSearchStageExecutorMapsEmptyPartialRetryableAndInvalidResults(t *testing.T) {
	emptyExecutor := newNewsSearchExecutor(newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "empty_provider"},
	}})
	empty := executeNewsStage(t, emptyExecutor, `{"query":"market","limit":10}`)
	if empty.StageStatus != researchflowdomain.StageSucceeded || empty.ProviderResult == nil {
		t.Fatalf("empty search is a successful completed query: %+v", empty)
	}
	if err := researchflowapplication.ValidateStageExecutionResult(empty); err != nil {
		t.Fatal(err)
	}

	partialExecutor := newNewsSearchExecutor(newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "success_provider", candidates: []newsdomain.Candidate{{
			Title: "headline", RawURL: "https://publisher.example/item", PublisherName: "Publisher",
		}}},
		newsStageTestProvider{code: "timeout_provider", err: &newsapp.ProviderError{
			Code: newsapp.ProviderErrorTimeout, Retryable: true,
		}},
	}})
	partial := executeNewsStage(t, partialExecutor, `{"query":"market","limit":10}`)
	if partial.StageStatus != researchflowdomain.StageDegraded || partial.ProviderResult == nil {
		t.Fatalf("partial search must retain usable output as degraded: %+v", partial)
	}
	if err := researchflowapplication.ValidateStageExecutionResult(partial); err != nil {
		t.Fatal(err)
	}

	retryExecutor := newNewsSearchExecutor(newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "timeout_provider", err: &newsapp.ProviderError{
			Code: newsapp.ProviderErrorTimeout, Retryable: true,
		}},
	}})
	retry := executeNewsStage(t, retryExecutor, `{"query":"market","limit":10}`)
	if retry.StageStatus != researchflowdomain.StageRetryWait || retry.AttemptStatus != researchflowdomain.AttemptFailed ||
		len(retry.ReasonCodes) != 1 || retry.ReasonCodes[0] != "news_provider_retryable_failure" {
		t.Fatalf("retryable all-provider failure was not mapped to retry_wait: %+v", retry)
	}
	if err := researchflowapplication.ValidateStageExecutionResult(retry); err != nil {
		t.Fatal(err)
	}

	invalid := executeNewsStage(t, emptyExecutor, `{"query":"market","limit":10,"unknown":true}`)
	if invalid.StageStatus != researchflowdomain.StageFailed ||
		len(invalid.ReasonCodes) != 1 || invalid.ReasonCodes[0] != "news_search_invalid_parameters" {
		t.Fatalf("invalid parameters must fail closed: %+v", invalid)
	}
}

func TestNewsSearchStageOutputPassesExactRegistryValidation(t *testing.T) {
	cfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	composition, err := newNewsSearchStageComposition(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()

	executor := newNewsSearchExecutor(newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "empty_provider"},
	}})
	result := executeNewsStage(t, executor, `{"query":"market","limit":10}`)
	identity := stageoutput.ExecutionIdentity{
		RunRef: "run-1", StageRef: "stage-1", AttemptRef: "attempt-1",
		StageCode: newsstage.SearchCandidatesStageCode, TemplateVersion: "1.0.0",
		RegistryVersion: composition.registry.Version(),
	}
	envelope, err := stageoutput.BuildEnvelope(identity, *result.ProviderResult, time.Date(2026, 11, 16, 1, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if err := composition.registry.Validate(envelope, stageoutput.ValidationContext{Identity: identity}); err != nil {
		t.Fatalf("News output failed exact Registry validation: %v", err)
	}
}

func TestNewsSearchStageExecutorRejectsUnsupportedStageCode(t *testing.T) {
	executor := newNewsSearchExecutor(newsapp.StaticProviderSet{})
	_, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: "news.other"},
	})
	if err == nil {
		t.Fatal("unsupported Stage Code must be rejected")
	}
}

func newNewsSearchExecutor(providers newsapp.ProviderSet) newsSearchStageExecutor {
	adapter, err := newsstage.NewSearchCandidatesAdapter(newsapp.NewSearchCandidates(providers))
	if err != nil {
		panic(err)
	}
	return newsSearchStageExecutor{adapter: adapter}
}

func executeNewsStage(t *testing.T, executor newsSearchStageExecutor, parameters string) researchflowapplication.StageExecutionResult {
	t.Helper()
	result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: newsstage.SearchCandidatesStageCode},
		Parameters: json.RawMessage(parameters),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

