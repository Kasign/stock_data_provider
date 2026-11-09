package runtime

import (
	"context"
	"errors"
	"reflect"
	"testing"
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
