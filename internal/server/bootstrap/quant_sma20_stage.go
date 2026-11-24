package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantregistry "example.com/stock-analysis-flow/internal/quant/registry"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
	"example.com/stock-analysis-flow/internal/server/resources"
)

const (
	quantSMA20StageOutputRegistryVersion = "1.1.0"
	quantSMA20PayloadSchemaRef = "doc/quant/schema/QNT-CTR-005-sma20-stage-output-1.0.0.schema.json"
	quantSMA20QualitySchemaRef = "doc/quant/schema/QNT-CTR-005-sma20-quality-summary-1.0.0.schema.json"
	quantSMA20WorkflowTemplateRef = "quant.sma20"
	quantSMA20WorkflowTemplateVersion = "1.0.0"
	quantSMA20RetryPolicyRef = "quant.sma20.retry@1.0.0"
)

type quantSMA20StageComposition struct {
	adapter *quantstage.SMA20Adapter
	executor researchflowapplication.StageExecutor
	registry *stageoutput.Registry
	parameterSchemas *parameterschema.Validator
	applicability template.ApplicabilityEvaluator
	providerCodes []string
}

func newQuantSMA20StageComposition(
	dataModule *marketdatabootstrap.Module,
	shared *resources.Resources,
) (*quantSMA20StageComposition, error) {
	if dataModule == nil || shared == nil || shared.DB == nil {
		return nil, errors.New("Quant SMA20 Stage requires the Data Module and shared MariaDB")
	}
	providers := dataModule.ProviderSummary().DailyBars
	sort.Strings(providers)
	if !containsString(providers, quantstage.SMA20ProviderCode) {
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
	service, err := quantapplication.NewCreateSMAService(
		dataAdapter, quantRegistry, quantRepository, func() time.Time { return time.Now().UTC() },
	)
	if err != nil {
		return nil, fmt.Errorf("create Quant SMA Application Service: %w", err)
	}
	adapter, err := quantstage.NewSMA20Adapter(dataAdapter, service)
	if err != nil {
		return nil, fmt.Errorf("create Quant SMA20 Stage Adapter: %w", err)
	}
	registry, err := stageoutput.NewRegistry(quantSMA20StageOutputRegistryVersion, []stageoutput.Entry{{
		StageCode: quantstage.SMA20StageCode,
		OutputType: quantstage.SMA20OutputType,
		SchemaVersion: quantstage.SMA20SchemaVersion,
		PayloadSchemaRef: quantSMA20PayloadSchemaRef,
		QualitySummarySchemaRef: quantSMA20QualitySchemaRef,
		PayloadValidator: stageoutput.ValidatorFunc(quantstage.ValidateSMA20OutputJSON),
		QualitySummaryValidator: stageoutput.ValidatorFunc(quantstage.ValidateSMA20QualitySummaryJSON),
	}})
	if err != nil {
		return nil, fmt.Errorf("create Quant SMA20 Output Registry: %w", err)
	}
	parameterSchemas, err := newQuantSMA20ParameterSchemaValidator()
	if err != nil {
		return nil, fmt.Errorf("create Quant SMA20 parameter Schema Validator: %w", err)
	}
	return &quantSMA20StageComposition{
		adapter: adapter,
		executor: quantSMA20StageExecutor{adapter: adapter},
		registry: registry,
		parameterSchemas: parameterSchemas,
		applicability: quantSMA20ApplicabilityEvaluator{},
		providerCodes: []string{quantstage.SMA20ProviderCode},
	}, nil
}

func buildQuantSMA20WorkflowTemplate(stage *quantSMA20StageComposition, policies *resourcepolicy.Registry) (template.WorkflowTemplate, error) {
	if stage == nil || stage.parameterSchemas == nil || stage.applicability == nil || stage.registry == nil || policies == nil {
		return template.WorkflowTemplate{}, errors.New("Quant SMA20 template prerequisites are incomplete")
	}
	if stage.registry.Version() != quantSMA20StageOutputRegistryVersion {
		return template.WorkflowTemplate{}, fmt.Errorf("Quant SMA20 Output Registry version mismatch: %q", stage.registry.Version())
	}
	if _, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, append([]string(nil), stage.providerCodes...)); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("resolve Quant Data Provider ResourcePolicy: %w", err)
	}
	parameters := json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`)
	schemaSnapshot, err := stage.parameterSchemas.Validate(context.Background(), quantSMA20ParameterSchemaRef, parameters)
	if err != nil || schemaSnapshot.Ref != quantSMA20ParameterSchemaRef ||
		schemaSnapshot.Version != quantSMA20ParameterSchemaVersion || len(schemaSnapshot.Fingerprint) != 64 {
		return template.WorkflowTemplate{}, fmt.Errorf("validate Quant SMA20 parameter Schema: %w", err)
	}
	applicable, reason, err := stage.applicability.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: quantSMA20ApplicabilityRuleRef, RuleVersion: quantSMA20ApplicabilityVersion,
	}, parameters)
	if err != nil || !applicable || reason != "" {
		return template.WorkflowTemplate{}, fmt.Errorf("evaluate Quant SMA20 applicability rule: %w", err)
	}
	entry, ok := stage.registry.Entry(quantstage.SMA20StageCode, quantstage.SMA20OutputType, quantstage.SMA20SchemaVersion)
	if !ok || entry.PayloadSchemaRef != quantSMA20PayloadSchemaRef ||
		entry.QualitySummarySchemaRef != quantSMA20QualitySchemaRef ||
		entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
		return template.WorkflowTemplate{}, errors.New("Quant SMA20 Output Registry entry does not match its frozen Stage contract")
	}
	propagation := template.PropagationPolicy{
		OnDegraded: researchflowdomain.PropagateAccept,
		OnSkipped: researchflowdomain.PropagateFail,
		OnFailed: researchflowdomain.PropagateFail,
		OnTimedOut: researchflowdomain.PropagateFail,
		OnBlocked: researchflowdomain.PropagateFail,
	}
	value := template.WorkflowTemplate{
		Ref: quantSMA20WorkflowTemplateRef,
		Version: quantSMA20WorkflowTemplateVersion,
		RequiresHardRiskGate: false,
		AllowEmptyInputs: true,
		ParameterSchemaRef: quantSMA20ParameterSchemaRef,
		OutputRegistryVersion: stage.registry.Version(),
		ResourcePolicyRef: resourcepolicy.QuantDailyBarsProvidersRef,
		Stages: []template.StageDefinition{{
			Code: quantstage.SMA20StageCode,
			Required: true,
			Dependencies: []string{},
			Priority: 0,
			Applicability: template.ApplicabilityRule{
				RuleRef: quantSMA20ApplicabilityRuleRef,
				RuleVersion: quantSMA20ApplicabilityVersion,
			},
			Retry: template.RetryPolicy{
				Ref: quantSMA20RetryPolicyRef,
				MaxAttempts: 1,
				RetryableReasonCodes: []string{},
				Backoff: template.BackoffPolicy{Algorithm: "exponential_full_jitter_v1", BaseDelayMS: 100, MaxDelayMS: 1000},
				AttemptTimeoutMS: 120000,
			},
			Propagation: propagation,
		}},
	}
	if err := template.Validate(value, nil); err != nil {
		return template.WorkflowTemplate{}, fmt.Errorf("validate frozen Quant SMA20 workflow template: %w", err)
	}
	return value, nil
}

type quantSMA20StageParameters struct {
	Symbol string `json:"symbol"`
	StartDate string `json:"start_date"`
	EndDate string `json:"end_date"`
	Limit int `json:"limit"`
}

type quantSMA20StageExecutor struct {
	adapter *quantstage.SMA20Adapter
}

func (executor quantSMA20StageExecutor) ExecuteStage(ctx context.Context, request researchflowapplication.StageExecutionRequest) (researchflowapplication.StageExecutionResult, error) {
	if executor.adapter == nil || ctx == nil || request.Stage.Code != quantstage.SMA20StageCode {
		return researchflowapplication.StageExecutionResult{}, errors.New("Quant SMA20 Stage Executor is unavailable")
	}
	providers, err := resolveQuantDailyBarsProviders(request.ResourcePolicy)
	if err != nil {
		return failedQuantSMA20Stage("quant_resource_policy_invalid"), nil
	}
	var parameters quantSMA20StageParameters
	decoder := json.NewDecoder(bytes.NewReader(request.Parameters))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&parameters); err != nil || ensureQuantJSONEOF(decoder) != nil {
		return failedQuantSMA20Stage("quant_sma20_invalid_parameters"), nil
	}
	output, quality, err := executor.adapter.Execute(ctx, quantstage.SMA20Request{
		Symbol: parameters.Symbol, StartDate: parameters.StartDate, EndDate: parameters.EndDate,
		Limit: parameters.Limit, SystemAsOf: request.SystemAsOf, AllowedProviderCodes: providers,
	})
	if err != nil {
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptTimedOut,
				StageStatus: researchflowdomain.StageTimedOut,
				ReasonCodes: []string{"quant_sma20_deadline_exceeded"},
			}, nil
		case errors.Is(err, context.Canceled):
			return researchflowapplication.StageExecutionResult{
				AttemptStatus: researchflowdomain.AttemptCancelled,
				StageStatus: researchflowdomain.StageCancelled,
				ReasonCodes: []string{"quant_sma20_cancelled"},
			}, nil
		case errors.Is(err, marketdataapplication.ErrDataAuthorizationDenied):
			return failedQuantSMA20Stage("data_authorization_denied"), nil
		default:
			return failedQuantSMA20Stage("quant_sma20_execution_failed"), nil
		}
	}
	payload, err := json.Marshal(output)
	if err != nil {
		return failedQuantSMA20Stage("quant_sma20_output_encoding_failed"), nil
	}
	qualityJSON, err := json.Marshal(quality)
	if err != nil {
		return failedQuantSMA20Stage("quant_sma20_quality_encoding_failed"), nil
	}
	providerResult := &stageoutput.ProviderResult{
		OutputType: quantstage.SMA20OutputType,
		SchemaVersion: quantstage.SMA20SchemaVersion,
		Payload: payload,
		QualitySummary: qualityJSON,
		DiagnosticRefs: []string{},
	}
	switch output.MetricStatus {
	case "available":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus: researchflowdomain.AttemptSucceeded,
			StageStatus: researchflowdomain.StageSucceeded,
			ProviderResult: providerResult,
		}, nil
	case "insufficient_data":
		return researchflowapplication.StageExecutionResult{
			AttemptStatus: researchflowdomain.AttemptDegraded,
			StageStatus: researchflowdomain.StageDegraded,
			ProviderResult: providerResult,
			ReasonCodes: []string{"quant_sma20_insufficient_data"},
		}, nil
	default:
		return failedQuantSMA20Stage("quant_sma20_result_status_invalid"), nil
	}
}

func resolveQuantDailyBarsProviders(snapshot resourcepolicy.Snapshot) ([]string, error) {
	registry, err := resourcepolicy.NewRegistry()
	if err != nil {
		return nil, err
	}
	if snapshot.PolicyRef != resourcepolicy.QuantDailyBarsProvidersRef {
		return nil, fmt.Errorf("unexpected Quant ResourcePolicy ref %q", snapshot.PolicyRef)
	}
	if err := registry.ValidateSnapshot(snapshot); err != nil {
		return nil, err
	}
	if !containsQuantProviderCode(snapshot.AllowedProviderCodes, quantstage.SMA20ProviderCode) {
		return nil, resourcepolicy.ErrNoProviders
	}
	return append([]string(nil), snapshot.AllowedProviderCodes...), nil
}

func failedQuantSMA20Stage(reasonCode string) researchflowapplication.StageExecutionResult {
	return researchflowapplication.StageExecutionResult{
		AttemptStatus: researchflowdomain.AttemptFailed,
		StageStatus: researchflowdomain.StageFailed,
		ReasonCodes: []string{reasonCode},
	}
}

func ensureQuantJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("Quant SMA20 parameters contain trailing JSON")
	}
	return nil
}

func containsQuantProviderCode(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

var _ researchflowapplication.StageExecutor = quantSMA20StageExecutor{}
