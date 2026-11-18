package stage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/news/domain"
)

const (
	SearchCandidatesPayloadSchemaRef        = "doc/news/schema/NEWS-STAGE-search-candidates-payload-1.0.0.schema.json"
	SearchCandidatesQualitySummarySchemaRef = "doc/news/schema/NEWS-STAGE-search-candidates-quality-summary-1.0.0.schema.json"
)

// ValidatePayloadJSON is the immutable, local validator registered by the
// Server composition root for this exact News output schema version.
func ValidatePayloadJSON(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errors.New("news stage: payload must be a JSON object")
	}
	for _, key := range []string{"status", "failure_code", "candidates", "outcomes"} {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("news stage: payload field %q is required", key)
		}
	}
	var output SearchCandidatesOutput
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&output); err != nil {
		return fmt.Errorf("news stage: decode payload: %w", err)
	}
	if !validSearchStatus(output.Status) || output.Candidates == nil || output.Outcomes == nil {
		return errors.New("news stage: payload status, candidates or outcomes is invalid")
	}
	for _, candidate := range output.Candidates {
		if candidate.ObservedAt.IsZero() || candidate.MissingFields == nil {
			return errors.New("news stage: candidate observation and missing_fields are required")
		}
		if (candidate.PublishedAt != nil && candidate.PublishedAt.IsZero()) ||
			(candidate.FetchedAt != nil && candidate.FetchedAt.IsZero()) ||
			(candidate.NormalizedAt != nil && candidate.NormalizedAt.IsZero()) {
			return errors.New("news stage: candidate contains a zero optional timestamp")
		}
		for _, field := range candidate.MissingFields {
			if !validMissingField(field) {
				return fmt.Errorf("news stage: unsupported missing field %q", field)
			}
		}
	}
	for _, outcome := range output.Outcomes {
		if !validProviderStatus(outcome.Status) || outcome.CandidateCount < 0 ||
			outcome.DurationNS < 0 || outcome.ObservedAt.IsZero() {
			return errors.New("news stage: provider outcome is invalid")
		}
	}
	return nil
}

// ValidateQualitySummaryJSON validates News-owned measurable quality counters
// without assigning a cross-module quality status.
func ValidateQualitySummaryJSON(raw json.RawMessage) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		return errors.New("news stage: quality summary must be a JSON object")
	}
	for _, key := range []string{"search_status", "provider_count", "provider_success_count", "provider_empty_count", "provider_failed_count", "provider_skipped_count", "candidate_count", "candidate_missing_field_count"} {
		if _, ok := fields[key]; !ok {
			return fmt.Errorf("news stage: quality summary field %q is required", key)
		}
	}
	var summary SearchCandidatesQualitySummary
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&summary); err != nil {
		return fmt.Errorf("news stage: decode quality summary: %w", err)
	}
	if !validSearchStatus(summary.SearchStatus) || summary.ProviderCount < 0 ||
		summary.ProviderSuccessCount < 0 || summary.ProviderEmptyCount < 0 ||
		summary.ProviderFailedCount < 0 || summary.ProviderSkippedCount < 0 ||
		summary.CandidateCount < 0 || summary.CandidateMissingFieldCount == nil {
		return errors.New("news stage: quality summary counters are invalid")
	}
	if summary.ProviderSuccessCount+summary.ProviderEmptyCount+
		summary.ProviderFailedCount+summary.ProviderSkippedCount != summary.ProviderCount {
		return errors.New("news stage: provider status counters do not equal provider_count")
	}
	for field, count := range summary.CandidateMissingFieldCount {
		if !validMissingField(field) || count < 0 || count > summary.CandidateCount {
			return fmt.Errorf("news stage: invalid missing-field count for %q", field)
		}
	}
	return nil
}

func validSearchStatus(status domain.SearchStatus) bool {
	switch status {
	case domain.SearchSuccess, domain.SearchEmpty, domain.SearchPartial, domain.SearchFailed, domain.SearchSkipped:
		return true
	default:
		return false
	}
}

func validProviderStatus(status domain.ProviderStatus) bool {
	switch status {
	case domain.ProviderSuccess, domain.ProviderEmpty, domain.ProviderFailed, domain.ProviderSkipped:
		return true
	default:
		return false
	}
}

func validMissingField(field string) bool {
	switch field {
	case "title", "url", "published_at", "publisher":
		return true
	default:
		return false
	}
}
