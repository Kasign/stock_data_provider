package storage

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestNormalizeDeliveryRecordTimesUsesMariaDBPrecision(t *testing.T) {
	now := time.Date(2026, 11, 9, 9, 30, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))
	nextAttempt := now.Add(time.Second + 987*time.Nanosecond)
	record := domain.DeliveryRecord{
		DeadlineAt: now.Add(time.Hour),
		CreatedAt: now,
		UpdatedAt: now,
		NextAttemptAt: &nextAttempt,
	}
	normalized := normalizeDeliveryRecordTimes(record)
	for name, value := range map[string]time.Time{
		"deadline_at": normalized.DeadlineAt,
		"created_at": normalized.CreatedAt,
		"updated_at": normalized.UpdatedAt,
		"next_attempt_at": *normalized.NextAttemptAt,
	} {
		if value.Location() != time.UTC || value.Nanosecond()%1000 != 0 {
			t.Errorf("%s must be UTC with microsecond precision, got %s", name, value)
		}
	}
	if record.CreatedAt.Nanosecond() != 123456789 {
		t.Fatal("normalization must not mutate the caller's record")
	}
	if !normalized.CreatedAt.Equal(now.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("unexpected normalized created_at: %s", normalized.CreatedAt)
	}
}
