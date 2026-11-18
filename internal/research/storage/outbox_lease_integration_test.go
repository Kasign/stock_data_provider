package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	researchevent "example.com/stock-analysis-flow/internal/research/event"
)

func TestResearchOutboxExpiredLeaseCanBeReclaimedAndFenced(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)
	eventID := fmt.Sprintf("event-outbox-expired-lease-%d", time.Now().UnixNano())
	eventRepo, err := NewEventRepository(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	outbox, err := NewOutboxRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_notification_outbox WHERE subject_ref=?", "research-event:"+eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID)
	})

	value := researchevent.ResearchEvent{
		EventID: eventID, EventVersion: 1, EventType: "integration_fixture",
		Title: "Expired Outbox lease fixture", Status: researchevent.StatusDetected,
		SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, TimeNormalizationRuleVersion: "1.0.0",
		DerivedFromRefs: []researchevent.ResearchEventRef{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := eventRepo.AppendWithLifecycle(ctx, value, nil, "corr_expired_lease"); err != nil {
		t.Fatalf("append Event and lifecycle Outbox: %v", err)
	}

	first, err := outbox.ClaimOutbox(ctx, "worker_lease_old", 1, 250*time.Millisecond)
	if err != nil {
		t.Fatalf("claim initial lease: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("initial claim returned %d messages, want 1", len(first))
	}

	// Wait beyond the DB-owned lease deadline. Reclaim eligibility is decided
	// by MariaDB UTC time, not by the Event's application clock.
	time.Sleep(400 * time.Millisecond)
	second, err := outbox.ClaimOutbox(ctx, "worker_lease_new", 1, time.Minute)
	if err != nil {
		t.Fatalf("reclaim expired lease: %v", err)
	}
	if len(second) != 1 || second[0].EventID != first[0].EventID {
		t.Fatalf("expired message was not reclaimed: first=%+v second=%+v", first, second)
	}
	if second[0].LeaseToken == first[0].LeaseToken {
		t.Fatal("reclaimed lease must receive a new fencing token")
	}

	if err := outbox.MarkOutboxPublished(ctx, first[0].EventID, "worker_lease_old", first[0].LeaseToken); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired lease confirmation = %v, want ErrConflict", err)
	}
	if err := outbox.ScheduleOutboxRetry(ctx, first[0].EventID, "worker_lease_old", first[0].LeaseToken, time.Second, "stale_retry"); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired lease retry = %v, want ErrConflict", err)
	}
	if err := outbox.RejectOutbox(ctx, first[0].EventID, "worker_lease_old", first[0].LeaseToken, "stale_reject"); !errors.Is(err, ErrConflict) {
		t.Fatalf("expired lease rejection = %v, want ErrConflict", err)
	}

	if err := outbox.RejectOutbox(ctx, second[0].EventID, "worker_lease_new", second[0].LeaseToken, "delivery_permanently_rejected"); err != nil {
		t.Fatalf("reject with current lease: %v", err)
	}
	if err := outbox.MarkOutboxPublished(ctx, second[0].EventID, "worker_lease_new", second[0].LeaseToken); !errors.Is(err, ErrConflict) {
		t.Fatalf("confirmation after rejection = %v, want ErrConflict", err)
	}
	again, err := outbox.ClaimOutbox(ctx, "worker_lease_third", 1, time.Minute)
	if err != nil || len(again) != 0 {
		t.Fatalf("rejected message was reclaimed: %+v, error %v", again, err)
	}

	var status, reason string
	var owner, token, leaseUntil sql.NullString
	var attempts int
	if err := db.QueryRowContext(ctx,
		"SELECT delivery_status, last_error_code, lease_owner, lease_token, lease_until, attempt_count FROM research_notification_outbox WHERE subject_ref=?",
		"research-event:"+eventID).Scan(&status, &reason, &owner, &token, &leaseUntil, &attempts); err != nil {
		t.Fatalf("read final Outbox state: %v", err)
	}
	if status != "rejected" || reason != "delivery_permanently_rejected" ||
		owner.Valid || token.Valid || leaseUntil.Valid || attempts != 2 {
		t.Fatalf("unexpected final Outbox state: status=%s reason=%s owner=%v token=%v lease_until=%v attempts=%d",
			status, reason, owner, token, leaseUntil, attempts)
	}

}
