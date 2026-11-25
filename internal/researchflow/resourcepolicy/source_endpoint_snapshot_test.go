package resourcepolicy

import (
	"errors"
	"reflect"
	"testing"
)

func TestResolveSourceEndpointsCreatesCanonicalImmutableSnapshot(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	input := []string{"source-b/secondary", "source-a/primary"}
	snapshot, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, input)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"source-a/primary", "source-b/secondary"}
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) {
		t.Fatalf("Source/Endpoint snapshot = %#v, want %#v", snapshot.AllowedSourceEndpointRefs, want)
	}
	if len(snapshot.AllowedProviderCodes) != 0 {
		t.Fatalf("Feed snapshot must not contain Provider Codes: %#v", snapshot.AllowedProviderCodes)
	}
	if err := registry.ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("valid Source/Endpoint snapshot rejected: %v", err)
	}
	input[0] = "mutated/value"
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) {
		t.Fatal("mutating the input inventory changed the immutable snapshot")
	}
}

func TestResolveSourceEndpointsRejectsInvalidAndDuplicateReferences(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	for _, input := range [][]string{
		nil,
		{"source-a/primary", "source-a/primary"},
		{"source-a"},
		{"source-a/primary/extra"},
		{" source-a/primary"},
		{"source-a/"},
	} {
		if _, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, input); err == nil {
			t.Errorf("ResolveSourceEndpoints accepted invalid inventory %#v", input)
		}
	}
	if _, err := registry.ResolveSourceEndpoints(NewsSearchConfiguredProvidersRef, []string{"source-a/primary"}); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("non-Feed policy accepted Source/Endpoint inventory: %v", err)
	}
	if _, err := registry.Resolve(NewsFeedConfiguredSourcesRef, []string{"news_feed_catalog"}); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("Feed policy accepted a generic Provider marker: %v", err)
	}
}

func TestValidateSnapshotInventoryRejectsMixedOrUnsortedFeedInventory(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, []string{"source-a/primary", "source-b/primary"})
	if err != nil {
		t.Fatal(err)
	}
	mixed := snapshot
	mixed.AllowedProviderCodes = []string{"tavily"}
	if err := registry.ValidateSnapshot(mixed); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("mixed resource inventory accepted: %v", err)
	}
	unsorted := snapshot
	unsorted.AllowedSourceEndpointRefs = []string{"source-b/primary", "source-a/primary"}
	if err := registry.ValidateSnapshot(unsorted); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("unsorted Source/Endpoint inventory accepted: %v", err)
	}
}
