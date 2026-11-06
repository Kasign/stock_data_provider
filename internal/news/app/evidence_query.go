package app

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"
)

type EvidencePurposeCode string

const (
	EvidencePurposeResearchAnalysis EvidencePurposeCode = "research_analysis"
	EvidencePurposeRiskReview       EvidencePurposeCode = "risk_review"
	EvidencePurposeUserDisplay      EvidencePurposeCode = "user_display"
)

type EvidenceContentLevel string

const (
	EvidenceContentMetadata EvidenceContentLevel = "metadata"
	EvidenceContentSummary  EvidenceContentLevel = "summary"
	EvidenceContentSnippet  EvidenceContentLevel = "snippet"
	EvidenceContentFull     EvidenceContentLevel = "full"
)

type EvidenceBusinessQualityStatus string

const (
	EvidenceBusinessQualityValid    EvidenceBusinessQualityStatus = "valid"
	EvidenceBusinessQualityDegraded EvidenceBusinessQualityStatus = "degraded"
	EvidenceBusinessQualityInvalid  EvidenceBusinessQualityStatus = "invalid"
	EvidenceBusinessQualityUnknown  EvidenceBusinessQualityStatus = "unknown"
)

type EvidenceQualityStatus string

const (
	EvidenceQualityValid    EvidenceQualityStatus = "valid"
	EvidenceQualityDegraded EvidenceQualityStatus = "degraded"
	EvidenceQualityStale    EvidenceQualityStatus = "stale"
	EvidenceQualityInvalid  EvidenceQualityStatus = "invalid"
	EvidenceQualityUnknown  EvidenceQualityStatus = "unknown"
)

const (
	evidenceSortVersion = "news-evidence-v1"
	evidenceCursorTTL   = 15 * time.Minute
)

var ErrInvalidEvidenceQuery = errors.New("news.evidence.invalid_query")

// EvidenceQueryFilters is a closed filter set. It deliberately has no SQL,
// field-name, URL, or caller-supplied ordering fields.
type EvidenceQueryFilters struct {
	NewsIDs         	[]string               	`json:"news_ids,omitempty"`
	SourceRefs      	[]string               	`json:"source_refs,omitempty"`
	CanonicalSymbols	[]string               	`json:"canonical_symbols,omitempty"`
	PublishedAtFrom 	*time.Time             	`json:"published_at_from,omitempty"`
	PublishedAtTo   	*time.Time             	`json:"published_at_to,omitempty"`
	QualityStatuses 	[]EvidenceQualityStatus	`json:"quality_statuses,omitempty"`
}

// ListEvidenceRequest is an in-process Application request. CallerRef must be
// injected from trusted call context; it must never be copied from an untrusted
// public HTTP request.
type ListEvidenceRequest struct {
	CallerRef            	string               	`json:"caller_ref"`
	PurposeCode          	EvidencePurposeCode  	`json:"purpose_code"`
	RequestedContentLevel	EvidenceContentLevel 	`json:"requested_content_level"`
	SystemAsOf           	time.Time            	`json:"system_as_of"`
	Filters              	*EvidenceQueryFilters	`json:"filters"`
	BusinessTimeFrom     	*time.Time           	`json:"business_time_from,omitempty"`
	BusinessTimeTo       	*time.Time           	`json:"business_time_to,omitempty"`
	PageSize             	int                  	`json:"page_size"`
	Cursor               	*string              	`json:"cursor,omitempty"`
}

// GetEvidenceRequest identifies one News Item and an optional immutable
// version. SystemAsOf is mandatory even when VersionID is supplied.
type GetEvidenceRequest struct {
	CallerRef            	string              	`json:"caller_ref"`
	PurposeCode          	EvidencePurposeCode 	`json:"purpose_code"`
	RequestedContentLevel	EvidenceContentLevel	`json:"requested_content_level"`
	SystemAsOf           	time.Time           	`json:"system_as_of"`
	NewsID               	string              	`json:"news_id"`
	VersionID            	string              	`json:"version_id,omitempty"`
}

// ResearchEvidenceRecord is the stable, read-only DTO consumed by Research.
// Nullable source times remain nil when News has no verifiable evidence.
type ResearchEvidenceRecord struct {
	NewsID               	string                       	`json:"news_id"`
	ObservationID        	string                       	`json:"observation_id"`
	VersionID            	string                       	`json:"version_id"`
	VersionNo            	int                          	`json:"version_no"`
	ContentFingerprint   	string                       	`json:"content_fingerprint"`
	SourceRef            	string                       	`json:"source_ref"`
	ProviderRef          	*string                      	`json:"provider_ref,omitempty"`
	IdentityStatus       	string                       	`json:"identity_status"`
	BusinessQualityStatus	EvidenceBusinessQualityStatus	`json:"business_quality_status"`
	QualityStatus        	EvidenceQualityStatus        	`json:"quality_status"`
	Title                	*string                      	`json:"title,omitempty"`
	PublisherName        	*string                      	`json:"publisher_name,omitempty"`
	PublisherDomain      	*string                      	`json:"publisher_domain,omitempty"`
	PublishedAt          	*time.Time                   	`json:"published_at"`
	PublishedAtRaw       	*string                      	`json:"published_at_raw,omitempty"`
	AvailableAt          	*time.Time                   	`json:"available_at"`
	SystemObservedAt     	time.Time                    	`json:"system_observed_at"`
	FetchedAt            	*time.Time                   	`json:"fetched_at,omitempty"`
	NormalizedAt         	*time.Time                   	`json:"normalized_at,omitempty"`
	ContentLevel         	EvidenceContentLevel         	`json:"content_level"`
	ContentRef           	*string                      	`json:"content_ref,omitempty"`
	ContentExcerpt       	*string                      	`json:"content_excerpt,omitempty"`
	AuthorizationRef     	string                       	`json:"authorization_ref"`
	AuthorizationStatus  	string                       	`json:"authorization_status"`
	SystemAsOf           	time.Time                    	`json:"system_as_of"`
	RelationRefs         	[]string                     	`json:"relation_refs"`
}

