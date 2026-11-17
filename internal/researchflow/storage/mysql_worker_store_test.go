package storage

import (
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

func TestValidatePersistedResourcePolicySnapshot(t *testing.T) {
	registry, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Resolve(resourcepolicy.NewsSearchConfiguredProvidersRef, []string{"bocha", "tavily"})
	if err != nil { t.Fatal(err) }
	resolved := template.ResolvedTemplate{ResourcePolicyRef: resourcepolicy.NewsSearchConfiguredProvidersRef, ResourcePolicy: snapshot}
	if resolved.ResourcePolicy.PolicyRef != resolved.ResourcePolicyRef {
		t.Fatal("policy reference mismatch")
	}
	if err := registry.ValidateSnapshot(resolved.ResourcePolicy); err != nil {
		t.Fatal(err)
	}
	resolved.ResourcePolicy.AllowedProviderCodes[0] = "invalid code"
	if err := registry.ValidateSnapshot(resolved.ResourcePolicy); err == nil {
		t.Fatal("tampered allowlist must be rejected")
	}
}

func TestWorkerResultCommitMapsToExistingStorageTransaction(t *testing.T) {
	output := "snapshot://output"
	fault := "fault://provider"
	input := application.WorkerResultCommit{
		RunRef: "run-1", StageRef: "stage-1", AttemptRef: "attempt-1",
		WorkerID: "worker-1", FencingToken: 9,
		AttemptStatus: domain.AttemptSucceeded,
		Stage: domain.Stage{
			Ref: "stage-1", RunRef: "run-1", AttemptNo: 2,
			Version: 8, Status: domain.StageSucceeded, OutputRef: &output,
		},
		OutputRef: &output, FaultRef: &fault,
	}
	got, err := workerResultCommitToStorage(input)
	if err != nil {
		t.Fatal(err)
	}
	if got.RunRef != input.RunRef || got.StageRef != input.StageRef ||
		got.AttemptRef != input.AttemptRef || got.WorkerID != input.WorkerID ||
		got.FencingToken != input.FencingToken || got.AttemptStatus != input.AttemptStatus ||
		got.Stage.Version != input.Stage.Version || got.OutputRef == nil || *got.OutputRef != output ||
		got.FaultRef == nil || *got.FaultRef != fault {
		t.Fatalf("unexpected storage commit mapping: %+v", got)
	}
	*got.OutputRef = "mutated"
	*got.FaultRef = "mutated"
	if *input.OutputRef != output || *input.FaultRef != fault {
		t.Fatal("storage mapping must not alias caller-owned references")
	}
}

func TestWorkerResultCommitRejectsMismatchedOutputReference(t *testing.T) {
	stageOutput := "snapshot://stage"
	commitOutput := "snapshot://commit"
	_, err := workerResultCommitToStorage(application.WorkerResultCommit{
		RunRef: "run-1", StageRef: "stage-1", AttemptRef: "attempt-1",
		WorkerID: "worker-1", FencingToken: 1,
		Stage:     domain.Stage{Ref: "stage-1", RunRef: "run-1", OutputRef: &stageOutput},
		OutputRef: &commitOutput,
	})
	if err == nil {
		t.Fatal("expected mismatched output references to be rejected")
	}
}

func TestResolveWorkerRetryPolicyUsesPersistedStageSnapshot(t *testing.T) {
	resolved := template.ResolvedTemplate{Stages: []template.ResolvedStage{{
		Definition: template.StageDefinition{
			Code: "quant",
			Retry: template.RetryPolicy{
				Ref: "retry-v1", MaxAttempts: 3, AttemptTimeoutMS: 60000,
				RetryableReasonCodes: []string{"provider_unavailable"},
			},
		},
	}}}
	stage := domain.Stage{Code: "quant", RetryPolicyRef: "retry-v1"}
	got, err := resolveWorkerRetryPolicy(resolved, stage)
	if err != nil {
		t.Fatal(err)
	}
	got.RetryableReasonCodes[0] = "mutated"
	if resolved.Stages[0].Definition.Retry.RetryableReasonCodes[0] != "provider_unavailable" {
		t.Fatal("retry policy result must not alias persisted resolved template")
	}
}

func TestResolveWorkerRetryPolicyRejectsMismatchedOrDuplicateStage(t *testing.T) {
	stage := domain.Stage{Code: "quant", RetryPolicyRef: "retry-v1"}
	base := template.ResolvedStage{Definition: template.StageDefinition{
		Code: "quant", Retry: template.RetryPolicy{Ref: "retry-v1", MaxAttempts: 2, AttemptTimeoutMS: 1000},
	}}
	for name, resolved := range map[string]template.ResolvedTemplate{
		"missing": {},
		"mismatched reference": {Stages: []template.ResolvedStage{{Definition: template.StageDefinition{
			Code: "quant", Retry: template.RetryPolicy{Ref: "retry-v2", MaxAttempts: 2, AttemptTimeoutMS: 1000},
		}}}},
		"duplicate": {Stages: []template.ResolvedStage{base, base}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := resolveWorkerRetryPolicy(resolved, stage); err == nil {
				t.Fatal("expected invalid persisted retry policy to be rejected")
			}
		})
	}
}
