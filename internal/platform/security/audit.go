package security

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

type SecurityAuditEventType string

const (
	AuditLoginSucceeded    SecurityAuditEventType = "login.succeeded"
	AuditLoginFailed       SecurityAuditEventType = "login.failed"
	AuditLoginRateLimited  SecurityAuditEventType = "login.rate_limited"
	AuditLogout            SecurityAuditEventType = "logout"
	AuditSessionCreated    SecurityAuditEventType = "session.created"
	AuditSessionRotated    SecurityAuditEventType = "session.rotated"
	AuditSessionRevoked    SecurityAuditEventType = "session.revoked"
	AuditPasswordChanged   SecurityAuditEventType = "password.changed"
	AuditPasswordRecovered SecurityAuditEventType = "password.recovered"
	AuditSensitiveAction   SecurityAuditEventType = "security.sensitive_action"
)

type SecurityAuditResult string

const (
	AuditResultSuccess SecurityAuditResult = "success"
	AuditResultFailure SecurityAuditResult = "failure"
	AuditResultBlocked SecurityAuditResult = "blocked"
)

type SecurityAuditSource string

const (
	AuditSourceAnonymous    SecurityAuditSource = "anonymous"
	AuditSourceOwnerBearer  SecurityAuditSource = "owner_bearer"
	AuditSourceOwnerSession SecurityAuditSource = "owner_session"
	AuditSourceService      SecurityAuditSource = "service"
	AuditSourceInternal     SecurityAuditSource = "internal"
)

// SecurityAuditEvent intentionally contains only bounded, non-secret fields.
// Do not add arbitrary metadata, credentials, request bodies, or raw IP fields.
type SecurityAuditEvent struct {
	EventType  SecurityAuditEventType `json:"event_type"`
	Subject    string                 `json:"subject"`
	Result     SecurityAuditResult    `json:"result"`
	OccurredAt time.Time              `json:"occurred_at"`
	RequestID  string                 `json:"request_id"`
	Source     SecurityAuditSource    `json:"source"`
	ReasonCode string                 `json:"reason_code"`
}

// Validate rejects unbounded or unrecognized values before an event reaches a Writer.
func (e SecurityAuditEvent) Validate() error {
	if !validAuditEventType(e.EventType) ||
		(e.Subject != "owner" && e.Subject != "anonymous" && e.Subject != "system") ||
		!validAuditResult(e.Result) || e.OccurredAt.IsZero() ||
		!validAuditSource(e.Source) || !validAuditRequestID(e.RequestID) ||
		!validAuditReasonCode(e.ReasonCode) {
		return fault.New(fault.ClassValidation, "platform.security.invalid_audit_event", "security audit event is invalid", nil)
	}
	return nil
}

func validAuditEventType(value SecurityAuditEventType) bool {
	switch value {
	case AuditLoginSucceeded, AuditLoginFailed, AuditLoginRateLimited, AuditLogout,
		AuditSessionCreated, AuditSessionRotated, AuditSessionRevoked,
		AuditPasswordChanged, AuditPasswordRecovered, AuditSensitiveAction:
		return true
	default:
		return false
	}
}

func validAuditResult(value SecurityAuditResult) bool {
	return value == AuditResultSuccess || value == AuditResultFailure || value == AuditResultBlocked
}

func validAuditSource(value SecurityAuditSource) bool {
	return value == AuditSourceAnonymous || value == AuditSourceOwnerBearer ||
		value == AuditSourceOwnerSession || value == AuditSourceService || value == AuditSourceInternal
}

func validAuditRequestID(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune("._:-", char) {
			continue
		}
		return false
	}
	return true
}

func validAuditReasonCode(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i, char := range value {
		valid := (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') ||
			strings.ContainsRune("._-", char)
		if !valid || (i == 0 && strings.ContainsRune("._-", char)) {
			return false
		}
	}
	return true
}

// String contains only the bounded fields allowed in an audit event.
func (e SecurityAuditEvent) String() string {
	return fmt.Sprintf("security audit event (type=%s subject=%s result=%s request_id=%s source=%s reason=%s)",
		e.EventType, e.Subject, e.Result, e.RequestID, e.Source, e.ReasonCode)
}

// LogValue exposes only the allowlisted event fields to structured logging.
func (e SecurityAuditEvent) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("event_type", string(e.EventType)),
		slog.String("subject", e.Subject),
		slog.String("result", string(e.Result)),
		slog.Time("occurred_at", e.OccurredAt.UTC()),
		slog.String("request_id", e.RequestID),
		slog.String("source", string(e.Source)),
		slog.String("reason_code", e.ReasonCode),
	)
}

// SecurityAuditWriter is injected by Server; Platform does not choose a storage backend.
type SecurityAuditWriter interface {
	Write(ctx context.Context, event SecurityAuditEvent) error
}

type SecurityAuditWriterFunc func(ctx context.Context, event SecurityAuditEvent) error

func (f SecurityAuditWriterFunc) Write(ctx context.Context, event SecurityAuditEvent) error {
	if f == nil {
		return fmt.Errorf("security audit writer function is nil")
	}
	return f(ctx, event)
}

// WriteSecurityAudit validates the event and wraps Writer failures in a typed Fault.
// Server decides whether a failed write blocks the associated business operation.
func WriteSecurityAudit(ctx context.Context, writer SecurityAuditWriter, event SecurityAuditEvent) error {
	if ctx == nil || writer == nil {
		return fault.New(fault.ClassValidation, "platform.security.invalid_audit_writer", "security audit writer is not configured", nil)
	}
	if err := event.Validate(); err != nil {
		return err
	}
	if err := writer.Write(ctx, event); err != nil {
		return &fault.Fault{
			Class: fault.ClassUnavailable, Code: "platform.security.audit_writer_unavailable",
			SafeMessage: "security audit event could not be persisted", Dependency: "audit_writer",
			Retryable: false, Cause: fmt.Errorf("write security audit event: %w", err),
		}
	}
	return nil
}