type ResearchEvidencePage struct {
	Items      	[]ResearchEvidenceRecord	`json:"items"`
	NextCursor 	*string                 	`json:"next_cursor"`
	SnapshotRef	string                  	`json:"snapshot_ref"`
	SystemAsOf 	time.Time               	`json:"system_as_of"`
	Diagnostics	[]string                	`json:"diagnostics"`
}

func NewResearchEvidencePage(systemAsOf time.Time) ResearchEvidencePage {
	return ResearchEvidencePage{
		Items:       make([]ResearchEvidenceRecord, 0),
		Diagnostics: make([]string, 0),
		SystemAsOf:  systemAsOf.UTC(),
	}
}

// ResearchEvidenceQuery is the only cross-module read port. It never searches
// Providers or triggers source collection.
type ResearchEvidenceQuery interface {
	GetEvidence(context.Context, GetEvidenceRequest) (ResearchEvidenceRecord, error)
	ListEvidence(context.Context, ListEvidenceRequest) (ResearchEvidencePage, error)
}

func (r GetEvidenceRequest) Validate(now time.Time) error {
	if !validEvidenceCaller(r.CallerRef) ||
		!validEvidencePurpose(r.PurposeCode) ||
		!validEvidenceContentLevel(r.RequestedContentLevel) ||
		!validEvidenceTime(r.SystemAsOf, now) ||
		!validEvidenceOpaqueRef(r.NewsID) ||
		(r.VersionID != "" && !validEvidenceOpaqueRef(r.VersionID)) {
		return ErrInvalidEvidenceQuery
	}
	return nil
}

func (r ListEvidenceRequest) Validate(now time.Time) error {
	if !validEvidenceCaller(r.CallerRef) ||
		!validEvidencePurpose(r.PurposeCode) ||
		!validEvidenceContentLevel(r.RequestedContentLevel) ||
		!validEvidenceTime(r.SystemAsOf, now) ||
		r.Filters == nil ||
		r.PageSize < 1 || r.PageSize > 100 ||
		(r.Cursor != nil && (*r.Cursor == "" || len(*r.Cursor) > 4096 || hasEvidenceControl(*r.Cursor))) {
		return ErrInvalidEvidenceQuery
	}
	filters := r.Filters
	if !validEvidenceRefList(filters.NewsIDs, 100) ||
		!validEvidenceRefList(filters.SourceRefs, 50) ||
		!validEvidenceRefList(filters.CanonicalSymbols, 100) ||
		!validEvidenceQualityList(filters.QualityStatuses) ||
		!validEvidenceOptionalTime(filters.PublishedAtFrom) ||
		!validEvidenceOptionalTime(filters.PublishedAtTo) ||
		!validEvidenceOptionalTime(r.BusinessTimeFrom) ||
		!validEvidenceOptionalTime(r.BusinessTimeTo) {
		return ErrInvalidEvidenceQuery
	}
	if !orderedEvidenceTimes(filters.PublishedAtFrom, filters.PublishedAtTo) ||
		!orderedEvidenceTimes(r.BusinessTimeFrom, r.BusinessTimeTo) {
		return ErrInvalidEvidenceQuery
	}
	return nil
}

func validEvidenceCaller(value string) bool {
	return validEvidenceOpaqueRef(value)
}

func validEvidenceOpaqueRef(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 256 && !hasEvidenceControl(value)
}

func validEvidencePurpose(value EvidencePurposeCode) bool {
	switch value {
	case EvidencePurposeResearchAnalysis, EvidencePurposeRiskReview, EvidencePurposeUserDisplay:
		return true
	default:
		return false
	}
}

func validEvidenceContentLevel(value EvidenceContentLevel) bool {
	switch value {
	case EvidenceContentMetadata, EvidenceContentSummary, EvidenceContentSnippet, EvidenceContentFull:
		return true
	default:
		return false
	}
}

func validEvidenceQualityList(values []EvidenceQualityStatus) bool {
	seen := make(map[EvidenceQualityStatus]struct{}, len(values))
	for _, value := range values {
		switch value {
		case EvidenceQualityValid, EvidenceQualityDegraded, EvidenceQualityStale, EvidenceQualityInvalid, EvidenceQualityUnknown:
		default:
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validEvidenceRefList(values []string, maximum int) bool {
	if len(values) > maximum {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !validEvidenceOpaqueRef(value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}

func validEvidenceTime(value, now time.Time) bool {
	return !value.IsZero() && !value.After(now) && validEvidenceUTC(value)
}

func validEvidenceOptionalTime(value *time.Time) bool {
	return value == nil || (!value.IsZero() && validEvidenceUTC(*value))
}

func validEvidenceUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}

func orderedEvidenceTimes(from, to *time.Time) bool {
	return from == nil || to == nil || !from.After(*to)
}

func hasEvidenceControl(value string) bool {
	return strings.IndexFunc(value, unicode.IsControl) >= 0
}
