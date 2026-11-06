package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type evidenceSnapshotRepositoryStub struct {
	created EvidenceSnapshot
	calls   int
}

func (s *evidenceSnapshotRepositoryStub) CreateEvidenceSnapshot(_ context.Context, snapshot EvidenceSnapshot) error {
	s.calls++
	s.created = snapshot
	return nil
}

func (s *evidenceSnapshotRepositoryStub) GetEvidenceSnapshot(context.Context, string, string, time.Time) (EvidenceSnapshot, error) {
	return EvidenceSnapshot{}, ErrEvidenceSnapshotUnavailable
}

func (s *evidenceSnapshotRepositoryStub) DeleteExpiredEvidenceSnapshots(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func validEvidenceSnapshot(now time.Time) EvidenceSnapshot {
	asOf := now.Add(-time.Minute)
	return EvidenceSnapshot{
		SnapshotRef: "snapshot-1",
		CallerRef: "research",
		FiltersFingerprint: strings.Repeat("a", 64),
		PageSize: 20,
		SystemAsOf: asOf,
		SortVersion: evidenceSortVersion,
		CreatedAt: now,
		ExpiresAt: now.Add(EvidenceSnapshotTTL),
		Items: []EvidenceSnapshotItem{
			{NewsID: "news-2", VersionID: "version-1", ObservationID: "observation-2", SystemObservedAt: asOf.Add(-time.Minute)},
			{NewsID: "news-1", VersionID: "version-2", ObservationID: "observation-1", SystemObservedAt: asOf.Add(-2*time.Minute)},
			{NewsID: "news-3", VersionID: "version-3", ObservationID: "observation-3", SystemObservedAt: asOf.Add(-3*time.Minute)},
		},
	}
}

func TestEvidenceSnapshotValidateAcceptsStableReferenceOnlySnapshot(t *testing.T) {
	snapshot := validEvidenceSnapshot(time.Date(2026, 11, 6, 10, 0, 0, 0, time.UTC))
	if err := snapshot.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEvidenceSnapshotValidateRejectsInvalidTTLFingerprintAndPITKey(t *testing.T) {
	now := time.Now().UTC()
	cases := []struct {
		name string
		mutate func(*EvidenceSnapshot)
	}{
		{"wrong ttl", func(s *EvidenceSnapshot) { s.ExpiresAt = s.ExpiresAt.Add(time.Second) }},
		{"system as of after snapshot creation", func(s *EvidenceSnapshot) { s.SystemAsOf = s.CreatedAt.Add(time.Second) }},
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
		SnapshotRef: snapshot.SnapshotRef,
		LastSystemObservedAt: last.SystemObservedAt,
		LastNewsID: last.NewsID,
		LastVersionID: last.VersionID,
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
