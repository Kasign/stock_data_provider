package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

const (
	newsTransactionLockWaitSeconds = 10
	newsTransactionReleaseTimeout  = 5 * time.Second
)

// MySQLRepository implements News persistence ports on a borrowed shared DB.
// It never opens, configures, or closes the database connection pool.
type MySQLRepository struct {
	db *sql.DB
}

var _ app.IdentityRepository = (*MySQLRepository)(nil)
var _ app.ResearchUpdateRepository = (*MySQLRepository)(nil)
var _ app.NewsItemRelationRepository = (*MySQLRepository)(nil)
var _ app.EvidencePITRepository = (*MySQLRepository)(nil)

var _ app.IdentityTransaction = (*mysqlNewsTransaction)(nil)
var _ app.ResearchUpdateTransaction = (*mysqlNewsTransaction)(nil)
var _ app.NewsItemRelationTransaction = (*mysqlNewsTransaction)(nil)

// NewMySQLRepository wraps the Server-owned MariaDB connection pool.
func NewMySQLRepository(db *sql.DB) (*MySQLRepository, error) {
	if db == nil {
		return nil, errors.New("news storage: database is required")
	}
	return &MySQLRepository{db: db}, nil
}

func (r *MySQLRepository) WithinIdentityTransaction(ctx context.Context, fn func(app.IdentityTransaction) error) error {
	if r == nil || r.db == nil || ctx == nil || fn == nil {
		return app.ErrInvalidRequest
	}
	return r.withTransaction(ctx, "", func(tx *sql.Tx, acquire func(string) error) error {
		return fn(&mysqlNewsTransaction{tx: tx, acquire: acquire})
	})
}

func (r *MySQLRepository) WithinResearchUpdateTransaction(ctx context.Context, fn func(app.ResearchUpdateTransaction) error) error {
	if r == nil || r.db == nil || ctx == nil || fn == nil {
		return app.ErrInvalidRequest
	}
	return r.withTransaction(ctx, "", func(tx *sql.Tx, acquire func(string) error) error {
		return fn(&mysqlNewsTransaction{tx: tx, acquire: acquire})
	})
}

func (r *MySQLRepository) WithinNewsItemRelationTransaction(ctx context.Context, fn func(app.NewsItemRelationTransaction) error) error {
	if r == nil || r.db == nil || ctx == nil || fn == nil {
		return app.ErrInvalidRequest
	}
	// The relation graph is cross-news and the current Application port reads
	// the complete graph. A single advisory lock prevents concurrent writers
	// from validating against stale graph snapshots.
	return r.withTransaction(ctx, "news_item_relation_graph", func(tx *sql.Tx, acquire func(string) error) error {
		return fn(&mysqlNewsTransaction{tx: tx, acquire: acquire})
	})
}

type mysqlNewsTransaction struct {
	tx      *sql.Tx
	acquire func(string) error
}

func (r *MySQLRepository) withTransaction(
	ctx context.Context,
	initialLock string,
	fn func(*sql.Tx, func(string) error) error,
) (resultErr error) {
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	heldLocks := make([]string, 0, 2)
	held := make(map[string]struct{})
	releaseLocks := func() error {
		var releaseErr error
		releaseCtx, cancel := context.WithTimeout(context.Background(), newsTransactionReleaseTimeout)
		defer cancel()
		for index := len(heldLocks) - 1; index >= 0; index-- {
			name := heldLocks[index]
			var released sql.NullInt64
			err := conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", name).Scan(&released)
			if err == nil && (!released.Valid || released.Int64 != 1) {
				err = fmt.Errorf("news storage: advisory lock release not confirmed")
			}
			if err != nil {
				releaseErr = errors.Join(releaseErr, err)
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
		return releaseErr
	}
	acquireOnConn := func(lockName string) error {
		if _, exists := held[lockName]; exists {
			return nil
		}
		var acquired sql.NullInt64
		if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, newsTransactionLockWaitSeconds).Scan(&acquired); err != nil {
			return err
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			return errors.New("news storage: advisory lock unavailable")
		}
		held[lockName] = struct{}{}
		heldLocks = append(heldLocks, lockName)
		return nil
	}

	if initialLock != "" {
		if err := acquireOnConn(initialLock); err != nil {
			return err
		}
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		if releaseErr := releaseLocks(); releaseErr != nil {
			return errors.Join(err, releaseErr)
		}
		return err
	}
	acquireInTx := func(lockName string) error {
		if _, exists := held[lockName]; exists {
			return nil
		}
		var acquired sql.NullInt64
		if err := tx.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", lockName, newsTransactionLockWaitSeconds).Scan(&acquired); err != nil {
			return err
		}
		if !acquired.Valid || acquired.Int64 != 1 {
			return errors.New("news storage: advisory lock unavailable")
		}
		held[lockName] = struct{}{}
		heldLocks = append(heldLocks, lockName)
		return nil
	}

	if err := fn(tx, acquireInTx); err != nil {
		_ = tx.Rollback()
		resultErr = err
	} else if err := tx.Commit(); err != nil {
		resultErr = err
	}
	if releaseErr := releaseLocks(); releaseErr != nil {
		resultErr = errors.Join(resultErr, releaseErr)
	}
	return resultErr
}

