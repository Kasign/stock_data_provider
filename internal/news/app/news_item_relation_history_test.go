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
	afterRead func()
}

func (s *relationHistoryRepositoryStub) ListNewsItemRelationsAt(
	_ context.Context, request NewsItemRelationHistoryRequest,
) ([]domain.NewsItemRelation, error) {
	s.calls++
	s.request = request
	if s.afterRead != nil {
		s.afterRead()
	}
	return s.relations, s.err
}

func validRelationHistoryItem(id string, observedAt time.Time) domain.NewsItemRelation {
	return domain.NewsItemRelation{
		ID: id, FromNewsID: "news-1", FromSourceID: "source-1",
		ToNewsID: "news-2", ToSourceID: "source-2",
		Kind: domain.NewsItemRelationRepostOf, Status: domain.NewsItemRelationConfirmed,
		FromVersionID: "version-from-" + id, ToVersionID: "version-to-" + id,
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

func TestListNewsItemRelationHistoryPreservesAppendOnlyStatuses(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	proposed := validRelationHistoryItem("relation-proposed", observedAt)
	proposed.Status = domain.NewsItemRelationProposed
	confirmed := validRelationHistoryItem("relation-confirmed", observedAt.Add(time.Second))
	retracted := validRelationHistoryItem("relation-retracted", observedAt.Add(2*time.Second))
	retracted.Status = domain.NewsItemRelationRetracted
	retracted.SupersedesRelationID = "relation-confirmed"

	repository := &relationHistoryRepositoryStub{relations: []domain.NewsItemRelation{
		proposed, confirmed, retracted,
	}}
	useCase, err := NewListNewsItemRelationHistory(repository)
	if err != nil {
		t.Fatal(err)
	}
	useCase.now = func() time.Time { return now }

	result, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{
		NewsID: "news-1", SystemAsOf: now,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("history length = %d, want all three append-only assertions", len(result))
	}
	want := []domain.NewsItemRelationStatus{
		domain.NewsItemRelationProposed,
		domain.NewsItemRelationConfirmed,
		domain.NewsItemRelationRetracted,
	}
	for i := range want {
		if result[i].Status != want[i] {
			t.Errorf("history[%d].Status = %q, want %q", i, result[i].Status, want[i])
		}
	}
	if result[2].SupersedesRelationID != "relation-confirmed" {
		t.Fatalf("retraction lost its supersedes link: %+v", result[2])
	}
}

func TestListNewsItemRelationHistoryPropagatesCancellationAfterRepositoryRead(t *testing.T) {
	now := time.Now().UTC()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	repository := &relationHistoryRepositoryStub{afterRead: cancel}
	useCase, _ := NewListNewsItemRelationHistory(repository)
	useCase.now = func() time.Time { return now }
	_, err := useCase.Execute(ctx, NewsItemRelationHistoryRequest{
		NewsID: "news-1", SystemAsOf: now.Add(-time.Second),
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
}

func TestListNewsItemRelationHistoryKeepsSupersededAncestorForTargetSideQuery(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	parent := validRelationHistoryItem("relation-parent", now.Add(-2*time.Minute))
	child := validRelationHistoryItem("relation-child", now.Add(-time.Minute))
	child.ToNewsID = "news-3"
	child.ToSourceID = "source-3"
	child.SupersedesRelationID = parent.ID
	repository := &relationHistoryRepositoryStub{relations: []domain.NewsItemRelation{parent, child}}
	useCase, err := NewListNewsItemRelationHistory(repository)
	if err != nil {
		t.Fatal(err)
	}
	useCase.now = func() time.Time { return now }
	result, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{NewsID: "news-3", SystemAsOf: now})
	if err != nil {
		t.Fatalf("target-side history with superseded ancestor: %v", err)
	}
	if len(result) != 2 || result[0].ID != parent.ID || result[1].ID != child.ID {
		t.Fatalf("history lost superseded ancestor: %+v", result)
	}
}

func TestListNewsItemRelationHistoryRejectsUnrelatedAncestorRows(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	direct := validRelationHistoryItem("relation-direct", now.Add(-time.Minute))
	unrelated := validRelationHistoryItem("relation-unrelated", now.Add(-2*time.Minute))
	unrelated.FromNewsID, unrelated.ToNewsID = "news-7", "news-8"
	unrelated.FromSourceID, unrelated.ToSourceID = "source-7", "source-8"
	repository := &relationHistoryRepositoryStub{relations: []domain.NewsItemRelation{unrelated, direct}}
	useCase, _ := NewListNewsItemRelationHistory(repository)
	useCase.now = func() time.Time { return now }
	_, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{NewsID: "news-1", SystemAsOf: now})
	if !errors.Is(err, ErrNewsItemRelationHistoryInvalid) {
		t.Fatalf("Execute() error = %v, want invalid history", err)
	}
}

func TestListNewsItemRelationHistoryReturnsEmptyForItemWithoutRelations(t *testing.T) {
	now := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	repository := &relationHistoryRepositoryStub{}
	useCase, _ := NewListNewsItemRelationHistory(repository)
	useCase.now = func() time.Time { return now }
	result, err := useCase.Execute(context.Background(), NewsItemRelationHistoryRequest{
		NewsID: "news-without-relations", SystemAsOf: now,
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil || len(result) != 0 {
		t.Fatalf("empty relation history = %#v, want non-nil empty slice", result)
	}
}
