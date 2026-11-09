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
	trace := app.ObservationPolicyTrace{
		SourcePolicyVersionID: "source-policy-v1",
		EndpointID:            "endpoint-v1", EndpointPolicyVersionID: "endpoint-policy-v1",
		ProviderOutputPolicyRef:       "provider-policy-ref",
		ProviderOutputPolicyVersionID: "provider-policy-v1",
		PolicyEvaluatedAt:             time.Now().UTC(),
	}
	if _, err := ingest.ExecuteWithPolicyTrace(ctx, traceSourceID, "provider-trace-key", traceCandidate, domain.SourceURLPolicy{}, trace); !errors.Is(err, app.ErrProviderOutputPolicyRequired) {
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

	proposal, err := relationService.Execute(ctx, app.NewsItemRelationRequest{
		Relation: domain.NewsItemRelation{
			FromNewsID: newsID, FromSourceID: sourceID,
			ToNewsID: secondObservation.NewsID, ToSourceID: secondSourceID,
			Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationProposed,
			SupersedesRelationID: relation.ID,
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
	if len(history) != 2 || history[0].ID != relation.ID || history[1].ID != proposal.ID {
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
	if len(history) != 3 ||
		history[0].ID != relation.ID ||
		history[1].ID != proposal.ID ||
		history[2].ID != retraction.ID {
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
			ID: fmt.Sprintf("news-retention-it-%d-%d", time.Now().UnixNano(), i),
			IdempotencyKey: fmt.Sprintf("news-retention-key-%d-%d", time.Now().UnixNano(), i),
			SourceID: "news-retention-integration",
			Candidate: domain.Candidate{SourceItemID: fmt.Sprintf("retention-source-item-%d", i), Title: "must be purged"},
			ObservedAt: now.Add(-2 * time.Hour), RetentionUntil: now.Add(-time.Hour),
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

	// 两个并发清理调用处理同一批到期记录时，不得重复清理或遗漏。
	concurrent := make([]domain.Observation, 4)
	for i := range concurrent {
		concurrent[i] = domain.Observation{
			ID: fmt.Sprintf("news-retention-concurrent-%d-%d", time.Now().UnixNano(), i),
			IdempotencyKey: fmt.Sprintf("news-retention-concurrent-key-%d-%d", time.Now().UnixNano(), i),
			SourceID: "news-retention-integration",
			Candidate: domain.Candidate{SourceItemID: fmt.Sprintf("retention-concurrent-item-%d", i), Title: "concurrent purge"},
			ObservedAt: now.Add(-2 * time.Hour), RetentionUntil: now.Add(-time.Hour),
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
		err error
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
