package security

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
)

const (
	SecurityAuditMigrationNamespace = "server.security_audit"
	securityAuditTable              = "server_security_audit_events"
)

// SQLSecurityAuditWriter persists bounded Platform security audit events in
// the Server-owned MariaDB schema. The caller owns the shared DB lifecycle.
type SQLSecurityAuditWriter struct {
	db *sql.DB
}

func NewSQLSecurityAuditWriter(db *sql.DB) (*SQLSecurityAuditWriter, error) {
	if db == nil {
		return nil, errors.New("security audit database is required")
	}
	return &SQLSecurityAuditWriter{db: db}, nil
}

// Write validates the Platform event contract before inserting only its
// allowlisted fields. It never accepts arbitrary metadata or request payloads.
func (w *SQLSecurityAuditWriter) Write(ctx context.Context, event platformsecurity.SecurityAuditEvent) error {
	if w == nil || w.db == nil || ctx == nil {
		return platformfault.New(platformfault.ClassValidation, "server.invalid_security_audit_writer", "security audit writer is not configured", nil)
	}
	if err := event.Validate(); err != nil {
		return err
	}
	_, err := w.db.ExecContext(ctx,
		"INSERT INTO "+securityAuditTable+" (event_type, subject, result, occurred_at, request_id, source, reason_code) VALUES (?, ?, ?, ?, ?, ?, ?)",
		string(event.EventType), event.Subject, string(event.Result), event.OccurredAt.UTC(),
		event.RequestID, string(event.Source), event.ReasonCode,
	)
	if err != nil {
		return &platformfault.Fault{
			Class: platformfault.ClassUnavailable, Code: "server.security_audit.write_failed",
			SafeMessage: "security audit event could not be persisted",
			Dependency: "mariadb", Retryable: false,
			Cause: fmt.Errorf("insert security audit event: %w", err),
		}
	}
	return nil
}

// SecurityAuditMigrations returns the Server-owned audit persistence schema.
// Retention and archival are intentionally not inferred; they require an
// explicit operational policy before destructive cleanup is introduced.
func SecurityAuditMigrations() []platformstorage.Migration {
	return []platformstorage.Migration{{
		Version:     1,
		Description: "create security audit event persistence",
		Statements: []string{`CREATE TABLE server_security_audit_events (
			audit_id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
			event_type VARCHAR(64) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			subject VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			result VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			occurred_at DATETIME(6) NOT NULL,
			request_id VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			source VARCHAR(16) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			reason_code VARCHAR(128) CHARACTER SET ascii COLLATE ascii_bin NOT NULL,
			PRIMARY KEY (audit_id),
			KEY idx_server_security_audit_occurred (occurred_at, audit_id),
			KEY idx_server_security_audit_subject_occurred (subject, occurred_at, audit_id),
			KEY idx_server_security_audit_request_id (request_id)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`},
	}}
}
