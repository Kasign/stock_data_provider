package stage

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

type feedCollectorStub struct {
	results map[string]app.CollectSourceResult
	errors map[string]error
	requests []app.CollectSourceRequest
}

func (s *feedCollectorStub) Execute(_ context.Context, request app.CollectSourceRequest) (app.CollectSourceResult, error) {
	s.requests = append(s.requests, request)
	if err := s.errors[request.SourceID]; err != nil {
		return app.CollectSourceResult{}, err
	}
	return s.results[request.SourceID], nil
}

type feedIngesterStub struct {
	observations map[string][]domain.Observation
	errors map[string]error
	requests []app.IngestCollectedSourceRequest
}

func (s *feedIngesterStub) Execute(_ context.Context, request app.IngestCollectedSourceRequest) ([]domain.Observation, error) {
	s.requests = append(s.requests, request)
	for _, entry := range request.Collection.Entries {
		if err := s.errors[entry.Candidate.SourceItemID]; err != nil {
			return nil, err
		}
	}
	if len(request.Collection.Entries) == 0 {
		return []domain.Observation{}, nil
	}
	return append([]domain.Observation(nil), s.observations[request.Collection.SourceID]...), nil
}

func feedRegistration(sourceID, endpointID string) app.SourceEndpointRegistration {
	return app.SourceEndpointRegistration{
		SourceID: sourceID, EndpointID: endpointID, Enabled: true,
		AccessMethod: domain.AccessRSS,
		SourcePolicy: domain.SourcePolicy{MaxRetention: 48 * time.Hour},
	}
}

func feedCollectionFixture(sourceID string, count int) app.CollectSourceResult {
	entries := make([]app.CollectedSourceEntry, count)
	for i := range entries {
		entries[i].Candidate.SourceItemID = sourceID + "-item-" + string(rune('a'+i))
	}
	return app.CollectSourceResult{SourceID: sourceID, Entries: entries}
}

func TestFeedCollectionAdapterPersistsOnlyRunSnapshotAndReturnsReferenceSummary(t *testing.T) {
	registrationA := feedRegistration("trusted-a", "primary")
	registrationB := feedRegistration("trusted-b", "primary")
	collector := &feedCollectorStub{
		results: map[string]app.CollectSourceResult{
			"trusted-a": feedCollectionFixture("trusted-a", 1),
			"trusted-b": feedCollectionFixture("trusted-b", 1),
		},
		errors: map[string]error{},
	}
	ingester := &feedIngesterStub{
		observations: map[string][]domain.Observation{
			"trusted-a": {{NewsID: "news-a"}},
			"trusted-b": {{NewsID: "news-b"}},
		},
		errors: map[string]error{},
	}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{registrationA, registrationB}, collector, ingester)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-1/attempt-1", CollectFeedsStageCode,
		[]string{SourceEndpointResourceRef("trusted-b", "primary")})
	if err != nil {
		t.Fatal(err)
	}
	if len(collector.requests) != 1 || collector.requests[0].SourceID != "trusted-b" {
		t.Fatalf("collector calls = %+v, want only trusted-b from Run snapshot", collector.requests)
	}
	if len(ingester.requests) != 1 || ingester.requests[0].Collection.SourceID != "trusted-b" {
		t.Fatalf("ingester calls = %+v, want only trusted-b", ingester.requests)
	}
	request := ingester.requests[0]
	if request.Purpose != domain.PurposeInternalPersonalResearch ||
		request.Retention != defaultFeedRetention ||
		request.IdempotencyPrefix != "run-1/attempt-1/"+CollectFeedsStageCode+"/trusted-b/primary" {
		t.Fatalf("formal ingest request is not bound to the authorized Researchflow attempt: %+v", request)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "success" || output.SourceCount != 1 || output.IngestedCount != 1 ||
		len(output.Sources) != 1 || output.Sources[0].NewsIDs[0] != "news-b" {
		t.Fatalf("Feed Stage output = %+v", output)
	}
	if strings.Contains(string(result.Payload), "https://") || strings.Contains(string(result.Payload), "article body") {
		t.Fatalf("StageOutput leaked a URL or article content: %s", result.Payload)
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("ValidateFeedCollectionPayload: %v", err)
	}
	if err := ValidateFeedCollectionQualitySummary(result.QualitySummary); err != nil {
		t.Fatalf("ValidateFeedCollectionQualitySummary: %v", err)
	}
}

