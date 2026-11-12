package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type expiredAttemptReaderFake struct {
	items      []ExpiredAttemptCandidate
	err        error
	afterRun   string
	afterStage string
	limit      int
}

func (f *expiredAttemptReaderFake) ListExpiredAttemptCandidates(_ context.Context, runRef, stageRef string, limit int) ([]ExpiredAttemptCandidate, error) {
	f.afterRun, f.afterStage, f.limit = runRef, stageRef, limit
	return append([]ExpiredAttemptCandidate(nil), f.items...), f.err
}

func TestExpiredAttemptScannerUsesCompositeKeysetPages(t *testing.T) {
	items := []ExpiredAttemptCandidate{
		{RunRef: "run-a", StageRef: "stage-b", AttemptRef: "attempt-1"},
		{RunRef: "run-b", StageRef: "stage-a", AttemptRef: "attempt-2"},
	}
	reader := &expiredAttemptReaderFake{items: items}
	scanner, err := NewExpiredAttemptScanner(reader, 2)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ListNextPage(context.Background(), "run-a", "stage-a")
	if err != nil {
		t.Fatal(err)
	}
	if reader.afterRun != "run-a" || reader.afterStage != "stage-a" || reader.limit != 2 ||
		!reflect.DeepEqual(page.Candidates, items) || page.NextRunRef != "run-b" ||
		page.NextStageRef != "stage-a" || !page.HasMore {
		t.Fatalf("unexpected expired Attempt page: reader=%#v page=%#v", reader, page)
	}
}

func TestExpiredAttemptScannerRejectsInvalidPagesAndCursors(t *testing.T) {
	for _, items := range [][]ExpiredAttemptCandidate{
		{{RunRef: "", StageRef: "stage", AttemptRef: "attempt"}},
		{{RunRef: "run", StageRef: "", AttemptRef: "attempt"}},
		{{RunRef: "run", StageRef: "stage", AttemptRef: ""}},
		{{RunRef: "run-b", StageRef: "stage-a", AttemptRef: "attempt-1"}, {RunRef: "run-a", StageRef: "stage-a", AttemptRef: "attempt-2"}},
		{{RunRef: "run-a", StageRef: "stage-b", AttemptRef: "attempt-1"}, {RunRef: "run-a", StageRef: "stage-b", AttemptRef: "attempt-2"}},
		{{RunRef: "run-a", StageRef: "stage-a", AttemptRef: "attempt-1"}, {RunRef: "run-a", StageRef: "stage-b", AttemptRef: "attempt-2"}, {RunRef: "run-b", StageRef: "stage-a", AttemptRef: "attempt-3"}},
	} {
		scanner, err := NewExpiredAttemptScanner(&expiredAttemptReaderFake{items: items}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scanner.ListNextPage(context.Background(), "", ""); err == nil {
			t.Errorf("invalid expired Attempt page accepted: %#v", items)
		}
	}
	scanner, _ := NewExpiredAttemptScanner(&expiredAttemptReaderFake{}, 2)
	if _, err := scanner.ListNextPage(context.Background(), "", "stage"); err == nil {
		t.Fatal("cursor with StageRef but no RunRef was accepted")
	}
}

func TestExpiredAttemptScannerPropagatesReaderError(t *testing.T) {
	want := errors.New("database unavailable")
	scanner, err := NewExpiredAttemptScanner(&expiredAttemptReaderFake{err: want}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ListNextPage(context.Background(), "", ""); !errors.Is(err, want) {
		t.Fatalf("reader error=%v, want wrapped %v", err, want)
	}
}
