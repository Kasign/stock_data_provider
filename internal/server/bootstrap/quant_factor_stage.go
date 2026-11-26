package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantregistry "example.com/stock-analysis-flow/internal/quant/registry"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	"example.com/stock-analysis-flow/internal/server/resources"
)

// Quant Factor Stage 的 Server 组合。
//
// Output Registry 版本在 Registry Set 内必须唯一（Set 以 version 为键），
// 因此这里使用与 News(1.0.0/1.1.0/1.2.0) 和 Quant SMA20(1.1.0) 不同的独立版本。
const (
	quantFactorStageOutputRegistryVersion = "1.3.0"
	quantFactorPayloadSchemaRef           = "doc/quant/schema/QNT-CTR-003-factor-stage-output-1.0.0.schema.json"
	quantFactorQualitySchemaRef           = "doc/quant/schema/QNT-CTR-003-factor-quality-summary-1.0.0.schema.json"
	quantFactorWorkflowTemplateRef        = "quant.factor"
	quantFactorWorkflowTemplateVersion    = "1.0.0"
	quantFactorRetryPolicyRef             = "quant.factor.retry@1.0.0"
)

type quantFactorStageComposition struct {
	adapter          *quantstage.FactorAdapter
	repository       *quantstorage.MySQLRepository
	executor         researchflowapplication.StageExecutor
	registry         *stageoutput.Registry
	parameterSchemas *parameterschema.Validator
	applicability    template.ApplicabilityEvaluator
	providerCodes    []string
}

func newQuantFactorStageComposition(
	dataModule *marketdatabootstrap.Module,
	shared *resources.Resources,
) (*quantFactorStageComposition, error) {
	if dataModule == nil || shared == nil || shared.DB == nil {
		return nil, errors.New("Quant Factor Stage requires the Data Module and shared MariaDB")
	}
	allowedProviderCodes := quantDailyBarsProviderCodes(dataModule, quantstage.QuantProviderCode)
	if len(allowedProviderCodes) == 0 {
		return nil, nil
	}
	dataAdapter, err := newQuantDataSnapshotAdapter(dataModule)
	if err != nil {
		return nil, fmt.Errorf("create Quant Data Snapshot adapter: %w", err)
	}
	quantRegistry, err := quantregistry.NewProductionRegistry()
	if err != nil {
		return nil, fmt.Errorf("create Quant production Registry: %w", err)
	}
	quantRepository, err := quantstorage.NewMySQLRepository(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("create Quant result Repository: %w", err)
	}
	service, err := quantapplication.NewCreateFactorService(
		dataAdapter, quantRegistry, quantRepository, func() time.Time { return time.Now().UTC() },
	)
	if err != nil {
		return nil, fmt.Errorf("create Quant Factor Application Service: %w", err)
	}
	adapter, err := quantstage.NewFactorAdapter(dataAdapter, service)
	if err != nil {
		return nil, fmt.Errorf("create Quant Factor Stage Adapter: %w", err)
	}
	registry, err := newQuantFactorOutputRegistry()
	if err != nil {
		return nil, fmt.Errorf("create Quant Factor Output Registry: %w", err)
	}
	parameterSchemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		return nil, fmt.Errorf("create Quant Factor parameter Schema Validator: %w", err)
	}
	return &quantFactorStageComposition{
		adapter:          adapter,
		repository:       quantRepository,
		executor:         quantFactorStageExecutor{adapter: adapter},
		registry:         registry,
		parameterSchemas: parameterSchemas,
		applicability:    quantFactorApplicabilityEvaluator{},
		providerCodes:    append([]string(nil), allowedProviderCodes...),
	}, nil
}

// newQuantFactorOutputRegistry 构建 quant.factor 的唯一 Output Registry。
//
// 组合根与契约测试共用本函数，避免测试自己复制一份 Entry 而掩盖真实注册漂移。
func newQuantFactorOutputRegistry() (*stageoutput.Registry, error) {
	return stageoutput.NewRegistry(quantFactorStageOutputRegistryVersion, []stageoutput.Entry{{
		StageCode:               quantstage.FactorStageCode,
		OutputType:              quantstage.FactorOutputType,
		SchemaVersion:           quantstage.FactorSchemaVersion,
		PayloadSchemaRef:        quantFactorPayloadSchemaRef,
		QualitySummarySchemaRef: quantFactorQualitySchemaRef,
		PayloadValidator:        stageoutput.ValidatorFunc(func(value json.RawMessage) error { return quantstage.ValidateFactorStageOutputJSON(value) }),
		QualitySummaryValidator: stageoutput.ValidatorFunc(func(value json.RawMessage) error { return quantstage.ValidateFactorQualitySummaryJSON(value) }),
	}})
}

