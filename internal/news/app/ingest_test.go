package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type memoryIdentityRepository struct {
	credentials                   []domain.IdentityCredential
	observations                  []domain.Observation
	availabilityEvidence          []domain.AvailabilityEvidence
	items                         []string
	newsItems                     []domain.NewsItem
	versions                      []domain.NewsItemVersion
	failAppend                    bool
	sourcePolicy                  domain.SourcePolicy
	sourcePolicyVersionID         string
	endpointPolicy                *domain.EndpointPolicy
	endpointPolicyVersionID       string
	providerOutputPolicy          domain.ProviderOutputPolicy
	providerOutputPolicyRef       string
	providerOutputPolicyVersionID string
}

type memoryIdentityTransaction struct{ repository *memoryIdentityRepository }

func (r *memoryIdentityRepository) WithinIdentityTransaction(ctx context.Context, fn func(IdentityTransaction) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	working := *r
	working.credentials = append([]domain.IdentityCredential(nil), r.credentials...)
	working.observations = append([]domain.Observation(nil), r.observations...)
	working.availabilityEvidence = append([]domain.AvailabilityEvidence(nil), r.availabilityEvidence...)
	working.items = append([]string(nil), r.items...)
	working.newsItems = append([]domain.NewsItem(nil), r.newsItems...)
	working.versions = append([]domain.NewsItemVersion(nil), r.versions...)
	if err := fn(memoryIdentityTransaction{repository: &working}); err != nil {
		return err
	}
	*r = working
	return nil
}

func (tx memoryIdentityTransaction) LockCurrentIngestPolicies(
	_ context.Context, sourceID, sourceVersionID, endpointID, endpointVersionID, providerRef, providerVersion string,
) (domain.SourcePolicy, *domain.EndpointPolicy, *domain.ProviderOutputPolicy, error) {
	r := tx.repository
	if r == nil || sourceID == "" || endpointID == "" ||
		r.sourcePolicyVersionID != sourceVersionID || r.endpointPolicyVersionID != endpointVersionID ||
		r.providerOutputPolicyRef != providerRef || r.providerOutputPolicyVersionID != providerVersion {
		return domain.SourcePolicy{}, nil, nil, ErrSourcePersistencePolicyUnavailable
	}
	if providerRef == "" {
		return r.sourcePolicy, r.endpointPolicy, nil, nil
	}
	p := r.providerOutputPolicy
	return r.sourcePolicy, r.endpointPolicy, &p, nil
}

func (tx memoryIdentityTransaction) FindNewsItemByCanonicalURL(_ context.Context, canonicalURL, hash string) (domain.NewsItem, bool, error) {
	for _, item := range tx.repository.newsItems {
		if item.CanonicalURLHash != hash {
			continue
		}
		if item.CanonicalURL != canonicalURL {
			return domain.NewsItem{}, false, ErrCanonicalURLHashCollision
		}
		return item, true, nil
	}
	return domain.NewsItem{}, false, nil
}

func (tx memoryIdentityTransaction) FindNewsItemByID(_ context.Context, id string) (domain.NewsItem, bool, error) {
	for _, item := range tx.repository.newsItems {
		if item.ID == id {
			return item, true, nil
		}
	}
	return domain.NewsItem{}, false, nil
}

func (tx memoryIdentityTransaction) FindIdentityCredentials(_ context.Context, sourceID string, candidates []domain.IdentityCredential) ([]domain.IdentityCredential, error) {
	var found []domain.IdentityCredential
	for _, stored := range tx.repository.credentials {
		for _, candidate := range candidates {
			if stored.SourceID == sourceID && stored.Hash == candidate.Hash {
				found = append(found, stored)
			}
		}
	}
	return found, nil
}

func (tx memoryIdentityTransaction) CreateNewsItem(_ context.Context, item domain.NewsItem) error {
	tx.repository.items = append(tx.repository.items, item.ID)
	tx.repository.newsItems = append(tx.repository.newsItems, item)
	return nil
}

func (tx memoryIdentityTransaction) UpdateNewsItem(_ context.Context, item domain.NewsItem) error {
	for i := range tx.repository.newsItems {
		if tx.repository.newsItems[i].ID == item.ID {
			tx.repository.newsItems[i] = item
			return nil
		}
	}
	return errors.New("news item not found")
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
		if existing.PayloadPurgedAt != nil {
			return domain.Observation{}, ErrObservationPayloadPurged
		}
		if !SameObservationPayload(existing, observation) {
			return domain.Observation{}, ErrObservationIdempotencyConflict
		}
		return existing, nil
	}
	tx.repository.observations = append(tx.repository.observations, observation)
	return observation, nil
}

