package bootstrap

import (
	"testing"

	newsapp "example.com/stock-analysis-flow/internal/news/app"
	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

func TestBuildNewsSearchWorkflowTemplatePreflightsAndFreezesDependencies(t *testing.T) {
	stage, err := newNewsSearchStageCompositionWithProviders(newsapp.StaticProviderSet{})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	value, err := buildNewsSearchWorkflowTemplate(stage, policies)
	if err != nil {
		t.Fatal(err)
	}
	if err := template.Validate(value, nil); err != nil {
		t.Fatalf("formal template failed validation: %v", err)
	}
	if value.Ref != "news.search_candidates" || value.Version != "1.0.0" ||
		!value.AllowEmptyInputs || value.RequiresHardRiskGate ||
		value.ParameterSchemaRef != newsSearchParameterSchemaRef ||
		value.OutputRegistryVersion != newsSearchStageOutputRegistryVersion ||
		value.ResourcePolicyRef != resourcepolicy.NewsSearchConfiguredProvidersRef {
		t.Fatalf("formal template identity or top-level policy drifted: %+v", value)
	}
	if len(value.Stages) != 1 {
		t.Fatalf("formal News Search template must contain exactly one Stage, got %d", len(value.Stages))
	}
	got := value.Stages[0]
	if got.Code != newsstage.SearchCandidatesStageCode || !got.Required ||
		got.Applicability.RuleRef != newsSearchApplicabilityRuleRef ||
		got.Applicability.RuleVersion != newsSearchApplicabilityVersion {
		t.Fatalf("formal Stage identity or applicability drifted: %+v", got)
	}
	if got.Retry.Ref != newsSearchRetryPolicyRef || got.Retry.MaxAttempts != 2 ||
		got.Retry.AttemptTimeoutMS != 30000 || got.Retry.Backoff.Algorithm != "exponential_full_jitter_v1" ||
		got.Retry.Backoff.BaseDelayMS != 100 || got.Retry.Backoff.MaxDelayMS != 1000 ||
		len(got.Retry.RetryableReasonCodes) != 1 ||
		got.Retry.RetryableReasonCodes[0] != "news_provider_retryable_failure" {
		t.Fatalf("formal RetryPolicy drifted: %+v", got.Retry)
	}
	if got.Propagation.OnDegraded != domain.PropagateAccept ||
		got.Propagation.OnSkipped != domain.PropagateFail ||
		got.Propagation.OnFailed != domain.PropagateFail ||
		got.Propagation.OnTimedOut != domain.PropagateFail ||
		got.Propagation.OnBlocked != domain.PropagateFail {
		t.Fatalf("formal propagation policy drifted: %+v", got.Propagation)
	}
}

func TestBuildNewsSearchWorkflowTemplateFailsClosedOnMissingPrerequisites(t *testing.T) {
	stage, err := newNewsSearchStageCompositionWithProviders(newsapp.StaticProviderSet{})
	if err != nil {
		t.Fatal(err)
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	broken := *stage
	broken.registry = nil
	if _, err := buildNewsSearchWorkflowTemplate(&broken, policies); err == nil {
		t.Fatal("missing Output Registry was accepted")
	}
	if _, err := buildNewsSearchWorkflowTemplate(stage, nil); err == nil {
		t.Fatal("missing ResourcePolicy Registry was accepted")
	}
	broken = *stage
	broken.parameterSchemas = nil
	if _, err := buildNewsSearchWorkflowTemplate(&broken, policies); err == nil {
		t.Fatal("missing parameter Schema validator was accepted")
	}
	broken = *stage
	broken.applicability = nil
	if _, err := buildNewsSearchWorkflowTemplate(&broken, policies); err == nil {
		t.Fatal("missing Applicability evaluator was accepted")
	}
}
