package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

// ListNewsItemRelationsAt returns the append-only relation assertion history
// visible at the requested knowledge time. Superseded and retracted assertions
// are intentionally retained; callers must not mistake this for a current graph.
func (r *MySQLRepository) ListNewsItemRelationsAt(
	ctx context.Context,
	request app.NewsItemRelationHistoryRequest,
) ([]domain.NewsItemRelation, error) {
	now := time.Now().UTC()
	if r == nil || r.db == nil || ctx == nil || request.Validate(now) != nil {
		return nil, app.ErrInvalidNewsItemRelationHistoryQuery
	}

	rows, err := r.db.QueryContext(ctx, `SELECT
		item_relation_id, from_news_id, from_source_id, to_news_id, to_source_id,
		relation_kind, status, supersedes_relation_id, evidence_observation_id,
		from_version_id, to_version_id, asserted_at, observed_at
		FROM news_item_relations
		WHERE (from_news_id = ? OR to_news_id = ?) AND observed_at <= ?
		ORDER BY observed_at ASC, item_relation_id ASC`,
		request.NewsID, request.NewsID, request.SystemAsOf.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	relations := make([]domain.NewsItemRelation, 0)
	for rows.Next() {
		var relation domain.NewsItemRelation
		var supersedes, evidence, fromVersion, toVersion sql.NullString
		if err := rows.Scan(
			&relation.ID, &relation.FromNewsID, &relation.FromSourceID,
			&relation.ToNewsID, &relation.ToSourceID, &relation.Kind, &relation.Status,
			&supersedes, &evidence, &fromVersion, &toVersion,
			&relation.AssertedAt, &relation.ObservedAt,
		); err != nil {
			return nil, err
		}
		if supersedes.Valid {
			relation.SupersedesRelationID = supersedes.String
		}
		if evidence.Valid {
			relation.EvidenceObservation = evidence.String
		}
		if fromVersion.Valid {
			relation.FromVersionID = fromVersion.String
		}
		if toVersion.Valid {
			relation.ToVersionID = toVersion.String
		}
		relation.AssertedAt = relation.AssertedAt.UTC()
		relation.ObservedAt = relation.ObservedAt.UTC()
		if err := domain.ValidateNewsItemRelation(relation); err != nil {
			return nil, errors.Join(app.ErrNewsItemRelationHistory, err)
		}
		relations = append(relations, relation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return relations, nil
}
