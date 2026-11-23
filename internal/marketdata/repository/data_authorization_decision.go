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
	"strings"
	"time"

	platformauthorization "example.com/stock-analysis-flow/internal/platform/authorization"
)

var (
	ErrDataAuthorizationDecisionInvalid = errors.New("market-data authorization decision invalid")
	ErrDataAuthorizationDecisionConflict = errors.New("market-data authorization decision conflict")
	ErrDataAuthorizationDecisionRevoked = errors.New("market-data authorization decision revoked")
	ErrDataAuthorizationDecisionUnavailable = errors.New("market-data authorization decision unavailable")
)

const maxDataAuthorizationObjectBytes = 64 * 1024

type DataAuthorizationDecision struct {
	AuthorizationRef json.RawMessage
	PolicySnapshot     json.RawMessage
}

type storedDataAuthorizationRef struct {
	AuthorizationRef     string   `json:"authorization_ref"`
	PolicyOwner          string   `json:"policy_owner"`
	PolicyVersion        string   `json:"policy_version"`
	Decision             string   `json:"decision"`
	PurposeCode          string   `json:"purpose_code"`
	ContentLevel         string   `json:"content_level"`
	SubjectRef           string   `json:"subject_ref"`
	ResourceRef          string   `json:"resource_ref"`
	DecidedAt            string   `json:"decided_at"`
	ExpiresAt            *string  `json:"expires_at"`
	Conditions           []string `json:"conditions"`
	DecisionFingerprint  string   `json:"decision_fingerprint"`
	SchemaVersion        string   `json:"schema_version"`
}

