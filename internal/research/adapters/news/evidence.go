// Package news adapts the public News Evidence Query Port to Research.
package news

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

var (
	ErrInvalidRequest = errors.New("research news evidence request is invalid")
	ErrEvidenceNotPITSafe = errors.New("news evidence is not eligible for strict PIT")
	ErrEvidenceNotAuthorized = errors.New("news evidence is not currently authorized")
	ErrEvidenceMapping = errors.New("news evidence cannot be mapped to Research")
)

// ImportRequest carries trusted in-process identity and one immutable News version.
type ImportRequest struct {
	CallerRef string
	PurposeCode string
	RequestedContentLevel string
	SystemAsOf time.Time
	NewsID string
	VersionID string
}

// Importer imports one already-persisted News version; it never triggers collection.
type Importer struct {
	query newsapp.ResearchEvidenceQuery
	research researchapp.Port
}

func NewImporter(query newsapp.ResearchEvidenceQuery, research researchapp.Port) (*Importer, error) {
	if query == nil || research == nil { return nil, ErrInvalidRequest }
	return &Importer{query: query, research: research}, nil
}

func (i *Importer) ImportEvidence(ctx context.Context, request ImportRequest) (researchapp.EvidenceRecord, error) {
	if i == nil || i.query == nil || i.research == nil || ctx == nil ||
		strings.TrimSpace(request.CallerRef) == "" || !validPurpose(request.PurposeCode) ||
		!validContentLevel(request.RequestedContentLevel) || request.SystemAsOf.IsZero() ||
		!isUTC(request.SystemAsOf) || strings.TrimSpace(request.NewsID) == "" ||
		strings.TrimSpace(request.VersionID) == "" {
		return researchapp.EvidenceRecord{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil { return researchapp.EvidenceRecord{}, err }
	record, err := i.query.GetEvidence(ctx, newsapp.GetEvidenceRequest{
		CallerRef: request.CallerRef, PurposeCode: newsapp.EvidencePurposeCode(request.PurposeCode),
		RequestedContentLevel: newsapp.EvidenceContentLevel(request.RequestedContentLevel),
		SystemAsOf: request.SystemAsOf, NewsID: request.NewsID, VersionID: request.VersionID,
	})
	if err != nil { return researchapp.EvidenceRecord{}, err }
	if err := ctx.Err(); err != nil { return researchapp.EvidenceRecord{}, err }
	draft, err := mapNewsEvidence(record, request)
	if err != nil { return researchapp.EvidenceRecord{}, err }
	return i.research.AddEvidence(ctx, researchapp.AddEvidenceCommand{
		CallerRef: request.CallerRef, PurposeCode: request.PurposeCode, Draft: draft,
	})
}

// Authorizer rechecks current News authorization for every Research Evidence read/write.
// The stored AuthorizationRef is provenance, not a bearer token.
type Authorizer struct { query newsapp.ResearchEvidenceQuery }

func NewAuthorizer(query newsapp.ResearchEvidenceQuery) (*Authorizer, error) {
	if query == nil { return nil, ErrInvalidRequest }
	return &Authorizer{query: query}, nil
}

func (a *Authorizer) AuthorizeEvidence(ctx context.Context, callerRef string, purpose researchevidence.PurposeCode, value researchevidence.Evidence) error {
	if a == nil || a.query == nil || ctx == nil || strings.TrimSpace(callerRef) == "" ||
		value.SourceOwner != researchevidence.SourceNews || value.SourceVersion == nil ||
		!validPurpose(string(purpose)) || !validContentLevel(string(value.ContentLevel)) {
		return researchapp.NewAuthorizationError(researchapp.AuthorizationInternal, ErrInvalidRequest)
	}
	if err := ctx.Err(); err != nil {
		return researchapp.NewAuthorizationError(researchapp.AuthorizationTimeout, err)
	}
	record, err := a.query.GetEvidence(ctx, newsapp.GetEvidenceRequest{
		CallerRef: callerRef, PurposeCode: newsapp.EvidencePurposeCode(purpose),
		RequestedContentLevel: newsapp.EvidenceContentLevel(value.ContentLevel),
		SystemAsOf: value.SystemAsOf, NewsID: value.SourceRef, VersionID: *value.SourceVersion,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return researchapp.NewAuthorizationError(researchapp.AuthorizationTimeout, err)
		case errors.Is(err, context.Canceled), errors.Is(err, newsapp.ErrEvidenceAuthorizationUnavailable):
			return researchapp.NewAuthorizationError(researchapp.AuthorizationUnavailable, err)
		case errors.Is(err, newsapp.ErrEvidencePITNotFound):
			return researchapp.NewAuthorizationError(researchapp.AuthorizationDenied, err)
		default:
			return researchapp.NewAuthorizationError(researchapp.AuthorizationInternal, err)
		}
	}
	if record.NewsID != value.SourceRef || record.VersionID != *value.SourceVersion ||
		record.AuthorizationStatus != "allowed" || record.AuthorizationRef == "" ||
		record.SystemAsOf.IsZero() || !record.SystemAsOf.Equal(value.SystemAsOf) ||
		record.SystemObservedAt.IsZero() || !record.SystemObservedAt.Equal(value.SystemObservedAt) ||
		record.AvailableAt == nil || value.AvailableAt == nil ||
		!record.AvailableAt.Equal(*value.AvailableAt) ||
		string(record.ContentLevel) != string(value.ContentLevel) {
		return researchapp.NewAuthorizationError(researchapp.AuthorizationDenied, ErrEvidenceNotAuthorized)
	}
	return nil
}

func mapNewsEvidence(record newsapp.ResearchEvidenceRecord, request ImportRequest) (researchapp.EvidenceDraft, error) {
	if record.NewsID != request.NewsID || record.VersionID != request.VersionID ||
		record.VersionNo < 1 || record.SourceRef == "" ||
		len(record.ContentFingerprint) != 64 || strings.ToLower(record.ContentFingerprint) != record.ContentFingerprint {
		return researchapp.EvidenceDraft{}, ErrEvidenceMapping
	}
	if _, err := hex.DecodeString(record.ContentFingerprint); err != nil {
		return researchapp.EvidenceDraft{}, ErrEvidenceMapping
	}
	if record.AuthorizationStatus != "allowed" || record.AuthorizationRef == "" {
		return researchapp.EvidenceDraft{}, ErrEvidenceNotAuthorized
	}
	if record.SystemAsOf.IsZero() || !isUTC(record.SystemAsOf) ||
		!record.SystemAsOf.Equal(request.SystemAsOf) ||
		record.SystemObservedAt.IsZero() || !isUTC(record.SystemObservedAt) ||
		record.SystemObservedAt.After(request.SystemAsOf) {
		return researchapp.EvidenceDraft{}, ErrEvidenceNotPITSafe
	}
	if record.AvailableAt == nil || record.AvailableAt.IsZero() ||
		!isUTC(*record.AvailableAt) || record.AvailableAt.After(request.SystemAsOf) {
		return researchapp.EvidenceDraft{}, ErrEvidenceNotPITSafe
	}
	if record.PublishedAt != nil && (record.PublishedAt.IsZero() || !isUTC(*record.PublishedAt)) {
		return researchapp.EvidenceDraft{}, ErrEvidenceMapping
	}
	if !validContentLevel(string(record.ContentLevel)) ||
		contentLevelRank(string(record.ContentLevel)) > contentLevelRank(request.RequestedContentLevel) {
		return researchapp.EvidenceDraft{}, ErrEvidenceMapping
	}

	version := record.VersionID
	var locator *string
	if record.ContentRef != nil && strings.TrimSpace(*record.ContentRef) != "" {
		value := *record.ContentRef
		locator = &value
	}
	limitations := []string{"news_subject_refs_not_provided"}
	quality := mapNewsQuality(record.QualityStatus, record.BusinessQualityStatus)
	if quality == researchevidence.QualityDegraded {
		limitations = append(limitations, "news_source_quality_degraded_or_stale")
	}
	return researchapp.EvidenceDraft{
		EvidenceType: string(researchevidence.TypeReportedFact),
		SourceOwner: string(researchevidence.SourceNews),
		SourceRef: record.NewsID, SourceVersion: &version, SourceLocator: locator,
		DerivedFromRefs: []string{}, ClaimRef: nil, SubjectRefs: []string{},
		PublishedAt: cloneTime(record.PublishedAt), AvailableAt: cloneTime(record.AvailableAt),
		SystemObservedAt: record.SystemObservedAt.UTC(), SystemAsOf: request.SystemAsOf.UTC(),
		Stance: string(researchevidence.StanceNeutral), QualityStatus: string(quality),
		ContentLevel: string(record.ContentLevel), AuthorizationRef: record.AuthorizationRef,
		Limitations: limitations,
	}, nil
}

func mapNewsQuality(quality newsapp.EvidenceQualityStatus, business newsapp.EvidenceBusinessQualityStatus) researchevidence.QualityStatus {
	switch {
	case quality == newsapp.EvidenceQualityInvalid || business == newsapp.EvidenceBusinessQualityInvalid:
		return researchevidence.QualityInvalid
	case quality == newsapp.EvidenceQualityUnknown || business == newsapp.EvidenceBusinessQualityUnknown:
		return researchevidence.QualityUnknown
	case quality == newsapp.EvidenceQualityDegraded || quality == newsapp.EvidenceQualityStale ||
		business == newsapp.EvidenceBusinessQualityDegraded:
		return researchevidence.QualityDegraded
	case quality == newsapp.EvidenceQualityValid && business == newsapp.EvidenceBusinessQualityValid:
		return researchevidence.QualityValid
	default:
		return researchevidence.QualityUnknown
	}
}

func validPurpose(value string) bool {
	switch value {
	case string(newsapp.EvidencePurposeResearchAnalysis), string(newsapp.EvidencePurposeRiskReview), string(newsapp.EvidencePurposeUserDisplay):
		return true
	default:
		return false
	}
}

func validContentLevel(value string) bool {
	switch value {
	case string(newsapp.EvidenceContentMetadata), string(newsapp.EvidenceContentSummary),
		string(newsapp.EvidenceContentSnippet), string(newsapp.EvidenceContentFull):
		return true
	default:
		return false
	}
}

func contentLevelRank(value string) int {
	switch value {
	case string(newsapp.EvidenceContentMetadata): return 1
	case string(newsapp.EvidenceContentSummary): return 2
	case string(newsapp.EvidenceContentSnippet): return 3
	case string(newsapp.EvidenceContentFull): return 4
	default: return 0
	}
}

func isUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0 && value.Location() == time.UTC
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil { return nil }
	clone := value.UTC()
	return &clone
}

var _ researchapp.EvidenceAuthorizer = (*Authorizer)(nil)
