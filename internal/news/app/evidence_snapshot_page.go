package app

import (
	"context"
	"errors"
	"time"
)

var ErrEvidenceSnapshotPageUnavailable = errors.New("news.evidence.snapshot_page_unavailable")

// EvidenceSnapshotPage exposes immutable references only. Content disclosure is
// intentionally left to a separate authorization-aware query path.
type EvidenceSnapshotPage struct {
	Items       []EvidenceSnapshotItem
	NextCursor  *string
	SnapshotRef string
	SystemAsOf  time.Time
}

// ListEvidenceSnapshotPage composes PIT snapshot creation and cursor
// continuation without returning article content or re-running PIT on later pages.
type ListEvidenceSnapshotPage struct {
	factory    *CreateEvidenceSnapshotForQuery
	repository EvidenceSnapshotRepository
	codec      *EvidenceCursorCodec
	now        func() time.Time
}

func NewListEvidenceSnapshotPage(
	factory *CreateEvidenceSnapshotForQuery,
	repository EvidenceSnapshotRepository,
	codec *EvidenceCursorCodec,
) (*ListEvidenceSnapshotPage, error) {
	if factory == nil || repository == nil || codec == nil {
		return nil, ErrInvalidEvidenceQuery
	}
	return &ListEvidenceSnapshotPage{
		factory: factory, repository: repository, codec: codec, now: time.Now,
	}, nil
}

func (u *ListEvidenceSnapshotPage) Execute(
	ctx context.Context,
	request ListEvidenceRequest,
) (EvidenceSnapshotPage, error) {
	if u == nil || u.factory == nil || u.repository == nil || u.codec == nil ||
		u.now == nil || ctx == nil {
		return EvidenceSnapshotPage{}, ErrInvalidEvidenceQuery
	}
	now := u.now().UTC()
	if request.Validate(now) != nil {
		return EvidenceSnapshotPage{}, ErrInvalidEvidenceQuery
	}

	var snapshot EvidenceSnapshot
	var position *EvidenceCursorPosition
	if request.Cursor == nil {
		created, err := u.factory.Execute(ctx, request)
		if err != nil {
			return EvidenceSnapshotPage{}, err
		}
		snapshot = created
	} else {
		decoded, err := u.codec.DecodeCursor(*request.Cursor, request, now)
		if err != nil {
			return EvidenceSnapshotPage{}, err
		}
		loaded, err := u.repository.GetEvidenceSnapshot(ctx, decoded.SnapshotRef, request.CallerRef, now)
		if err != nil {
			return EvidenceSnapshotPage{}, err
		}
		if !loaded.MatchesRequest(request) || loaded.SortVersion != evidenceSortVersion {
			return EvidenceSnapshotPage{}, ErrEvidenceSnapshotPageUnavailable
		}
		snapshot = loaded
		position = &decoded
	}

	pageItems, hasMore, err := snapshot.PageAfter(position)
	if err != nil {
		return EvidenceSnapshotPage{}, err
	}
	page := EvidenceSnapshotPage{
		Items:       pageItems,
		SnapshotRef: snapshot.SnapshotRef,
		SystemAsOf:  snapshot.SystemAsOf,
	}
	if hasMore {
		if len(pageItems) == 0 {
			return EvidenceSnapshotPage{}, ErrInvalidEvidenceSnapshot
		}
		last := pageItems[len(pageItems)-1]
		token, err := u.codec.EncodeNextCursor(request, snapshot.SnapshotRef, ResearchEvidenceRecord{
			NewsID:           last.NewsID,
			VersionID:        last.VersionID,
			SystemObservedAt: last.SystemObservedAt,
			SystemAsOf:       snapshot.SystemAsOf,
		}, now)
		if err != nil {
			return EvidenceSnapshotPage{}, err
		}
		page.NextCursor = &token
	}
	return page, nil
}
