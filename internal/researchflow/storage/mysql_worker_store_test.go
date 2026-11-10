package storage

import (
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/application"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

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
		Stage: domain.Stage{Ref: "stage-1", RunRef: "run-1", OutputRef: &stageOutput},
		OutputRef: &commitOutput,
	})
	if err == nil {
		t.Fatal("expected mismatched output references to be rejected")
	}
}
