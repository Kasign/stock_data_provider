package stage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

const (
	CollectFeedsStageCode = "news.collect_feeds"
	CollectFeedsOutputType = "news.feed_collection"
	CollectFeedsSchemaVersion = "1.0.0"
	CollectFeedsPayloadSchemaRef = "doc/researchflow/schema/news-feed-collection-output-1.0.0.schema.json"
	CollectFeedsQualitySummarySchemaRef = "doc/researchflow/schema/news-feed-collection-quality-1.0.0.schema.json"
	defaultFeedEntryLimit = 200
	defaultFeedRetention = 24 * time.Hour
)

var ErrFeedCollectionAdapterRequired = errors.New("news stage: Feed collection use cases and registrations are required")

type FeedCollectionOutput struct {
	Status string `json:"status"`
	FailureCode string `json:"failure_code"`
	SourceCount int `json:"source_count"`
	SuccessSourceCount int `json:"success_source_count"`
	EmptySourceCount int `json:"empty_source_count"`
	SkippedSourceCount int `json:"skipped_source_count"`
	FailedSourceCount int `json:"failed_source_count"`
	CollectedEntryCount int `json:"collected_entry_count"`
	IngestedCount int `json:"ingested_count"`
	Sources []FeedSourceOutcome `json:"sources"`
}

type FeedSourceOutcome struct {
	SourceID string `json:"source_id"`
	EndpointID string `json:"endpoint_id"`
	Status string `json:"status"`
	CollectedCount int `json:"collected_count"`
	IngestedCount int `json:"ingested_count"`
	NewsIDs []string `json:"news_ids"`
	ErrorCode      string   `json:"error_code"`
	Retryable bool `json:"retryable"`
}

type FeedCollectionQualitySummary struct {
	Status string `json:"status"`
	SourceCount int `json:"source_count"`
	SuccessSourceCount int `json:"success_source_count"`
	EmptySourceCount int `json:"empty_source_count"`
	SkippedSourceCount int `json:"skipped_source_count"`
	FailedSourceCount int `json:"failed_source_count"`
	CollectedEntryCount int `json:"collected_entry_count"`
	IngestedCount int `json:"ingested_count"`
}

type FeedCollectionResult struct {
	OutputType string
	SchemaVersion string
	Payload json.RawMessage
	QualitySummary json.RawMessage
	Retryable bool
	ReasonCodes []string
}

type FeedCollectionAdapter struct {
	registrations []app.SourceEndpointRegistration
	collect *app.CollectSource
	ingest *app.IngestCollectedSource
}

func NewFeedCollectionAdapter(
	registrations []app.SourceEndpointRegistration,
	collect *app.CollectSource,
	ingest *app.IngestCollectedSource,
) (*FeedCollectionAdapter, error) {
	if len(registrations) == 0 || collect == nil || ingest == nil {
		return nil, ErrFeedCollectionAdapterRequired
	}
	owned := make([]app.SourceEndpointRegistration, len(registrations))
	copy(owned, registrations)
	sort.Slice(owned, func(i, j int) bool {
		if owned[i].SourceID != owned[j].SourceID {
			return owned[i].SourceID < owned[j].SourceID
		}
		return owned[i].EndpointID < owned[j].EndpointID
	})
	for i, registration := range owned {
		if strings.TrimSpace(registration.SourceID) == "" ||
			strings.TrimSpace(registration.EndpointID) == "" ||
			!registration.Enabled || registration.SourcePolicy.MaxRetention <= 0 ||
			(i > 0 && owned[i-1].SourceID == registration.SourceID && owned[i-1].EndpointID == registration.EndpointID) {
			return nil, ErrFeedCollectionAdapterRequired
		}
	}
	return &FeedCollectionAdapter{registrations: owned, collect: collect, ingest: ingest}, nil
}

