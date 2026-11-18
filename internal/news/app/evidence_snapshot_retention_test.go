package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

type evidenceSnapshotRetentionRepositoryStub struct {
	deleted int64
	err     error
	now     time.Time
	limit   int
	calls   int
	afterDelete func()
}

func (s *evidenceSnapshotRetentionRepositoryStub) DeleteExpiredEvidenceSnapshots(
	_ context.Context,
	now time.Time,
	limit int,
) (int64, error) {
	s.calls++
	s.now = now
	s.limit = limit
	if s.afterDelete != nil {
		s.afterDelete()
	}
	return s.deleted, s.err
}

func TestEvidenceSnapshotRetentionCleanerPassesBoundedUTCRequest(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 30, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	repository := &evidenceSnapshotRetentionRepositoryStub{deleted: 7}
	cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := cleaner.Execute(context.Background(), now, 25)
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 7 || repository.calls != 1 ||
		!repository.now.Equal(now.UTC()) || repository.limit != 25 {
		t.Fatalf("deleted=%d calls=%d now=%s limit=%d", deleted, repository.calls, repository.now, repository.limit)
	}
}

func TestEvidenceSnapshotRetentionCleanerRejectsInvalidRequestsAndResults(t *testing.T) {
	if _, err := NewEvidenceSnapshotRetentionCleaner(nil); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Fatalf("nil repository error=%v", err)
	}
	repository := &evidenceSnapshotRetentionRepositoryStub{}
	cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	for _, limit := range []int{0, 1001} {
		if _, err := cleaner.Execute(context.Background(), now, limit); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
			t.Errorf("limit=%d error=%v, want ErrInvalidEvidenceSnapshot", limit, err)
		}
	}
	if _, err := cleaner.Execute(context.Background(), time.Time{}, 1); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
		t.Errorf("zero time error=%v, want ErrInvalidEvidenceSnapshot", err)
	}
	if repository.calls != 0 {
		t.Fatalf("repository calls=%d, want 0 for invalid requests", repository.calls)
	}

	for _, deleted := range []int64{-1, 11} {
		repository.deleted = deleted
		if _, err := cleaner.Execute(context.Background(), now, 10); !errors.Is(err, ErrEvidenceSnapshotRetentionResultInvalid) {
			t.Errorf("deleted=%d error=%v, want invalid result", deleted, err)
		}
	}
}

func TestEvidenceSnapshotRetentionCleanerPropagatesRepositoryError(t *testing.T) {
	repositoryErr := errors.New("storage transaction failed")
	repository := &evidenceSnapshotRetentionRepositoryStub{err: repositoryErr}
	cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cleaner.Execute(context.Background(), time.Now().UTC(), 10); !errors.Is(err, repositoryErr) {
		t.Fatalf("Execute() error=%v, want repository error", err)
	}
	if repository.calls != 1 {
		t.Fatalf("repository calls=%d, want 1", repository.calls)
	}
}

func TestEvidenceSnapshotRetentionCleanerHonorsContextCancellation(t *testing.T) {
	now := time.Now().UTC()
	t.Run("already canceled does not call repository", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		repository := &evidenceSnapshotRetentionRepositoryStub{}
		cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cleaner.Execute(ctx, now, 10); !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error=%v, want context.Canceled", err)
		}
		if repository.calls != 0 {
			t.Fatalf("repository calls=%d, want 0", repository.calls)
		}
	})

	t.Run("repository success wins over later cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		repository := &evidenceSnapshotRetentionRepositoryStub{
			deleted: 3,
			afterDelete: cancel,
		}
		cleaner, err := NewEvidenceSnapshotRetentionCleaner(repository)
		if err != nil {
			t.Fatal(err)
		}
		deleted, err := cleaner.Execute(ctx, now, 10)
		if err != nil || deleted != 3 {
			t.Fatalf("Execute() = (%d, %v), want confirmed delete result", deleted, err)
		}
		if repository.calls != 1 {
			t.Fatalf("repository calls=%d, want 1", repository.calls)
		}
	})
}
