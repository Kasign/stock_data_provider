package bootstrap

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type fakeRetentionStore struct {
	calls  atomic.Int32
	called chan struct{}
}

func (f *fakeRetentionStore) PurgeQuotesBefore(ctx context.Context, _ time.Time) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	f.calls.Add(1)
	select {
	case f.called <- struct{}{}:
	default:
	}
	return 1, nil
}

func TestModuleStartStopWaitAndCloseLifecycle(t *testing.T) {
	store := &fakeRetentionStore{called: make(chan struct{}, 2)}
	module := &Module{
		retentionStore: store,
		retention:      24 * time.Hour,
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	ctx := context.Background()

	if err := module.Start(ctx); err != nil {
		t.Fatalf("first Start() error = %v", err)
	}
	if err := module.Start(ctx); err != nil {
		t.Fatalf("repeated Start() error = %v", err)
	}
	select {
	case <-store.called:
	case <-time.After(time.Second):
		t.Fatal("retention worker did not execute its initial purge")
	}

	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancelWait()
	if err := module.Wait(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Wait() error = %v, want deadline while worker is active", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("Stop() error = %v", err)
	}
	if err := module.Wait(ctx); err != nil {
		t.Fatalf("Wait() after Stop error = %v", err)
	}
	if err := module.Stop(ctx); err != nil {
		t.Fatalf("repeated Stop() error = %v", err)
	}

	if err := module.Start(ctx); err != nil {
		t.Fatalf("Start() after Stop error = %v", err)
	}
	select {
	case <-store.called:
	case <-time.After(time.Second):
		t.Fatal("retention worker did not restart after Stop")
	}
	if err := module.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := module.Start(ctx); !errors.Is(err, errModuleClosed) {
		t.Fatalf("Start() after Close error = %v, want errModuleClosed", err)
	}
	if err := module.Close(ctx); err != nil {
		t.Fatalf("repeated Close() error = %v", err)
	}
	if got := store.calls.Load(); got < 2 {
		t.Fatalf("retention worker calls = %d, want at least 2", got)
	}
}


func TestModuleCloseTimeoutKeepsModuleInClosingState(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	module := &Module{
		retentionStore: blockingRetentionStore{started: started, release: release},
		retention:      24 * time.Hour,
		logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := module.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("retention worker did not enter blocking purge")
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := module.Close(closeCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Close() error = %v, want deadline while purge is blocked", err)
	}
	if err := module.Start(context.Background()); !errors.Is(err, errModuleClosed) {
		t.Fatalf("Start() during retryable close error = %v, want errModuleClosed", err)
	}

	close(release)
	if err := module.Close(context.Background()); err != nil {
		t.Fatalf("Close() retry error = %v", err)
	}
}

type blockingRetentionStore struct {
	started chan struct{}
	release chan struct{}
}

func (s blockingRetentionStore) PurgeQuotesBefore(context.Context, time.Time) (int64, error) {
	close(s.started)
	<-s.release
	return 1, nil
}
