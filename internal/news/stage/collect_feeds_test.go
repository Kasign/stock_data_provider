package stage

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

type feedCollectorStub struct {
	calls   []string
	entries map[string][]app.CollectedSourceEntry
	errors  map[string]error
}

func (s *feedCollectorStub) Execute(ctx context.Context, request app.CollectSourceRequest) (app.CollectSourceResult, error) {
	if err := ctx.Err(); err != nil {
		return app.CollectSourceResult{}, err
	}
	ref := SourceEndpointResourceRef(request.SourceID, request.EndpointID)
	s.calls = append(s.calls, ref)
	if err := s.errors[ref]; err != nil {
		return app.CollectSourceResult{}, err
	}
	return app.CollectSourceResult{
		SourceID: request.SourceID, EndpointID: request.EndpointID,
		Entries: append([]app.CollectedSourceEntry(nil), s.entries[ref]...),
	}, nil
}

type feedIngesterStub struct {
	prefixes []string
	calls    []app.IngestCollectedSourceRequest
}

func (s *feedIngesterStub) Execute(ctx context.Context, request app.IngestCollectedSourceRequest) ([]domain.Observation, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.prefixes = append(s.prefixes, request.IdempotencyPrefix)
	s.calls = append(s.calls, request)
	return []domain.Observation{{NewsID: "news-" + request.Collection.SourceID}}, nil
}

type feedAvailabilityQualifierStub struct {
	requests []app.QualifyCollectedAvailabilityRequest
	results  []app.QualifiedCollectedAvailability
	err      error
}

func (s *feedAvailabilityQualifierStub) Execute(
	ctx context.Context,
	request app.QualifyCollectedAvailabilityRequest,
) ([]app.QualifiedCollectedAvailability, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.requests = append(s.requests, request)
	return append([]app.QualifiedCollectedAvailability(nil), s.results...), s.err
}

func feedRegistration(sourceID, endpointID string) app.SourceEndpointRegistration {
	return app.SourceEndpointRegistration{
		SourceID: sourceID, EndpointID: endpointID, Enabled: true,
		SourcePolicy: domain.SourcePolicy{MaxRetention: 48 * time.Hour},
	}
}

func TestSourceEndpointSnapshotRefChangesWhenRegistrationChanges(t *testing.T) {
	registration := feedRegistration("source-a", "primary")
	registration.URL = "https://feeds.example/source-a.xml"
	original := SourceEndpointSnapshotRef(registration)
	changedURL := registration
	changedURL.URL = "https://other.example/source-a.xml"
	if original == SourceEndpointSnapshotRef(changedURL) {
		t.Fatal("changing endpoint URL did not change Run resource identity")
	}
	changedPolicy := registration
	changedPolicy.SourcePolicy.MaxRetention += time.Hour
	if original == SourceEndpointSnapshotRef(changedPolicy) {
		t.Fatal("changing Source policy did not change Run resource identity")
	}
}

func TestFeedCollectionAdapterUsesRunResourceSnapshot(t *testing.T) {
	collector := &feedCollectorStub{entries: map[string][]app.CollectedSourceEntry{
		"source-a/primary": {{Candidate: domain.Candidate{Title: "标题甲"}}},
		"source-b/primary": {{Candidate: domain.Candidate{Title: "标题乙"}}},
	}}
	ingester := &feedIngesterStub{}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{
		feedRegistration("source-a", "primary"),
		feedRegistration("source-b", "primary"),
	}, collector, ingester)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-1", CollectFeedsStageCode, []string{SourceEndpointSnapshotRef(feedRegistration("source-a", "primary"))})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(collector.calls, []string{"source-a/primary"}) {
		t.Fatalf("collector calls escaped Run resource snapshot: %#v", collector.calls)
	}
	if !reflect.DeepEqual(ingester.prefixes, []string{"run-1/" + CollectFeedsStageCode + "/source-a/primary"}) {
		t.Fatalf("ingest idempotency prefix = %#v", ingester.prefixes)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.SourceCount != 1 || output.SuccessSourceCount != 1 ||
		output.CollectedEntryCount != 1 || output.IngestedCount != 1 {
		t.Fatalf("Feed output counters = %+v", output)
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("valid Feed output rejected: %v", err)
	}
	if err := ValidateFeedCollectionQualitySummary(result.QualitySummary); err != nil {
		t.Fatalf("valid Feed quality summary rejected: %v", err)
	}
}

