// Package stage adapts News Application use cases to versioned Researchflow outputs
// without introducing a dependency from News to Researchflow.
package stage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

const (
	SearchCandidatesStageCode = "news.search_candidates"
	SearchCandidatesOutputType = "news.search_candidates"
	SearchCandidatesSchemaVersion = "1.0.0"
)

var ErrSearchUseCaseRequired = errors.New("news stage: SearchCandidates use case is required")

// SearchCandidatesOutput is the stable, JSON-facing representation of one
// News discovery execution. It deliberately represents discovery candidates,
// not Research Evidence or verified facts.
type SearchCandidatesOutput struct {
	Status      domain.SearchStatus `json:"status"`
	FailureCode string              `json:"failure_code"`
	Candidates  []Candidate         `json:"candidates"`
	Outcomes    []ProviderOutcome   `json:"outcomes"`
}

type Candidate struct {
	ProviderCode    string     `json:"provider_code"`
	ProviderItemID  string     `json:"provider_item_id"`
	SourceItemID    string     `json:"source_item_id"`
	PublisherName   string     `json:"publisher_name"`
	Author          string     `json:"author"`
	PublisherDomain string     `json:"publisher_domain"`
	Title           string     `json:"title"`
	ProviderSummary string     `json:"provider_summary"`
	RawURL          string     `json:"raw_url"`
	CanonicalURL    string     `json:"canonical_url"`
	PublishedAtRaw  string     `json:"published_at_raw"`
	PublishedAt     *time.Time `json:"published_at"`
	FetchedAt       *time.Time `json:"fetched_at"`
	NormalizedAt    *time.Time `json:"normalized_at"`
	ObservedAt      time.Time  `json:"observed_at"`
	QueryDimension  string     `json:"query_dimension"`
	MissingFields   []string   `json:"missing_fields"`
}

type ProviderOutcome struct {
	ProviderCode   string                `json:"provider_code"`
	QueryDimension string                `json:"query_dimension"`
	Status         domain.ProviderStatus `json:"status"`
	CandidateCount int                   `json:"candidate_count"`
	ErrorCode      string                `json:"error_code"`
	Retryable      bool                  `json:"retryable"`
	DurationNS     int64                 `json:"duration_ns"`
	ObservedAt     time.Time             `json:"observed_at"`
}

// SearchCandidatesQualitySummary reports only directly measurable execution
// facts. It does not invent a global quality_status or infer evidence validity.
type SearchCandidatesQualitySummary struct {
	SearchStatus               domain.SearchStatus `json:"search_status"`
	ProviderCount              int                 `json:"provider_count"`
	ProviderSuccessCount       int                 `json:"provider_success_count"`
	ProviderEmptyCount         int                 `json:"provider_empty_count"`
	ProviderFailedCount        int                 `json:"provider_failed_count"`
	ProviderSkippedCount       int                 `json:"provider_skipped_count"`
	CandidateCount             int                 `json:"candidate_count"`
	CandidateMissingFieldCount map[string]int      `json:"candidate_missing_field_count"`
}

// SearchCandidatesResult is the News-owned payload/quality pair consumed by
// the Server-side Researchflow adapter.
type SearchCandidatesResult struct {
	OutputType     string
	SchemaVersion  string
	Payload        json.RawMessage
	QualitySummary json.RawMessage
}

type SearchCandidatesAdapter struct {
	search *app.SearchCandidates
}

func NewSearchCandidatesAdapter(search *app.SearchCandidates) (*SearchCandidatesAdapter, error) {
	if search == nil {
		return nil, ErrSearchUseCaseRequired
	}
	return &SearchCandidatesAdapter{search: search}, nil
}

// DecodeParameters decodes the frozen Stage input shape and rejects unknown
// fields or trailing JSON values before the Application use case is invoked.
func DecodeParameters(raw json.RawMessage) (domain.SearchRequest, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return domain.SearchRequest{}, errors.New("news stage: parameters must be a JSON object")
	}
	var request domain.SearchRequest
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return domain.SearchRequest{}, fmt.Errorf("news stage: invalid parameters: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return domain.SearchRequest{}, errors.New("news stage: parameters contain trailing JSON")
	}
	return request, nil
}

