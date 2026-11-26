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

// Quant Trend Stage 的 Server 组合。
//
// Output Registry 版本在 Registry Set 内必须唯一（Set 以 version 为键），
// 因此这里使用与 News(1.0.0/1.2.0)、SMA20(1.1.0)、Factor(1.3.0) 不同的独立版本。
const (
	quantTrendStageOutputRegistryVersion = "1.4.0"
	quantTrendPayloadSchemaRef           = "doc/quant/schema/QNT-CTR-003-trend-stage-output-1.0.0.schema.json"
	quantTrendQualitySchemaRef           = "doc/quant/schema/QNT-CTR-003-trend-quality-summary-1.0.0.schema.json"
	quantTrendWorkflowTemplateRef        = "quant.trend"
	quantTrendWorkflowTemplateVersion    = "1.0.0"
	quantTrendRetryPolicyRef             = "quant.trend.retry@1.0.0"
)

type quantTrendStageComposition struct {
	adapter          *quantstage.TrendAdapter
	repository       *quantstorage.MySQLRepository
	executor         researchflowapplication.StageExecutor
	registry         *stageoutput.Registry
	parameterSchemas *parameterschema.Validator
	applicability    template.ApplicabilityEvaluator
	providerCodes    []string
}

func newQuantTrendStageComposition(
	dataModule *marketdatabootstrap.Module,
	shared *resources.Resources,
) (*quantTrendStageComposition, error) {
	if dataModule == nil || shared == nil || shared.DB == nil {
		return nil, errors.New("Quant Trend Stage requires the Data Module and shared MariaDB")
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
	service, err := quantapplication.NewCreateTrendService(
		dataAdapter, quantRegistry, quantRepository, func() time.Time { return time.Now().UTC() },
	)
	if err != nil {
		return nil, fmt.Errorf("create Quant Trend Application Service: %w", err)
	}
	adapter, err := quantstage.NewTrendAdapter(dataAdapter, service)
	if err != nil {
		return nil, fmt.Errorf("create Quant Trend Stage Adapter: %w", err)
	}
	registry, err := newQuantTrendOutputRegistry()
	if err != nil {
		return nil, fmt.Errorf("create Quant Trend Output Registry: %w", err)
	}
	parameterSchemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		return nil, fmt.Errorf("create Quant Trend parameter Schema Validator: %w", err)
	}
	return &quantTrendStageComposition{
		adapter:          adapter,
		repository:       quantRepository,
		executor:         quantTrendStageExecutor{adapter: adapter},
		registry:         registry,
		parameterSchemas: parameterSchemas,
		applicability:    quantTrendApplicabilityEvaluator{},
		providerCodes:    append([]string(nil), allowedProviderCodes...),
	}, nil
}

// newQuantTrendOutputRegistry 构建 quant.trend 的唯一 Output Registry。
// 组合根与契约测试共用本函数，避免测试复制 Entry 而掩盖真实注册漂移。
func newQuantTrendOutputRegistry() (*stageoutput.Registry, error) {
	return stageoutput.NewRegistry(quantTrendStageOutputRegistryVersion, []stageoutput.Entry{{
		StageCode:               quantstage.TrendStageCode,
		OutputType:              quantstage.TrendOutputType,
		SchemaVersion:           quantstage.TrendSchemaVersion,
		PayloadSchemaRef:        quantTrendPayloadSchemaRef,
		QualitySummarySchemaRef: quantTrendQualitySchemaRef,
		PayloadValidator:        stageoutput.ValidatorFunc(func(value json.RawMessage) error { return quantstage.ValidateTrendStageOutputJSON(value) }),
		QualitySummaryValidator: stageoutput.ValidatorFunc(func(value json.RawMessage) error { return quantstage.ValidateTrendQualitySummaryJSON(value) }),
	}})
}

func buildQuantTrendWorkflowTemplate(
	stage *quantTrendStageComposition,
	policies *resourcepolicy.Registry,
) (template.WorkflowTemplate, error) {
	if stage == nil || stage.parameterSchemas == nil || stage.applicability == nil || stage.registry == nil || policies == nil {
		return template.WorkflowTemplate{}, errors.New("Quant Trend template prerequisites are incomplete")
	}
	if stage.registry.Version() != quantTrendStageOutputRegistryVersion {
		return template.WorkflowTemplate{}, fmt.Errorf("Quant Trend Output Registry version mismatch: %q", stage.registry.Version())
	}
	if _, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{quantstage.QuantProviderCode}); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("resolve Quant Data Provider ResourcePolicy: %w", err)
	}
	parameters := json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":61}`)
	schemaSnapshot, err := stage.parameterSchemas.Validate(context.Background(), quantTrendParameterSchemaRef, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate Quant Trend parameter Schema: %w", err)
	}
	if schemaSnapshot.Ref != quantTrendParameterSchemaRef ||
		schemaSnapshot.Version != quantTrendParameterSchemaVersion || len(schemaSnapshot.Fingerprint) != 64 {
		return template.WorkflowTemplate{}, errors.New("Quant Trend parameter Schema returned an inconsistent immutable snapshot")
	}
	applicable, reason, err := stage.applicability.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: quantTrendApplicabilityRuleRef, RuleVersion: quantTrendApplicabilityVersion,
	}, parameters)
	if err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("evaluate Quant Trend applicability rule: %w", err)
	}
	if !applicable || reason != "" {
		return template.WorkflowTemplate{}, errors.New("Quant Trend applicability preflight did not accept the validated parameters")
	}
	entry, ok := stage.registry.Entry(quantstage.TrendStageCode, quantstage.TrendOutputType, quantstage.TrendSchemaVersion)
	if !ok || entry.PayloadSchemaRef != quantTrendPayloadSchemaRef ||
		entry.QualitySummarySchemaRef != quantTrendQualitySchemaRef ||
		entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
		return template.WorkflowTemplate{}, errors.New("Quant Trend Output Registry entry does not match its Stage contract")
	}
	propagation := template.PropagationPolicy{
		OnDegraded: researchflowdomain.PropagateAccept,
		OnSkipped:  researchflowdomain.PropagateFail,
		OnFailed:   researchflowdomain.PropagateFail,
		OnTimedOut: researchflowdomain.PropagateFail,
		OnBlocked:  researchflowdomain.PropagateFail,
	}
	value := template.WorkflowTemplate{
		Ref:                   quantTrendWorkflowTemplateRef,
		Version:               quantTrendWorkflowTemplateVersion,
		RequiresHardRiskGate:  false,
		AllowEmptyInputs:      true,
		ParameterSchemaRef:    quantTrendParameterSchemaRef,
		OutputRegistryVersion: stage.registry.Version(),
		ResourcePolicyRef:     resourcepolicy.QuantDailyBarsProvidersRef,
		Stages: []template.StageDefinition{{
			Code:         quantstage.TrendStageCode,
			Required:     true,
			Dependencies: []string{},
			Priority:     0,
			Applicability: template.ApplicabilityRule{
				RuleRef:     quantTrendApplicabilityRuleRef,
				RuleVersion: quantTrendApplicabilityVersion,
			},
			Retry: template.RetryPolicy{
				Ref:                  quantTrendRetryPolicyRef,
				MaxAttempts:          1,
				RetryableReasonCodes: []string{},
				Backoff:              template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
				AttemptTimeoutMS:     120000,
			},
			Propagation: propagation,
		}},
	}
	if err := template.Validate(value, nil); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate frozen Quant Trend workflow template: %w", err)
	}
	return value, nil
}

