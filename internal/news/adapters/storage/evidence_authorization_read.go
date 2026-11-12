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

// GetEvidenceContentWithCurrentAuthorization 在同一 InnoDB 事务中锁定当前策略指针，
// 重新读取并校验 Observation，再执行授权回调和精确正文读取。事务提交成功后才返回正文。
func (r *MySQLRepository) GetEvidenceContentWithCurrentAuthorization(
	ctx context.Context,
	binding domain.NewsAuthorizationBinding,
	observation domain.Observation,
	version domain.NewsItemVersion,
	systemAsOf time.Time,
	authorize func(context.Context, domain.CurrentAuthorizationPolicySet, domain.Observation) error,
) (domain.ContentLayers, domain.Observation, error) {
	if r == nil || r.db == nil || ctx == nil || authorize == nil ||
		domain.ValidateNewsAuthorizationBindingForEvidence(binding, observation, version) != nil ||
		systemAsOf.IsZero() || !isEvidenceUTC(systemAsOf) || systemAsOf.After(time.Now().UTC()) {
		return domain.ContentLayers{}, domain.Observation{}, app.ErrInvalidEvidenceQuery
	}
	if err := ctx.Err(); err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
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
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	current, err := buildCurrentNewsAuthorizationPolicySet(binding, loaded, time.Now().UTC())
	if err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}

	// 元数据定位发生在事务外；这里重新读取 Observation 与 Version，防止并发策略轨迹
	// 修改、载荷清理或版本变更在预检之后、正文读取之前形成 TOCTOU。
	record, err := scanEvidencePITRecord(tx.QueryRowContext(ctx, evidencePITSelect+` WHERE
		v.news_id = ? AND v.version_id = ? AND v.observed_at <= ? AND o.observed_at <= ?`,
		version.NewsID, version.ID, systemAsOf.UTC(), systemAsOf.UTC(),
	))
	if errors.Is(err, sql.ErrNoRows) || errors.Is(err, app.ErrEvidencePITNotFound) {
		return domain.ContentLayers{}, domain.Observation{}, app.ErrEvidencePITNotFound
	}
	if err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	currentObservation := record.Observation
	currentVersion := record.Version
	if currentObservation.ID != observation.ID || currentObservation.NewsID != observation.NewsID ||
		!currentObservation.ObservedAt.Equal(observation.ObservedAt) ||
		currentVersion.ID != version.ID || currentVersion.NewsID != version.NewsID ||
		currentVersion.VersionNo != version.VersionNo ||
		currentVersion.ObservationID != version.ObservationID ||
		currentVersion.Fingerprint != version.Fingerprint ||
		!currentVersion.ObservedAt.Equal(version.ObservedAt) ||
		domain.ValidateNewsAuthorizationBindingForEvidence(binding, currentObservation, currentVersion) != nil {
		return domain.ContentLayers{}, domain.Observation{}, app.ErrInvalidEvidenceQuery
	}

	if err := authorize(ctx, current, currentObservation); err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	if err := ctx.Err(); err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
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
		currentVersion.NewsID, currentVersion.ID, systemAsOf.UTC(), systemAsOf.UTC(),
	).Scan(&payload, &storedFingerprint, &storedObservationID, &versionObservedAt, &observationObservedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ContentLayers{}, domain.Observation{}, app.ErrEvidencePITNotFound
	}
	if err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	if storedFingerprint != currentVersion.Fingerprint || storedObservationID != currentObservation.ID ||
		!versionObservedAt.UTC().Equal(currentVersion.ObservedAt.UTC()) ||
		!observationObservedAt.UTC().Equal(currentObservation.ObservedAt.UTC()) {
		return domain.ContentLayers{}, domain.Observation{}, app.ErrInvalidEvidenceQuery
	}
	var content domain.ContentLayers
	if len(payload) == 0 || json.Unmarshal(payload, &content) != nil {
		return domain.ContentLayers{}, domain.Observation{}, app.ErrInvalidEvidenceQuery
	}
	if err := ctx.Err(); err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	if err := tx.Commit(); err != nil {
		return domain.ContentLayers{}, domain.Observation{}, err
	}
	committed = true
	return content, currentObservation, nil
}
