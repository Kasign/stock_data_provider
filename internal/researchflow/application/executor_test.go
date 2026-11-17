package application

import (
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
)

func stringPointerForTest(value string) *string { return &value }

func TestValidateStageExecutionResult(t *testing.T) {
	providerResult := &stageoutput.ProviderResult{OutputType: "trend", SchemaVersion: "1.0.0", Payload: []byte(`{"trend":"up"}`), QualitySummary: []byte(`{"quality":"valid"}`), DiagnosticRefs: []string{}}
	retryAt := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		result StageExecutionResult
		valid  bool
	}{
		{name: "succeeded", result: StageExecutionResult{AttemptStatus: domain.AttemptSucceeded, StageStatus: domain.StageSucceeded, ProviderResult: providerResult}, valid: true},
		{name: "degraded", result: StageExecutionResult{AttemptStatus: domain.AttemptDegraded, StageStatus: domain.StageDegraded, ProviderResult: providerResult}, valid: true},
		{name: "retry wait", result: StageExecutionResult{AttemptStatus: domain.AttemptFailed, StageStatus: domain.StageRetryWait}, valid: true},
		{name: "failed", result: StageExecutionResult{AttemptStatus: domain.AttemptFailed, StageStatus: domain.StageFailed}, valid: true},
		{name: "timed out", result: StageExecutionResult{AttemptStatus: domain.AttemptTimedOut, StageStatus: domain.StageTimedOut}, valid: true},
		{name: "cancelled", result: StageExecutionResult{AttemptStatus: domain.AttemptCancelled, StageStatus: domain.StageCancelled}, valid: true},
		{name: "success without output", result: StageExecutionResult{AttemptStatus: domain.AttemptSucceeded, StageStatus: domain.StageSucceeded}, valid: false},
		{name: "retry without provider time", result: StageExecutionResult{AttemptStatus: domain.AttemptFailed, StageStatus: domain.StageRetryWait}, valid: true},
		{name: "retry with output", result: StageExecutionResult{AttemptStatus: domain.AttemptFailed, StageStatus: domain.StageRetryWait, RetryAt: &retryAt, OutputRef: stringPointerForTest("provider://legacy")}, valid: false},
		{name: "mismatched attempt", result: StageExecutionResult{AttemptStatus: domain.AttemptFailed, StageStatus: domain.StageSucceeded, OutputRef: stringPointerForTest("provider://legacy")}, valid: false},
		{name: "unknown stage state", result: StageExecutionResult{AttemptStatus: domain.AttemptFailed, StageStatus: domain.StageReady}, valid: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStageExecutionResult(tt.result)
			if tt.valid && err != nil {
				t.Fatalf("expected valid result, got %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatal("expected invalid result to be rejected")
			}
		})
	}
}

func TestValidateStageExecutionResultRejectsEmptyReferences(t *testing.T) {
	empty := "  "
	err := ValidateStageExecutionResult(StageExecutionResult{
		AttemptStatus: domain.AttemptSucceeded,
		StageStatus:   domain.StageSucceeded,
		OutputRef:     &empty,
	})
	if err == nil {
		t.Fatal("expected empty output reference to be rejected")
	}
}
