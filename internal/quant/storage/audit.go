package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/quant/application"
)

var (
	ErrInvalidAuditEvent = errors.New("quant access audit event is invalid")
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
	if !validAuditText(event.SubjectRef) || !validAuditText(event.ResourceRef) ||
		event.PurposeCode != "quant_analysis" || !validAuditText(event.ReasonCode) ||
		!validAuditDecision(event.Decision) || (!validAuditText(event.AuthorizationRef) && event.AuthorizationRef != "") ||
		event.OccurredAt.IsZero() || event.OccurredAt.Location() != time.UTC {
		return ErrInvalidAuditEvent
	}

	var authorizationRef any
	if event.AuthorizationRef != "" {
		authorizationRef = event.AuthorizationRef
	}
	_, err := a.db.ExecContext(ctx, `INSERT INTO quant_access_audit
		(subject_ref, resource_ref, purpose_code, decision, reason_code, authorization_ref, occurred_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.SubjectRef, event.ResourceRef, event.PurposeCode, event.Decision,
		event.ReasonCode, authorizationRef, event.OccurredAt.UTC().Format(time.RFC3339Nano),
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

func validAuditText(value string) bool {
	return utf8.ValidString(value) && strings.TrimSpace(value) != "" && strings.TrimSpace(value) == value
}

func validAuditDecision(value string) bool {
	switch value {
	case "allowed", "denied", "unknown":
		return true
	default:
		return false
	}
}