func TestFeedCollectionAdapterRunsAvailabilityQualifierBeforeIngest(t *testing.T) {
	registration := feedRegistration("cninfo", "announcements")
	registration.ProviderCode = "cninfo"
	registration.ProviderOutputPolicyRef = "cninfo-provider-output-policy"
	registration.ProviderOutputPolicyVersionID = "cninfo-provider-output-policy-v1"
	collector := &feedCollectorStub{entries: map[string][]app.CollectedSourceEntry{
		"cninfo/announcements": {{Candidate: domain.Candidate{PublishedAtRaw: "1793376000000"}}},
	}}
	ingester := &feedIngesterStub{}
	qualifier := &feedAvailabilityQualifierStub{results: []app.QualifiedCollectedAvailability{{
		EntryIndex:   0,
		SourceItemID: "1225649440",
		Qualified: app.QualifiedAvailabilityEvidence{
			FieldPath: "announcements.announcementTime",
		},
	}}}
	adapter, err := NewFeedCollectionAdapterWithAvailabilityQualifier(
		[]app.SourceEndpointRegistration{registration}, collector, ingester,
		qualifier, []string{"announcements.announcementTime"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-g3", CollectFeedsStageCode,
		[]string{SourceEndpointSnapshotRef(registration)})
	if err != nil {
		t.Fatal(err)
	}
	if len(qualifier.requests) != 1 {
		t.Fatalf("qualifier calls=%d want 1", len(qualifier.requests))
	}
	request := qualifier.requests[0]
	if request.Registration.SourceID != "cninfo" ||
		request.Registration.ProviderOutputPolicyVersionID != "cninfo-provider-output-policy-v1" ||
		!reflect.DeepEqual(request.FieldPaths, []string{"announcements.announcementTime"}) {
		t.Fatalf("qualifier request=%+v", request)
	}
	if len(result.QualifiedAvailability) != 1 || result.QualifiedAvailability[0].SourceItemID != "1225649440" {
		t.Fatalf("qualified availability=%+v", result.QualifiedAvailability)
	}
	if len(ingester.calls) != 1 {
		t.Fatalf("ingest calls=%d want 1", len(ingester.calls))
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("Feed output schema changed unexpectedly: %v", err)
	}
}

func TestFeedCollectionAdapterFailsSourceWhenAvailabilityQualificationFails(t *testing.T) {
	registration := feedRegistration("cninfo", "announcements")
	collector := &feedCollectorStub{entries: map[string][]app.CollectedSourceEntry{
		"cninfo/announcements": {{Candidate: domain.Candidate{PublishedAtRaw: "1793376000000"}}},
	}}
	ingester := &feedIngesterStub{}
	qualifier := &feedAvailabilityQualifierStub{err: app.ErrAvailabilitySourceQualificationDenied}
	adapter, err := NewFeedCollectionAdapterWithAvailabilityQualifier(
		[]app.SourceEndpointRegistration{registration}, collector, ingester,
		qualifier, []string{"announcements.announcementTime"},
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-g3", CollectFeedsStageCode,
		[]string{SourceEndpointSnapshotRef(registration)})
	if err != nil {
		t.Fatal(err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.FailedSourceCount != 1 || output.IngestedCount != 0 || len(ingester.calls) != 0 {
		t.Fatalf("qualification failure did not fail closed: output=%+v ingest_calls=%d", output, len(ingester.calls))
	}
	if len(output.Sources) != 1 || output.Sources[0].ErrorCode != "availability_qualification_failed" {
		t.Fatalf("source outcome=%+v", output.Sources)
	}
}

func TestFeedCollectionAdapterReportsPartialAndRetryableFailures(t *testing.T) {
	collector := &feedCollectorStub{
		entries: map[string][]app.CollectedSourceEntry{
			"source-b/primary": {{Candidate: domain.Candidate{Title: "标题乙"}}},
		},
		errors: map[string]error{
			"source-a/primary": &app.SourceCollectionError{Code: "source_fetch_failed", Retryable: true},
		},
	}
	ingester := &feedIngesterStub{}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{
		feedRegistration("source-a", "primary"),
		feedRegistration("source-b", "primary"),
	}, collector, ingester)
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-2/attempt-1", CollectFeedsStageCode, []string{
		SourceEndpointSnapshotRef(feedRegistration("source-a", "primary")),
		SourceEndpointSnapshotRef(feedRegistration("source-b", "primary")),
		SourceEndpointSnapshotRef(feedRegistration("source-removed", "primary")),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "partial" || output.SourceCount != 3 || output.SuccessSourceCount != 1 ||
		output.FailedSourceCount != 1 || output.SkippedSourceCount != 1 || result.Retryable {
		t.Fatalf("partial Feed outcome = %+v retryable=%v", output, result.Retryable)
	}
	if !reflect.DeepEqual(result.ReasonCodes, []string{"news_feed_retryable_source_failure", "news_feed_source_skipped"}) {
		t.Fatalf("Feed reason codes = %#v", result.ReasonCodes)
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("valid partial Feed payload rejected: %v", err)
	}
	if err := ValidateFeedCollectionQualitySummary(result.QualitySummary); err != nil {
		t.Fatalf("valid partial Feed quality summary rejected: %v", err)
	}
}

func TestFeedCollectionAdapterMarksRetryablePartialResultForResearchflowRetry(t *testing.T) {
	collector := &feedCollectorStub{
		entries: map[string][]app.CollectedSourceEntry{
			"source-b/primary": {{Candidate: domain.Candidate{Title: "标题乙"}}},
		},
		errors: map[string]error{
			"source-a/primary": &app.SourceCollectionError{Code: "source_fetch_failed", Retryable: true},
		},
	}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{
		feedRegistration("source-a", "primary"),
		feedRegistration("source-b", "primary"),
	}, collector, &feedIngesterStub{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-partial-retry", CollectFeedsStageCode, []string{
		SourceEndpointSnapshotRef(feedRegistration("source-a", "primary")),
		SourceEndpointSnapshotRef(feedRegistration("source-b", "primary")),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "partial" || output.FailedSourceCount != 1 || output.SuccessSourceCount != 1 ||
		!result.Retryable || !reflect.DeepEqual(result.ReasonCodes, []string{"news_feed_retryable_source_failure"}) {
		t.Fatalf("retryable partial result = %+v, retryable=%v, reasons=%#v", output, result.Retryable, result.ReasonCodes)
	}
}

func TestFeedCollectionAdapterMarksAllRetryableFailuresForResearchflowRetry(t *testing.T) {
	collector := &feedCollectorStub{errors: map[string]error{
		"source-a/primary": &app.SourceCollectionError{Code: "source_fetch_failed", Retryable: true},
	}}
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{
		feedRegistration("source-a", "primary"),
	}, collector, &feedIngesterStub{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := adapter.Execute(context.Background(), "run-3/attempt-1", CollectFeedsStageCode, []string{SourceEndpointSnapshotRef(feedRegistration("source-a", "primary"))})
	if err != nil {
		t.Fatal(err)
	}
	var output FeedCollectionOutput
	if err := json.Unmarshal(result.Payload, &output); err != nil {
		t.Fatal(err)
	}
	if output.Status != "failed_retryable" || !result.Retryable || output.FailureCode != "news_feed_retryable_source_failure" {
		t.Fatalf("all-retryable Feed outcome = %+v retryable=%v", output, result.Retryable)
	}
	if err := ValidateFeedCollectionPayload(result.Payload); err != nil {
		t.Fatalf("valid retryable Feed payload rejected: %v", err)
	}
	if err := ValidateFeedCollectionQualitySummary(result.QualitySummary); err != nil {
		t.Fatalf("valid retryable Feed quality summary rejected: %v", err)
	}
}

func TestFeedCollectionAdapterRejectsInvalidResourceSnapshotAndHonorsCancellation(t *testing.T) {
	adapter, err := NewFeedCollectionAdapter([]app.SourceEndpointRegistration{
		feedRegistration("source-a", "primary"),
	}, &feedCollectorStub{}, &feedIngesterStub{})
	if err != nil {
		t.Fatal(err)
	}
	validRef := SourceEndpointSnapshotRef(feedRegistration("source-a", "primary"))
	for _, refs := range [][]string{nil, {"source-a"}, {validRef, validRef}, {" " + validRef}} {
		if _, err := adapter.Execute(context.Background(), "run-4", CollectFeedsStageCode, refs); !errors.Is(err, ErrFeedCollectionAdapterRequired) {
			t.Errorf("invalid resource snapshot %#v returned %v", refs, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := adapter.Execute(ctx, "run-4", CollectFeedsStageCode, []string{SourceEndpointSnapshotRef(feedRegistration("source-a", "primary"))}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Feed collection returned %v", err)
	}
	if got := SourceEndpointResourceRef("source/a", "primary"); got != "" {
		t.Fatalf("invalid Source ID produced resource ref %q", got)
	}
}

func TestValidateFeedCollectionQualitySummaryRejectsStatusCounterDrift(t *testing.T) {
	raw := json.RawMessage("{\"status\":\"success\",\"source_count\":1,\"success_source_count\":0,\"empty_source_count\":0,\"skipped_source_count\":0,\"failed_source_count\":1,\"collected_entry_count\":1,\"ingested_count\":0}")
	if err := ValidateFeedCollectionQualitySummary(raw); err == nil {
		t.Fatal("quality summary with successful status and failed source was accepted")
	}
}

func TestValidateFeedCollectionPayloadRejectsAggregateCounterDrift(t *testing.T) {
	raw := json.RawMessage("{\"status\":\"success\",\"failure_code\":\"\",\"source_count\":1,\"success_source_count\":1,\"empty_source_count\":0,\"skipped_source_count\":0,\"failed_source_count\":0,\"collected_entry_count\":2,\"ingested_count\":1,\"sources\":[{\"source_id\":\"source-a\",\"endpoint_id\":\"primary\",\"status\":\"success\",\"collected_count\":1,\"ingested_count\":1,\"news_ids\":[\"news-1\"],\"error_code\":\"\",\"retryable\":false}]}")
	if err := ValidateFeedCollectionPayload(raw); err == nil {
		t.Fatal("payload with aggregate counter drift was accepted")
	}
}