func (tx memoryIdentityTransaction) LatestNewsItemVersion(_ context.Context, newsID string) (domain.NewsItemVersion, bool, error) {
	var latest domain.NewsItemVersion
	found := false
	for _, version := range tx.repository.versions {
		if version.NewsID == newsID && (!found || version.VersionNo > latest.VersionNo) {
			latest, found = version, true
		}
	}
	return latest, found, nil
}

func (tx memoryIdentityTransaction) AppendNewsItemVersion(_ context.Context, version domain.NewsItemVersion) error {
	tx.repository.versions = append(tx.repository.versions, version)
	return nil
}

func (tx memoryIdentityTransaction) AppendAvailabilityEvidence(_ context.Context, evidence domain.AvailabilityEvidence, _ domain.Observation, _ domain.NewsItemVersion) error {
	if tx.repository == nil {
		return ErrAvailabilityEvidenceWriteUnavailable
	}
	tx.repository.availabilityEvidence = append(tx.repository.availabilityEvidence, evidence)
	return nil
}

func identityTrace() ObservationPolicyTrace {
	now := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	return ObservationPolicyTrace{SourcePolicyVersionID: "source-policy-v1", PolicyEvaluatedAt: now, Retention: time.Hour}
}

func identityURLPolicy() domain.SourceURLPolicy {
	return domain.SourceURLPolicy{AllowedSchemes: []string{"https"}, AllowedHosts: []string{"news.example"}}
}

func TestIngestCandidateRejectsProviderOutputWithoutPolicyTrace(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "observation-provider-policy-required" })
	_, err := usecase.executeUnscoped(context.Background(), "source-1", "provider-result-key",
		domain.Candidate{ProviderCode: "brave", SourceItemID: "item-1"}, domain.SourceURLPolicy{})
	if !errors.Is(err, ErrProviderOutputPolicyRequired) || len(repository.observations) != 0 {
		t.Fatalf("err=%v repo=%+v", err, repository)
	}
}

func TestIngestCandidateCreatesCurrentNewsAndInitialVersionAtomically(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "observation-1" })
	observation, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "run-1/item-1",
		domain.Candidate{SourceItemID: "item-1", Title: "headline"}, domain.SourceURLPolicy{}, identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationNew || observation.NewsID == "" {
		t.Fatalf("observation=%+v", observation)
	}
	if len(repository.newsItems) != 1 || len(repository.versions) != 1 || len(repository.credentials) != 1 || len(repository.observations) != 1 {
		t.Fatalf("repo=%+v", repository)
	}
	if repository.newsItems[0].Title != "headline" || repository.newsItems[0].ContentFingerprint == "" ||
		repository.versions[0].ObservationID != observation.ID || repository.versions[0].VersionNo != 1 {
		t.Fatalf("current item/version not persisted: items=%+v versions=%+v", repository.newsItems, repository.versions)
	}
}

func TestIngestCandidateUsesCanonicalURLAsGlobalIdentityWithoutNativeID(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "url-only" })
	observation, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "url-only-key",
		domain.Candidate{RawURL: "https://news.example/story", Title: "headline"}, identityURLPolicy(), identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationNew || len(repository.newsItems) != 1 || len(repository.credentials) != 0 {
		t.Fatalf("observation=%+v repo=%+v", observation, repository)
	}
	if repository.newsItems[0].CanonicalURL != "https://news.example/story" || repository.newsItems[0].CanonicalURLHash == "" {
		t.Fatalf("URL identity not persisted: %+v", repository.newsItems[0])
	}
}

func TestIngestCandidateReusesGlobalURLAcrossSources(t *testing.T) {
	repository := &memoryIdentityRepository{}
	next := 0
	usecase := newCandidateIngest(repository, func() string { next++; return "id-" + string(rune('0'+next)) })
	candidate := domain.Candidate{SourceItemID: "native-1", RawURL: "https://news.example/story", Title: "headline"}
	first, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "source-1-key", candidate, identityURLPolicy(), identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	second, err := usecase.executeWithPolicyTrace(context.Background(), "source-2", "source-2-key", candidate, identityURLPolicy(), identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	if first.NewsID != second.NewsID || len(repository.newsItems) != 1 || len(repository.credentials) != 2 || len(repository.versions) != 1 {
		t.Fatalf("first=%+v second=%+v repo=%+v", first, second, repository)
	}
}

func TestIngestCandidateRejectsDifferentURLForExistingNativeID(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "first" })
	first, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "first-key",
		domain.Candidate{SourceItemID: "native-1", RawURL: "https://news.example/one", Title: "one"}, identityURLPolicy(), identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	next := newCandidateIngest(repository, func() string { return "second" })
	second, err := next.executeWithPolicyTrace(context.Background(), "source-1", "second-key",
		domain.Candidate{SourceItemID: "native-1", RawURL: "https://news.example/two", Title: "two"}, identityURLPolicy(), identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	if second.IdentityStatus != domain.ObservationConflict || second.ReasonCode != "news.identity.canonical_url_mismatch" ||
		second.NewsID != "" || first.NewsID == "" || len(repository.newsItems) != 1 {
		t.Fatalf("first=%+v second=%+v repo=%+v", first, second, repository)
	}
}

