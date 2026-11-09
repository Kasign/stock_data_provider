package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type relationHistoryRepositoryStub struct {
	relations []domain.NewsItemRelation
	err       error
	request   NewsItemRelationHistoryRequest
	calls     int
}

func (s *relationHistoryRepositoryStub) ListNewsItemRelationsAt(
	_ context.Context, request NewsItemRelationHistoryRequest,
) ([]domain.NewsItemRelation, error) {
	s.calls++
	s.request = request
	return s.relations, s.err
}

func validRelationHistoryItem(id string, observedAt time.Time) domain.NewsItemRelation {
	return domain.NewsItemRelation{
		ID: id, FromNewsID: "news-1", FromSourceID: "source-1",
		ToNewsID: "news-2", ToSourceID: "source-2",
		Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationConfirmed,
		AssertedAt: observedAt, ObservedAt: observedAt,
	}
}

func TestListNewsItemRelationHistoryPreservesInclusiveBoundaryAndStableOrder(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	boundary := now.Add(-time.Minute)
	repository := &relationHistoryRepositoryStub{relations: []domain.NewsItemRelation{
		validRelationHistoryItem("relation-a", boundary),
		validRelationHistoryItem("relation-b", boundary),
	}}
	useCase, err := NewListNewsItemRelationHistory(repository)
	if err != nil {
		t.Fatal(err)
	}
	useCase.now = func() time.Time { return now }

	result, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{
		NewsID: "news-2", SystemAsOf: boundary,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result) != 2 || result[0].ID != "relation-a" || result[1].ID != "relation-b" {
		t.Fatalf("inclusive boundary/order result = %+v", result)
	}
	if repository.calls != 1 || repository.request.NewsID != "news-2" ||
		!repository.request.SystemAsOf.Equal(boundary) {
		t.Fatalf("repository request/calls = %+v/%d", repository.request, repository.calls)
	}
}

func TestListNewsItemRelationHistoryRejectsInvalidRepositoryResults(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	boundary := now.Add(-time.Minute)
	tests := []struct {
		name      string
		relations []domain.NewsItemRelation
	}{
		{"future assertion", []domain.NewsItemRelation{validRelationHistoryItem("relation-a", boundary.Add(time.Second))}},
		{"unrelated assertion", []domain.NewsItemRelation{func() domain.NewsItemRelation {
			item := validRelationHistoryItem("relation-a", boundary)
			item.FromNewsID, item.ToNewsID = "news-3", "news-4"
			return item
		}()}},
		{"unstable equal-time order", []domain.NewsItemRelation{
			validRelationHistoryItem("relation-b", boundary),
			validRelationHistoryItem("relation-a", boundary),
		}},
		{"descending time order", []domain.NewsItemRelation{
			validRelationHistoryItem("relation-a", boundary),
			validRelationHistoryItem("relation-b", boundary.Add(-time.Second)),
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &relationHistoryRepositoryStub{relations: test.relations}
			useCase, _ := NewListNewsItemRelationHistory(repository)
			useCase.now = func() time.Time { return now }
			_, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{
				NewsID: "news-1", SystemAsOf: boundary,
			})
			if !errors.Is(err, ErrNewsItemRelationHistoryInvalid) {
				t.Fatalf("Execute() error = %v, want invalid history", err)
			}
		})
	}
}

func TestListNewsItemRelationHistoryPropagatesStorageFailure(t *testing.T) {
	now := time.Now().UTC()
	storageErr := errors.New("database unavailable")
	repository := &relationHistoryRepositoryStub{err: storageErr}
	useCase, _ := NewListNewsItemRelationHistory(repository)
	useCase.now = func() time.Time { return now }

	_, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{
		NewsID: "news-1", SystemAsOf: now.Add(-time.Second),
	})
	if !errors.Is(err, storageErr) {
		t.Fatalf("Execute() error = %v, want original storage failure", err)
	}
}

func TestListNewsItemRelationHistoryRejectsInvalidRequestBeforeStorage(t *testing.T) {
	now := time.Now().UTC()
	repository := &relationHistoryRepositoryStub{}
	useCase, _ := NewListNewsItemRelationHistory(repository)
	useCase.now = func() time.Time { return now }

	_, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{
		NewsID: "news-1", SystemAsOf: now.Add(time.Second),
	})
	if !errors.Is(err, ErrInvalidNewsItemRelationHistoryQuery) || repository.calls != 0 {
		t.Fatalf("Execute() error/calls = %v/%d", err, repository.calls)
	}
}
