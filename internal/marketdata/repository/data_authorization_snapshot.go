package repository

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

// ReadDataAuthorizedDailyBarsCloseSeriesSnapshot 将授权决策、撤销状态、当前策略和 Snapshot
// 固定在同一个 MariaDB 一致性读事务中，避免检查撤销后再读取 Payload 的竞态。
func (s *Store) ReadDataAuthorizedDailyBarsCloseSeriesSnapshot(
	ctx context.Context, authorizationRef, snapshotRef string,
) (app.DataAuthorizationDecision, app.DailyBarsCloseSeriesSnapshot, authorization.CurrentPolicy, error) {
	if s == nil || s.db == nil || ctx == nil || !validDataAuthorizationOpaque(authorizationRef) {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	if _, err := parseDailyBarsCloseSeriesSnapshotRef(snapshotRef); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: begin authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	defer tx.Rollback()

	var authJSON, policyJSON []byte
	var storedHash string
	err = tx.QueryRowContext(ctx, `SELECT authorization_json, policy_snapshot_json, record_hash
FROM market_data_authorization_decisions WHERE authorization_ref=?`, authorizationRef).Scan(&authJSON, &policyJSON, &storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: read authorization decision: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	decision := app.DataAuthorizationDecision{AuthorizationRef: authJSON, PolicySnapshot: policyJSON}
	ref, _, _, canonicalAuth, canonicalPolicy, recordHash, err := validateDataAuthorizationDecision(decision)
	if err != nil || ref.AuthorizationRef != authorizationRef ||
		subtle.ConstantTimeCompare([]byte(storedHash), []byte(recordHash)) != 1 ||
		!bytes.Equal(authJSON, canonicalAuth) || !bytes.Equal(policyJSON, canonicalPolicy) {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	var revoked int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data_authorization_revocations WHERE authorization_ref=?`, authorizationRef).Scan(&revoked); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: read revocation state: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	if revoked != 0 {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionRevoked
	}

	var version authorization.PolicyVersion
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT v.policy_type, v.policy_ref, v.policy_version, v.payload_json, v.payload_hash, v.created_at
FROM market_data_current_authorization_policy_versions c
JOIN market_data_authorization_policy_versions v
ON v.policy_type=c.policy_type AND v.policy_ref=c.policy_ref AND v.policy_version=c.policy_version
WHERE c.policy_type=? AND c.policy_ref=?`, authorization.DataAccessPolicyType, authorization.DailyBarsV2PolicyRef).
		Scan(&version.PolicyType, &version.PolicyRef, &version.PolicyVersion, &version.Payload, &version.PayloadHash, &createdAt)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: read current authorization policy: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	version.CreatedAt = createdAt.UTC()
	currentPolicy, err := authorization.DecodePolicyVersion(version)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationPolicyUnavailable
	}
	sum := sha256.Sum256(version.Payload)
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(version.PayloadHash)) != 1 {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationPolicyUnavailable
	}

	var manifestFingerprint, payloadFingerprint, manifestVersion, payloadVersion string
	var manifestJSON, payloadJSON []byte
	err = tx.QueryRowContext(ctx, `SELECT manifest_fingerprint, payload_fingerprint, manifest_schema_version,
payload_schema_version, manifest_json, payload_json
FROM market_data_daily_bars_close_series_snapshots WHERE snapshot_ref=?`, snapshotRef).Scan(
		&manifestFingerprint, &payloadFingerprint, &manifestVersion, &payloadVersion, &manifestJSON, &payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotNotFound
	}
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("read authorized Snapshot payload: %w", err)
	}
	var snapshot app.DailyBarsCloseSeriesSnapshot
	if err := json.Unmarshal(manifestJSON, &snapshot.Manifest); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	if err := json.Unmarshal(payloadJSON, &snapshot.Payload); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	snapshot.PayloadFingerprint = payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != manifestVersion ||
		snapshot.Payload.SchemaVersion != payloadVersion {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	if err := snapshot.Verify(); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrDailyBarsCloseSeriesSnapshotConflict, err)
	}
	canonicalManifest, canonicalPayload, err := canonicalDailyBarsCloseSeriesSnapshotJSON(snapshot)
	if err != nil || !bytes.Equal(canonicalManifest, manifestJSON) || !bytes.Equal(canonicalPayload, payloadJSON) {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	if err := tx.Commit(); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: commit authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	return decision, snapshot, authorization.CurrentPolicy{Version: version, Policy: currentPolicy}, nil
}
