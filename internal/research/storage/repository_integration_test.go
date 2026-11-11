package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	mysql "github.com/go-sql-driver/mysql"

	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	researchapp "example.com/stock-analysis-flow/internal/research/application"
	researchclaim "example.com/stock-analysis-flow/internal/research/claim"
	researchcontext "example.com/stock-analysis-flow/internal/research/context"
	researchevent "example.com/stock-analysis-flow/internal/research/event"
	researchevidence "example.com/stock-analysis-flow/internal/research/evidence"
)

func openResearchIntegrationDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("RESEARCH_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RESEARCH_TEST_MYSQL_DSN is not set; skipping MariaDB integration test")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse RESEARCH_TEST_MYSQL_DSN: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(config.DBName), "_test") {
		t.Fatalf("RESEARCH_TEST_MYSQL_DSN database %q is unsafe; dedicated database name must end with _test", config.DBName)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open MariaDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping MariaDB: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, Migrations()); err != nil {
		t.Fatalf("apply Research migrations: %v", err)
	}
	for _, column := range []struct{ table, name string }{
		{"research_event_versions", "event_version"},
		{"research_claim_versions", "claim_version"},
	} {
		var dataType string
		if err := db.QueryRowContext(ctx, "SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?", column.table, column.name).Scan(&dataType); err != nil {
			t.Fatalf("read %s.%s column type: %v", column.table, column.name, err)
		}
		if dataType != "bigint" {
			t.Fatalf("%s.%s data type = %q, want bigint", column.table, column.name, dataType)
		}
	}
	return db
}

func assertResearchPITColumn(t *testing.T, ctx context.Context, db *sql.DB, query string, args []any, want time.Time) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(ctx, query, args...).Scan(&got); err != nil {
		t.Fatalf("read persisted PIT timestamp: %v", err)
	}
	if expected := formatPIT(want); got != expected {
		t.Fatalf("persisted PIT timestamp = %q, want %q", got, expected)
	}
}

func TestResearchEventLifecycleOutboxFailureRollsBackEventVersion(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Date(2026, 11, 9, 2, 0, 0, 123456789, time.UTC)
	eventID := fmt.Sprintf("event-outbox-rollback-%d", time.Now().UnixNano())
	repo, err := NewEventRepository(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_notification_outbox WHERE subject_ref=?", "research-event:"+eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID)
	})
	previous := researchevent.ResearchEvent{
		EventID: eventID, EventVersion: 1, EventType: "integration_fixture",
		Title: "Outbox rollback fixture", Status: researchevent.StatusDetected,
		SubjectRefs: []string{"security:600000"}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := repo.Append(ctx, previous); err != nil {
		t.Fatalf("append Event v1: %v", err)
	}
	next := previous
	next.EventVersion = 2
	next.Status = researchevent.StatusConsolidating
	next.SystemAsOf = now.Add(time.Minute)
	idempotencyKey := "research-event-state:research-event:" + eventID + ":2"
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_notification_outbox (event_id, event_type, schema_version, source_module, occurred_at, published_at, subject_ref, idempotency_key, correlation_id, payload_fingerprint, payload, created_at, available_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"nte_seed_"+eventID, researchLifecycleEventType, researchLifecycleSchemaVersion, "research",
		formatPIT(now), formatPIT(now), "research-event:"+eventID, idempotencyKey, "corr_seed",
		strings.Repeat("a", 64), "{}", now, formatPIT(now), now); err != nil {
		t.Fatalf("seed Outbox idempotency conflict: %v", err)
	}
	err = repo.AppendWithLifecycle(ctx, next, nil, "req_test")
	if err == nil {
		t.Fatal("duplicate Outbox idempotency key must abort the Event append transaction")
	}
	if _, err := repo.GetVersion(ctx, eventID, 2); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Event v2 after Outbox rollback = %v, want ErrNotFound", err)
	}
	var latest int64
	if err := db.QueryRowContext(ctx, "SELECT latest_version FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID).Scan(&latest); err != nil {
		t.Fatalf("read aggregate head after rollback: %v", err)
	}
	if latest != 1 {
		t.Fatalf("aggregate latest_version = %d after Outbox rollback, want 1", latest)
	}
}

func TestResearchOutboxClaimRetryAndConfirmation(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	now := time.Date(2026, 11, 9, 3, 0, 0, 123456789, time.UTC)
	eventID := fmt.Sprintf("event-outbox-delivery-%d", time.Now().UnixNano())
	repo, err := NewEventRepository(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_notification_outbox WHERE subject_ref=?", "research-event:"+eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID)
	})
	value := researchevent.ResearchEvent{
		EventID: eventID, EventVersion: 1, EventType: "integration_fixture", Title: "Outbox delivery fixture",
		Status: researchevent.StatusDetected, SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now, ImpactHypothesisRefs: []string{},
		InvalidationConditions: []string{}, Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := repo.AppendWithLifecycle(ctx, value, nil, "corr_integration"); err != nil {
		t.Fatalf("append Event and initial Outbox: %v", err)
	}
	claimed, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim Outbox: %v", err)
	}
	if len(claimed) != 1 || claimed[0].IdempotencyKey != "research-event-state:research-event:"+eventID+":1" {
		t.Fatalf("unexpected claimed Outbox messages: %+v", claimed)
	}
	if err := repo.base.ScheduleOutboxRetry(ctx, claimed[0].EventID, "worker_test", claimed[0].LeaseToken, now.Add(2*time.Minute), "notification_unavailable", now); err != nil {
		t.Fatalf("schedule retry: %v", err)
	}
	tooEarly, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(3*time.Minute))
	if err != nil {
		t.Fatalf("claim before retry time: %v", err)
	}
	if len(tooEarly) != 0 {
		t.Fatalf("retry became available too early: %+v", tooEarly)
	}
	now = now.Add(2 * time.Minute)
	reclaimed, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("reclaim retry: %v", err)
	}
	if len(reclaimed) != 1 || reclaimed[0].EventID != claimed[0].EventID || reclaimed[0].LeaseToken == claimed[0].LeaseToken {
		t.Fatalf("retry must reuse the persisted Envelope identity: %+v", reclaimed)
	}
	if err := repo.base.MarkOutboxPublished(ctx, claimed[0].EventID, "worker_test", claimed[0].LeaseToken, now); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale lease token confirmation = %v, want ErrConflict", err)
	}
	if err := repo.base.MarkOutboxPublished(ctx, reclaimed[0].EventID, "worker_test", reclaimed[0].LeaseToken, now); err != nil {
		t.Fatalf("confirm Outbox delivery: %v", err)
	}
	if err := repo.base.MarkOutboxPublished(ctx, reclaimed[0].EventID, "worker_test", reclaimed[0].LeaseToken, now); err != nil {
		t.Fatalf("repeated confirmation must be idempotent: %v", err)
	}
	after, err := repo.base.ClaimOutbox(ctx, "worker_test", 10, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("claim after confirmation: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("delivered Outbox message was claimed again: %+v", after)
	}
	var attempts int
	var status string
	if err := db.QueryRowContext(ctx, "SELECT attempt_count, delivery_status FROM research_notification_outbox WHERE event_id=?", claimed[0].EventID).Scan(&attempts, &status); err != nil {
		t.Fatalf("read Outbox delivery state: %v", err)
	}
	if attempts != 2 || status != "published" {
		t.Fatalf("Outbox attempts/status = %d/%s, want 2/published", attempts, status)
	}
}