func (a *SearchCandidatesAdapter) ExecuteParameters(ctx context.Context, raw json.RawMessage) (SearchCandidatesResult, error) {
	request, err := DecodeParameters(raw)
	if err != nil {
		return SearchCandidatesResult{}, err
	}
	return a.Execute(ctx, request)
}

func (a *SearchCandidatesAdapter) Execute(ctx context.Context, request domain.SearchRequest) (SearchCandidatesResult, error) {
	if a == nil || a.search == nil {
		return SearchCandidatesResult{}, ErrSearchUseCaseRequired
	}
	result := a.search.Execute(ctx, request)
	payload, err := marshalSearchCandidates(result)
	if err != nil {
		return SearchCandidatesResult{}, fmt.Errorf("marshal News search payload: %w", err)
	}
	summary, err := marshalSearchCandidatesQualitySummary(result)
	if err != nil {
		return SearchCandidatesResult{}, fmt.Errorf("marshal News search quality summary: %w", err)
	}
	return SearchCandidatesResult{
		OutputType: SearchCandidatesOutputType, SchemaVersion: SearchCandidatesSchemaVersion,
		Payload: payload, QualitySummary: summary,
	}, nil
}

func marshalSearchCandidates(result domain.SearchResult) (json.RawMessage, error) {
	payload := SearchCandidatesOutput{
		Status: result.Status, FailureCode: result.FailureCode,
		Candidates: make([]Candidate, 0, len(result.Candidates)),
		Outcomes: make([]ProviderOutcome, 0, len(result.Outcomes)),
	}
	for _, candidate := range result.Candidates {
		payload.Candidates = append(payload.Candidates, Candidate{
			ProviderCode: candidate.ProviderCode, ProviderItemID: candidate.ProviderItemID,
			SourceItemID: candidate.SourceItemID, PublisherName: candidate.PublisherName,
			Author: candidate.Author, PublisherDomain: candidate.PublisherDomain,
			Title: candidate.Title, ProviderSummary: candidate.ProviderSummary,
			RawURL: candidate.RawURL, CanonicalURL: candidate.CanonicalURL,
			PublishedAtRaw: candidate.PublishedAtRaw, PublishedAt: toUTC(candidate.PublishedAt),
			FetchedAt: toUTC(candidate.FetchedAt), NormalizedAt: toUTC(candidate.NormalizedAt),
			ObservedAt: candidate.ObservedAt.UTC(), QueryDimension: candidate.QueryDimension,
			MissingFields: append([]string{}, candidate.MissingFields...),
		})
	}
	for _, outcome := range result.Outcomes {
		payload.Outcomes = append(payload.Outcomes, ProviderOutcome{
			ProviderCode: outcome.ProviderCode, QueryDimension: outcome.QueryDimension,
			Status: outcome.Status, CandidateCount: outcome.CandidateCount,
			ErrorCode: outcome.ErrorCode, Retryable: outcome.Retryable,
			DurationNS: outcome.Duration.Nanoseconds(), ObservedAt: outcome.ObservedAt.UTC(),
		})
	}
	raw, err := json.Marshal(payload)
	return raw, err
}

func marshalSearchCandidatesQualitySummary(result domain.SearchResult) (json.RawMessage, error) {
	summary := SearchCandidatesQualitySummary{
		SearchStatus: result.Status, ProviderCount: len(result.Outcomes),
		CandidateCount: len(result.Candidates), CandidateMissingFieldCount: map[string]int{},
	}
	for _, outcome := range result.Outcomes {
		switch outcome.Status {
		case domain.ProviderSuccess:
			summary.ProviderSuccessCount++
		case domain.ProviderEmpty:
			summary.ProviderEmptyCount++
		case domain.ProviderFailed:
			summary.ProviderFailedCount++
		case domain.ProviderSkipped:
			summary.ProviderSkippedCount++
		}
	}
	for _, candidate := range result.Candidates {
		for _, field := range candidate.MissingFields {
			summary.CandidateMissingFieldCount[field]++
		}
	}
	raw, err := json.Marshal(summary)
	return raw, err
}

func toUTC(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	converted := value.UTC()
	return &converted
}
