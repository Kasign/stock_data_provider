package taskqueue

import (
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
)

func TestSameRequestIgnoresRunID(t *testing.T) {
	a := domain.RunRequest{RunID: "run-a", TaskType: domain.TaskPostMarketScan, Symbols: []string{"600519"}}
	b := domain.RunRequest{RunID: "run-b", TaskType: domain.TaskPostMarketScan, Symbols: []string{"600519"}}
	if !sameRequest(a, b) {
		t.Fatal("requests should match when only RunID differs")
	}
	b.Symbols = []string{"000001"}
	if sameRequest(a, b) {
		t.Fatal("requests with different symbols must not match")
	}

	realtimeA := domain.RunRequest{TaskType: domain.TaskPostMarketScan, AsOfMode: domain.AsOfModeRealtimeSnapshot, AsOf: time.Date(2026, 10, 21, 10, 0, 0, 0, time.FixedZone("CST", 8*3600)), Symbols: []string{"600519"}}
	realtimeB := realtimeA
	realtimeB.AsOf = realtimeA.AsOf.Add(time.Minute)
	if !sameRequest(realtimeA, realtimeB) {
		t.Fatal("retries of the same realtime request should ignore generated as_of")
	}
	realtimeB.AsOfMode = domain.AsOfModeExplicit
	if sameRequest(realtimeA, realtimeB) {
		t.Fatal("explicit and realtime requests must not be treated as identical")
	}
}

func TestSafeMessageRemovesControlsAndBoundsLength(t *testing.T) {
	got := safeMessage("provider\nfailed\x00" + strings.Repeat("x", 400))
	if len(got) > 300 {
		t.Fatalf("message was not bounded: %d", len(got))
	}
	for _, r := range got {
		if r == '\n' || r == 0 {
			t.Fatalf("control character was not removed: %q", got)
		}
	}
}

func TestNewIDReturnsHexIdentifier(t *testing.T) {
	id, err := newID()
	if err != nil {
		t.Fatal(err)
	}
	if len(id) != 32 {
		t.Fatalf("unexpected ID length %d", len(id))
	}
	for _, r := range id {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			t.Fatalf("ID is not lowercase hex: %q", id)
		}
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	q, err := New(nil, nil, 0, 0, 0, 0)
	if err == nil || q != nil {
		t.Fatal("queue must reject missing dependencies")
	}
}