// AppendDataAuthorizationDecision 保存完整 GOV-CTR-006 决策对象；重复写入必须逐字节等价。
// 指纹由 Platform 通用 GOV-CTR-006 验证器计算，不信任调用方附带的 fingerprint_input。
func (s *Store) AppendDataAuthorizationDecision(ctx context.Context, decision DataAuthorizationDecision) error {
	if s == nil || s.db == nil || ctx == nil {
		return ErrDataAuthorizationDecisionUnavailable
	}
	ref, decidedAt, expiresAt, authJSON, policyJSON, recordHash, err := validateDataAuthorizationDecision(decision)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO market_data_authorization_decisions
(authorization_ref, subject_ref, resource_ref, purpose_code, policy_version, decided_at, expires_at,
 decision_fingerprint, authorization_json, policy_snapshot_json, record_hash)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON DUPLICATE KEY UPDATE authorization_ref=VALUES(authorization_ref)`,
		ref.AuthorizationRef, ref.SubjectRef, ref.ResourceRef, ref.PurposeCode, ref.PolicyVersion,
		decidedAt, expiresAt, ref.DecisionFingerprint, authJSON, policyJSON, recordHash)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	var storedHash string
	if err := s.db.QueryRowContext(ctx, `SELECT record_hash FROM market_data_authorization_decisions WHERE authorization_ref=?`, ref.AuthorizationRef).Scan(&storedHash); err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	if subtle.ConstantTimeCompare([]byte(storedHash), []byte(recordHash)) != 1 {
		return ErrDataAuthorizationDecisionConflict
	}
	return nil
}

// RevokeDataAuthorizationDecision 追加撤销记录，不修改授权原件；撤销不可逆且幂等。
func (s *Store) RevokeDataAuthorizationDecision(ctx context.Context, authorizationRef, revokedBy, reasonCode string, revokedAt time.Time) error {
	if s == nil || s.db == nil || ctx == nil || !validDataAuthorizationOpaque(authorizationRef) ||
		!validDataAuthorizationOpaque(revokedBy) || !validDataAuthorizationReason(reasonCode) || revokedAt.IsZero() {
		return ErrDataAuthorizationDecisionInvalid
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO market_data_authorization_revocations
(authorization_ref, revoked_at, revoked_by, reason_code) VALUES (?, ?, ?, ?)
ON DUPLICATE KEY UPDATE authorization_ref=VALUES(authorization_ref)`,
		authorizationRef, revokedAt.UTC(), revokedBy, reasonCode)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "foreign key") {
			return ErrDataAuthorizationDecisionInvalid
		}
		return fmt.Errorf("%w: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	var storedBy, storedReason string
	var storedAt time.Time
	if err := s.db.QueryRowContext(ctx, `SELECT revoked_at, revoked_by, reason_code FROM market_data_authorization_revocations WHERE authorization_ref=?`, authorizationRef).Scan(&storedAt, &storedBy, &storedReason); err != nil {
		return fmt.Errorf("%w: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	if !storedAt.Equal(revokedAt.UTC()) || storedBy != revokedBy || storedReason != reasonCode {
		return ErrDataAuthorizationDecisionConflict
	}
	return nil
}

// GetDataAuthorizationDecision 在返回前重新验证 JSON 指纹、记录哈希和撤销状态。
// 当前策略版本/有效期/调用主体的动态判断由上层 Owner Application 执行。
func (s *Store) GetDataAuthorizationDecision(ctx context.Context, authorizationRef string) (DataAuthorizationDecision, error) {
	if s == nil || s.db == nil || ctx == nil || !validDataAuthorizationOpaque(authorizationRef) {
		return DataAuthorizationDecision{}, ErrDataAuthorizationDecisionInvalid
	}
	var authJSON, policyJSON []byte
	var storedHash string
	err := s.db.QueryRowContext(ctx, `SELECT authorization_json, policy_snapshot_json, record_hash
FROM market_data_authorization_decisions WHERE authorization_ref=?`, authorizationRef).Scan(&authJSON, &policyJSON, &storedHash)
	if errors.Is(err, sql.ErrNoRows) {
		return DataAuthorizationDecision{}, ErrDataAuthorizationDecisionInvalid
	}
	if err != nil { return DataAuthorizationDecision{}, fmt.Errorf("%w: %v", ErrDataAuthorizationDecisionUnavailable, err) }
	decision := DataAuthorizationDecision{AuthorizationRef: authJSON, PolicySnapshot: policyJSON}
	ref, _, _, canonicalAuth, canonicalPolicy, recordHash, err := validateDataAuthorizationDecision(decision)
	if err != nil || ref.AuthorizationRef != authorizationRef ||
		subtle.ConstantTimeCompare([]byte(storedHash), []byte(recordHash)) != 1 ||
		!bytes.Equal(authJSON, canonicalAuth) || !bytes.Equal(policyJSON, canonicalPolicy) {
		return DataAuthorizationDecision{}, ErrDataAuthorizationDecisionInvalid
	}
	var revoked int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM market_data_authorization_revocations WHERE authorization_ref=?`, authorizationRef).Scan(&revoked); err != nil {
		return DataAuthorizationDecision{}, fmt.Errorf("%w: %v", ErrDataAuthorizationDecisionUnavailable, err)
	}
	if revoked != 0 { return DataAuthorizationDecision{}, ErrDataAuthorizationDecisionRevoked }
	return decision, nil
}

func validateDataAuthorizationDecision(value DataAuthorizationDecision) (storedDataAuthorizationRef, time.Time, any, []byte, []byte, string, error) {
	var ref storedDataAuthorizationRef
	if len(value.AuthorizationRef) == 0 || len(value.AuthorizationRef) > maxDataAuthorizationObjectBytes ||
		len(value.PolicySnapshot) == 0 || len(value.PolicySnapshot) > maxDataAuthorizationObjectBytes {
		return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
	}
	if err := json.Unmarshal(value.AuthorizationRef, &ref); err != nil ||
		ref.AuthorizationRef == "" || ref.PolicyOwner != "data" || ref.Decision != "allowed" ||
		!validDataAuthorizationPurpose(ref.PurposeCode) || ref.ContentLevel != "full" ||
		!validDataAuthorizationOpaque(ref.PolicyVersion) || !strings.HasPrefix(ref.SubjectRef, "principal:") ||
		!validDataAuthorizationOpaque(ref.SubjectRef) || !strings.HasPrefix(ref.ResourceRef, "data_snapshot:sha256:") ||
		!validDataAuthorizationOpaque(ref.ResourceRef) || ref.SchemaVersion != "1.0.0" ||
		ref.Conditions == nil || len(ref.Conditions) != 0 || !validDataAuthorizationFingerprint(ref.DecisionFingerprint) {
		return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
	}
	if err := platformauthorization.VerifyFingerprint(value.AuthorizationRef, value.PolicySnapshot); err != nil {
		return ref, time.Time{}, nil, nil, nil, "", fmt.Errorf("%w: %v", ErrDataAuthorizationDecisionInvalid, err)
	}
	decidedAt, err := time.Parse(time.RFC3339Nano, ref.DecidedAt)
	if err != nil || decidedAt.Location() != time.UTC || decidedAt.Format(time.RFC3339Nano) != ref.DecidedAt {
		return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
	}
	var expiresAt any
	if ref.ExpiresAt != nil {
		parsed, parseErr := time.Parse(time.RFC3339Nano, *ref.ExpiresAt)
		if parseErr != nil || !parsed.After(decidedAt) || parsed.Location() != time.UTC || parsed.Format(time.RFC3339Nano) != *ref.ExpiresAt {
			return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
		}
		expiresAt = parsed
	}
	authJSON, err := json.Marshal(ref)
	if err != nil {
		return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
	}
	var policy any
	if err := json.Unmarshal(value.PolicySnapshot, &policy); err != nil {
		return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
	}
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
	}
	// Require canonical JSON bytes so duplicate keys, alternate encodings and unknown shape drift cannot be silently normalized.
	if !bytes.Equal(bytes.TrimSpace(value.AuthorizationRef), authJSON) || !bytes.Equal(bytes.TrimSpace(value.PolicySnapshot), policyJSON) {
		return ref, time.Time{}, nil, nil, nil, "", ErrDataAuthorizationDecisionInvalid
	}
	hash := sha256.New()
	hash.Write(authJSON)
	hash.Write([]byte{0})
	hash.Write(policyJSON)
	recordHash := hex.EncodeToString(hash.Sum(nil))
	return ref, decidedAt.UTC(), expiresAt, authJSON, policyJSON, recordHash, nil
}

func validDataAuthorizationPurpose(value string) bool {
	return value == "quant_analysis" || value == "research_analysis" || value == "risk_review"
}
func validDataAuthorizationOpaque(value string) bool {
	return value != "" && len(value) <= 256 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, "\r\n\t")
}
func validDataAuthorizationReason(value string) bool {
	return value != "" && len(value) <= 128 && strings.TrimSpace(value) == value && !strings.ContainsAny(value, " \r\n\t")
}
func validDataAuthorizationFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil && strings.ToLower(value) == value
}
