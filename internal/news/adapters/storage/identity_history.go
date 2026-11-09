package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var _ app.IdentityHistoryRepository = (*MySQLRepository)(nil)

// ListIdentityCredentialsAt returns the latest credential status event visible
// at systemAsOf. Credentials created later or without a visible history event
// are excluded rather than projected backward from current state.
func (r *MySQLRepository) ListIdentityCredentialsAt(
	ctx context.Context,
	sourceID string,
	candidates []domain.IdentityCredential,
	systemAsOf time.Time,
) ([]domain.IdentityCredential, error) {
	_, offset := systemAsOf.Zone()
	if r == nil || r.db == nil || ctx == nil || sourceID == "" ||
		len(candidates) == 0 || systemAsOf.IsZero() || offset != 0 ||
		domain.ResolveIdentity(sourceID, candidates, nil).Status == domain.IdentityConflict {
		return nil, app.ErrInvalidRequest
	}
	result := make([]domain.IdentityCredential, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.SourceID != sourceID {
			return nil, app.ErrInvalidRequest
		}
		var credential domain.IdentityCredential
		var identifierType, identifierStatus string
		err := r.db.QueryRowContext(ctx, `SELECT i.source_id, i.identifier_type, i.identifier_value,
			i.identifier_hash, h.identifier_status, i.news_id, h.observed_at
			FROM news_identifiers i
			INNER JOIN news_identifier_status_history h
				ON h.source_id = i.source_id
				AND h.identifier_type = i.identifier_type
				AND h.identifier_hash = i.identifier_hash
			WHERE i.source_id = ? AND i.identifier_type = ? AND i.identifier_hash = ?
				AND i.identifier_value = ? AND i.created_at <= ?
				AND h.history_id = (
					SELECT h2.history_id FROM news_identifier_status_history h2
					WHERE h2.source_id = i.source_id
						AND h2.identifier_type = i.identifier_type
						AND h2.identifier_hash = i.identifier_hash
						AND h2.observed_at <= ?
					ORDER BY h2.observed_at DESC, h2.history_id DESC LIMIT 1
				)`,
			sourceID, string(candidate.Type), candidate.Hash, candidate.Value,
			systemAsOf.UTC(), systemAsOf.UTC(),
		).Scan(&credential.SourceID, &identifierType, &credential.Value, &credential.Hash,
			&identifierStatus, &credential.NewsID, &credential.ObservedAt)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		switch domain.IdentifierStatus(identifierStatus) {
		case domain.IdentifierActive, domain.IdentifierHistorical, domain.IdentifierInvalid:
		default:
			return nil, app.ErrIdentityHistoryInvalid
		}
		credential.Type = domain.IdentifierType(identifierType)
		credential.Status = domain.IdentifierStatus(identifierStatus)
		credential.ObservedAt = credential.ObservedAt.UTC()
		result = append(result, credential)
	}
	return result, nil
}
