package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type evidenceSnapshotRepositoryStub struct {
	created     EvidenceSnapshot
	calls       int
	deleted     int64
	deleteErr   error
	deleteNow   time.Time
	deleteLimit int
	deleteCalls int
	afterDelete func()
}

func (s *evidenceSnapshotRepositoryStub) CreateEvidenceSnapshot(_ context.Context, snapshot EvidenceSnapshot) error {
	s.calls++
	s.created = snapshot
	return nil
}

func (s *evidenceSnapshotRepositoryStub) GetEvidenceSnapshot(context.Context, string, string, time.Time) (EvidenceSnapshot, error) {
	return EvidenceSnapshot{}, ErrEvidenceSnapshotUnavailable
}

func (s *evidenceSnapshotRepositoryStub) DeleteExpiredEvidenceSnapshots(_ context.Context, now time.Time, limit int) (int64, error) {
	s.deleteCalls++
	s.deleteNow, s.deleteLimit = now, limit
	if s.afterDelete != nil {
		s.afterDelete()
	}
	return s.deleted, s.deleteErr
}

func validEvidenceSnapshot(now time.Time) EvidenceSnapshot {
	asOf := now.Add(-time.Minute)
	return EvidenceSnapshot{
		SnapshotRef:           "snapshot-1",
		CallerRef:             "research",
		PurposeCode:           EvidencePurposeResearchAnalysis,
		RequestedContentLevel: EvidenceContentSummary,
		FiltersFingerprint:    strings.Repeat("a", 64),
		PageSize:              20,
		SystemAsOf:            asOf,
		SortVersion:           evidenceSortVersion,
		CreatedAt:             now,
		ExpiresAt:             now.Add(EvidenceSnapshotTTL),
		Items: []EvidenceSnapshotItem{
			{NewsID: "news-2", VersionID: "version-1", ObservationID: "observation-2", SystemObservedAt: asOf.Add(-time.Minute)},
			{NewsID: "news-1", VersionID: "version-2", ObservationID: "observation-1", SystemObservedAt: asOf.Add(-2 * time.Minute)},
			{NewsID: "news-3", VersionID: "version-3", ObservationID: "observation-3", SystemObservedAt: asOf.Add(-3 * time.Minute)},
		},
	}
}

func TestEvidenceSnapshotValidateAcceptsStableReferenceOnlySnapshot(t *testing.T) {
	snapshot := validEvidenceSnapshot(time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEvidenceSnapshotMatchesExactQueryContext(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	snapshot := validEvidenceSnapshot(now)
	request := ListEvidenceRequest{
		CallerRef:             snapshot.CallerRef,
		PurposeCode:           snapshot.PurposeCode,
		RequestedContentLevel: snapshot.RequestedContentLevel,
		SystemAsOf:            snapshot.SystemAsOf,
		Filters:               &EvidenceQueryFilters{},
		PageSize:              snapshot.PageSize,
	}
	fingerprint, err := evidenceFiltersFingerprint(request)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.FiltersFingerprint = fingerprint
	if !snapshot.MatchesRequest(request) {
		t.Fatal("exact query context did not match snapshot")
	}

	cases := []struct {
		name   string
		mutate func(*ListEvidenceRequest)
	}{
		{"caller", func(r *ListEvidenceRequest) { r.CallerRef = "service:other" }},
		{"purpose", func(r *ListEvidenceRequest) { r.PurposeCode = EvidencePurposeRiskReview }},
		{"content level", func(r *ListEvidenceRequest) { r.RequestedContentLevel = EvidenceContentMetadata }},
		{"filters", func(r *ListEvidenceRequest) { r.Filters.NewsIDs = []string{"news-other"} }},
		{"page size", func(r *ListEvidenceRequest) { r.PageSize++ }},
		{"system as of", func(r *ListEvidenceRequest) { r.SystemAsOf = r.SystemAsOf.Add(-time.Second) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			candidate := request
			candidate.Filters = &EvidenceQueryFilters{}
			candidate.Filters.NewsIDs = append([]string(nil), request.Filters.NewsIDs...)
			testCase.mutate(&candidate)
			if snapshot.MatchesRequest(candidate) {
				t.Fatal("mismatched query context unexpectedly matched snapshot")
			}
		})
	}
}

func TestEvidenceSnapshotValidateRejectsInvalidTTLFingerprintAndPITKey(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name   string
		mutate func(*EvidenceSnapshot)
	}{
		{"wrong ttl", func(s *EvidenceSnapshot) { s.ExpiresAt = s.ExpiresAt.Add(time.Second) }},
		{"system as of after snapshot creation", func(s *EvidenceSnapshot) { s.SystemAsOf = s.CreatedAt.Add(time.Second) }},
		{"missing purpose", func(s *EvidenceSnapshot) { s.PurposeCode = "" }},
		{"invalid purpose", func(s *EvidenceSnapshot) { s.PurposeCode = "unrestricted" }},
		{"invalid content level", func(s *EvidenceSnapshot) { s.RequestedContentLevel = "unrestricted" }},
		{"uppercase fingerprint", func(s *EvidenceSnapshot) { s.FiltersFingerprint = strings.Repeat("A", 64) }},
		{"invalid fingerprint", func(s *EvidenceSnapshot) { s.FiltersFingerprint = "not-a-hash" }},
		{"future observation", func(s *EvidenceSnapshot) { s.Items[0].SystemObservedAt = s.SystemAsOf.Add(time.Second) }},
		{"duplicate evidence key", func(s *EvidenceSnapshot) { s.Items[1] = s.Items[0] }},
		{"unstable sort key", func(s *EvidenceSnapshot) {
			s.Items[1].SystemObservedAt = s.Items[0].SystemObservedAt
			s.Items[1].NewsID = s.Items[0].NewsID
			s.Items[1].VersionID = s.Items[0].VersionID
		}},
		{"nil items", func(s *EvidenceSnapshot) { s.Items = nil }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			snapshot := validEvidenceSnapshot(now)
			testCase.mutate(&snapshot)
			if !errors.Is(snapshot.Validate(), ErrInvalidEvidenceSnapshot) {
				t.Fatalf("Validate() error = %v, want ErrInvalidEvidenceSnapshot", snapshot.Validate())
			}
		})
	}
}

