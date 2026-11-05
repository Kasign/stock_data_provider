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

func (tx memoryIdentityTransaction) AppendObservation(_ context.Context, observation domain.Observation) (domain.Observation, error) {
	if tx.repository.failAppend {
		return domain.Observation{}, errors.New("append failed")
	}
	for _, existing := range tx.repository.observations {
		if existing.IdempotencyKey != observation.IdempotencyKey {
			continue
		}
		if !SameObservationPayload(existing, observation) {
			return domain.Observation{}, ErrObservationIdempotencyConflict
		}
		return existing, nil
	}
	tx.repository.observations = append(tx.repository.observations, observation)
	return observation, nil
}

func TestIngestCandidateCreatesNewsItemAndObservation(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := NewIngestCandidate(repository, func() string { return "observation-1" })
	observation, err := usecase.Execute(context.Background(), "source-1", "run-1/provider-a/item-1", domain.Candidate{
		ProviderCode: "provider-a", ProviderItemID: "provider-result-1", SourceItemID: "item-1", Title: " headline ",
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
	observation, err := usecase.Execute(context.Background(), "source-1", "run-1/provider-a/item-1", domain.Candidate{
		ProviderItemID: "provider-result-2", SourceItemID: "item-1", RawURL: "https://news.example/new-url",
	}, domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationMatched || observation.NewsID != "news-existing" {
		t.Fatalf("observation=%+v", observation)
	}
	if observation.Candidate.CanonicalURL != "https://news.example/new-url" {
		t.Fatalf("canonical URL was not retained on the observation candidate: %+v", observation.Candidate)
	}
	if len(repository.items) != 0 || len(repository.credentials) != 2 {
		t.Fatalf("repository=%+v", repository)
	}
}

func TestIngestCandidateDoesNotTrustCallerCanonicalURL(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := NewIngestCandidate(repository, func() string { return "observation-untrusted-url" })
	observation, err := usecase.Execute(context.Background(), "source-1", "run-1/untrusted-url", domain.Candidate{
		SourceItemID: "item-1",
		RawURL:       "https://untrusted.example/article",
		CanonicalURL: "https://news.example/forged-canonical",
	}, domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}})
	if err != nil {
		t.Fatal(err)
	}
	if observation.Candidate.CanonicalURL != "" {
		t.Fatalf("caller-supplied canonical URL must not survive policy rejection: %+v", observation.Candidate)
	}
	if observation.IdentityStatus != domain.ObservationConflict {
		t.Fatalf("identity status=%s, want conflict", observation.IdentityStatus)
	}
}

func TestIngestCandidateDoesNotCreateItemWithoutIdentity(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := NewIngestCandidate(repository, func() string { return "observation-3" })
	observation, err := usecase.Execute(context.Background(), "source-1", "run-1/provider-a/unknown-1", domain.Candidate{ProviderItemID: "provider-only-1", Title: "unidentified"}, domain.SourceURLPolicy{})
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
	observation, err := usecase.Execute(context.Background(), "source-1", "run-1/provider-a/item-1", domain.Candidate{
		ProviderItemID: "provider-result-3", SourceItemID: "item-1", RawURL: "https://news.example/item",
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
	_, err := usecase.Execute(context.Background(), "source-1", "run-1/provider-a/item-1", domain.Candidate{SourceItemID: "item-1"}, domain.SourceURLPolicy{})
	if !errors.Is(err, ErrIdentityTransaction) {
		t.Fatalf("err=%v", err)
	}
	if len(repository.items) != 0 || len(repository.credentials) != 0 || len(repository.observations) != 0 {
		t.Fatalf("transaction was not rolled back: %+v", repository)
	}
}

func TestIngestCandidateRetriesIdempotently(t *testing.T) {
	repository := &memoryIdentityRepository{}
	nextID := 0
	usecase := NewIngestCandidate(repository, func() string {
		nextID++
		return "observation-" + string(rune('0'+nextID))
	})
	candidate := domain.Candidate{ProviderCode: "provider-a", ProviderItemID: "provider-result-1", SourceItemID: "source-item-1", Title: "headline"}
	first, err := usecase.Execute(context.Background(), "source-1", "run-1/provider-a/item-1", candidate, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	second, err := usecase.Execute(context.Background(), "source-1", "run-1/provider-a/item-1", candidate, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || len(repository.observations) != 1 || len(repository.items) != 1 {
		t.Fatalf("first=%+v second=%+v repository=%+v", first, second, repository)
	}
}

func TestIngestCandidateRejectsIdempotencyKeyPayloadReuse(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := NewIngestCandidate(repository, func() string { return "observation-fixed" })
	_, err := usecase.Execute(context.Background(), "source-1", "same-key", domain.Candidate{ProviderItemID: "provider-result-1", SourceItemID: "source-item-1"}, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = usecase.Execute(context.Background(), "source-1", "same-key", domain.Candidate{ProviderItemID: "provider-result-2", SourceItemID: "source-item-2"}, domain.SourceURLPolicy{})
	if !errors.Is(err, ErrIdentityTransaction) || !errors.Is(err, ErrObservationIdempotencyConflict) {
		t.Fatalf("err=%v", err)
	}
	if len(repository.items) != 1 || len(repository.observations) != 1 {
		t.Fatalf("repository=%+v", repository)
	}
}
