package application

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/diagnostics"
)

func TestLeaseRecoveryReportDiagnosticsClassification(t *testing.T) {
	checkedAt := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	tests := []struct {
		name string
		report LeaseRecoveryReport
		status diagnostics.Status
		reason string
	}{
		{"clean", LeaseRecoveryReport{Scanned: 2, Recovered: 2}, diagnostics.StatusHealthy, ""},
		{"conflict", LeaseRecoveryReport{Scanned: 2, Recovered: 1, Conflicted: 1}, diagnostics.StatusDegraded, "notification.lease_recovery_conflict"},
		{"uncertain commit", LeaseRecoveryReport{Scanned: 1, OutcomeUnknown: 1, OutcomeUnknownDeliveryIDs: []string{"delivery-1"}}, diagnostics.StatusDegraded, "notification.persistence_outcome_unknown"},
		{"unprocessed", LeaseRecoveryReport{Scanned: 2, Recovered: 1, Unprocessed: 1}, diagnostics.StatusDegraded, "notification.lease_recovery_incomplete"},
		{"failure takes precedence", LeaseRecoveryReport{Scanned: 2, OutcomeUnknown: 1, OutcomeUnknownDeliveryIDs: []string{"delivery-1"}, Failed: 1}, diagnostics.StatusFailed, "notification.lease_recovery_failed"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			observation, err := tc.report.ToDiagnosticsObservation(checkedAt, time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			if observation.Status != tc.status || observation.ReasonCode != tc.reason {
				t.Fatalf("unexpected diagnostics mapping: %+v", observation)
			}
			if err := observation.Validate(); err != nil {
				t.Fatalf("mapped observation must satisfy Platform Diagnostics: %v", err)
			}
		})
	}
}

func TestLeaseRecoveryReportDiagnosticsRejectsInconsistentCounts(t *testing.T) {
	_, err := (LeaseRecoveryReport{Scanned: 2, Recovered: 1}).ToDiagnosticsObservation(time.Now(), 0)
	if err == nil {
		t.Fatal("inconsistent report counts must not become a valid diagnostic observation")
	}
	_, err = (LeaseRecoveryReport{Scanned: 1, OutcomeUnknown: 1}).ToDiagnosticsObservation(time.Now(), 0)
	if err == nil {
		t.Fatal("uncertain outcome count without delivery identity must be rejected")
	}
}
