package application

import (
	"context"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type runReaderStub struct {
	result CreateRunResult
	err    error
	called string
}

func (s *runReaderStub) GetRun(_ context.Context, ref string) (CreateRunResult, error) {
	s.called = ref
	return s.result, s.err
}

func validReadResult() CreateRunResult {
	return CreateRunResult{
		Run: domain.Run{
			Ref: "run-1", Version: 3, Status: domain.RunRunning,
			InputSnapshotRefs: []string{"snapshot-1"}, ExcludedStages: []domain.ExcludedStage{},
		},
		Stages: []domain.Stage{{Ref: "stage-1", RunRef: "run-1", Code: "context", Version: 2}},
	}
}

func TestGetRunReturnsValidatedAggregate(t *testing.T) {
	reader := &runReaderStub{result: validReadResult()}
	service, err := NewGetRunService(reader)
	if err != nil {
		t.Fatal(err)
	}
	got, err := service.GetRun(context.Background(), "run-1")
	if err != nil { t.Fatal(err) }
	if got.Run.Ref != "run-1" || len(got.Stages) != 1 || reader.called != "run-1" {
		t.Fatalf("unexpected GetRun result: %#v", got)
	}
}

func TestGetRunRejectsInvalidReferenceBeforeStorage(t *testing.T) {
	reader := &runReaderStub{result: validReadResult()}
	service, _ := NewGetRunService(reader)
	for _, ref := range []string{"", " run-1", "run-1 "} {
		if _, err := service.GetRun(context.Background(), ref); !errors.Is(err, domain.ErrInvalidCommand) {
			t.Fatalf("GetRun(%q) error = %v, want ErrInvalidCommand", ref, err)
		}
	}
	if reader.called != "" {
		t.Fatal("invalid Run reference reached Storage")
	}
}

func TestGetRunRejectsCrossRunAndDuplicateStages(t *testing.T) {
	cases := []struct {
		name   string
		stages []domain.Stage
	}{
		{name: "cross run", stages: []domain.Stage{{Ref: "stage-1", RunRef: "other", Code: "context", Version: 1}}},
		{name: "duplicate ref", stages: []domain.Stage{
			{Ref: "stage-1", RunRef: "run-1", Code: "context", Version: 1},
			{Ref: "stage-1", RunRef: "run-1", Code: "quant", Version: 1},
		}},
		{name: "duplicate code", stages: []domain.Stage{
			{Ref:"stage-1", RunRef:"run-1", Code:"context", Version:1},
			{Ref: "stage-2", RunRef: "run-1", Code: "context", Version: 1},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result := validReadResult()
			result.Stages = tc.stages
			service, _ := NewGetRunService(&runReaderStub{result: result})
			if _, err := service.GetRun(context.Background(), "run-1"); err == nil {
				t.Fatal("expected inconsistent Stage set to be rejected")
			}
		})
	}
}

func TestGetRunRequiresResultSummaryForTerminalRun(t *testing.T) {
	result := validReadResult()
	result.Run.Status = domain.RunSucceeded
	service, _ := NewGetRunService(&runReaderStub{result:result})
	if _, err := service.GetRun(context.Background(), "run-1"); err == nil {
		t.Fatal("terminal Run without ResultSummary must be rejected")
	}
}

func TestGetRunPropagatesStorageError(t *testing.T) {
	want := errors.New("storage unavailable")
	service, _ := NewGetRunService(&runReaderStub{err: want})
	if _, err := service.GetRun(context.Background(), "run-1"); !errors.Is(err, want) {
		t.Fatalf("GetRun error = %v, want wrapped storage error", err)
	}
}
