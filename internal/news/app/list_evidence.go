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
	pages   EvidenceSnapshotPageReader
	records EvidenceRecordReader
	now     func() time.Time
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
	if err := ctx.Err(); err != nil {
		return ResearchEvidencePage{}, err
	}
	if !validEvidenceOpaqueRef(referencePage.SnapshotRef) ||
		referencePage.SystemAsOf.IsZero() || !referencePage.SystemAsOf.Equal(request.SystemAsOf) ||
		len(referencePage.Items) > request.PageSize ||
		(referencePage.NextCursor != nil &&
			(len(referencePage.Items) != request.PageSize || !validEvidencePageCursor(*referencePage.NextCursor))) {
		return ResearchEvidencePage{}, ErrEvidenceSnapshotPageUnavailable
	}
	result := NewResearchEvidencePage(referencePage.SystemAsOf)
	seen := make(map[string]struct{}, len(referencePage.Items))
	result.SnapshotRef = referencePage.SnapshotRef
	result.NextCursor = referencePage.NextCursor
	for index, item := range referencePage.Items {
		if err := ctx.Err(); err != nil {
			return ResearchEvidencePage{}, err
		}
		if !validEvidenceOpaqueRef(item.NewsID) || !validEvidenceOpaqueRef(item.VersionID) ||
			!validEvidenceOpaqueRef(item.ObservationID) ||
			item.SystemObservedAt.IsZero() || !validEvidenceUTC(item.SystemObservedAt) ||
			item.SystemObservedAt.After(referencePage.SystemAsOf) {
			return ResearchEvidencePage{}, ErrEvidenceSnapshotPageUnavailable
		}
		key := item.NewsID + "\x00" + item.VersionID + "\x00" + item.ObservationID
		if _, exists := seen[key]; exists {
			return ResearchEvidencePage{}, ErrEvidenceSnapshotPageUnavailable
		}
		seen[key] = struct{}{}
		if index > 0 && compareEvidenceSnapshotItems(referencePage.Items[index-1], item) >= 0 {
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
		if err := ctx.Err(); err != nil {
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

// validEvidencePageCursor mirrors the request boundary for cursors returned by
// the internal page reader; malformed repository output must not escape the facade.
func validEvidencePageCursor(value string) bool {
	return value != "" && len(value) <= 4096 && !hasEvidenceControl(value)
}
