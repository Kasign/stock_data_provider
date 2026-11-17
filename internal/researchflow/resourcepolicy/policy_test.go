package resourcepolicy

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveSnapshotsSortedProviderIdentityOnly(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	inventory := []string{"tavily", "bocha"}
	snapshot, err := registry.Resolve(NewsSearchConfiguredProvidersRef, inventory)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(snapshot.AllowedProviderCodes, []string{"bocha", "tavily"}) {
		t.Fatalf("provider allowlist is not canonical: %#v", snapshot.AllowedProviderCodes)
	}
	inventory[0] = "changed"
	if !reflect.DeepEqual(snapshot.AllowedProviderCodes, []string{"bocha", "tavily"}) {
		t.Fatal("snapshot retained caller-owned inventory memory")
	}
	if err := registry.ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("valid snapshot rejected: %v", err)
	}
}

func TestResolveFailsClosedForUnknownPolicyAndEmptyInventory(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Resolve("news.search.configured-providers@latest", []string{"bocha"}); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("unknown policy must fail closed, got %v", err)
	}
	if _, err := registry.Resolve(NewsSearchConfiguredProvidersRef, nil); !errors.Is(err, ErrNoProviders) {
		t.Fatalf("empty inventory must fail closed, got %v", err)
	}
}

func TestResolveRejectsInvalidAndDuplicateProviderCodes(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, inventory := range [][]string{{"bocha", "bocha"}, {"bocha", "Tavily"}, {"bocha", " tavily"}, {"bocha", "../tavily"}} {
		if _, err := registry.Resolve(NewsSearchConfiguredProvidersRef, inventory); !errors.Is(err, ErrInvalidInventory) {
			t.Errorf("inventory %#v must be rejected, got %v", inventory, err)
		}
	}
}

func TestValidateSnapshotRejectsNonCanonicalMutation(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Resolve(NewsSearchConfiguredProvidersRef, []string{"bocha", "tavily"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.AllowedProviderCodes[1] = "bocha"
	if err := registry.ValidateSnapshot(snapshot); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("mutated snapshot accepted: %v", err)
	}
}
