package bootstrap

import (
	"context"
	"testing"
	"time"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
)

type newsRetentionTestRepository struct {
	observationCalls int
	snapshotCalls    int
	observationNow   time.Time
	snapshotNow      time.Time
	observationLimit int
	snapshotLimit    int
}

func (r *newsRetentionTestRepository) PurgeExpiredObservationPayloads(
	_ context.Context, now time.Time, limit int,
) (newsapp.ObservationPayloadPurgeResult, error) {
	r.observationCalls++
	r.observationNow = now
	r.observationLimit = limit
	return newsapp.ObservationPayloadPurgeResult{Scanned: 2, Purged: 2, EvidenceScanned: 1, EvidencePurged: 1}, nil
}

func (r *newsRetentionTestRepository) DeleteExpiredEvidenceSnapshots(
	_ context.Context, now time.Time, limit int,
) (int64, error) {
	r.snapshotCalls++
	r.snapshotNow = now
	r.snapshotLimit = limit
	return 3, nil
}

type newsRetentionTestClock struct{ now time.Time }

func (c newsRetentionTestClock) Now() time.Time { return c.now }

func TestNewsRetentionComponentRunsBothModuleCleanersWithBoundedBatch(t *testing.T) {
	repository := &newsRetentionTestRepository{}
	component, err := newNewsRetentionComponent(
		repository,
		serverconfig.NewsRetention{Interval: time.Minute, BatchSize: 25},
		nil,
	)
	if err != nil {
		t.Fatalf("newNewsRetentionComponent() error = %v", err)
	}
	now := time.Date(2026, 11, 18, 12, 30, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))
	component.clock = newsRetentionTestClock{now: now}

	component.runOnce(context.Background())

	wantNow := now.UTC()
	if repository.observationCalls != 1 || repository.snapshotCalls != 1 {
		t.Fatalf("cleaner calls = observation:%d snapshot:%d, want 1 each", repository.observationCalls, repository.snapshotCalls)
	}
	if !repository.observationNow.Equal(wantNow) || !repository.snapshotNow.Equal(wantNow) {
		t.Fatalf("cleaner times = observation:%s snapshot:%s, want UTC %s", repository.observationNow, repository.snapshotNow, wantNow)
	}
	if repository.observationLimit != 25 || repository.snapshotLimit != 25 {
		t.Fatalf("cleaner limits = observation:%d snapshot:%d, want 25 each", repository.observationLimit, repository.snapshotLimit)
	}
}
