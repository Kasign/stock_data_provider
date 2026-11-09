package app

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var ErrNewsItemRelationHistoryInvalid = errors.New("news.item_relation.history_invalid")

// ListNewsItemRelationHistory reads append-only assertions visible at a
// knowledge-time boundary. It preserves superseded and retracted assertions.
type ListNewsItemRelationHistory struct {
	repository NewsItemRelationHistoryRepository
	now        func() time.Time
}

func NewListNewsItemRelationHistory(repository NewsItemRelationHistoryRepository) (*ListNewsItemRelationHistory, error) {
	if repository == nil {
		return nil, ErrInvalidRequest
	}
	return &ListNewsItemRelationHistory{repository: repository, now: time.Now}, nil
}

func (u *ListNewsItemRelationHistory) Execute(
	ctx context.Context,
	request NewsItemRelationHistoryRequest,
) ([]domain.NewsItemRelation, error) {
	if u == nil || u.repository == nil || u.now == nil || ctx == nil ||
		request.Validate(u.now().UTC()) != nil {
		return nil, ErrInvalidNewsItemRelationHistoryQuery
	}
	relations, err := u.repository.ListNewsItemRelationsAt(ctx, request)
	if err != nil {
		// Storage failures must remain visible; never convert them into empty history.
		return nil, err
	}
	for i, relation := range relations {
		if (relation.FromNewsID != request.NewsID && relation.ToNewsID != request.NewsID) ||
			relation.ObservedAt.IsZero() || relation.ObservedAt.After(request.SystemAsOf) ||
			domain.ValidateNewsItemRelation(relation) != nil {
			return nil, ErrNewsItemRelationHistoryInvalid
		}
		if i > 0 {
			previous := relations[i-1]
			if relation.ObservedAt.Before(previous.ObservedAt) ||
				(relation.ObservedAt.Equal(previous.ObservedAt) && relation.ID <= previous.ID) {
				return nil, ErrNewsItemRelationHistoryInvalid
			}
		}
	}
	return relations, nil
}
