package bootstrap

import (
	"context"
	"encoding/json"
	"testing"

	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

func TestBuildNewsFeedWorkflowTemplateUsesCombinedImmutableOutputRegistry(t *testing.T) {
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	registry, err := newNewsStageOutputRegistry(newsFeedStageOutputRegistryVersion, true)
	if err != nil {
		t.Fatal(err)
	}
	if registry.Version() != "1.1.0" {
		t.Fatalf("combined News Output Registry version = %q, want 1.1.0", registry.Version())
	}
	if _, ok := registry.Entry(newsstage.SearchCandidatesStageCode, newsstage.SearchCandidatesOutputType, newsstage.SearchCandidatesSchemaVersion); !ok {
		t.Fatal("combined News Output Registry is missing SearchCandidates")
	}
	if _, ok := registry.Entry(newsstage.CollectFeedsStageCode, newsstage.CollectFeedsOutputType, newsstage.CollectFeedsSchemaVersion); !ok {
		t.Fatal("combined News Output Registry is missing Feed Collection")
	}
	value, err := buildNewsFeedWorkflowTemplate(&newsFeedStageComposition{
		registry: registry, resourceRefs: []string{"trusted-feed/primary"},
	}, policies)
	if err != nil {
		t.Fatalf("build News Feed workflow template: %v", err)
	}
	if value.Ref != newsFeedWorkflowTemplateRef || value.Version != newsFeedWorkflowTemplateVersion ||
		value.OutputRegistryVersion != registry.Version() || value.ResourcePolicyRef != resourcepolicy.NewsFeedConfiguredSourcesRef ||
		len(value.Stages) != 1 || value.Stages[0].Code != newsstage.CollectFeedsStageCode || !value.Stages[0].Required {
		t.Fatalf("News Feed workflow template contract = %+v", value)
	}
}

func TestNewsFeedApplicabilityRequiresEmptyValidatedParameters(t *testing.T) {
	evaluator := newsFeedApplicabilityEvaluator{}
	rule := template.ApplicabilityRule{RuleRef: newsFeedApplicabilityRuleRef, RuleVersion: newsFeedApplicabilityVersion}
	applicable, reason, err := evaluator.Evaluate(context.Background(), rule, json.RawMessage(`{}`))
	if err != nil || !applicable || reason != "" {
		t.Fatalf("empty Feed parameter object applicability = %v %q %v", applicable, reason, err)
	}
	for _, raw := range []string{`{"source_id":"untrusted"}`, `[]`, `null`} {
		if _, _, err := evaluator.Evaluate(context.Background(), rule, json.RawMessage(raw)); err == nil {
			t.Fatalf("Feed applicability accepted parameters %s", raw)
		}
	}
}