// Execute runs only the immutable Source/Endpoint snapshot supplied at Server startup.
// It persists authorized excerpts through the unified Application ingest use case;
// StageOutput contains identifiers and counters only, never article content or URLs.
func (a *FeedCollectionAdapter) Execute(ctx context.Context, runID, stageCode string) (FeedCollectionResult, error) {
	if a == nil || a.collect == nil || a.ingest == nil || ctx == nil ||
		strings.TrimSpace(runID) == "" || stageCode != CollectFeedsStageCode {
		return FeedCollectionResult{}, ErrFeedCollectionAdapterRequired
	}
	output := FeedCollectionOutput{
		Status: "success", SourceCount: len(a.registrations),
		Sources: make([]FeedSourceOutcome, 0, len(a.registrations)),
	}
	reasonSet := make(map[string]struct{})
	retryableFailureCount := 0
	for _, registration := range a.registrations {
		if err := ctx.Err(); err != nil {
			return FeedCollectionResult{}, err
		}
		retention := defaultFeedRetention
		if registration.SourcePolicy.MaxRetention < retention {
			retention = registration.SourcePolicy.MaxRetention
		}
		outcome := FeedSourceOutcome{
			SourceID: registration.SourceID, EndpointID: registration.EndpointID,
			Status: "success", NewsIDs: []string{},
		}
		collection, err := a.collect.Execute(ctx, app.CollectSourceRequest{
			SourceID: registration.SourceID, EndpointID: registration.EndpointID,
			Purpose: domain.PurposeInternalPersonalResearch,
			ContentKind: domain.ContentAbstract, StorageMode: domain.StorageTransient,
			Retention: retention, MaxEntries: defaultFeedEntryLimit,
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return FeedCollectionResult{}, ctxErr
			}
			var collectionErr *app.SourceCollectionError
			if errors.As(err, &collectionErr) {
				outcome.ErrorCode = collectionErr.Code
				outcome.Retryable = collectionErr.Retryable
				if collectionErr.Skipped {
					outcome.Status = "skipped"
					output.SkippedSourceCount++
					reasonSet["news_feed_source_skipped"] = struct{}{}
				} else {
					outcome.Status = "failed"
					output.FailedSourceCount++
					reasonSet[feedCollectionFailureReason(collectionErr)] = struct{}{}
					if collectionErr.Retryable {
						retryableFailureCount++
					}
				}
			} else {
				outcome.Status = "failed"
				outcome.ErrorCode = "source_collection_failed"
				output.FailedSourceCount++
				reasonSet["news_feed_source_failed"] = struct{}{}
			}
			output.Sources = append(output.Sources, outcome)
			continue
		}
		outcome.CollectedCount = len(collection.Entries)
		output.CollectedEntryCount += len(collection.Entries)
		observations, err := a.ingest.Execute(ctx, app.IngestCollectedSourceRequest{
			Collection: collection,
			IdempotencyPrefix: strings.Join([]string{runID, stageCode, registration.SourceID, registration.EndpointID}, "/"),
			Purpose: domain.PurposeInternalPersonalResearch,
			Retention: retention,
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return FeedCollectionResult{}, ctxErr
			}
			outcome.Status = "failed"
			outcome.ErrorCode = "formal_ingest_failed"
			output.FailedSourceCount++
			reasonSet["news_feed_ingest_failed"] = struct{}{}
			output.Sources = append(output.Sources, outcome)
			continue
		}
		seenNewsIDs := make(map[string]struct{}, len(observations))
		for _, observation := range observations {
			if observation.NewsID == "" {
				continue
			}
			if _, exists := seenNewsIDs[observation.NewsID]; exists {
				continue
			}
			seenNewsIDs[observation.NewsID] = struct{}{}
			outcome.NewsIDs = append(outcome.NewsIDs, observation.NewsID)
		}
		sort.Strings(outcome.NewsIDs)
		outcome.IngestedCount = len(observations)
		output.IngestedCount += len(observations)
		if len(collection.Entries) == 0 {
			outcome.Status = "empty"
			output.EmptySourceCount++
		} else {
			output.SuccessSourceCount++
		}
		output.Sources = append(output.Sources, outcome)
	}

	switch {
	case output.FailedSourceCount == 0 && output.SkippedSourceCount == 0 && output.IngestedCount == 0:
		output.Status = "empty"
	case output.FailedSourceCount == 0 && output.SkippedSourceCount == 0:
		output.Status = "success"
	case output.SuccessSourceCount > 0 || output.EmptySourceCount > 0:
		output.Status = "partial"
	case output.FailedSourceCount > 0 && retryableFailureCount == output.FailedSourceCount && output.SkippedSourceCount == 0:
		output.Status = "failed_retryable"
		output.FailureCode = "news_feed_retryable_source_failure"
	case output.SkippedSourceCount == output.SourceCount:
		output.Status = "failed"
		output.FailureCode = "news_feed_all_sources_skipped"
	default:
		output.Status = "failed"
		output.FailureCode = "news_feed_collection_failed"
	}

	reasons := make([]string, 0, len(reasonSet))
	for reason := range reasonSet {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	payload, err := json.Marshal(output)
	if err != nil {
		return FeedCollectionResult{}, fmt.Errorf("marshal News Feed collection output: %w", err)
	}
	quality, err := json.Marshal(FeedCollectionQualitySummary{
		Status: output.Status, SourceCount: output.SourceCount,
		SuccessSourceCount: output.SuccessSourceCount, EmptySourceCount: output.EmptySourceCount,
		SkippedSourceCount: output.SkippedSourceCount, FailedSourceCount: output.FailedSourceCount,
		CollectedEntryCount: output.CollectedEntryCount, IngestedCount: output.IngestedCount,
	})
	if err != nil {
		return FeedCollectionResult{}, fmt.Errorf("marshal News Feed collection quality summary: %w", err)
	}
	return FeedCollectionResult{
		OutputType: CollectFeedsOutputType, SchemaVersion: CollectFeedsSchemaVersion,
		Payload: payload, QualitySummary: quality, Retryable: output.Status == "failed_retryable",
		ReasonCodes: reasons,
	}, nil
}

func feedCollectionFailureReason(err *app.SourceCollectionError) string {
	if err != nil && err.Retryable {
		return "news_feed_retryable_source_failure"
	}
	return "news_feed_source_failed"
}

func ValidateFeedCollectionPayload(raw json.RawMessage) error {
	var output FeedCollectionOutput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("News Feed payload contains trailing JSON")
	}
	if output.Status != "success" && output.Status != "empty" && output.Status != "partial" &&
		output.Status != "failed" && output.Status != "failed_retryable" {
		return errors.New("News Feed payload status is invalid")
	}
	if output.SourceCount < 1 || output.SourceCount != len(output.Sources) ||
		output.SuccessSourceCount < 0 || output.EmptySourceCount < 0 ||
		output.SkippedSourceCount < 0 || output.FailedSourceCount < 0 ||
		output.SuccessSourceCount+output.EmptySourceCount+output.SkippedSourceCount+output.FailedSourceCount != output.SourceCount ||
		output.CollectedEntryCount < 0 || output.IngestedCount < 0 {
		return errors.New("News Feed payload counters are inconsistent")
	}
	seen := make(map[string]struct{}, len(output.Sources))
	for _, source := range output.Sources {
		key := source.SourceID + "\x00" + source.EndpointID
		if source.SourceID == "" || source.EndpointID == "" || source.CollectedCount < 0 ||
			source.IngestedCount < 0 || source.NewsIDs == nil {
			return errors.New("News Feed source outcome is invalid")
		}
		if _, exists := seen[key]; exists {
			return errors.New("News Feed source outcome identity is duplicated")
		}
		seen[key] = struct{}{}
		switch source.Status {
		case "success", "empty":
			if source.ErrorCode != "" || source.Retryable {
				return errors.New("successful News Feed source carries failure metadata")
			}
		case "skipped", "failed":
			if source.ErrorCode == "" {
				return errors.New("failed News Feed source is missing error code")
			}
		default:
			return errors.New("News Feed source status is invalid")
		}
	}
	return nil
}

func ValidateFeedCollectionQualitySummary(raw json.RawMessage) error {
	var summary FeedCollectionQualitySummary
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return errors.New("News Feed quality summary contains trailing JSON")
	}
	if summary.SourceCount < 1 || summary.SuccessSourceCount < 0 || summary.EmptySourceCount < 0 ||
		summary.SkippedSourceCount < 0 || summary.FailedSourceCount < 0 ||
		summary.SuccessSourceCount+summary.EmptySourceCount+summary.SkippedSourceCount+summary.FailedSourceCount != summary.SourceCount ||
		summary.CollectedEntryCount < 0 || summary.IngestedCount < 0 {
		return errors.New("News Feed quality summary counters are inconsistent")
	}
	return nil
}
