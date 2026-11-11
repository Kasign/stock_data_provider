package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

// GetEvidenceVersionAt returns a version only when both it and its owning
// Observation were known by system_as_of. Missing and future versions share
// one result to avoid leaking whether a News ID exists.
func (r *MySQLRepository) GetEvidenceVersionAt(
	ctx context.Context,
	request app.GetEvidenceRequest,
) (app.EvidencePITRecord, error) {
	if r == nil || r.db == nil || ctx == nil ||
		request.NewsID == "" || request.NewsID != strings.TrimSpace(request.NewsID) ||
		(request.VersionID != "" && request.VersionID != strings.TrimSpace(request.VersionID)) ||
		request.SystemAsOf.IsZero() || !isEvidenceUTC(request.SystemAsOf) {
		return app.EvidencePITRecord{}, app.ErrInvalidEvidenceQuery
	}

	query := evidencePITSelect + ` WHERE v.news_id = ?
		AND v.observed_at <= ? AND o.observed_at <= ?`
	args := []any{request.NewsID, request.SystemAsOf.UTC(), request.SystemAsOf.UTC()}
	if request.VersionID != "" {
		query += " AND v.version_id = ?"
		args = append(args, request.VersionID)
	} else {
		query += " ORDER BY v.observed_at DESC, v.version_no DESC, v.version_id DESC LIMIT 1"
	}

	record, err := scanEvidencePITRecord(r.db.QueryRowContext(ctx, query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return app.EvidencePITRecord{}, app.ErrEvidencePITNotFound
	}
	if err != nil {
		return app.EvidencePITRecord{}, err
	}
	return record, nil
}

const evidencePITSelect = `SELECT
	o.observation_id, o.idempotency_key, o.source_id, o.provider_code,
	o.query_dimension, o.source_policy_version_id, o.endpoint_policy_version_id,
	o.policy_evaluated_at, o.candidate_json, o.observed_at, o.identity_status,
	o.news_id, o.reason_code,
	v.version_id, v.news_id, v.version_no, v.fingerprint, v.title,
	v.publisher_name, v.publisher_domain, v.author, v.published_at,
	v.content_json, v.observed_at, v.observation_id
	FROM news_item_versions v
	INNER JOIN news_observations o
		ON o.observation_id = v.observation_id AND o.news_id = v.news_id`

func scanEvidencePITRecord(row rowScanner) (app.EvidencePITRecord, error) {
	var result app.EvidencePITRecord
	var observation domain.Observation
	var version domain.NewsItemVersion
	var policyAt, publishedAt sql.NullTime
	var newsID sql.NullString
	var candidateJSON, contentJSON []byte
	var identityStatus string

	err := row.Scan(
		&observation.ID, &observation.IdempotencyKey, &observation.SourceID,
		&observation.ProviderCode, &observation.QueryDimension,
		&observation.SourcePolicyVersionID, &observation.EndpointPolicyVersionID,
		&policyAt, &candidateJSON, &observation.ObservedAt, &identityStatus,
		&newsID, &observation.ReasonCode,
		&version.ID, &version.NewsID, &version.VersionNo, &version.Fingerprint,
		&version.Title, &version.PublisherName, &version.PublisherDomain, &version.Author,
		&publishedAt, &contentJSON, &version.ObservedAt, &version.ObservationID,
	)
	if err != nil {
		return app.EvidencePITRecord{}, err
	}
	if err := json.Unmarshal(candidateJSON, &observation.Candidate); err != nil {
		return app.EvidencePITRecord{}, err
	}
	if err := json.Unmarshal(contentJSON, &version.Content); err != nil {
		return app.EvidencePITRecord{}, err
	}
	if policyAt.Valid {
		observation.PolicyEvaluatedAt = policyAt.Time.UTC()
	}
	if newsID.Valid {
		observation.NewsID = newsID.String
	}
	if publishedAt.Valid {
		value := publishedAt.Time.UTC()
		version.PublishedAt = &value
	}
	observation.IdentityStatus = domain.ObservationStatus(identityStatus)
	observation.ObservedAt = observation.ObservedAt.UTC()
	version.ObservedAt = version.ObservedAt.UTC()
	result.SourceRef = observation.SourceID
	result.ProviderRef = observation.ProviderCode
	result.Observation = observation
	result.Version = version
	return result, nil
}

func isEvidenceUTC(value time.Time) bool {
	_, offset := value.Zone()
	return offset == 0
}
