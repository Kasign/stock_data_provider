package bootstrap

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/domain"
	"example.com/stock-analysis-flow/internal/platform/taskqueue"
	runtime "example.com/stock-analysis-flow/internal/server/runtime"
)

type workerLifecycleConnector struct {
	connectErr error
}

func (c workerLifecycleConnector) Connect(context.Context) (driver.Conn, error) {
	if c.connectErr != nil {
		return nil, c.connectErr
	}
	return &workerLifecycleConn{}, nil
}

func (c workerLifecycleConnector) Driver() driver.Driver {
	return workerLifecycleDriver{connectErr: c.connectErr}
}

type workerLifecycleDriver struct {
	connectErr error
}

func (d workerLifecycleDriver) Open(string) (driver.Conn, error) {
	if d.connectErr != nil {
		return nil, d.connectErr
	}
	return &workerLifecycleConn{}, nil
}

type workerLifecycleConn struct{}

func (*workerLifecycleConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepared statements are not supported")
}

func (*workerLifecycleConn) Close() error { return nil }

func (*workerLifecycleConn) Begin() (driver.Tx, error) {
	return &workerLifecycleTx{}, nil
}

func (*workerLifecycleConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &workerLifecycleTx{}, nil
}

func (*workerLifecycleConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}

func (*workerLifecycleConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &workerLifecycleRows{}, nil
}

type workerLifecycleTx struct{}

func (*workerLifecycleTx) Commit() error   { return nil }
func (*workerLifecycleTx) Rollback() error { return nil }

type workerLifecycleRows struct{}

func (*workerLifecycleRows) Columns() []string {
	return []string{"task_id", "run_id", "request_json"}
}

func (*workerLifecycleRows) Close() error { return nil }

func (*workerLifecycleRows) Next([]driver.Value) error { return io.EOF }

type workerLifecycleRunner struct{}

func (workerLifecycleRunner) Run(context.Context, domain.RunRequest) (domain.RunResult, error) {
	return domain.RunResult{}, nil
}

func newWorkerLifecycleQueue(t *testing.T, connectErr error) *taskqueue.Queue {
	t.Helper()
	db := sql.OpenDB(workerLifecycleConnector{connectErr: connectErr})
	t.Cleanup(func() { _ = db.Close() })
	queue, err := taskqueue.New(db, workerLifecycleRunner{}, 1, 1, time.Second, time.Millisecond)
	if err != nil {
		t.Fatalf("taskqueue.New() error = %v", err)
	}
	return queue
}

func TestLegacyAnalysisWorkerStartsAndStopsThroughLifecycle(t *testing.T) {
	queue := newWorkerLifecycleQueue(t, nil)
	worker := legacyAnalysisWorker{name: "legacy-analysis-worker", queue: queue, enabled: true}
	lifecycle := runtime.NewLifecycle()
	if err := lifecycle.Register(worker); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	if err := lifecycle.Start(ctx); err != nil {
		cancel()
		t.Fatalf("Lifecycle.Start() error = %v", err)
	}
	cancel()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	if err := lifecycle.Stop(stopCtx); err != nil {
		t.Fatalf("Lifecycle.Stop() error = %v", err)
	}
	if err := queue.Start(context.Background()); !errors.Is(err, taskqueue.ErrQueueClosed) {
		t.Fatalf("queue restart after lifecycle stop = %v, want ErrQueueClosed", err)
	}
}

func TestLegacyAnalysisWorkerDisabledRoleDoesNotStartQueue(t *testing.T) {
	queue := newWorkerLifecycleQueue(t, nil)
	worker := legacyAnalysisWorker{name: "legacy-analysis-worker", queue: queue, enabled: false}
	lifecycle := runtime.NewLifecycle()
	if err := lifecycle.Register(worker); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatalf("Lifecycle.Start() error = %v", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Lifecycle.Stop() error = %v", err)
	}
	if err := queue.Start(context.Background()); !errors.Is(err, taskqueue.ErrQueueClosed) {
		t.Fatalf("disabled queue restart after lifecycle stop = %v, want ErrQueueClosed", err)
	}
}

func TestLegacyAnalysisWorkerStartupFailureRollsBackQueue(t *testing.T) {
	queue := newWorkerLifecycleQueue(t, errors.New("database unavailable"))
	worker := legacyAnalysisWorker{name: "legacy-analysis-worker", queue: queue, enabled: true}
	lifecycle := runtime.NewLifecycle()
	if err := lifecycle.Register(worker); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := lifecycle.Start(context.Background()); err == nil {
		t.Fatal("Lifecycle.Start() should return the queue startup failure")
	}
	if err := queue.Start(context.Background()); !errors.Is(err, taskqueue.ErrQueueClosed) {
		t.Fatalf("queue restart after failed lifecycle startup = %v, want ErrQueueClosed", err)
	}
}
