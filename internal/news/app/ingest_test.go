package app

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type memoryIdentityRepository struct {
	credentials  []domain.IdentityCredential
	observations []domain.Observation
	items        []string
	failAppend   bool
}

type memoryIdentityTransaction struct {
	repository *memoryIdentityRepository
}

func (r *memoryIdentityRepository) WithinIdentityTransaction(ctx context.Context, fn func(IdentityTransaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	working := *r
	working.credentials = append([]domain.IdentityCredential(nil), r.credentials...)
	working.observations = append([]domain.Observation(nil), r.observations...)
	working.items = append([]string(nil), r.items...)
	if err := fn(memoryIdentityTransaction{repository: &working}); err != nil {
		return err
	}
	*r = working
	return nil
}

func (tx memoryIdentityTransaction) FindIdentityCredentials(_ context.Context, sourceID string, candidates []domain.IdentityCredential) ([]domain.IdentityCredential, error) {
	var found []domain.IdentityCredential
	for _, stored := range tx.repository.credentials {
		for _, candidate := range candidates {
			if stored.SourceID == sourceID && stored.Type == candidate.Type && stored.Hash == candidate.Hash {
				found = append(found, stored)
			}
		}
	}
	return found, nil
}

func (tx memoryIdentityTransaction) CreateNewsItem(_ context.Context, sourceID string) (string, error) {
	id := "news-" + sourceID + "-" + string(rune('1'+len(tx.repository.items)))
	tx.repository.items = append(tx.repository.items, id)
	return id, nil
}

func (tx memoryIdentityTransaction) SaveIdentityCredentials(_ context.Context, credentials []domain.IdentityCredential) error {
	tx.repository.credentials = append(tx.repository.credentials, credentials...)
	return nil
}

func (tx memoryIdentityTransaction) AppendObservation(_ context.Context, observation domain.Observation) error {
	if tx.repository.failAppend {
		return errors.New("append failed")
	}
	tx.repository.observations = append(tx.repository.observations, observation)
	return nil
}

func TestIngestCandidateCreatesNewsItemAndObservation(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := NewIngestCandidate(repository, func() string { return "observation-1" })
	observation, err := usecase.Execute(context.Background(), "source-1", domain.Candidate{
		ProviderCode: "provider-a", ProviderItemID: "item-1", Title: " headline ",
	}, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationNew || observation.NewsID == "" {
		t.Fatalf("observation=%+v", observation)
	}
	if len(repository.items) != 1 || len(repository.credentials) != 1 || len(repository.observations) != 1 {
		t.Fatalf("repository=%+v", repository)
	}
}

func TestIngestCandidateMatchesExistingItemAndAddsOnlyNewAlias(t *testing.T) {
	existing := domain.IdentityCredential{
		SourceID: "source-1", Type: domain.IdentifierSourceNativeID,
		Value: "item-1", Hash: identityHashForTest("item-1"), Status: domain.IdentifierActive, NewsID: "news-existing",
	}
	repository := &memoryIdentityRepository{credentials: []domain.IdentityCredential{existing}}
	usecase := NewIngestCandidate(repository, func() string { return "observation-2" })
	observation, err := usecase.Execute(context.Background(), "source-1", domain.Candidate{
		ProviderItemID: "item-1", RawURL: "https://news.example/new-url",
	}, domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationMatched || observation.NewsID != "news-existing" {
		t.Fatalf("observation=%+v", observation)
	}
	if len(repository.items) != 0 || len(repository.credentials) != 2 {
		t.Fatalf("repository=%+v", repository)
	}
}

func TestIngestCandidateDoesNotCreateItemWithoutIdentity(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := NewIngestCandidate(repository, func() string { return "observation-3" })
	observation, err := usecase.Execute(context.Background(), "source-1", domain.Candidate{Title: "unidentified"}, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationUnidentified || len(repository.items) != 0 || len(repository.observations) != 1 {
		t.Fatalf("observation=%+v repository=%+v", observation, repository)
	}
}

func TestIngestCandidateConflictOnlyAppendsObservation(t *testing.T) {
	native := domain.IdentityCredential{
		SourceID: "source-1", Type: domain.IdentifierSourceNativeID,
		Value: "item-1", Hash: identityHashForTest("item-1"), Status: domain.IdentifierActive, NewsID: "news-1",
	}
	url := domain.IdentityCredential{
		SourceID: "source-1", Type: domain.IdentifierNormalizedURL,
		Value: "https://news.example/item", Hash: identityHashForTest("https://news.example/item"),
		Status: domain.IdentifierHistorical, NewsID: "news-2",
	}
	repository := &memoryIdentityRepository{credentials: []domain.IdentityCredential{native, url}}
	usecase := NewIngestCandidate(repository, func() string { return "observation-4" })
	observation, err := usecase.Execute(context.Background(), "source-1", domain.Candidate{
		ProviderItemID: "item-1", RawURL: "https://news.example/item",
	}, domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationConflict || observation.ReasonCode != "news.identity.multiple_items" {
		t.Fatalf("observation=%+v", observation)
	}
	if len(repository.items) != 0 || len(repository.observations) != 1 {
		t.Fatalf("repository=%+v", repository)
	}
}

func identityHashForTest(value string) string {
	credential, _ := domain.BuildIdentityCredentials("source-1", value, "", domain.SourceURLPolicy{})
	return credential[0].Hash
}

func TestIngestCandidateRollsBackWhenObservationAppendFails(t *testing.T) {
	repository := &memoryIdentityRepository{failAppend: true}
	usecase := NewIngestCandidate(repository, func() string { return "observation-5" })
	_, err := usecase.Execute(context.Background(), "source-1", domain.Candidate{ProviderItemID: "item-1"}, domain.SourceURLPolicy{})
	if !errors.Is(err, ErrIdentityTransaction) {
		t.Fatalf("err=%v", err)
	}
	if len(repository.items) != 0 || len(repository.credentials) != 0 || len(repository.observations) != 0 {
		t.Fatalf("transaction was not rolled back: %+v", repository)
	}
}
