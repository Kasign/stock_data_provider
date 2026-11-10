package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func sourcePersistenceRegistration() SourceEndpointRegistration {
	return SourceEndpointRegistration{
		SourceID: "source-feed", EndpointID: "endpoint-feed", Enabled: true,
		AccessMethod: domain.AccessRSS, URL: "https://feed.example/rss",
		SourcePolicyVersionID: "source-policy-v3",
		SourcePolicy: domain.SourcePolicy{
			Status:               domain.AuthorizationApproved,
			AllowedAccessMethods: []domain.AccessMethod{domain.AccessRSS},
			AllowedPurposes:      []domain.Purpose{domain.PurposeInternalPersonalResearch},
			AllowedContentKinds:  []domain.ContentKind{domain.ContentSnippet, domain.ContentMetadata},
			AllowedStorageModes:  []domain.StorageMode{domain.StorageTransient, domain.StoragePersistentMetadata},
			MaxRetention:         24 * time.Hour, EvidenceRef: "source-evidence",
		},
		URLPolicy: domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"feed.example"}},
	}
}

func sealedCollectedSourceResult() CollectSourceResult {
	result := CollectSourceResult{
		SourceID:              "source-feed",
		EndpointID:            "endpoint-feed",
		SourcePolicyVersionID: "source-policy-v3",
		PolicyEvaluatedAt:     time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}
	_ = sealCollectSourceResult(&result)
	return result
}

func TestIngestCollectedSourceRechecksVersionAndMetadataPersistencePolicy(t *testing.T) {
	registration := sourcePersistenceRegistration()
	repository := &memoryIdentityRepository{}
	ingest := newCandidateIngest(repository, func() string { return "observation-feed-1" })
	usecase, err := NewIngestCollectedSource(ingest, providerIngestCatalogStub{registration: registration})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	usecase.now = func() time.Time { return now }
	result := sealedCollectedSourceResult()
	result.Entries = []CollectedSourceEntry{{
		Candidate: domain.Candidate{SourceItemID: "item-1", Title: "title", ProviderSummary: "must-not-persist"},
		Content:   domain.ContentLayers{ProviderSummary: "must-not-persist"},
	}}
	if err := sealCollectSourceResult(&result); err != nil {
		t.Fatal(err)
	}
	observations, err := usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "collection-1",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 1 || len(repository.observations) != 1 {
		t.Fatalf("observations=%+v repository=%+v", observations, repository)
	}
	if repository.observations[0].RetentionUntil.IsZero() || repository.observations[0].RetentionUntil.Sub(repository.observations[0].ObservedAt) != time.Hour {
		t.Fatalf("actual requested retention was not persisted: %+v", repository.observations[0])
	}
	if repository.observations[0].Candidate.ProviderSummary != "" {
		t.Fatalf("provider summary leaked into persistent metadata: %+v", repository.observations[0].Candidate)
	}
	if repository.observations[0].SourcePolicyVersionID != "source-policy-v3" {
		t.Fatalf("policy trace mismatch: %+v", repository.observations[0])
	}
}

func TestIngestCollectedSourceFailsClosedOnPolicyVersionChange(t *testing.T) {
	registration := sourcePersistenceRegistration()
	registration.SourcePolicyVersionID = "source-policy-v4"
	repository := &memoryIdentityRepository{}
	ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
	usecase, err := NewIngestCollectedSource(ingest, providerIngestCatalogStub{registration: registration})
	if err != nil {
		t.Fatal(err)
	}
	result := sealedCollectedSourceResult()
	_, err = usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "collection-2",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if !errors.Is(err, ErrSourcePersistencePolicyUnavailable) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repository=%+v", err, repository)
	}
}

func TestIngestCollectedSourceRequiresSeparateMetadataPersistenceGrant(t *testing.T) {
	registration := sourcePersistenceRegistration()
	registration.SourcePolicy.AllowedStorageModes = []domain.StorageMode{domain.StorageTransient}
	repository := &memoryIdentityRepository{}
	ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
	usecase, err := NewIngestCollectedSource(ingest, providerIngestCatalogStub{registration: registration})
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC) }
	_, err = usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: sealedCollectedSourceResult(), IdempotencyPrefix: "collection-3",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if !errors.Is(err, ErrSourcePersistencePolicyDenied) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repository=%+v", err, repository)
	}
}

