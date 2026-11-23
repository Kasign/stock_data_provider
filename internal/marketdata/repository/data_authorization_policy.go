package repository

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/authorization"
)

var _ app.DataAuthorizationPolicyRepository = (*Store)(nil)

var (
	ErrDataAuthorizationPolicyUnavailable = errors.New("market-data authorization policy unavailable")
	ErrDataAuthorizationPolicyConflict    = errors.New("market-data authorization policy version conflict")
)

func (s *Store) PublishDataAuthorizationPolicyVersion(ctx context.Context, value authorization.PolicyVersion) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrDataAuthorizationPolicyUnavailable
	}
	if _, err := authorization.DecodePolicyVersion(value); err != nil {
		return err
	}
	sum := sha256.Sum256(value.Payload)
	actualHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(actualHash), []byte(value.PayloadHash)) != 1 {
		return authorization.ErrPolicyInvalid
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.ExecContext(ctx, `INSERT INTO market_data_authorization_policy_versions
(policy_type, policy_ref, policy_version, payload_json, payload_hash, created_at)
VALUES (?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE policy_version=VALUES(policy_version)`,
		value.PolicyType, value.PolicyRef, value.PolicyVersion, string(value.Payload), value.PayloadHash, value.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}

	var storedHash string
	if err := tx.QueryRowContext(ctx, `SELECT payload_hash FROM market_data_authorization_policy_versions
WHERE policy_type=? AND policy_ref=? AND policy_version=? FOR UPDATE`,
		value.PolicyType, value.PolicyRef, value.PolicyVersion).Scan(&storedHash); err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(value.PayloadHash)) != 1 {
		return ErrDataAuthorizationPolicyConflict
	}

	_, err = tx.ExecContext(ctx, `INSERT INTO market_data_current_authorization_policy_versions
(policy_type, policy_ref, policy_version, updated_at)
VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE policy_version=VALUES(policy_version), updated_at=VALUES(updated_at)`,
		value.PolicyType, value.PolicyRef, value.PolicyVersion, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	committed = true
	return nil
}

func (s *Store) ResolveCurrentDataAuthorizationPolicy(ctx context.Context) (authorization.CurrentPolicy, error) {
	if s == nil || s.db == nil || ctx == nil {
		return authorization.CurrentPolicy{}, ErrDataAuthorizationPolicyUnavailable
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	defer tx.Rollback()

	var value authorization.PolicyVersion
	var createdAt time.Time
	err = tx.QueryRowContext(ctx, `SELECT v.policy_type, v.policy_ref, v.policy_version,
v.payload_json, v.payload_hash, v.created_at
FROM market_data_current_authorization_policy_versions c
JOIN market_data_authorization_policy_versions v
ON v.policy_type=c.policy_type AND v.policy_ref=c.policy_ref AND v.policy_version=c.policy_version
WHERE c.policy_type=? AND c.policy_ref=?`,
		authorization.DataAccessPolicyType, authorization.DailyBarsV2PolicyRef).
		Scan(&value.PolicyType, &value.PolicyRef, &value.PolicyVersion, &value.Payload, &value.PayloadHash, &createdAt)
	if err != nil {
		return authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	value.CreatedAt = createdAt.UTC()
	policy, err := authorization.DecodePolicyVersion(value)
	if err != nil {
		return authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	sum := sha256.Sum256(value.Payload)
	actualHash := hex.EncodeToString(sum[:])
	if subtle.ConstantTimeCompare([]byte(actualHash), []byte(value.PayloadHash)) != 1 {
		return authorization.CurrentPolicy{}, ErrDataAuthorizationPolicyUnavailable
	}
	if err := tx.Commit(); err != nil {
		return authorization.CurrentPolicy{}, fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	return authorization.CurrentPolicy{Version: value, Policy: policy}, nil
}

// validateDataAuthorizationPolicySchema verifies both immutable history and current-pointer tables.
func (s *Store) validateDataAuthorizationPolicySchema(ctx context.Context) error {
	for table, columns := range map[string][]string{
		"market_data_authorization_policy_versions":          {"policy_type", "policy_ref", "policy_version", "payload_json", "payload_hash", "created_at"},
		"market_data_current_authorization_policy_versions":  {"policy_type", "policy_ref", "policy_version", "updated_at"},
		"market_data_authorization_decisions":               {"authorization_ref", "subject_ref", "resource_ref", "purpose_code", "policy_version", "decided_at", "expires_at", "decision_fingerprint", "authorization_json", "policy_snapshot_json", "record_hash"},
		"market_data_authorization_revocations":             {"authorization_ref", "revoked_at", "revoked_by", "reason_code"},
	} {
		for _, column := range columns {
			var count int
			if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns
WHERE table_schema=DATABASE() AND table_name=? AND column_name=?`, table, column).Scan(&count); err != nil {
				return fmt.Errorf("inspect %s.%s: %w", table, column, err)
			}
			if count != 1 {
				return fmt.Errorf("%s.%s is missing", table, column)
			}
		}
	}
	return nil
}
