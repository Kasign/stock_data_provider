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
	"example.com/stock-analysis-flow/internal/platform/observability"
)

var _ app.DataAuthorizationPolicyRepository = (*Store)(nil)

var (
	ErrDataAuthorizationPolicyUnavailable = errors.New("market-data authorization policy unavailable")
	ErrDataAuthorizationPolicyConflict    = app.ErrDataAuthorizationPolicyVersionConflict
)

func (s *Store) PublishDataAuthorizationPolicyVersion(
	ctx context.Context,
	expectedCurrentVersion string,
	value authorization.PolicyVersion,
	audit app.DataAuthorizationPolicyPublicationAudit,
) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrDataAuthorizationPolicyUnavailable
	}
	if _, err := authorization.DecodePolicyVersion(value); err != nil {
		return err
	}
	if audit.ActorRef != "principal:owner" || !observability.ValidRequestID(audit.RequestID) ||
		audit.OccurredAt.IsZero() || audit.OccurredAt.Location() != time.UTC {
		return authorization.ErrPolicyInvalid
	}
	if expectedCurrentVersion == value.PolicyVersion {
		return ErrDataAuthorizationPolicyConflict
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
VALUES (?, ?, ?, ?, ?, ?)`,
		value.PolicyType, value.PolicyRef, value.PolicyVersion, string(value.Payload), value.PayloadHash, value.CreatedAt.UTC())
	if err != nil {
		var existingVersion string
		lookupErr := tx.QueryRowContext(ctx, `SELECT policy_version FROM market_data_authorization_policy_versions
WHERE policy_type=? AND policy_ref=? AND policy_version=? FOR UPDATE`,
			value.PolicyType, value.PolicyRef, value.PolicyVersion).Scan(&existingVersion)
		if lookupErr == nil {
			// A historical version ID is never reusable, even with identical bytes.
			// Otherwise an old AuthorizationRef could become current again after rollback.
			return ErrDataAuthorizationPolicyConflict
		}
		return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}

	if expectedCurrentVersion == "" {
		_, err = tx.ExecContext(ctx, `INSERT INTO market_data_current_authorization_policy_versions
(policy_type, policy_ref, policy_version, updated_at)
VALUES (?, ?, ?, ?)`,
			value.PolicyType, value.PolicyRef, value.PolicyVersion, time.Now().UTC())
		if err != nil {
			var currentVersion string
			lookupErr := tx.QueryRowContext(ctx, `SELECT policy_version FROM market_data_current_authorization_policy_versions
WHERE policy_type=? AND policy_ref=? FOR UPDATE`, value.PolicyType, value.PolicyRef).Scan(&currentVersion)
			if lookupErr == nil {
				return ErrDataAuthorizationPolicyConflict
			}
			return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
		}
	} else {
		result, updateErr := tx.ExecContext(ctx, `UPDATE market_data_current_authorization_policy_versions
SET policy_version=?, updated_at=?
WHERE policy_type=? AND policy_ref=? AND policy_version=?`,
			value.PolicyVersion, time.Now().UTC(), value.PolicyType, value.PolicyRef, expectedCurrentVersion)
		if updateErr != nil {
			return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, updateErr)
		}
		affected, rowsErr := result.RowsAffected()
		if rowsErr != nil {
			return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, rowsErr)
		}
		if affected != 1 {
			return ErrDataAuthorizationPolicyConflict
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO market_data_authorization_policy_publications
(policy_type, policy_ref, previous_policy_version, new_policy_version, payload_hash, actor_ref, request_id, occurred_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		value.PolicyType, value.PolicyRef, nullablePolicyVersion(expectedCurrentVersion),
		value.PolicyVersion, value.PayloadHash, audit.ActorRef, audit.RequestID, audit.OccurredAt.UTC())
	if err != nil {
		return fmt.Errorf("%w: record policy publication audit: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationPolicyUnavailable, err)
	}
	committed = true
	return nil
}

func nullablePolicyVersion(value string) any {
	if value == "" {
		return nil
	}
	return value
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
	if errors.Is(err, sql.ErrNoRows) {
		return authorization.CurrentPolicy{}, app.ErrDataAuthorizationPolicyNotPublished
	}
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
		"market_data_authorization_policy_versions":         {"policy_type", "policy_ref", "policy_version", "payload_json", "payload_hash", "created_at"},
		"market_data_current_authorization_policy_versions": {"policy_type", "policy_ref", "policy_version", "updated_at"},
		"market_data_authorization_decisions":               {"authorization_ref", "subject_ref", "resource_ref", "purpose_code", "policy_version", "decided_at", "expires_at", "decision_fingerprint", "authorization_json", "policy_snapshot_json", "record_hash"},
		"market_data_authorization_revocations":             {"authorization_ref", "revoked_at", "revoked_by", "reason_code"},
		"market_data_authorization_policy_publications":     {"policy_type", "policy_ref", "previous_policy_version", "new_policy_version", "payload_hash", "actor_ref", "request_id", "occurred_at"},
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
