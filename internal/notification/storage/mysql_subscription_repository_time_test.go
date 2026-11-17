package storage

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestNormalizeSubscriptionDatabaseTimesUsesMariaDBPrecision(t *testing.T) {
	created := time.Date(2026, 11, 9, 9, 30, 0, 123456789, time.FixedZone("UTC+8", 8*60*60))
	value := domain.Subscription{CreatedAt: created, UpdatedAt: created.Add(time.Second), EffectiveAt: created.Add(2 * time.Second)}
	normalized := normalizeSubscriptionDatabaseTimes(value)
	for name, timestamp := range map[string]time.Time{
		"created_at":   normalized.CreatedAt,
		"updated_at":   normalized.UpdatedAt,
		"effective_at": normalized.EffectiveAt,
	} {
		if timestamp.Location() != time.UTC || timestamp.Nanosecond()%1000 != 0 {
			t.Errorf("%s must be UTC with microsecond precision, got %s", name, timestamp)
		}
	}
	if value.CreatedAt.Nanosecond() != 123456789 {
		t.Fatal("normalization must not mutate caller's subscription")
	}
	if !normalized.CreatedAt.Equal(created.UTC().Truncate(time.Microsecond)) {
		t.Fatalf("unexpected created_at: %s", normalized.CreatedAt)
	}
}
