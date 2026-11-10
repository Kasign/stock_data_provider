package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/go-sql-driver/mysql"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

// repositoryIngestHarness 仅供 Storage 集成测试直接验证 Repository 的身份事务、
// 幂等键和历史查询；它不替代生产 Application 的 Source/Provider 授权入口。
type repositoryIngestHarness struct {
	repository app.IdentityRepository
	newID      func() string
}

func (h repositoryIngestHarness) Execute(
	ctx context.Context,
	sourceID, idempotencyKey string,
	candidate domain.Candidate,
	policy domain.SourceURLPolicy,
) (domain.Observation, error) {
	if h.repository == nil || h.newID == nil || ctx == nil || strings.TrimSpace(sourceID) == "" ||
		strings.TrimSpace(idempotencyKey) == "" || strings.TrimSpace(candidate.ProviderCode) != "" {
		if strings.TrimSpace(candidate.ProviderCode) != "" {
			return domain.Observation{}, app.ErrProviderOutputPolicyRequired
		}
		return domain.Observation{}, app.ErrInvalidRequest
	}
	observedAt := time.Now().UTC()
	candidate = domain.NormalizeCandidate(candidate, observedAt, candidate.QueryDimension)
	candidate.CanonicalURL = ""
	if candidate.RawURL != "" {
		if decision := domain.EvaluateSourceURL(candidate.RawURL, policy); decision.Allowed {
			candidate.CanonicalURL = decision.CanonicalURL
		}
	}
	credentials, credentialErr := domain.BuildIdentityCredentials(sourceID, candidate.SourceItemID, candidate.RawURL, policy)
	if credentialErr != nil {
		credentials = nil
	}
	observation := domain.Observation{
		ID: h.newID(), IdempotencyKey: idempotencyKey, SourceID: sourceID,
		Candidate: candidate, ObservedAt: observedAt,
	}
	if observation.ID == "" {
		return domain.Observation{}, app.ErrInvalidRequest
	}
	var persisted domain.Observation
	err := h.repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		if len(credentials) == 0 {
			observation.IdentityStatus = domain.ObservationUnidentified
			observation.ReasonCode = "news.identity.credential_required"
			var appendErr error
			persisted, appendErr = tx.AppendObservation(ctx, observation)
			return appendErr
		}
		existing, err := tx.FindIdentityCredentials(ctx, sourceID, credentials)
		if err != nil {
			return err
		}
		resolution := domain.ResolveIdentity(sourceID, credentials, existing)
		switch resolution.Status {
		case domain.IdentityConflict:
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = resolution.Reason
		case domain.IdentityMatched:
			observation.IdentityStatus = domain.ObservationMatched
			observation.NewsID = resolution.NewsID
		case domain.IdentityNew:
			newsID, err := tx.CreateNewsItem(ctx, sourceID)
			if err != nil {
				return err
			}
			if strings.TrimSpace(newsID) == "" {
				return errors.New("news.identity.created_item_id_missing")
			}
			observation.IdentityStatus = domain.ObservationNew
			observation.NewsID = newsID
			toSave := append([]domain.IdentityCredential(nil), credentials...)
			for index := range toSave {
				toSave[index].NewsID = newsID
				toSave[index].ObservedAt = observedAt
			}
			if err := tx.SaveIdentityCredentials(ctx, toSave); err != nil {
				return err
			}
		default:
			return errors.New("news.identity.resolution_invalid")
		}
		var appendErr error
		persisted, appendErr = tx.AppendObservation(ctx, observation)
		return appendErr
	})
	if ctxErr := ctx.Err(); ctxErr != nil {
		return domain.Observation{}, ctxErr
	}
	if err != nil {
		return domain.Observation{}, errors.Join(app.ErrIdentityTransaction, err)
	}
	return persisted, nil
}

func mustMarshalAuthorizationBinding(t *testing.T, binding domain.NewsAuthorizationBinding) []byte {
	t.Helper()
	payload, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("marshal authorization binding: %v", err)
	}
	return payload
}

func TestNewsMariaDBRepositoryIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(16)
	db.SetMaxIdleConns(8)
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("MariaDB ping failed: %v", err)
	}
	defer db.Close()

	if err := platformstorage.ApplyMigrations(ctx, db, "news", Migrations()); err != nil {
		t.Fatalf("apply News migrations: %v", err)
	}
	// Re-applying the same immutable migration set must be a no-op.
	if err := platformstorage.ApplyMigrations(ctx, db, "news", Migrations()); err != nil {
		t.Fatalf("re-apply News migrations: %v", err)
	}

	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.Ping(ctx); err != nil {
		t.Fatalf("News storage ping: %v", err)
	}
	missingTables, err := repository.MissingSchemaTables(ctx)
	if err != nil {
		t.Fatalf("News schema probe: %v", err)
	}
	if len(missingTables) != 0 {
		t.Fatalf("News schema missing tables: %v", missingTables)
	}

	binding := loadIntegrationAuthorizationBinding(t)
	authorizationObservation, authorizationVersion := integrationAuthorizationEvidence()
	if err := repository.AppendAuthorizationSnapshot(ctx, binding, authorizationObservation, authorizationVersion); err != nil {
		t.Fatalf("append authorization snapshot: %v", err)
	}
	if err := repository.AppendAuthorizationSnapshot(ctx, binding, authorizationObservation, authorizationVersion); err != nil {
		t.Fatalf("idempotent authorization snapshot append: %v", err)
	}
	loadedBinding, err := repository.GetAuthorizationSnapshot(ctx, binding.AuthorizationRef.AuthorizationRef)
	if err != nil {
		t.Fatalf("get authorization snapshot: %v", err)
	}
	if loadedBinding.AuthorizationRef.DecisionFingerprint != binding.AuthorizationRef.DecisionFingerprint ||
		loadedBinding.PolicySnapshot.SnapshotRef != binding.PolicySnapshot.SnapshotRef {
		t.Fatalf("authorization snapshot changed during persistence: %+v", loadedBinding)
	}
	// A later deny is authoritative: latest lookup must not fall back to this allow.
	latestDeny := binding
	latestDeny.AuthorizationRef.AuthorizationRef = fmt.Sprintf("%s-latest-deny-%d", latestDeny.AuthorizationRef.AuthorizationRef, time.Now().UnixNano())
	latestDeny.AuthorizationRef.Decision = domain.AuthorizationDecisionDenied
	latestDenyDecidedAt := time.Now().UTC()
	latestDenyExpiry := latestDenyDecidedAt.Add(time.Hour).Format(time.RFC3339Nano)
	latestDeny.AuthorizationRef.DecidedAt = latestDenyDecidedAt.Format(time.RFC3339Nano)
	latestDeny.AuthorizationRef.ExpiresAt = domain.NullableDateTime{Present: true, Value: &latestDenyExpiry}
	latestDeny.AuthorizationRef.DecisionFingerprint = strings.Repeat("b", 64)
	if err := repository.AppendAuthorizationSnapshot(ctx, latestDeny, authorizationObservation, authorizationVersion); err != nil {
		t.Fatalf("append latest deny authorization snapshot: %v", err)
	}
	latestBinding, err := repository.GetLatestAuthorizationSnapshotForEvidence(ctx,
		binding.AuthorizationRef.SubjectRef, binding.AuthorizationRef.ResourceRef,
		binding.AuthorizationRef.PurposeCode, binding.AuthorizationRef.ContentLevel)
	if err != nil {
		t.Fatalf("get latest authorization snapshot: %v", err)
	}
	if latestBinding.AuthorizationRef.Decision != domain.AuthorizationDecisionDenied ||
		latestBinding.AuthorizationRef.AuthorizationRef != latestDeny.AuthorizationRef.AuthorizationRef {
		t.Fatalf("latest authorization fell back from deny: %+v", latestBinding.AuthorizationRef)
	}
	conflictingBinding := binding
	conflictingBinding.AuthorizationRef.DecisionFingerprint = strings.Repeat("f", 64)
	if err := repository.AppendAuthorizationSnapshot(ctx, conflictingBinding, authorizationObservation, authorizationVersion); !errors.Is(err, app.ErrAuthorizationSnapshotConflict) {
		t.Fatalf("conflicting authorization snapshot append error = %v", err)
	}
	if _, err := repository.GetAuthorizationSnapshot(ctx, "authorization-not-found"); !errors.Is(err, app.ErrAuthorizationSnapshotNotFound) {
		t.Fatalf("missing authorization snapshot error = %v", err)
	}

	var storedAuthorizationHash string
	if err := db.QueryRowContext(ctx,
		"SELECT payload_hash FROM news_authorization_snapshots WHERE authorization_ref = ?",
		binding.AuthorizationRef.AuthorizationRef,
	).Scan(&storedAuthorizationHash); err != nil {
		t.Fatalf("read authorization payload hash: %v", err)
	}
	tamperedAuthorizationHash := strings.Repeat("0", 64)
	if storedAuthorizationHash == tamperedAuthorizationHash {
		tamperedAuthorizationHash = strings.Repeat("1", 64)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_authorization_snapshots SET payload_hash = ? WHERE authorization_ref = ?",
		tamperedAuthorizationHash, binding.AuthorizationRef.AuthorizationRef,
	); err != nil {
		t.Fatalf("tamper authorization payload hash: %v", err)
	}
	_, authorizationTamperErr := repository.GetAuthorizationSnapshot(ctx, binding.AuthorizationRef.AuthorizationRef)
	if _, err := db.ExecContext(ctx,
		"UPDATE news_authorization_snapshots SET payload_hash = ? WHERE authorization_ref = ?",
		storedAuthorizationHash, binding.AuthorizationRef.AuthorizationRef,
	); err != nil {
		t.Fatalf("restore authorization payload hash: %v", err)
	}
	if !errors.Is(authorizationTamperErr, app.ErrInvalidAuthorizationSnapshot) {
		t.Fatalf("tampered authorization snapshot error = %v, want invalid snapshot", authorizationTamperErr)
	}
	// Lookup columns are an immutable envelope and must match the verified payload.
	if _, err := db.ExecContext(ctx,
		"UPDATE news_authorization_snapshots SET decision = ? WHERE authorization_ref = ?",
		string(domain.AuthorizationDecisionDenied), binding.AuthorizationRef.AuthorizationRef,
	); err != nil {
		t.Fatalf("tamper authorization envelope: %v", err)
	}
	_, authorizationMetadataErr := repository.GetAuthorizationSnapshot(ctx, binding.AuthorizationRef.AuthorizationRef)
	if _, err := db.ExecContext(ctx,
		"UPDATE news_authorization_snapshots SET decision = ? WHERE authorization_ref = ?",
		string(binding.AuthorizationRef.Decision), binding.AuthorizationRef.AuthorizationRef,
	); err != nil {
		t.Fatalf("restore authorization envelope: %v", err)
	}
	if !errors.Is(authorizationMetadataErr, app.ErrInvalidAuthorizationSnapshot) {
		t.Fatalf("tampered authorization envelope error = %v, want invalid snapshot", authorizationMetadataErr)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_authorization_snapshots SET payload_json = JSON_OBJECT('oversized', REPEAT('x', ?)) WHERE authorization_ref = ?",
		maximumAuthorizationSnapshotBytes, binding.AuthorizationRef.AuthorizationRef,
	); err != nil {
		t.Fatalf("write oversized authorization payload: %v", err)
	}
	_, authorizationOversizeErr := repository.GetAuthorizationSnapshot(ctx, binding.AuthorizationRef.AuthorizationRef)
	if _, err := db.ExecContext(ctx,
		"UPDATE news_authorization_snapshots SET payload_json = ? WHERE authorization_ref = ?",
		mustMarshalAuthorizationBinding(t, binding), binding.AuthorizationRef.AuthorizationRef,
	); err != nil {
		t.Fatalf("restore authorization payload: %v", err)
	}
	if !errors.Is(authorizationOversizeErr, app.ErrInvalidAuthorizationSnapshot) {
		t.Fatalf("oversized authorization payload error = %v, want invalid snapshot", authorizationOversizeErr)
	}

	snapshotCreatedAt := time.Now().UTC()
	snapshot := app.EvidenceSnapshot{
		SnapshotRef:           fmt.Sprintf("news-snapshot-it-%d", time.Now().UnixNano()),
		CallerRef:             "service:researchflow",
		PurposeCode:           app.EvidencePurposeResearchAnalysis,
		RequestedContentLevel: app.EvidenceContentSummary,
		FiltersFingerprint:    strings.Repeat("a", 64),
		PageSize:              20,
		SystemAsOf:            snapshotCreatedAt.Add(-time.Minute),
		SortVersion:           "news-evidence-v1",
		CreatedAt:             snapshotCreatedAt,
		ExpiresAt:             snapshotCreatedAt.Add(app.EvidenceSnapshotTTL),
		Items: []app.EvidenceSnapshotItem{{
			NewsID: "news-it-item-1", VersionID: "news-it-version-1",
			ObservationID:    "news-it-observation-1",
			SystemObservedAt: snapshotCreatedAt.Add(-2 * time.Minute),
		}},
	}
	if err := repository.CreateEvidenceSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("create evidence snapshot: %v", err)
	}
	if err := repository.CreateEvidenceSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("idempotent evidence snapshot create: %v", err)
	}

	// Concurrent retries with the same immutable reference and payload must
	// converge to the already persisted row instead of surfacing duplicate-key errors.
	const snapshotRetryWorkers = 8
	snapshotRetryErrors := make(chan error, snapshotRetryWorkers)
	var snapshotRetryWait sync.WaitGroup
	for index := 0; index < snapshotRetryWorkers; index++ {
		snapshotRetryWait.Add(1)
		go func() {
			defer snapshotRetryWait.Done()
			snapshotRetryErrors <- repository.CreateEvidenceSnapshot(ctx, snapshot)
		}()
	}
	snapshotRetryWait.Wait()
	close(snapshotRetryErrors)
	for retryErr := range snapshotRetryErrors {
		if retryErr != nil {
			t.Fatalf("concurrent idempotent evidence snapshot create: %v", retryErr)
		}
	}
	loadedSnapshot, err := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, snapshot.CallerRef, snapshotCreatedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("get evidence snapshot: %v", err)
	}
	if len(loadedSnapshot.Items) != 1 ||
		loadedSnapshot.Items[0].VersionID != snapshot.Items[0].VersionID ||
		loadedSnapshot.PurposeCode != snapshot.PurposeCode ||
		loadedSnapshot.RequestedContentLevel != snapshot.RequestedContentLevel {
		t.Fatalf("evidence snapshot payload changed: %+v", loadedSnapshot)
	}

	var storedEvidenceHash string
	if err := db.QueryRowContext(ctx,
		"SELECT payload_hash FROM news_evidence_snapshots WHERE snapshot_ref = ?",
		snapshot.SnapshotRef,
	).Scan(&storedEvidenceHash); err != nil {
		t.Fatalf("read evidence payload hash: %v", err)
	}
	tamperedEvidenceHash := strings.Repeat("0", 64)
	if storedEvidenceHash == tamperedEvidenceHash {
		tamperedEvidenceHash = strings.Repeat("1", 64)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_hash = ? WHERE snapshot_ref = ?",
		tamperedEvidenceHash, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("tamper evidence snapshot payload hash: %v", err)
	}
	_, evidenceTamperErr := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, snapshot.CallerRef, snapshotCreatedAt.Add(time.Minute))
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_hash = ? WHERE snapshot_ref = ?",
		storedEvidenceHash, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("restore evidence snapshot payload hash: %v", err)
	}
	if !errors.Is(evidenceTamperErr, app.ErrInvalidEvidenceSnapshot) {
		t.Fatalf("tampered evidence snapshot error = %v, want invalid snapshot", evidenceTamperErr)
	}
	// The denormalized lookup columns are part of the immutable snapshot
	// envelope and must agree with the hash-verified JSON payload.
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET page_size = page_size + 1 WHERE snapshot_ref = ?",
		snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("tamper evidence snapshot metadata: %v", err)
	}
	_, evidenceMetadataTamperErr := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, snapshot.CallerRef, snapshotCreatedAt.Add(time.Minute))
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET page_size = ? WHERE snapshot_ref = ?",
		snapshot.PageSize, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("restore evidence snapshot metadata: %v", err)
	}
	if !errors.Is(evidenceMetadataTamperErr, app.ErrInvalidEvidenceSnapshot) {
		t.Fatalf("tampered evidence snapshot metadata error = %v, want invalid snapshot", evidenceMetadataTamperErr)
	}
	// Idempotent replay must reject a matching payload hash with a corrupted
	// denormalized envelope instead of reporting a successful create.
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET caller_ref = ? WHERE snapshot_ref = ?",
		"service:tampered", snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("tamper evidence snapshot caller envelope: %v", err)
	}
	if err := repository.CreateEvidenceSnapshot(ctx, snapshot); !errors.Is(err, app.ErrInvalidEvidenceSnapshot) {
		t.Fatalf("idempotent create with tampered envelope error = %v, want invalid snapshot", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET caller_ref = ? WHERE snapshot_ref = ?",
		snapshot.CallerRef, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("restore evidence snapshot caller envelope: %v", err)
	}
	if err := repository.CreateEvidenceSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("idempotent create after envelope restoration: %v", err)
	}
	var originalEvidencePayload []byte
	if err := db.QueryRowContext(ctx,
		"SELECT payload_json FROM news_evidence_snapshots WHERE snapshot_ref = ?",
		snapshot.SnapshotRef,
	).Scan(&originalEvidencePayload); err != nil {
		t.Fatalf("read original evidence payload: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_json = JSON_OBJECT('tampered', 'payload') WHERE snapshot_ref = ?",
		snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("tamper evidence snapshot payload: %v", err)
	}
	if err := repository.CreateEvidenceSnapshot(ctx, snapshot); !errors.Is(err, app.ErrInvalidEvidenceSnapshot) {
		t.Fatalf("idempotent create with tampered payload error = %v, want invalid snapshot", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_json = ? WHERE snapshot_ref = ?",
		originalEvidencePayload, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("restore evidence snapshot payload: %v", err)
	}
	if err := repository.CreateEvidenceSnapshot(ctx, snapshot); err != nil {
		t.Fatalf("idempotent create after payload restoration: %v", err)
	}
	// Read must independently verify the payload bytes and enforce the same
	// size ceiling as Create/replay before hashing or JSON decoding.
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_json = JSON_OBJECT('tampered', 'payload') WHERE snapshot_ref = ?",
		snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("tamper evidence payload before read: %v", err)
	}
	if _, err := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, snapshot.CallerRef, snapshotCreatedAt.Add(time.Minute)); !errors.Is(err, app.ErrInvalidEvidenceSnapshot) {
		t.Fatalf("get tampered evidence payload error = %v, want invalid snapshot", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_json = ? WHERE snapshot_ref = ?",
		originalEvidencePayload, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("restore evidence payload after read tamper: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_json = JSON_OBJECT('padding', REPEAT('x', ?)) WHERE snapshot_ref = ?",
		maximumEvidenceSnapshotBytes+1, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("oversize evidence payload before read: %v", err)
	}
	if _, err := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, snapshot.CallerRef, snapshotCreatedAt.Add(time.Minute)); !errors.Is(err, app.ErrInvalidEvidenceSnapshot) {
		t.Fatalf("get oversized evidence payload error = %v, want invalid snapshot", err)
	}
	if _, err := db.ExecContext(ctx,
		"UPDATE news_evidence_snapshots SET payload_json = ? WHERE snapshot_ref = ?",
		originalEvidencePayload, snapshot.SnapshotRef,
	); err != nil {
		t.Fatalf("restore evidence payload after size test: %v", err)
	}
	conflictingSnapshot := snapshot
	conflictingSnapshot.PageSize++
	if err := repository.CreateEvidenceSnapshot(ctx, conflictingSnapshot); !errors.Is(err, app.ErrEvidenceSnapshotConflict) {
		t.Fatalf("conflicting evidence snapshot create error = %v", err)
	}
	if _, err := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, "different-caller", snapshotCreatedAt.Add(time.Minute)); !errors.Is(err, app.ErrEvidenceSnapshotUnavailable) {
		t.Fatalf("snapshot caller mismatch error = %v", err)
	}
	if _, err := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, snapshot.CallerRef, snapshot.ExpiresAt); !errors.Is(err, app.ErrEvidenceSnapshotUnavailable) {
		t.Fatalf("expired evidence snapshot error = %v", err)
	}
	deleted, err := repository.DeleteExpiredEvidenceSnapshots(ctx, snapshot.ExpiresAt, 100)
	if err != nil || deleted < 1 {
		t.Fatalf("delete expired evidence snapshots: deleted=%d err=%v", deleted, err)
	}
	var remainingSnapshotRows int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM news_evidence_snapshots WHERE snapshot_ref = ?",
		snapshot.SnapshotRef,
	).Scan(&remainingSnapshotRows); err != nil {
		t.Fatalf("count deleted evidence snapshot: %v", err)
	}
	if remainingSnapshotRows != 0 {
		t.Fatalf("expired evidence snapshot rows remaining=%d, want 0", remainingSnapshotRows)
	}
	// 清理完成后，读取端必须将已删除的 Snapshot 视为不可用，
	// 不得因为 payload 曾经存在而返回缓存或残留内容。
	if _, err := repository.GetEvidenceSnapshot(ctx, snapshot.SnapshotRef, snapshot.CallerRef, snapshot.ExpiresAt); !errors.Is(err, app.ErrEvidenceSnapshotUnavailable) {
		t.Fatalf("get deleted evidence snapshot error = %v, want unavailable", err)
	}
	repeatedDeleted, err := repository.DeleteExpiredEvidenceSnapshots(ctx, snapshot.ExpiresAt, 100)
	if err != nil || repeatedDeleted != 0 {
		t.Fatalf("repeat expired snapshot cleanup: deleted=%d err=%v, want 0 and nil", repeatedDeleted, err)
	}

	// 并发清理 Worker 使用不同请求处理同一批到期 Snapshot；总删除数必须
	// 等于到期记录数，且每个 Worker 不得超过自己的批次上限。
	concurrentSnapshotCreatedAt := time.Now().UTC().Add(-time.Second)
	concurrentSnapshots := make([]app.EvidenceSnapshot, 4)
	for index := range concurrentSnapshots {
		concurrentSnapshot := snapshot
		concurrentSnapshot.SnapshotRef = fmt.Sprintf("news-snapshot-cleanup-concurrent-%d-%d", time.Now().UnixNano(), index)
		concurrentSnapshot.CreatedAt = concurrentSnapshotCreatedAt
		concurrentSnapshot.SystemAsOf = concurrentSnapshotCreatedAt.Add(-time.Minute)
		concurrentSnapshot.ExpiresAt = concurrentSnapshotCreatedAt.Add(app.EvidenceSnapshotTTL)
		concurrentSnapshot.Items = append([]app.EvidenceSnapshotItem(nil), snapshot.Items...)
		concurrentSnapshot.Items[0].SystemObservedAt = concurrentSnapshotCreatedAt.Add(-2 * time.Minute)
		concurrentSnapshots[index] = concurrentSnapshot
		if err := repository.CreateEvidenceSnapshot(ctx, concurrentSnapshot); err != nil {
			t.Fatalf("create concurrent cleanup snapshot %d: %v", index, err)
		}
	}
	concurrentCleanupAt := concurrentSnapshots[0].ExpiresAt
	type snapshotCleanupOutcome struct {
		deleted int64
		err     error
	}
	const snapshotCleanupWorkers = 2
	snapshotCleanupOutcomes := make(chan snapshotCleanupOutcome, snapshotCleanupWorkers)
	var snapshotCleanupWait sync.WaitGroup
	for worker := 0; worker < snapshotCleanupWorkers; worker++ {
		snapshotCleanupWait.Add(1)
		go func() {
			defer snapshotCleanupWait.Done()
			deleted, cleanupErr := repository.DeleteExpiredEvidenceSnapshots(ctx, concurrentCleanupAt, 2)
			snapshotCleanupOutcomes <- snapshotCleanupOutcome{deleted: deleted, err: cleanupErr}
		}()
	}
	snapshotCleanupWait.Wait()
	close(snapshotCleanupOutcomes)
	var concurrentDeleted int64
	for outcome := range snapshotCleanupOutcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent evidence snapshot cleanup failed: %v", outcome.err)
		}
		if outcome.deleted < 0 || outcome.deleted > 2 {
			t.Fatalf("concurrent evidence snapshot cleanup deleted=%d, want 0..2", outcome.deleted)
		}
		concurrentDeleted += outcome.deleted
	}
	if concurrentDeleted != int64(len(concurrentSnapshots)) {
		t.Fatalf("concurrent evidence snapshot cleanup total=%d, want %d", concurrentDeleted, len(concurrentSnapshots))
	}
	for _, concurrentSnapshot := range concurrentSnapshots {
		var remaining int
		if err := db.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM news_evidence_snapshots WHERE snapshot_ref = ?",
			concurrentSnapshot.SnapshotRef,
		).Scan(&remaining); err != nil {
			t.Fatalf("count concurrent cleanup snapshot %s: %v", concurrentSnapshot.SnapshotRef, err)
		}
		if remaining != 0 {
			t.Fatalf("concurrent cleanup snapshot %s remains: %d rows", concurrentSnapshot.SnapshotRef, remaining)
		}
	}
	repeatedConcurrentCleanup, err := repository.DeleteExpiredEvidenceSnapshots(ctx, concurrentCleanupAt, 2)
	if err != nil || repeatedConcurrentCleanup != 0 {
		t.Fatalf("repeat concurrent snapshot cleanup deleted=%d err=%v, want 0 and nil", repeatedConcurrentCleanup, err)
	}
	var idSequence atomic.Uint64
	newID := func() string { return fmt.Sprintf("news-it-%d", idSequence.Add(1)) }
	ingest := repositoryIngestHarness{repository: repository, newID: newID}
	sourceID := fmt.Sprintf("news-it-source-%d", time.Now().UnixNano())
	const workers = 6

	candidate := domain.Candidate{
		SourceItemID:   "native-item-001",
		Title:          "News integration title",
		PublisherName:  "Integration Publisher",
		PublishedAtRaw: "2026-11-06T01:02:03Z",
	}
	observations := make([]domain.Observation, workers)
	errs := make(chan error, workers)
	var wait sync.WaitGroup
	for index := 0; index < workers; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			observation, err := ingest.Execute(ctx, sourceID, fmt.Sprintf("news-it-key-%d", index), candidate, domain.SourceURLPolicy{})
			if err != nil {
				errs <- err
				return
			}
			observations[index] = observation
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		t.Fatalf("concurrent repository identity transaction failed: %v", err)
	}

	newsID := observations[0].NewsID
	if newsID == "" {
		t.Fatal("identity repository transaction did not create a NewsID")
	}
	for index, observation := range observations {
		if observation.NewsID != newsID {
			t.Fatalf("observation %d NewsID = %q, want %q", index, observation.NewsID, newsID)
		}
	}
	var itemCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM news_items WHERE origin_source_id = ?", sourceID,
	).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 1 {
		t.Fatalf("concurrent repository transactions created %d News items, want 1", itemCount)
	}

	identityCandidates, err := domain.BuildIdentityCredentials(sourceID, "native-item-001", "", domain.SourceURLPolicy{})
	if err != nil {
		t.Fatalf("build identity history query: %v", err)
	}
	identityBefore := observations[0].ObservedAt.Add(-time.Hour)
	credentialsBefore, err := repository.ListIdentityCredentialsAt(ctx, sourceID, identityCandidates, identityBefore)
	if err != nil {
		t.Fatalf("query identity before creation: %v", err)
	}
	if len(credentialsBefore) != 0 {
		t.Fatalf("identity credential visible before creation: %+v", credentialsBefore)
	}
	identityAsOf := time.Now().UTC().Add(time.Second)
	credentialsAt, err := repository.ListIdentityCredentialsAt(ctx, sourceID, identityCandidates, identityAsOf)
	if err != nil {
		t.Fatalf("query identity at system_as_of: %v", err)
	}
	if len(credentialsAt) != 1 || credentialsAt[0].NewsID != newsID ||
		credentialsAt[0].Status != domain.IdentifierActive ||
		credentialsAt[0].ObservedAt.After(identityAsOf) {
		t.Fatalf("unexpected identity PIT result: %+v", credentialsAt)
	}

	traceSourceID := sourceID + "-provider-trace"
	traceCandidate := domain.Candidate{SourceItemID: "provider-trace-item", ProviderCode: "tavily", Title: "trace test"}
	if _, err := ingest.Execute(ctx, traceSourceID, "provider-trace-key", traceCandidate, domain.SourceURLPolicy{}); !errors.Is(err, app.ErrProviderOutputPolicyRequired) {
		t.Fatalf("self-declared provider policy error = %v, want fail-closed rejection", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM news_items WHERE origin_source_id = ?", traceSourceID,
	).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 0 {
		t.Fatalf("provider policy rejection created %d News items, want 0", itemCount)
	}

	replayed, err := ingest.Execute(ctx, sourceID, "news-it-key-0", candidate, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatalf("same-payload idempotency replay failed: %v", err)
	}
	if replayed.ID != observations[0].ID || replayed.NewsID != newsID {
		t.Fatalf("idempotency replay returned a different Observation: %#v", replayed)
	}

	changed := candidate
	changed.Title = "changed payload must conflict"
	if _, err := ingest.Execute(ctx, sourceID, "news-it-key-0", changed, domain.SourceURLPolicy{}); !errors.Is(err, app.ErrObservationIdempotencyConflict) {
		t.Fatalf("same key with different payload error = %v, want idempotency conflict", err)
	}

	rollbackSource := sourceID + "-rollback"
	rollbackErr := errors.New("force transaction rollback")
	err = repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		if _, err := tx.CreateNewsItem(ctx, rollbackSource); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback transaction error = %v, want injected error", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM news_items WHERE origin_source_id = ?", rollbackSource,
	).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 0 {
		t.Fatalf("rolled-back News Item count = %d, want 0", itemCount)
	}

	updater := app.NewApplyResearchUpdate(repository, newID)
	var updateWait sync.WaitGroup
	updateErrors := make(chan error, workers)
	for index := 0; index < workers; index++ {
		index := index
		updateWait.Add(1)
		go func() {
			defer updateWait.Done()
			result, err := updater.Execute(ctx, app.ResearchUpdateRequest{
				NewsID:        newsID,
				Candidate:     candidate,
				ObservationID: observations[index].ID,
				Content: domain.ContentLayers{
					SourceSummary:       fmt.Sprintf("summary revision %d", index),
					SourceSummaryOrigin: domain.ContentOriginFeedSummary,
				},
			})
			if err != nil {
				updateErrors <- err
				return
			}
			if !result.VersionCreated {
				updateErrors <- fmt.Errorf("concurrent revision %d did not create a version", index)
			}
		}()
	}
	updateWait.Wait()
	close(updateErrors)
	for err := range updateErrors {
		t.Fatalf("concurrent version append failed: %v", err)
	}

	var latest domain.NewsItemVersion
	err = repository.WithinResearchUpdateTransaction(ctx, func(tx app.ResearchUpdateTransaction) error {
		version, exists, err := tx.LatestNewsItemVersion(ctx, newsID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("latest version missing")
		}
		latest = version
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if latest.VersionNo != workers {
		t.Fatalf("latest VersionNo = %d, want %d", latest.VersionNo, workers)
	}

	secondSourceID := sourceID + "-second"
	secondCandidate := domain.Candidate{
		SourceItemID: "native-item-002",
		Title:        "Second news item", PublisherName: "Second Publisher",
	}
	secondObservation, err := ingest.Execute(ctx, secondSourceID, "news-it-second-key", secondCandidate, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatalf("create second news item: %v", err)
	}
	secondVersion, err := updater.Execute(ctx, app.ResearchUpdateRequest{
		NewsID: secondObservation.NewsID, Candidate: secondCandidate,
		ObservationID: secondObservation.ID,
		Content: domain.ContentLayers{
			SourceSummary: "second source summary", SourceSummaryOrigin: domain.ContentOriginFeedSummary,
		},
	})
	if err != nil {
		t.Fatalf("create second news version: %v", err)
	}

	// Both the version and its owning Observation must be visible at system_as_of.
	beforeVersion := app.GetEvidenceRequest{
		NewsID:     secondObservation.NewsID,
		SystemAsOf: secondVersion.Version.ObservedAt.Add(-time.Second),
	}
	if _, err := repository.GetEvidenceVersionAt(ctx, beforeVersion); !errors.Is(err, app.ErrEvidencePITNotFound) {
		t.Fatalf("version before system_as_of error = %v, want PIT not found", err)
	}
	atVersion := app.GetEvidenceRequest{
		NewsID:     secondObservation.NewsID,
		VersionID:  secondVersion.Version.ID,
		SystemAsOf: secondVersion.Version.ObservedAt.Add(time.Second),
	}
	pitRecord, err := repository.GetEvidenceVersionAt(ctx, atVersion)
	if err != nil {
		t.Fatalf("get evidence at system_as_of: %v", err)
	}
	if pitRecord.Version.ID != secondVersion.Version.ID ||
		pitRecord.Observation.ID != secondObservation.ID ||
		pitRecord.SourceRef != secondSourceID ||
		pitRecord.ProviderRef != "" {
		t.Fatalf("PIT record mismatch: %+v", pitRecord)
	}
	atVersion.SystemAsOf = secondVersion.Version.ObservedAt.Add(-time.Second)
	if _, err := repository.GetEvidenceVersionAt(ctx, atVersion); !errors.Is(err, app.ErrEvidencePITNotFound) {
		t.Fatalf("explicit future version error = %v, want PIT not found", err)
	}

	listRequest := app.ListEvidenceRequest{
		CallerRef:             "service:research",
		PurposeCode:           app.EvidencePurposeResearchAnalysis,
		RequestedContentLevel: app.EvidenceContentSummary,
		SystemAsOf:            time.Now().UTC(),
		Filters: &app.EvidenceQueryFilters{
			NewsIDs:    []string{secondObservation.NewsID},
			SourceRefs: []string{secondSourceID},
		},
		PageSize: 20,
	}
	// 写入可追踪的历史策略引用，验证 PIT 查询完整回读 V6 字段。
	policyEvaluatedAt := time.Now().UTC()
	if _, err := db.ExecContext(ctx, `UPDATE news_observations
		SET endpoint_id = ?, provider_output_policy_ref = ?,
		    provider_output_policy_version_id = ?, policy_evaluated_at = ?
		WHERE observation_id = ?`,
		"endpoint-pit-v1", "provider-policy-pit", "provider-policy-v7",
		policyEvaluatedAt, secondObservation.ID,
	); err != nil {
		t.Fatalf("set PIT policy trace fixture: %v", err)
	}

	listed, err := repository.ListEvidenceVersionsAt(ctx, listRequest)
	if err != nil {
		t.Fatalf("list evidence at system_as_of: %v", err)
	}
	if len(listed) != 1 || listed[0].Version.ID != secondVersion.Version.ID ||
		listed[0].Observation.ID != secondObservation.ID || listed[0].SourceRef != secondSourceID {
		t.Fatalf("ListEvidence PIT returned unexpected records: %+v", listed)
	}
	gotObservation := listed[0].Observation
	if gotObservation.EndpointID != "endpoint-pit-v1" ||
		gotObservation.ProviderOutputPolicyRef != "provider-policy-pit" ||
		gotObservation.ProviderOutputPolicyVersionID != "provider-policy-v7" ||
		!gotObservation.PolicyEvaluatedAt.Equal(policyEvaluatedAt) {
		t.Fatalf("ListEvidence PIT lost policy trace: %+v", gotObservation)
	}
	listRequest.SystemAsOf = secondVersion.Version.ObservedAt.Add(-time.Second)
	listed, err = repository.ListEvidenceVersionsAt(ctx, listRequest)
	if err != nil {
		t.Fatalf("list evidence before system_as_of: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListEvidence PIT exposed %d future records", len(listed))
	}
	listRequest.SystemAsOf = time.Now().UTC()
	listRequest.Filters.CanonicalSymbols = []string{"SSE:600000"}
	if _, err := repository.ListEvidenceVersionsAt(ctx, listRequest); !errors.Is(err, app.ErrInvalidEvidenceQuery) {
		t.Fatalf("unsupported temporal symbol filter error = %v, want invalid query", err)
	}

	// A later source revision must not rewrite the version visible at an earlier
	// knowledge time; the latest version changes only after its observation.
	time.Sleep(10 * time.Millisecond)
	revisedCandidate := secondCandidate
	revisedCandidate.Title = "Second news item revised"
	revisedObservation, err := ingest.Execute(
		ctx, secondSourceID, "news-it-second-revision-key", revisedCandidate, domain.SourceURLPolicy{},
	)
	if err != nil {
		t.Fatalf("ingest second news revision: %v", err)
	}
	if revisedObservation.NewsID != secondObservation.NewsID {
		t.Fatalf("revision NewsID = %q, want %q", revisedObservation.NewsID, secondObservation.NewsID)
	}
	revisedVersion, err := updater.Execute(ctx, app.ResearchUpdateRequest{
		NewsID: revisedObservation.NewsID, Candidate: revisedCandidate,
		ObservationID: revisedObservation.ID,
		Content: domain.ContentLayers{
			SourceSummary: "revised source summary", SourceSummaryOrigin: domain.ContentOriginFeedSummary,
		},
	})
	if err != nil {
		t.Fatalf("append second news revision: %v", err)
	}
	listRequest.Filters.CanonicalSymbols = nil
	listRequest.SystemAsOf = secondVersion.Version.ObservedAt
	listed, err = repository.ListEvidenceVersionsAt(ctx, listRequest)
	if err != nil {
		t.Fatalf("list evidence before later revision: %v", err)
	}
	if len(listed) != 1 || listed[0].Version.ID != secondVersion.Version.ID {
		t.Fatalf("historical PIT version = %+v, want original version %q", listed, secondVersion.Version.ID)
	}
	listRequest.SystemAsOf = time.Now().UTC()
	listed, err = repository.ListEvidenceVersionsAt(ctx, listRequest)
	if err != nil {
		t.Fatalf("list evidence after later revision: %v", err)
	}
	if len(listed) != 1 || listed[0].Version.ID != revisedVersion.Version.ID {
		t.Fatalf("latest PIT version = %+v, want revised version %q", listed, revisedVersion.Version.ID)
	}

	relationService := app.NewApplyNewsItemRelation(repository, newID)
	relation, err := relationService.Execute(ctx, app.NewsItemRelationRequest{
		Relation: domain.NewsItemRelation{
			FromNewsID: newsID, FromSourceID: sourceID,
			ToNewsID: secondObservation.NewsID, ToSourceID: secondSourceID,
			Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationConfirmed,
			EvidenceObservation: observations[0].ID,
			FromVersionID:       latest.ID, ToVersionID: revisedVersion.Version.ID,
		},
	})
	if err != nil {
		t.Fatalf("append cross-news relation: %v", err)
	}
	if relation.ID == "" {
		t.Fatal("persisted relation ID is empty")
	}

	err = repository.WithinNewsItemRelationTransaction(ctx, func(tx app.NewsItemRelationTransaction) error {
		relations, err := tx.ListAllNewsItemRelations(ctx)
		if err != nil {
			return err
		}
		if len(relations) != 1 || relations[0].ID != relation.ID {
			return fmt.Errorf("persisted relation history = %#v", relations)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Knowledge-time reads retain old assertions but hide assertions not yet observed.
	beforeRelation := relation.ObservedAt.Add(-time.Microsecond)
	history, err := repository.ListNewsItemRelationsAt(ctx, app.NewsItemRelationHistoryRequest{
		NewsID: newsID, SystemAsOf: beforeRelation,
	})
	if err != nil {
		t.Fatalf("read relation history before assertion: %v", err)
	}
	if len(history) != 0 {
		t.Fatalf("relation history before assertion = %#v, want empty", history)
	}
	afterRelation := relation.ObservedAt.Add(time.Microsecond)
	history, err = repository.ListNewsItemRelationsAt(ctx, app.NewsItemRelationHistoryRequest{
		NewsID: newsID, SystemAsOf: afterRelation,
	})
	if err != nil {
		t.Fatalf("read relation history from source item: %v", err)
	}
	if len(history) != 1 || history[0].ID != relation.ID {
		t.Fatalf("source-side relation history = %#v, want relation %q", history, relation.ID)
	}
	history, err = repository.ListNewsItemRelationsAt(ctx, app.NewsItemRelationHistoryRequest{
		NewsID: secondObservation.NewsID, SystemAsOf: afterRelation,
	})
	if err != nil {
		t.Fatalf("read relation history from target item: %v", err)
	}
	if len(history) != 1 || history[0].ID != relation.ID {
		t.Fatalf("target-side relation history = %#v, want relation %q", history, relation.ID)
	}

	// A replacement may change the target item. Both old and new target queries
	// must retain the complete supersedes chain.
	var changedTargetNewsID string
	if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		var err error
		changedTargetNewsID, err = tx.CreateNewsItem(ctx, "news-it-relation-new-source")
		return err
	}); err != nil {
		t.Fatalf("create changed relation target item: %v", err)
	}
	changedTarget, err := relationService.Execute(ctx, app.NewsItemRelationRequest{
		Relation: domain.NewsItemRelation{
			FromNewsID: newsID, FromSourceID: sourceID,
			ToNewsID: changedTargetNewsID, ToSourceID: "news-it-relation-new-source",
			Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationConfirmed,
			SupersedesRelationID: relation.ID, EvidenceObservation: observations[0].ID,
		},
	})
	if err != nil {
		t.Fatalf("append changed-target relation replacement: %v", err)
	}
	for _, targetID := range []string{relation.ToNewsID, changedTarget.ToNewsID} {
		chain, err := repository.ListNewsItemRelationsAt(ctx, app.NewsItemRelationHistoryRequest{
			NewsID: targetID, SystemAsOf: time.Now().UTC().Add(time.Second),
		})
		if err != nil {
			t.Fatalf("read target-side relation chain for %s: %v", targetID, err)
		}
		if len(chain) != 2 || chain[0].ID != relation.ID || chain[1].ID != changedTarget.ID {
			t.Fatalf("target-side relation chain for %s = %#v, want original and replacement", targetID, chain)
		}
	}

	proposal, err := relationService.Execute(ctx, app.NewsItemRelationRequest{
		Relation: domain.NewsItemRelation{
			FromNewsID: newsID, FromSourceID: sourceID,
			ToNewsID: secondObservation.NewsID, ToSourceID: secondSourceID,
			Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationProposed,
			SupersedesRelationID: changedTarget.ID,
			EvidenceObservation:  observations[0].ID,
			FromVersionID:        latest.ID, ToVersionID: revisedVersion.Version.ID,
		},
	})
	if err != nil {
		t.Fatalf("append proposed relation replacement: %v", err)
	}
	afterProposal := proposal.ObservedAt.Add(time.Microsecond)
	history, err = repository.ListNewsItemRelationsAt(ctx, app.NewsItemRelationHistoryRequest{
		NewsID: secondObservation.NewsID, SystemAsOf: afterProposal,
	})
	if err != nil {
		t.Fatalf("read relation history after proposal: %v", err)
	}
	if len(history) != 3 || history[0].ID != relation.ID || history[1].ID != changedTarget.ID || history[2].ID != proposal.ID {
		t.Fatalf("history after proposal = %#v, want original then proposal", history)
	}
	// 提议状态不改变当前关系图；原确认断言仍保留在历史中。

	retraction, err := relationService.Execute(ctx, app.NewsItemRelationRequest{
		Relation: domain.NewsItemRelation{
			FromNewsID: newsID, FromSourceID: sourceID,
			ToNewsID: secondObservation.NewsID, ToSourceID: secondSourceID,
			Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationRetracted,
			SupersedesRelationID: proposal.ID,
			EvidenceObservation:  observations[0].ID,
			FromVersionID:        latest.ID, ToVersionID: revisedVersion.Version.ID,
		},
	})
	if err != nil {
		t.Fatalf("append proposal retraction: %v", err)
	}
	afterRetraction := retraction.ObservedAt.Add(time.Microsecond)
	history, err = repository.ListNewsItemRelationsAt(ctx, app.NewsItemRelationHistoryRequest{
		NewsID: secondObservation.NewsID, SystemAsOf: afterRetraction,
	})
	if err != nil {
		t.Fatalf("read relation history after retraction: %v", err)
	}
	if len(history) != 4 ||
		history[0].ID != relation.ID ||
		history[1].ID != changedTarget.ID ||
		history[2].ID != proposal.ID ||
		history[3].ID != retraction.ID {
		t.Fatalf("history after retraction = %#v, want complete append-only chain", history)
	}
}

func loadIntegrationAuthorizationBinding(t *testing.T) domain.NewsAuthorizationBinding {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "doc", "整体任务规划", "golden", "NEWS-CTR-002-authorization-binding.golden.json")
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		AuthorizationBinding domain.NewsAuthorizationBinding `json:"authorization_binding"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	if err := domain.ValidateNewsAuthorizationBinding(fixture.AuthorizationBinding); err != nil {
		t.Fatalf("invalid NEWS-CTR-002 integration golden: %v", err)
	}
	return fixture.AuthorizationBinding
}

func integrationAuthorizationEvidence() (domain.Observation, domain.NewsItemVersion) {
	observation := domain.Observation{
		ID: "news-it-auth-observation", NewsID: "news_001", SourceID: "sec-edgar",
		ProviderCode: "tavily", SourcePolicyVersionID: "2.1.0",
		EndpointID: "sec-edgar-filings", EndpointPolicyVersionID: "1.0.0",
		ProviderOutputPolicyRef: "tavily-output", ProviderOutputPolicyVersionID: "3.0.0",
	}
	version := domain.NewsItemVersion{
		ID: "news-it-auth-version", NewsID: "news_001", VersionNo: 1,
		ObservationID: observation.ID,
	}
	return observation, version
}

// TestObservationRetentionPurgeIntegration covers MariaDB V8 retention writes,
// idempotent purge, and the no-resurrection rule for an expired payload.
func TestObservationRetentionPurgeIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News MariaDB integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(8)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("MariaDB ping failed: %v", err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "news", Migrations()); err != nil {
		t.Fatalf("apply News migrations: %v", err)
	}
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	observations := make([]domain.Observation, 3)
	for i := range observations {
		observations[i] = domain.Observation{
			ID:             fmt.Sprintf("news-retention-it-%d-%d", time.Now().UnixNano(), i),
			IdempotencyKey: fmt.Sprintf("news-retention-key-%d-%d", time.Now().UnixNano(), i),
			SourceID:       "news-retention-integration",
			Candidate:      domain.Candidate{SourceItemID: fmt.Sprintf("retention-source-item-%d", i), Title: "must be purged"},
			ObservedAt:     now.Add(-2 * time.Hour), RetentionUntil: now.Add(-time.Hour),
			IdentityStatus: domain.ObservationUnidentified,
		}
	}
	referenceNewsID := fmt.Sprintf("news-retention-reference-%d", time.Now().UnixNano())
	referenceVersionID := fmt.Sprintf("news-retention-version-%d", time.Now().UnixNano())
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_item_versions WHERE version_id = ?", referenceVersionID)
		for _, observation := range observations {
			_, _ = db.ExecContext(context.Background(), "DELETE FROM news_observations WHERE observation_id = ?", observation.ID)
		}
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_items WHERE news_id = ?", referenceNewsID)
	}()
	for _, observation := range observations {
		if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
			_, err := tx.AppendObservation(ctx, observation)
			return err
		}); err != nil {
			t.Fatalf("insert expiring observation %s: %v", observation.ID, err)
		}
	}

	// 让一条 Observation 被版本事实外键引用，确认载荷清理不会破坏历史引用。
	if _, err := db.ExecContext(ctx, "INSERT INTO news_items (news_id, origin_source_id) VALUES (?, ?)", referenceNewsID, "news-retention-integration"); err != nil {
		t.Fatalf("insert referenced news item: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE news_observations SET news_id = ? WHERE observation_id = ?", referenceNewsID, observations[0].ID); err != nil {
		t.Fatalf("bind referenced observation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_item_versions
		(version_id, news_id, version_no, fingerprint, title, content_json, observed_at, observation_id)
		VALUES (?, ?, 1, ?, ?, JSON_OBJECT(), ?, ?)`,
		referenceVersionID, referenceNewsID, strings.Repeat("a", 64), "retention reference",
		observations[0].ObservedAt, observations[0].ID); err != nil {
		t.Fatalf("insert version referencing observation: %v", err)
	}

	firstBatch, err := repository.PurgeExpiredObservationPayloads(ctx, now, 2)
	if err != nil || firstBatch.Scanned != 2 || firstBatch.Purged != 2 {
		t.Fatalf("first batch result=%+v err=%v", firstBatch, err)
	}
	secondBatch, err := repository.PurgeExpiredObservationPayloads(ctx, now, 2)
	if err != nil || secondBatch.Scanned != 1 || secondBatch.Purged != 1 {
		t.Fatalf("second batch result=%+v err=%v", secondBatch, err)
	}
	repeated, err := repository.PurgeExpiredObservationPayloads(ctx, now.Add(time.Minute), 2)
	if err != nil || repeated.Scanned != 0 || repeated.Purged != 0 {
		t.Fatalf("repeat purge result=%+v err=%v", repeated, err)
	}

	for _, observation := range observations {
		var candidateJSON []byte
		var purgedAt sql.NullTime
		if err := db.QueryRowContext(ctx, "SELECT candidate_json, payload_purged_at FROM news_observations WHERE observation_id = ?", observation.ID).Scan(&candidateJSON, &purgedAt); err != nil {
			t.Fatalf("read purged observation %s: %v", observation.ID, err)
		}
		var candidate domain.Candidate
		if err := json.Unmarshal(candidateJSON, &candidate); err != nil {
			t.Fatalf("decode purged candidate %s: %v", observation.ID, err)
		}
		if candidate.Title != "" || !purgedAt.Valid {
			t.Fatalf("candidate payload was not cleared: candidate=%+v purged_at=%+v", candidate, purgedAt)
		}
		var retainedRows int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_observations WHERE observation_id = ?", observation.ID).Scan(&retainedRows); err != nil {
			t.Fatal(err)
		}
		if retainedRows != 1 {
			t.Fatalf("observation tombstone row count=%d, want 1", retainedRows)
		}
	}
	pitRecord, err := repository.GetEvidenceVersionAt(ctx, app.GetEvidenceRequest{
		NewsID: referenceNewsID, VersionID: referenceVersionID, SystemAsOf: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("PIT query after observation payload purge: %v", err)
	}
	if pitRecord.Observation.PayloadPurgedAt == nil || pitRecord.Observation.Candidate.Title != "" {
		t.Fatalf("PIT query disclosed a purged candidate payload: %+v", pitRecord.Observation)
	}
	var retainedVersionRows int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_item_versions WHERE version_id = ? AND observation_id = ?", referenceVersionID, observations[0].ID).Scan(&retainedVersionRows); err != nil {
		t.Fatal(err)
	}
	if retainedVersionRows != 1 {
		t.Fatalf("version reference row count=%d, want 1", retainedVersionRows)
	}
	if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		_, err := tx.AppendObservation(ctx, observations[0])
		return err
	}); !errors.Is(err, app.ErrObservationPayloadPurged) {
		t.Fatalf("idempotent replay after purge error=%v, want ErrObservationPayloadPurged", err)
	}

	// 用数据库触发器在载荷 UPDATE 阶段注入失败，验证清理事务不会留下半成品。
	rollbackObservation := domain.Observation{
		ID:             fmt.Sprintf("news-retention-rollback-%d", time.Now().UnixNano()),
		IdempotencyKey: fmt.Sprintf("news-retention-rollback-key-%d", time.Now().UnixNano()),
		SourceID:       "news-retention-integration",
		Candidate:      domain.Candidate{SourceItemID: "retention-rollback-item", Title: "must survive rollback"},
		ObservedAt:     now.Add(-2 * time.Hour), RetentionUntil: now.Add(-time.Hour),
		IdentityStatus: domain.ObservationUnidentified,
	}
	if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		_, err := tx.AppendObservation(ctx, rollbackObservation)
		return err
	}); err != nil {
		t.Fatalf("insert rollback observation: %v", err)
	}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DROP TRIGGER IF EXISTS news_retention_force_rollback")
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_observations WHERE observation_id = ?", rollbackObservation.ID)
	}()
	triggerSQL := "CREATE TRIGGER news_retention_force_rollback " +
		"BEFORE UPDATE ON news_observations FOR EACH ROW " +
		"SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'forced observation purge rollback'"
	if _, err := db.ExecContext(ctx, triggerSQL); err != nil {
		t.Fatalf("create rollback injection trigger: %v", err)
	}
	if result, purgeErr := repository.PurgeExpiredObservationPayloads(ctx, now, 1); purgeErr == nil {
		t.Fatalf("purge with injected update failure unexpectedly succeeded: result=%+v", result)
	}
	var rollbackCandidateJSON []byte
	var rollbackPurgedAt sql.NullTime
	if err := db.QueryRowContext(ctx, "SELECT candidate_json, payload_purged_at FROM news_observations WHERE observation_id = ?", rollbackObservation.ID).Scan(&rollbackCandidateJSON, &rollbackPurgedAt); err != nil {
		t.Fatalf("read rollback observation after failed purge: %v", err)
	}
	var rollbackCandidate domain.Candidate
	if err := json.Unmarshal(rollbackCandidateJSON, &rollbackCandidate); err != nil {
		t.Fatalf("decode rollback candidate: %v", err)
	}
	if rollbackCandidate.Title != rollbackObservation.Candidate.Title || rollbackPurgedAt.Valid {
		t.Fatalf("failed purge left partial mutation: candidate=%+v purged_at=%+v", rollbackCandidate, rollbackPurgedAt)
	}
	if _, err := db.ExecContext(ctx, "DROP TRIGGER news_retention_force_rollback"); err != nil {
		t.Fatalf("drop rollback injection trigger: %v", err)
	}
	recovered, err := repository.PurgeExpiredObservationPayloads(ctx, now, 1)
	if err != nil || recovered.Scanned != 1 || recovered.Purged != 1 {
		t.Fatalf("purge retry after rollback result=%+v err=%v", recovered, err)
	}

	// 两个并发清理调用处理同一批到期记录时，不得重复清理或遗漏。
	concurrent := make([]domain.Observation, 4)
	for i := range concurrent {
		concurrent[i] = domain.Observation{
			ID:             fmt.Sprintf("news-retention-concurrent-%d-%d", time.Now().UnixNano(), i),
			IdempotencyKey: fmt.Sprintf("news-retention-concurrent-key-%d-%d", time.Now().UnixNano(), i),
			SourceID:       "news-retention-integration",
			Candidate:      domain.Candidate{SourceItemID: fmt.Sprintf("retention-concurrent-item-%d", i), Title: "concurrent purge"},
			ObservedAt:     now.Add(-2 * time.Hour), RetentionUntil: now.Add(-time.Hour),
			IdentityStatus: domain.ObservationUnidentified,
		}
	}
	defer func() {
		for _, observation := range concurrent {
			_, _ = db.ExecContext(context.Background(), "DELETE FROM news_observations WHERE observation_id = ?", observation.ID)
		}
	}()
	for _, observation := range concurrent {
		if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
			_, err := tx.AppendObservation(ctx, observation)
			return err
		}); err != nil {
			t.Fatalf("insert concurrent observation %s: %v", observation.ID, err)
		}
	}
	type purgeOutcome struct {
		result app.ObservationPayloadPurgeResult
		err    error
	}
	outcomes := make(chan purgeOutcome, 2)
	var workers sync.WaitGroup
	for i := 0; i < 2; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			result, purgeErr := repository.PurgeExpiredObservationPayloads(ctx, now, 2)
			outcomes <- purgeOutcome{result: result, err: purgeErr}
		}()
	}
	workers.Wait()
	close(outcomes)
	totalPurged := 0
	for outcome := range outcomes {
		if outcome.err != nil {
			t.Fatalf("concurrent purge failed: %v", outcome.err)
		}
		if outcome.result.Purged > 2 || outcome.result.Purged != outcome.result.Scanned {
			t.Fatalf("invalid concurrent purge result: %+v", outcome.result)
		}
		totalPurged += outcome.result.Purged
	}
	if totalPurged != len(concurrent) {
		t.Fatalf("concurrent purged total=%d, want %d", totalPurged, len(concurrent))
	}
}
