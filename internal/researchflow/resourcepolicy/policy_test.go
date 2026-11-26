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

func TestQuantDailyBarsPolicySnapshotsOnlyConfiguredDataProviders(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.Resolve(QuantDailyBarsProvidersRef, []string{"eastmoney", "baostock"})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.PolicyRef != QuantDailyBarsProvidersRef ||
		!reflect.DeepEqual(snapshot.AllowedProviderCodes, []string{"baostock", "eastmoney"}) {
		t.Fatalf("unexpected Quant Data Provider snapshot: %+v", snapshot)
	}
	if err := registry.ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("valid Quant Data Provider snapshot rejected: %v", err)
	}
}

func TestResolveSourceEndpointsSnapshotsExactSortedIdentityOnly(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	inventory := []string{"source-b/secondary", "source-a/primary"}
	snapshot, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, inventory)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"source-a/primary", "source-b/secondary"}
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) ||
		len(snapshot.AllowedProviderCodes) != 0 {
		t.Fatalf("Feed Source/Endpoint snapshot = %+v", snapshot)
	}
	inventory[0] = "mutated/endpoint"
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) {
		t.Fatal("Feed snapshot retained caller-owned inventory memory")
	}
	if err := registry.ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("valid Feed Source/Endpoint snapshot rejected: %v", err)
	}
}

func TestResolveSourceEndpointsRejectsUnknownEmptyInvalidAndDuplicateInventory(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := registry.ResolveSourceEndpoints("news.feed.configured-sources@latest", []string{"source/endpoint"}); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("unknown Feed ResourcePolicy must fail closed, got %v", err)
	}
	if _, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, nil); !errors.Is(err, ErrNoProviders) {
		t.Fatalf("empty Feed inventory must fail closed, got %v", err)
	}
	for _, inventory := range [][]string{
		{"source/endpoint", "source/endpoint"},
		{"source/endpoint/extra"},
		{"source /endpoint"},
		{"../source/endpoint"},
		{"source/"},
	} {
		if _, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, inventory); !errors.Is(err, ErrInvalidInventory) {
			t.Errorf("Feed inventory %#v must be rejected, got %v", inventory, err)
		}
	}
	if _, err := registry.Resolve(NewsFeedConfiguredSourcesRef, []string{"news_feed_catalog"}); !errors.Is(err, ErrPolicyNotFound) {
		t.Fatalf("Feed ResourcePolicy must not accept Provider codes, got %v", err)
	}
}

func TestValidateFeedResourceSnapshotRejectsProviderCodesAndNonCanonicalRefs(t *testing.T) {
	registry, err := NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.ResolveSourceEndpoints(NewsFeedConfiguredSourcesRef, []string{"source/endpoint"})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.AllowedProviderCodes = []string{"bocha"}
	if err := registry.ValidateSnapshot(snapshot); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("Feed snapshot with Provider codes accepted: %v", err)
	}
	snapshot.AllowedProviderCodes = nil
	snapshot.AllowedSourceEndpointRefs[0] = "source/endpoint/extra"
	if err := registry.ValidateSnapshot(snapshot); !errors.Is(err, ErrInvalidInventory) {
		t.Fatalf("Feed snapshot with malformed Source/Endpoint ref accepted: %v", err)
	}
}
