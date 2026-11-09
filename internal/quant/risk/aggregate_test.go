package risk

import "testing"

func TestAggregateStatusRequiresExplicitHardGateEvidence(t *testing.T) {
	tests := []struct {
		name    string
		reasons []Reason
		want    DecisionStatus
	}{
		{"no rules", nil, DecisionUnknown},
		{"warnings only", []Reason{{Severity: SeverityWarn, Status: ReasonNotTriggered}}, DecisionUnknown},
		{"all hard gates clear", []Reason{
			{Severity: SeverityBlock, Status: ReasonNotTriggered},
			{Severity: SeverityBlock, Status: ReasonNotTriggered},
			{Severity: SeverityWarn, Status: ReasonUnknown},
		}, DecisionPass},
		{"unknown hard gate", []Reason{
			{Severity: SeverityBlock, Status: ReasonNotTriggered},
			{Severity: SeverityBlock, Status: ReasonUnknown},
		}, DecisionUnknown},
		{"triggered hard gate", []Reason{
			{Severity: SeverityBlock, Status: ReasonTriggered},
			{Severity: SeverityBlock, Status: ReasonUnknown},
		}, DecisionReject},
		{"triggered hard gate dominates unknown", []Reason{
			{Severity: SeverityBlock, Status: ReasonUnknown},
			{Severity: SeverityBlock, Status: ReasonTriggered},
		}, DecisionReject},
		{"warning cannot override rejection", []Reason{
			{Severity: SeverityBlock, Status: ReasonTriggered},
			{Severity: SeverityWarn, Status: ReasonNotTriggered},
		}, DecisionReject},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AggregateStatus(tt.reasons); got != tt.want {
				t.Fatalf("AggregateStatus() = %q, want %q", got, tt.want)
			}
		})
	}
}
