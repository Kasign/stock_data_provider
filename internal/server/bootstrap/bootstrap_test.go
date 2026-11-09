package bootstrap

import (
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
