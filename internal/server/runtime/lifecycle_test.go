package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

type testComponent struct {
	name     string
	log      *[]string
	startErr error
	stopErr  error
}

func (c testComponent) Name() string { return c.name }
func (c testComponent) Start(context.Context) error {
	*c.log = append(*c.log, "start:"+c.name)
	return c.startErr
}
func (c testComponent) Stop(context.Context) error {
	*c.log = append(*c.log, "stop:"+c.name)
	return c.stopErr
}

type cancelOnStartComponent struct {
	testComponent
	cancel context.CancelFunc
}

func (c cancelOnStartComponent) Start(context.Context) error {
	*c.log = append(*c.log, "start:"+c.name)
	c.cancel()
	return nil
}

func TestLifecycleStartsInOrderAndStopsInReverse(t *testing.T) {
	var events []string
	lifecycle := NewLifecycle()
	for _, name := range []string{"storage", "worker", "transport"} {
		if err := lifecycle.Register(testComponent{name: name, log: &events}); err != nil {
			t.Fatal(err)
		}
	}
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"start:storage", "start:worker", "start:transport", "stop:transport", "stop:worker", "stop:storage"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("second Stop() = %v", err)
	}
}

func TestLifecycleRollsBackFailedComponentAndPriorComponents(t *testing.T) {
	var events []string
	startErr := errors.New("worker startup failed")
	lifecycle := NewLifecycle()
	_ = lifecycle.Register(testComponent{name: "storage", log: &events})
	_ = lifecycle.Register(testComponent{name: "worker", log: &events, startErr: startErr})
	_ = lifecycle.Register(testComponent{name: "transport", log: &events})
	err := lifecycle.Start(context.Background())
	if !errors.Is(err, startErr) {
		t.Fatalf("Start() error = %v, want original error", err)
	}
	want := []string{"start:storage", "start:worker", "stop:worker", "stop:storage"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Stop after rollback = %v", err)
	}
}

func TestLifecycleDoesNotStartComponentsWithCanceledContext(t *testing.T) {
	var events []string
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	lifecycle := NewLifecycle()
	_ = lifecycle.Register(testComponent{name: "worker", log: &events})
	err := lifecycle.Start(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
	if len(events) != 0 {
		t.Fatalf("components ran with canceled context: %v", events)
	}
}

func TestLifecycleRollsBackWhenContextCanceledDuringStartup(t *testing.T) {
	var events []string
	ctx, cancel := context.WithCancel(context.Background())
	lifecycle := NewLifecycle()
	_ = lifecycle.Register(cancelOnStartComponent{
		testComponent: testComponent{name: "worker", log: &events},
		cancel:        cancel,
	})
	_ = lifecycle.Register(testComponent{name: "transport", log: &events})
	err := lifecycle.Start(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Start() error = %v, want context.Canceled", err)
	}
	want := []string{"start:worker", "stop:worker"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}

func TestLifecycleRejectsDuplicateAndLateRegistration(t *testing.T) {
	var events []string
	lifecycle := NewLifecycle()
	component := testComponent{name: "worker", log: &events}
	if err := lifecycle.Register(component); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Register(component); err == nil {
		t.Fatal("duplicate registration must fail")
	}
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Register(testComponent{name: "late", log: &events}); err == nil {
		t.Fatal("late registration must fail")
	}
}

func TestLifecycleCollectsStopErrors(t *testing.T) {
	var events []string
	stopErr := errors.New("close failed")
	lifecycle := NewLifecycle()
	_ = lifecycle.Register(testComponent{name: "one", log: &events, stopErr: stopErr})
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Stop(context.Background()); !errors.Is(err, stopErr) {
		t.Fatalf("Stop() error = %v", err)
	}
}

type blockingStopComponent struct {
	name    string
	entered chan struct{}
	release chan struct{}
}

func (c blockingStopComponent) Name() string                { return c.name }
func (c blockingStopComponent) Start(context.Context) error { return nil }
func (c blockingStopComponent) Stop(ctx context.Context) error {
	close(c.entered)
	select {
	case <-c.release:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestLifecycleConcurrentStopWaitsForFirstStop(t *testing.T) {
	lifecycle := NewLifecycle()
	component := blockingStopComponent{
		name: "blocking-worker", entered: make(chan struct{}), release: make(chan struct{}),
	}
	if err := lifecycle.Register(component); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	go func() { firstDone <- lifecycle.Stop(context.Background()) }()
	<-component.entered

	waitCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := lifecycle.Stop(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("concurrent Stop() error = %v, want context deadline", err)
	}

	close(component.release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first Stop() error = %v", err)
	}
	if err := lifecycle.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() after completion = %v", err)
	}
}


type blockingStartComponent struct {
	name    string
	entered chan struct{}
	release chan struct{}
	events  *[]string
}

func (c blockingStartComponent) Name() string { return c.name }
func (c blockingStartComponent) Start(context.Context) error {
	*c.events = append(*c.events, "start:entered")
	close(c.entered)
	<-c.release
	*c.events = append(*c.events, "start:finished")
	return nil
}
func (c blockingStartComponent) Stop(context.Context) error {
	*c.events = append(*c.events, "stop")
	return nil
}

func TestLifecycleConcurrentStopWaitsForStartupToFinish(t *testing.T) {
	var events []string
	component := blockingStartComponent{
		name: "blocking-start", entered: make(chan struct{}), release: make(chan struct{}), events: &events,
	}
	lifecycle := NewLifecycle()
	if err := lifecycle.Register(component); err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Register(testComponent{name: "must-not-start", log: &events}); err != nil {
		t.Fatal(err)
	}

	startDone := make(chan error, 1)
	go func() { startDone <- lifecycle.Start(context.Background()) }()
	<-component.entered

	stopDone := make(chan error, 1)
	go func() { stopDone <- lifecycle.Stop(context.Background()) }()

	deadline := time.Now().Add(time.Second)
	for {
		lifecycle.mu.Lock()
		stopping := lifecycle.stopped
		lifecycle.mu.Unlock()
		if stopping {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("Stop did not enter stopping state")
		}
		time.Sleep(time.Millisecond)
	}

	close(component.release)
	if err := <-startDone; err == nil {
		t.Fatal("Start() must report interruption when Stop begins during startup")
	}
	if err := <-stopDone; err != nil {
		t.Fatalf("Stop() error = %v", err)
	}

	want := []string{"start:entered", "start:finished", "stop"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events = %v, want %v", events, want)
	}
}
