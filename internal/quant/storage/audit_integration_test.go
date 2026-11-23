package storage

import (
	"context"
	"database/sql"
	"encoding/json"
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
		RunRef:           "run_0123456789abcdef0123456789abcdef",
		SubjectRef:       "service:researchflow",
		ResourceRef:      "quant_result:sha256:" + strings.Repeat("a", 64),
		PurposeCode:               "quant_analysis",
		ContentLevel:              "full",
		Decision:                  "allowed",
		ReasonCode:                "result_released",
		AuthorizationRef:          "auth_quant_test",
		AuthorizationSnapshotJSON: json.RawMessage(`{"authorization_ref":"auth_quant_test"}`),
		PolicySnapshotRef:         "policy-snapshot:quant:1",
		PolicySnapshotJSON:        json.RawMessage(`{"snapshot_ref":"policy-snapshot:quant:1"}`),
		DecisionFingerprint:       "sha256:" + strings.Repeat("c", 64),
		OccurredAt:                time.Date(2026, 11, 20, 3, 0, 0, 123456789, time.UTC),
	}
	if err := auditor.RecordResultAccess(ctx, event); err != nil {
		t.Fatalf("RecordResultAccess: %v", err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM quant_access_audit WHERE resource_ref = ?", event.ResourceRef)
	})
	var runRef, subject, resource, purpose, contentLevel, decision, reason, authorizationRef string
	var authorizationSnapshot, policySnapshotRef, policySnapshot, decisionFingerprint, occurredAt sql.NullString
	if err := db.QueryRowContext(ctx, `SELECT run_ref, subject_ref, resource_ref, purpose_code, content_level,
		decision, reason_code, authorization_ref, authorization_snapshot_json, policy_snapshot_ref,
		policy_snapshot_json, decision_fingerprint, occurred_at
		FROM quant_access_audit WHERE resource_ref = ?`, event.ResourceRef).
		Scan(&runRef, &subject, &resource, &purpose, &contentLevel, &decision, &reason, &authorizationRef,
			&authorizationSnapshot, &policySnapshotRef, &policySnapshot, &decisionFingerprint, &occurredAt); err != nil {
		t.Fatal(err)
	}
	if runRef != event.RunRef || subject != event.SubjectRef || resource != event.ResourceRef || purpose != event.PurposeCode ||
		contentLevel != event.ContentLevel || decision != event.Decision || reason != event.ReasonCode ||
		authorizationRef != event.AuthorizationRef || !authorizationSnapshot.Valid ||
		authorizationSnapshot.String != string(event.AuthorizationSnapshotJSON) ||
		!policySnapshotRef.Valid || policySnapshotRef.String != event.PolicySnapshotRef ||
		!policySnapshot.Valid || policySnapshot.String != string(event.PolicySnapshotJSON) ||
		!decisionFingerprint.Valid || decisionFingerprint.String != event.DecisionFingerprint ||
		!occurredAt.Valid || occurredAt.String != event.OccurredAt.Format(time.RFC3339Nano) {
		t.Fatalf("persisted audit event mismatch: run=%q subject=%q resource=%q purpose=%q level=%q decision=%q reason=%q", runRef, subject, resource, purpose, contentLevel, decision, reason)
	}
}

func TestMySQLAccessAuditorRejectsInvalidEventAndCancellation(t *testing.T) {
	db := openQuantIntegrationDB(t)
	auditor, err := NewMySQLAccessAuditor(db)
	if err != nil {
		t.Fatal(err)
	}
	valid := application.AccessAuditEvent{
		RunRef:      "run_abcdef0123456789abcdef0123456789",
		SubjectRef:  "service:researchflow",
		ResourceRef: "quant_result:sha256:" + strings.Repeat("b", 64),
		PurposeCode: "quant_analysis",
		ContentLevel: "full",
		Decision:    "denied",
		ReasonCode:  "authorization_denied",
		OccurredAt:  time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC),
	}
	missingRun := valid
	missingRun.RunRef = ""
	if err := auditor.RecordResultAccess(context.Background(), missingRun); !errors.Is(err, ErrInvalidAuditEvent) {
		t.Fatalf("missing RunRef error = %v", err)
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
