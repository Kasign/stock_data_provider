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
	if err := repository.AppendAuthorizationSnapshot(ctx, binding); err != nil {
		t.Fatalf("append authorization snapshot: %v", err)
	}
	if err := repository.AppendAuthorizationSnapshot(ctx, binding); err != nil {
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
	conflictingBinding := binding
	conflictingBinding.AuthorizationRef.DecisionFingerprint = strings.Repeat("f", 64)
	if err := repository.AppendAuthorizationSnapshot(ctx, conflictingBinding); !errors.Is(err, app.ErrAuthorizationSnapshotConflict) {
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

	snapshotCreatedAt := time.Now().UTC().Truncate(time.Microsecond)
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
	var idSequence atomic.Uint64
	newID := func() string { return fmt.Sprintf("news-it-%d", idSequence.Add(1)) }
	ingest := app.NewIngestCandidate(repository, newID)
	sourceID := fmt.Sprintf("news-it-source-%d", time.Now().UnixNano())
	const workers = 6

	candidate := domain.Candidate{
		ProviderCode:   "integration",
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
			observation, err := ingest.Execute(
				ctx, sourceID, fmt.Sprintf("news-it-key-%d", index), candidate, domain.SourceURLPolicy{},
			)
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
		t.Fatalf("concurrent identity ingestion failed: %v", err)
	}

	newsID := observations[0].NewsID
	if newsID == "" {
		t.Fatal("identity ingestion did not create a NewsID")
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
		t.Fatalf("concurrent first identity created %d News items, want 1", itemCount)
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
		ProviderCode: "integration", SourceItemID: "native-item-002",
		Title: "Second news item", PublisherName: "Second Publisher",
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
		pitRecord.ProviderRef != "integration" {
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
	listed, err := repository.ListEvidenceVersionsAt(ctx, listRequest)
	if err != nil {
		t.Fatalf("list evidence at system_as_of: %v", err)
	}
	if len(listed) != 1 || listed[0].Version.ID != secondVersion.Version.ID ||
		listed[0].Observation.ID != secondObservation.ID || listed[0].SourceRef != secondSourceID {
		t.Fatalf("ListEvidence PIT returned unexpected records: %+v", listed)
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
