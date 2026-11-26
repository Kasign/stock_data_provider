package resourcepolicy

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestResolveSourceEndpointsCreatesCanonicalImmutableSnapshot(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil { t.Fatal(err) }
	input := []string{
		"source-b/secondary@" + strings.Repeat("b", 64),
		"source-a/primary@" + strings.Repeat("a", 64),
	}
	snapshot, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, input)
	if err != nil { t.Fatal(err) }
	want := []string{
		"source-a/primary@" + strings.Repeat("a", 64),
		"source-b/secondary@" + strings.Repeat("b", 64),
	}
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) {
		t.Fatalf("Source/Endpoint snapshot = %#v, want %#v", snapshot.AllowedSourceEndpointRefs, want)
	}
	if len(snapshot.AllowedProviderCodes) != 0 { t.Fatalf("Feed snapshot contains Provider Codes: %#v", snapshot.AllowedProviderCodes) }
	if err := registry.ValidateSnapshot(snapshot); err != nil { t.Fatalf("valid snapshot rejected: %v", err) }
	input[0] = "mutated/value"
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) { t.Fatal("input mutation changed snapshot") }
}

func TestResolveSourceEndpointsRejectsInvalidAndDuplicateReferences(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil { t.Fatal(err) }
	valid := "source-a/primary@" + strings.Repeat("a", 64)
	for _, input := range [][]string{
		nil, {valid, valid}, {"source-a"}, {"source-a/primary/extra@" + strings.Repeat("a", 64)},
		{" source-a/primary@" + strings.Repeat("a", 64)},
		{"source-a/primary@" + strings.Repeat("A", 64)},
		{"source-a/primary@" + strings.Repeat("a", 63)},
		{"source-a/primary@" + strings.Repeat("a", 64) + "/extra"},
	} {
		if _, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, input); err == nil {
			t.Errorf("invalid inventory accepted: %#v", input)
		}
	}
	if _, err := registry.ResolveSourceEndpoints(NewsSearchConfiguredProvidersRef, []string{valid}); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("non-Feed policy accepted Feed inventory: %v", err)
	}
	if _, err := registry.Resolve(NewsFeedConfiguredSourcesRef, []string{"news_feed_catalog"}); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("Feed policy accepted generic Provider marker: %v", err)
	}
}

func TestValidateSnapshotInventoryRejectsMixedOrUnsortedFeedInventory(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil { t.Fatal(err) }
	a := "source-a/primary@" + strings.Repeat("a", 64)
	b := "source-b/primary@" + strings.Repeat("b", 64)
	snapshot, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, []string{a, b})
	if err != nil { t.Fatal(err) }
	mixed := snapshot
	mixed.AllowedProviderCodes = []string{"tavily"}
	if err := registry.ValidateSnapshot(mixed); !errors.Is(err, ErrInvalidInventory) { t.Fatalf("mixed inventory accepted: %v", err) }
	unsorted := snapshot
	unsorted.AllowedSourceEndpointRefs = []string{b, a}
	if err := registry.ValidateSnapshot(unsorted); !errors.Is(err, ErrInvalidInventory) { t.Fatalf("unsorted inventory accepted: %v", err) }
}
