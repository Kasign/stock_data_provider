package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

var _ app.EvidenceAtomicContentRepository = (*MySQLRepository)(nil)

// GetEvidenceContentWithCurrentAuthorization 在同一 InnoDB 事务中锁定完整当前策略指针，
// 执行授权回调并读取精确 Evidence 正文。事务提交失败时正文不会返回给调用方。
func (r *MySQLRepository) GetEvidenceContentWithCurrentAuthorization(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
	observation domain.Observation,
	version domain.NewsItemVersion,
	systemAsOf time.Time,
	authorize func(context.Context, domain.CurrentAuthorizationPolicySet) error,
) (domain.ContentLayers, error) {
	if r == nil || r.db == nil || ctx == nil || authorize == nil ||
		domain.ValidateNewsAuthorizationBindingForEvidence(binding, observation, version) != nil ||
		systemAsOf.IsZero() || !isEvidenceUTC(systemAsOf) || systemAsOf.After(time.Now().UTC()) {
		return domain.ContentLayers{}, app.ErrInvalidEvidenceQuery
	}
	if err := ctx.Err(); err != nil {
		return domain.ContentLayers{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ContentLayers{}, err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// Binding 的策略 tuple 已在 Domain 校验中按 (policy_type, policy_ref) 严格排序。
	// 锁住当前指针行后，发布/撤销事务更新同一行必须等待本次读取提交。
	loaded, err := loadCurrentNewsPolicyVersions(ctx, tx, binding, true)
	if err != nil {
		return domain.ContentLayers{}, err
	}
	current, err := buildCurrentNewsAuthorizationPolicySet(binding, loaded, time.Now().UTC())
	if err != nil {
		return domain.ContentLayers{}, err
	}
	if err := authorize(ctx, current); err != nil {
		return domain.ContentLayers{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.ContentLayers{}, err
	}

	var payload []byte
	var storedFingerprint, storedObservationID string
	var versionObservedAt, observationObservedAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT v.content_json, v.fingerprint, v.observation_id,
		v.observed_at, o.observed_at
		FROM news_item_versions v
		INNER JOIN news_observations o
			ON o.observation_id = v.observation_id AND o.news_id = v.news_id
		WHERE v.news_id = ? AND v.version_id = ?
			AND v.observed_at <= ? AND o.observed_at <= ?`,
		version.NewsID, version.ID, systemAsOf.UTC(), systemAsOf.UTC(),
	).Scan(&payload, &storedFingerprint, &storedObservationID, &versionObservedAt, &observationObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ContentLayers{}, app.ErrEvidencePITNotFound
	}
	if err != nil {
		return domain.ContentLayers{}, err
	}
	if storedFingerprint != version.Fingerprint || storedObservationID != observation.ID ||
		!versionObservedAt.UTC().Equal(version.ObservedAt.UTC()) ||
		!observationObservedAt.UTC().Equal(observation.ObservedAt.UTC()) {
		return domain.ContentLayers{}, app.ErrInvalidEvidenceQuery
	}
	var content domain.ContentLayers
	if len(payload) == 0 || json.Unmarshal(payload, &content) != nil {
		return domain.ContentLayers{}, app.ErrInvalidEvidenceQuery
	}
	if err := ctx.Err(); err != nil {
		return domain.ContentLayers{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ContentLayers{}, err
	}
	committed = true
	return content, nil
}
