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
	return CollectSourceResult{
		collectionSeal:        sourceCollectionResultSeal,
		SourceID:              "source-feed",
		EndpointID:            "endpoint-feed",
		SourcePolicyVersionID: "source-policy-v3",
		PolicyEvaluatedAt:     time.Date(2026, 11, 9, 9, 0, 0, 0, time.UTC),
	}
}

func TestIngestCollectedSourceRechecksVersionAndMetadataPersistencePolicy(t *testing.T) {
	registration := sourcePersistenceRegistration()
	repository := &memoryIdentityRepository{}
	ingest := NewIngestCandidate(repository, func() string { return "observation-feed-1" })
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
	ingest := NewIngestCandidate(repository, func() string { return "must-not-persist" })
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
	ingest := NewIngestCandidate(repository, func() string { return "must-not-persist" })
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

func TestIngestCollectedSourceRejectsUnsealedResult(t *testing.T) {
	repository := &memoryIdentityRepository{}
	ingest := NewIngestCandidate(repository, func() string { return "must-not-persist" })
	usecase, err := NewIngestCollectedSource(ingest, providerIngestCatalogStub{registration: sourcePersistenceRegistration()})
	if err != nil {
		t.Fatal(err)
	}
	result := sealedCollectedSourceResult()
	result.collectionSeal = ""
	_, err = usecase.Execute(context.Background(), IngestCollectedSourceRequest{
		Collection: result, IdempotencyPrefix: "collection-4",
		Purpose: domain.PurposeInternalPersonalResearch, Retention: time.Hour,
	})
	if !errors.Is(err, ErrInvalidRequest) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repository=%+v", err, repository)
	}
}
