package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	newssource "example.com/stock-analysis-flow/internal/news/adapters/source"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
)

const newsFeedStageOutputRegistryVersion = "1.1.0"

type newsFeedStageComposition struct {
	catalog *newssource.StaticSourceCatalog
	registrations []newsapp.SourceEndpointRegistration
	executor researchflowapplication.StageExecutor
	registry *stageoutput.Registry
	resourceRefs []string
}

func newNewsFeedStageComposition(
	ctx context.Context,
	cfg newsconfig.Config,
	client *platformhttpclient.Client,
	repository *newsstorage.MySQLRepository,
) (*newsFeedStageComposition, error) {
	feedSources := cfg.FeedSources()
	if len(feedSources) == 0 {
		return nil, nil
	}
	if ctx == nil || client == nil || repository == nil {
		return nil, errors.New("News Feed composition requires startup context, shared Platform HTTP Client and News Repository")
	}

	registrations := newsFeedRegistrations(feedSources)
	policies := make(map[string]newsconfig.FeedSource, len(feedSources))
	for _, source := range feedSources {
		if existing, ok := policies[source.SourceID]; ok {
			if existing.SourcePolicyVersionID != source.SourcePolicyVersionID ||
				existing.ExpectedCurrentSourcePolicyVersionID != source.ExpectedCurrentSourcePolicyVersionID {
				return nil, fmt.Errorf("News Feed source %q has conflicting policy version expectations", source.SourceID)
			}
			continue
		}
		policies[source.SourceID] = source
	}

	catalog, err := newssource.NewStaticSourceCatalog(registrations)
	if err != nil {
		return nil, fmt.Errorf("validate trusted News Feed source catalog: %w", err)
	}

	publisher, err := newsapp.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		return nil, fmt.Errorf("create News Feed policy publisher: %w", err)
	}
	for sourceID, source := range policies {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		payload, err := json.Marshal(source.SourcePolicy)
		if err != nil {
			return nil, fmt.Errorf("encode News Feed source policy %q: %w", sourceID, err)
		}
		if err := publisher.ExecuteIfCurrent(
			ctx, newsdomain.NewsPolicyTypeSource, sourceID,
			source.SourcePolicyVersionID, source.ExpectedCurrentSourcePolicyVersionID, payload,
		); err != nil {
			return nil, fmt.Errorf("publish News Feed source policy %q@%s with expected current %q: %w",
				sourceID, source.SourcePolicyVersionID, source.ExpectedCurrentSourcePolicyVersionID, err)
		}
	}

	transport, err := newssource.NewPlatformEndpointTransport(client, registrations)
	if err != nil {
		return nil, fmt.Errorf("bind News Feed endpoints to Platform HTTP Client: %w", err)
	}
	collect, err := newsapp.NewCollectSourceWithPolicyResolver(catalog, transport, repository)
	if err != nil {
		return nil, fmt.Errorf("create News Feed CollectSource use case: %w", err)
	}
	ingest, err := newsapp.NewIngestCollectedSource(repository, newsFeedIngestID, catalog)
	if err != nil {
		return nil, fmt.Errorf("create News Feed IngestCollectedSource use case: %w", err)
	}
	adapter, err := newsstage.NewFeedCollectionAdapter(registrations, collect, ingest)
	if err != nil {
		return nil, fmt.Errorf("create News Feed collection Stage adapter: %w", err)
	}

	resourceRefs := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		ref := newsstage.SourceEndpointResourceRef(registration.SourceID, registration.EndpointID)
		if ref == "" {
			return nil, errors.New("News Feed registration has an invalid Source/Endpoint identity")
		}
		resourceRefs = append(resourceRefs, ref)
	}
	sort.Strings(resourceRefs)
	return &newsFeedStageComposition{
		catalog: catalog, registrations: registrations,
		executor: newsFeedStageExecutor{adapter: adapter},
		resourceRefs: resourceRefs,
	}, nil
}

func newsFeedIngestID() string {
	value, err := (researchflowIDGenerator{}).NewRef(context.Background(), "news")
	if err != nil {
		return ""
	}
	return value
}

type newsFeedStageExecutor struct {
	adapter *newsstage.FeedCollectionAdapter
}

