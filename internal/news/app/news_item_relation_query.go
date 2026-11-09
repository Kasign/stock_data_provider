package app

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

var ErrInvalidNewsItemRelationHistoryQuery = errors.New("news.item_relation.history_query_invalid")

// NewsItemRelationHistoryRequest selects append-only relation assertions visible
// at a knowledge-time boundary. It does not infer the current relation graph.
type NewsItemRelationHistoryRequest struct {
	NewsID     string
	SystemAsOf time.Time
}

func (r NewsItemRelationHistoryRequest) Validate(now time.Time) error {
	if !validEvidenceOpaqueRef(r.NewsID) ||
		!validEvidenceTime(r.SystemAsOf, now) {
		return ErrInvalidNewsItemRelationHistoryQuery
	}
	return nil
}

// NewsItemRelationHistoryRepository returns every assertion touching the item
// that was observed no later than SystemAsOf, including superseded assertions.
type NewsItemRelationHistoryRepository interface {
	ListNewsItemRelationsAt(context.Context, NewsItemRelationHistoryRequest) ([]domain.NewsItemRelation, error)
}
