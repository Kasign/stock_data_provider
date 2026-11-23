package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/quant/application"
)

var (
	ErrInvalidAuditEvent     = errors.New("quant access audit event is invalid")
	ErrAuditStoreUnavailable = errors.New("quant access audit storage is unavailable")
)

// MySQLAccessAuditor append-only persists authorization decisions without
// storing result payloads or other restricted content.
type MySQLAccessAuditor struct {
	db *sql.DB
}

func NewMySQLAccessAuditor(db *sql.DB) (*MySQLAccessAuditor, error) {
	if db == nil {
		return nil, ErrInvalidRepository
	}
	return &MySQLAccessAuditor{db: db}, nil
}

var _ application.AccessAuditor = (*MySQLAccessAuditor)(nil)

func (a *MySQLAccessAuditor) RecordResultAccess(ctx context.Context, event application.AccessAuditEvent) error {
	if a == nil || a.db == nil || ctx == nil {
		return ErrInvalidAuditEvent
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validAuditRunRef(event.RunRef) || !validAuditText(event.SubjectRef) || !validQuantResultRef(event.ResourceRef) ||
		event.PurposeCode != "quant_analysis" || event.ContentLevel != "full" ||
		!validAuditText(event.ReasonCode) || !validAuditDecision(event.Decision) ||
		(!validAuditText(event.AuthorizationRef) && event.AuthorizationRef != "") ||
		(!validAuditText(event.PolicySnapshotRef) && event.PolicySnapshotRef != "") ||
		(!validAuditText(event.DecisionFingerprint) && event.DecisionFingerprint != "") ||
		!validOptionalAuditJSON(event.AuthorizationSnapshotJSON) ||
		!validOptionalAuditJSON(event.PolicySnapshotJSON) ||
		event.OccurredAt.IsZero() || event.OccurredAt.Location() != time.UTC {
		return ErrInvalidAuditEvent
	}

	var authorizationRef, authorizationSnapshot, policySnapshotRef, policySnapshot, decisionFingerprint any
	if event.AuthorizationRef != "" {
		authorizationRef = event.AuthorizationRef
	}
	if len(event.AuthorizationSnapshotJSON) > 0 {
		authorizationSnapshot = string(event.AuthorizationSnapshotJSON)
	}
	if event.PolicySnapshotRef != "" {
		policySnapshotRef = event.PolicySnapshotRef
	}
	if len(event.PolicySnapshotJSON) > 0 {
		policySnapshot = string(event.PolicySnapshotJSON)
	}
	if event.DecisionFingerprint != "" {
		decisionFingerprint = event.DecisionFingerprint
	}
	_, err := a.db.ExecContext(ctx, `INSERT INTO quant_access_audit
		(subject_ref, run_ref, resource_ref, purpose_code, content_level, decision, reason_code,
		 authorization_ref, authorization_snapshot_json, policy_snapshot_ref, policy_snapshot_json,
		 decision_fingerprint, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.SubjectRef, event.RunRef, event.ResourceRef, event.PurposeCode, event.ContentLevel,
		event.Decision, event.ReasonCode, authorizationRef, authorizationSnapshot, policySnapshotRef,
		policySnapshot, decisionFingerprint, event.OccurredAt.UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return errors.Join(ErrAuditStoreUnavailable, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func validAuditRunRef(value string) bool {
	if !validAuditText(value) || len(value) > 128 {
		return false
	}
	for i := 0; i < len(value); i++ {
		char := value[i]
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' || char == ':') {
			return false
		}
	}
	return true
}

func validAuditText(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value
}

func validOptionalAuditJSON(value json.RawMessage) bool {
	return len(value) == 0 || (len(value) <= 64*1024 && json.Valid(value))
}

func validAuditDecision(value string) bool {
	switch value {
	case "allowed", "denied", "unknown":
		return true
	default:
		return false
	}
}

func validQuantResultRef(value string) bool {
	const prefix = "quant_result:sha256:"
	digest := strings.TrimPrefix(value, prefix)
	if digest == value || len(digest) != 64 {
		return false
	}
	for _, r := range digest {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}