func TestIngestCandidateRejectsURLAndNativeIDPointingToDifferentItems(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "first" })
	first, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "first-key",
		domain.Candidate{SourceItemID: "native-1", Title: "one"}, domain.SourceURLPolicy{}, identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	second, err := newCandidateIngest(repository, func() string { return "second" }).executeWithPolicyTrace(
		context.Background(), "source-2", "second-key",
		domain.Candidate{RawURL: "https://news.example/two", Title: "two"}, identityURLPolicy(), identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	if first.NewsID == second.NewsID {
		t.Fatalf("test setup must create distinct news IDs: %+v %+v", first, second)
	}

	// 将来源原生 ID 指向第一篇、URL 指向第二篇，验证身份冲突不会自动合并。
	credential, _ := domain.BuildIdentityCredentials("source-1", "native-1")
	repository.newsItems[1].CanonicalURL = "https://news.example/two"
	repository.newsItems[1].CanonicalURLHash = canonicalHashForTest("https://news.example/two")
	repository.credentials = append(repository.credentials, domain.IdentityCredential{
		SourceID: credential[0].SourceID, Value: credential[0].Value, Hash: credential[0].Hash, NewsID: first.NewsID,
	})
	conflict, err := newCandidateIngest(repository, func() string { return "third" }).executeWithPolicyTrace(
		context.Background(), "source-1", "third-key",
		domain.Candidate{SourceItemID: "native-1", RawURL: "https://news.example/two", Title: "two"},
		identityURLPolicy(), identityTrace())
	if err != nil {
		t.Fatal(err)
	}
	if conflict.IdentityStatus != domain.ObservationConflict || conflict.ReasonCode != "news.identity.multiple_items" {
		t.Fatalf("conflict=%+v", conflict)
	}
}

func TestIngestCandidateDoesNotCreateItemWithoutIdentity(t *testing.T) {
	repository := &memoryIdentityRepository{}
	observation, err := newCandidateIngest(repository, func() string { return "unidentified" }).executeUnscoped(
		context.Background(), "source-1", "unidentified-key",
		domain.Candidate{Title: "unidentified"}, domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	if observation.IdentityStatus != domain.ObservationUnidentified || len(repository.newsItems) != 0 || len(repository.observations) != 1 {
		t.Fatalf("observation=%+v repo=%+v", observation, repository)
	}
}

func TestIngestCandidateRollsBackWhenObservationAppendFails(t *testing.T) {
	repository := &memoryIdentityRepository{failAppend: true}
	_, err := newCandidateIngest(repository, func() string { return "rollback" }).executeWithPolicyTrace(
		context.Background(), "source-1", "rollback-key", domain.Candidate{SourceItemID: "item-1"},
		domain.SourceURLPolicy{}, identityTrace())
	if !errors.Is(err, ErrIdentityTransaction) {
		t.Fatalf("err=%v", err)
	}
	if len(repository.newsItems) != 0 || len(repository.credentials) != 0 || len(repository.observations) != 0 || len(repository.versions) != 0 {
		t.Fatalf("transaction was not rolled back: %+v", repository)
	}
}

func TestIngestCandidateIdempotencyAndContentVersioning(t *testing.T) {
	repository := &memoryIdentityRepository{}
	next := 0
	usecase := newCandidateIngest(repository, func() string { next++; return "generated-" + string(rune('0'+next)) })
	trace := identityTrace()
	firstCandidate := domain.Candidate{SourceItemID: "item-1", Title: "headline A"}
	first, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "version-a", firstCandidate, domain.SourceURLPolicy{}, trace)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "version-a", firstCandidate, domain.SourceURLPolicy{}, trace)
	if err != nil {
		t.Fatal(err)
	}
	if replay.ID != first.ID || len(repository.versions) != 1 || len(repository.observations) != 1 {
		t.Fatalf("idempotent replay changed state: first=%+v replay=%+v repo=%+v", first, replay, repository)
	}
	changed := firstCandidate
	changed.Title = "headline B"
	if _, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "version-b", changed, domain.SourceURLPolicy{}, trace); err != nil {
		t.Fatal(err)
	}
	if len(repository.versions) != 2 || repository.versions[1].VersionNo != 2 ||
		repository.newsItems[0].Title != "headline B" || repository.newsItems[0].ContentFingerprint != repository.versions[1].Fingerprint {
		t.Fatalf("content change did not append/update version: %+v", repository)
	}
}