func TestIngestCollectedSourceRejectsTamperedEntries(t *testing.T) {
	repository := &memoryIdentityRepository{}
	ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
	usecase, err := NewIngestCollectedSource(ingest, providerIngestCatalogStub{registration: sourcePersistenceRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	result := sealedCollectedSourceResult()
	result.collectionSeal = [32]byte{}
	_, err = usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "collection-4",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if !errors.Is(err, ErrInvalidRequest) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repository=%+v", err, repository)
	}
}

func TestIngestCollectedSourceRejectsTamperedPolicyVersion(t *testing.T) {
	repository := &memoryIdentityRepository{}
	ingest := newCandidateIngest(repository, func() string { return "must-not-persist" })
	usecase, err := NewIngestCollectedSource(ingest, providerIngestCatalogStub{registration: sourcePersistenceRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	result := sealedCollectedSourceResult()
	result.SourcePolicyVersionID = "source-policy-v4"
	_, err = usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "collection-tampered",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if !errors.Is(err, ErrInvalidRequest) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repository=%+v", err, repository)
	}
}

func TestIngestCollectedSourceStopsAfterCatalogContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repository := &memoryIdentityRepository{}
	ingest := newCandidateIngest(repository, func() string { return "must-not-persist-after-cancel" })
	catalog := providerIngestCatalogStub{registration: sourcePersistenceRegistration(), afterLookup: cancel}
	usecase, err := NewIngestCollectedSource(ingest, catalog)
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.Execute(ctx, IngestCollectedSourceRequest{
		Collection: sealedCollectedSourceResult(), IdempotencyPrefix: "cancelled",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error=%v, want context.Canceled", err)
	}
	if len(repository.observations) != 0 || len(repository.items) != 0 {
		t.Fatalf("cancelled ingest persisted data: %+v", repository)
	}
}

type failNthIdentityTransactionRepository struct {
	inner  IdentityRepository
	calls  int
	failAt int
}

func (r *failNthIdentityTransactionRepository) WithinIdentityTransaction(ctx context.Context, fn func(IdentityTransaction) error) error {
	r.calls++
	if r.calls == r.failAt {
		return errors.New("injected identity transaction failure")
	}
	return r.inner.WithinIdentityTransaction(ctx, fn)
}

func TestIngestCollectedSourcePartialCommitCanBeSafelyRetried(t *testing.T) {
	repository := &memoryIdentityRepository{}
	failingRepository := &failNthIdentityTransactionRepository{
		inner:  repository,
		failAt: 2,
	}
	ingest := newCandidateIngest(failingRepository, func() string { return "observation-batch" })
	usecase, err := NewIngestCollectedSource(ingest, providerIngestCatalogStub{registration: sourcePersistenceRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	usecase.now = func() time.Time { return now }
	result := sealedCollectedSourceResult()
	result.Entries = []CollectedSourceEntry{
		{Candidate: domain.Candidate{SourceItemID: "item-1", Title: "first"}},
		{Candidate: domain.Candidate{SourceItemID: "item-2", Title: "second"}},
	}
	if err := sealCollectSourceResult(&result); err != nil {
		t.Fatal(err)
	}
	request := IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "collection-partial",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	}
	observations, err := usecase.Execute(context.Background(), request)
	if !errors.Is(err, ErrIdentityTransaction) {
		t.Fatalf("expected second item transaction failure, got %v", err)
	}
	if observations != nil || len(repository.observations) != 1 {
		t.Fatalf("expected one committed item and no success page, returned=%+v persisted=%+v", observations, repository.observations)
	}

	// 每条 Observation 独立事务；重试必须复用相同幂等前缀，已提交条目会重放既有记录。
	retryIngest := newCandidateIngest(repository, func() string { return "observation-batch-retry" })
	retryUsecase, err := NewIngestCollectedSource(retryIngest, providerIngestCatalogStub{registration: sourcePersistenceRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	retryUsecase.now = func() time.Time { return now }
	retried, err := retryUsecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(retried) != 2 || len(repository.observations) != 2 {
		t.Fatalf("retry did not converge the batch: returned=%+v persisted=%+v", retried, repository.observations)
	}
	if retried[0].ID != repository.observations[0].ID {
		t.Fatalf("retry did not reuse the first committed observation: returned=%+v persisted=%+v", retried, repository.observations)
	}
}
