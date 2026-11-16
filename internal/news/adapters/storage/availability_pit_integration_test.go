package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/news/app"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

func TestNewsAvailabilityPITResolverMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News availability PIT integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "news", Migrations()); err != nil {
		t.Fatalf("apply News migrations: %v", err)
	}
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	newsID := "availability-pit-news-" + suffix
	sourceID := "availability-pit-source-" + suffix
	observationID := "availability-pit-observation-" + suffix
	versionID := "availability-pit-version-" + suffix
	evidenceID := "availability-pit-evidence-" + suffix
	firstDecisionID := "availability-pit-decision-1-" + suffix
	secondDecisionID := "availability-pit-decision-2-" + suffix

	now := time.Now().UTC().Truncate(time.Microsecond)
	observedAt := now.Add(-time.Hour)
	recordedAt := now.Add(-10 * time.Second)
	firstDecisionAt := now.Add(-5 * time.Second)
	secondDecisionAt := now.Add(-2 * time.Second)
	retentionUntil := now.Add(time.Hour)
	candidateAvailableAt := observedAt.Truncate(time.Second).Add(-time.Second)

	if _, err := db.ExecContext(ctx, `INSERT INTO news_items (news_id, origin_source_id)
		VALUES (?, ?)`, newsID, sourceID); err != nil {
		t.Fatalf("insert News item: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_observations
		(observation_id, idempotency_key, source_id, provider_code, candidate_json,
		 observed_at, identity_status, news_id, retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observationID, observationID+"-key", sourceID, "test-provider", []byte(`{"title":"availability PIT fixture"}`),
		observedAt, "matched", newsID, retentionUntil,
	); err != nil {
		t.Fatalf("insert Observation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_item_versions
		(version_id, news_id, version_no, fingerprint, title, content_json, observed_at, observation_id)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?)`,
		versionID, newsID, strings.Repeat("a", 64), "availability PIT fixture",
		[]byte(`{"title":"availability PIT fixture"}`), observedAt, observationID,
	); err != nil {
		t.Fatalf("insert Version: %v", err)
	}

	assertAvailableAt := func(asOf time.Time, want *time.Time) {
		t.Helper()
		record, err := repository.GetEvidenceVersionAt(ctx, app.GetEvidenceRequest{
			NewsID: newsID, VersionID: versionID, SystemAsOf: asOf.UTC(),
		})
		if err != nil {
			t.Fatalf("GetEvidenceVersionAt(%s): %v", asOf, err)
		}
		if want == nil {
			if record.AvailableAt != nil {
				t.Fatalf("available_at = %s, want NULL", record.AvailableAt)
			}
			return
		}
		if record.AvailableAt == nil || !record.AvailableAt.Equal(*want) ||
			record.AvailableAt.Nanosecond() != 0 {
			t.Fatalf("available_at = %v, want %s", record.AvailableAt, want)
		}
	}

	assertAvailableAt(firstDecisionAt.Add(-time.Microsecond), nil)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_evidence
		(evidence_id, news_id, version_id, observation_id, field_path, raw_value,
		 source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		 candidate_available_at_utc, parser_version, observed_at, recorded_at,
		 retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		evidenceID, newsID, versionID, observationID, "data.available_time",
		candidateAvailableAt.Format(time.RFC3339), "Z", "second", "parsed",
		candidateAvailableAt, "availability-time-v1", observedAt, recordedAt, retentionUntil,
	); err != nil {
		t.Fatalf("insert availability Evidence: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_decisions
		(decision_id, version_id, decision_seq, status, selected_evidence_id,
		 considered_evidence_ids, reason_code, review_rule_version, recorded_at)
		VALUES (?, ?, 1, 'verified', ?, JSON_ARRAY(?), ?, ?, ?)`,
		firstDecisionID, versionID, evidenceID, evidenceID,
		"availability.source_verified", "review-v1", firstDecisionAt,
	); err != nil {
		t.Fatalf("insert verified Decision: %v", err)
	}
	assertAvailableAt(secondDecisionAt.Add(-time.Microsecond), &candidateAvailableAt)

	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_decisions
		(decision_id, version_id, decision_seq, status, selected_evidence_id,
		 considered_evidence_ids, reason_code, review_rule_version,
		 supersedes_decision_id, recorded_at)
		VALUES (?, ?, 2, 'unresolved', NULL, JSON_ARRAY(?), ?, ?, ?, ?)`,
		secondDecisionID, versionID, evidenceID,
		"availability.review_pending", "review-v1", firstDecisionID, secondDecisionAt,
	); err != nil {
		t.Fatalf("insert superseding unresolved Decision: %v", err)
	}
	assertAvailableAt(secondDecisionAt.Add(time.Second), nil)

	thirdDecisionID := "availability-pit-decision-3-" + suffix
	thirdDecisionAt := now.Add(-500 * time.Millisecond)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_decisions
		(decision_id, version_id, decision_seq, status, selected_evidence_id,
		 considered_evidence_ids, reason_code, review_rule_version,
		 supersedes_decision_id, recorded_at)
		VALUES (?, ?, 3, 'verified', ?, JSON_ARRAY(?), ?, ?, ?, ?)`,
		thirdDecisionID, versionID, evidenceID, evidenceID,
		"availability.reverified", "review-v1", secondDecisionID, thirdDecisionAt,
	); err != nil {
		t.Fatalf("insert re-verified Decision: %v", err)
	}
	assertAvailableAt(now, &candidateAvailableAt)

	purgedAt := now
	if _, err := db.ExecContext(ctx, `UPDATE news_version_availability_evidence
		SET raw_value = NULL, candidate_available_at_utc = NULL, payload_purged_at = ?
		WHERE evidence_id = ?`, purgedAt, evidenceID); err != nil {
		t.Fatalf("purge availability Evidence payload: %v", err)
	}
	assertAvailableAt(now, nil)
}