package diagnostics

import "testing"

func TestAggregateMatchesPLATCTR004ContractMatrix(t *testing.T) {
	tests := []struct {
		name string
		results []Result
		want Status
	}{
		{name: "empty", want: StatusSkipped},
		{name: "all skipped", results: []Result{{Observation: Observation{Status: StatusSkipped}}}, want: StatusSkipped},
		{name: "all healthy", results: []Result{{Required: true, Observation: Observation{Status: StatusHealthy}}}, want: StatusHealthy},
		{name: "degraded", results: []Result{{Observation: Observation{Status: StatusDegraded}}}, want: StatusDegraded},
		{name: "optional failed", results: []Result{{Observation: Observation{Status: StatusFailed}}}, want: StatusDegraded},
		{name: "optional unavailable", results: []Result{{Observation: Observation{Status: StatusUnavailable}}}, want: StatusDegraded},
		{name: "required unavailable", results: []Result{{Required: true, Observation: Observation{Status: StatusUnavailable}}}, want: StatusUnavailable},
		{name: "required failed", results: []Result{{Required: true, Observation: Observation{Status: StatusFailed}}}, want: StatusFailed},
		{name: "required failure priority", results: []Result{
			{Required: true, Observation: Observation{Status: StatusUnavailable}},
			{Required: true, Observation: Observation{Status: StatusFailed}},
		}, want: StatusFailed},
		{name: "required unavailable priority", results: []Result{
			{Observation: Observation{Status: StatusDegraded}},
			{Required: true, Observation: Observation{Status: StatusUnavailable}},
		}, want: StatusUnavailable},
		{name: "optional skipped keeps healthy", results: []Result{
			{Required: true, Observation: Observation{Status: StatusHealthy}},
			{Observation: Observation{Status: StatusSkipped}},
		}, want: StatusHealthy},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := aggregate(tt.results).Status; got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
