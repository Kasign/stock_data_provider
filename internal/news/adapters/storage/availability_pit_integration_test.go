package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
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

	// PIT 读取必须独立验证 selected_evidence_id 属于 considered_evidence_ids；
	// 即使数据库中存在绕过 Domain Validator 的异常历史决策，也不能采信。
	invalidDecisionID := "availability-pit-decision-4-" + suffix
	invalidDecisionAt := now.Add(-250 * time.Millisecond)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_decisions
		(decision_id, version_id, decision_seq, status, selected_evidence_id,
		 considered_evidence_ids, reason_code, review_rule_version,
		 supersedes_decision_id, recorded_at)
		VALUES (?, ?, 4, 'verified', ?, JSON_ARRAY(), ?, ?, ?, ?)`,
		invalidDecisionID, versionID, evidenceID,
		"availability.invalid_selected_evidence", "review-v1", thirdDecisionID, invalidDecisionAt,
	); err != nil {
		t.Fatalf("insert malformed verified Decision: %v", err)
	}
	assertAvailableAt(now, nil)

	purgedAt := now
	if _, err := db.ExecContext(ctx, `UPDATE news_version_availability_evidence
		SET raw_value = NULL, candidate_available_at_utc = NULL, payload_purged_at = ?
		WHERE evidence_id = ?`, purgedAt, evidenceID); err != nil {
		t.Fatalf("purge availability Evidence payload: %v", err)
	}
	assertAvailableAt(now, nil)
}

func TestAppendAvailabilityDecisionSerializesConcurrentWritersMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News availability decision integration")
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
	newsID := "availability-decision-news-" + suffix
	sourceID := "availability-decision-source-" + suffix
	observationID := "availability-decision-observation-" + suffix
	versionID := "availability-decision-version-" + suffix
	if _, err := db.ExecContext(ctx, `INSERT INTO news_items (news_id, origin_source_id) VALUES (?, ?)`, newsID, sourceID); err != nil {
		t.Fatalf("insert News item: %v", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	retentionUntil := now.Add(time.Hour)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_observations
		(observation_id, idempotency_key, source_id, provider_code, candidate_json,
		 observed_at, identity_status, news_id, retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observationID, observationID+"-key", sourceID, "test-provider",
		[]byte(`{"title":"decision append fixture"}`), now.Add(-time.Minute),
		"matched", newsID, retentionUntil,
	); err != nil {
		t.Fatalf("insert Observation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_item_versions
		(version_id, news_id, version_no, fingerprint, title, content_json, observed_at, observation_id)
		VALUES (?, ?, 1, ?, ?, ?, ?, ?)`,
		versionID, newsID, strings.Repeat("b", 64), "decision append fixture",
		[]byte(`{"title":"decision append fixture"}`), now.Add(-time.Minute), observationID,
	); err != nil {
		t.Fatalf("insert Version: %v", err)
	}
	evidenceID := "availability-decision-evidence-" + suffix
	candidateAvailableAt := now.Add(-2 * time.Minute).Truncate(time.Second)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_evidence
		(evidence_id, news_id, version_id, observation_id, field_path, raw_value,
		 source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		 candidate_available_at_utc, parser_version, observed_at, recorded_at,
		 retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?)`,
		evidenceID, newsID, versionID, observationID, "data.available_time",
		candidateAvailableAt.Format(time.RFC3339), "Z", "second", "parsed",
		candidateAvailableAt, "availability-time-v1", now.Add(-time.Minute),
		now.Add(-30*time.Second), retentionUntil,
	); err != nil {
		t.Fatalf("insert availability Evidence: %v", err)
	}

	recordedAt := now
	makeFirst := func(id string) domain.AvailabilityDecision {
		return domain.AvailabilityDecision{
			ID:                    id,
			VersionID:             versionID,
			Sequence:              1,
			Status:                domain.AvailabilityDecisionUnresolved,
			ConsideredEvidenceIDs: []string{},
			ReasonCode:            "availability.review_pending",
			ReviewRuleVersion:     "review-v1",
			RecordedAt:            recordedAt,
		}
	}
	unknownEvidence := makeFirst("availability-decision-unknown-evidence-" + suffix)
	unknownEvidence.ConsideredEvidenceIDs = []string{"evidence-from-another-version"}
	if err := repository.AppendAvailabilityDecision(ctx, unknownEvidence); err == nil {
		t.Fatal("append with an Evidence ID not belonging to this Version succeeded")
	}

	start := make(chan struct{})
	results := make(chan struct {
		id  string
		err error
	}, 2)
	ids := []string{"availability-decision-a-" + suffix, "availability-decision-b-" + suffix}
	for _, id := range ids {
		go func(id string) {
			<-start
			results <- struct {
				id  string
				err error
			}{id: id, err: repository.AppendAvailabilityDecision(ctx, makeFirst(id))}
		}(id)
	}
	close(start)

	var winner string
	successes := 0
	conflicts := 0
	for range ids {
		result := <-results
		switch {
		case result.err == nil:
			successes++
			winner = result.id
		case errors.Is(result.err, ErrAvailabilityDecisionSequenceConflict):
			conflicts++
		default:
			t.Fatalf("concurrent append returned unexpected error: %v", result.err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent append successes=%d conflicts=%d, want 1 and 1", successes, conflicts)
	}

	wrongParent := makeFirst("availability-decision-wrong-parent-" + suffix)
	wrongParent.Sequence = 2
	wrongParentID := "not-the-latest-decision"
	wrongParent.SupersedesDecisionID = &wrongParentID
	if err := repository.AppendAvailabilityDecision(ctx, wrongParent); !errors.Is(err, ErrAvailabilityDecisionSequenceConflict) {
		t.Fatalf("wrong predecessor error = %v, want sequence conflict", err)
	}

	next := domain.AvailabilityDecision{
		ID:                    "availability-decision-next-" + suffix,
		VersionID:             versionID,
		Sequence:              2,
		Status:                domain.AvailabilityDecisionUnresolved,
		ConsideredEvidenceIDs: []string{},
		ReasonCode:            "availability.review_pending",
		ReviewRuleVersion:     "review-v1",
		SupersedesDecisionID:  &winner,
		RecordedAt:            recordedAt,
	}
	if err := repository.AppendAvailabilityDecision(ctx, next); err != nil {
		t.Fatalf("append valid successor: %v", err)
	}
	selectedEvidenceID := evidenceID
	verified := domain.AvailabilityDecision{
		ID:                    "availability-decision-verified-" + suffix,
		VersionID:             versionID,
		Sequence:              3,
		Status:                domain.AvailabilityDecisionVerified,
		SelectedEvidenceID:    &selectedEvidenceID,
		ConsideredEvidenceIDs: []string{evidenceID},
		ReasonCode:            "availability.source_verified",
		ReviewRuleVersion:     "review-v1",
		SupersedesDecisionID:  &next.ID,
		RecordedAt:            recordedAt,
	}
	if err := repository.AppendAvailabilityDecision(ctx, verified); err != nil {
		t.Fatalf("append verified Decision with eligible Evidence: %v", err)
	}

	var count int
	var maxSequence uint64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*), MAX(decision_seq)
		FROM news_version_availability_decisions WHERE version_id = ?`, versionID).Scan(&count, &maxSequence); err != nil {
		t.Fatal(err)
	}
	if count != 3 || maxSequence != 3 {
		t.Fatalf("decision chain count=%d max_sequence=%d, want 3 and 3", count, maxSequence)
	}
}