func (e newsFeedStageExecutor) ExecuteStage(
	ctx context.Context,
	request researchflowapplication.StageExecutionRequest,
) (researchflowapplication.StageExecutionResult, error) {
	if request.Stage.Code != newsstage.CollectFeedsStageCode {
		return researchflowapplication.StageExecutionResult{}, fmt.Errorf("News Feed Stage Executor does not support stage code %q", request.Stage.Code)
	}
	policyRegistry, policyErr := resourcepolicy.NewRegistry()
	if policyErr != nil || e.adapter == nil ||
		request.ResourcePolicy.PolicyRef != resourcepolicy.NewsFeedConfiguredSourcesRef ||
		len(request.ResourcePolicy.AllowedProviderCodes) != 0 ||
		len(request.ResourcePolicy.AllowedSourceEndpointRefs) == 0 ||
		policyRegistry.ValidateSnapshot(request.ResourcePolicy) != nil {
		return failedNewsFeedStage("news_feed_resource_policy_invalid"), nil
	}
	output, err := e.adapter.Execute(ctx, request.Run.Ref+"/"+request.Attempt.Ref, request.Stage.Code, request.ResourcePolicy.AllowedSourceEndpointRefs)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return researchflowapplication.StageExecutionResult{}, ctxErr
		}
		return failedNewsFeedStage("news_feed_adapter_failed"), nil
	}
	providerResult := &stageoutput.ProviderResult{
		OutputType: output.OutputType, SchemaVersion: output.SchemaVersion,
		Payload: output.Payload, QualitySummary: output.QualitySummary,
		DiagnosticRefs: []string{},
	}
	var payload newsstage.FeedCollectionOutput
	if err := json.Unmarshal(output.Payload, &payload); err != nil {
		return failedNewsFeedStage("news_feed_output_invalid"), nil
	}
	switch payload.Status {
	case "success", "empty":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus: researchflowdomain.AttemptSucceeded,
			StageStatus: researchflowdomain.StageSucceeded,
			ProviderResult: providerResult,
		}, nil
	case "partial":
		if output.Retryable {
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptFailed,
				StageStatus: researchflowdomain.StageRetryWait,
				ProviderResult: providerResult,
				ReasonCodes: []string{"news_feed_retryable_source_failure"},
			}, nil
		}
		return researchflowapplication.StageExecutionResult{
			AttemptStatus: researchflowdomain.AttemptDegraded,
			StageStatus: researchflowdomain.StageDegraded,
			ProviderResult: providerResult,
			ReasonCodes: output.ReasonCodes,
		}, nil
	case "failed_retryable":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus: researchflowdomain.AttemptFailed,
			StageStatus: researchflowdomain.StageRetryWait,
			ProviderResult: providerResult,
			ReasonCodes: []string{"news_feed_retryable_source_failure"},
		}, nil
	case "failed":
		reason := payload.FailureCode
		if reason == "" {
			reason = "news_feed_collection_failed"
		}
		return failedNewsFeedStageWithOutput(reason, providerResult), nil
	default:
		return failedNewsFeedStage("news_feed_status_invalid"), nil
	}
}

func failedNewsFeedStage(reasonCode string) researchflowapplication.StageExecutionResult {
	return researchflowapplication.StageExecutionResult{
		AttemptStatus: researchflowdomain.AttemptFailed,
		StageStatus: researchflowdomain.StageFailed,
		ReasonCodes: []string{reasonCode},
	}
}

func failedNewsFeedStageWithOutput(reasonCode string, output *stageoutput.ProviderResult) researchflowapplication.StageExecutionResult {
	result := failedNewsFeedStage(reasonCode)
	result.ProviderResult = output
	return result
}

func newsFeedRegistrations(feedSources []newsconfig.FeedSource) []newsapp.SourceEndpointRegistration {
	registrations := make([]newsapp.SourceEndpointRegistration, 0, len(feedSources))
	for _, source := range feedSources {
		registrations = append(registrations, newsapp.SourceEndpointRegistration{
			SourceID: source.SourceID, EndpointID: source.EndpointID, Enabled: true,
			URL: source.URL, PublisherName: source.PublisherName, PublisherDomain: source.PublisherDomain,
			AccessMethod: source.AccessMethod, SourcePolicy: source.SourcePolicy,
			SourcePolicyVersionID: source.SourcePolicyVersionID,
			URLPolicy: source.URLPolicy, MaxResponseBytes: source.MaxResponseBytes,
		})
	}
	return registrations
}
