package bootstrap

import (
	"errors"
	"fmt"

	"example.com/stock-analysis-flow/internal/quant/stage"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

const (
	// quantResearchWorkflowTemplateRef is the four-Stage parallel research
	// template. v1 declares no Stage dependencies (D2) and no hard risk gate (D4).
	quantResearchWorkflowTemplateRef     = "quant.research"
	quantResearchWorkflowTemplateVersion = "1.0.0"
	// 1.8.0 is the next free Output Registry version after news 1.0.0/1.2.0,
	// sma20/factor/trend 1.1.0/1.3.0/1.4.0 and R2 risk 1.7.0. The previous
	// combined Set 1.6.0 remains frozen and is never replaced in place.
	quantResearchStageOutputRegistryVersion = "1.8.0"
)

// newQuantResearchOutputRegistry builds the combined Output Registry for the
// quant.research template. Entries are read from the four single-Stage factories
// so there is exactly one source of truth for each discriminator and validator;
// hand-copying entries here would let a single-Stage contract drift unnoticed.
func newQuantResearchOutputRegistry() (*stageoutput.Registry, error) {
	factories := []func() (*stageoutput.Registry, error){
		newQuantSMA20OutputRegistry,
		newQuantFactorOutputRegistry,
		newQuantTrendOutputRegistry,
		newQuantRiskOutputRegistry,
	}
	discriminators := [][3]string{
		{stage.SMA20StageCode, stage.SMA20OutputType, stage.SMA20SchemaVersion},
		{stage.FactorStageCode, stage.FactorOutputType, stage.FactorSchemaVersion},
		{stage.TrendStageCode, stage.TrendOutputType, stage.TrendSchemaVersion},
		{stage.RiskStageCode, stage.RiskOutputType, stage.RiskSchemaVersion},
	}
	entries := make([]stageoutput.Entry, 0, len(factories))
	for i, factory := range factories {
		registry, err := factory()
		if err != nil {
			return nil, fmt.Errorf("build combined Quant Output Registry: %w", err)
		}
		d := discriminators[i]
		entry, ok := registry.Entry(d[0], d[1], d[2])
		if !ok {
			return nil, fmt.Errorf("combined Quant Output Registry: missing entry %s/%s@%s", d[0], d[1], d[2])
		}
		entries = append(entries, entry)
	}
	registry, err := stageoutput.NewRegistry(quantResearchStageOutputRegistryVersion, entries)
	if err != nil {
		return nil, fmt.Errorf("build combined Quant Output Registry: %w", err)
	}
	return registry, nil
}

// propagation matches all four single-Stage Quant templates and decision D5:
// degraded is accepted, every other non-success propagates as Fail. risk's
// `reject` is a succeeded Stage carrying decision_status=reject, so it never
// traverses the degraded path.
func quantResearchPropagation() template.PropagationPolicy {
	return template.PropagationPolicy{
		OnDegraded: researchflowdomain.PropagateAccept,
		OnSkipped:  researchflowdomain.PropagateFail,
		OnFailed:   researchflowdomain.PropagateFail,
		OnTimedOut: researchflowdomain.PropagateFail,
		OnBlocked:  researchflowdomain.PropagateFail,
	}
}

// retry builds an identical bounded retry policy for one Stage of the combined
// template. MaxAttempts=1 and empty retry codes match the single-Stage templates;
// the per-Stage timeout is preserved from the corresponding single template.
func quantResearchRetry(ref string, attemptTimeoutMS int64) template.RetryPolicy {
	return template.RetryPolicy{
		Ref:                  ref,
		MaxAttempts:          1,
		RetryableReasonCodes: []string{},
		Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
		AttemptTimeoutMS:     attemptTimeoutMS,
	}
}

// buildQuantResearchWorkflowTemplate builds the four-Stage parallel template.
// preflight stages carry the already-built single-Stage Registries; the combined
// Registry is built here and its version is referenced by the template. The
// template is only assembled when all four Quant Stages are configured.
func buildQuantResearchWorkflowTemplate(
	sma, factor, trend, risk *stageoutput.Registry,
	policies *resourcepolicy.Registry,
) (template.WorkflowTemplate, *stageoutput.Registry, error) {
	if sma == nil || factor == nil || trend == nil || risk == nil || policies == nil {
		return template.WorkflowTemplate{}, nil, errors.New("quant.research template prerequisites are incomplete")
	}
	if sma.Version() != quantSMA20StageOutputRegistryVersion ||
		factor.Version() != quantFactorStageOutputRegistryVersion ||
		trend.Version() != quantTrendStageOutputRegistryVersion ||
		risk.Version() != quantRiskStageOutputRegistryVersion {
		return template.WorkflowTemplate{}, nil, errors.New("quant.research Stage Output Registries do not match their frozen versions")
	}
	combined, err := newQuantResearchOutputRegistry()
	if err != nil {
		return template.WorkflowTemplate{}, nil, err
	}
	propagation := quantResearchPropagation()
	value := template.WorkflowTemplate{
		Ref:                   quantResearchWorkflowTemplateRef,
		Version:               quantResearchWorkflowTemplateVersion,
		RequiresHardRiskGate:  false,
		AllowEmptyInputs:      true,
		ParameterSchemaRef:    quantResearchParameterSchemaRef,
		ParameterScope:        template.ParameterScopePerStage,
		OutputRegistryVersion: combined.Version(),
		ResourcePolicyRef:     resourcepolicy.QuantDailyBarsProvidersRef,
		Stages: []template.StageDefinition{
			{
				Code:         stage.SMA20StageCode,
				Required:     true,
				Dependencies: []string{},
				Applicability: template.ApplicabilityRule{
					RuleRef: quantSMA20ApplicabilityRuleRef, RuleVersion: quantSMA20ApplicabilityVersion,
				},
				Retry:       quantResearchRetry(quantSMA20RetryPolicyRef, 120000),
				Propagation: propagation,
			},
			{
				Code:         stage.FactorStageCode,
				Required:     true,
				Dependencies: []string{},
				Applicability: template.ApplicabilityRule{
					RuleRef: quantFactorApplicabilityRuleRef, RuleVersion: quantFactorApplicabilityVersion,
				},
				Retry:       quantResearchRetry(quantFactorRetryPolicyRef, 120000),
				Propagation: propagation,
			},
			{
				Code:         stage.TrendStageCode,
				Required:     true,
				Dependencies: []string{},
				Applicability: template.ApplicabilityRule{
					RuleRef: quantTrendApplicabilityRuleRef, RuleVersion: quantTrendApplicabilityVersion,
				},
				Retry:       quantResearchRetry(quantTrendRetryPolicyRef, 120000),
				Propagation: propagation,
			},
			{
				Code:         stage.RiskStageCode,
				Required:     true,
				Dependencies: []string{},
				Applicability: template.ApplicabilityRule{
					RuleRef: quantRiskApplicabilityRuleRef, RuleVersion: quantRiskApplicabilityVersion,
				},
				Retry:       quantResearchRetry(quantRiskRetryPolicyRef, 60000),
				Propagation: propagation,
			},
		},
	}
	if err := template.Validate(value, nil); err != nil {
		return template.WorkflowTemplate{}, nil, fmt.Errorf("validate quant.research workflow template: %w", err)
	}
	return value, combined, nil
}