func TestResearchOutboxConcurrentClaimsDoNotDuplicateLease(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Date(2026, 11, 9, 7, 0, 0, 123456789, time.UTC)
	eventID := fmt.Sprintf("event-outbox-concurrent-%d", time.Now().UnixNano())
	repo, err := NewEventRepository(db, func() time.Time { return now })
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
		Title: "Concurrent Outbox claim fixture", Status: researchevent.StatusDetected,
		SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := repo.AppendWithLifecycle(ctx, value, nil, "corr_concurrent_claim"); err != nil {
		t.Fatalf("append Event and Outbox: %v", err)
	}

	type claimResult struct {
		worker string
		values []researchapp.NotificationEventEnvelope
		err    error
	}
	start := make(chan struct{})
	results := make(chan claimResult, 2)
	var workers sync.WaitGroup
	for _, worker := range []string{"worker_concurrent_a", "worker_concurrent_b"} {
		worker := worker
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			values, err := repo.base.ClaimOutbox(ctx, worker, 1, now.Add(time.Minute))
			results <- claimResult{worker: worker, values: values, err: err}
		}()
	}
	close(start)
	workers.Wait()
	close(results)

	claimedCount := 0
	owners := make(map[string]struct{})
	for result := range results {
		if result.err != nil {
			t.Fatalf("%s claim failed: %v", result.worker, result.err)
		}
		claimedCount += len(result.values)
		for _, envelope := range result.values {
			if envelope.EventID == "" || envelope.LeaseToken == "" {
				t.Fatalf("%s received incomplete lease: %+v", result.worker, envelope)
			}
			owners[result.worker] = struct{}{}
		}
	}
	if claimedCount != 1 || len(owners) != 1 {
		t.Fatalf("concurrent claims acquired %d leases across %d workers, want exactly one lease", claimedCount, len(owners))
	}

	var status, owner, token string
	var attempts int
	if err := db.QueryRowContext(ctx, "SELECT delivery_status, lease_owner, lease_token, attempt_count FROM research_notification_outbox WHERE subject_ref=?", "research-event:"+eventID).Scan(&status, &owner, &token, &attempts); err != nil {
		t.Fatalf("read concurrently claimed Outbox state: %v", err)
	}
	if status != "leased" || owner == "" || token == "" || attempts != 1 {
		t.Fatalf("unexpected concurrent Outbox state: status=%s owner=%s token_set=%t attempts=%d", status, owner, token != "", attempts)
	}
}

