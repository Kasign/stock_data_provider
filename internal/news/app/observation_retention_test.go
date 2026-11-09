package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

type observationRetentionRepositoryStub struct {
	result     ObservationPayloadPurgeResult
	err        error
	now        time.Time
	limit      int
	calls      int
	afterPurge func()
}

func (s *observationRetentionRepositoryStub) PurgeExpiredObservationPayloads(_ context.Context, now time.Time, limit int) (ObservationPayloadPurgeResult, error) {
	s.now, s.limit = now, limit
	s.calls++
	if s.afterPurge != nil {
		s.afterPurge()
	}
	return s.result, s.err
}

func TestObservationRetentionCleanerPassesBoundedRequest(t *testing.T) {
	repository := &observationRetentionRepositoryStub{result: ObservationPayloadPurgeResult{Scanned: 5, Purged: 5}}
	cleaner, err := NewObservationRetentionCleaner(repository)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	result, err := cleaner.Execute(context.Background(), now, 20)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 5 || result.Purged != 5 || !repository.now.Equal(now.UTC()) || repository.limit != 20 {
		t.Fatalf("result=%+v now=%s limit=%d", result, repository.now, repository.limit)
	}
}

func TestObservationRetentionCleanerRejectsInvalidRequestsAndResults(t *testing.T) {
	if _, err := NewObservationRetentionCleaner(nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("nil repository error=%v", err)
	}
	repository := &observationRetentionRepositoryStub{}
	cleaner, err := NewObservationRetentionCleaner(repository)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cleaner.Execute(context.Background(), time.Time{}, 1); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("zero time error=%v", err)
	}
	if _, err := cleaner.Execute(context.Background(), time.Now(), 1001); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("oversized batch error=%v", err)
	}
	repository.result = ObservationPayloadPurgeResult{Scanned: 1, Purged: 2}
	if _, err := cleaner.Execute(context.Background(), time.Now(), 10); !errors.Is(err, ErrObservationRetentionResultInvalid) {
		t.Fatalf("invalid repository result error=%v", err)
	}
}

func TestObservationRetentionCleanerHonorsContextCancellation(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)

	t.Run("already canceled does not call repository", func(t *testing.T) {
		repository := &observationRetentionRepositoryStub{}
		cleaner, err := NewObservationRetentionCleaner(repository)
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := cleaner.Execute(ctx, now, 10); !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error=%v, want context.Canceled", err)
		}
		if repository.calls != 0 {
			t.Fatalf("repository calls=%d, want 0", repository.calls)
		}
	})

	t.Run("canceled while repository is running returns cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		repository := &observationRetentionRepositoryStub{
			result: ObservationPayloadPurgeResult{Scanned: 2, Purged: 2},
			afterPurge: cancel,
		}
		cleaner, err := NewObservationRetentionCleaner(repository)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := cleaner.Execute(ctx, now, 10); !errors.Is(err, context.Canceled) {
			t.Fatalf("Execute() error=%v, want context.Canceled", err)
		}
		if repository.calls != 1 {
			t.Fatalf("repository calls=%d, want 1", repository.calls)
		}
	})
}
