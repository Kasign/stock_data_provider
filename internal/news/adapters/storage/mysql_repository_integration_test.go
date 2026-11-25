package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func insertIntegrationNewsItem(ctx context.Context, db *sql.DB, newsID string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO news_items
		(news_id, title, content_json, content_fingerprint)
		VALUES (?, 'integration fixture', JSON_OBJECT(), REPEAT('0', 64))`, newsID)
	return err
}

// repositoryIngestHarness 在 MariaDB 集成测试中调用与正式入库一致的事务端口。
type repositoryIngestHarness struct {
	repository app.IdentityRepository
	newID      func() string
}

func (h repositoryIngestHarness) Execute(
	ctx context.Context, sourceID, idempotencyKey string, candidate domain.Candidate,
	policy domain.SourceURLPolicy, contentLayers ...domain.ContentLayers,
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
	content := domain.ContentLayersFromCandidate(candidate)
	if len(contentLayers) > 0 {
		content = domain.NormalizeContentLayers(contentLayers[0])
	}
	fingerprint := domain.BuildVersionFingerprint(candidate, content)
	canonicalHash := ""
	if candidate.CanonicalURL != "" {
		canonicalHash = canonicalHashForIntegration(candidate.CanonicalURL)
	}
	credentials, err := domain.BuildIdentityCredentials(sourceID, candidate.SourceItemID)
	if err != nil {
		return domain.Observation{}, err
	}
	observation := domain.Observation{
		ID: h.newID(), IdempotencyKey: idempotencyKey, SourceID: sourceID,
		ProviderCode: candidate.ProviderCode, QueryDimension: candidate.QueryDimension,
		Candidate: candidate, ObservedAt: observedAt,
	}
	if observation.ID == "" {
		return domain.Observation{}, app.ErrInvalidRequest
	}
	if candidate.CanonicalURL == "" && len(credentials) == 0 {
		if candidate.RawURL != "" {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = "news.identity.url_rejected"
		} else {
			observation.IdentityStatus = domain.ObservationUnidentified
			observation.ReasonCode = "news.identity.credential_required"
		}
	}
	var persisted domain.Observation
	err = h.repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		if candidate.CanonicalURL == "" && len(credentials) == 0 {
			var appendErr error
			persisted, appendErr = tx.AppendObservation(ctx, observation)
			return appendErr
		}
		var urlItem domain.NewsItem
		urlMatched := false
		if candidate.CanonicalURL != "" {
			var findErr error
			urlItem, urlMatched, findErr = tx.FindNewsItemByCanonicalURL(ctx, candidate.CanonicalURL, canonicalHash)
			if errors.Is(findErr, app.ErrCanonicalURLHashCollision) {
				observation.IdentityStatus = domain.ObservationConflict
				observation.ReasonCode = "news.identity.canonical_url_hash_collision"
				persisted, findErr = tx.AppendObservation(ctx, observation)
				return findErr
			}
			if findErr != nil {
				return findErr
			}
		}
		var existing []domain.IdentityCredential
		resolution := domain.IdentityResolution{Status: domain.IdentityNew}
		if len(credentials) > 0 {
			existing, err = tx.FindIdentityCredentials(ctx, sourceID, credentials)
			if err != nil {
				return err
			}
			resolution = domain.ResolveIdentity(sourceID, credentials, existing)
		}
		if resolution.Status == domain.IdentityConflict {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = resolution.Reason
			persisted, err = tx.AppendObservation(ctx, observation)
			return err
		}
		if urlMatched && resolution.Status == domain.IdentityMatched && urlItem.ID != resolution.NewsID {
			observation.IdentityStatus = domain.ObservationConflict
			observation.ReasonCode = "news.identity.multiple_items"
			persisted, err = tx.AppendObservation(ctx, observation)
			return err
		}
		var item domain.NewsItem
		newItem := false
		switch {
		case urlMatched:
			item = urlItem
		case resolution.Status == domain.IdentityMatched:
			var found bool
			var findErr error
			item, found, findErr = tx.FindNewsItemByID(ctx, resolution.NewsID)
			if findErr != nil {
				return findErr
			}
			if !found {
				return errors.New("news.identity.native_item_missing")
			}
		default:
			item = domain.NewsItem{ID: h.newID() + "-news"}
			newItem = true
		}
		if candidate.CanonicalURL != "" {
			if item.CanonicalURL != "" && item.CanonicalURL != candidate.CanonicalURL {
				observation.IdentityStatus = domain.ObservationConflict
				observation.ReasonCode = "news.identity.canonical_url_mismatch"
				persisted, err = tx.AppendObservation(ctx, observation)
				return err
			}
			item.CanonicalURL, item.CanonicalURLHash = candidate.CanonicalURL, canonicalHash
		}
		if newItem {
			observation.IdentityStatus = domain.ObservationNew
		} else {
			observation.IdentityStatus = domain.ObservationMatched
		}
		observation.NewsID = item.ID
		item.Title, item.PublisherName, item.PublisherDomain, item.Author = candidate.Title, candidate.PublisherName, candidate.PublisherDomain, candidate.Author
		item.PublishedAt, item.Content, item.ContentFingerprint = candidate.PublishedAt, content, fingerprint
		item.UpdatedAt = observedAt
		if newItem {
			item.CreatedAt = observedAt
			if err := tx.CreateNewsItem(ctx, item); err != nil {
				return err
			}
		}
		toSave := credentialsNotYetStoredForIntegration(credentials, existing)
		for i := range toSave {
			toSave[i].NewsID, toSave[i].ObservedAt = item.ID, observedAt
		}
		if len(toSave) > 0 {
			if err := tx.SaveIdentityCredentials(ctx, toSave); err != nil {
				return err
			}
		}
		persisted, err = tx.AppendObservation(ctx, observation)
		if err != nil {
			return err
		}
		if persisted.ID != observation.ID {
			return nil
		}
		latest, exists, err := tx.LatestNewsItemVersion(ctx, item.ID)
		if err != nil {
			return err
		}
		if !exists || latest.Fingerprint != fingerprint {
			versionNo := 1
			if exists {
				versionNo = latest.VersionNo + 1
			}
			version := domain.NewsItemVersion{
				ID: h.newID() + "-version", NewsID: item.ID, VersionNo: versionNo, Fingerprint: fingerprint,
				Title: candidate.Title, PublisherName: candidate.PublisherName, PublisherDomain: candidate.PublisherDomain,
				Author: candidate.Author, PublishedAt: candidate.PublishedAt, Content: content,
				ObservedAt: observedAt, ObservationID: observation.ID,
			}
			if err := tx.AppendNewsItemVersion(ctx, version); err != nil {
				return err
			}
			if !newItem {
				if err := tx.UpdateNewsItem(ctx, item); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return domain.Observation{}, err
	}
	return persisted, nil
}

func credentialsNotYetStoredForIntegration(candidates, existing []domain.IdentityCredential) []domain.IdentityCredential {
	result := make([]domain.IdentityCredential, 0, len(candidates))
	for _, candidate := range candidates {
		found := false
		for _, stored := range existing {
			if stored.SourceID == candidate.SourceID && stored.Hash == candidate.Hash && stored.Value == candidate.Value {
				found = true
				break
			}
		}
		if !found {
			result = append(result, candidate)
		}
	}
	return result
}

func canonicalHashForIntegration(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func mustMarshalAuthorizationBinding(t *testing.T, binding domain.NewsAuthorizationBinding) []byte {
	t.Helper()
	payload, err := json.Marshal(binding)
	if err != nil {
		t.Fatalf("marshal authorization binding: %v", err)
	}
	return payload
}

func applyNewsBaselineAndVerifySchema(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	migrations := Migrations()
	if len(migrations) != 1 || migrations[0].Version != 1 || len(migrations[0].Statements) != 10 {
		t.Fatalf("News migration baseline = %#v, want one V1 with ten tables", migrations)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, MigrationNamespace, migrations); err != nil {
		t.Fatalf("apply News baseline: %v", err)
	}
	for _, column := range []string{
		"canonical_url", "canonical_url_hash", "title", "content_json", "content_fingerprint",
	} {
		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
			WHERE table_schema = DATABASE() AND table_name = 'news_items' AND column_name = ?`, column,
		).Scan(&count); err != nil {
			t.Fatalf("inspect News baseline column %s: %v", column, err)
		}
		if count != 1 {
			t.Fatalf("News baseline is missing news_items.%s", column)
		}
	}
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

	applyNewsBaselineAndVerifySchema(t, ctx, db)
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
	binding.AuthorizationRef.Decision = domain.AuthorizationDecisionAllowed
	binding.AuthorizationRef.Conditions = []string{}
	currentExpiry := time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)
	binding.AuthorizationRef.ExpiresAt.Value = &currentExpiry
	publisher, err := app.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		t.Fatal(err)
	}
	purpose := []domain.Purpose{domain.PurposeInternalPersonalResearch}
	contentKinds := []domain.ContentKind{domain.ContentSnippet}
	currentPolicyPayloads := map[string]any{
		domain.NewsPolicyTypeSource: domain.SourcePolicy{
			Status: domain.AuthorizationApproved, AllowedPurposes: purpose,
			AllowedContentKinds: contentKinds, EvidenceRef: "integration-source-evidence",
		},
		domain.NewsPolicyTypeEndpoint: domain.EndpointPolicy{},
		domain.NewsPolicyTypeProviderOutput: domain.ProviderOutputPolicy{
			Status: domain.AuthorizationApproved, AllowedPurposes: purpose,
			AllowedContentKinds: contentKinds, AllowRequestProcessing: true,
			EvidenceRef: "integration-provider-evidence",
		},
	}
	for _, expected := range binding.PolicySnapshot.Policies {
		payload, err := json.Marshal(currentPolicyPayloads[expected.PolicyType])
		if err != nil {
			t.Fatalf("marshal %s policy: %v", expected.PolicyType, err)
		}
		if err := publisher.Execute(ctx, expected.PolicyType, expected.PolicyRef, expected.PolicyVersion, payload); err != nil {
			t.Fatalf("publish %s policy %s: %v", expected.PolicyType, expected.PolicyRef, err)
		}
	}
	currentPolicies, err := repository.ResolveCurrentNewsAuthorizationPolicies(ctx, binding)
	if err != nil {
		t.Fatalf("resolve current authorization policies: %v", err)
	}
	if len(currentPolicies.Policies) != len(binding.PolicySnapshot.Policies) {
		t.Fatalf("resolved policy count = %d, want %d", len(currentPolicies.Policies), len(binding.PolicySnapshot.Policies))
	}
	currentDecision := domain.EvaluateNewsAuthorizationAgainstCurrentPolicies(binding, currentPolicies, time.Now().UTC())
	if !currentDecision.Allowed {
		t.Fatalf("matching current policies rejected: %+v", currentDecision)
	}
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
		"SELECT COUNT(DISTINCT news_id) FROM news_observations WHERE source_id = ? AND news_id IS NOT NULL", sourceID,
	).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 1 {
		t.Fatalf("concurrent repository transactions created %d News items, want 1", itemCount)
	}

	// 不同来源并发发现同一规范 URL，必须全局复用同一个新闻主体。
	urlPolicy := domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}}
	urlCandidate := domain.Candidate{
		SourceItemID: "same-native-id", RawURL: "https://news.example/global-story", Title: "Global URL identity",
	}
	urlObservations := make([]domain.Observation, 2)
	urlErrors := make(chan error, 2)
	var urlWait sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		urlWait.Add(1)
		go func() {
			defer urlWait.Done()
			source := fmt.Sprintf("%s-url-source-%d", sourceID, index)
			observation, ingestErr := ingest.Execute(ctx, source, fmt.Sprintf("url-global-key-%d", index), urlCandidate, urlPolicy)
			if ingestErr != nil {
				urlErrors <- ingestErr
				return
			}
			urlObservations[index] = observation
		}()
	}
	urlWait.Wait()
	close(urlErrors)
	for ingestErr := range urlErrors {
		t.Fatalf("cross-source canonical URL ingest failed: %v", ingestErr)
	}
	if urlObservations[0].NewsID == "" || urlObservations[0].NewsID != urlObservations[1].NewsID {
		t.Fatalf("cross-source URL did not resolve to one NewsID: %+v", urlObservations)
	}
	var globalURLItemCount, globalURLIdentityCount int
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM news_items WHERE canonical_url_hash = ?",
		canonicalHashForIntegration("https://news.example/global-story"),
	).Scan(&globalURLItemCount); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM news_identifiers WHERE news_id = ?", urlObservations[0].NewsID,
	).Scan(&globalURLIdentityCount); err != nil {
		t.Fatal(err)
	}
	if globalURLItemCount != 1 || globalURLIdentityCount != 2 {
		t.Fatalf("global URL deduplication produced items=%d source identities=%d, want 1 and 2",
			globalURLItemCount, globalURLIdentityCount)
	}

	traceSourceID := sourceID + "-provider-trace"
	traceCandidate := domain.Candidate{SourceItemID: "provider-trace-item", ProviderCode: "tavily", Title: "trace test"}
	if _, err := ingest.Execute(ctx, traceSourceID, "provider-trace-key", traceCandidate, domain.SourceURLPolicy{}); !errors.Is(err, app.ErrProviderOutputPolicyRequired) {
		t.Fatalf("self-declared provider policy error = %v, want fail-closed rejection", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT news_id) FROM news_observations WHERE source_id = ? AND news_id IS NOT NULL", traceSourceID,
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
		rollbackItem := domain.NewsItem{
			ID: "news-" + rollbackSource, Title: "rollback fixture",
			ContentFingerprint: strings.Repeat("0", 64), CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
		}
		if err := tx.CreateNewsItem(ctx, rollbackItem); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("rollback transaction error = %v, want injected error", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT COUNT(DISTINCT news_id) FROM news_observations WHERE source_id = ? AND news_id IS NOT NULL", rollbackSource,
	).Scan(&itemCount); err != nil {
		t.Fatal(err)
	}
	if itemCount != 0 {
		t.Fatalf("rolled-back News Item count = %d, want 0", itemCount)
	}

	var updateWait sync.WaitGroup
	updateErrors := make(chan error, workers)
	for index := 0; index < workers; index++ {
		index := index
		updateWait.Add(1)
		go func() {
			defer updateWait.Done()
			content := domain.ContentLayers{
				SourceSummary:       fmt.Sprintf("summary revision %d", index),
				SourceSummaryOrigin: domain.ContentOriginFeedSummary,
			}
			observation, err := ingest.Execute(ctx, sourceID, fmt.Sprintf("news-it-revision-%d", index),
				candidate, domain.SourceURLPolicy{}, content)
			if err != nil {
				updateErrors <- err
				return
			}
			if observation.NewsID != newsID {
				updateErrors <- fmt.Errorf("revision %d changed news ID to %s", index, observation.NewsID)
			}
		}()
	}
	updateWait.Wait()
	close(updateErrors)
	for err := range updateErrors {
		t.Fatalf("concurrent formal ingest version append failed: %v", err)
	}

	var latest domain.NewsItemVersion
	err = repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
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
	if latest.VersionNo != workers+1 {
		t.Fatalf("latest VersionNo = %d, want %d (initial plus concurrent content revisions)", latest.VersionNo, workers+1)
	}

	secondSourceID := sourceID + "-second"
	secondCandidate := domain.Candidate{
		SourceItemID: "native-item-002",
		Title:        "Second news item", PublisherName: "Second Publisher",
	}
	secondObservation, err := ingest.Execute(ctx, secondSourceID, "news-it-second-key", secondCandidate,
		domain.SourceURLPolicy{}, domain.ContentLayers{
			SourceSummary: "second source summary", SourceSummaryOrigin: domain.ContentOriginFeedSummary,
		})
	if err != nil {
		t.Fatalf("create second news item and version: %v", err)
	}
	var secondVersion struct{ Version domain.NewsItemVersion }
	err = repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		version, exists, err := tx.LatestNewsItemVersion(ctx, secondObservation.NewsID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("second news version missing")
		}
		secondVersion.Version = version
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Strict PIT fixtures require a traceable, version-bound verified available_at decision.
	observationRetention := secondObservation.ObservedAt.Add(24 * time.Hour)
	if _, err := db.ExecContext(ctx,
		"UPDATE news_observations SET retention_until = ? WHERE observation_id = ?",
		observationRetention, secondObservation.ID,
	); err != nil {
		t.Fatalf("set second Observation retention: %v", err)
	}
	availabilityEvidenceID := secondVersion.Version.ID + "-availability-evidence"
	candidateAvailableAt := secondObservation.ObservedAt.Add(-time.Minute).Truncate(time.Second)
	if !secondVersion.Version.ObservedAt.Equal(secondObservation.ObservedAt) {
		t.Fatalf("version observed_at %s must equal its source Observation time %s",
			secondVersion.Version.ObservedAt, secondObservation.ObservedAt)
	}
	availabilityRecordedAt := secondObservation.ObservedAt.Add(time.Microsecond)
	availabilityDecisionAt := secondVersion.Version.ObservedAt.Add(2 * time.Microsecond)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_evidence
		(evidence_id, news_id, version_id, observation_id, field_path, raw_value,
		 source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		 candidate_available_at_utc, parser_version, evidence_ref, observed_at, recorded_at,
		 retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'parsed', '', ?, ?, ?, ?, ?, ?)`,
		availabilityEvidenceID, secondObservation.NewsID, secondVersion.Version.ID,
		secondObservation.ID, "publisher.first_available_at", candidateAvailableAt.Format(time.RFC3339),
		"Z", "second", candidateAvailableAt, "integration-parser/1",
		"evidence://news-integration/available-at", secondObservation.ObservedAt,
		availabilityRecordedAt, observationRetention,
	); err != nil {
		t.Fatalf("insert verified available_at evidence: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_decisions
		(decision_id, version_id, decision_seq, status, selected_evidence_id,
		 considered_evidence_ids, reason_code, review_rule_version, recorded_at)
		VALUES (?, ?, 1, 'verified', ?, JSON_ARRAY(?), ?, ?, ?)`,
		availabilityEvidenceID+"-decision", secondVersion.Version.ID,
		availabilityEvidenceID, availabilityEvidenceID,
		"availability.source_verified", "integration-review/1", availabilityDecisionAt,
	); err != nil {
		t.Fatalf("insert verified available_at decision: %v", err)
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
	if !reflect.DeepEqual(pitRecord.Version.Content, domain.ContentLayers{}) {
		t.Fatalf("PIT metadata lookup loaded content before authorization: %+v", pitRecord.Version.Content)
	}

	atomicPolicyVersion := "atomic-source-v1"
	if _, err := db.ExecContext(ctx, "UPDATE news_observations SET source_policy_version_id = ? WHERE observation_id = ?",
		atomicPolicyVersion, secondObservation.ID); err != nil {
		t.Fatalf("set atomic read policy trace: %v", err)
	}
	expectedAtomicContent := secondVersion.Version.Content
	atomicPIT, err := repository.GetEvidenceVersionAt(ctx, app.GetEvidenceRequest{
		NewsID: secondObservation.NewsID, VersionID: secondVersion.Version.ID, SystemAsOf: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("reload Evidence PIT metadata for atomic read: %v", err)
	}
	secondObservation = atomicPIT.Observation
	secondVersion.Version = atomicPIT.Version
	atomicResourceRef, err := domain.NewsEvidenceResourceRef(secondVersion.Version.NewsID, secondVersion.Version.VersionNo)
	if err != nil {
		t.Fatal(err)
	}
	atomicBinding := domain.NewsAuthorizationBinding{
		CallerRef: "service:research", ResourceRef: atomicResourceRef,
		PurposeCode: "research_analysis", RequestedContentLevel: domain.AuthorizationContentSummary,
		ProviderRef: domain.NullableOpaqueRef{Present: true},
		EndpointRef: domain.NullableOpaqueRef{Present: true},
		SourceRef:   secondObservation.SourceID,
		AuthorizationRef: domain.NewsAuthorizationRef{
			AuthorizationRef: fmt.Sprintf("news-it-atomic-auth-%d", time.Now().UnixNano()),
			PolicyOwner:      "news", PolicyVersion: "news-it-atomic-snapshot",
			Decision: domain.AuthorizationDecisionAllowed, PurposeCode: "research_analysis",
			ContentLevel: domain.AuthorizationContentSummary, SubjectRef: "service:research",
			ResourceRef: atomicResourceRef, DecidedAt: time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano),
			ExpiresAt: domain.NullableDateTime{Present: true}, Conditions: []string{},
			DecisionFingerprint: strings.Repeat("a", 64), SchemaVersion: "1.0.0",
		},
		PolicySnapshot: domain.AuthorizationPolicySnapshot{
			SnapshotRef: "news-it-atomic-snapshot", SchemaVersion: "1.0.0",
			Policies: []domain.AuthorizationPolicyVersion{{
				PolicyType: domain.NewsPolicyTypeSource, PolicyRef: secondObservation.SourceID,
				PolicyVersion: atomicPolicyVersion,
			}},
		},
	}
	atomicPayload, err := json.Marshal(domain.SourcePolicy{
		Status:              domain.AuthorizationApproved,
		AllowedPurposes:     []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds: []domain.ContentKind{domain.ContentAbstract},
		EvidenceRef:         "news-it-atomic-source-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	atomicPublisher, err := app.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicPublisher.Execute(ctx, domain.NewsPolicyTypeSource, secondObservation.SourceID, atomicPolicyVersion, atomicPayload); err != nil {
		t.Fatalf("publish atomic read source policy: %v", err)
	}

	// Source Observation 写入事务也必须锁定同一当前策略指针，避免授权后、持久化前被撤销。
	lockSourceRef := "news-it-source-ingest-lock"
	lockSourceV1 := "news-it-source-ingest-v1"
	lockSourceV2 := "news-it-source-ingest-v2"
	lockPayloadV1, err := json.Marshal(domain.SourcePolicy{
		Status:               domain.AuthorizationApproved,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentAbstract},
		AllowedStorageModes:  []domain.StorageMode{domain.StoragePersistentExcerpt},
		MaxRetention:         time.Hour,
		EvidenceRef:          "news-it-source-ingest-lock-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicPublisher.Execute(ctx, domain.NewsPolicyTypeSource, lockSourceRef, lockSourceV1, lockPayloadV1); err != nil {
		t.Fatalf("publish source ingest lock policy: %v", err)
	}

	lockEndpointRef := "news-it-source-ingest-endpoint-with-policy"
	lockEndpointVersion := "news-it-source-ingest-endpoint-v1"
	endpointRetention := time.Hour
	endpointPayload, err := json.Marshal(domain.EndpointPolicy{
		Status:               domain.AuthorizationApproved,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentAbstract},
		AllowedStorageModes:  []domain.StorageMode{domain.StoragePersistentExcerpt},
		MaxRetention:         &endpointRetention,
		EvidenceRef:          "news-it-endpoint-ingest-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicPublisher.Execute(ctx, domain.NewsPolicyTypeEndpoint, lockEndpointRef, lockEndpointVersion, endpointPayload); err != nil {
		t.Fatalf("publish endpoint ingest policy: %v", err)
	}
	lockProviderRef := "news-it-source-ingest-provider-output"
	lockProviderVersion := "news-it-source-ingest-provider-v1"
	providerPayload, err := json.Marshal(domain.ProviderOutputPolicy{
		Status:                  domain.AuthorizationApproved,
		AllowedPurposes:         []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:     []domain.ContentKind{domain.ContentMetadata},
		AllowRequestProcessing:  true,
		MaxRequestRetention:     time.Hour,
		AllowedStorageModes:     []domain.StorageMode{domain.StoragePersistentMetadata},
		MaxPersistenceRetention: time.Hour,
		EvidenceRef:             "news-it-provider-output-ingest-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicPublisher.Execute(ctx, domain.NewsPolicyTypeProviderOutput, lockProviderRef, lockProviderVersion, providerPayload); err != nil {
		t.Fatalf("publish provider output ingest policy: %v", err)
	}
	var loadedSource domain.SourcePolicy
	var loadedEndpoint *domain.EndpointPolicy
	var loadedProvider *domain.ProviderOutputPolicy
	if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		policyTx, ok := tx.(app.CurrentIngestPolicyIdentityTransaction)
		if !ok {
			return errors.New("identity transaction lacks current ingest policy lock")
		}
		var loadErr error
		loadedSource, loadedEndpoint, loadedProvider, loadErr = policyTx.LockCurrentIngestPolicies(
			ctx, lockSourceRef, lockSourceV1,
			lockEndpointRef, lockEndpointVersion,
			lockProviderRef, lockProviderVersion,
		)
		return loadErr
	}); err != nil {
		t.Fatalf("load complete current ingest policy set: %v", err)
	}
	if loadedSource.EvidenceRef != "news-it-source-ingest-lock-evidence" ||
		loadedEndpoint == nil || loadedEndpoint.EvidenceRef != "news-it-endpoint-ingest-evidence" ||
		loadedProvider == nil || loadedProvider.EvidenceRef != "news-it-provider-output-ingest-evidence" {
		t.Fatalf("incomplete current ingest policy set: source=%+v endpoint=%+v provider=%+v",
			loadedSource, loadedEndpoint, loadedProvider)
	}
	lockPayloadV2, err := json.Marshal(domain.SourcePolicy{
		Status:               domain.AuthorizationRevoked,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentMetadata},
		AllowedStorageModes:  []domain.StorageMode{domain.StoragePersistentMetadata},
		MaxRetention:         time.Hour,
		EvidenceRef:          "news-it-source-ingest-revoked-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 没有 Endpoint 当前策略行时，Repeatable Read 的 InnoDB gap lock 也必须阻止并发首次发布。
	missingEndpointRef := "news-it-source-ingest-endpoint"
	endpointCreatePayload, err := json.Marshal(domain.EndpointPolicy{
		Status:               domain.AuthorizationApproved,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentMetadata},
		AllowedStorageModes:  []domain.StorageMode{domain.StoragePersistentMetadata},
		MaxRetention:         &endpointRetention,
		EvidenceRef:          "news-it-source-ingest-endpoint-first-publish",
	})
	if err != nil {
		t.Fatal(err)
	}
	sourcePolicyLocked := make(chan struct{})
	releaseSourceWrite := make(chan struct{})
	sourceWriteDone := make(chan error, 1)
	go func() {
		sourceWriteDone <- repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
			policyTx, ok := tx.(app.CurrentIngestPolicyIdentityTransaction)
			if !ok {
				return errors.New("identity transaction lacks source policy lock")
			}
			if _, _, _, err := policyTx.LockCurrentIngestPolicies(
				ctx, lockSourceRef, lockSourceV1, "news-it-source-ingest-endpoint", "", "", "",
			); err != nil {
				return err
			}
			close(sourcePolicyLocked)
			select {
			case <-releaseSourceWrite:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-sourcePolicyLocked:
	case <-time.After(5 * time.Second):
		close(releaseSourceWrite)
		t.Fatal("source ingest transaction did not lock current policy")
	}
	sourceEndpointPublishDone := make(chan error, 1)
	go func() {
		sourceEndpointPublishDone <- atomicPublisher.Execute(
			ctx, domain.NewsPolicyTypeEndpoint, missingEndpointRef,
			"news-it-source-ingest-endpoint-first", endpointCreatePayload,
		)
	}()
	select {
	case publishErr := <-sourceEndpointPublishDone:
		close(releaseSourceWrite)
		t.Fatalf("first Endpoint policy publication bypassed missing-row lock: %v", publishErr)
	case <-time.After(100 * time.Millisecond):
	}
	sourceRevokeDone := make(chan error, 1)
	go func() {
		sourceRevokeDone <- atomicPublisher.Execute(ctx, domain.NewsPolicyTypeSource, lockSourceRef, lockSourceV2, lockPayloadV2)
	}()
	select {
	case revokeErr := <-sourceRevokeDone:
		close(releaseSourceWrite)
		t.Fatalf("source policy replacement completed while Observation transaction held its lock: %v", revokeErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseSourceWrite)
	if err := <-sourceWriteDone; err != nil {
		t.Fatalf("source Observation transaction: %v", err)
	}
	if err := <-sourceEndpointPublishDone; err != nil {
		t.Fatalf("publish first Endpoint policy after Observation transaction: %v", err)
	}
	if err := <-sourceRevokeDone; err != nil {
		t.Fatalf("publish source policy revocation after Observation transaction: %v", err)
	}
	if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		policyTx, ok := tx.(app.CurrentIngestPolicyIdentityTransaction)
		if !ok {
			return errors.New("identity transaction lacks source policy lock")
		}
		_, _, _, err := policyTx.LockCurrentIngestPolicies(
			ctx, lockSourceRef, lockSourceV1,
			"news-it-source-ingest-endpoint", "news-it-source-ingest-endpoint-first", "", "",
		)
		return err
	}); !errors.Is(err, app.ErrNewsCurrentPolicyUnavailable) {
		t.Fatalf("stale source policy after committed replacement error = %v, want current policy unavailable", err)
	}

	// Provider Output 策略替换也必须与完整 Observation 写事务共享当前指针锁。
	providerLockSourceRef := "news-it-provider-ingest-lock-source"
	providerLockSourceVersion := "news-it-provider-ingest-source-v1"
	providerLockSourcePayload, err := json.Marshal(domain.SourcePolicy{
		Status:               domain.AuthorizationApproved,
		AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
		AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:  []domain.ContentKind{domain.ContentMetadata},
		AllowedStorageModes:  []domain.StorageMode{domain.StoragePersistentMetadata},
		MaxRetention:         time.Hour,
		EvidenceRef:          "news-it-provider-ingest-source-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := atomicPublisher.Execute(ctx, domain.NewsPolicyTypeSource, providerLockSourceRef, providerLockSourceVersion, providerLockSourcePayload); err != nil {
		t.Fatalf("publish Provider Ingest source policy: %v", err)
	}
	providerLockRevokedVersion := "news-it-source-ingest-provider-output-v2"
	providerLockRevokedPayload, err := json.Marshal(domain.ProviderOutputPolicy{
		Status:                  domain.AuthorizationRevoked,
		AllowedPurposes:         []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds:     []domain.ContentKind{domain.ContentMetadata},
		AllowRequestProcessing:  false,
		MaxRequestRetention:     time.Hour,
		AllowedStorageModes:     []domain.StorageMode{domain.StoragePersistentMetadata},
		MaxPersistenceRetention: time.Hour,
		EvidenceRef:             "news-it-provider-output-ingest-revoked-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	providerWriteLocked := make(chan struct{})
	releaseProviderWrite := make(chan struct{})
	providerWriteDone := make(chan error, 1)
	go func() {
		providerWriteDone <- repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
			policyTx, ok := tx.(app.CurrentIngestPolicyIdentityTransaction)
			if !ok {
				return errors.New("identity transaction lacks current ingest policy lock")
			}
			if _, _, _, err := policyTx.LockCurrentIngestPolicies(
				ctx,
				providerLockSourceRef, providerLockSourceVersion,
				lockEndpointRef, lockEndpointVersion,
				lockProviderRef, lockProviderVersion,
			); err != nil {
				return err
			}
			close(providerWriteLocked)
			select {
			case <-releaseProviderWrite:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()
	select {
	case <-providerWriteLocked:
	case <-time.After(5 * time.Second):
		close(releaseProviderWrite)
		t.Fatal("Provider Ingest transaction did not lock current policy set")
	}
	providerRevokeDone := make(chan error, 1)
	go func() {
		providerRevokeDone <- atomicPublisher.Execute(
			ctx, domain.NewsPolicyTypeProviderOutput, lockProviderRef,
			providerLockRevokedVersion, providerLockRevokedPayload,
		)
	}()
	select {
	case revokeErr := <-providerRevokeDone:
		close(releaseProviderWrite)
		t.Fatalf("Provider Output replacement completed while Observation transaction held its lock: %v", revokeErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseProviderWrite)
	if err := <-providerWriteDone; err != nil {
		t.Fatalf("Provider Ingest transaction: %v", err)
	}
	if err := <-providerRevokeDone; err != nil {
		t.Fatalf("publish Provider Output revocation after Observation transaction: %v", err)
	}
	if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		policyTx, ok := tx.(app.CurrentIngestPolicyIdentityTransaction)
		if !ok {
			return errors.New("identity transaction lacks current ingest policy lock")
		}
		_, _, _, err := policyTx.LockCurrentIngestPolicies(
			ctx,
			providerLockSourceRef, providerLockSourceVersion,
			lockEndpointRef, lockEndpointVersion,
			lockProviderRef, lockProviderVersion,
		)
		return err
	}); !errors.Is(err, app.ErrNewsCurrentPolicyUnavailable) {
		t.Fatalf("stale Provider Output policy after committed replacement error = %v, want current policy unavailable", err)
	}

	atomicContent, atomicReadObservation, err := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, atomicBinding, secondObservation, secondVersion.Version, time.Now().UTC(),
		func(readCtx context.Context, current domain.CurrentAuthorizationPolicySet, currentObservation domain.Observation) error {
			if err := readCtx.Err(); err != nil {
				return err
			}
			decision := domain.EvaluateNewsAuthorizationAgainstCurrentPolicies(atomicBinding, current, time.Now().UTC())
			if !decision.Allowed {
				return fmt.Errorf("atomic read authorization denied: %s", decision.ReasonCode)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("atomic authorized evidence read: %v", err)
	}
	if atomicContent.SourceSummary != expectedAtomicContent.SourceSummary {
		t.Fatalf("atomic evidence content mismatch: got %+v want %+v", atomicContent, expectedAtomicContent)
	}
	if atomicReadObservation.ID != secondObservation.ID ||
		atomicReadObservation.SourcePolicyVersionID != atomicPolicyVersion {
		t.Fatalf("atomic read Observation was not refreshed: %+v", atomicReadObservation)
	}

	if _, _, err := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, atomicBinding, secondObservation, secondVersion.Version, secondVersion.Version.ObservedAt.Add(-time.Second),
		func(_ context.Context, _ domain.CurrentAuthorizationPolicySet, _ domain.Observation) error {
			return nil
		},
	); !errors.Is(err, app.ErrEvidencePITNotFound) {
		t.Fatalf("atomic content before system_as_of error = %v, want PIT not found", err)
	}

	// 读取事务先锁定当前指针时，撤销发布必须等待读取提交；随后旧 Binding 立即失效。
	revokedPolicyVersion := "atomic-source-v2"
	revokedPayload, err := json.Marshal(domain.SourcePolicy{
		Status:              domain.AuthorizationRevoked,
		AllowedPurposes:     []domain.Purpose{domain.PurposeInternalPersonalResearch},
		AllowedContentKinds: []domain.ContentKind{domain.ContentAbstract},
		EvidenceRef:         "news-it-atomic-revocation-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	releaseRead := make(chan struct{})
	readDone := make(chan error, 1)
	go func() {
		_, _, readErr := repository.GetEvidenceContentWithCurrentAuthorization(
			ctx, atomicBinding, secondObservation, secondVersion.Version, time.Now().UTC(),
			func(readCtx context.Context, current domain.CurrentAuthorizationPolicySet, currentObservation domain.Observation) error {
				decision := domain.EvaluateNewsAuthorizationAgainstCurrentPolicies(atomicBinding, current, time.Now().UTC())
				if !decision.Allowed {
					return fmt.Errorf("read-first authorization denied: %s", decision.ReasonCode)
				}
				close(locked)
				select {
				case <-releaseRead:
					return readCtx.Err()
				case <-readCtx.Done():
					return readCtx.Err()
				}
			},
		)
		readDone <- readErr
	}()
	select {
	case <-locked:
	case <-time.After(5 * time.Second):
		close(releaseRead)
		t.Fatal("atomic read did not acquire current policy locks")
	}
	publishStarted := make(chan struct{})
	publishDone := make(chan error, 1)
	go func() {
		close(publishStarted)
		publishDone <- atomicPublisher.Execute(
			ctx, domain.NewsPolicyTypeSource, secondObservation.SourceID, revokedPolicyVersion, revokedPayload,
		)
	}()
	<-publishStarted
	select {
	case publishErr := <-publishDone:
		close(releaseRead)
		t.Fatalf("revocation publication completed while read held the policy lock: %v", publishErr)
	case <-time.After(100 * time.Millisecond):
	}
	close(releaseRead)
	if err := <-readDone; err != nil {
		t.Fatalf("read that acquired policy locks before revocation failed: %v", err)
	}
	if err := <-publishDone; err != nil {
		t.Fatalf("publish revocation after read commit: %v", err)
	}
	staleBindingCallbackCalled := false
	_, _, staleBindingErr := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, atomicBinding, secondObservation, secondVersion.Version, time.Now().UTC(),
		func(_ context.Context, _ domain.CurrentAuthorizationPolicySet, _ domain.Observation) error {
			staleBindingCallbackCalled = true
			return nil
		},
	)
	if !errors.Is(staleBindingErr, app.ErrNewsCurrentPolicyUnavailable) || staleBindingCallbackCalled {
		t.Fatalf("stale Binding after policy replacement error=%v authorization_callback_called=%t, want fail closed before authorization",
			staleBindingErr, staleBindingCallbackCalled)
	}

	// 撤销已经提交后，匹配新策略版本的读取也必须由领域授权规则拒绝。
	revokedBinding := atomicBinding
	revokedBinding.PolicySnapshot.Policies = append([]domain.AuthorizationPolicyVersion(nil), atomicBinding.PolicySnapshot.Policies...)
	revokedBinding.PolicySnapshot.Policies[0].PolicyVersion = revokedPolicyVersion
	revokedObservation := secondObservation
	revokedObservation.SourcePolicyVersionID = revokedPolicyVersion
	if _, err := db.ExecContext(ctx, "UPDATE news_observations SET source_policy_version_id = ? WHERE observation_id = ?",
		revokedPolicyVersion, revokedObservation.ID); err != nil {
		t.Fatalf("set revoked policy trace: %v", err)
	}
	_, _, revokedReadErr := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, revokedBinding, revokedObservation, secondVersion.Version, time.Now().UTC(),
		func(readCtx context.Context, current domain.CurrentAuthorizationPolicySet, currentObservation domain.Observation) error {
			decision := domain.EvaluateNewsAuthorizationAgainstCurrentPolicies(revokedBinding, current, time.Now().UTC())
			if !decision.Allowed {
				return app.ErrNewsCurrentPolicyDenied
			}
			return nil
		},
	)
	if !errors.Is(revokedReadErr, app.ErrNewsCurrentPolicyDenied) {
		t.Fatalf("read after committed revocation error = %v, want authorization denied", revokedReadErr)
	}

	// 策略 Payload Hash 被篡改时，原子读取必须在正文查询前拒绝。
	var originalPolicyHash string
	if err := db.QueryRowContext(ctx, `SELECT payload_hash FROM news_policy_versions
		WHERE policy_type = ? AND policy_ref = ? AND policy_version = ?`,
		domain.NewsPolicyTypeSource, secondObservation.SourceID, revokedPolicyVersion,
	).Scan(&originalPolicyHash); err != nil {
		t.Fatalf("read source policy hash: %v", err)
	}
	tamperedPolicyHash := strings.Repeat("0", 64)
	if tamperedPolicyHash == originalPolicyHash {
		tamperedPolicyHash = strings.Repeat("1", 64)
	}
	if _, err := db.ExecContext(ctx, `UPDATE news_policy_versions SET payload_hash = ?
		WHERE policy_type = ? AND policy_ref = ? AND policy_version = ?`,
		tamperedPolicyHash, domain.NewsPolicyTypeSource, secondObservation.SourceID, revokedPolicyVersion,
	); err != nil {
		t.Fatalf("tamper source policy hash: %v", err)
	}
	_, _, tamperedPolicyErr := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, revokedBinding, revokedObservation, secondVersion.Version, time.Now().UTC(),
		func(_ context.Context, _ domain.CurrentAuthorizationPolicySet, _ domain.Observation) error {
			return nil
		},
	)
	if _, err := db.ExecContext(ctx, `UPDATE news_policy_versions SET payload_hash = ?
		WHERE policy_type = ? AND policy_ref = ? AND policy_version = ?`,
		originalPolicyHash, domain.NewsPolicyTypeSource, secondObservation.SourceID, revokedPolicyVersion,
	); err != nil {
		t.Fatalf("restore source policy hash: %v", err)
	}
	if !errors.Is(tamperedPolicyErr, app.ErrNewsCurrentPolicyUnavailable) {
		t.Fatalf("tampered source policy read error = %v, want current policy unavailable", tamperedPolicyErr)
	}

	// 当前策略指针缺失时 fail closed，不允许回退到历史策略版本。
	if _, err := db.ExecContext(ctx, `DELETE FROM news_current_policy_versions
		WHERE policy_type = ? AND policy_ref = ?`, domain.NewsPolicyTypeSource, secondObservation.SourceID); err != nil {
		t.Fatalf("delete current source policy pointer: %v", err)
	}
	_, _, missingPointerErr := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, revokedBinding, revokedObservation, secondVersion.Version, time.Now().UTC(),
		func(_ context.Context, _ domain.CurrentAuthorizationPolicySet, _ domain.Observation) error {
			return nil
		},
	)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_current_policy_versions
		(policy_type, policy_ref, policy_version, updated_at) VALUES (?, ?, ?, ?)`,
		domain.NewsPolicyTypeSource, secondObservation.SourceID, revokedPolicyVersion, time.Now().UTC(),
	); err != nil {
		t.Fatalf("restore current source policy pointer: %v", err)
	}
	if !errors.Is(missingPointerErr, app.ErrNewsCurrentPolicyUnavailable) {
		t.Fatalf("missing current policy pointer error = %v, want current policy unavailable", missingPointerErr)
	}

	// Binding 的资源引用不匹配时，不得进入策略读取或正文读取事务。
	invalidResourceBinding := revokedBinding
	invalidResourceBinding.ResourceRef = "news-evidence-wrong-resource"
	if _, _, err := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, invalidResourceBinding, revokedObservation, secondVersion.Version, time.Now().UTC(),
		func(_ context.Context, _ domain.CurrentAuthorizationPolicySet, _ domain.Observation) error {
			return nil
		},
	); !errors.Is(err, app.ErrInvalidEvidenceQuery) {
		t.Fatalf("mismatched resource binding error = %v, want invalid evidence query", err)
	}

	// 授权回调期间取消上下文时，事务必须回滚且不能返回正文。
	cancelCtx, cancelRead := context.WithCancel(ctx)
	_, _, canceledReadErr := repository.GetEvidenceContentWithCurrentAuthorization(
		cancelCtx, revokedBinding, revokedObservation, secondVersion.Version, time.Now().UTC(),
		func(_ context.Context, _ domain.CurrentAuthorizationPolicySet, _ domain.Observation) error {
			cancelRead()
			return nil
		},
	)
	cancelRead()
	if !errors.Is(canceledReadErr, context.Canceled) {
		t.Fatalf("canceled atomic read error = %v, want context canceled", canceledReadErr)
	}

	// RACE-04：元数据定位后，若同一 Version 的 fingerprint 被替换，
	// 原子读取必须拒绝旧引用，且不得调用授权回调或读取正文。
	originalVersionFingerprint := secondVersion.Version.Fingerprint
	changedVersionFingerprint := strings.Repeat("c", 64)
	if changedVersionFingerprint == originalVersionFingerprint {
		changedVersionFingerprint = strings.Repeat("d", 64)
	}
	if _, err := db.ExecContext(ctx, `UPDATE news_item_versions SET fingerprint = ?
		WHERE news_id = ? AND version_id = ?`,
		changedVersionFingerprint, secondVersion.Version.NewsID, secondVersion.Version.ID,
	); err != nil {
		t.Fatalf("change evidence version fingerprint: %v", err)
	}
	authorizationCallbackCalled := false
	_, _, changedVersionErr := repository.GetEvidenceContentWithCurrentAuthorization(
		ctx, revokedBinding, revokedObservation, secondVersion.Version, time.Now().UTC(),
		func(_ context.Context, _ domain.CurrentAuthorizationPolicySet, _ domain.Observation) error {
			authorizationCallbackCalled = true
			return nil
		},
	)
	if _, err := db.ExecContext(ctx, `UPDATE news_item_versions SET fingerprint = ?
		WHERE news_id = ? AND version_id = ?`,
		originalVersionFingerprint, secondVersion.Version.NewsID, secondVersion.Version.ID,
	); err != nil {
		t.Fatalf("restore evidence version fingerprint: %v", err)
	}
	if !errors.Is(changedVersionErr, app.ErrInvalidEvidenceQuery) || authorizationCallbackCalled {
		t.Fatalf("changed evidence version error=%v authorization_callback_called=%t, want invalid query before authorization",
			changedVersionErr, authorizationCallbackCalled)
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
	policyEvaluatedAt := time.Now().UTC().Truncate(time.Microsecond)
	if _, err := db.ExecContext(ctx, `UPDATE news_observations
		SET source_policy_version_id = ?, endpoint_id = ?,
		    endpoint_policy_version_id = ?, provider_output_policy_ref = ?,
		    provider_output_policy_version_id = ?, policy_evaluated_at = ?
		WHERE observation_id = ?`,
		"source-policy-pit-v2", "endpoint-pit-v1", "endpoint-policy-pit-v3",
		"provider-policy-pit", "provider-policy-v7", policyEvaluatedAt, secondObservation.ID,
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
	if !reflect.DeepEqual(listed[0].Version.Content, domain.ContentLayers{}) {
		t.Fatalf("ListEvidence PIT metadata lookup loaded content before per-item authorization: %+v", listed[0].Version.Content)
	}
	gotObservation := listed[0].Observation
	if gotObservation.SourcePolicyVersionID != "source-policy-pit-v2" ||
		gotObservation.EndpointID != "endpoint-pit-v1" ||
		gotObservation.EndpointPolicyVersionID != "endpoint-policy-pit-v3" ||
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
	var revisedVersion struct{ Version domain.NewsItemVersion }
	err = repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
		version, exists, err := tx.LatestNewsItemVersion(ctx, revisedObservation.NewsID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("revised News version missing")
		}
		revisedVersion.Version = version
		return nil
	})
	if err != nil {
		t.Fatalf("load second news revision: %v", err)
	}

	// 每个版本独立绑定自己的 available_at 决策；旧版本的时间证据不能
	// 自动继承到内容已变化的新版本。
	revisedObservationRetention := revisedObservation.ObservedAt.Add(24 * time.Hour)
	if _, err := db.ExecContext(ctx,
		"UPDATE news_observations SET retention_until = ? WHERE observation_id = ?",
		revisedObservationRetention, revisedObservation.ID,
	); err != nil {
		t.Fatalf("set revised Observation retention: %v", err)
	}
	if !revisedVersion.Version.ObservedAt.Equal(revisedObservation.ObservedAt) {
		t.Fatalf("revised Version observed_at %s must equal Observation time %s",
			revisedVersion.Version.ObservedAt, revisedObservation.ObservedAt)
	}
	revisedAvailabilityEvidenceID := revisedVersion.Version.ID + "-availability-evidence"
	revisedAvailableAt := revisedObservation.ObservedAt.Add(-time.Minute).Truncate(time.Second)
	revisedAvailabilityRecordedAt := revisedObservation.ObservedAt.Add(time.Microsecond)
	revisedAvailabilityDecisionAt := revisedVersion.Version.ObservedAt.Add(2 * time.Microsecond)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_evidence
		(evidence_id, news_id, version_id, observation_id, field_path, raw_value,
		 source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		 candidate_available_at_utc, parser_version, evidence_ref, observed_at, recorded_at,
		 retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'parsed', '', ?, ?, ?, ?, ?, ?)`,
		revisedAvailabilityEvidenceID, revisedObservation.NewsID, revisedVersion.Version.ID,
		revisedObservation.ID, "publisher.first_available_at", revisedAvailableAt.Format(time.RFC3339),
		"Z", "second", revisedAvailableAt, "integration-parser/1",
		"evidence://news-integration/revised-available-at", revisedObservation.ObservedAt,
		revisedAvailabilityRecordedAt, revisedObservationRetention,
	); err != nil {
		t.Fatalf("insert revised available_at evidence: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_decisions
		(decision_id, version_id, decision_seq, status, selected_evidence_id,
		 considered_evidence_ids, reason_code, review_rule_version, recorded_at)
		VALUES (?, ?, 1, 'verified', ?, JSON_ARRAY(?), ?, ?, ?)`,
		revisedAvailabilityEvidenceID+"-decision", revisedVersion.Version.ID,
		revisedAvailabilityEvidenceID, revisedAvailabilityEvidenceID,
		"availability.source_verified", "integration-review/1", revisedAvailabilityDecisionAt,
	); err != nil {
		t.Fatalf("insert revised available_at decision: %v", err)
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

// TestObservationRetentionPurgeIntegration covers MariaDB retention writes,
// idempotent purge, and the no-resurrection rule for an expired payload.
// TestNewsCurrentPolicyResolverMariaDBIntegration verifies that the existing
// authorization resolver port reads the exact current immutable policy versions.
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
	if err := insertIntegrationNewsItem(ctx, db, referenceNewsID); err != nil {
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
	availabilityEvidenceID := fmt.Sprintf("news-retention-availability-%d", time.Now().UnixNano())
	candidateAvailableAt := now.Add(-3 * time.Hour).Truncate(time.Second)
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_evidence
		(evidence_id, news_id, version_id, observation_id, field_path, raw_value,
		 source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		 candidate_available_at_utc, parser_version, evidence_ref, evidence_hash,
		 observed_at, recorded_at, retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)`,
		availabilityEvidenceID, referenceNewsID, referenceVersionID, observations[0].ID,
		"payload.publishTime", "2026-11-16T06:00:00Z", "UTC", "second", "parsed",
		candidateAvailableAt, "test-parser/1", "evidence://retention-test", strings.Repeat("b", 64),
		observations[0].ObservedAt, observations[0].ObservedAt.Add(time.Second), now.Add(-time.Hour)); err != nil {
		t.Fatalf("insert expiring availability evidence: %v", err)
	}

	firstBatch, err := repository.PurgeExpiredObservationPayloads(ctx, now, 2)
	if err != nil || firstBatch.Scanned != 2 || firstBatch.Purged != 2 || firstBatch.EvidenceScanned != 1 || firstBatch.EvidencePurged != 1 {
		t.Fatalf("first batch result=%+v err=%v", firstBatch, err)
	}
	var rawValue sql.NullString
	var candidateAt, evidencePurgedAt sql.NullTime
	var evidenceRef, evidenceHash sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT raw_value, candidate_available_at_utc, evidence_ref,
		evidence_hash, payload_purged_at FROM news_version_availability_evidence WHERE evidence_id = ?`,
		availabilityEvidenceID).Scan(&rawValue, &candidateAt, &evidenceRef, &evidenceHash, &evidencePurgedAt); err != nil {
		t.Fatalf("read purged availability evidence: %v", err)
	}
	if rawValue.Valid || candidateAt.Valid || evidenceRef.Valid || evidenceHash.Valid || !evidencePurgedAt.Valid {
		t.Fatalf("availability evidence payload was not fully purged: raw=%+v candidate=%+v ref=%+v hash=%+v purged=%+v", rawValue, candidateAt, evidenceRef, evidenceHash, evidencePurgedAt)
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
	_, err = repository.GetEvidenceVersionAt(ctx, app.GetEvidenceRequest{
		NewsID: referenceNewsID, VersionID: referenceVersionID, SystemAsOf: now.Add(time.Minute),
	})
	if !errors.Is(err, app.ErrEvidencePITNotFound) {
		t.Fatalf("strict PIT query after availability evidence purge error = %v, want PIT not found", err)
	}

	// available_at 证据已过期并清理后，列表也必须排除该版本，不能把 Tombstone
	// 误当成仍满足严格 PIT 的可用事实。
	listed, err := repository.ListEvidenceVersionsAt(ctx, app.ListEvidenceRequest{
		CallerRef:             "service:research",
		PurposeCode:           app.EvidencePurposeResearchAnalysis,
		RequestedContentLevel: app.EvidenceContentSummary,
		SystemAsOf:            time.Now().UTC(),
		Filters: &app.EvidenceQueryFilters{
			NewsIDs:    []string{referenceNewsID},
			SourceRefs: []string{"news-retention-integration"},
		},
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("ListEvidence PIT query after observation payload purge: %v", err)
	}
	if len(listed) != 0 {
		t.Fatalf("ListEvidence PIT returned %d records after available_at evidence purge, want 0", len(listed))
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

	// 精确到期边界应清理；独立 Evidence 的更短 Retention 不应等待父 Observation 到期。
	boundary := now.Add(time.Minute).Truncate(time.Microsecond)
	boundaryObservation := domain.Observation{
		ID:             fmt.Sprintf("news-retention-boundary-%d", time.Now().UnixNano()),
		IdempotencyKey: fmt.Sprintf("news-retention-boundary-key-%d", time.Now().UnixNano()),
		SourceID:       "news-retention-boundary",
		Candidate:      domain.Candidate{SourceItemID: "retention-boundary-item", Title: "boundary payload"},
		ObservedAt:     boundary.Add(-time.Hour), RetentionUntil: boundary,
		IdentityStatus: domain.ObservationUnidentified,
	}
	evidenceOnlyObservation := domain.Observation{
		ID:             fmt.Sprintf("news-retention-evidence-only-%d", time.Now().UnixNano()),
		IdempotencyKey: fmt.Sprintf("news-retention-evidence-only-key-%d", time.Now().UnixNano()),
		SourceID:       "news-retention-boundary",
		Candidate:      domain.Candidate{SourceItemID: "retention-evidence-only-item", Title: "parent still retained"},
		ObservedAt:     boundary.Add(-time.Hour), RetentionUntil: boundary.Add(time.Hour),
		IdentityStatus: domain.ObservationUnidentified,
	}
	boundaryNewsID := fmt.Sprintf("news-retention-boundary-news-%d", time.Now().UnixNano())
	boundaryVersionID := fmt.Sprintf("news-retention-boundary-version-%d", time.Now().UnixNano())
	boundaryEvidenceID := fmt.Sprintf("news-retention-boundary-evidence-%d", time.Now().UnixNano())
	boundaryCandidateAvailableAt := evidenceOnlyObservation.ObservedAt.Add(-time.Minute)
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_version_availability_evidence WHERE evidence_id = ?", boundaryEvidenceID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_item_versions WHERE version_id = ?", boundaryVersionID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_observations WHERE observation_id IN (?, ?)", boundaryObservation.ID, evidenceOnlyObservation.ID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_items WHERE news_id = ?", boundaryNewsID)
	}()
	for _, observation := range []domain.Observation{boundaryObservation, evidenceOnlyObservation} {
		if err := repository.WithinIdentityTransaction(ctx, func(tx app.IdentityTransaction) error {
			_, err := tx.AppendObservation(ctx, observation)
			return err
		}); err != nil {
			t.Fatalf("insert retention boundary observation %s: %v", observation.ID, err)
		}
	}
	if err := insertIntegrationNewsItem(ctx, db, boundaryNewsID); err != nil {
		t.Fatalf("insert retention boundary news item: %v", err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE news_observations SET news_id = ? WHERE observation_id = ?", boundaryNewsID, evidenceOnlyObservation.ID); err != nil {
		t.Fatalf("bind evidence-only observation: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_item_versions
		(version_id, news_id, version_no, fingerprint, title, content_json, observed_at, observation_id)
		VALUES (?, ?, 1, ?, ?, JSON_OBJECT(), ?, ?)`,
		boundaryVersionID, boundaryNewsID, strings.Repeat("c", 64), "retention boundary",
		evidenceOnlyObservation.ObservedAt, evidenceOnlyObservation.ID); err != nil {
		t.Fatalf("insert retention boundary version: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO news_version_availability_evidence
		(evidence_id, news_id, version_id, observation_id, field_path, raw_value,
		 source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		 candidate_available_at_utc, parser_version, evidence_ref, evidence_hash,
		 observed_at, recorded_at, retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?, ?)`,
		boundaryEvidenceID, boundaryNewsID, boundaryVersionID, evidenceOnlyObservation.ID,
		"payload.publishTime", boundaryCandidateAvailableAt.Format(time.RFC3339Nano), "UTC", "second", "parsed",
		boundaryCandidateAvailableAt, "test-parser/1", "evidence://retention-boundary",
		strings.Repeat("d", 64), evidenceOnlyObservation.ObservedAt,
		evidenceOnlyObservation.ObservedAt.Add(time.Second), boundary); err != nil {
		t.Fatalf("insert shorter-retention availability evidence: %v", err)
	}

	boundaryResult, err := repository.PurgeExpiredObservationPayloads(ctx, boundary, 10)
	if err != nil || boundaryResult.Scanned != 1 || boundaryResult.Purged != 1 ||
		boundaryResult.EvidenceScanned != 1 || boundaryResult.EvidencePurged != 1 {
		t.Fatalf("boundary/independent evidence purge result=%+v err=%v", boundaryResult, err)
	}
	var boundaryPayload, retainedPayload []byte
	var boundaryPurgedAt, retainedPurgedAt sql.NullTime
	if err := db.QueryRowContext(ctx,
		"SELECT candidate_json, payload_purged_at FROM news_observations WHERE observation_id = ?",
		boundaryObservation.ID,
	).Scan(&boundaryPayload, &boundaryPurgedAt); err != nil {
		t.Fatalf("read boundary observation after purge: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		"SELECT candidate_json, payload_purged_at FROM news_observations WHERE observation_id = ?",
		evidenceOnlyObservation.ID,
	).Scan(&retainedPayload, &retainedPurgedAt); err != nil {
		t.Fatalf("read retained parent observation after evidence purge: %v", err)
	}
	var boundaryCandidate, retainedCandidate domain.Candidate
	if err := json.Unmarshal(boundaryPayload, &boundaryCandidate); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(retainedPayload, &retainedCandidate); err != nil {
		t.Fatal(err)
	}
	if boundaryCandidate.Title != "" || !boundaryPurgedAt.Valid {
		t.Fatalf("observation at exact expiry was not purged: candidate=%+v purged_at=%+v", boundaryCandidate, boundaryPurgedAt)
	}
	if retainedCandidate.Title != evidenceOnlyObservation.Candidate.Title || retainedPurgedAt.Valid {
		t.Fatalf("parent Observation was purged before its own expiry: candidate=%+v purged_at=%+v", retainedCandidate, retainedPurgedAt)
	}
	var boundaryRawValue, boundaryEvidenceRef, boundaryEvidenceHash sql.NullString
	var boundaryEvidencePurgedAt sql.NullTime
	if err := db.QueryRowContext(ctx, `SELECT raw_value, evidence_ref, evidence_hash, payload_purged_at
		FROM news_version_availability_evidence WHERE evidence_id = ?`, boundaryEvidenceID,
	).Scan(&boundaryRawValue, &boundaryEvidenceRef, &boundaryEvidenceHash, &boundaryEvidencePurgedAt); err != nil {
		t.Fatalf("read independently purged availability evidence: %v", err)
	}
	if boundaryRawValue.Valid || boundaryEvidenceRef.Valid || boundaryEvidenceHash.Valid || !boundaryEvidencePurgedAt.Valid {
		t.Fatalf("shorter-retention Evidence payload remains: raw=%+v ref=%+v hash=%+v purged_at=%+v",
			boundaryRawValue, boundaryEvidenceRef, boundaryEvidenceHash, boundaryEvidencePurgedAt)
	}
}

type availabilityEvidenceSourceCatalogStub struct {
	registration app.SourceEndpointRegistration
}

func (s availabilityEvidenceSourceCatalogStub) LookupEndpoint(
	_ context.Context, sourceID, endpointID string,
) (app.SourceEndpointRegistration, error) {
	if sourceID != s.registration.SourceID || endpointID != s.registration.EndpointID {
		return app.SourceEndpointRegistration{}, app.ErrSourceEndpointNotFound
	}
	return s.registration, nil
}

func TestAvailabilityEvidenceWriteRequiresCurrentFieldPolicyMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News availability Evidence write integration")
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
		t.Fatal(err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "news", Migrations()); err != nil {
		t.Fatal(err)
	}
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sourceID := "available-write-source-" + suffix
	endpointID := "available-write-endpoint-" + suffix
	sourceV1 := "available-write-source-v1-" + suffix
	sourceV2 := "available-write-source-v2-" + suffix
	newsIDs := []string{"available-write-news-a-" + suffix, "available-write-news-b-" + suffix}
	observationIDs := []string{"available-write-observation-a-" + suffix, "available-write-observation-b-" + suffix}
	versionIDs := []string{"available-write-version-a-" + suffix, "available-write-version-b-" + suffix}
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_version_availability_decisions WHERE version_id IN (?, ?)", versionIDs[0], versionIDs[1])
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_version_availability_evidence WHERE version_id IN (?, ?)", versionIDs[0], versionIDs[1])
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_item_versions WHERE version_id IN (?, ?)", versionIDs[0], versionIDs[1])
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_observations WHERE observation_id IN (?, ?)", observationIDs[0], observationIDs[1])
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_items WHERE news_id IN (?, ?)", newsIDs[0], newsIDs[1])
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_current_policy_versions WHERE policy_type = ? AND policy_ref = ?", domain.NewsPolicyTypeSource, sourceID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_policy_versions WHERE policy_type = ? AND policy_ref = ?", domain.NewsPolicyTypeSource, sourceID)
	}()

	publisher, err := app.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		t.Fatal(err)
	}
	makePolicy := func(fields []string) []byte {
		payload, marshalErr := json.Marshal(domain.SourcePolicy{
			Status:                domain.AuthorizationApproved,
			AllowedAccessMethods:  []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes:       []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:   []domain.ContentKind{domain.ContentMetadata},
			AllowedStorageModes:   []domain.StorageMode{domain.StoragePersistentMetadata},
			AllowedMetadataFields: fields,
			MaxRetention:          2 * time.Hour,
			EvidenceRef:           "integration-source-policy-evidence",
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return payload
	}
	if err := publisher.Execute(ctx, domain.NewsPolicyTypeSource, sourceID, sourceV1, makePolicy([]string{"payload.publishTime"})); err != nil {
		t.Fatalf("publish initial field policy: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	insertParents := func(index int, policyVersion string) (domain.Observation, domain.NewsItemVersion) {
		t.Helper()
		observedAt := now.Add(-time.Minute)
		retentionUntil := now.Add(10 * time.Minute)
		if execErr := insertIntegrationNewsItem(ctx, db, newsIDs[index]); execErr != nil {
			t.Fatalf("insert news item: %v", execErr)
		}
		observation := domain.Observation{
			ID: observationIDs[index], IdempotencyKey: "available-write-idempotency-" + suffix + fmt.Sprint(index),
			SourceID: sourceID, EndpointID: endpointID, SourcePolicyVersionID: policyVersion,
			PolicyEvaluatedAt: now, Candidate: domain.Candidate{SourceItemID: "source-item"},
			ObservedAt: observedAt, RetentionUntil: retentionUntil,
			IdentityStatus: domain.ObservationMatched, NewsID: newsIDs[index],
		}
		candidateJSON, marshalErr := json.Marshal(observation.Candidate)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, execErr := db.ExecContext(ctx, `INSERT INTO news_observations
			(observation_id, idempotency_key, source_id, provider_code, query_dimension, endpoint_id,
			 source_policy_version_id, endpoint_policy_version_id, provider_output_policy_ref,
			 provider_output_policy_version_id, policy_evaluated_at, candidate_json, observed_at,
			 retention_until, payload_purged_at, identity_status, news_id, reason_code)
			VALUES (?, ?, ?, '', '', ?, ?, '', '', '', ?, ?, ?, ?, NULL, ?, ?, '')`,
			observation.ID, observation.IdempotencyKey, observation.SourceID, observation.EndpointID,
			observation.SourcePolicyVersionID, observation.PolicyEvaluatedAt, candidateJSON,
			observation.ObservedAt, observation.RetentionUntil, string(observation.IdentityStatus), observation.NewsID,
		); execErr != nil {
			t.Fatalf("insert observation: %v", execErr)
		}
		version := domain.NewsItemVersion{
			ID: versionIDs[index], NewsID: newsIDs[index], VersionNo: 1,
			Fingerprint: strings.Repeat("a", 64), Title: "availability write integration",
			Content: domain.ContentLayers{}, ObservedAt: observedAt, ObservationID: observation.ID,
		}
		contentJSON, marshalErr := json.Marshal(version.Content)
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		if _, execErr := db.ExecContext(ctx, `INSERT INTO news_item_versions
			(version_id, news_id, version_no, fingerprint, title, content_json, observed_at, observation_id)
			VALUES (?, ?, 1, ?, ?, ?, ?, ?)`,
			version.ID, version.NewsID, version.Fingerprint, version.Title, contentJSON,
			version.ObservedAt, version.ObservationID,
		); execErr != nil {
			t.Fatalf("insert version: %v", execErr)
		}
		return observation, version
	}

	observationA, versionA := insertParents(0, sourceV1)
	registration := app.SourceEndpointRegistration{
		SourceID: sourceID, EndpointID: endpointID, Enabled: true, AccessMethod: domain.AccessRSS,
		SourcePolicyVersionID: sourceV1,
	}
	usecase, err := app.NewRecordAvailabilityEvidence(repository,
		availabilityEvidenceSourceCatalogStub{registration: registration}, nil,
		func() string { return "available-write-evidence-" + suffix })
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.ParseAvailabilityTime(observationA.ObservedAt.Format(time.RFC3339))
	evidence, err := usecase.Execute(ctx, app.RecordAvailabilityEvidenceRequest{
		Observation: observationA, Version: versionA, FieldPath: "payload.publishTime",
		Candidate: candidate, ParserVersion: "integration-parser/1",
	})
	if err != nil {
		t.Fatalf("append authorized Evidence: %v", err)
	}
	if evidence.RetentionUntil.After(observationA.RetentionUntil) {
		t.Fatalf("Evidence retention exceeds parent Observation: evidence=%s observation=%s",
			evidence.RetentionUntil, observationA.RetentionUntil)
	}
	var storedFieldPath, storedRaw string
	var storedRetention time.Time
	if err := db.QueryRowContext(ctx, `SELECT field_path, raw_value, retention_until
		FROM news_version_availability_evidence WHERE evidence_id = ?`, evidence.ID,
	).Scan(&storedFieldPath, &storedRaw, &storedRetention); err != nil {
		t.Fatalf("read appended Evidence: %v", err)
	}
	if storedFieldPath != "payload.publishTime" || storedRaw != candidate.RawValue ||
		storedRetention.After(observationA.RetentionUntil) {
		t.Fatalf("stored Evidence mismatch: field=%q raw=%q retention=%s", storedFieldPath, storedRaw, storedRetention)
	}

	if err := publisher.Execute(ctx, domain.NewsPolicyTypeSource, sourceID, sourceV2, makePolicy([]string{"payload.title"})); err != nil {
		t.Fatalf("publish tightened field policy: %v", err)
	}
	if _, err := usecase.Execute(ctx, app.RecordAvailabilityEvidenceRequest{
		Observation: observationA, Version: versionA, FieldPath: "payload.publishTime",
		Candidate: candidate, ParserVersion: "integration-parser/1",
	}); !errors.Is(err, app.ErrAvailabilityEvidencePolicyUnavailable) {
		t.Fatalf("stale Observation policy error = %v, want policy unavailable", err)
	}

	observationB, versionB := insertParents(1, sourceV2)
	registration.SourcePolicyVersionID = sourceV2
	deniedUsecase, err := app.NewRecordAvailabilityEvidence(repository,
		availabilityEvidenceSourceCatalogStub{registration: registration}, nil,
		func() string { return "must-not-write-" + suffix })
	if err != nil {
		t.Fatal(err)
	}
	_, err = deniedUsecase.Execute(ctx, app.RecordAvailabilityEvidenceRequest{
		Observation: observationB, Version: versionB, FieldPath: "payload.publishTime",
		Candidate:     domain.ParseAvailabilityTime(observationB.ObservedAt.Format(time.RFC3339)),
		ParserVersion: "integration-parser/1",
	})
	if !errors.Is(err, app.ErrAvailabilityEvidencePolicyDenied) {
		t.Fatalf("field not listed in current policy error = %v, want policy denied", err)
	}
	var deniedCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_version_availability_evidence WHERE version_id = ?", versionB.ID).Scan(&deniedCount); err != nil {
		t.Fatal(err)
	}
	if deniedCount != 0 {
		t.Fatalf("unauthorized field Evidence rows = %d, want 0", deniedCount)
	}
}

func TestResolveCurrentSourceCollectionPoliciesMariaDBIntegration(t *testing.T) {
	dsn := os.Getenv("NEWS_MARIADB_TEST_DSN")
	if dsn == "" {
		t.Skip("set NEWS_MARIADB_TEST_DSN to run News source collection policy integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(4)
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		t.Fatal(err)
	}
	if err := platformstorage.ApplyMigrations(ctx, db, "news", Migrations()); err != nil {
		t.Fatal(err)
	}
	repository, err := NewMySQLRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	publisher, err := app.NewPublishNewsPolicyVersion(repository)
	if err != nil {
		t.Fatal(err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	sourceRef := "collect-policy-source-" + suffix
	endpointRef := "collect-policy-endpoint-" + suffix
	sourceV1, sourceV2 := "source-v1-"+suffix, "source-v2-"+suffix
	endpointV1 := "endpoint-v1-" + suffix
	defer func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_current_policy_versions WHERE (policy_type = ? AND policy_ref = ?) OR (policy_type = ? AND policy_ref = ?)",
			domain.NewsPolicyTypeSource, sourceRef, domain.NewsPolicyTypeEndpoint, endpointRef)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM news_policy_versions WHERE (policy_type = ? AND policy_ref = ?) OR (policy_type = ? AND policy_ref = ?)",
			domain.NewsPolicyTypeSource, sourceRef, domain.NewsPolicyTypeEndpoint, endpointRef)
	}()
	makeSourcePolicy := func(status domain.AuthorizationStatus) []byte {
		payload, marshalErr := json.Marshal(domain.SourcePolicy{
			Status:               status,
			AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:  []domain.ContentKind{domain.ContentFullText},
			AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient},
			MaxRetention:         time.Hour,
			EvidenceRef:          "integration-source-license",
		})
		if marshalErr != nil {
			t.Fatal(marshalErr)
		}
		return payload
	}
	if err := publisher.Execute(ctx, domain.NewsPolicyTypeSource, sourceRef, sourceV1, makeSourcePolicy(domain.AuthorizationApproved)); err != nil {
		t.Fatalf("publish source policy v1: %v", err)
	}
	source, endpoint, err := repository.ResolveCurrentSourceCollectionPolicies(ctx, sourceRef, sourceV1, endpointRef, "")
	if err != nil || source.Status != domain.AuthorizationApproved || endpoint != nil {
		t.Fatalf("resolve source policy v1: source=%+v endpoint=%+v err=%v", source, endpoint, err)
	}
	if err := publisher.Execute(ctx, domain.NewsPolicyTypeSource, sourceRef, sourceV2, makeSourcePolicy(domain.AuthorizationRevoked)); err != nil {
		t.Fatalf("publish source policy v2: %v", err)
	}
	if _, _, err := repository.ResolveCurrentSourceCollectionPolicies(ctx, sourceRef, sourceV1, endpointRef, ""); !errors.Is(err, app.ErrSourceCollectionPolicyUnavailable) {
		t.Fatalf("stale source version error=%v, want unavailable", err)
	}
	source, endpoint, err = repository.ResolveCurrentSourceCollectionPolicies(ctx, sourceRef, sourceV2, endpointRef, "")
	if err != nil || source.Status != domain.AuthorizationRevoked || endpoint != nil {
		t.Fatalf("resolve current revoked source policy: source=%+v endpoint=%+v err=%v", source, endpoint, err)
	}
	endpointPayload, err := json.Marshal(domain.EndpointPolicy{Status: domain.AuthorizationRevoked, EvidenceRef: "integration-endpoint-revocation"})
	if err != nil {
		t.Fatal(err)
	}
	if err := publisher.Execute(ctx, domain.NewsPolicyTypeEndpoint, endpointRef, endpointV1, endpointPayload); err != nil {
		t.Fatalf("publish endpoint policy: %v", err)
	}
	if _, _, err := repository.ResolveCurrentSourceCollectionPolicies(ctx, sourceRef, sourceV2, endpointRef, ""); !errors.Is(err, app.ErrSourceCollectionPolicyUnavailable) {
		t.Fatalf("unexpected endpoint policy with empty catalog version error=%v, want unavailable", err)
	}
	source, endpoint, err = repository.ResolveCurrentSourceCollectionPolicies(ctx, sourceRef, sourceV2, endpointRef, endpointV1)
	if err != nil || endpoint == nil || endpoint.Status != domain.AuthorizationRevoked || source.Status != domain.AuthorizationRevoked {
		t.Fatalf("resolve current source+endpoint policies: source=%+v endpoint=%+v err=%v", source, endpoint, err)
	}
}