func TestResearchMigrationsUpgradeHistoricalRowsFromV1ToV5(t *testing.T) {
	dsn := os.Getenv("RESEARCH_MIGRATION_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Skip("RESEARCH_MIGRATION_TEST_MYSQL_DSN is not set; skipping isolated migration upgrade test")
	}
	config, err := mysql.ParseDSN(dsn)
	if err != nil {
		t.Fatalf("parse RESEARCH_MIGRATION_TEST_MYSQL_DSN: %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(config.DBName), "_migration_test") {
		t.Fatalf("RESEARCH_MIGRATION_TEST_MYSQL_DSN database %q is unsafe; isolated database name must end with _migration_test", config.DBName)
	}
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatalf("open isolated MariaDB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping isolated MariaDB: %v", err)
	}
	var existingTables int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE()").Scan(&existingTables); err != nil {
		t.Fatalf("inspect isolated migration database: %v", err)
	}
	if existingTables != 0 {
		t.Fatalf("isolated migration database contains %d tables; refusing destructive upgrade test", existingTables)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"research_notification_outbox",
			"research_claim_versions",
			"research_event_versions",
			"research_evidence_records",
			"research_context_versions",
			"research_aggregate_heads",
			"platform_schema_migrations",
		} {
			_, _ = db.ExecContext(context.Background(), "DROP TABLE IF EXISTS "+table)
		}
	})

	migrations := Migrations()
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, migrations[:1]); err != nil {
		t.Fatalf("apply Research migration v1: %v", err)
	}

	const (
		contextTime  = "2026-11-08 12:34:56.123456"
		observedTime = "2026-11-08 12:34:57.654321"
		asOfTime     = "2026-11-08 12:35:00.000001"
		createdTime  = "2026-11-08 12:36:00.654321"
	)
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_context_versions (context_id, context_version, system_as_of, payload_sha256, payload, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		"ctx_migration_fixture", 1, contextTime, strings.Repeat("a", 64), "{\"legacy\":\"context\"}", createdTime); err != nil {
		t.Fatalf("seed v1 Context row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_evidence_records (evidence_id, source_owner, source_ref, source_version, available_at, system_observed_at, system_as_of, quality_status, content_level, authorization_ref, content_fingerprint, payload_sha256, payload, created_at) VALUES (?, ?, ?, NULL, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"ev_migration_fixture", "data", "data:fixture:1", observedTime, asOfTime, "valid", "metadata", "auth:migration", strings.Repeat("b", 64), strings.Repeat("c", 64), "{\"legacy\":\"evidence\"}", createdTime); err != nil {
		t.Fatalf("seed v1 Evidence row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_event_versions (event_id, event_version, status, system_as_of, payload_sha256, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
		"evt_migration_fixture", 1, "detected", asOfTime, strings.Repeat("d", 64), "{\"legacy\":\"event\"}", createdTime); err != nil {
		t.Fatalf("seed v1 Event row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_claim_versions (claim_id, claim_version, epistemic_type, status, system_as_of, payload_sha256, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		"clm_migration_fixture", 1, "reported_fact", "proposed", asOfTime, strings.Repeat("e", 64), "{\"legacy\":\"claim\"}", createdTime); err != nil {
		t.Fatalf("seed v1 Claim row: %v", err)
	}

	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, migrations[:4]); err != nil {
		t.Fatalf("upgrade Research schema from v1 through v4: %v", err)
	}

	const outboxPIT = "2026-11-08T12:36:00.654321000Z"
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_notification_outbox (event_id, event_type, schema_version, source_module, occurred_at, published_at, subject_ref, idempotency_key, correlation_id, payload_fingerprint, payload, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"nte_migration_fixture", "research.lifecycle_changed", "1.0.0", "research",
		outboxPIT, outboxPIT, "research-event:evt_migration_fixture", "migration-fixture-key",
		"corr_migration_fixture", strings.Repeat("f", 64), "{\"legacy\":\"outbox\"}", createdTime); err != nil {
		t.Fatalf("seed v4 Outbox row: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, migrations); err != nil {
		t.Fatalf("upgrade Research schema from v4 to v5: %v", err)
	}

	assertExact := func(name, query, want string, args ...any) {
		t.Helper()
		var got string
		if err := db.QueryRowContext(ctx, query, args...).Scan(&got); err != nil {
			t.Fatalf("read migrated %s: %v", name, err)
		}
		if got != want {
			t.Fatalf("migrated %s = %q, want %q", name, got, want)
		}
	}
	assertExact("Context system_as_of", "SELECT system_as_of FROM research_context_versions WHERE context_id=?", "2026-11-08T12:34:56.123456000Z", "ctx_migration_fixture")
	assertExact("Evidence system_observed_at", "SELECT system_observed_at FROM research_evidence_records WHERE evidence_id=?", "2026-11-08T12:34:57.654321000Z", "ev_migration_fixture")
	assertExact("Evidence system_as_of", "SELECT system_as_of FROM research_evidence_records WHERE evidence_id=?", "2026-11-08T12:35:00.000001000Z", "ev_migration_fixture")
	assertExact("Event system_as_of", "SELECT system_as_of FROM research_event_versions WHERE event_id=?", "2026-11-08T12:35:00.000001000Z", "evt_migration_fixture")
	assertExact("Claim system_as_of", "SELECT system_as_of FROM research_claim_versions WHERE claim_id=?", "2026-11-08T12:35:00.000001000Z", "clm_migration_fixture")
	var availableAt sql.NullString
	if err := db.QueryRowContext(ctx, "SELECT available_at FROM research_evidence_records WHERE evidence_id=?", "ev_migration_fixture").Scan(&availableAt); err != nil {
		t.Fatalf("read migrated Evidence available_at: %v", err)
	}
	if availableAt.Valid {
		t.Fatalf("Evidence available_at = %q, want preserved NULL", availableAt.String)
	}
	assertExact("Context payload", "SELECT payload FROM research_context_versions WHERE context_id=?", "{\"legacy\":\"context\"}", "ctx_migration_fixture")
	assertExact("Evidence payload", "SELECT payload FROM research_evidence_records WHERE evidence_id=?", "{\"legacy\":\"evidence\"}", "ev_migration_fixture")
	assertExact("Event payload", "SELECT payload FROM research_event_versions WHERE event_id=?", "{\"legacy\":\"event\"}", "evt_migration_fixture")
	assertExact("Claim payload", "SELECT payload FROM research_claim_versions WHERE claim_id=?", "{\"legacy\":\"claim\"}", "clm_migration_fixture")

	var eventType, claimType string
	if err := db.QueryRowContext(ctx, "SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='research_event_versions' AND COLUMN_NAME='event_version'").Scan(&eventType); err != nil {
		t.Fatalf("read migrated Event version type: %v", err)
	}
	if err := db.QueryRowContext(ctx, "SELECT DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=DATABASE() AND TABLE_NAME='research_claim_versions' AND COLUMN_NAME='claim_version'").Scan(&claimType); err != nil {
		t.Fatalf("read migrated Claim version type: %v", err)
	}
	if eventType != "bigint" || claimType != "bigint" {
		t.Fatalf("migrated version types = Event:%q Claim:%q, want bigint", eventType, claimType)
	}

	var status, available string
	var attempts int
	var updated string
	if err := db.QueryRowContext(ctx,
		"SELECT delivery_status, available_at, attempt_count, DATE_FORMAT(updated_at, '%Y-%m-%d %H:%i:%s.%f') FROM research_notification_outbox WHERE event_id=?",
		"nte_migration_fixture").Scan(&status, &available, &attempts, &updated); err != nil {
		t.Fatalf("read migrated Outbox row: %v", err)
	}
	if status != "pending" || available != outboxPIT || attempts != 0 || updated != createdTime {
		t.Fatalf("migrated Outbox state = status:%q available_at:%q attempts:%d updated_at:%q", status, available, attempts, updated)
	}
}

func TestResearchSQLRepositoriesAppendAndReadVersions(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	now := time.Date(2026, 11, 6, 8, 0, 0, 123456789, time.UTC)
	clock := func() time.Time { return now }

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	contextID := "ctx-it-" + suffix
	eventID := "event-it-" + suffix
	claimID := "claim-it-" + suffix
	evidenceID := "evidence-it-" + suffix
	concurrentEvidenceID := "evidence-concurrent-it-" + suffix
	cancelledContextID := "ctx-cancel-it-" + suffix
	orphanContextID := "ctx-orphan-it-" + suffix
	lockedContextID := "ctx-lock-cancel-it-" + suffix

	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_context_versions WHERE context_id IN (?, ?, ?)", contextID, orphanContextID, lockedContextID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_claim_versions WHERE claim_id=?", claimID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_evidence_records WHERE evidence_id IN (?, ?)", evidenceID, concurrentEvidenceID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_id IN (?, ?, ?, ?, ?, ?)", contextID, eventID, claimID, cancelledContextID, orphanContextID, lockedContextID)
	})

	contextRepo, err := NewContextRepository(db, clock)
	if err != nil {
		t.Fatal(err)
	}
	eventRepo, err := NewEventRepository(db, clock)
	if err != nil {
		t.Fatal(err)
	}
	claimRepo, err := NewClaimRepository(db, clock)
	if err != nil {
		t.Fatal(err)
	}

	cutoff := now
	contextV1 := researchcontext.Context{
		SchemaVersion: "1.0.0", ContextID: contextID, ContextVersion: 1,
		ResearchQuestion: "integration test", Scope: researchcontext.ScopeStock,
		SubjectRefs: []string{"600000.SH"}, SystemAsOf: cutoff,
		SectionRefs: []researchcontext.SectionRef{{
			SectionID: "profile", SectionVersion: "v1", SourceOwner: researchcontext.SourceOwnerData,
			SourceRefs: []string{"data:fixture:1"}, EvidenceRefs: []string{},
			QualityStatus: researchcontext.QualityValid, Required: true, Included: true,
			AssembledAt: cutoff, SystemAsOf: cutoff,
		}},
		QualitySummary:   researchcontext.QualitySummary{Status: researchcontext.QualityValid, ReasonCodes: []string{}},
		IncludedSections: []string{"profile"}, OmittedSections: []researchcontext.OmittedSection{},
		BuilderVersion: "integration-v1", CreatedAt: cutoff,
	}

	// Seed an orphan immutable row to force a duplicate-key failure after the
	// adapter has created/locked the aggregate head. The transaction must roll
	// back that newly-created head row.
	if _, err := db.ExecContext(ctx, "INSERT INTO research_context_versions "+
		"(context_id, context_version, system_as_of, payload_sha256, payload, created_at) VALUES (?, 1, ?, ?, ?, ?)",
		orphanContextID, formatPIT(cutoff), payloadDigest([]byte("{}")), "{}", cutoff); err != nil {
		t.Fatalf("seed orphan Context version: %v", err)
	}
	orphanContext := contextV1
	orphanContext.ContextID = orphanContextID
	if err := contextRepo.Append(ctx, orphanContext); err == nil {
		t.Fatal("append over orphan Context version must fail")
	}
	var orphanHeadCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_aggregate_heads "+
		"WHERE aggregate_type='context' AND aggregate_id=?", orphanContextID).Scan(&orphanHeadCount); err != nil {
		t.Fatalf("check rolled-back aggregate head: %v", err)
	}
	if orphanHeadCount != 0 {
		t.Fatalf("failed append left %d aggregate head rows, want 0", orphanHeadCount)
	}

	if err := contextRepo.Append(ctx, contextV1); err != nil {
		t.Fatalf("append Context v1: %v", err)
	}
	assertResearchPITColumn(t, ctx, db, "SELECT system_as_of FROM research_context_versions WHERE context_id=? AND context_version=1", []any{contextID}, cutoff)
	if err := contextRepo.Append(ctx, contextV1); err != nil {
		t.Fatalf("replay Context v1: %v", err)
	}
	conflictingContext := contextV1
	conflictingContext.ResearchQuestion = "different payload"
	if err := contextRepo.Append(ctx, conflictingContext); !errors.Is(err, ErrConflict) {
		t.Fatalf("conflicting Context replay error = %v, want ErrConflict", err)
	}
	contextGap := contextV1
	contextGap.ContextVersion = 3
	if err := contextRepo.Append(ctx, contextGap); !errors.Is(err, ErrConflict) {
		t.Fatalf("Context version gap error = %v, want ErrConflict", err)
	}
	contextV2 := contextV1
	contextV2.ContextVersion = 2
	contextV2.CreatedAt = cutoff.Add(time.Minute)
	contextV2.SystemAsOf = cutoff.Add(time.Minute)
	contextV2.SectionRefs = append([]researchcontext.SectionRef(nil), contextV1.SectionRefs...)
	contextV2.SectionRefs[0].SystemAsOf = contextV2.SystemAsOf
	contextV2.SectionRefs[0].AssembledAt = contextV2.SystemAsOf
	if err := contextRepo.Append(ctx, contextV2); err != nil {
		t.Fatalf("append Context v2: %v", err)
	}
	if got, err := contextRepo.GetVersion(ctx, contextID, 1); err != nil || got.ContextVersion != 1 {
		t.Fatalf("get Context v1 = version %d, error %v", got.ContextVersion, err)
	}

	// Identical concurrent appends must converge to one immutable version.
	contextV3 := contextV2
	contextV3.ContextVersion = 3
	contextV3.CreatedAt = cutoff.Add(2 * time.Minute)
	contextV3.SystemAsOf = cutoff.Add(2 * time.Minute)
	contextV3.SectionRefs = append([]researchcontext.SectionRef(nil), contextV2.SectionRefs...)
	contextV3.SectionRefs[0].SystemAsOf = contextV3.SystemAsOf
	contextV3.SectionRefs[0].AssembledAt = contextV3.SystemAsOf
	var concurrent sync.WaitGroup
	replayErrors := make(chan error, 2)
	for i := 0; i < 2; i++ {
		concurrent.Add(1)
		go func() {
			defer concurrent.Done()
			replayErrors <- contextRepo.Append(ctx, contextV3)
		}()
	}
	concurrent.Wait()
	close(replayErrors)
	for err := range replayErrors {
		if err != nil {
			t.Fatalf("concurrent identical Context append: %v", err)
		}
	}

	// Conflicting payloads for one version must have exactly one winner.
	contextV4A := contextV3
	contextV4A.ContextVersion = 4
	contextV4A.CreatedAt = cutoff.Add(3 * time.Minute)
	contextV4A.SystemAsOf = cutoff.Add(3 * time.Minute)
	contextV4A.SectionRefs = append([]researchcontext.SectionRef(nil), contextV3.SectionRefs...)
	contextV4A.SectionRefs[0].SystemAsOf = contextV4A.SystemAsOf
	contextV4A.SectionRefs[0].AssembledAt = contextV4A.SystemAsOf
	contextV4B := contextV4A
	contextV4B.ResearchQuestion = "concurrent competing payload"
	var outcomes sync.WaitGroup
	conflictErrors := make(chan error, 2)
	for _, candidate := range []researchcontext.Context{contextV4A, contextV4B} {
		candidate := candidate
		outcomes.Add(1)
		go func() {
			defer outcomes.Done()
			conflictErrors <- contextRepo.Append(ctx, candidate)
		}()
	}
	outcomes.Wait()
	close(conflictErrors)
	successes, conflicts := 0, 0
	for err := range conflictErrors {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent Context append error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent conflicting append results: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}

	// A request canceled before Append starts must not create an aggregate.
	cancelledContext := contextV1
	cancelledContext.ContextID = cancelledContextID
	cancelCtx, cancelAppend := context.WithCancel(ctx)
	cancelAppend()
	if err := contextRepo.Append(cancelCtx, cancelledContext); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Context append error = %v, want context.Canceled", err)
	}
	if _, err := contextRepo.GetVersion(ctx, cancelledContextID, 1); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cancelled Context append GetVersion error = %v, want ErrNotFound", err)
	}
	var cancelledHeadCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_aggregate_heads WHERE aggregate_type='context' AND aggregate_id=?", cancelledContextID).Scan(&cancelledHeadCount); err != nil {
		t.Fatalf("check pre-canceled aggregate head: %v", err)
	}
	if cancelledHeadCount != 0 {
		t.Fatalf("pre-canceled append left %d aggregate head rows, want 0", cancelledHeadCount)
	}

	// Cancellation while waiting for the aggregate-head row lock must abort
	// the append transaction and leave the previous version/head unchanged.
	lockedContextV1 := contextV1
	lockedContextV1.ContextID = lockedContextID
	if err := contextRepo.Append(ctx, lockedContextV1); err != nil {
		t.Fatalf("append lock-cancel Context v1: %v", err)
	}
	blocker, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin aggregate-head lock transaction: %v", err)
	}
	if _, err := blocker.ExecContext(ctx, "UPDATE research_aggregate_heads SET updated_at=updated_at WHERE aggregate_type='context' AND aggregate_id=?", lockedContextID); err != nil {
		_ = blocker.Rollback()
		t.Fatalf("lock aggregate head: %v", err)
	}
	lockedContextV2 := lockedContextV1
	lockedContextV2.ContextVersion = 2
	lockedContextV2.CreatedAt = cutoff.Add(time.Minute)
	lockedContextV2.SystemAsOf = cutoff.Add(time.Minute)
	lockedContextV2.SectionRefs = append([]researchcontext.SectionRef(nil), lockedContextV1.SectionRefs...)
	lockedContextV2.SectionRefs[0].SystemAsOf = lockedContextV2.SystemAsOf
	lockedContextV2.SectionRefs[0].AssembledAt = lockedContextV2.SystemAsOf
	lockWaitCtx, cancelLockWait := context.WithCancel(ctx)
	appendResult := make(chan error, 1)
	appendResultReceived := false
	started := make(chan struct{})
	go func() {
		close(started)
		appendResult <- contextRepo.Append(lockWaitCtx, lockedContextV2)
	}()
	// 无论断言在哪个分支失败，都先取消请求并释放阻塞事务，再回收工作协程。
	defer func() {
		cancelLockWait()
		_ = blocker.Rollback()
		if !appendResultReceived {
			select {
			case <-appendResult:
			case <-time.After(5 * time.Second):
				t.Errorf("append goroutine did not exit after lock-wait test cleanup")
			}
		}
	}()
	<-started
	// Wait until the append SQL is visible in MariaDB's process list while the
	// blocker still owns the row lock; this avoids mistaking pre-cancellation
	// for cancellation during an actual database lock wait.
	waitDeadline := time.Now().Add(5 * time.Second)
	lockWaitObserved := false
	for time.Now().Before(waitDeadline) {
		var active int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.PROCESSLIST WHERE INFO LIKE 'INSERT INTO research_aggregate_heads%' OR INFO LIKE 'SELECT latest_version FROM research_aggregate_heads%'").Scan(&active); err != nil {
			cancelLockWait()
			_ = blocker.Rollback()
			t.Fatalf("inspect MariaDB process list for blocked append: %v", err)
		}
		if active > 0 {
			lockWaitObserved = true
			break
		}
		select {
		case appendErr := <-appendResult:
			appendResultReceived = true
			cancelLockWait()
			_ = blocker.Rollback()
			t.Fatalf("append returned before lock wait was observed: %v", appendErr)
		case <-time.After(20 * time.Millisecond):
		}
	}
	if !lockWaitObserved {
		cancelLockWait()
		_ = blocker.Rollback()
		t.Fatal("MariaDB did not expose the append statement while the aggregate-head lock was held")
	}
	cancelLockWait()
	var appendErr error
	select {
	case appendErr = <-appendResult:
		appendResultReceived = true
	case <-time.After(5 * time.Second):
		_ = blocker.Rollback()
		t.Fatal("append did not return after lock-wait context cancellation")
	}
	if err := blocker.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("release aggregate-head lock: %v", err)
	}
	if !errors.Is(appendErr, context.Canceled) {
		t.Fatalf("lock-wait append error = %v, want context.Canceled", appendErr)
	}
	var lockedLatest int64
	if err := db.QueryRowContext(ctx, "SELECT latest_version FROM research_aggregate_heads WHERE aggregate_type='context' AND aggregate_id=?", lockedContextID).Scan(&lockedLatest); err != nil {
		t.Fatalf("read aggregate head after canceled lock wait: %v", err)
	}
	if lockedLatest != 1 {
		t.Fatalf("aggregate head after canceled lock wait = %d, want 1", lockedLatest)
	}
	var lockedV2Count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM research_context_versions WHERE context_id=? AND context_version=2", lockedContextID).Scan(&lockedV2Count); err != nil {
		t.Fatalf("read version after canceled lock wait: %v", err)
	}
	if lockedV2Count != 0 {
		t.Fatalf("canceled lock-wait append left %d version-2 rows, want 0", lockedV2Count)
	}

	eventV1 := researchevent.ResearchEvent{
		EventID: eventID, EventVersion: 1, EventType: "integration_fixture",
		Title: "Research storage integration", Status: researchevent.StatusDetected,
		SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: cutoff, SystemAsOf: cutoff,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := eventRepo.Append(ctx, eventV1); err != nil {
		t.Fatalf("append Event v1: %v", err)
	}
	assertResearchPITColumn(t, ctx, db, "SELECT system_as_of FROM research_event_versions WHERE event_id=? AND event_version=1", []any{eventID}, cutoff)
	if err := eventRepo.Append(ctx, eventV1); err != nil {
		t.Fatalf("replay Event v1: %v", err)
	}
	eventGap := eventV1
	eventGap.EventVersion = 3
	if err := eventRepo.Append(ctx, eventGap); !errors.Is(err, ErrConflict) {
		t.Fatalf("Event version gap error = %v, want ErrConflict", err)
	}
	eventV2 := eventV1
	eventV2.EventVersion = 2
	eventV2.SystemAsOf = cutoff.Add(time.Minute)
	if err := eventRepo.Append(ctx, eventV2); err != nil {
		t.Fatalf("append Event v2: %v", err)
	}
	if got, err := eventRepo.GetVersion(ctx, eventID, 2); err != nil || got.EventVersion != 2 {
		t.Fatalf("get Event v2 = version %d, error %v", got.EventVersion, err)
	}

	claimV1 := researchclaim.Claim{
		ClaimID: claimID, ClaimVersion: 1, Statement: "integration fixture",
		EpistemicType: researchclaim.EpistemicReportedFact,
		ScopeRefs:     []string{}, SystemAsOf: cutoff,
		SupportingEvidenceRefs: []string{}, ContradictingEvidenceRefs: []string{},
		RequiredDataRefs: []string{}, Status: researchclaim.StatusProposed,
		Limitations: []string{},
	}
	if err := claimRepo.Append(ctx, claimV1); err != nil {
		t.Fatalf("append Claim v1: %v", err)
	}
	assertResearchPITColumn(t, ctx, db, "SELECT system_as_of FROM research_claim_versions WHERE claim_id=? AND claim_version=1", []any{claimID}, cutoff)
	if err := claimRepo.Append(ctx, claimV1); err != nil {
		t.Fatalf("replay Claim v1: %v", err)
	}
	claimGap := claimV1
	claimGap.ClaimVersion = 3
	if err := claimRepo.Append(ctx, claimGap); !errors.Is(err, ErrConflict) {
		t.Fatalf("Claim version gap error = %v, want ErrConflict", err)
	}
	claimV2 := claimV1
	claimV2.ClaimVersion = 2
	claimV2.SystemAsOf = cutoff.Add(time.Minute)
	if err := claimRepo.Append(ctx, claimV2); err != nil {
		t.Fatalf("append Claim v2: %v", err)
	}
	if got, err := claimRepo.GetVersion(ctx, claimID, 2); err != nil || got.ClaimVersion != 2 {
		t.Fatalf("get Claim v2 = version %d, error %v", got.ClaimVersion, err)
	}

	evidenceRepo, err := NewEvidenceRepository(db, clock)
	if err != nil {
		t.Fatal(err)
	}
	availableAt := cutoff.Add(-time.Second)
	evidenceV1 := researchevidence.Evidence{
		EvidenceID: evidenceID, EvidenceType: researchevidence.TypeFact,
		SourceOwner: researchevidence.SourceData, SourceRef: "data:integration:fixture",
		DerivedFromRefs: []string{}, SubjectRefs: []string{},
		SystemObservedAt: cutoff, SystemAsOf: cutoff, AvailableAt: &availableAt,
		Stance: researchevidence.StanceNeutral, QualityStatus: researchevidence.QualityValid,
		ContentLevel: researchevidence.ContentMetadata, AuthorizationRef: "auth-integration-v1",
		Limitations: []string{},
	}
	evidenceV1.ContentFingerprint, err = evidenceV1.ComputeContentFingerprint()
	if err != nil {
		t.Fatalf("compute Evidence fingerprint: %v", err)
	}
	if err := evidenceRepo.Append(ctx, evidenceV1); err != nil {
		t.Fatalf("append Evidence: %v", err)
	}

	// Two writers with different immutable authorization snapshots must not
	// overwrite each other when they race to create the same Evidence ID.
	concurrentEvidence := evidenceV1
	concurrentEvidence.EvidenceID = concurrentEvidenceID
	concurrentEvidence.ContentFingerprint, err = concurrentEvidence.ComputeContentFingerprint()
	if err != nil {
		t.Fatalf("compute concurrent Evidence fingerprint: %v", err)
	}
	competingEvidence := concurrentEvidence
	competingEvidence.AuthorizationRef = "auth-integration-competing"
	writeErrors := make(chan error, 2)
	var writers sync.WaitGroup
	writers.Add(2)
	go func() {
		defer writers.Done()
		writeErrors <- evidenceRepo.Append(ctx, concurrentEvidence)
	}()
	go func() {
		defer writers.Done()
		writeErrors <- evidenceRepo.Append(ctx, competingEvidence)
	}()
	writers.Wait()
	close(writeErrors)
	successes, conflicts = 0, 0
	for writeErr := range writeErrors {
		switch {
		case writeErr == nil:
			successes++
		case errors.Is(writeErr, ErrConflict):
			conflicts++
		default:
			t.Fatalf("concurrent Evidence append returned unexpected error: %v", writeErr)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent Evidence append results: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}
	storedConcurrent, err := evidenceRepo.Get(ctx, concurrentEvidenceID)
	if err != nil {
		t.Fatalf("read winning concurrent Evidence: %v", err)
	}
	if storedConcurrent.AuthorizationRef != concurrentEvidence.AuthorizationRef &&
		storedConcurrent.AuthorizationRef != competingEvidence.AuthorizationRef {
		t.Fatalf("stored concurrent authorization_ref = %q, not either submitted snapshot", storedConcurrent.AuthorizationRef)
	}

	assertResearchPITColumn(t, ctx, db, "SELECT system_as_of FROM research_evidence_records WHERE evidence_id=?", []any{evidenceID}, cutoff)
	assertResearchPITColumn(t, ctx, db, "SELECT system_observed_at FROM research_evidence_records WHERE evidence_id=?", []any{evidenceID}, cutoff)
	assertResearchPITColumn(t, ctx, db, "SELECT available_at FROM research_evidence_records WHERE evidence_id=?", []any{evidenceID}, availableAt)
	if err := evidenceRepo.Append(ctx, evidenceV1); err != nil {
		t.Fatalf("replay Evidence: %v", err)
	}
	if got, err := evidenceRepo.Get(ctx, evidenceID); err != nil || got.AuthorizationRef != evidenceV1.AuthorizationRef {
		t.Fatalf("get Evidence authorization_ref = %q, error %v", got.AuthorizationRef, err)
	}
	evidenceRenewed := evidenceV1
	evidenceRenewed.AuthorizationRef = "auth-integration-v2"
	if err := evidenceRepo.Append(ctx, evidenceRenewed); !errors.Is(err, ErrConflict) {
		t.Fatalf("Evidence authorization snapshot replacement error = %v, want ErrConflict", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE research_evidence_records SET payload=CONCAT(payload, ' ') WHERE evidence_id=?", evidenceID); err != nil {
		t.Fatalf("tamper stored Evidence payload: %v", err)
	}
	if _, err := evidenceRepo.Get(ctx, evidenceID); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Evidence read error = %v, want ErrCorruptRecord", err)
	}

	if _, err := db.ExecContext(ctx, "UPDATE research_context_versions SET payload=CONCAT(payload, ' ') WHERE context_id=? AND context_version=2", contextID); err != nil {
		t.Fatalf("tamper stored Context payload: %v", err)
	}
	if _, err := contextRepo.GetVersion(ctx, contextID, 2); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Context read error = %v, want ErrCorruptRecord", err)
	}
	if err := contextRepo.Append(ctx, contextV2); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Context idempotent replay error = %v, want ErrCorruptRecord", err)
	}
	latestContext, err := contextRepo.GetVersion(ctx, contextID, 4)
	if err != nil {
		t.Fatalf("read latest Context before corruption: %v", err)
	}
	contextV5 := latestContext
	contextV5.ContextVersion = 5
	contextV5.SystemAsOf = latestContext.SystemAsOf.Add(time.Minute)
	contextV5.CreatedAt = latestContext.CreatedAt.Add(time.Minute)
	contextV5.SectionRefs = append([]researchcontext.SectionRef(nil), latestContext.SectionRefs...)
	for i := range contextV5.SectionRefs {
		contextV5.SectionRefs[i].SystemAsOf = contextV5.SystemAsOf
		contextV5.SectionRefs[i].AssembledAt = contextV5.SystemAsOf
	}
	if _, err := db.ExecContext(ctx, "UPDATE research_context_versions SET payload=CONCAT(payload, ' ') WHERE context_id=? AND context_version=4", contextID); err != nil {
		t.Fatalf("tamper latest Context payload: %v", err)
	}
	if _, err := contextRepo.GetVersion(ctx, contextID, 4); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered latest Context read error = %v, want ErrCorruptRecord", err)
	}
	if err := contextRepo.Append(ctx, contextV5); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("append after tampered latest Context error = %v, want ErrCorruptRecord", err)
	}

	// Event 历史版本损坏后，精确读取和后续版本追加都必须 fail-closed。
	if _, err := db.ExecContext(ctx, "UPDATE research_event_versions SET payload=CONCAT(payload, ' ') WHERE event_id=? AND event_version=2", eventID); err != nil {
		t.Fatalf("tamper stored Event payload: %v", err)
	}
	if _, err := eventRepo.GetVersion(ctx, eventID, 2); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Event read error = %v, want ErrCorruptRecord", err)
	}
	eventV3 := eventV2
	eventV3.EventVersion = 3
	eventV3.SystemAsOf = eventV2.SystemAsOf.Add(time.Minute)
	if err := eventRepo.Append(ctx, eventV3); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("append after tampered previous Event error = %v, want ErrCorruptRecord", err)
	}

	// Claim 历史版本损坏后，同样不得读取或继续追加。
	if _, err := db.ExecContext(ctx, "UPDATE research_claim_versions SET payload=CONCAT(payload, ' ') WHERE claim_id=? AND claim_version=2", claimID); err != nil {
		t.Fatalf("tamper stored Claim payload: %v", err)
	}
	if _, err := claimRepo.GetVersion(ctx, claimID, 2); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("tampered Claim read error = %v, want ErrCorruptRecord", err)
	}
	claimV3 := claimV2
	claimV3.ClaimVersion = 3
	claimV3.SystemAsOf = claimV2.SystemAsOf.Add(time.Minute)
	if err := claimRepo.Append(ctx, claimV3); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("append after tampered previous Claim error = %v, want ErrCorruptRecord", err)
	}
}