type quantTrendStageParameters struct {
	Symbol    string `json:"symbol"`
	StartDate string `json:"start_date"`
	EndDate   string `json:"end_date"`
	Limit     int    `json:"limit"`
}

type quantTrendStageAdapter interface {
	Execute(context.Context, quantstage.TrendRequest) (quantstage.TrendOutput, quantstage.TrendQualitySummary, error)
}

type quantTrendStageExecutor struct {
	adapter quantTrendStageAdapter
}

func (executor quantTrendStageExecutor) ExecuteStage(
	ctx context.Context,
	request researchflowapplication.StageExecutionRequest,
) (researchflowapplication.StageExecutionResult, error) {
	if executor.adapter == nil || ctx == nil || request.Stage.Code != quantstage.TrendStageCode {
		return researchflowapplication.StageExecutionResult{}, errors.New("Quant Trend Stage Executor is unavailable")
	}
	providers, err := resolveQuantDailyBarsProviders(request.ResourcePolicy)
	if err != nil {
		return failedQuantTrendStage("quant_resource_policy_invalid"), nil
	}
	var parameters quantTrendStageParameters
	decoder := json.NewDecoder(bytes.NewReader(request.Parameters))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil || ensureQuantJSONEOF(decoder) != nil {
		return failedQuantTrendStage("quant_trend_invalid_parameters"), nil
	}
	output, quality, err := executor.adapter.Execute(ctx, quantstage.TrendRequest{
		Symbol:    parameters.Symbol,
		StartDate: parameters.StartDate, EndDate: parameters.EndDate,
		Limit: parameters.Limit, SystemAsOf: request.SystemAsOf, AllowedProviderCodes: providers,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptTimedOut,
				StageStatus:   researchflowdomain.StageTimedOut,
				ReasonCodes:   []string{"quant_trend_deadline_exceeded"},
			}, nil
		case errors.Is(err, context.Canceled):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptCancelled,
				StageStatus:   researchflowdomain.StageCancelled,
				ReasonCodes:   []string{"quant_trend_cancelled"},
			}, nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationDenied):
			return failedQuantTrendStage("data_authorization_denied"), nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationUnavailable):
			return failedQuantTrendStage("data_authorization_unavailable"), nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationInvalid):
			return failedQuantTrendStage("data_authorization_reference_invalid"), nil
		case errors.Is(err, quantstage.ErrInvalidTrendStageRequest):
			return failedQuantTrendStage("quant_trend_invalid_parameters"), nil
		default:
			if reasonCode, ok := quantInputInapplicableReasonCode("quant_trend_", err); ok {
				return failedQuantTrendStage(reasonCode), nil
			}
			return failedQuantTrendStage("quant_trend_execution_failed"), nil
		}
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return failedQuantTrendStage("quant_trend_output_encoding_failed"), nil
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return failedQuantTrendStage("quant_trend_quality_encoding_failed"), nil
	}
	providerResult := &stageoutput.ProviderResult{
		OutputType:     quantstage.TrendOutputType,
		SchemaVersion:  quantstage.TrendSchemaVersion,
		Payload:        payload,
		QualitySummary: qualityJSON,
		DiagnosticRefs: []string{},
	}
	switch output.ResultStatus {
	case "completed":
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
			ReasonCodes:    []string{"quant_trend_insufficient_data"},
		}, nil
	default:
		return failedQuantTrendStage("quant_trend_result_status_invalid"), nil
	}
}

func failedQuantTrendStage(reasonCode string) researchflowapplication.StageExecutionResult {
	return researchflowapplication.StageExecutionResult{
		AttemptStatus: researchflowdomain.AttemptFailed,
		StageStatus:   researchflowdomain.StageFailed,
		ReasonCodes:   []string{reasonCode},
	}
}

var _ researchflowapplication.StageExecutor = quantTrendStageExecutor{}
