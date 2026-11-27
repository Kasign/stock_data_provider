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

// authorizedSnapshotRow 是共享快照表的一行原始列值。
// 它不携带 payload_kind：类型已由读取时的 WHERE 条件确定，读出的行不得再按内容猜类型。
type authorizedSnapshotRow struct {
	manifestFingerprint string
	payloadFingerprint  string
	manifestVersion     string
	payloadVersion      string
	manifestJSON        []byte
	payloadJSON         []byte
}

// ReadDataAuthorizedDailyBarsCloseSeriesSnapshot 将授权决策、撤销状态、当前策略和 Snapshot
// 固定在同一个 MariaDB 一致性读事务中，避免检查撤销后再读取 Payload 的竞态。
func (s *Store) ReadDataAuthorizedDailyBarsCloseSeriesSnapshot(
	ctx context.Context, authorizationRef, snapshotRef string,
) (app.DataAuthorizationDecision, app.DailyBarsCloseSeriesSnapshot, authorization.CurrentPolicy, error) {
	if s == nil || s.db == nil || ctx == nil || !validDataAuthorizationOpaque(authorizationRef) {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	if _, err := parseDataSnapshotRef(snapshotRef); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: begin authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	defer tx.Rollback()

	decision, currentPolicy, err := readAuthorizedSnapshotDecision(
		ctx, tx, authorizationRef, authorization.DailyBarsV2PolicyRef,
	)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, err
	}
	row, err := readAuthorizedSnapshotRow(ctx, tx, snapshotRef, app.DailyBarsCloseSeriesPayloadKind)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, err
	}

	var snapshot app.DailyBarsCloseSeriesSnapshot
	if err := json.Unmarshal(row.manifestJSON, &snapshot.Manifest); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	if err := json.Unmarshal(row.payloadJSON, &snapshot.Payload); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	snapshot.PayloadFingerprint = row.payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != row.manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != row.manifestVersion ||
		snapshot.Payload.SchemaVersion != row.payloadVersion {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	if err := snapshot.Verify(); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrDailyBarsCloseSeriesSnapshotConflict, err)
	}
	canonicalManifest, canonicalPayload, err := canonicalDailyBarsCloseSeriesSnapshotJSON(snapshot)
	if err != nil || !bytes.Equal(canonicalManifest, row.manifestJSON) || !bytes.Equal(canonicalPayload, row.payloadJSON) {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, ErrDailyBarsCloseSeriesSnapshotConflict
	}
	if err := tx.Commit(); err != nil {
		return app.DataAuthorizationDecision{}, app.DailyBarsCloseSeriesSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: commit authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	return decision, snapshot, currentPolicy, nil
}

// ReadDataAuthorizedSecurityTradingStatusFactsSnapshot 与 close-series 读取共享同一事务边界与
// 授权校验顺序，区别只有两点：策略引用必须是交易事实数据类，数据行必须按 facts kind 读取。
//
// 两点都必须显式校验：仅凭 snapshot_ref 无法区分 payload 类型，仅凭 policy_version 也无法
// 区分数据类；任何一项缺省都会让 close-series 的授权被当成交易事实授权使用。
func (s *Store) ReadDataAuthorizedSecurityTradingStatusFactsSnapshot(
	ctx context.Context, authorizationRef, snapshotRef string,
) (app.DataAuthorizationDecision, app.SecurityTradingStatusFactsSnapshot, authorization.CurrentPolicy, error) {
	if s == nil || s.db == nil || ctx == nil || !validDataAuthorizationOpaque(authorizationRef) {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	if _, err := parseDataSnapshotRef(snapshotRef); err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: begin authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	defer tx.Rollback()

	decision, currentPolicy, err := readAuthorizedSnapshotDecision(
		ctx, tx, authorizationRef, authorization.SecurityStatusFactsPolicyRef,
	)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	row, err := readAuthorizedSnapshotRow(ctx, tx, snapshotRef, app.SecurityTradingStatusFactsPayloadKind)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}

	var snapshot app.SecurityTradingStatusFactsSnapshot
	if err := json.Unmarshal(row.manifestJSON, &snapshot.Manifest); err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, ErrSecurityTradingStatusFactsSnapshotConflict
	}
	if err := json.Unmarshal(row.payloadJSON, &snapshot.Payload); err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, ErrSecurityTradingStatusFactsSnapshotConflict
	}
	snapshot.PayloadFingerprint = row.payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != row.manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != row.manifestVersion ||
		snapshot.Payload.SchemaVersion != row.payloadVersion {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, ErrSecurityTradingStatusFactsSnapshotConflict
	}
	if err := snapshot.Verify(); err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrSecurityTradingStatusFactsSnapshotConflict, err)
	}
	canonicalManifest, canonicalPayload, err := canonicalSecurityTradingStatusFactsSnapshotJSON(snapshot)
	if err != nil || !bytes.Equal(canonicalManifest, row.manifestJSON) || !bytes.Equal(canonicalPayload, row.payloadJSON) {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, ErrSecurityTradingStatusFactsSnapshotConflict
	}
	if err := tx.Commit(); err != nil {
		return app.DataAuthorizationDecision{}, app.SecurityTradingStatusFactsSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: commit authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	return decision, snapshot, currentPolicy, nil
}