func TestResearchAppendRejectsAggregateHeadWithMissingLatestVersion(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	eventID := fmt.Sprintf("event-missing-latest-%d", time.Now().UnixNano())
	now := time.Date(2026, 11, 9, 5, 0, 0, 123456789, time.UTC)
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_event_versions WHERE event_id=?", eventID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID)
	})

	// A head claiming version 1 without the corresponding immutable row is
	// persisted-state corruption, not a normal version conflict or DB outage.
	if _, err := db.ExecContext(ctx,
		"INSERT INTO research_aggregate_heads (aggregate_type, aggregate_id, latest_version, created_at, updated_at) VALUES ('event', ?, 1, ?, ?)",
		eventID, now, now); err != nil {
		t.Fatalf("seed aggregate head without version row: %v", err)
	}

	repo, err := NewEventRepository(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	next := researchevent.ResearchEvent{
		EventID: eventID, EventVersion: 2, EventType: "integration_fixture",
		Title: "Missing latest version fixture", Status: researchevent.StatusDetected,
		SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := repo.Append(ctx, next); !errors.Is(err, ErrCorruptRecord) {
		t.Fatalf("append with missing latest Event row = %v, want ErrCorruptRecord", err)
	}
	var latest int64
	if err := db.QueryRowContext(ctx,
		"SELECT latest_version FROM research_aggregate_heads WHERE aggregate_type='event' AND aggregate_id=?", eventID).Scan(&latest); err != nil {
		t.Fatalf("read aggregate head after rejected append: %v", err)
	}
	if latest != 1 {
		t.Fatalf("aggregate head advanced to %d after missing-version rejection, want 1", latest)
	}
	var versionCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM research_event_versions WHERE event_id=?", eventID).Scan(&versionCount); err != nil {
		t.Fatalf("count Event versions after rejected append: %v", err)
	}
	if versionCount != 0 {
		t.Fatalf("append created %d Event versions despite missing previous version", versionCount)
	}
}

