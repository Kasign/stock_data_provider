package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

const (
	EvidenceSnapshotTTL         = 15 * time.Minute
	MaximumEvidenceSnapshotSize = 5000
)

var (
	ErrInvalidEvidenceSnapshot    = errors.New("news.evidence.snapshot_invalid")
	ErrEvidenceSnapshotConflict   = errors.New("news.evidence.snapshot_conflict")
	ErrEvidenceSnapshotUnavailable = errors.New("news.evidence.snapshot_unavailable")
)

type EvidenceSnapshotItem struct {
	NewsID           string    `json:"news_id"`
	VersionID        string    `json:"version_id"`
	ObservationID    string    `json:"observation_id"`
	SystemObservedAt time.Time `json:"system_observed_at"`
}

// EvidenceSnapshot stores only immutable evidence references and sort keys.
// It never copies titles, excerpts, summaries, or article bodies.
type EvidenceSnapshot struct {
	SnapshotRef           string                 `json:"snapshot_ref"`
	CallerRef             string                 `json:"caller_ref"`
	PurposeCode           EvidencePurposeCode    `json:"purpose_code"`
	RequestedContentLevel EvidenceContentLevel   `json:"requested_content_level"`
	FiltersFingerprint    string                 `json:"filters_fingerprint"`
	PageSize              int                    `json:"page_size"`
	SystemAsOf            time.Time              `json:"system_as_of"`
	SortVersion           string                 `json:"sort_version"`
	CreatedAt             time.Time              `json:"created_at"`
	ExpiresAt             time.Time              `json:"expires_at"`
	Items                 []EvidenceSnapshotItem `json:"items"`
}

func (s EvidenceSnapshot) Validate() error {
	if !validEvidenceOpaqueRef(s.SnapshotRef) ||
		!validEvidenceCaller(s.CallerRef) ||
		!validEvidencePurpose(s.PurposeCode) ||
		!validEvidenceContentLevel(s.RequestedContentLevel) ||
		len(s.FiltersFingerprint) != sha256.Size*2 || s.FiltersFingerprint != strings.ToLower(s.FiltersFingerprint) ||
		s.PageSize < 1 || s.PageSize > 100 ||
		s.SortVersion != evidenceSortVersion ||
		s.SystemAsOf.IsZero() || !validEvidenceUTC(s.SystemAsOf) ||
		s.CreatedAt.IsZero() || !validEvidenceUTC(s.CreatedAt) || s.SystemAsOf.After(s.CreatedAt) ||
		s.ExpiresAt.IsZero() || !validEvidenceUTC(s.ExpiresAt) ||
		!s.ExpiresAt.Equal(s.CreatedAt.Add(EvidenceSnapshotTTL)) ||
		s.Items == nil || len(s.Items) > MaximumEvidenceSnapshotSize {
		return ErrInvalidEvidenceSnapshot
	}
	if _, err := hex.DecodeString(s.FiltersFingerprint); err != nil {
		return ErrInvalidEvidenceSnapshot
	}
	seen := make(map[string]struct{}, len(s.Items))
	for index, item := range s.Items {
		if !validEvidenceOpaqueRef(item.NewsID) ||
			!validEvidenceOpaqueRef(item.VersionID) ||
			!validEvidenceOpaqueRef(item.ObservationID) ||
			item.SystemObservedAt.IsZero() || !validEvidenceUTC(item.SystemObservedAt) ||
			item.SystemObservedAt.After(s.SystemAsOf) {
			return ErrInvalidEvidenceSnapshot
		}
		key := item.NewsID + "\x00" + item.VersionID + "\x00" + item.ObservationID
		if _, exists := seen[key]; exists {
			return ErrInvalidEvidenceSnapshot
		}
		seen[key] = struct{}{}
		if index > 0 && compareEvidenceSnapshotItems(s.Items[index-1], item) >= 0 {
			return ErrInvalidEvidenceSnapshot
		}
	}
	return nil
}

