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
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	relations, err := u.repository.ListNewsItemRelationsAt(ctx, request)
	if err != nil {
		// Storage failures must remain visible; never convert them into empty history.
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(relations) == 0 {
		return []domain.NewsItemRelation{}, nil
	}
	byID := make(map[string]domain.NewsItemRelation, len(relations))
	direct := make(map[string]struct{}, len(relations))
	for i, relation := range relations {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if relation.ObservedAt.IsZero() || relation.ObservedAt.After(request.SystemAsOf) ||
			domain.ValidateNewsItemRelation(relation) != nil {
			return nil, ErrNewsItemRelationHistoryInvalid
		}
		if _, exists := byID[relation.ID]; exists {
			return nil, ErrNewsItemRelationHistoryInvalid
		}
		byID[relation.ID] = relation
		if relation.FromNewsID == request.NewsID || relation.ToNewsID == request.NewsID {
			direct[relation.ID] = struct{}{}
		}
		if i > 0 {
			previous := relations[i-1]
			if relation.ObservedAt.Before(previous.ObservedAt) ||
				(relation.ObservedAt.Equal(previous.ObservedAt) && relation.ID <= previous.ID) {
				return nil, ErrNewsItemRelationHistoryInvalid
			}
		}
	}
	if len(direct) == 0 {
		return nil, ErrNewsItemRelationHistoryInvalid
	}
	// 保留直接命中断言的完整 supersedes 链；目标变更后，旧目标查询也必须看到后继。
	successors := make(map[string]string, len(relations))
	for _, relation := range relations {
		if relation.SupersedesRelationID != "" {
			successors[relation.SupersedesRelationID] = relation.ID
		}
	}
	included := make(map[string]struct{}, len(relations))
	for id := range direct {
		currentID := id
		for currentID != "" {
			if _, exists := included[currentID]; exists {
				break
			}
			included[currentID] = struct{}{}
			current, exists := byID[currentID]
			if !exists {
				return nil, ErrNewsItemRelationHistoryInvalid
			}
			if current.SupersedesRelationID != "" {
				if _, exists := byID[current.SupersedesRelationID]; !exists {
					return nil, ErrNewsItemRelationHistoryInvalid
				}
			}
			currentID = current.SupersedesRelationID
		}
		currentID = successors[id]
		for currentID != "" {
			if _, exists := included[currentID]; exists {
				break
			}
			included[currentID] = struct{}{}
			currentID = successors[currentID]
		}
	}
	if len(included) != len(relations) || domain.ValidateNewsItemRelationHistory(relations) != nil {
		return nil, ErrNewsItemRelationHistoryInvalid
	}
	return relations, nil
}
