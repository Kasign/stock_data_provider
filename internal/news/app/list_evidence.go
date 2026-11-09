package app

import (
	"context"
	"time"
)

// EvidenceSnapshotPageReader produces immutable PIT references and a cursor page.
type EvidenceSnapshotPageReader interface {
	Execute(context.Context, ListEvidenceRequest) (EvidenceSnapshotPage, error)
}

// EvidenceRecordReader resolves and authorizes one immutable evidence reference.
type EvidenceRecordReader interface {
	Execute(context.Context, GetEvidenceRequest) (ResearchEvidenceRecord, error)
}

// ListEvidence composes reference-only PIT pagination with per-item current authorization.
// A single item failure aborts the whole page so callers never receive partial disclosure.
type ListEvidence struct {
	pages EvidenceSnapshotPageReader
	records EvidenceRecordReader
	now func() time.Time
}

// NewListEvidence creates the authorized content page facade.
func NewListEvidence(pages EvidenceSnapshotPageReader, records EvidenceRecordReader) (*ListEvidence, error) {
	if pages == nil || records == nil {
		return nil, ErrInvalidEvidenceQuery
	}
	return &ListEvidence{pages: pages, records: records, now: time.Now}, nil
}

// Execute resolves every reference in the immutable snapshot page under the same
// caller, purpose, content ceiling, and system-as-of boundary.
func (u *ListEvidence) Execute(ctx context.Context, request ListEvidenceRequest) (ResearchEvidencePage, error) {
	if u == nil || u.pages == nil || u.records == nil || u.now == nil || ctx == nil {
		return ResearchEvidencePage{}, ErrInvalidEvidenceQuery
	}
	if err := ctx.Err(); err != nil {
		return ResearchEvidencePage{}, err
	}
	if request.Validate(u.now().UTC()) != nil {
		return ResearchEvidencePage{}, ErrInvalidEvidenceQuery
	}
	referencePage, err := u.pages.Execute(ctx, request)
	if err != nil {
		return ResearchEvidencePage{}, err
	}
	if referencePage.SnapshotRef == "" || referencePage.SystemAsOf.IsZero() ||
		!referencePage.SystemAsOf.Equal(request.SystemAsOf) {
		return ResearchEvidencePage{}, ErrEvidenceSnapshotPageUnavailable
	}
	result := NewResearchEvidencePage(referencePage.SystemAsOf)
	result.SnapshotRef = referencePage.SnapshotRef
	result.NextCursor = referencePage.NextCursor
	for _, item := range referencePage.Items {
		if item.NewsID == "" || item.VersionID == "" || item.ObservationID == "" ||
			item.SystemObservedAt.IsZero() || !validEvidenceUTC(item.SystemObservedAt) ||
			item.SystemObservedAt.After(referencePage.SystemAsOf) {
			return ResearchEvidencePage{}, ErrEvidenceSnapshotPageUnavailable
		}
		record, err := u.records.Execute(ctx, GetEvidenceRequest{
			CallerRef: request.CallerRef, PurposeCode: request.PurposeCode,
			RequestedContentLevel: request.RequestedContentLevel,
			SystemAsOf: referencePage.SystemAsOf, NewsID: item.NewsID, VersionID: item.VersionID,
		})
		if err != nil {
			return ResearchEvidencePage{}, err
		}
		if record.NewsID != item.NewsID || record.VersionID != item.VersionID ||
			record.ObservationID != item.ObservationID ||
			!record.SystemObservedAt.Equal(item.SystemObservedAt) ||
			!record.SystemAsOf.Equal(referencePage.SystemAsOf) {
			return ResearchEvidencePage{}, ErrEvidenceSnapshotPageUnavailable
		}
		result.Items = append(result.Items, record)
	}
	return result, nil
}