func buildQuantFactorWorkflowTemplate(
	stage *quantFactorStageComposition,
	policies *resourcepolicy.Registry,
) (template.WorkflowTemplate, error) {
	if stage == nil || stage.parameterSchemas == nil || stage.applicability == nil || stage.registry == nil || policies == nil {
		return template.WorkflowTemplate{}, errors.New("Quant Factor template prerequisites are incomplete")
	}
	if stage.registry.Version() != quantFactorStageOutputRegistryVersion {
		return template.WorkflowTemplate{}, fmt.Errorf("Quant Factor Output Registry version mismatch: %q", stage.registry.Version())
	}
	if _, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{quantstage.QuantProviderCode}); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("resolve Quant Data Provider ResourcePolicy: %w", err)
	}
	parameters := json.RawMessage(`{"factor_code":"ma20_distance","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`)
	schemaSnapshot, err := stage.parameterSchemas.Validate(context.Background(), quantFactorParameterSchemaRef, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate Quant Factor parameter Schema: %w", err)
	}
	if schemaSnapshot.Ref != quantFactorParameterSchemaRef ||
		schemaSnapshot.Version != quantFactorParameterSchemaVersion || len(schemaSnapshot.Fingerprint) != 64 {
		return template.WorkflowTemplate{}, errors.New("Quant Factor parameter Schema returned an inconsistent immutable snapshot")
	}
	applicable, reason, err := stage.applicability.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: quantFactorApplicabilityRuleRef, RuleVersion: quantFactorApplicabilityVersion,
	}, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("evaluate Quant Factor applicability rule: %w", err)
	}
	if !applicable || reason != "" {
		return template.WorkflowTemplate{}, errors.New("Quant Factor applicability preflight did not accept the validated parameters")
	}
	entry, ok := stage.registry.Entry(quantstage.FactorStageCode, quantstage.FactorOutputType, quantstage.FactorSchemaVersion)
	if !ok || entry.PayloadSchemaRef != quantFactorPayloadSchemaRef ||
		entry.QualitySummarySchemaRef != quantFactorQualitySchemaRef ||
		entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
		return template.WorkflowTemplate{}, errors.New("Quant Factor Output Registry entry does not match its Stage contract")
	}
	propagation := template.PropagationPolicy{
		OnDegraded: researchflowdomain.PropagateAccept,
		OnSkipped:  researchflowdomain.PropagateFail,
		OnFailed:   researchflowdomain.PropagateFail,
		OnTimedOut: researchflowdomain.PropagateFail,
		OnBlocked:  researchflowdomain.PropagateFail,
	}
	value := template.WorkflowTemplate{
		Ref:                   quantFactorWorkflowTemplateRef,
		Version:               quantFactorWorkflowTemplateVersion,
		RequiresHardRiskGate:  false,
		AllowEmptyInputs:      true,
		ParameterSchemaRef:    quantFactorParameterSchemaRef,
		OutputRegistryVersion: stage.registry.Version(),
		ResourcePolicyRef:     resourcepolicy.QuantDailyBarsProvidersRef,
		Stages: []template.StageDefinition{{
			Code:         quantstage.FactorStageCode,
			Required:     true,
			Dependencies: []string{},
			Priority:     0,
			Applicability: template.ApplicabilityRule{
				RuleRef:     quantFactorApplicabilityRuleRef,
				RuleVersion: quantFactorApplicabilityVersion,
			},
			Retry: template.RetryPolicy{
				Ref:                  quantFactorRetryPolicyRef,
				MaxAttempts:          1,
				RetryableReasonCodes: []string{},
				Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
				AttemptTimeoutMS:     120000,
			},
			Propagation: propagation,
		}},
	}
	if err := template.Validate(value, nil); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate frozen Quant Factor workflow template: %w", err)
	}
	return value, nil
}

