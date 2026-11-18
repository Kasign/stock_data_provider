package application

import (
	"context"
	"errors"
	"testing"
)

type deadlineStoreFake struct {
	candidates []DeadlineCandidate
	expired    map[string]bool
	listErr    error
	expireErr  error
}

func (f *deadlineStoreFake) ListExpiredRunCandidates(context.Context, int) ([]DeadlineCandidate, error) {
	return f.candidates, f.listErr
}
func (f *deadlineStoreFake) ExpireRunDeadline(_ context.Context, runRef string, _ uint64) (bool, uint64, error) {
	if f.expireErr != nil {
		return false, 0, f.expireErr
	}
	return f.expired[runRef], 2, nil
}

func TestDeadlineScannerExpiresCandidatesAndCountsConflicts(t *testing.T) {
	store := &deadlineStoreFake{
		candidates: []DeadlineCandidate{{RunRef: "run-a", Version: 4}, {RunRef: "run-b", Version: 8}},
		expired:    map[string]bool{"run-a": true},
	}
	scanner, err := NewDeadlineScanner(store, DeadlineScanConfig{BatchSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	got, err := scanner.ScanBatch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Scanned != 2 || got.Expired != 1 || got.Conflicts != 1 {
		t.Fatalf("unexpected deadline scan result: %+v", got)
	}
}

func TestDeadlineScannerRejectsDuplicateAndPropagatesErrors(t *testing.T) {
	store := &deadlineStoreFake{candidates: []DeadlineCandidate{{RunRef: "run-a"}, {RunRef: "run-a"}}}
	scanner, err := NewDeadlineScanner(store, DeadlineScanConfig{BatchSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := scanner.ScanBatch(context.Background()); err == nil {
		t.Fatal("expected duplicate candidate to fail")
	}
	store.candidates = []DeadlineCandidate{{RunRef: "run-a"}, {RunRef: "run-b"}}
	store.expireErr = errors.New("database unavailable")
	result, err := scanner.ScanBatch(context.Background())
	if err == nil || result.Scanned != 2 || result.Failures != 2 {
		t.Fatalf("expiration failures must be reported after all candidates are processed: result=%+v err=%v", result, err)
	}
}

func TestNewDeadlineScannerValidatesConfiguration(t *testing.T) {
	store := &deadlineStoreFake{}
	for _, size := range []int{0, -1, 1001} {
		if _, err := NewDeadlineScanner(store, DeadlineScanConfig{BatchSize: size}); err == nil {
			t.Fatalf("expected invalid batch size %d to fail", size)
		}
	}
}
