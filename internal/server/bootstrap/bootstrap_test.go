package bootstrap

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestNewMigrationRegistryRegistersAvailableModules(t *testing.T) {
	registry, err := newMigrationRegistry()
	if err != nil {
		t.Fatalf("newMigrationRegistry() error = %v", err)
	}
	want := []string{"news", "notification", "research"}
	if got := registry.Namespaces(); !reflect.DeepEqual(got, want) {
		t.Fatalf("registered namespaces = %v, want %v", got, want)
	}
}

func TestStopConsumersThenCloseResourcesPreservesOrder(t *testing.T) {
	var events []string
	stopErr := errors.New("consumer stop failed")
	closeErr := errors.New("resource close failed")
	err := stopConsumersThenCloseResources(
		context.Background(),
		func(context.Context) error {
			events = append(events, "stop-consumers")
			return stopErr
		},
		func() error {
			events = append(events, "close-resources")
			return closeErr
		},
	)
	if !errors.Is(err, stopErr) || !errors.Is(err, closeErr) {
		t.Fatalf("shutdown error = %v, want both stop and close errors", err)
	}
	want := []string{"stop-consumers", "close-resources"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("shutdown order = %v, want %v", events, want)
	}
}

func TestStopConsumersThenCloseResourcesHandlesMissingLifecycle(t *testing.T) {
	closed := false
	err := stopConsumersThenCloseResources(context.Background(), nil, func() error {
		closed = true
		return nil
	})
	if err != nil || !closed {
		t.Fatalf("shutdown without lifecycle = (%v, closed=%v), want nil and closed", err, closed)
	}
}
