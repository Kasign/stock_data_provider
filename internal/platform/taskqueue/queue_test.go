package taskqueue

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
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

func TestTaskStatusForRun(t *testing.T) {
	tests := []struct {
		name   string
		status domain.RunStatus
		want   string
	}{
		{name: "completed run succeeds", status: domain.RunStatusCompleted, want: "succeeded"},
		{name: "quality-blocked run still has a completed task", status: domain.RunStatusBlocked, want: "succeeded"},
		{name: "partial run remains partial", status: domain.RunStatusPartial, want: "partial"},
		{name: "failed run fails task", status: domain.RunStatusFailed, want: "failed"},
		{name: "rejected request fails task", status: domain.RunStatusRejected, want: "failed"},
		{name: "non-terminal running result fails task", status: domain.RunStatusRunning, want: "failed"},
		{name: "unknown result status fails task", status: domain.RunStatus("future_status"), want: "failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := taskStatusForRun(tt.status); got != tt.want {
				t.Fatalf("taskStatusForRun(%q) = %q, want %q", tt.status, got, tt.want)
			}
		})
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

type startFailureConnector struct{ err error }

func (c startFailureConnector) Connect(context.Context) (driver.Conn, error) { return nil, c.err }
func (c startFailureConnector) Driver() driver.Driver                        { return startFailureDriver{err: c.err} }

type startFailureDriver struct{ err error }

func (d startFailureDriver) Open(string) (driver.Conn, error) { return nil, d.err }

type queueTestRunner struct{}

func (queueTestRunner) Run(context.Context, domain.RunRequest) (domain.RunResult, error) {
	return domain.RunResult{}, nil
}

func TestStartReturnsTheOriginalStartupFailureOnEveryCall(t *testing.T) {
	wantErr := errors.New("database unavailable")
	db := sql.OpenDB(startFailureConnector{err: wantErr})
	defer db.Close()
	q, err := New(db, queueTestRunner{}, 1, 1, time.Second, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	first := q.Start(context.Background())
	second := q.Start(context.Background())
	if first == nil || second == nil || first.Error() != second.Error() {
		t.Fatalf("startup failure must remain stable: first=%v second=%v", first, second)
	}
	q.Close()
}

func TestStartRejectsNilContextAndClosedQueue(t *testing.T) {
	db := sql.OpenDB(startFailureConnector{err: errors.New("unused")})
	defer db.Close()
	q, err := New(db, queueTestRunner{}, 1, 1, time.Second, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.Start(nil); err == nil {
		t.Fatal("nil start context must be rejected")
	}
	q.Close()
	if err := q.Start(context.Background()); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("start after close = %v, want ErrQueueClosed", err)
	}
	if _, _, err := q.Create(context.Background(), domain.RunRequest{}, ""); !errors.Is(err, ErrQueueClosed) {
		t.Fatalf("create after close = %v, want ErrQueueClosed", err)
	}
}

func TestNewRejectsMissingDependencies(t *testing.T) {
	q, err := New(nil, nil, 0, 0, 0, 0)
	if err == nil || q != nil {
		t.Fatal("queue must reject missing dependencies")
	}
}
