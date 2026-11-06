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

	relationService := app.NewApplyNewsItemRelation(repository, newID)
	relation, err := relationService.Execute(ctx, app.NewsItemRelationRequest{
		Relation: domain.NewsItemRelation{
			FromNewsID: newsID, FromSourceID: sourceID,
			ToNewsID: secondObservation.NewsID, ToSourceID: secondSourceID,
			Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationConfirmed,
			EvidenceObservation: observations[0].ID,
			FromVersionID:       latest.ID, ToVersionID: secondVersion.Version.ID,
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
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "golden", "NEWS-CTR-002-authorization-binding.golden.json")
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
