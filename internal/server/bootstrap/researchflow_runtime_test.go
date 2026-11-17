package bootstrap

import (
	"context"
	"testing"
	"time"
)

func TestResearchflowRuntimeLaunchTaskBoundsAndDeduplicatesWork(t *testing.T) {
	component := &researchflowRuntimeComponent{
		taskSlots: make(chan struct{}, 2),
		inFlight:  make(map[string]struct{}),
	}
	ctx := context.Background()
	started := make(chan struct{}, 2)
	release := make(chan struct{})

	if !component.launchTask(ctx, "run:one", func() {
		started <- struct{}{}
		<-release
	}) {
		t.Fatal("first task was not launched")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first task did not start")
	}
	if component.launchTask(ctx, "run:one", func() {}) {
		t.Fatal("duplicate task key was launched while a slot was available")
	}
	if !component.launchTask(ctx, "run:two", func() {
		started <- struct{}{}
		<-release
	}) {
		t.Fatal("second task within the local concurrency bound was not launched")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("second task did not start")
	}
	if component.launchTask(ctx, "run:three", func() {}) {
		t.Fatal("task beyond local concurrency bound was launched")
	}

	close(release)
	done := make(chan struct{})
	go func() {
		component.tasks.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("launched task did not finish")
	}
	if !component.launchTask(ctx, "run:two", func() {}) {
		t.Fatal("available local slot was not reused")
	}
	component.tasks.Wait()
}

func TestResearchflowRuntimeLaunchTaskRejectsCancelledContext(t *testing.T) {
	component := &researchflowRuntimeComponent{
		taskSlots: make(chan struct{}, 1),
		inFlight:  make(map[string]struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if component.launchTask(ctx, "run:one", func() {}) {
		t.Fatal("task was launched with a cancelled context")
	}
	if len(component.taskSlots) != 0 {
		t.Fatal("cancelled task consumed a local worker slot")
	}
}
