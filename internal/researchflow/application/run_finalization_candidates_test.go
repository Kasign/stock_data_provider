package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type finalizableRunReaderFake struct {
	refs  []string
	err   error
	after string
	limit int
}

func (f *finalizableRunReaderFake) ListFinalizableRunRefs(_ context.Context, after string, limit int) ([]string, error) {
	f.after, f.limit = after, limit
	return append([]string(nil), f.refs...), f.err
}

func TestRunFinalizationCandidateScannerUsesKeysetPages(t *testing.T) {
	reader := &finalizableRunReaderFake{refs: []string{"run-b", "run-c"}}
	scanner, err := NewRunFinalizationCandidateScanner(reader, 2)
	if err != nil {
		t.Fatal(err)
	}
	page, err := scanner.ListNextPage(context.Background(), "run-a")
	if err != nil {
		t.Fatal(err)
	}
	if reader.after != "run-a" || reader.limit != 2 ||
		!reflect.DeepEqual(page.RunRefs, []string{"run-b", "run-c"}) ||
		page.NextCursor != "run-c" || !page.HasMore {
		t.Fatalf("unexpected finalization candidate page: reader=%#v page=%#v", reader, page)
	}
}

func TestRunFinalizationCandidateScannerRejectsInvalidReaderPages(t *testing.T) {
	for _, refs := range [][]string{
		{""},
		{"run-c", "run-b"},
		{"run-a"},
		{"run-b", "run-b"},
		{"run-b", "run-c", "run-d"},
	} {
		scanner, err := NewRunFinalizationCandidateScanner(&finalizableRunReaderFake{refs: refs}, 2)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scanner.ListNextPage(context.Background(), "run-a"); err == nil {
			t.Errorf("invalid finalization candidate page was accepted: %#v", refs)
		}
	}
}

func TestRunFinalizationCandidateScannerPropagatesReaderError(t *testing.T) {
	want := errors.New("database unavailable")
	scanner, err := NewRunFinalizationCandidateScanner(&finalizableRunReaderFake{err: want}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ListNextPage(context.Background(), ""); !errors.Is(err, want) {
		t.Fatalf("reader error=%v, want wrapped %v", err, want)
	}
}
