package template

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
)

func TestBindResourcePolicySnapshotSupportsExactFeedSourceInventory(t *testing.T) {
	registry, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	tmpl := testTemplate()
	tmpl.ResourcePolicyRef = resourcepolicy.NewsFeedConfiguredSourcesRef
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := registry.ResolveSourceEndpoints(resourcepolicy.NewsFeedConfiguredSourcesRef, []string{
		"source-b/secondary", "source-a/primary",
	})
	if err != nil {
		t.Fatal(err)
	}
	bound, err := BindResourcePolicySnapshot(resolved, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"source-a/primary", "source-b/secondary"}
	if !reflect.DeepEqual(bound.ResourcePolicy.AllowedSourceEndpointRefs, want) ||
		len(bound.ResourcePolicy.AllowedProviderCodes) != 0 {
		t.Fatalf("Feed ResourcePolicy snapshot = %+v", bound.ResourcePolicy)
	}
	bound.ResourcePolicy.AllowedSourceEndpointRefs[0] = "mutated/value"
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) {
		t.Fatal("binding ResourcePolicy snapshot retained caller-owned inventory memory")
	}

	invalid := snapshot
	invalid.AllowedSourceEndpointRefs = []string{"source-b/secondary", "source-a/primary"}
	if _, err := BindResourcePolicySnapshot(resolved, invalid); err == nil || !strings.Contains(err.Error(), "resource policy") {
		t.Fatalf("non-canonical Feed inventory was accepted: %v", err)
	}
}
