package app

import (
	"context"
	"errors"
	"time"
)

var ErrEvidenceSnapshotTooLarge = errors.New("news.evidence.snapshot_too_large")

// CreateEvidenceSnapshotForQuery freezes the PIT result set before pagination.
// The persisted snapshot contains references and sort keys only, never content.
type CreateEvidenceSnapshotForQuery struct {
	query      EvidencePITRepository
	repository EvidenceSnapshotRepository
	now        func() time.Time
	newID      func() string
}

func NewCreateEvidenceSnapshotForQuery(
	query EvidencePITRepository,
	repository EvidenceSnapshotRepository,
	newID func() string,
) (*CreateEvidenceSnapshotForQuery, error) {
	if query == nil || repository == nil || newID == nil {
		return nil, ErrInvalidEvidenceSnapshot
	}
	return &CreateEvidenceSnapshotForQuery{
		query:      query,
		repository: repository,
		now:        time.Now,
		newID:      newID,
	}, nil
}

// Execute creates one immutable snapshot for a first-page request. Cursor
// continuation is deliberately handled by the reader and cannot re-run PIT.
func (u *CreateEvidenceSnapshotForQuery) Execute(
	ctx context.Context,
	request ListEvidenceRequest,
) (EvidenceSnapshot, error) {
	if u == nil || u.query == nil || u.repository == nil || u.newID == nil || ctx == nil {
		return EvidenceSnapshot{}, ErrInvalidEvidenceQuery
	}
	now := u.now().UTC()
	if request.Cursor != nil || request.Validate(now) != nil {
		return EvidenceSnapshot{}, ErrInvalidEvidenceQuery
	}

	records, err := u.query.ListEvidenceVersionsAt(ctx, request)
	if err != nil {
		return EvidenceSnapshot{}, err
	}
	if len(records) > MaximumEvidenceSnapshotSize {
		return EvidenceSnapshot{}, ErrEvidenceSnapshotTooLarge
	}

	snapshotRef := u.newID()
	fingerprint, err := evidenceFiltersFingerprint(request)
	if err != nil {
		return EvidenceSnapshot{}, ErrInvalidEvidenceQuery
	}
	items := make([]EvidenceSnapshotItem, 0, len(records))
	for _, record := range records {
		if record.Version.ID == "" ||
			record.Version.NewsID == "" ||
			record.Version.ObservationID == "" ||
			record.Version.ObservationID != record.Observation.ID ||
			record.Observation.NewsID != record.Version.NewsID ||
			record.Version.ObservedAt.IsZero() ||
			record.Observation.ObservedAt.IsZero() ||
			record.Version.ObservedAt.After(request.SystemAsOf) ||
			record.Observation.ObservedAt.After(request.SystemAsOf) {
			return EvidenceSnapshot{}, ErrInvalidEvidenceSnapshot
		}
		items = append(items, EvidenceSnapshotItem{
			NewsID:           record.Version.NewsID,
			VersionID:        record.Version.ID,
			ObservationID:    record.Observation.ID,
			SystemObservedAt: record.Version.ObservedAt.UTC(),
		})
	}

	snapshot := EvidenceSnapshot{
		SnapshotRef:           snapshotRef,
		CallerRef:             request.CallerRef,
		PurposeCode:           request.PurposeCode,
		RequestedContentLevel: request.RequestedContentLevel,
		FiltersFingerprint:    fingerprint,
		PageSize:              request.PageSize,
		SystemAsOf:            request.SystemAsOf.UTC(),
		SortVersion:           evidenceSortVersion,
		CreatedAt:             now,
		ExpiresAt:             now.Add(EvidenceSnapshotTTL),
		Items:                 items,
	}
	if err := snapshot.ValidateAt(now); err != nil {
		return EvidenceSnapshot{}, ErrInvalidEvidenceSnapshot
	}
	if err := u.repository.CreateEvidenceSnapshot(ctx, snapshot); err != nil {
		return EvidenceSnapshot{}, err
	}
	return snapshot, nil
}
