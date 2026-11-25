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
	newsFeedWorkflowTemplateRef = "news.feed_collection"
	newsFeedWorkflowTemplateVersion = "1.0.0"
	newsFeedRetryPolicyRef = "news.feed_collection.retry@1.0.0"
)

func buildNewsFeedWorkflowTemplate(
	stage *newsFeedStageComposition,
	policies *resourcepolicy.Registry,
) (template.WorkflowTemplate, error) {
	if stage == nil || stage.registry == nil || policies == nil || len(stage.resourceCodes) == 0 {
		return template.WorkflowTemplate{}, errors.New("News Feed template prerequisites are incomplete")
	}
	if stage.registry.Version() != newsFeedStageOutputRegistryVersion {
		return template.WorkflowTemplate{}, fmt.Errorf("News Feed Output Registry version mismatch: %q", stage.registry.Version())
	}
	if newsFeedParameterSchemaRef == "" || newsFeedParameterSchemaVersion != "1.0.0" {
		return template.WorkflowTemplate{}, errors.New("News Feed parameter Schema identity is invalid")
	}
	if resourcepolicy.NewsFeedConfiguredSourcesRef != "news.feed.configured-sources@1.0.0" {
		return template.WorkflowTemplate{}, errors.New("News Feed ResourcePolicy identity is invalid")
	}
	if _, err := policies.ResolveSourceEndpoints(resourcepolicy.NewsFeedConfiguredSourcesRef, stage.resourceRefs); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("resolve registered News Feed ResourcePolicy: %w", err)
	}

	parameters := json.RawMessage(`{}`)
	schemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("create News Feed parameter Schema Validator: %w", err)
	}
	schemaSnapshot, err := schemas.Validate(context.Background(), newsFeedParameterSchemaRef, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate registered News Feed parameter Schema: %w", err)
	}
	if schemaSnapshot.Ref != newsFeedParameterSchemaRef || schemaSnapshot.Version != newsFeedParameterSchemaVersion || len(schemaSnapshot.Fingerprint) != 64 {
		return template.WorkflowTemplate{}, errors.New("News Feed parameter Schema returned an inconsistent immutable snapshot")
	}
	applicability := newsFeedApplicabilityEvaluator{}
	applicable, reason, err := applicability.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: newsFeedApplicabilityRuleRef, RuleVersion: newsFeedApplicabilityVersion,
	}, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("evaluate registered News Feed applicability rule: %w", err)
	}
	if !applicable || reason != "" {
		return template.WorkflowTemplate{}, errors.New("News Feed applicability rule did not include the configured-source Stage")
	}
	entry, ok := stage.registry.Entry(newsstage.CollectFeedsStageCode, newsstage.CollectFeedsOutputType, newsstage.CollectFeedsSchemaVersion)
	if !ok || entry.PayloadSchemaRef != newsstage.CollectFeedsPayloadSchemaRef ||
		entry.QualitySummarySchemaRef != newsstage.CollectFeedsQualitySummarySchemaRef ||
		entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
		return template.WorkflowTemplate{}, errors.New("News Feed Output Registry entry does not match the frozen Stage output contract")
	}

	value := template.WorkflowTemplate{
		Ref: newsFeedWorkflowTemplateRef,
		Version: newsFeedWorkflowTemplateVersion,
		RequiresHardRiskGate: false,
		AllowEmptyInputs: true,
		ParameterSchemaRef: newsFeedParameterSchemaRef,
		OutputRegistryVersion: stage.registry.Version(),
		ResourcePolicyRef: resourcepolicy.NewsFeedConfiguredSourcesRef,
		Stages: []template.StageDefinition{{
			Code: newsstage.CollectFeedsStageCode,
			Required: true,
			Dependencies: []string{},
			Priority: 0,
			Applicability: template.ApplicabilityRule{RuleRef: newsFeedApplicabilityRuleRef, RuleVersion: newsFeedApplicabilityVersion},
			Retry: template.RetryPolicy{
				Ref: newsFeedRetryPolicyRef,
				MaxAttempts: 3,
				RetryableReasonCodes: []string{"news_feed_retryable_source_failure"},
				Backoff: template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 1000, MaxDelayMS: 30000},
				AttemptTimeoutMS: 60000,
			},
			Propagation: template.PropagationPolicy{
				OnDegraded: domain.PropagatePartial,
				OnSkipped: domain.PropagateFail,
				OnFailed: domain.PropagateFail,
				OnTimedOut: domain.PropagateFail,
				OnBlocked: domain.PropagateFail,
			},
		}},
	}
	if err := template.Validate(value, nil); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate frozen News Feed workflow template: %w", err)
	}
	return value, nil
}
