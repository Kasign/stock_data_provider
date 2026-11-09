package security

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

func validTestSecurityAuditEvent() SecurityAuditEvent {
	return SecurityAuditEvent{
		EventType: AuditLoginFailed, Subject: "anonymous", Result: AuditResultFailure,
		OccurredAt: time.Now().UTC(), RequestID: "req-01:abc",
		Source: AuditSourceAnonymous, ReasonCode: "security.invalid_credentials",
	}
}

func TestSecurityAuditEventValidation(t *testing.T) {
	event := validTestSecurityAuditEvent()
	if err := event.Validate(); err != nil {
		t.Fatalf("valid event rejected: %v", err)
	}
	event.Subject = "owner@example.com"
	if err := event.Validate(); err == nil {
		t.Fatal("PII subject must be rejected")
	}
	event = validTestSecurityAuditEvent()
	event.ReasonCode = "bad reason with spaces"
	if err := event.Validate(); err == nil {
		t.Fatal("unsafe reason code must be rejected")
	}
	event = validTestSecurityAuditEvent()
	event.EventType = "custom.event.with.arbitrary.payload"
	if err := event.Validate(); err == nil {
		t.Fatal("unregistered event type must be rejected")
	}
	event = validTestSecurityAuditEvent()
	event.RequestID = "header:Bearer-secret"
	if err := event.Validate(); err == nil {
		t.Fatal("request ID containing unsafe characters must be rejected")
	}
}

func TestWriteSecurityAuditAndWriterFailure(t *testing.T) {
	event := validTestSecurityAuditEvent()
	var written SecurityAuditEvent
	err := WriteSecurityAudit(context.Background(), SecurityAuditWriterFunc(func(_ context.Context, got SecurityAuditEvent) error {
		written = got
		return nil
	}), event)
	if err != nil || written != event {
		t.Fatalf("event was not written: written=%+v err=%v", written, err)
	}
	writerErr := errors.New("storage unavailable")
	err = WriteSecurityAudit(context.Background(), SecurityAuditWriterFunc(func(context.Context, SecurityAuditEvent) error {
		return writerErr
	}), event)
	var writerFault *fault.Fault
	if !errors.As(err, &writerFault) || writerFault.Dependency != "audit_writer" || writerFault.Retryable {
		t.Fatalf("writer error was not mapped to a safe dependency Fault: %v", err)
	}
	if err := WriteSecurityAudit(context.Background(), nil, event); err == nil {
		t.Fatal("missing Writer must not silently discard an event")
	}
}

func TestSecurityAuditEventStructuredLoggingContainsOnlyAllowlistedFields(t *testing.T) {
	event := validTestSecurityAuditEvent()
	value := event.LogValue()
	if value.Kind() != slog.KindGroup {
		t.Fatalf("LogValue kind = %v, want group", value.Kind())
	}
	if got := event.String(); got == "" {
		t.Fatal("String() must produce a diagnostic representation")
	}
}
