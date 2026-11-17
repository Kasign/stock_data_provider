package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	newsstage "example.com/stock-analysis-flow/internal/news/stage"
	"example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

const (
	newsSearchWorkflowTemplateRef     = "news.search_candidates"
	newsSearchWorkflowTemplateVersion = "1.0.0"
	newsSearchRetryPolicyRef          = "news.search_candidates.retry@1.0.0"
)

func buildNewsSearchWorkflowTemplate(stage *newsSearchStageComposition, policies *resourcepolicy.Registry) (template.WorkflowTemplate, error) {
	if stage == nil || stage.parameterSchemas == nil || stage.applicability == nil || stage.registry == nil || policies == nil {
		return template.WorkflowTemplate{}, errors.New("News Search template prerequisites are incomplete")
	}
	if stage.registry.Version() != newsSearchStageOutputRegistryVersion {
		return template.WorkflowTemplate{}, fmt.Errorf("News Search Output Registry version mismatch: %q", stage.registry.Version())
	}
	if newsSearchParameterSchemaRef == "" || newsSearchParameterSchemaVersion != "1.0.0" {
		return template.WorkflowTemplate{}, errors.New("News Search parameter Schema identity is invalid")
	}
	if resourcepolicy.NewsSearchConfiguredProvidersRef != "news.search.configured-providers@1.0.0" {
		return template.WorkflowTemplate{}, errors.New("News Search ResourcePolicy identity is invalid")
	}
	// Resolve with an empty inventory: ErrNoProviders proves the exact policy
	// ref is registered while preserving the intentional fail-closed behavior.
	if _, err := policies.Resolve(resourcepolicy.NewsSearchConfiguredProvidersRef, nil); !errors.Is(err, resourcepolicy.ErrNoProviders) {
		if err != nil {
			return template.WorkflowTemplate{}, fmt.Errorf("resolve registered News Search ResourcePolicy: %w", err)
		}
		return template.WorkflowTemplate{}, errors.New("News Search ResourcePolicy unexpectedly accepted an empty Provider inventory")
	}

	parameters := json.RawMessage(`{"query":"template publication preflight","limit":10}`)
	schemaSnapshot, err := stage.parameterSchemas.Validate(context.Background(), newsSearchParameterSchemaRef, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate registered News Search parameter Schema: %w", err)
	}
	if schemaSnapshot.Ref != newsSearchParameterSchemaRef || schemaSnapshot.Version != newsSearchParameterSchemaVersion || len(schemaSnapshot.Fingerprint) != 64 {
		return template.WorkflowTemplate{}, errors.New("News Search parameter Schema returned an inconsistent immutable snapshot")
	}
	applicable, reason, err := stage.applicability.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: newsSearchApplicabilityRuleRef, RuleVersion: newsSearchApplicabilityVersion,
	}, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("evaluate registered News Search applicability rule: %w", err)
	}
	if !applicable || reason != "" {
		return template.WorkflowTemplate{}, errors.New("News Search applicability rule did not include the required Stage")
	}
	entry, ok := stage.registry.Entry(newsstage.SearchCandidatesStageCode, newsstage.SearchCandidatesOutputType, newsstage.SearchCandidatesSchemaVersion)
	if !ok || entry.PayloadSchemaRef != newsstage.SearchCandidatesPayloadSchemaRef ||
		entry.QualitySummarySchemaRef != newsstage.SearchCandidatesQualitySummarySchemaRef ||
		entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
		return template.WorkflowTemplate{}, errors.New("News Search Output Registry entry does not match the frozen Stage output contract")
	}

	propagation := template.PropagationPolicy{
		OnDegraded: domain.PropagateAccept,
		OnSkipped:  domain.PropagateFail,
		OnFailed:   domain.PropagateFail,
		OnTimedOut: domain.PropagateFail,
		OnBlocked:  domain.PropagateFail,
	}
	value := template.WorkflowTemplate{
		Ref:                   newsSearchWorkflowTemplateRef,
		Version:               newsSearchWorkflowTemplateVersion,
		RequiresHardRiskGate:  false,
		AllowEmptyInputs:      true,
		ParameterSchemaRef:    newsSearchParameterSchemaRef,
		OutputRegistryVersion: stage.registry.Version(),
		ResourcePolicyRef:     resourcepolicy.NewsSearchConfiguredProvidersRef,
		Stages: []template.StageDefinition{{
			Code:          newsstage.SearchCandidatesStageCode,
			Required:      true,
			Dependencies:  []string{},
			Priority:      0,
			Applicability: template.ApplicabilityRule{RuleRef: newsSearchApplicabilityRuleRef, RuleVersion: newsSearchApplicabilityVersion},
			Retry: template.RetryPolicy{
				Ref:                  newsSearchRetryPolicyRef,
				MaxAttempts:          2,
				RetryableReasonCodes: []string{"news_provider_retryable_failure"},
				Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
				AttemptTimeoutMS:     30000,
			},
			Propagation: propagation,
		}},
	}
	if err := template.Validate(value, nil); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate frozen News Search workflow template: %w", err)
	}
	return value, nil
}
