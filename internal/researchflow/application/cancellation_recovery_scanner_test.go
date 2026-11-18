package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type cancellationRecoveryStoreFake struct {
	candidates []ExpiredCancellationAttemptCandidate
	listErr    error
	fenceErr   error
	fenced     map[string]bool
	afterRun   string
	afterStage string
	limit      int
}

func (f *cancellationRecoveryStoreFake) ListExpiredCancellationAttemptCandidates(_ context.Context, runRef, stageRef string, limit int) ([]ExpiredCancellationAttemptCandidate, error) {
	f.afterRun, f.afterStage, f.limit = runRef, stageRef, limit
	return append([]ExpiredCancellationAttemptCandidate(nil), f.candidates...), f.listErr
}

func (f *cancellationRecoveryStoreFake) FenceExpiredCancellationAttempt(_ context.Context, _, _, attemptRef string, _ uint64) (bool, error) {
	if f.fenceErr != nil {
		return false, f.fenceErr
	}
	return f.fenced[attemptRef], nil
}

func TestCancellationRecoveryScannerUsesCompositeKeysetAndFencesExpiredAttempts(t *testing.T) {
	store := &cancellationRecoveryStoreFake{
		candidates: []ExpiredCancellationAttemptCandidate{
			{RunRef: "run-b", StageRef: "stage-a", AttemptRef: "attempt-a", ExpectedStageVersion: 2},
			{RunRef: "run-b", StageRef: "stage-b", AttemptRef: "attempt-b", ExpectedStageVersion: 3},
		},
		fenced: map[string]bool{"attempt-a": true},
	}
	scanner, err := NewCancellationRecoveryScanner(store, 2)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ListNextPage(context.Background(), "run-a", "stage-z")
	if err != nil {
		t.Fatal(err)
	}
	if store.afterRun != "run-a" || store.afterStage != "stage-z" || store.limit != 2 ||
		page.NextRunRef != "run-b" || page.NextStageRef != "stage-b" || !page.HasMore ||
		!reflect.DeepEqual(page.Candidates, store.candidates) {
		t.Fatalf("unexpected cancellation recovery page: store=%#v page=%#v", store, page)
	}
	result, err := scanner.RecoverPage(context.Background(), page)
	if err != nil {
		t.Fatal(err)
	}
	if result.Scanned != 2 || result.Fenced != 1 || result.Conflicts != 1 {
		t.Fatalf("unexpected cancellation recovery result: %+v", result)
	}
}

func TestCancellationRecoveryScannerRejectsInvalidCandidatePages(t *testing.T) {
	for _, candidates := range [][]ExpiredCancellationAttemptCandidate{
		{{RunRef: "", StageRef: "stage", AttemptRef: "attempt", ExpectedStageVersion: 1}},
		{{RunRef: "run-b", StageRef: "stage", AttemptRef: "attempt", ExpectedStageVersion: 0}},
		{{RunRef: "run-b", StageRef: "stage-b", AttemptRef: "attempt-b", ExpectedStageVersion: 1},
			{RunRef: "run-b", StageRef: "stage-a", AttemptRef: "attempt-a", ExpectedStageVersion: 1}},
		{{RunRef: "run-b", StageRef: "stage-a", AttemptRef: "attempt-a", ExpectedStageVersion: 1},
			{RunRef: "run-b", StageRef: "stage-a", AttemptRef: "attempt-b", ExpectedStageVersion: 1}},
	} {
		scanner, err := NewCancellationRecoveryScanner(&cancellationRecoveryStoreFake{candidates: candidates}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scanner.ListNextPage(context.Background(), "run-a", ""); err == nil {
			t.Errorf("invalid cancellation recovery candidates accepted: %#v", candidates)
		}
	}
}

func TestCancellationRecoveryScannerPropagatesStoreErrors(t *testing.T) {
	want := errors.New("database unavailable")
	scanner, err := NewCancellationRecoveryScanner(&cancellationRecoveryStoreFake{listErr: want}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ListNextPage(context.Background(), "", ""); !errors.Is(err, want) {
		t.Fatalf("list error=%v, want wrapped %v", err, want)
	}
}
