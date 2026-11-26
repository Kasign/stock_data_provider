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
	if err != nil { t.Fatal(err) }
	tmpl := testTemplate()
	tmpl.ResourcePolicyRef = resourcepolicy.NewsFeedConfiguredSourcesRef
	resolved, err := Resolve(context.Background(), tmpl, json.RawMessage("{}"), testParameterSchemas{}, testEvaluator{"always": true}, nil)
	if err != nil { t.Fatal(err) }
	a := "source-a/primary@" + strings.Repeat("a", 64)
	b := "source-b/secondary@" + strings.Repeat("b", 64)
	snapshot, err := registry.ResolveSourceEndpoints(resourcepolicy.NewsFeedConfiguredSourcesRef, []string{b, a})
	if err != nil { t.Fatal(err) }
	bound, err := BindResourcePolicySnapshot(resolved, snapshot)
	if err != nil { t.Fatal(err) }
	want := []string{a, b}
	if !reflect.DeepEqual(bound.ResourcePolicy.AllowedSourceEndpointRefs, want) ||
		len(bound.ResourcePolicy.AllowedProviderCodes) != 0 {
		t.Fatalf("Feed ResourcePolicy snapshot = %+v", bound.ResourcePolicy)
	}
	bound.ResourcePolicy.AllowedSourceEndpointRefs[0] = "mutated/value"
	if !reflect.DeepEqual(snapshot.AllowedSourceEndpointRefs, want) { t.Fatal("binding retained caller-owned inventory memory") }
	invalid := snapshot
	invalid.AllowedSourceEndpointRefs = []string{b, a}
	if _, err := BindResourcePolicySnapshot(resolved, invalid); err == nil || !strings.Contains(err.Error(), "resource policy") {
		t.Fatalf("non-canonical inventory accepted: %v", err)
	}
}
