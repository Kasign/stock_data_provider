package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type evidenceSnapshotFailureStore struct {
	snapshot  EvidenceSnapshot
	createErr error
	getErr    error
	creates   int
	gets      int
}

func (s *evidenceSnapshotFailureStore) CreateEvidenceSnapshot(_ context.Context, snapshot EvidenceSnapshot) error {
	s.creates++
	if s.createErr != nil {
		return s.createErr
	}
	s.snapshot = snapshot
	return nil
}

func (s *evidenceSnapshotFailureStore) GetEvidenceSnapshot(_ context.Context, _, _ string, _ time.Time) (EvidenceSnapshot, error) {
	s.gets++
	if s.getErr != nil {
		return EvidenceSnapshot{}, s.getErr
	}
	return s.snapshot, nil
}

func (s *evidenceSnapshotFailureStore) DeleteExpiredEvidenceSnapshots(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestListEvidenceSnapshotPagePropagatesRepositoryFailures(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	request := validEvidenceListRequest(now)
	request.Filters = &EvidenceQueryFilters{}
	originalErr := errors.New("repository unavailable")

	t.Run("PIT query failure", func(t *testing.T) {
		query := &evidencePITRepositoryStub{err: originalErr}
		store := &evidenceSnapshotFailureStore{}
		factory, err := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-1" })
		if err != nil {
			t.Fatal(err)
		}
		factory.now = func() time.Time { return now }
		codec := newTestEvidenceCursorCodec(t, "active", map[string]string{"active": strings.Repeat("k", 32)})
		service, err := NewListEvidenceSnapshotPage(factory, codec)
		if err != nil {
			t.Fatal(err)
		}
		service.now = func() time.Time { return now }

		if _, err := service.Execute(context.Background(), request); !errors.Is(err, originalErr) {
			t.Fatalf("Execute() error = %v, want original repository error", err)
		}
		if query.calls != 1 || store.creates != 0 {
			t.Fatalf("unexpected calls: PIT=%d create=%d", query.calls, store.creates)
		}
	})

	t.Run("snapshot create failure", func(t *testing.T) {
		query := &evidencePITRepositoryStub{records: evidencePITRecords(request.SystemAsOf)}
		store := &evidenceSnapshotFailureStore{createErr: originalErr}
		factory, err := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-1" })
		if err != nil {
			t.Fatal(err)
		}
		factory.now = func() time.Time { return now }
		codec := newTestEvidenceCursorCodec(t, "active", map[string]string{"active": strings.Repeat("k", 32)})
		service, err := NewListEvidenceSnapshotPage(factory, codec)
		if err != nil {
			t.Fatal(err)
		}
		service.now = func() time.Time { return now }

		if _, err := service.Execute(context.Background(), request); !errors.Is(err, originalErr) {
			t.Fatalf("Execute() error = %v, want original repository error", err)
		}
		if query.calls != 1 || store.creates != 1 {
			t.Fatalf("unexpected calls: PIT=%d create=%d", query.calls, store.creates)
		}
	})

	t.Run("snapshot read failure", func(t *testing.T) {
		store := &evidenceSnapshotFailureStore{getErr: originalErr}
		factory, err := NewCreateEvidenceSnapshotForQuery(&evidencePITRepositoryStub{}, store, func() string { return "unused" })
		if err != nil {
			t.Fatal(err)
		}
		codec := newTestEvidenceCursorCodec(t, "active", map[string]string{"active": strings.Repeat("k", 32)})
		service, err := NewListEvidenceSnapshotPage(factory, codec)
		if err != nil {
			t.Fatal(err)
		}
		service.now = func() time.Time { return now }

		last := ResearchEvidenceRecord{
			NewsID: "news-1", VersionID: "version-1",
			SystemObservedAt: request.SystemAsOf.Add(-time.Minute),
			SystemAsOf:       request.SystemAsOf,
		}
		token, err := codec.EncodeNextCursor(request, "snapshot-1", last, now)
		if err != nil {
			t.Fatal(err)
		}
		continuation := request
		continuation.Cursor = &token
		if _, err := service.Execute(context.Background(), continuation); !errors.Is(err, originalErr) {
			t.Fatalf("Execute() error = %v, want original repository error", err)
		}
		if store.gets != 1 {
			t.Fatalf("snapshot reads = %d, want 1", store.gets)
		}
	})
}

func TestListEvidenceSnapshotPageRejectsAllCursorQueryContextMismatchesBeforeStorage(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	request := validEvidenceListRequest(now)
	request.Filters = &EvidenceQueryFilters{}
	codec := newTestEvidenceCursorCodec(t, "active", map[string]string{"active": strings.Repeat("k", 32)})
	last := ResearchEvidenceRecord{
		NewsID: "news-1", VersionID: "version-1",
		SystemObservedAt: request.SystemAsOf.Add(-time.Minute),
		SystemAsOf:       request.SystemAsOf,
	}
	token, err := codec.EncodeNextCursor(request, "snapshot-1", last, now)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		mutate func(*ListEvidenceRequest)
	}{
		{"caller", func(r *ListEvidenceRequest) { r.CallerRef = "other-caller" }},
		{"purpose", func(r *ListEvidenceRequest) { r.PurposeCode = EvidencePurposeRiskReview }},
		{"content level", func(r *ListEvidenceRequest) { r.RequestedContentLevel = EvidenceContentMetadata }},
		{"filters", func(r *ListEvidenceRequest) { r.Filters.NewsIDs = []string{"news-other"} }},
		{"page size", func(r *ListEvidenceRequest) { r.PageSize++ }},
		{"system as of", func(r *ListEvidenceRequest) { r.SystemAsOf = r.SystemAsOf.Add(-time.Second) }},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			query := &evidencePITRepositoryStub{}
			store := &evidenceSnapshotFailureStore{}
			factory, err := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "unused" })
			if err != nil {
				t.Fatal(err)
			}
			service, err := NewListEvidenceSnapshotPage(factory, codec)
			if err != nil {
				t.Fatal(err)
			}
			service.now = func() time.Time { return now }

			changed := request
			changed.Filters = &EvidenceQueryFilters{}
			changed.Cursor = &token
			testCase.mutate(&changed)
			if _, err := service.Execute(context.Background(), changed); !errors.Is(err, ErrEvidenceCursorInvalid) {
				t.Fatalf("Execute() error = %v, want invalid cursor", err)
			}
			if query.calls != 0 || store.gets != 0 || store.creates != 0 {
				t.Fatalf("mismatched cursor reached storage: PIT=%d get=%d create=%d", query.calls, store.gets, store.creates)
			}
		})
	}
}
