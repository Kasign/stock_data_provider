package storage

import (
	"context"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

func TestFinalizeDeadlineRunRejectsNonDeadlineTermination(t *testing.T) {
	repository := &MySQLRunRepository{}
	record := application.DeadlineFinalizationRecord{
		RunRef: "run-1", ExpectedVersion: 2, ResultSummaryRef: "summary-1",
		SummarySchemaVersion: "1.0.0", OutboxRef: "outbox-1",
		TerminalStatus: domain.RunFailed, TerminationReason: "user_cancelled",
	}
	if _, err := repository.FinalizeDeadlineRun(context.Background(), record); err == nil {
		t.Fatal("expected user-cancelled deadline finalization record to be rejected")
	}
	record.TerminationReason = "deadline_exceeded"
	record.TerminalStatus = domain.RunSucceeded
	if _, err := repository.FinalizeDeadlineRun(context.Background(), record); err == nil {
		t.Fatal("expected succeeded deadline finalization record to be rejected")
	}
}

func TestFinalizeDeadlineRunDelegatesToAtomicFinalizer(t *testing.T) {
	repository := &MySQLRunRepository{}
	record := application.DeadlineFinalizationRecord{
		RunRef: "run-1", ExpectedVersion: 2, ResultSummaryRef: "summary-1",
		SummarySchemaVersion: "1.0.0", OutboxRef: "outbox-1",
		TerminalStatus: domain.RunFailed, TerminationReason: "deadline_exceeded",
		Summary: domain.ResultSummary{
			StageOutcomes: map[string]domain.StageStatus{"required": domain.StageTimedOut},
			RiskGateDecisions: map[string]string{}, ExcludedStages: []domain.ExcludedStage{},
			MissingStageCodes: []string{"required"}, DegradedStageCodes: []string{},
			BlockedStageCodes: []string{}, ReasonCodes: []string{},
		},
	}
	if _, err := repository.FinalizeDeadlineRun(context.Background(), record); err == nil {
		t.Fatal("expected adapter to reach configured SQL repository validation")
	}
}
