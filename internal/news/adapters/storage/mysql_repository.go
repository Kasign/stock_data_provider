package storage

import (
	"context"
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
var _ app.EvidencePITRepository = (*MySQLRepository)(nil)
var _ app.AvailabilityDecisionStore = (*MySQLRepository)(nil)
var _ app.AvailabilityEvidenceBindingResolver = (*MySQLRepository)(nil)

var _ app.IdentityTransaction = (*mysqlNewsTransaction)(nil)

// NewMySQLRepository wraps the Server-owned MariaDB connection pool.
func NewMySQLRepository(db *sql.DB) (*MySQLRepository, error) {
	if db == nil {
		return nil, errors.New("news storage: database is required")
	}
	return &MySQLRepository{db: db}, nil
}

var ErrAvailabilityDecisionSequenceConflict = errors.New("news storage: availability decision sequence conflict")
var ErrAvailabilityDecisionNotFound = app.ErrAvailabilityDecisionNotFound
var ErrAvailabilityEvidenceBindingNotFound = app.ErrAvailabilityEvidenceBindingNotFound

// ResolveAvailabilityEvidenceSourceBinding 从持久化证据回溯其精确来源/策略绑定。
// 只有证据、版本和 Observation 三方身份全部一致时才返回绑定。
func (r *MySQLRepository) ResolveAvailabilityEvidenceSourceBinding(
	ctx context.Context,
	evidence domain.AvailabilityEvidence,
) (app.AvailabilityEvidenceSourceBinding, error) {
	if r == nil || r.db == nil || ctx == nil ||
		evidence.ID == "" || evidence.NewsID == "" || evidence.VersionID == "" || evidence.ObservationID == "" {
		return app.AvailabilityEvidenceSourceBinding{}, app.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return app.AvailabilityEvidenceSourceBinding{}, err
	}

	var binding app.AvailabilityEvidenceSourceBinding
	var providerCode, endpointID, endpointPolicyVersionID, providerPolicyRef, providerPolicyVersionID sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT e.evidence_id, o.source_id, o.endpoint_id, o.provider_code,
		o.source_policy_version_id, o.endpoint_policy_version_id,
		o.provider_output_policy_ref, o.provider_output_policy_version_id
		FROM news_version_availability_evidence e
		INNER JOIN news_item_versions v
			ON v.version_id = e.version_id AND v.news_id = e.news_id AND v.observation_id = e.observation_id
		INNER JOIN news_observations o
			ON o.observation_id = v.observation_id AND o.news_id = v.news_id
		WHERE e.evidence_id = ? AND e.version_id = ? AND e.news_id = ? AND e.observation_id = ?`,
		evidence.ID, evidence.VersionID, evidence.NewsID, evidence.ObservationID,
	).Scan(
		&binding.EvidenceID, &binding.SourceID, &endpointID, &providerCode,
		&binding.SourcePolicyVersionID, &endpointPolicyVersionID,
		&providerPolicyRef, &providerPolicyVersionID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return app.AvailabilityEvidenceSourceBinding{}, ErrAvailabilityEvidenceBindingNotFound
	}
	if err != nil {
		return app.AvailabilityEvidenceSourceBinding{}, err
	}
	if binding.EvidenceID != evidence.ID || binding.SourceID == "" {
		return app.AvailabilityEvidenceSourceBinding{}, ErrAvailabilityEvidenceBindingNotFound
	}
	if endpointID.Valid {
		binding.EndpointID = endpointID.String
	}
	if providerCode.Valid {
		binding.ProviderCode = providerCode.String
	}
	if endpointPolicyVersionID.Valid {
		binding.EndpointPolicyVersionID = endpointPolicyVersionID.String
	}
	if providerPolicyRef.Valid {
		binding.ProviderOutputPolicyRef = providerPolicyRef.String
	}
	if providerPolicyVersionID.Valid {
		binding.ProviderOutputPolicyVersionID = providerPolicyVersionID.String
	}
	return binding, nil
}

// LatestAvailabilityDecisionForVersion 返回版本当前决策链头。
// 读取本身不加锁；并发写入由 AppendAvailabilityDecision 的序号 CAS 拒绝陈旧前驱。
func (r *MySQLRepository) LatestAvailabilityDecisionForVersion(
	ctx context.Context,
	versionID string,
) (app.AvailabilityDecisionState, error) {
	if r == nil || r.db == nil || ctx == nil ||
		versionID == "" || versionID != strings.TrimSpace(versionID) {
		return app.AvailabilityDecisionState{}, app.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return app.AvailabilityDecisionState{}, err
	}

	var state app.AvailabilityDecisionState
	var status string
	err := r.db.QueryRowContext(ctx, `SELECT version_id, decision_id, decision_seq, status
		FROM news_version_availability_decisions
		WHERE version_id = ?
		ORDER BY decision_seq DESC
		LIMIT 1`, versionID,
	).Scan(&state.VersionID, &state.DecisionID, &state.Sequence, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return app.AvailabilityDecisionState{}, ErrAvailabilityDecisionNotFound
	}
	if err != nil {
		return app.AvailabilityDecisionState{}, err
	}
	state.Status = domain.AvailabilityDecisionStatus(status)
	if state.VersionID != versionID || state.DecisionID == "" || state.Sequence == 0 ||
		(state.Status != domain.AvailabilityDecisionVerified &&
			state.Status != domain.AvailabilityDecisionConflict &&
			state.Status != domain.AvailabilityDecisionUnresolved &&
			state.Status != domain.AvailabilityDecisionRevoked) {
		return app.AvailabilityDecisionState{}, fmt.Errorf("news storage: invalid availability decision chain head")
	}
	return state, nil
}

// AppendAvailabilityDecision 在版本行锁保护下追加完整决策状态。
// 版本行是串行化锚点，因此即使当前尚无 Decision 行，并发首写也不会分叉。
func (r *MySQLRepository) AppendAvailabilityDecision(ctx context.Context, decision domain.AvailabilityDecision) error {
	if r == nil || r.db == nil || ctx == nil || decision.Validate() != nil {
		return app.ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	tx, err := r.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 锁定稳定存在的 Version 行，覆盖“尚无 Decision 行”的首写竞争。
	var lockedVersionID, lockedNewsID, lockedObservationID string
	if err := tx.QueryRowContext(ctx,
		"SELECT version_id, news_id, observation_id FROM news_item_versions WHERE version_id = ? FOR UPDATE",
		decision.VersionID,
	).Scan(&lockedVersionID, &lockedNewsID, &lockedObservationID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("news storage: availability decision version %q not found", decision.VersionID)
		}
		return err
	}

	var latestID string
	var latestSequence uint64
	var latestRecordedAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT decision_id, decision_seq, recorded_at
		FROM news_version_availability_decisions
		WHERE version_id = ?
		ORDER BY decision_seq DESC
		LIMIT 1 FOR UPDATE`, decision.VersionID,
	).Scan(&latestID, &latestSequence, &latestRecordedAt)
	if errors.Is(err, sql.ErrNoRows) {
		if decision.Sequence != 1 || decision.SupersedesDecisionID != nil {
			return ErrAvailabilityDecisionSequenceConflict
		}
	} else if err != nil {
		return err
	} else {
		if latestSequence == ^uint64(0) ||
			decision.Sequence != latestSequence+1 ||
			decision.SupersedesDecisionID == nil ||
			*decision.SupersedesDecisionID != latestID ||
			decision.RecordedAt.Before(latestRecordedAt) {
			return ErrAvailabilityDecisionSequenceConflict
		}
	}

	// 所有被审阅的 Evidence 都必须属于同一 Version；锁住证据行，
	// 防止写入期间并发清理选中 Evidence 的受限载荷。
	if len(decision.ConsideredEvidenceIDs) > 0 {
		query := `SELECT evidence_id, news_id, version_id, observation_id, field_path,
			raw_value, source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
			candidate_available_at_utc, parser_version, evidence_ref, evidence_hash,
			payload_purged_at, observed_at, recorded_at, retention_until
			FROM news_version_availability_evidence
			WHERE version_id = ? AND evidence_id IN (` +
			evidencePlaceholders(len(decision.ConsideredEvidenceIDs)) + `) FOR UPDATE`
		args := make([]any, 0, len(decision.ConsideredEvidenceIDs)+1)
		args = append(args, decision.VersionID)
		for _, id := range decision.ConsideredEvidenceIDs {
			args = append(args, id)
		}
		rows, queryErr := tx.QueryContext(ctx, query, args...)
		if queryErr != nil {
			return queryErr
		}
		found := make(map[string]struct{}, len(decision.ConsideredEvidenceIDs))
		for rows.Next() {
			var evidence domain.AvailabilityEvidence
			var rawValue, evidenceRef, evidenceHash sql.NullString
			var candidateAvailableAt, payloadPurgedAt sql.NullTime
			var sourcePrecision, parseStatus string
			var observedAt, recordedAt, retentionUntil time.Time
			if scanErr := rows.Scan(
				&evidence.ID, &evidence.NewsID, &evidence.VersionID, &evidence.ObservationID,
				&evidence.FieldPath, &rawValue, &evidence.SourceTimezoneOrOffset,
				&sourcePrecision, &parseStatus, &evidence.ParseReasonCode,
				&candidateAvailableAt, &evidence.ParserVersion, &evidenceRef, &evidenceHash,
				&payloadPurgedAt, &observedAt, &recordedAt, &retentionUntil,
			); scanErr != nil {
				_ = rows.Close()
				return scanErr
			}
			evidence.Precision = domain.AvailabilityTimePrecision(sourcePrecision)
			evidence.ParseStatus = domain.AvailabilityTimeParseStatus(parseStatus)
			if rawValue.Valid {
				evidence.RawValue = rawValue.String
			}
			if candidateAvailableAt.Valid {
				value := candidateAvailableAt.Time.UTC()
				evidence.CandidateAvailableAt = &value
			}
			if evidenceRef.Valid {
				value := evidenceRef.String
				evidence.EvidenceRef = &value
			}
			if evidenceHash.Valid {
				value := evidenceHash.String
				evidence.EvidenceHash = &value
			}
			if payloadPurgedAt.Valid {
				value := payloadPurgedAt.Time.UTC()
				evidence.PayloadPurgedAt = &value
			}
			evidence.ObservedAt = observedAt.UTC()
			evidence.RecordedAt = recordedAt.UTC()
			evidence.RetentionUntil = retentionUntil.UTC()

			found[evidence.ID] = struct{}{}
			if decision.SelectedEvidenceID != nil && evidence.ID == *decision.SelectedEvidenceID {
				if evidence.NewsID != lockedNewsID || evidence.VersionID != lockedVersionID ||
					evidence.ObservationID != lockedObservationID ||
					evidence.ParseStatus != domain.AvailabilityTimeParsed ||
					evidence.PayloadPurgedAt != nil ||
					evidence.CandidateAvailableAt == nil ||
					evidence.Validate() != nil ||
					(evidence.EvidenceRef == nil && evidence.EvidenceHash == nil) ||
					evidence.RecordedAt.After(decision.RecordedAt) ||
					evidence.ObservedAt.After(decision.RecordedAt) ||
					!evidence.RetentionUntil.After(decision.RecordedAt) {
					_ = rows.Close()
					return fmt.Errorf("news storage: selected availability evidence is not eligible")
				}
			}
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if rowsErr != nil {
			return rowsErr
		}
		if closeErr != nil {
			return closeErr
		}
		if len(found) != len(decision.ConsideredEvidenceIDs) {
			return fmt.Errorf("news storage: considered availability evidence does not belong to version")
		}
	}

	consideredJSON, err := json.Marshal(decision.ConsideredEvidenceIDs)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO news_version_availability_decisions
		(decision_id, version_id, decision_seq, status, selected_evidence_id,
		 considered_evidence_ids, reason_code, review_rule_version,
		 supersedes_decision_id, recorded_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		decision.ID, decision.VersionID, decision.Sequence, string(decision.Status),
		nullableStringPointer(decision.SelectedEvidenceID), consideredJSON,
		decision.ReasonCode, decision.ReviewRuleVersion,
		nullableStringPointer(decision.SupersedesDecisionID), decision.RecordedAt,
	)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func nullableStringPointer(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func (r *MySQLRepository) WithinIdentityTransaction(ctx context.Context, fn func(app.IdentityTransaction) error) error {
	if r == nil || r.db == nil || ctx == nil || fn == nil {
		return app.ErrInvalidRequest
	}
	return r.withTransaction(ctx, "", &sql.TxOptions{Isolation: sql.LevelRepeatableRead}, func(tx *sql.Tx, acquire func(string) error) error {
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
	options *sql.TxOptions,
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

	tx, err := conn.BeginTx(ctx, options)
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

func canonicalURLAdvisoryLockName(hash string) string {
	sum := sha256.Sum256([]byte(hash))
	return "news_url_" + hex.EncodeToString(sum[:])[:48]
}

// FindNewsItemByCanonicalURL 先取得跨实例 URL 锁，再按全局唯一 Hash 查询并比较 URL 原文。
func (t *mysqlNewsTransaction) FindNewsItemByCanonicalURL(ctx context.Context, canonicalURL, canonicalURLHash string) (domain.NewsItem, bool, error) {
	if t == nil || t.tx == nil || t.acquire == nil || strings.TrimSpace(canonicalURL) == "" || strings.TrimSpace(canonicalURLHash) == "" {
		return domain.NewsItem{}, false, app.ErrInvalidRequest
	}
	if err := t.acquire(canonicalURLAdvisoryLockName(canonicalURLHash)); err != nil {
		return domain.NewsItem{}, false, err
	}
	item, err := scanNewsItem(t.tx.QueryRowContext(ctx, newsItemSelect+" WHERE canonical_url_hash = ? FOR UPDATE", canonicalURLHash))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewsItem{}, false, nil
	}
	if err != nil {
		return domain.NewsItem{}, false, err
	}
	if item.CanonicalURL != canonicalURL || item.CanonicalURLHash != canonicalURLHash {
		return domain.NewsItem{}, false, app.ErrCanonicalURLHashCollision
	}
	return item, true, nil
}

func (t *mysqlNewsTransaction) FindNewsItemByID(ctx context.Context, newsID string) (domain.NewsItem, bool, error) {
	if t == nil || t.tx == nil || strings.TrimSpace(newsID) == "" {
		return domain.NewsItem{}, false, app.ErrInvalidRequest
	}
	item, err := scanNewsItem(t.tx.QueryRowContext(ctx, newsItemSelect+" WHERE news_id = ? FOR UPDATE", newsID))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.NewsItem{}, false, nil
	}
	return item, err == nil, err
}

const newsItemSelect = `SELECT news_id, canonical_url, canonical_url_hash, title, publisher_name,
	publisher_domain, author, published_at, content_json, content_fingerprint, created_at, updated_at
	FROM news_items`

func scanNewsItem(row rowScanner) (domain.NewsItem, error) {
	var item domain.NewsItem
	var canonicalURL, canonicalURLHash sql.NullString
	var publishedAt sql.NullTime
	var contentJSON []byte
	err := row.Scan(
		&item.ID, &canonicalURL, &canonicalURLHash, &item.Title, &item.PublisherName,
		&item.PublisherDomain, &item.Author, &publishedAt, &contentJSON,
		&item.ContentFingerprint, &item.CreatedAt, &item.UpdatedAt,
	)
	if err != nil {
		return domain.NewsItem{}, err
	}
	if canonicalURL.Valid {
		item.CanonicalURL = canonicalURL.String
	}
	if canonicalURLHash.Valid {
		item.CanonicalURLHash = canonicalURLHash.String
	}
	if publishedAt.Valid {
		value := publishedAt.Time.UTC()
		item.PublishedAt = &value
	}
	if err := json.Unmarshal(contentJSON, &item.Content); err != nil {
		return domain.NewsItem{}, err
	}
	item.CreatedAt = item.CreatedAt.UTC()
	item.UpdatedAt = item.UpdatedAt.UTC()
	return item, nil
}

// FindIdentityCredentials serializes only source-native identity within its source.
func (t *mysqlNewsTransaction) FindIdentityCredentials(ctx context.Context, sourceID string, candidates []domain.IdentityCredential) ([]domain.IdentityCredential, error) {
	if t == nil || t.tx == nil || strings.TrimSpace(sourceID) == "" {
		return nil, app.ErrInvalidRequest
	}
	if len(candidates) == 0 {
		return []domain.IdentityCredential{}, nil
	}
	if t.acquire == nil {
		return nil, app.ErrInvalidRequest
	}
	if err := t.acquire(identityAdvisoryLockName(sourceID)); err != nil {
		return nil, err
	}
	result := make([]domain.IdentityCredential, 0, len(candidates))
	for _, candidate := range candidates {
		var stored domain.IdentityCredential
		err := t.tx.QueryRowContext(ctx, `SELECT source_id, identifier_value, identifier_hash, news_id
			FROM news_identifiers
			WHERE source_id = ? AND identifier_hash = ?
			FOR UPDATE`, sourceID, candidate.Hash,
		).Scan(&stored.SourceID, &stored.Value, &stored.Hash, &stored.NewsID)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		result = append(result, stored)
	}
	return result, nil
}

func (t *mysqlNewsTransaction) CreateNewsItem(ctx context.Context, item domain.NewsItem) error {
	if t == nil || t.tx == nil || strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ContentFingerprint) == "" {
		return app.ErrInvalidRequest
	}
	contentJSON, err := json.Marshal(item.Content)
	if err != nil {
		return err
	}
	_, err = t.tx.ExecContext(ctx, `INSERT INTO news_items
		(news_id, canonical_url, canonical_url_hash, title, publisher_name, publisher_domain,
		 author, published_at, content_json, content_fingerprint, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		item.ID, nullableString(item.CanonicalURL), nullableString(item.CanonicalURLHash),
		item.Title, item.PublisherName, item.PublisherDomain, item.Author, item.PublishedAt,
		contentJSON, item.ContentFingerprint, item.CreatedAt.UTC(), item.UpdatedAt.UTC(),
	)
	return err
}

func (t *mysqlNewsTransaction) UpdateNewsItem(ctx context.Context, item domain.NewsItem) error {
	if t == nil || t.tx == nil || strings.TrimSpace(item.ID) == "" || strings.TrimSpace(item.ContentFingerprint) == "" {
		return app.ErrInvalidRequest
	}
	contentJSON, err := json.Marshal(item.Content)
	if err != nil {
		return err
	}
	result, err := t.tx.ExecContext(ctx, `UPDATE news_items SET
		canonical_url = ?, canonical_url_hash = ?, title = ?, publisher_name = ?,
		publisher_domain = ?, author = ?, published_at = ?, content_json = ?,
		content_fingerprint = ?, updated_at = ?
		WHERE news_id = ?`,
		nullableString(item.CanonicalURL), nullableString(item.CanonicalURLHash),
		item.Title, item.PublisherName, item.PublisherDomain, item.Author, item.PublishedAt,
		contentJSON, item.ContentFingerprint, item.UpdatedAt.UTC(), item.ID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected != 1 {
		return fmt.Errorf("news storage: update current item %q affected %d rows", item.ID, affected)
	}
	return nil
}

func (t *mysqlNewsTransaction) SaveIdentityCredentials(ctx context.Context, credentials []domain.IdentityCredential) error {
	if t == nil || t.tx == nil {
		return app.ErrInvalidRequest
	}
	for _, credential := range credentials {
		_, observedOffset := credential.ObservedAt.Zone()
		sum := sha256.Sum256([]byte(credential.Value))
		if credential.ObservedAt.IsZero() || observedOffset != 0 ||
			credential.SourceID == "" || credential.Value == "" ||
			credential.Hash != hex.EncodeToString(sum[:]) || credential.NewsID == "" {
			return app.ErrInvalidRequest
		}
		if _, err := t.tx.ExecContext(ctx, `INSERT INTO news_identifiers
			(source_id, identifier_hash, identifier_value, news_id, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			credential.SourceID, credential.Hash, credential.Value, credential.NewsID, credential.ObservedAt.UTC(),
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
		(observation_id, idempotency_key, source_id, provider_code, query_dimension, endpoint_id,
		 source_policy_version_id, endpoint_policy_version_id, provider_output_policy_ref, provider_output_policy_version_id,
		 policy_evaluated_at, candidate_json, observed_at, retention_until, payload_purged_at,
		 identity_status, news_id, reason_code)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		observation.ID, observation.IdempotencyKey, observation.SourceID, observation.ProviderCode,
		observation.QueryDimension, observation.EndpointID, observation.SourcePolicyVersionID,
		observation.EndpointPolicyVersionID, observation.ProviderOutputPolicyRef, observation.ProviderOutputPolicyVersionID,
		policyEvaluatedAt, candidateJSON, observation.ObservedAt.UTC(),
		nullableTime(observation.RetentionUntil), nullableTimePointer(observation.PayloadPurgedAt),
		string(observation.IdentityStatus), newsID, observation.ReasonCode,
	)
	if insertErr == nil {
		return observation, nil
	}

	existing, lookupErr := scanObservation(t.tx.QueryRowContext(ctx, observationSelect+" WHERE idempotency_key = ? FOR UPDATE", observation.IdempotencyKey))
	if lookupErr != nil {
		return domain.Observation{}, errors.Join(insertErr, lookupErr)
	}
	if existing.PayloadPurgedAt != nil {
		return domain.Observation{}, app.ErrObservationPayloadPurged
	}
	if !app.SameObservationPayload(existing, observation) {
		return domain.Observation{}, app.ErrObservationIdempotencyConflict
	}
	return existing, nil
}

const observationSelect = `SELECT observation_id, idempotency_key, source_id, provider_code,
	query_dimension, endpoint_id, source_policy_version_id, endpoint_policy_version_id,
	provider_output_policy_ref, provider_output_policy_version_id, policy_evaluated_at, candidate_json, observed_at, retention_until, payload_purged_at,
	identity_status, news_id, reason_code
	FROM news_observations`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanObservation(row rowScanner) (domain.Observation, error) {
	var result domain.Observation
	var policyAt, retentionUntil, payloadPurgedAt sql.NullTime
	var newsID sql.NullString
	var candidateJSON []byte
	var status string
	err := row.Scan(
		&result.ID, &result.IdempotencyKey, &result.SourceID, &result.ProviderCode,
		&result.QueryDimension, &result.EndpointID, &result.SourcePolicyVersionID,
		&result.EndpointPolicyVersionID, &result.ProviderOutputPolicyRef, &result.ProviderOutputPolicyVersionID, &policyAt, &candidateJSON, &result.ObservedAt, &retentionUntil, &payloadPurgedAt,
		&status, &newsID, &result.ReasonCode,
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
	if retentionUntil.Valid {
		result.RetentionUntil = retentionUntil.Time.UTC()
	}
	if payloadPurgedAt.Valid {
		value := payloadPurgedAt.Time.UTC()
		result.PayloadPurgedAt = &value
		result.Candidate = domain.Candidate{}
	}
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

var _ app.AvailabilityEvidenceWriteTransaction = (*mysqlNewsTransaction)(nil)

// AppendAvailabilityEvidence 在当前策略锁事务内追加字段级授权通过的 Evidence。
// 父 Version、Observation 和授权版本均从数据库重新核对，不信任请求对象中的快照。
func (t *mysqlNewsTransaction) AppendAvailabilityEvidence(
	ctx context.Context,
	evidence domain.AvailabilityEvidence,
	observation domain.Observation,
	version domain.NewsItemVersion,
) error {
	if t == nil || t.tx == nil || ctx == nil || evidence.Validate() != nil ||
		evidence.NewsID != version.NewsID || evidence.VersionID != version.ID ||
		evidence.ObservationID != observation.ID || version.NewsID != observation.NewsID ||
		version.ObservationID != observation.ID {
		return app.ErrInvalidRequest
	}

	var newsID, observationID, sourceID, providerCode, endpointID, sourcePolicyVersionID string
	var endpointPolicyVersionID, providerPolicyRef, providerPolicyVersionID string
	var versionObservedAt, observationObservedAt, retentionUntil time.Time
	var payloadPurgedAt sql.NullTime
	err := t.tx.QueryRowContext(ctx, `SELECT v.news_id, v.observation_id, v.observed_at,
		o.source_id, o.provider_code, o.endpoint_id, o.source_policy_version_id,
		o.endpoint_policy_version_id, o.provider_output_policy_ref, o.provider_output_policy_version_id,
		o.observed_at, o.retention_until, o.payload_purged_at
		FROM news_item_versions v
		INNER JOIN news_observations o
			ON o.observation_id = v.observation_id AND o.news_id = v.news_id
		WHERE v.version_id = ? FOR UPDATE`, version.ID,
	).Scan(&newsID, &observationID, &versionObservedAt,
		&sourceID, &providerCode, &endpointID, &sourcePolicyVersionID,
		&endpointPolicyVersionID, &providerPolicyRef, &providerPolicyVersionID,
		&observationObservedAt, &retentionUntil, &payloadPurgedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return app.ErrInvalidRequest
	}
	if err != nil {
		return err
	}
	if payloadPurgedAt.Valid ||
		newsID != evidence.NewsID || observationID != evidence.ObservationID ||
		sourceID != observation.SourceID || providerCode != observation.ProviderCode ||
		endpointID != observation.EndpointID || sourcePolicyVersionID != observation.SourcePolicyVersionID ||
		endpointPolicyVersionID != observation.EndpointPolicyVersionID ||
		providerPolicyRef != observation.ProviderOutputPolicyRef ||
		providerPolicyVersionID != observation.ProviderOutputPolicyVersionID ||
		!versionObservedAt.UTC().Equal(version.ObservedAt.UTC()) ||
		!observationObservedAt.UTC().Equal(observation.ObservedAt.UTC()) ||
		!evidence.ObservedAt.Equal(observationObservedAt.UTC()) ||
		!retentionUntil.UTC().Equal(observation.RetentionUntil.UTC()) ||
		evidence.RetentionUntil.After(retentionUntil.UTC()) ||
		!retentionUntil.After(evidence.RecordedAt) {
		return app.ErrInvalidRequest
	}

	var existingNewsID, existingVersionID, existingObservationID, existingFieldPath, existingRawValue string
	var existingTimezone, existingPrecision, existingParseStatus, existingParseReasonCode, existingParserVersion string
	var existingEvidenceRef, existingEvidenceHash sql.NullString
	var existingCandidateAt, existingObservedAt sql.NullTime
	err = t.tx.QueryRowContext(ctx, `SELECT news_id, version_id, observation_id, field_path, raw_value,
		source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		candidate_available_at_utc, parser_version, evidence_ref, evidence_hash, observed_at
		FROM news_version_availability_evidence WHERE evidence_id = ? FOR UPDATE`, evidence.ID,
	).Scan(&existingNewsID, &existingVersionID, &existingObservationID, &existingFieldPath, &existingRawValue,
		&existingTimezone, &existingPrecision, &existingParseStatus, &existingParseReasonCode,
		&existingCandidateAt, &existingParserVersion, &existingEvidenceRef, &existingEvidenceHash,
		&existingObservedAt)
	if err == nil {
		if !samePersistedAvailabilityEvidence(existingNewsID, existingVersionID, existingObservationID,
			existingFieldPath, existingRawValue, existingTimezone, existingPrecision,
			existingParseStatus, existingParseReasonCode, existingParserVersion,
			existingCandidateAt, existingEvidenceRef, existingEvidenceHash, existingObservedAt, evidence) {
			return app.ErrAvailabilityEvidenceIDConflict
		}
		return nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	_, err = t.tx.ExecContext(ctx, `INSERT INTO news_version_availability_evidence
		(evidence_id, news_id, version_id, observation_id, field_path, raw_value,
		 source_timezone_or_offset, source_precision, parse_status, parse_reason_code,
		 candidate_available_at_utc, parser_version, evidence_ref, evidence_hash,
		 observed_at, recorded_at, retention_until)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		evidence.ID, evidence.NewsID, evidence.VersionID, evidence.ObservationID,
		evidence.FieldPath, evidence.RawValue, nullableString(evidence.SourceTimezoneOrOffset),
		string(evidence.Precision), string(evidence.ParseStatus), evidence.ParseReasonCode,
		nullableTimePointer(evidence.CandidateAvailableAt), evidence.ParserVersion,
		nullableStringPointer(evidence.EvidenceRef), nullableStringPointer(evidence.EvidenceHash),
		evidence.ObservedAt.UTC(), evidence.RecordedAt.UTC(), evidence.RetentionUntil.UTC(),
	)
	return err
}

func samePersistedAvailabilityEvidence(
	newsID, versionID, observationID, fieldPath, rawValue string,
	timezone, precision, parseStatus, parseReasonCode, parserVersion string,
	candidateAt sql.NullTime, evidenceRef, evidenceHash sql.NullString,
	observedAt sql.NullTime, evidence domain.AvailabilityEvidence,
) bool {
	if newsID != evidence.NewsID || versionID != evidence.VersionID ||
		observationID != evidence.ObservationID || fieldPath != evidence.FieldPath ||
		rawValue != evidence.RawValue || timezone != evidence.SourceTimezoneOrOffset ||
		precision != string(evidence.Precision) || parseStatus != string(evidence.ParseStatus) ||
		parseReasonCode != evidence.ParseReasonCode || parserVersion != evidence.ParserVersion ||
		!observedAt.Valid || !observedAt.Time.UTC().Equal(evidence.ObservedAt.UTC()) {
		return false
	}
	if !nullableStringMatches(evidenceRef, evidence.EvidenceRef) ||
		!nullableStringMatches(evidenceHash, evidence.EvidenceHash) {
		return false
	}
	if candidateAt.Valid != (evidence.CandidateAvailableAt != nil) {
		return false
	}
	return !candidateAt.Valid || candidateAt.Time.UTC().Equal(evidence.CandidateAvailableAt.UTC())
}

func nullableStringMatches(stored sql.NullString, request *string) bool {
	if request == nil {
		return !stored.Valid
	}
	return stored.Valid && stored.String == *request
}

func (r *MySQLRepository) ObservationRetentionBacklog(ctx context.Context, now time.Time) (int, *time.Time, error) {
	if r == nil || r.db == nil || ctx == nil || now.IsZero() {
		return 0, nil, app.ErrInvalidRequest
	}
	var count int
	var oldest sql.NullTime
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*), MIN(retention_until)
		FROM news_observations
		WHERE retention_until IS NOT NULL AND retention_until <= ? AND payload_purged_at IS NULL`, now.UTC()).Scan(&count, &oldest); err != nil {
		return 0, nil, err
	}
	if !oldest.Valid {
		return count, nil, nil
	}
	value := oldest.Time.UTC()
	return count, &value, nil
}

// PurgeExpiredObservationPayloads 清除到期候选载荷并保留 Observation tombstone。
// Scanned 仅统计本事务成功锁定的记录；SKIP LOCKED 跳过的行不计入。
// 被锁定记录必须全部更新成功，否则回滚整批，避免报告部分清理。
func (r *MySQLRepository) PurgeExpiredObservationPayloads(ctx context.Context, now time.Time, limit int) (app.ObservationPayloadPurgeResult, error) {
	if r == nil || r.db == nil || ctx == nil || now.IsZero() || limit < 1 || limit > 1000 {
		return app.ObservationPayloadPurgeResult{}, app.ErrInvalidRequest
	}
	result := app.ObservationPayloadPurgeResult{}
	err := r.withTransaction(ctx, "news_observation_retention", nil, func(tx *sql.Tx, _ func(string) error) error {
		// Availability Evidence 有独立 retention；不能等父 Observation 到期才清理，
		// 否则会在更短的来源许可期限结束后继续保留原始时间值。
		evidenceRows, err := tx.QueryContext(ctx, `SELECT evidence_id FROM news_version_availability_evidence
			WHERE retention_until <= ? AND payload_purged_at IS NULL
			ORDER BY retention_until, evidence_id LIMIT ? FOR UPDATE SKIP LOCKED`, now.UTC(), limit)
		if err != nil {
			return err
		}
		evidenceIDs := make([]string, 0, limit)
		for evidenceRows.Next() {
			var id string
			if err := evidenceRows.Scan(&id); err != nil {
				_ = evidenceRows.Close()
				return err
			}
			evidenceIDs = append(evidenceIDs, id)
		}
		evidenceRowsErr := evidenceRows.Err()
		evidenceCloseErr := evidenceRows.Close()
		if err := errors.Join(evidenceRowsErr, evidenceCloseErr); err != nil {
			return err
		}
		result.EvidenceScanned = len(evidenceIDs)
		if len(evidenceIDs) > 0 {
			placeholders := strings.TrimRight(strings.Repeat("?,", len(evidenceIDs)), ",")
			args := make([]any, 0, len(evidenceIDs)+1)
			args = append(args, now.UTC())
			for _, id := range evidenceIDs {
				args = append(args, id)
			}
			updated, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE news_version_availability_evidence
				SET raw_value = NULL, candidate_available_at_utc = NULL,
					evidence_ref = NULL, evidence_hash = NULL, payload_purged_at = ?
				WHERE payload_purged_at IS NULL AND evidence_id IN (%s)`, placeholders), args...)
			if err != nil {
				return err
			}
			affected, err := updated.RowsAffected()
			if err != nil {
				return err
			}
			if affected != int64(len(evidenceIDs)) {
				return app.ErrObservationRetentionResultInvalid
			}
			result.EvidencePurged = int(affected)
		}

		rows, err := tx.QueryContext(ctx, `SELECT observation_id FROM news_observations
			WHERE retention_until IS NOT NULL AND retention_until <= ? AND payload_purged_at IS NULL
			ORDER BY retention_until, observation_id LIMIT ? FOR UPDATE SKIP LOCKED`, now.UTC(), limit)
		if err != nil {
			return err
		}
		ids := make([]string, 0, limit)
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				_ = rows.Close()
				return err
			}
			ids = append(ids, id)
		}
		rowsErr := rows.Err()
		closeErr := rows.Close()
		if err := errors.Join(rowsErr, closeErr); err != nil {
			return err
		}
		result.Scanned = len(ids)
		if len(ids) == 0 {
			return nil
		}
		placeholders := strings.TrimRight(strings.Repeat("?,", len(ids)), ",")
		args := make([]any, 0, len(ids)+1)
		args = append(args, now.UTC())
		for _, id := range ids {
			args = append(args, id)
		}
		updated, err := tx.ExecContext(ctx, fmt.Sprintf(`UPDATE news_observations
			SET candidate_json = JSON_OBJECT(), payload_purged_at = ?
			WHERE payload_purged_at IS NULL AND observation_id IN (%s)`, placeholders), args...)
		if err != nil {
			return err
		}
		affected, err := updated.RowsAffected()
		if err != nil {
			return err
		}
		if affected != int64(len(ids)) {
			return app.ErrObservationRetentionResultInvalid
		}
		result.Purged = int(affected)
		return nil
	})
	if err != nil {
		return app.ObservationPayloadPurgeResult{}, err
	}
	return result, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.UTC()
}

func nullableTimePointer(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.UTC()
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
	"news_observations",
	"news_item_versions",
	"news_authorization_snapshots",
	"news_evidence_snapshots",
	"news_policy_versions",
	"news_current_policy_versions",
	"news_version_availability_evidence",
	"news_version_availability_decisions",
}

// Ping verifies the borrowed SQL pool without taking ownership of it.
func (r *MySQLRepository) Ping(ctx context.Context) error {
	if r == nil || r.db == nil || ctx == nil {
		return app.ErrInvalidRequest
	}
	return r.db.PingContext(ctx)
}

// RequiredSchemaTableCount reports the number of News-owned tables checked by MissingSchemaTables.
func (r *MySQLRepository) RequiredSchemaTableCount() int {
	return len(requiredNewsSchemaTables)
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
