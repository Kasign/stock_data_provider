package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type finalizationRouterStoreFake struct {
	snapshot DeadlineFinalizationSnapshot
	err      error
}

func (f finalizationRouterStoreFake) LoadRunFinalizationSnapshot(context.Context, string) (DeadlineFinalizationSnapshot, error) {
	return f.snapshot, f.err
}

type normalFinalizerFake struct {
	result RunFinalizationResult
	calls  int
	err    error
}

func (f *normalFinalizerFake) Finalize(context.Context, string) (RunFinalizationResult, error) {
	f.calls++
	return f.result, f.err
}

type expiredFinalizerFake struct {
	result DeadlineFinalizationResult
	calls  int
	err    error
}

func (f *expiredFinalizerFake) Finalize(context.Context, string) (DeadlineFinalizationResult, error) {
	f.calls++
	return f.result, f.err
}

type cancelledFinalizerFake struct {
	result CancellationFinalizationResult
	calls  int
	err    error
}

func (f *cancelledFinalizerFake) Finalize(context.Context, string) (CancellationFinalizationResult, error) {
	f.calls++
	return f.result, f.err
}

func TestRunFinalizationCoordinatorRoutesByPersistedState(t *testing.T) {
	cases := []struct {
		name string
		run  domain.Run
		want RunFinalizationPath
	}{
		{name: "normal", run: domain.Run{Ref: "run-1", Version: 1, Status: domain.RunRunning}, want: RunFinalizationPathNormal},
		{name: "deadline from created", run: domain.Run{Ref: "run-1", Version: 1, Status: domain.RunCreated, TerminationReason: "deadline_exceeded"}, want: RunFinalizationPathDeadline},
		{name: "deadline from queued", run: domain.Run{Ref: "run-1", Version: 1, Status: domain.RunQueued, TerminationReason: "deadline_exceeded"}, want: RunFinalizationPathDeadline},
		{name: "deadline from running", run: domain.Run{Ref: "run-1", Version: 1, Status: domain.RunRunning, TerminationReason: "deadline_exceeded"}, want: RunFinalizationPathDeadline},
		{name: "user cancellation", run: domain.Run{Ref: "run-1", Version: 1, Status: domain.RunCancelling, TerminationReason: "user_cancelled"}, want: RunFinalizationPathCancellation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			normal := &normalFinalizerFake{result: RunFinalizationResult{Run: tc.run, Finalized: true}}
			deadline := &expiredFinalizerFake{result: DeadlineFinalizationResult{Run: tc.run, Finalized: true}}
			cancelled := &cancelledFinalizerFake{result: CancellationFinalizationResult{Run: tc.run, Finalized: true}}
			coordinator, err := NewRunFinalizationCoordinator(
				finalizationRouterStoreFake{snapshot: DeadlineFinalizationSnapshot{Run: tc.run}},
				normal, deadline, cancelled,
			)
			if err != nil {
				t.Fatal(err)
			}
			got, err := coordinator.Finalize(context.Background(), "run-1")
			if err != nil {
				t.Fatal(err)
			}
			if got.Path != tc.want || !got.Finalized {
				t.Fatalf("got path=%q finalized=%v, want %q", got.Path, got.Finalized, tc.want)
			}
			wantCalls := map[RunFinalizationPath]int{
				RunFinalizationPathNormal:       normal.calls,
				RunFinalizationPathDeadline:     deadline.calls,
				RunFinalizationPathCancellation: cancelled.calls,
			}
			for path, calls := range wantCalls {
				expected := 0
				if path == tc.want {
					expected = 1
				}
				if calls != expected {
					t.Fatalf("%s calls=%d, want %d", path, calls, expected)
				}
			}
		})
	}
}

func TestRunFinalizationCoordinatorRejectsAmbiguousAndTerminalStates(t *testing.T) {
	states := []domain.Run{
		{Ref: "run-1", Version: 1, Status: domain.RunRunning, TerminationReason: "user_cancelled"},
		{Ref: "run-1", Version: 1, Status: domain.RunCancelling},
		{Ref: "run-1", Version: 1, Status: domain.RunSucceeded},
		{Ref: "run-1", Version: 1, Status: domain.RunQueued},
	}
	for _, run := range states {
		coordinator, err := NewRunFinalizationCoordinator(
			finalizationRouterStoreFake{snapshot: DeadlineFinalizationSnapshot{Run: run}},
			&normalFinalizerFake{}, &expiredFinalizerFake{}, &cancelledFinalizerFake{},
		)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := coordinator.Finalize(context.Background(), "run-1"); err == nil {
			t.Fatalf("expected state %q / reason %q to be rejected", run.Status, run.TerminationReason)
		}
	}
}

func TestRunFinalizationCoordinatorPropagatesSnapshotAndFinalizerErrors(t *testing.T) {
	storeErr := errors.New("storage unavailable")
	coordinator, err := NewRunFinalizationCoordinator(
		finalizationRouterStoreFake{err: storeErr},
		&normalFinalizerFake{}, &expiredFinalizerFake{}, &cancelledFinalizerFake{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Finalize(context.Background(), "run-1"); !errors.Is(err, storeErr) {
		t.Fatalf("got error %v, want wrapped storage error", err)
	}

	finalizerErr := errors.New("finalizer unavailable")
	normal := &normalFinalizerFake{err: finalizerErr}
	coordinator, err = NewRunFinalizationCoordinator(
		finalizationRouterStoreFake{snapshot: DeadlineFinalizationSnapshot{
			Run: domain.Run{Ref: "run-1", Version: 1, Status: domain.RunRunning},
		}},
		normal, &expiredFinalizerFake{}, &cancelledFinalizerFake{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Finalize(context.Background(), "run-1"); !errors.Is(err, finalizerErr) {
		t.Fatalf("got error %v, want wrapped finalizer error", err)
	}
}

func TestRunFinalizationCoordinatorRejectsInvalidSnapshotAndReference(t *testing.T) {
	coordinator, err := NewRunFinalizationCoordinator(
		finalizationRouterStoreFake{snapshot: DeadlineFinalizationSnapshot{
			Run: domain.Run{Ref: "different", Version: 1, Status: domain.RunRunning},
		}},
		&normalFinalizerFake{}, &expiredFinalizerFake{}, &cancelledFinalizerFake{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Finalize(context.Background(), "run-1"); err == nil {
		t.Fatal("expected mismatched Run reference to be rejected")
	}
	if _, err := coordinator.Finalize(context.Background(), " "); err == nil {
		t.Fatal("expected empty Run reference to be rejected")
	}
}