func TestResearchOutboxRejectsExpiredLeaseEvenWithBackdatedActionTime(t *testing.T) {
	db := openResearchIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	now := time.Date(2026, 11, 9, 6, 0, 0, 123456789, time.UTC)
	eventID := fmt.Sprintf("event-outbox-expired-lease-%d", time.Now().UnixNano())
	repo, err := NewEventRepository(db, func() time.Time { return now })
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
		Title: "Expired lease fixture", Status: researchevent.StatusDetected,
		SubjectRefs: []string{}, EvidenceRefs: []string{},
		SystemObservedAt: now, SystemAsOf: now,
		ImpactHypothesisRefs: []string{}, InvalidationConditions: []string{},
		Uncertainty: []string{}, SourceTimes: []researchevent.EventSourceTime{},
	}
	if err := repo.AppendWithLifecycle(ctx, value, nil, "corr_expired_lease"); err != nil {
		t.Fatalf("append Event and Outbox: %v", err)
	}
	claimed, err := repo.base.ClaimOutbox(ctx, "worker_expired", 10, now.Add(time.Minute))
	if err != nil || len(claimed) != 1 {
		t.Fatalf("claim Outbox = %d messages, error %v", len(claimed), err)
	}

	// Advance the trusted repository clock beyond the lease deadline, but
	// deliberately supply action timestamps from before that deadline.
	now = now.Add(2 * time.Minute)
	backdated := now.Add(-2 * time.Minute)
	if err := repo.base.MarkOutboxPublished(ctx, claimed[0].EventID, "worker_expired", claimed[0].LeaseToken, backdated); !errors.Is(err, ErrConflict) {
		t.Fatalf("confirm with expired lease and backdated timestamp = %v, want ErrConflict", err)
	}
	if err := repo.base.ScheduleOutboxRetry(ctx, claimed[0].EventID, "worker_expired", claimed[0].LeaseToken, now.Add(time.Minute), "temporary_failure", backdated); !errors.Is(err, ErrConflict) {
		t.Fatalf("retry with expired lease and backdated timestamp = %v, want ErrConflict", err)
	}
	if err := repo.base.RejectOutbox(ctx, claimed[0].EventID, "worker_expired", claimed[0].LeaseToken, "permanent_failure", backdated); !errors.Is(err, ErrConflict) {
		t.Fatalf("reject with expired lease and backdated timestamp = %v, want ErrConflict", err)
	}

	var status string
	if err := db.QueryRowContext(ctx, "SELECT delivery_status FROM research_notification_outbox WHERE event_id=?", claimed[0].EventID).Scan(&status); err != nil {
		t.Fatalf("read expired Outbox state: %v", err)
	}
	if status != "leased" {
		t.Fatalf("expired action changed Outbox state to %q, want leased until reclaimed", status)
	}
	reclaimed, err := repo.base.ClaimOutbox(ctx, "worker_reclaimed", 10, now.Add(time.Minute))
	if err != nil || len(reclaimed) != 1 || reclaimed[0].LeaseToken == claimed[0].LeaseToken {
		t.Fatalf("reclaim expired Outbox lease = %+v, error %v", reclaimed, err)
	}
}