func TestEvidenceSnapshotPageAfterUsesExactStableSortPosition(t *testing.T) {
	now := time.Now().UTC()
	snapshot := validEvidenceSnapshot(now)
	snapshot.PageSize = 2
	firstPage, hasMore, err := snapshot.PageAfter(nil)
	if err != nil || !hasMore || len(firstPage) != 2 {
		t.Fatalf("first PageAfter() = (%d, %t, %v)", len(firstPage), hasMore, err)
	}
	last := firstPage[len(firstPage)-1]
	position := &EvidenceCursorPosition{
		SnapshotRef:          snapshot.SnapshotRef,
		LastSystemObservedAt: last.SystemObservedAt,
		LastNewsID:           last.NewsID,
		LastVersionID:        last.VersionID,
	}
	secondPage, hasMore, err := snapshot.PageAfter(position)
	if err != nil || hasMore || len(secondPage) != 1 || secondPage[0].NewsID != "news-3" {
		t.Fatalf("second PageAfter() = (%+v, %t, %v)", secondPage, hasMore, err)
	}
	position.SnapshotRef = "another-snapshot"
	if _, _, err := snapshot.PageAfter(position); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Fatalf("foreign snapshot position error = %v", err)
	}
	position.SnapshotRef = snapshot.SnapshotRef
	position.LastNewsID = "missing-news"
	if _, _, err := snapshot.PageAfter(position); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Fatalf("unknown sort position error = %v", err)
	}
}

func TestSaveEvidenceSnapshotValidatesBeforePersistence(t *testing.T) {
	repository := &evidenceSnapshotRepositoryStub{}
	service, err := NewSaveEvidenceSnapshot(repository)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := validEvidenceSnapshot(time.Now().UTC())
	if err := service.Execute(context.Background(), snapshot); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if repository.calls != 1 || repository.created.SnapshotRef != snapshot.SnapshotRef {
		t.Fatalf("repository calls=%d snapshot=%+v", repository.calls, repository.created)
	}
	snapshot.Items[0].NewsID = ""
	if err := service.Execute(context.Background(), snapshot); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Fatalf("invalid snapshot error = %v", err)
	}
	if repository.calls != 1 {
		t.Fatalf("invalid snapshot reached repository; calls=%d", repository.calls)
	}
}

func TestEvidenceSnapshotRetentionCleanerUsesBoundedRepositoryRequest(t *testing.T) {
	repository := &evidenceSnapshotRepositoryStub{deleted: 7}
	cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	deleted, err := cleaner.Execute(context.Background(), now, 20)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 7 || repository.deleteCalls != 1 || !repository.deleteNow.Equal(now.UTC()) || repository.deleteLimit != 20 {
		t.Fatalf("deleted=%d calls=%d now=%s limit=%d", deleted, repository.deleteCalls, repository.deleteNow, repository.deleteLimit)
	}
}

func TestEvidenceSnapshotRetentionCleanerRejectsInvalidRequestsAndResults(t *testing.T) {
	if _, err := NewEvidenceSnapshotRetentionCleaner(nil); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Fatalf("nil repository error=%v", err)
	}
	repository := &evidenceSnapshotRepositoryStub{}
	cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cleaner.Execute(context.Background(), time.Time{}, 1); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Fatalf("zero time error=%v", err)
	}
	if _, err := cleaner.Execute(context.Background(), time.Now(), 1001); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Fatalf("oversized limit error=%v", err)
	}
	repository.deleted = -1
	if _, err := cleaner.Execute(context.Background(), time.Now(), 10); !errors.Is(err, ErrEvidenceSnapshotRetentionResultInvalid) {
		t.Fatalf("negative deleted count error=%v", err)
	}
	repository.deleted = 11
	if _, err := cleaner.Execute(context.Background(), time.Now(), 10); !errors.Is(err, ErrEvidenceSnapshotRetentionResultInvalid) {
		t.Fatalf("over-limit deleted count error=%v", err)
	}
}

func TestEvidenceSnapshotRetentionCleanerHonorsCancellation(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	t.Run("already canceled does not call repository", func(t *testing.T) {
		repository := &evidenceSnapshotRepositoryStub{}
		cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
		if err != nil {
		t.Fatal(err)
	}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := cleaner.Execute(ctx, now, 10); !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error=%v", err)
		}
		if repository.deleteCalls != 0 {
			t.Fatalf("repository calls=%d, want 0", repository.deleteCalls)
		}
	})
	t.Run("canceled while repository is running", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		repository := &evidenceSnapshotRepositoryStub{deleted: 2, afterDelete: cancel}
		cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
		if err != nil {
		t.Fatal(err)
	}
		if _, err := cleaner.Execute(ctx, now, 10); !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error=%v", err)
		}
		if repository.deleteCalls != 1 {
			t.Fatalf("repository calls=%d, want 1", repository.deleteCalls)
		}
	})
}
