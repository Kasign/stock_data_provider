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

type contextAwareBlockingStartComponent struct {
	name    string
	entered chan struct{}
	stopped chan struct{}
}

func (c contextAwareBlockingStartComponent) Name() string { return c.name }
func (c contextAwareBlockingStartComponent) Start(ctx context.Context) error {
	close(c.entered)
	<-ctx.Done()
	return ctx.Err()
}
func (c contextAwareBlockingStartComponent) Stop(context.Context) error {
	close(c.stopped)
	return nil
}

func TestLifecycleCancellationReleasesBlockingStartBeforeRollback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	component := contextAwareBlockingStartComponent{
		name: "context-aware-start", entered: make(chan struct{}), stopped: make(chan struct{}),
	}
	lifecycle := NewLifecycle()
	if err := lifecycle.Register(component); err != nil {
		t.Fatal(err)
	}

	startDone := make(chan error, 1)
	go func() { startDone <- lifecycle.Start(ctx) }()
	select {
	case <-component.entered:
	case <-time.After(time.Second):
		t.Fatal("component Start did not begin")
	}
	cancel()

	select {
	case err := <-startDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Start() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Start did not return after its process context was canceled")
	}
	select {
	case <-component.stopped:
	case <-time.After(time.Second):
		t.Fatal("started component was not stopped after cancellation")
	}
}

type rollbackDeadlineComponent struct {
	name     string
	startErr error
	deadline chan time.Time
}

func (c rollbackDeadlineComponent) Name() string                 { return c.name }
func (c rollbackDeadlineComponent) Start(context.Context) error  { return c.startErr }
func (c rollbackDeadlineComponent) Stop(ctx context.Context) error {
	deadline, ok := ctx.Deadline()
	if !ok {
		return errors.New("rollback context has no deadline")
	}
	c.deadline <- deadline
	return nil
}

type rollbackBlockingComponent struct {
	name     string
	startErr error
}

func (c rollbackBlockingComponent) Name() string                { return c.name }
func (c rollbackBlockingComponent) Start(context.Context) error { return c.startErr }
func (c rollbackBlockingComponent) Stop(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func TestLifecycleStartupRollbackUsesConfiguredDeadline(t *testing.T) {
	startErr := errors.New("startup failed")
	deadlines := make(chan time.Time, 1)
	lifecycle, err := NewLifecycleWithRollbackTimeout(250 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Register(rollbackDeadlineComponent{
		name: "deadline-check", startErr: startErr, deadline: deadlines,
	}); err != nil {
		t.Fatal(err)
	}
	err = lifecycle.Start(context.Background())
	if !errors.Is(err, startErr) {
		t.Fatalf("Start() error = %v, want original startup error", err)
	}
	select {
	case deadline := <-deadlines:
		remaining := time.Until(deadline)
		if remaining <= 0 || remaining > 250*time.Millisecond {
			t.Fatalf("rollback deadline remaining = %v, want within configured budget", remaining)
		}
	default:
		t.Fatal("rollback Stop did not receive a deadline")
	}
}

func TestLifecycleStartupRollbackStopsAtConfiguredTimeout(t *testing.T) {
	startErr := errors.New("startup failed")
	lifecycle, err := NewLifecycleWithRollbackTimeout(20 * time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.Register(rollbackBlockingComponent{name: "blocking-stop", startErr: startErr}); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	err = lifecycle.Start(context.Background())
	if !errors.Is(err, startErr) || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Start() error = %v, want startup and rollback deadline errors", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("startup rollback took %v, want bounded completion", elapsed)
	}
}

func TestNewLifecycleWithRollbackTimeoutRejectsNonPositiveValues(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		if _, err := NewLifecycleWithRollbackTimeout(timeout); err == nil {
			t.Fatalf("NewLifecycleWithRollbackTimeout(%v) error = nil", timeout)
		}
	}
}
