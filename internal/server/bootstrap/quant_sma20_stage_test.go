package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

type quantSMA20StageAdapterStub struct {
	request quantstage.SMA20Request
	output  quantstage.SMA20Output
	quality quantstage.SMA20QualitySummary
	err     error
	calls   int
}

func (stub *quantSMA20StageAdapterStub) Execute(_ context.Context, request quantstage.SMA20Request) (quantstage.SMA20Output, quantstage.SMA20QualitySummary, error) {
	stub.calls++
	stub.request = request
	return stub.output, stub.quality, stub.err
}

func TestQuantSMA20StageExecutorUsesImmutableProviderPolicyAndPersistsOutputContract(t *testing.T) {
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{"eastmoney"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &quantSMA20StageAdapterStub{
		output: quantstage.SMA20Output{
			SchemaVersion: quantstage.SMA20SchemaVersion,
			ResultID:      "quant_result:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SnapshotRef:   "data_snapshot:sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			MetricCode:    "sma", Value: json.RawMessage(`"43.1200"`), Unit: "CNY/share", Period: "20d",
			MetricStatus: "available", SampleCount: 20, TradeDate: "2026-10-30",
			SystemAsOf: "2026-11-20T08:00:00Z", QualityStatus: "valid", ResultStatus: "completed",
		},
		quality: quantstage.SMA20QualitySummary{
			SchemaVersion: quantstage.SMA20QualitySchema,
			QualityStatus: "valid", CalendarQualityStatus: "valid",
			ExpectedTradingDays: 20, ObservedBars: 20, MinimumRequiredBars: 20, CoverageSufficient: true,
		},
	}
	executor := quantSMA20StageExecutor{adapter: adapter}
	result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage:          researchflowdomain.Stage{Code: quantstage.SMA20StageCode},
		Parameters:     json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`),
		SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
		ResourcePolicy: policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 || adapter.request.Symbol != "600036.SH" ||
		!reflect.DeepEqual(adapter.request.AllowedProviderCodes, []string{"eastmoney"}) {
		t.Fatalf("Quant adapter did not receive the frozen Stage inputs: %+v", adapter)
	}
	if result.AttemptStatus != researchflowdomain.AttemptSucceeded ||
		result.StageStatus != researchflowdomain.StageSucceeded || result.ProviderResult == nil {
		t.Fatalf("unexpected successful Stage result: %+v", result)
	}
	if err := quantstage.ValidateSMA20OutputJSON(result.ProviderResult.Payload); err != nil {
		t.Fatalf("SMA20 output failed its frozen validator: %v", err)
	}
	if err := quantstage.ValidateSMA20QualitySummaryJSON(result.ProviderResult.QualitySummary); err != nil {
		t.Fatalf("SMA20 quality summary failed its frozen validator: %v", err)
	}
}

func TestQuantSMA20StageExecutorRejectsWrongProviderPolicyBeforeCallingAdapter(t *testing.T) {
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{"baostock"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &quantSMA20StageAdapterStub{}
	result, err := (quantSMA20StageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.SMA20StageCode},
			Parameters:     json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: policy,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 0 || result.StageStatus != researchflowdomain.StageFailed ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"quant_resource_policy_invalid"}) {
		t.Fatalf("invalid provider policy was not rejected before Data access: adapter=%+v result=%+v", adapter, result)
	}
}

func TestQuantSMA20StageExecutorMapsDataAuthorizationDenialToNonRetryableFailure(t *testing.T) {
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{"eastmoney"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &quantSMA20StageAdapterStub{err: marketdataapplication.ErrDataAuthorizationDenied}
	result, err := (quantSMA20StageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.SMA20StageCode},
			Parameters:     json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: policy,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"data_authorization_denied"}) ||
		errors.Is(adapter.err, context.DeadlineExceeded) {
		t.Fatalf("Data authorization denial was not classified as a non-retryable Stage failure: %+v", result)
	}
}

func TestQuantSMA20StageExecutorRejectsUnknownParameters(t *testing.T) {
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{"eastmoney"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &quantSMA20StageAdapterStub{}
	result, err := (quantSMA20StageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.SMA20StageCode},
			Parameters:     json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20,"unexpected":true}`),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: policy,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 0 || result.StageStatus != researchflowdomain.StageFailed ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"quant_sma20_invalid_parameters"}) {
		t.Fatalf("unknown Stage parameter was accepted: adapter=%+v result=%+v", adapter, result)
	}
}

func TestBuildQuantSMA20WorkflowTemplateFreezesParametersResourcePolicyAndOutputRegistry(t *testing.T) {
	registry, err := stageoutput.NewRegistry(quantSMA20StageOutputRegistryVersion, []stageoutput.Entry{{
		StageCode:               quantstage.SMA20StageCode,
		OutputType:              quantstage.SMA20OutputType,
		SchemaVersion:           quantstage.SMA20SchemaVersion,
		PayloadSchemaRef:        quantSMA20PayloadSchemaRef,
		QualitySummarySchemaRef: quantSMA20QualitySchemaRef,
		PayloadValidator:        stageoutput.ValidatorFunc(quantstage.ValidateSMA20OutputJSON),
		QualitySummaryValidator: stageoutput.ValidatorFunc(quantstage.ValidateSMA20QualitySummaryJSON),
	}})
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	stage := &quantSMA20StageComposition{
		registry: registry, parameterSchemas: schemas,
		applicability: quantSMA20ApplicabilityEvaluator{},
		providerCodes: []string{"eastmoney"},
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := buildQuantSMA20WorkflowTemplate(stage, policies)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Ref != quantSMA20WorkflowTemplateRef ||
		workflow.Version != quantSMA20WorkflowTemplateVersion ||
		workflow.OutputRegistryVersion != quantSMA20StageOutputRegistryVersion ||
		workflow.ResourcePolicyRef != resourcepolicy.QuantDailyBarsProvidersRef ||
		len(workflow.Stages) != 1 || workflow.Stages[0].Code != quantstage.SMA20StageCode ||
		workflow.RequiresHardRiskGate {
		t.Fatalf("unexpected Quant SMA20 workflow template: %+v", workflow)
	}
	if err := template.Validate(workflow, nil); err != nil {
		t.Fatalf("frozen Quant SMA20 workflow template is invalid: %v", err)
	}
}