type quantFactorStageParameters struct {
	FactorCode string `json:"factor_code"`
	Symbol     string `json:"symbol"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
	Limit      int    `json:"limit"`
}

type quantFactorStageAdapter interface {
	Execute(context.Context, quantstage.FactorRequest) (quantstage.FactorOutput, quantstage.FactorQualitySummary, error)
}

type quantFactorStageExecutor struct {
	adapter quantFactorStageAdapter
}

func (executor quantFactorStageExecutor) ExecuteStage(
	ctx context.Context,
	request researchflowapplication.StageExecutionRequest,
) (researchflowapplication.StageExecutionResult, error) {
	if executor.adapter == nil || ctx == nil || request.Stage.Code != quantstage.FactorStageCode {
		return researchflowapplication.StageExecutionResult{}, errors.New("Quant Factor Stage Executor is unavailable")
	}
	providers, err := resolveQuantDailyBarsProviders(request.ResourcePolicy)
	if err != nil {
		return failedQuantFactorStage("quant_resource_policy_invalid"), nil
	}
	var parameters quantFactorStageParameters
	decoder := json.NewDecoder(bytes.NewReader(request.Parameters))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil || ensureQuantJSONEOF(decoder) != nil {
		return failedQuantFactorStage("quant_factor_invalid_parameters"), nil
	}
	output, quality, err := executor.adapter.Execute(ctx, quantstage.FactorRequest{
		FactorCode: parameters.FactorCode,
		Symbol:     parameters.Symbol, StartDate: parameters.StartDate, EndDate: parameters.EndDate,
		Limit: parameters.Limit, SystemAsOf: request.SystemAsOf, AllowedProviderCodes: providers,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptTimedOut,
				StageStatus:   researchflowdomain.StageTimedOut,
				ReasonCodes:   []string{"quant_factor_deadline_exceeded"},
			}, nil
		case errors.Is(err, context.Canceled):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptCancelled,
				StageStatus:   researchflowdomain.StageCancelled,
				ReasonCodes:   []string{"quant_factor_cancelled"},
			}, nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationDenied):
			return failedQuantFactorStage("data_authorization_denied"), nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationUnavailable):
			return failedQuantFactorStage("data_authorization_unavailable"), nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationInvalid):
			return failedQuantFactorStage("data_authorization_reference_invalid"), nil
		case errors.Is(err, quantstage.ErrInvalidFactorStageRequest):
			return failedQuantFactorStage("quant_factor_invalid_parameters"), nil
		default:
			if reasonCode, ok := quantInputInapplicableReasonCode("quant_factor_", err); ok {
				return failedQuantFactorStage(reasonCode), nil
			}
			return failedQuantFactorStage("quant_factor_execution_failed"), nil
		}
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return failedQuantFactorStage("quant_factor_output_encoding_failed"), nil
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return failedQuantFactorStage("quant_factor_quality_encoding_failed"), nil
	}
	providerResult := &stageoutput.ProviderResult{
		OutputType:     quantstage.FactorOutputType,
		SchemaVersion:  quantstage.FactorSchemaVersion,
		Payload:        payload,
		QualitySummary: qualityJSON,
		DiagnosticRefs: []string{},
	}
	switch output.MetricStatus {
	case "available":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptSucceeded,
			StageStatus:    researchflowdomain.StageSucceeded,
			ProviderResult: providerResult,
		}, nil
	case "insufficient_data":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus:  researchflowdomain.AttemptDegraded,
			StageStatus:    researchflowdomain.StageDegraded,
			ProviderResult: providerResult,
			ReasonCodes:    []string{"quant_factor_insufficient_data"},
		}, nil
	default:
		return failedQuantFactorStage("quant_factor_result_status_invalid"), nil
	}
}

func failedQuantFactorStage(reasonCode string) researchflowapplication.StageExecutionResult {
	return researchflowapplication.StageExecutionResult{
		AttemptStatus: researchflowdomain.AttemptFailed,
		StageStatus:   researchflowdomain.StageFailed,
		ReasonCodes:   []string{reasonCode},
	}
}

var _ researchflowapplication.StageExecutor = quantFactorStageExecutor{}