// ReadDataAuthorizedOrderBookFactsSnapshot 与交易事实读取共享同一事务边界，
// 但策略引用必须是 order-book-facts-access，数据行必须按 order_book kind 读取。
// 两项任一缺省都会让其它数据类的授权被误用为日内盘口事实授权。
func (s *Store) ReadDataAuthorizedOrderBookFactsSnapshot(
	ctx context.Context, authorizationRef, snapshotRef string,
) (app.DataAuthorizationDecision, app.OrderBookFactsSnapshot, authorization.CurrentPolicy, error) {
	if s == nil || s.db == nil || ctx == nil || !validDataAuthorizationOpaque(authorizationRef) {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	if _, err := parseDataSnapshotRef(snapshotRef); err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: begin authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	defer tx.Rollback()

	decision, currentPolicy, err := readAuthorizedSnapshotDecision(
		ctx, tx, authorizationRef, authorization.OrderBookFactsPolicyRef,
	)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	row, err := readAuthorizedSnapshotRow(ctx, tx, snapshotRef, app.OrderBookFactsPayloadKind)
	if err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, err
	}
	var snapshot app.OrderBookFactsSnapshot
	if err := json.Unmarshal(row.manifestJSON, &snapshot.Manifest); err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, ErrOrderBookFactsSnapshotConflict
	}
	if err := json.Unmarshal(row.payloadJSON, &snapshot.Payload); err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, ErrOrderBookFactsSnapshotConflict
	}
	snapshot.PayloadFingerprint = row.payloadFingerprint
	if snapshot.Manifest.SnapshotRef != snapshotRef ||
		snapshot.Manifest.ManifestFingerprint != row.manifestFingerprint ||
		snapshot.Manifest.SchemaVersion != row.manifestVersion ||
		snapshot.Payload.SchemaVersion != row.payloadVersion {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, ErrOrderBookFactsSnapshotConflict
	}
	if err := snapshot.Verify(); err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrOrderBookFactsSnapshotConflict, err)
	}
	canonicalManifest, canonicalPayload, err := canonicalOrderBookFactsSnapshotJSON(snapshot)
	if err != nil || !bytes.Equal(canonicalManifest, row.manifestJSON) || !bytes.Equal(canonicalPayload, row.payloadJSON) {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, ErrOrderBookFactsSnapshotConflict
	}
	if err := tx.Commit(); err != nil {
		return app.DataAuthorizationDecision{}, app.OrderBookFactsSnapshot{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: commit authorized snapshot read: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	return decision, snapshot, currentPolicy, nil
}

// readAuthorizedSnapshotDecision 校验授权决策自身、撤销状态，并按数据类解析当前策略。
// policyRef 决定读到哪一套策略：不同数据类的策略必须分开发布、分开撤销。
func readAuthorizedSnapshotDecision(
	ctx context.Context, tx *sql.Tx, authorizationRef, policyRef string,
) (app.DataAuthorizationDecision, authorization.CurrentPolicy, error) {
	var authJSON, policyJSON []byte
	var storedHash string
	err := tx.QueryRowContext(ctx, `SELECT authorization_json, policy_snapshot_json, record_hash
FROM market_data_authorization_decisions WHERE authorization_ref=?`, authorizationRef).Scan(&authJSON, &policyJSON, &storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	if err != nil {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: read authorization decision: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	decision := app.DataAuthorizationDecision{AuthorizationRef: authJSON, PolicySnapshot: policyJSON}
	ref, _, _, canonicalAuth, canonicalPolicy, recordHash, err := validateDataAuthorizationDecision(decision)
	if err != nil || ref.AuthorizationRef != authorizationRef ||
		subtle.ConstantTimeCompare([]byte(storedHash), []byte(recordHash)) != 1 ||
		!bytes.Equal(authJSON, canonicalAuth) || !bytes.Equal(policyJSON, canonicalPolicy) {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionInvalid
	}
	var revoked int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data_authorization_revocations WHERE authorization_ref=?`, authorizationRef).Scan(&revoked); err != nil {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: read revocation state: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	if revoked != 0 {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, ErrDataAuthorizationDecisionRevoked
	}

	var version authorization.PolicyVersion
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT v.policy_type, v.policy_ref, v.policy_version, v.payload_json, v.payload_hash, v.created_at
FROM market_data_current_authorization_policy_versions c
JOIN market_data_authorization_policy_versions v
ON v.policy_type=c.policy_type AND v.policy_ref=c.policy_ref AND v.policy_version=c.policy_version
WHERE c.policy_type=? AND c.policy_ref=?`, authorization.DataAccessPolicyType, policyRef).
		Scan(&version.PolicyType, &version.PolicyRef, &version.PolicyVersion, &version.Payload, &version.PayloadHash, &createdAt)
	if err != nil {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, fmt.Errorf("%w: read current authorization policy: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	version.CreatedAt = createdAt.UTC()
	currentPolicy, err := authorization.DecodePolicyVersionFor(version, policyRef)
	if err != nil {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, ErrDataAuthorizationPolicyUnavailable
	}
	sum := sha256.Sum256(version.Payload)
	if subtle.ConstantTimeCompare([]byte(hex.EncodeToString(sum[:])), []byte(version.PayloadHash)) != 1 {
		return app.DataAuthorizationDecision{}, authorization.CurrentPolicy{}, ErrDataAuthorizationPolicyUnavailable
	}
	return decision, authorization.CurrentPolicy{Version: version, Policy: currentPolicy}, nil
}

// readAuthorizedSnapshotRow 按引用与 payload_kind 精确读取一行。
// kind 条件不可省：SnapshotRef 的前缀对所有类型相同，只有该列能阻止跨类型串读。
func readAuthorizedSnapshotRow(
	ctx context.Context, tx *sql.Tx, snapshotRef, payloadKind string,
) (authorizedSnapshotRow, error) {
	var row authorizedSnapshotRow
	err := tx.QueryRowContext(ctx, `SELECT manifest_fingerprint, payload_fingerprint, manifest_schema_version,
payload_schema_version, manifest_json, payload_json
FROM market_data_daily_bars_close_series_snapshots WHERE snapshot_ref=? AND payload_kind=?`,
		snapshotRef, payloadKind).Scan(
		&row.manifestFingerprint, &row.payloadFingerprint, &row.manifestVersion, &row.payloadVersion, &row.manifestJSON, &row.payloadJSON)
	if errors.Is(err, sql.ErrNoRows) {
		switch payloadKind {
		case app.SecurityTradingStatusFactsPayloadKind:
			return authorizedSnapshotRow{}, fmt.Errorf("%w: %s", ErrSecurityTradingStatusFactsSnapshotNotFound, snapshotRef)
		case app.OrderBookFactsPayloadKind:
			return authorizedSnapshotRow{}, fmt.Errorf("%w: %s", ErrOrderBookFactsSnapshotNotFound, snapshotRef)
		default:
			return authorizedSnapshotRow{}, fmt.Errorf("%w: %s", ErrDailyBarsCloseSeriesSnapshotNotFound, snapshotRef)
		}
	}
	if err != nil {
		return authorizedSnapshotRow{}, fmt.Errorf("read authorized Snapshot payload: %w", err)
	}
	return row, nil
}