func TestIngestCandidateRejectsChangedRetentionForSameIdempotencyKey(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "retention" })
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	usecase.now = func() time.Time { return base }
	trace := identityTrace()
	trace.PolicyEvaluatedAt = base
	if _, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "same-key", domain.Candidate{SourceItemID: "item-1"}, domain.SourceURLPolicy{}, trace); err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return base.Add(time.Minute) }
	trace.PolicyEvaluatedAt = base.Add(time.Minute)
	trace.Retention = 2 * time.Hour
	_, err := usecase.executeWithPolicyTrace(context.Background(), "source-1", "same-key", domain.Candidate{SourceItemID: "item-1"}, domain.SourceURLPolicy{}, trace)
	if !errors.Is(err, ErrIdentityTransaction) || !errors.Is(err, ErrObservationIdempotencyConflict) {
		t.Fatalf("changed retention must conflict, got %v", err)
	}
}

func TestIngestCandidateRejectsReplayAfterObservationPayloadPurge(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "purged" })
	candidate := domain.Candidate{SourceItemID: "item-1", Title: "private"}
	if _, err := usecase.executeUnscoped(context.Background(), "source-1", "purged-key", candidate, domain.SourceURLPolicy{}); err != nil {
		t.Fatal(err)
	}
	purgedAt := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	repository.observations[0].Candidate = domain.Candidate{}
	repository.observations[0].PayloadPurgedAt = &purgedAt
	_, err := usecase.executeUnscoped(context.Background(), "source-1", "purged-key", candidate, domain.SourceURLPolicy{})
	if !errors.Is(err, ErrIdentityTransaction) || !errors.Is(err, ErrObservationPayloadPurged) {
		t.Fatalf("err=%v", err)
	}
	if len(repository.versions) != 1 || repository.observations[0].PayloadPurgedAt == nil {
		t.Fatalf("purged payload restored: %+v", repository)
	}
}

func TestIngestCandidateRejectsIdempotencyKeyPayloadReuse(t *testing.T) {
	repository := &memoryIdentityRepository{}
	usecase := newCandidateIngest(repository, func() string { return "fixed" })
	if _, err := usecase.executeUnscoped(context.Background(), "source-1", "same-key", domain.Candidate{SourceItemID: "item-1"}, domain.SourceURLPolicy{}); err != nil {
		t.Fatal(err)
	}
	_, err := usecase.executeUnscoped(context.Background(), "source-1", "same-key", domain.Candidate{SourceItemID: "item-2"}, domain.SourceURLPolicy{})
	if !errors.Is(err, ErrIdentityTransaction) || !errors.Is(err, ErrObservationIdempotencyConflict) {
		t.Fatalf("err=%v", err)
	}
	if len(repository.newsItems) != 1 || len(repository.observations) != 1 {
		t.Fatalf("repo=%+v", repository)
	}
}

func TestIngestCandidatePropagatesCancellationAfterTransactionCommit(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	repository := &memoryIdentityRepository{}
	wrapped := cancelAfterIdentityTransactionRepository{inner: repository, cancel: cancel}
	_, err := newCandidateIngest(wrapped, func() string { return "cancel" }).executeUnscoped(
		ctx, "source-1", "cancel-key", domain.Candidate{SourceItemID: "item-1"}, domain.SourceURLPolicy{})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	if len(repository.observations) != 1 {
		t.Fatalf("committed observation lost: %+v", repository.observations)
	}
}

type cancelAfterIdentityTransactionRepository struct {
	inner  *memoryIdentityRepository
	cancel context.CancelFunc
}

func (r cancelAfterIdentityTransactionRepository) WithinIdentityTransaction(ctx context.Context, fn func(IdentityTransaction) error) error {
	err := r.inner.WithinIdentityTransaction(ctx, fn)
	r.cancel()
	return err
}

func canonicalHashForTest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
