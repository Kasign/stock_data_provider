package application

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/researchflow/domain"
)

type workerPoolBlockingDispatcher struct {
	started chan string
	release chan struct{}
	active  atomic.Int32
	maximum atomic.Int32
}

func (d *workerPoolBlockingDispatcher) Execute(ctx context.Context, runRef, stageRef, attemptRef string) (WorkerExecution, error) {
	active := d.active.Add(1)
	for {
		maximum := d.maximum.Load()
		if active <= maximum || d.maximum.CompareAndSwap(maximum, active) {
			break
		}
	}
	defer d.active.Add(-1)
	d.started <- stageRef
	select {
	case <-d.release:
		return WorkerExecution{Attempt: domain.Attempt{Ref: attemptRef, StageRef: stageRef}}, nil
	case <-ctx.Done():
		return WorkerExecution{}, ctx.Err()
	}
}

func TestWorkerPoolBoundsConcurrentExecutionAndDrainsOnShutdown(t *testing.T) {
	dispatcher := &workerPoolBlockingDispatcher{
		started: make(chan string, 4),
		release: make(chan struct{}, 4),
	}
	pool, err := NewWorkerPool(dispatcher, WorkerPoolConfig{Workers: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	var wait sync.WaitGroup
	errorsFound := make(chan error, 4)
	for index := 0; index < 4; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, executeErr := pool.Execute(context.Background(), "run", string(rune('a'+index)), "attempt")
			errorsFound <- executeErr
		}()
	}
	for index := 0; index < 2; index++ {
		select {
		case <-dispatcher.started:
		case <-time.After(time.Second):
			t.Fatal("Worker Pool did not start the configured number of concurrent tasks")
		}
	}
	if active := dispatcher.active.Load(); active != 2 {
		t.Fatalf("active executions=%d, want exactly 2 before releasing any task", active)
	}
	dispatcher.release <- struct{}{}
	dispatcher.release <- struct{}{}
	for index := 0; index < 2; index++ {
		select {
		case <-dispatcher.started:
		case <-time.After(time.Second):
			t.Fatal("Worker Pool did not accept the remaining bounded tasks")
		}
	}
	dispatcher.release <- struct{}{}
	dispatcher.release <- struct{}{}
	wait.Wait()
	close(errorsFound)
	for executeErr := range errorsFound {
		if executeErr != nil {
			t.Fatalf("Worker Pool execution failed: %v", executeErr)
		}
	}
	if maximum := dispatcher.maximum.Load(); maximum != 2 {
		t.Fatalf("maximum concurrent executions=%d, want 2", maximum)
	}

	if err := pool.Shutdown(context.Background()); err != nil {
		t.Fatalf("graceful Worker Pool shutdown failed: %v", err)
	}
	if _, err := pool.Execute(context.Background(), "run", "stage", "attempt"); !errors.Is(err, ErrWorkerPoolNotRunning) {
		t.Fatalf("Execute after shutdown error=%v, want ErrWorkerPoolNotRunning", err)
	}
}

func TestWorkerPoolShutdownCancelsInFlightExecutionAfterDeadline(t *testing.T) {
	dispatcher := &workerPoolBlockingDispatcher{
		started: make(chan string, 1),
		release: make(chan struct{}),
	}
	pool, err := NewWorkerPool(dispatcher, WorkerPoolConfig{Workers: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := pool.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	executeDone := make(chan error, 1)
	go func() {
		_, executeErr := pool.Execute(context.Background(), "run", "stage", "attempt")
		executeDone <- executeErr
	}()
	select {
	case <-dispatcher.started:
	case <-time.After(time.Second):
		t.Fatal("Worker Pool did not start execution")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := pool.Shutdown(shutdownCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timed-out shutdown error=%v, want context deadline", err)
	}
	select {
	case executeErr := <-executeDone:
		if executeErr == nil {
			t.Fatal("in-flight execution unexpectedly succeeded after forced cancellation")
		}
	case <-time.After(time.Second):
		t.Fatal("pool cancellation did not reach the in-flight executor")
	}
}
