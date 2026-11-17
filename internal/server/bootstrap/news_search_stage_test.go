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
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
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

func TestNewsSearchStageCompositionProviderCodesAreStableAndValidated(t *testing.T) {
	composition, err := newNewsSearchStageCompositionWithProviders(newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "tavily"},
		newsStageTestProvider{code: "bocha"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	codes, err := composition.providerCodes()
	if err != nil {
		t.Fatal(err)
	}
	if len(codes) != 2 || codes[0] != "bocha" || codes[1] != "tavily" {
		t.Fatalf("Provider inventory is not canonical: %#v", codes)
	}

	duplicate, err := newNewsSearchStageCompositionWithProviders(newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "bocha"},
		newsStageTestProvider{code: "bocha"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := duplicate.providerCodes(); err == nil {
		t.Fatal("duplicate Provider Code was accepted")
	}
}

func TestNewsSearchStageCompositionRegistersExactOutputAndUsesPlatformClientOnlyWhenConfigured(t *testing.T) {
	unconfigured, err := newsconfig.Load(platformconfig.NewEnvWithLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	composition, err := newNewsSearchStageComposition(unconfigured, nil)
	if err != nil {
		t.Fatal(err)
	}
	if composition.parameterSchemas == nil || composition.applicability == nil {
		t.Fatal("News Search production parameter schema and applicability dependencies must be composed")
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
		Stage:          researchflowdomain.Stage{Code: newsstage.SearchCandidatesStageCode},
		Parameters:     json.RawMessage(`{"query":"market","limit":10}`),
		ResourcePolicy: testNewsResourcePolicy(t, []string{"bocha"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed ||
		len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "news_all_providers_skipped" {
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
	sharedClient, err := platformhttpclient.New(platformhttpclient.Config{})
	if err != nil {
		t.Fatal(err)
	}
	defer sharedClient.CloseIdleConnections()
	if _, err := newNewsSearchStageComposition(configured, nil); err == nil {
		t.Fatal("configured providers must fail closed without the Server-owned shared HTTP Client")
	}
	withProvider, err := newNewsSearchStageComposition(configured, sharedClient)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := withProvider.registry.Entry(
		newsstage.SearchCandidatesStageCode, newsstage.SearchCandidatesOutputType, newsstage.SearchCandidatesSchemaVersion,
	); !ok {
		t.Fatal("configured composition lost the exact output registry entry")
	}
}

func TestNewsSearchStageExecutorEnforcesRunProviderAllowlist(t *testing.T) {
	providers := newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "bocha", candidates: []newsdomain.Candidate{{Title: "authorized", RawURL: "https://a.example/item"}}},
		newsStageTestProvider{code: "tavily", candidates: []newsdomain.Candidate{{Title: "not-authorized", RawURL: "https://b.example/item"}}},
	}}
	executor := newNewsSearchExecutor(providers)
	result := executeNewsStageWithPolicy(t, executor, `{"query":"market","limit":10}`, []string{"bocha"})
	if result.ProviderResult == nil {
		t.Fatal("authorized Provider should produce a result")
	}
	var payload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(result.ProviderResult.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Outcomes) != 1 || payload.Outcomes[0].ProviderCode != "bocha" {
		t.Fatalf("executor called Provider outside Run allowlist: %+v", payload.Outcomes)
	}
	if len(payload.Candidates) != 1 || payload.Candidates[0].Title != "authorized" {
		t.Fatalf("unexpected authorized Provider candidates: %+v", payload.Candidates)
	}
}

func TestNewsSearchStageExecutorReportsProviderRemovedAfterRunCreation(t *testing.T) {
	executor := newNewsSearchExecutor(newsapp.StaticProviderSet{})
	result := executeNewsStageWithPolicy(t, executor, `{"query":"market","limit":10}`, []string{"bocha"})
	if result.StageStatus != researchflowdomain.StageFailed || len(result.ReasonCodes) != 1 ||
		result.ReasonCodes[0] != "news_all_providers_skipped" {
		t.Fatalf("removed Provider must be an explicit skipped outcome, not silently replaced: %+v", result)
	}
	if result.ProviderResult == nil {
		t.Fatal("removed Provider outcome should remain observable")
	}
	var payload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(result.ProviderResult.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Outcomes) != 1 || payload.Outcomes[0].ProviderCode != "bocha" ||
		payload.Outcomes[0].Status != newsdomain.ProviderSkipped ||
		payload.Outcomes[0].ErrorCode != newsapp.ProviderErrorNotConfigured {
		t.Fatalf("removed Provider outcome is not explicit: %+v", payload.Outcomes)
	}
}

func TestNewsSearchStageExecutorRejectsInvalidResourcePolicy(t *testing.T) {
	executor := newNewsSearchExecutor(newsapp.StaticProviderSet{Providers: []newsapp.Provider{
		newsStageTestProvider{code: "bocha"},
	}})
	result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage:      researchflowdomain.Stage{Code: newsstage.SearchCandidatesStageCode},
		Parameters: json.RawMessage(`{"query":"market","limit":10}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed || len(result.ReasonCodes) != 1 ||
		result.ReasonCodes[0] != "news_resource_policy_invalid" {
		t.Fatalf("missing ResourcePolicy must fail closed: %+v", result)
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
	retry := executeNewsStageWithPolicy(t, retryExecutor, `{"query":"market","limit":10}`, []string{"removed_provider", "timeout_provider"})
	if retry.StageStatus != researchflowdomain.StageRetryWait || retry.AttemptStatus != researchflowdomain.AttemptFailed ||
		len(retry.ReasonCodes) != 1 || retry.ReasonCodes[0] != "news_provider_retryable_failure" {
		t.Fatalf("retryable all-provider failure was not mapped to retry_wait: %+v", retry)
	}
	if err := researchflowapplication.ValidateStageExecutionResult(retry); err != nil {
		t.Fatal(err)
	}
	if retry.ProviderResult == nil {
		t.Fatal("retryable Provider outcomes must be retained in the Stage diagnostic output")
	}
	var retryPayload newsstage.SearchCandidatesOutput
	if err := json.Unmarshal(retry.ProviderResult.Payload, &retryPayload); err != nil {
		t.Fatal(err)
	}
	if len(retryPayload.Outcomes) != 2 {
		t.Fatalf("retry diagnostic output lost a Provider Outcome: %+v", retryPayload.Outcomes)
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
	composition, err := newNewsSearchStageComposition(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}

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

func testNewsResourcePolicy(t *testing.T, codes []string) resourcepolicy.Snapshot {
	t.Helper()
	registry, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Resolve(resourcepolicy.NewsSearchConfiguredProvidersRef, codes)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func newsProviderCodes(providers newsapp.ProviderSet) []string {
	if providers == nil {
		return nil
	}
	all := providers.All()
	codes := make([]string, 0, len(all))
	for _, provider := range all {
		if provider != nil {
			codes = append(codes, provider.Code())
		}
	}
	return codes
}

func newNewsSearchExecutor(providers newsapp.ProviderSet) newsSearchStageExecutor {
	return newsSearchStageExecutor{providers: providers}
}

func executeNewsStageWithPolicy(t *testing.T, executor newsSearchStageExecutor, parameters string, allowed []string) researchflowapplication.StageExecutionResult {
	t.Helper()
	result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage:          researchflowdomain.Stage{Code: newsstage.SearchCandidatesStageCode},
		Parameters:     json.RawMessage(parameters),
		ResourcePolicy: testNewsResourcePolicy(t, allowed),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func executeNewsStage(t *testing.T, executor newsSearchStageExecutor, parameters string) researchflowapplication.StageExecutionResult {
	t.Helper()
	result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage:          researchflowdomain.Stage{Code: newsstage.SearchCandidatesStageCode},
		Parameters:     json.RawMessage(parameters),
		ResourcePolicy: testNewsResourcePolicy(t, newsProviderCodes(executor.providers)),
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
