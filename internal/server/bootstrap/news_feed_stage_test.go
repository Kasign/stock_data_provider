package bootstrap

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsdomain "example.com/stock-analysis-flow/internal/news/domain"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
)

type newsFeedStageCollectorStub struct {
	entries map[string][]newsapp.CollectedSourceEntry
	errors map[string]error
}

func (s newsFeedStageCollectorStub) Execute(ctx context.Context, request newsapp.CollectSourceRequest) (newsapp.CollectSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return newsapp.CollectSourceResult{}, err
	}
	key := request.SourceID + "/" + request.EndpointID
	if err := s.errors[key]; err != nil {
		return newsapp.CollectSourceResult{}, err
	}
	return newsapp.CollectSourceResult{
		SourceID: request.SourceID, EndpointID: request.EndpointID,
		Entries: append([]newsapp.CollectedSourceEntry(nil), s.entries[key]...),
	}, nil
}

type newsFeedStageIngesterStub struct{}

func (newsFeedStageIngesterStub) Execute(ctx context.Context, request newsapp.IngestCollectedSourceRequest) ([]newsdomain.Observation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return []newsdomain.Observation{{NewsID: "news-" + request.Collection.SourceID}}, nil
}

func newsFeedStageRegistration(sourceID string) newsapp.SourceEndpointRegistration {
	return newsapp.SourceEndpointRegistration{
		SourceID: sourceID, EndpointID: "primary", Enabled: true,
		URL: "https://feeds.example/" + sourceID + ".xml",
		SourcePolicy: newsdomain.SourcePolicy{MaxRetention: 24 * time.Hour},
	}
}

func newsFeedStagePolicy(t *testing.T, registrations ...newsapp.SourceEndpointRegistration) resourcepolicy.Snapshot {
	t.Helper()
	registry, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	refs := make([]string, 0, len(registrations))
	for _, registration := range registrations {
		refs = append(refs, newsstage.SourceEndpointSnapshotRef(registration))
	}
	snapshot, err := registry.ResolveSourceEndpoints(resourcepolicy.NewsFeedConfiguredSourcesRef, refs)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestNewsFeedStageExecutorMapsSuccessfulApplicationIngestToSucceeded(t *testing.T) {
	registration := newsFeedStageRegistration("source-a")
	adapter, err := newsstage.NewFeedCollectionAdapter([]newsapp.SourceEndpointRegistration{registration},
		newsFeedStageCollectorStub{entries: map[string][]newsapp.CollectedSourceEntry{
			"source-a/primary": {{Candidate: newsdomain.Candidate{Title: "授权摘要"}}},
		}}, newsFeedStageIngesterStub{})
	if err != nil {
		t.Fatal(err)
	}
	executor := newsFeedStageExecutor{adapter: adapter}
	result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Run: researchflowdomain.Run{Ref: "run-feed-success"},
		Attempt: researchflowdomain.Attempt{Ref: "attempt-1"},
		Stage: researchflowdomain.Stage{Code: newsstage.CollectFeedsStageCode},
		ResourcePolicy: newsFeedStagePolicy(t, registration),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageSucceeded ||
		result.AttemptStatus != researchflowdomain.AttemptSucceeded || result.ProviderResult == nil {
		t.Fatalf("successful Feed Stage result = %+v", result)
	}
	if err := researchflowapplication.ValidateStageExecutionResult(result); err != nil {
		t.Fatalf("successful Feed Stage result violates Researchflow contract: %v", err)
	}
	var output newsstage.FeedCollectionOutput
	if err := json.Unmarshal(result.ProviderResult.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "success" || output.IngestedCount != 1 {
		t.Fatalf("Feed Stage output = %+v", output)
	}
}

func TestNewsFeedStageExecutorRetriesPartialResultWhenRemainingFailureIsRetryable(t *testing.T) {
	first := newsFeedStageRegistration("source-a")
	second := newsFeedStageRegistration("source-b")
	adapter, err := newsstage.NewFeedCollectionAdapter([]newsapp.SourceEndpointRegistration{first, second},
		newsFeedStageCollectorStub{
			entries: map[string][]newsapp.CollectedSourceEntry{
				"source-b/primary": {{Candidate: newsdomain.Candidate{Title: "可用摘要"}}},
			},
			errors: map[string]error{
				"source-a/primary": &newsapp.SourceCollectionError{Code: "source_fetch_failed", Retryable: true},
			},
		}, newsFeedStageIngesterStub{})
	if err != nil {
		t.Fatal(err)
	}
	executor := newsFeedStageExecutor{adapter: adapter}
	result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Run: researchflowdomain.Run{Ref: "run-feed-partial"},
		Attempt: researchflowdomain.Attempt{Ref: "attempt-1"},
		Stage: researchflowdomain.Stage{Code: newsstage.CollectFeedsStageCode},
		ResourcePolicy: newsFeedStagePolicy(t, first, second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageRetryWait ||
		result.AttemptStatus != researchflowdomain.AttemptFailed || result.ProviderResult == nil ||
		len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "news_feed_retryable_source_failure" {
		t.Fatalf("retryable partial Feed Stage result = %+v", result)
	}
	if err := researchflowapplication.ValidateStageExecutionResult(result); err != nil {
		t.Fatalf("retryable Feed Stage result violates Researchflow contract: %v", err)
	}
}

func TestNewsFeedStageCompositionIsDisabledWithoutExplicitFeedConfiguration(t *testing.T) {
	cfg, err := newsconfig.Load(platformconfig.NewEnvWithLookup(nil))
	if err != nil {
		t.Fatal(err)
	}
	composition, err := newNewsFeedStageComposition(context.Background(), cfg, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if composition != nil {
		t.Fatal("News Feed Stage was enabled without explicit trusted Feed source configuration")
	}
}