func TestFeedCollectionAdapterDoesNotRetryPartialSuccess(t *testing.T) {
	collector := &feedCollectorStub{
		results: map[string]app.CollectSourceResult{"trusted-a": feedCollectionFixture("trusted-a", 1)},
		errors: map[string]error{"trusted-b": &app.SourceCollectionError{Code: "source_http_status_502", Retryable: true}},
	}
	ingester := &feedIngesterStub{
		observations: map[string][]domain.Observation{"trusted-a": {{NewsID: "news-a"}}},
		errors: map[string]error{},
	}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{
		feedRegistration("trusted-a", "primary"), feedRegistration("trusted-b", "primary"),
	}, collector, ingester)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-1/attempt-1", CollectFeedsStageCode, []string{
		SourceEndpointResourceRef("trusted-a", "primary"),
		SourceEndpointResourceRef("trusted-b", "primary"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "partial" || result.Retryable {
		t.Fatalf("partial success must not retry already-ingested sources: status=%s retryable=%v", output.Status, result.Retryable)
	}
}

func TestFeedCollectionAdapterRetriesOnlyWhenAllSourcesFailedRetryably(t *testing.T) {
	collector := &feedCollectorStub{
		results: map[string]app.CollectSourceResult{},
		errors: map[string]error{"trusted-a": &app.SourceCollectionError{Code: "source_http_status_502", Retryable: true}},
	}
	ingester := &feedIngesterStub{observations: map[string][]domain.Observation{}, errors: map[string]error{}}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{feedRegistration("trusted-a", "primary")}, collector, ingester)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-1/attempt-1", CollectFeedsStageCode,
		[]string{SourceEndpointResourceRef("trusted-a", "primary")})
	if err != nil {
		t.Fatal(err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "failed_retryable" || !result.Retryable || output.FailedSourceCount != 1 {
		t.Fatalf("all-retryable failure = %+v retryable=%v", output, result.Retryable)
	}
}

func TestFeedCollectionAdapterRejectsInvalidRunResourceSnapshot(t *testing.T) {
	collector := &feedCollectorStub{results: map[string]app.CollectSourceResult{}, errors: map[string]error{}}
	ingester := &feedIngesterStub{observations: map[string][]domain.Observation{}, errors: map[string]error{}}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{feedRegistration("trusted-a", "primary")}, collector, ingester)
	if err != nil {
		t.Fatal(err)
	}
	for _, refs := range [][]string{
		nil,
		{"trusted-a/primary", "trusted-a/primary"},
		{"invalid ref"},
	} {
		if _, err := adapter.Execute(context.Background(), "run-1", CollectFeedsStageCode, refs); !errors.Is(err, ErrFeedCollectionAdapterRequired) {
			t.Fatalf("Execute refs=%v error=%v, want invalid snapshot", refs, err)
		}
	}
	if len(collector.requests) != 0 || len(ingester.requests) != 0 {
		t.Fatalf("invalid Run resource snapshot reached collectors: collect=%d ingest=%d", len(collector.requests), len(ingester.requests))
	}
}

func TestFeedCollectionAdapterHonorsContextCancellation(t *testing.T) {
	collector := &feedCollectorStub{results: map[string]app.CollectSourceResult{}, errors: map[string]error{}}
	ingester := &feedIngesterStub{observations: map[string][]domain.Observation{}, errors: map[string]error{}}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{feedRegistration("trusted-a", "primary")}, collector, ingester)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.Execute(ctx, "run-1", CollectFeedsStageCode, []string{SourceEndpointResourceRef("trusted-a", "primary")}); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute canceled error = %v", err)
	}
	if len(collector.requests) != 0 {
		t.Fatalf("collector called after cancellation: %+v", collector.requests)
	}
}
