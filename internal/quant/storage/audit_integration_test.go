package storage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/application"
)

func TestMySQLAccessAuditorAppendsDecisionWithoutPayload(t *testing.T) {
	db := openQuantIntegrationDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	auditor, err := NewMySQLAccessAuditor(db)
	if err != nil {
		t.Fatal(err)
	}
	event := application.AccessAuditEvent{
		SubjectRef:       "service:researchflow",
		ResourceRef:      "quant_result:sha256:" + strings.Repeat("a", 64),
		PurposeCode:      "quant_analysis",
		Decision:         "allowed",
		ReasonCode:       "result_released",
		AuthorizationRef: "auth_quant_test",
		OccurredAt:       time.Date(2026, 11, 20, 3, 0, 0, 123456789, time.UTC),
	}
	if err := auditor.RecordResultAccess(ctx, event); err != nil {
		t.Fatalf("RecordResultAccess: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM quant_access_audit WHERE resource_ref = ?", event.ResourceRef)
	})
	var subject, resource, purpose, decision, reason, authorizationRef, occurredAt string
	if err := db.QueryRowContext(ctx, `SELECT subject_ref, resource_ref, purpose_code, decision, reason_code,
		authorization_ref, occurred_at FROM quant_access_audit WHERE resource_ref = ?`, event.ResourceRef).
		Scan(&subject, &resource, &purpose, &decision, &reason, &authorizationRef, &occurredAt); err != nil {
		t.Fatal(err)
	}
	if subject != event.SubjectRef || resource != event.ResourceRef || purpose != event.PurposeCode ||
		decision != event.Decision || reason != event.ReasonCode || authorizationRef != event.AuthorizationRef ||
		occurredAt != event.OccurredAt.Format(time.RFC3339Nano) {
		t.Fatalf("persisted audit event mismatch: %q %q %q %q %q %q %q", subject, resource, purpose, decision, reason, authorizationRef, occurredAt)
	}
}

func TestMySQLAccessAuditorRejectsInvalidEventAndCancellation(t *testing.T) {
	db := openQuantIntegrationDB(t)
	auditor, err := NewMySQLAccessAuditor(db)
	if err != nil {
		t.Fatal(err)
	}
	valid := application.AccessAuditEvent{
		SubjectRef:  "service:researchflow",
		ResourceRef: "quant_result:sha256:" + strings.Repeat("b", 64),
		PurposeCode: "quant_analysis",
		Decision:    "denied",
		ReasonCode:  "authorization_denied",
		OccurredAt:  time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC),
	}
	invalid := valid
	invalid.Decision = "conditional"
	if err := auditor.RecordResultAccess(context.Background(), invalid); !errors.Is(err, ErrInvalidAuditEvent) {
		t.Fatalf("invalid decision error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := auditor.RecordResultAccess(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled audit error = %v", err)
	}
}