// MatchesRequest ensures a stored snapshot is reused only for the exact query
// security scope and pagination contract that created it. The caller must
// validate the request before using this comparison.
func (s EvidenceSnapshot) MatchesRequest(request ListEvidenceRequest) bool {
	if s.Validate() != nil || request.Filters == nil ||
		!validEvidenceCaller(request.CallerRef) ||
		!validEvidencePurpose(request.PurposeCode) ||
		!validEvidenceContentLevel(request.RequestedContentLevel) ||
		request.PageSize < 1 || request.PageSize > 100 ||
		request.SystemAsOf.IsZero() || !validEvidenceUTC(request.SystemAsOf) {
		return false
	}
	fingerprint, err := evidenceFiltersFingerprint(request)
	return err == nil &&
		s.CallerRef == request.CallerRef &&
		s.PurposeCode == request.PurposeCode &&
		s.RequestedContentLevel == request.RequestedContentLevel &&
		s.FiltersFingerprint == fingerprint &&
		s.PageSize == request.PageSize &&
		s.SystemAsOf.Equal(request.SystemAsOf)
}

func (s EvidenceSnapshot) ValidateAt(now time.Time) error {
	if s.Validate() != nil || now.IsZero() || !validEvidenceUTC(now) ||
		s.CreatedAt.After(now) || s.SystemAsOf.After(now) {
		return ErrInvalidEvidenceSnapshot
	}
	return nil
}

// PageAfter applies the immutable sort-key continuation. A cursor position
// that is not part of this exact snapshot is rejected rather than treated as an offset.
func (s EvidenceSnapshot) PageAfter(position *EvidenceCursorPosition) ([]EvidenceSnapshotItem, bool, error) {
	if s.Validate() != nil {
		return nil, false, ErrInvalidEvidenceSnapshot
	}
	start := 0
	if position != nil {
		if position.SnapshotRef != s.SnapshotRef ||
			position.LastSystemObservedAt.IsZero() ||
			!validEvidenceUTC(position.LastSystemObservedAt) {
			return nil, false, ErrInvalidEvidenceSnapshot
		}
		found := false
		for index, item := range s.Items {
			if item.NewsID == position.LastNewsID &&
				item.VersionID == position.LastVersionID &&
				item.SystemObservedAt.Equal(position.LastSystemObservedAt) {
				start = index + 1
				found = true
				break
			}
		}
		if !found {
			return nil, false, ErrInvalidEvidenceSnapshot
		}
	}
	end := start + s.PageSize
	if end > len(s.Items) {
		end = len(s.Items)
	}
	page := append([]EvidenceSnapshotItem(nil), s.Items[start:end]...)
	return page, end < len(s.Items), nil
}

func compareEvidenceSnapshotItems(a, b EvidenceSnapshotItem) int {
	if !a.SystemObservedAt.Equal(b.SystemObservedAt) {
		if a.SystemObservedAt.After(b.SystemObservedAt) {
			return -1
		}
		return 1
	}
	if a.NewsID != b.NewsID {
		if a.NewsID < b.NewsID {
			return -1
		}
		return 1
	}
	if a.VersionID != b.VersionID {
		if a.VersionID < b.VersionID {
			return -1
		}
		return 1
	}
	return 0
}

type EvidenceSnapshotRepository interface {
	CreateEvidenceSnapshot(context.Context, EvidenceSnapshot) error
	GetEvidenceSnapshot(context.Context, string, string, time.Time) (EvidenceSnapshot, error)
	DeleteExpiredEvidenceSnapshots(context.Context, time.Time, int) (int64, error)
}

type SaveEvidenceSnapshot struct {
	repository EvidenceSnapshotRepository
}

func NewSaveEvidenceSnapshot(repository EvidenceSnapshotRepository) (*SaveEvidenceSnapshot, error) {
	if repository == nil {
		return nil, ErrInvalidEvidenceSnapshot
	}
	return &SaveEvidenceSnapshot{repository: repository}, nil
}

func (s *SaveEvidenceSnapshot) Execute(ctx context.Context, snapshot EvidenceSnapshot) error {
	if s == nil || s.repository == nil || ctx == nil || snapshot.ValidateAt(time.Now().UTC()) != nil {
		return ErrInvalidEvidenceSnapshot
	}
	return s.repository.CreateEvidenceSnapshot(ctx, snapshot)
}