func identityAdvisoryLockName(sourceID string) string {
	sum := sha256.Sum256([]byte(sourceID))
	// MariaDB advisory lock names are limited to 64 characters.
	return "news_identity_" + hex.EncodeToString(sum[:])[:48]
}

func newStorageID() (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// FindIdentityCredentials serializes identity resolution per source across
// service instances, then compares the original identifier value after hash lookup.
func (t *mysqlNewsTransaction) FindIdentityCredentials(ctx context.Context, sourceID string, candidates []domain.IdentityCredential) ([]domain.IdentityCredential, error) {
	if t == nil || t.tx == nil || t.acquire == nil || strings.TrimSpace(sourceID) == "" {
		return nil, app.ErrInvalidRequest
	}
	if err := t.acquire(identityAdvisoryLockName(sourceID)); err != nil {
		return nil, err
	}
	result := make([]domain.IdentityCredential, 0, len(candidates))
	for _, candidate := range candidates {
		var stored domain.IdentityCredential
		var identifierType, identifierStatus string
		err := t.tx.QueryRowContext(ctx, `SELECT source_id, identifier_type, identifier_value,
			identifier_hash, identifier_status, news_id
			FROM news_identifiers
			WHERE source_id = ? AND identifier_type = ? AND identifier_hash = ?
			FOR UPDATE`,
			sourceID, string(candidate.Type), candidate.Hash,
		).Scan(&stored.SourceID, &identifierType, &stored.Value, &stored.Hash, &identifierStatus, &stored.NewsID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		stored.Status = domain.IdentifierStatus(identifierStatus)
		stored.Type = domain.IdentifierType(identifierType)
		result = append(result, stored)
	}
	return result, nil
}

func (t *mysqlNewsTransaction) CreateNewsItem(ctx context.Context, sourceID string) (string, error) {
	if t == nil || t.tx == nil || strings.TrimSpace(sourceID) == "" {
		return "", app.ErrInvalidRequest
	}
	newsID, err := newStorageID()
	if err != nil {
		return "", err
	}
	_, err = t.tx.ExecContext(ctx,
		"INSERT INTO news_items (news_id, origin_source_id) VALUES (?, ?)",
		newsID, sourceID,
	)
	if err != nil {
		return "", err
	}
	return newsID, nil
}

func (t *mysqlNewsTransaction) SaveIdentityCredentials(ctx context.Context, credentials []domain.IdentityCredential) error {
	if t == nil || t.tx == nil {
		return app.ErrInvalidRequest
	}
	for _, credential := range credentials {
		_, observedOffset := credential.ObservedAt.Zone()
		if credential.ObservedAt.IsZero() || observedOffset != 0 {
			return app.ErrIdentityHistoryInvalid
		}
		if _, err := t.tx.ExecContext(ctx, `INSERT INTO news_identifiers
			(source_id, identifier_type, identifier_hash, identifier_value, identifier_status, news_id, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			credential.SourceID, string(credential.Type), credential.Hash,
			credential.Value, string(credential.Status), credential.NewsID, credential.ObservedAt.UTC(),
		); err != nil {
			return err
		}
		if _, err := t.tx.ExecContext(ctx, `INSERT INTO news_identifier_status_history
			(source_id, identifier_type, identifier_hash, identifier_status, observed_at)
			VALUES (?, ?, ?, ?, ?)`,
			credential.SourceID, string(credential.Type), credential.Hash,
			string(credential.Status), credential.ObservedAt.UTC(),
		); err != nil {
			return err
		}
	}
	return nil
}

func (t *mysqlNewsTransaction) AppendObservation(ctx context.Context, observation domain.Observation) (domain.Observation, error) {
	if t == nil || t.tx == nil {
		return domain.Observation{}, app.ErrInvalidRequest
	}
	candidateJSON, err := json.Marshal(observation.Candidate)
	if err != nil {
		return domain.Observation{}, err
	}
	var policyEvaluatedAt any
	if !observation.PolicyEvaluatedAt.IsZero() {
		policyEvaluatedAt = observation.PolicyEvaluatedAt.UTC()
	}
	var newsID any
	if observation.NewsID != "" {
		newsID = observation.NewsID
	}
	_, insertErr := t.tx.ExecContext(ctx, `INSERT INTO news_observations
		(observation_id, idempotency_key, source_id, provider_code, query_dimension,
		 source_policy_version_id, endpoint_policy_version_id, policy_evaluated_at,
		 candidate_json, observed_at, identity_status, news_id, reason_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.ID, observation.IdempotencyKey, observation.SourceID, observation.ProviderCode,
		observation.QueryDimension, observation.SourcePolicyVersionID, observation.EndpointPolicyVersionID,
		policyEvaluatedAt, candidateJSON, observation.ObservedAt.UTC(),
		string(observation.IdentityStatus), newsID, observation.ReasonCode,
	)
	if insertErr == nil {
		return observation, nil
	}

	existing, lookupErr := scanObservation(t.tx.QueryRowContext(ctx, observationSelect+" WHERE idempotency_key = ? FOR UPDATE", observation.IdempotencyKey))
	if lookupErr != nil {
		return domain.Observation{}, errors.Join(insertErr, lookupErr)
	}
	if !app.SameObservationPayload(existing, observation) {
		return domain.Observation{}, app.ErrObservationIdempotencyConflict
	}
	return existing, nil
}

const observationSelect = `SELECT observation_id, idempotency_key, source_id, provider_code,
	query_dimension, source_policy_version_id, endpoint_policy_version_id,
	policy_evaluated_at, candidate_json, observed_at, identity_status, news_id, reason_code
	FROM news_observations`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanObservation(row rowScanner) (domain.Observation, error) {
	var result domain.Observation
	var policyAt sql.NullTime
	var newsID sql.NullString
	var candidateJSON []byte
	var status string
	err := row.Scan(
		&result.ID, &result.IdempotencyKey, &result.SourceID, &result.ProviderCode,
		&result.QueryDimension, &result.SourcePolicyVersionID, &result.EndpointPolicyVersionID,
		&policyAt, &candidateJSON, &result.ObservedAt, &status, &newsID, &result.ReasonCode,
	)
	if err != nil {
		return domain.Observation{}, err
	}
	if err := json.Unmarshal(candidateJSON, &result.Candidate); err != nil {
		return domain.Observation{}, err
	}
	if policyAt.Valid {
		result.PolicyEvaluatedAt = policyAt.Time.UTC()
	}
	if newsID.Valid {
		result.NewsID = newsID.String
	}
	result.IdentityStatus = domain.ObservationStatus(status)
	result.ObservedAt = result.ObservedAt.UTC()
	return result, nil
}

func (t *mysqlNewsTransaction) ObservationByID(ctx context.Context, observationID string) (domain.Observation, bool, error) {
	if t == nil || t.tx == nil || strings.TrimSpace(observationID) == "" {
		return domain.Observation{}, false, app.ErrInvalidRequest
	}
	observation, err := scanObservation(t.tx.QueryRowContext(ctx, observationSelect+" WHERE observation_id = ?", observationID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Observation{}, false, nil
	}
	return observation, err == nil, err
}

func (t *mysqlNewsTransaction) LatestNewsItemVersion(ctx context.Context, newsID string) (domain.NewsItemVersion, bool, error) {
	if t == nil || t.tx == nil || strings.TrimSpace(newsID) == "" {
		return domain.NewsItemVersion{}, false, app.ErrInvalidRequest
	}
	var lockedID string
	if err := t.tx.QueryRowContext(ctx, "SELECT news_id FROM news_items WHERE news_id = ? FOR UPDATE", newsID).Scan(&lockedID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return domain.NewsItemVersion{}, false, nil
		}
		return domain.NewsItemVersion{}, false, err
	}
	version, err := scanVersion(t.tx.QueryRowContext(ctx, versionSelect+" WHERE news_id = ? ORDER BY version_no DESC LIMIT 1 FOR UPDATE", newsID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewsItemVersion{}, false, nil
	}
	return version, err == nil, err
}

func (t *mysqlNewsTransaction) NewsItemVersionByID(ctx context.Context, versionID string) (domain.NewsItemVersion, bool, error) {
	if t == nil || t.tx == nil || strings.TrimSpace(versionID) == "" {
		return domain.NewsItemVersion{}, false, app.ErrInvalidRequest
	}
	version, err := scanVersion(t.tx.QueryRowContext(ctx, versionSelect+" WHERE version_id = ?", versionID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewsItemVersion{}, false, nil
	}
	return version, err == nil, err
}

const versionSelect = `SELECT version_id, news_id, version_no, fingerprint, title,
	publisher_name, publisher_domain, author, published_at, content_json, observed_at, observation_id
	FROM news_item_versions`

func scanVersion(row rowScanner) (domain.NewsItemVersion, error) {
	var result domain.NewsItemVersion
	var publishedAt sql.NullTime
	var contentJSON []byte
	err := row.Scan(
		&result.ID, &result.NewsID, &result.VersionNo, &result.Fingerprint, &result.Title,
		&result.PublisherName, &result.PublisherDomain, &result.Author,
		&publishedAt, &contentJSON, &result.ObservedAt, &result.ObservationID,
	)
	if err != nil {
		return domain.NewsItemVersion{}, err
	}
	if publishedAt.Valid {
		value := publishedAt.Time.UTC()
		result.PublishedAt = &value
	}
	if err := json.Unmarshal(contentJSON, &result.Content); err != nil {
		return domain.NewsItemVersion{}, err
	}
	result.ObservedAt = result.ObservedAt.UTC()
	return result, nil
}

func (t *mysqlNewsTransaction) AppendNewsItemVersion(ctx context.Context, version domain.NewsItemVersion) error {
	if t == nil || t.tx == nil {
		return app.ErrInvalidRequest
	}
	contentJSON, err := json.Marshal(version.Content)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(ctx, `INSERT INTO news_item_versions
		(version_id, news_id, version_no, fingerprint, title, publisher_name, publisher_domain,
		 author, published_at, content_json, observed_at, observation_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		version.ID, version.NewsID, version.VersionNo, version.Fingerprint, version.Title,
		version.PublisherName, version.PublisherDomain, version.Author, version.PublishedAt,
		contentJSON, version.ObservedAt.UTC(), version.ObservationID,
	)
	return err
}

func (t *mysqlNewsTransaction) ListRelationAssertions(ctx context.Context, newsID string) ([]domain.RelationAssertion, error) {
	if t == nil || t.tx == nil || strings.TrimSpace(newsID) == "" {
		return nil, app.ErrInvalidRequest
	}
	rows, err := t.tx.QueryContext(ctx, `SELECT relation_id, news_id, subject_type, subject_json,
		relation_kind, method, status, supersedes_relation_id, valid_from, valid_to, observed_at
		FROM news_relation_assertions WHERE news_id = ? ORDER BY observed_at, relation_id FOR UPDATE`, newsID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.RelationAssertion, 0)
	for rows.Next() {
		assertion, err := scanRelationAssertion(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, assertion)
	}
	return result, rows.Err()
}

func scanRelationAssertion(row rowScanner) (domain.RelationAssertion, error) {
	var result domain.RelationAssertion
	var subjectType, kind, method, status string
	var subjectJSON []byte
	var supersedes sql.NullString
	var validFrom, validTo sql.NullTime
	err := row.Scan(
		&result.ID, &result.NewsID, &subjectType, &subjectJSON,
		&kind, &method, &status, &supersedes, &validFrom, &validTo, &result.ObservedAt,
	)
	if err != nil {
		return domain.RelationAssertion{}, err
	}
	result.SubjectType = domain.RelationSubjectType(subjectType)
	result.Kind = domain.RelationKind(kind)
	result.Method = domain.RelationMethod(method)
	result.Status = domain.RelationStatus(status)
	if supersedes.Valid {
		result.SupersedesRelationID = supersedes.String
	}
	if validFrom.Valid {
		value := validFrom.Time.UTC()
		result.ValidFrom = &value
	}
	if validTo.Valid {
		value := validTo.Time.UTC()
		result.ValidTo = &value
	}
	switch result.SubjectType {
	case domain.RelationSubjectSecurity:
		var subject domain.SecurityRelationSubject
		if err := json.Unmarshal(subjectJSON, &subject); err != nil {
			return domain.RelationAssertion{}, err
		}
		result.Security = &subject
	case domain.RelationSubjectIndustry:
		var subject domain.IndustryRelationSubject
		if err := json.Unmarshal(subjectJSON, &subject); err != nil {
			return domain.RelationAssertion{}, err
		}
		result.Industry = &subject
	default:
		return domain.RelationAssertion{}, domain.ErrRelationInvalid
	}
	result.ObservedAt = result.ObservedAt.UTC()
	return result, nil
}

func (t *mysqlNewsTransaction) AppendRelationAssertion(ctx context.Context, assertion domain.RelationAssertion, evidence []domain.RelationEvidence) error {
	if t == nil || t.tx == nil {
		return app.ErrInvalidRequest
	}
	var subject any
	if assertion.SubjectType == domain.RelationSubjectSecurity {
		subject = assertion.Security
	} else {
		subject = assertion.Industry
	}
	subjectJSON, err := json.Marshal(subject)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(ctx, `INSERT INTO news_relation_assertions
		(relation_id, news_id, subject_type, subject_json, relation_kind, method, status,
		 supersedes_relation_id, valid_from, valid_to, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		assertion.ID, assertion.NewsID, string(assertion.SubjectType), subjectJSON,
		string(assertion.Kind), string(assertion.Method), string(assertion.Status),
		nullableString(assertion.SupersedesRelationID), assertion.ValidFrom, assertion.ValidTo, assertion.ObservedAt.UTC(),
	)
	if err != nil {
		return err
	}
	for _, item := range evidence {
		evidenceID, err := newStorageID()
		if err != nil {
			return err
		}
		_, err = t.tx.ExecContext(ctx, `INSERT INTO news_relation_evidence
			(evidence_id, relation_id, evidence_type, source_version_id, observation_id,
			 rule_version, model_version, field_path, evidence_hash)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			evidenceID, assertion.ID, string(item.Type), nullableString(item.SourceVersionID),
			nullableString(item.ObservationID), item.RuleVersion, item.ModelVersion,
			item.FieldPath, item.EvidenceHash,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func (t *mysqlNewsTransaction) ListAllNewsItemRelations(ctx context.Context) ([]domain.NewsItemRelation, error) {
	if t == nil || t.tx == nil {
		return nil, app.ErrInvalidRequest
	}
	rows, err := t.tx.QueryContext(ctx, `SELECT item_relation_id, from_news_id, from_source_id,
		to_news_id, to_source_id, relation_kind, status, supersedes_relation_id,
		evidence_observation_id, from_version_id, to_version_id, asserted_at, observed_at
		FROM news_item_relations ORDER BY observed_at, item_relation_id FOR UPDATE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]domain.NewsItemRelation, 0)
	for rows.Next() {
		var relation domain.NewsItemRelation
		var kind, status string
		var supersedes, evidenceObservation, fromVersion, toVersion sql.NullString
		if err := rows.Scan(
			&relation.ID, &relation.FromNewsID, &relation.FromSourceID, &relation.ToNewsID,
			&relation.ToSourceID, &kind, &status, &supersedes, &evidenceObservation,
			&fromVersion, &toVersion, &relation.AssertedAt, &relation.ObservedAt,
		); err != nil {
			return nil, err
		}
		relation.Kind = domain.NewsItemRelationKind(kind)
		relation.Status = domain.NewsItemRelationStatus(status)
		if supersedes.Valid {
			relation.SupersedesRelationID = supersedes.String
		}
		if evidenceObservation.Valid {
			relation.EvidenceObservation = evidenceObservation.String
		}
		if fromVersion.Valid {
			relation.FromVersionID = fromVersion.String
		}
		if toVersion.Valid {
			relation.ToVersionID = toVersion.String
		}
		relation.AssertedAt = relation.AssertedAt.UTC()
		relation.ObservedAt = relation.ObservedAt.UTC()
		result = append(result, relation)
	}
	return result, rows.Err()
}

func (t *mysqlNewsTransaction) AppendNewsItemRelation(ctx context.Context, relation domain.NewsItemRelation) error {
	if t == nil || t.tx == nil {
		return app.ErrInvalidRequest
	}
	_, err := t.tx.ExecContext(ctx, `INSERT INTO news_item_relations
		(item_relation_id, from_news_id, from_source_id, to_news_id, to_source_id,
		 relation_kind, status, supersedes_relation_id, evidence_observation_id,
		 from_version_id, to_version_id, asserted_at, observed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		relation.ID, relation.FromNewsID, relation.FromSourceID, relation.ToNewsID, relation.ToSourceID,
		string(relation.Kind), string(relation.Status), nullableString(relation.SupersedesRelationID),
		nullableString(relation.EvidenceObservation), nullableString(relation.FromVersionID),
		nullableString(relation.ToVersionID), relation.AssertedAt.UTC(), relation.ObservedAt.UTC(),
	)
	return err
}

func nullableString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

var requiredNewsSchemaTables = []string{
	"news_items",
	"news_identifiers",
	"news_identifier_status_history",
	"news_observations",
	"news_item_versions",
	"news_relation_assertions",
	"news_relation_evidence",
	"news_item_relations",
	"news_authorization_snapshots",
	"news_evidence_snapshots",
}

// Ping verifies the borrowed SQL pool without taking ownership of it.
func (r *MySQLRepository) Ping(ctx context.Context) error {
	if r == nil || r.db == nil || ctx == nil {
		return app.ErrInvalidRequest
	}
	return r.db.PingContext(ctx)
}

// MissingSchemaTables returns only the names of News-owned tables that are absent.
// It is read-only and does not attempt to apply migrations.
func (r *MySQLRepository) MissingSchemaTables(ctx context.Context) ([]string, error) {
	if r == nil || r.db == nil || ctx == nil {
		return nil, app.ErrInvalidRequest
	}
	placeholders := strings.TrimRight(strings.Repeat("?,", len(requiredNewsSchemaTables)), ",")
	args := make([]any, 0, len(requiredNewsSchemaTables))
	for _, table := range requiredNewsSchemaTables {
		args = append(args, table)
	}
	rows, err := r.db.QueryContext(ctx, fmt.Sprintf(
		"SELECT table_name FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name IN (%s)",
		placeholders,
	), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	present := make(map[string]struct{}, len(requiredNewsSchemaTables))
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			return nil, err
		}
		present[table] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	missing := make([]string, 0)
	for _, table := range requiredNewsSchemaTables {
		if _, ok := present[table]; !ok {
			missing = append(missing, table)
		}
	}
	return missing, nil
}
