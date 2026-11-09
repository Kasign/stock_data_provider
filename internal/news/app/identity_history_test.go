package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type identityHistoryRepositoryStub struct {
	credentials []domain.IdentityCredential
	err         error
	calls       int
	asOf        time.Time
}

func (s *identityHistoryRepositoryStub) ListIdentityCredentialsAt(
	_ context.Context, _ string, _ []domain.IdentityCredential, asOf time.Time,
) ([]domain.IdentityCredential, error) {
	s.calls++
	s.asOf = asOf
	return s.credentials, s.err
}

func TestResolveIdentityAtUsesOnlyCredentialsVisibleAtSystemAsOf(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Hour)
	candidate, err := domain.BuildIdentityCredentials("source-1", "native-1", "", domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	repository := &identityHistoryRepositoryStub{credentials: []domain.IdentityCredential{{
		SourceID: "source-1", Type: domain.IdentifierSourceNativeID, Value: "native-1",
		Hash: candidate[0].Hash, Status: domain.IdentifierActive, NewsID: "news-1", ObservedAt: observedAt,
	}}}
	useCase, err := NewResolveIdentityAt(repository)
	if err != nil {
		t.Fatal(err)
	}
	useCase.now = func() time.Time { return now }
	resolution, err := useCase.Execute(context.Background(), ResolveIdentityAtRequest{
		SourceID: "source-1", Credentials: candidate, SystemAsOf: observedAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("ResolveIdentityAt() error = %v", err)
	}
	if resolution.Status != domain.IdentityMatched || resolution.NewsID != "news-1" ||
		repository.calls != 1 || !repository.asOf.Equal(observedAt.Add(time.Minute)) {
		t.Fatalf("unexpected PIT identity result: resolution=%+v calls=%d asOf=%s", resolution, repository.calls, repository.asOf)
	}
}

func TestResolveIdentityAtRejectsFutureAndInvalidHistory(t *testing.T) {
	now := time.Now().UTC()
	candidate, err := domain.BuildIdentityCredentials("source-1", "native-1", "", domain.SourceURLPolicy{})
	if err != nil {
		t.Fatal(err)
	}
	repository := &identityHistoryRepositoryStub{}
	useCase, _ := NewResolveIdentityAt(repository)
	useCase.now = func() time.Time { return now }

	_, err = useCase.Execute(context.Background(), ResolveIdentityAtRequest{
		SourceID: "source-1", Credentials: candidate, SystemAsOf: now.Add(time.Second),
	})
	if !errors.Is(err, ErrInvalidRequest) || repository.calls != 0 {
		t.Fatalf("future system_as_of error=%v calls=%d", err, repository.calls)
	}

	repository.credentials = []domain.IdentityCredential{{
		SourceID: "source-1", Type: domain.IdentifierSourceNativeID, Value: "native-1",
		Hash: candidate[0].Hash, Status: domain.IdentifierActive, NewsID: "news-1",
		ObservedAt: now.Add(time.Second),
	}}
	_, err = useCase.Execute(context.Background(), ResolveIdentityAtRequest{
		SourceID: "source-1", Credentials: candidate, SystemAsOf: now.Add(-time.Second),
	})
	if !errors.Is(err, ErrIdentityHistoryInvalid) {
		t.Fatalf("future identity history error = %v", err)
	}
}
