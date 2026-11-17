package application

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type schedulableRunReaderFake struct {
	refs  []string
	err   error
	after string
	limit int
}

func (f *schedulableRunReaderFake) ListSchedulableRunRefs(_ context.Context, after string, limit int) ([]string, error) {
	f.after, f.limit = after, limit
	return append([]string(nil), f.refs...), f.err
}

func TestSchedulerCandidateScannerUsesKeysetPages(t *testing.T) {
	reader := &schedulableRunReaderFake{refs: []string{"run-b", "run-c"}}
	scanner, err := NewSchedulerCandidateScanner(reader, 2)
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
		t.Fatalf("unexpected keyset page: reader=%#v page=%#v", reader, page)
	}
}

func TestSchedulerCandidateScannerRejectsInvalidReaderPages(t *testing.T) {
	for _, refs := range [][]string{
		{""},
		{"run-b", "run-a"},
		{"run-a"},
		{"run-b", "run-b"},
		{"run-b", "run-c", "run-d"},
	} {
		reader := &schedulableRunReaderFake{refs: refs}
		scanner, err := NewSchedulerCandidateScanner(reader, 2)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := scanner.ListNextPage(context.Background(), "run-a"); err == nil {
			t.Errorf("invalid candidate page was accepted: %#v", refs)
		}
	}
}

func TestSchedulerCandidateScannerPropagatesReaderError(t *testing.T) {
	want := errors.New("database unavailable")
	scanner, err := NewSchedulerCandidateScanner(&schedulableRunReaderFake{err: want}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ListNextPage(context.Background(), ""); !errors.Is(err, want) {
		t.Fatalf("reader error=%v, want wrapped %v", err, want)
	}
}
