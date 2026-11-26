package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	quantFactorTestResultID    = "quant_result:sha256:" + strings.Repeat("a", 64)
	quantFactorTestSnapshotRef = "data_snapshot:sha256:" + strings.Repeat("b", 64)
)

type quantFactorStageAdapterStub struct {
	request quantstage.FactorRequest
	output  quantstage.FactorOutput
	quality quantstage.FactorQualitySummary
	err     error
	calls   int
}

func (stub *quantFactorStageAdapterStub) Execute(
	_ context.Context,
	request quantstage.FactorRequest,
) (quantstage.FactorOutput, quantstage.FactorQualitySummary, error) {
	stub.calls++
	stub.request = request
	return stub.output, stub.quality, stub.err
}

func validQuantFactorStageAdapterStub() *quantFactorStageAdapterStub {
	return &quantFactorStageAdapterStub{
		output: quantstage.FactorOutput{
			SchemaVersion:      quantstage.FactorSchemaVersion,
			ResultID:           quantFactorTestResultID,
			SnapshotRef:        quantFactorTestSnapshotRef,
			MetricCode:         "ma20_distance",
			Value:              json.RawMessage(`"0.904761904762"`),
			Unit:               "ratio",
			Period:             "20d",
			MetricStatus:       "available",
			SampleCount:        20,
			TradeDate:          "2026-10-30",
			SystemAsOf:         "2026-11-20T08:00:00Z",
			QualityStatus:      "valid",
			ResultStatus:       "completed",
			CalculationVersion: "1.0.0",
		},
		quality: quantstage.FactorQualitySummary{
			SchemaVersion: quantstage.FactorQualitySchemaVersion,
			MetricCode:    "ma20_distance",
			QualityStatus: "valid", CalendarQualityStatus: "valid",
			ExpectedTradingDays: 20, ObservedBars: 20, MinimumRequiredBars: 20, CoverageSufficient: true,
		},
	}
}

func validQuantFactorStagePolicy(t *testing.T) resourcepolicy.Snapshot {
	t.Helper()
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{quantstage.QuantProviderCode})
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func validQuantFactorStageParameters() json.RawMessage {
	return json.RawMessage(`{"factor_code":"ma20_distance","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`)
}

func TestQuantFactorStageExecutorPersistsOutputContractForProductionFactor(t *testing.T) {
	adapter := validQuantFactorStageAdapterStub()
	result, err := (quantFactorStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.FactorStageCode},
			Parameters:     validQuantFactorStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 || adapter.request.FactorCode != "ma20_distance" ||
		adapter.request.Symbol != "600036.SH" || adapter.request.Limit != 20 ||
		!reflect.DeepEqual(adapter.request.AllowedProviderCodes, []string{quantstage.QuantProviderCode}) {
		t.Fatalf("Factor adapter did not receive the frozen Stage inputs: %+v", adapter)
	}
	if result.AttemptStatus != researchflowdomain.AttemptSucceeded ||
		result.StageStatus != researchflowdomain.StageSucceeded ||
		len(result.ReasonCodes) != 0 || result.ProviderResult == nil {
		t.Fatalf("unexpected successful Factor Stage result: %+v", result)
	}
	if result.ProviderResult.OutputType != quantstage.FactorOutputType ||
		result.ProviderResult.SchemaVersion != quantstage.FactorSchemaVersion {
		t.Fatalf("Factor Stage Output type/version drifted: %+v", result.ProviderResult)
	}
	if err := quantstage.ValidateFactorStageOutputJSON(result.ProviderResult.Payload); err != nil {
		t.Fatalf("Factor Stage payload failed its validator: %v", err)
	}
	if err := quantstage.ValidateFactorQualitySummaryJSON(result.ProviderResult.QualitySummary); err != nil {
		t.Fatalf("Factor Stage quality summary failed its validator: %v", err)
	}
}

func TestQuantFactorStageExecutorMapsInsufficientDataToDegraded(t *testing.T) {
	adapter := validQuantFactorStageAdapterStub()
	adapter.output.MetricStatus = "insufficient_data"
	adapter.output.ResultStatus = "insufficient_data"
	adapter.output.Value = json.RawMessage("null")
	adapter.output.SampleCount = 5
	adapter.output.MetricCode = "momentum_5d"
	adapter.output.Period = "5d"
	adapter.quality.MetricCode = "momentum_5d"
	adapter.quality.ObservedBars = 5
	adapter.quality.ExpectedTradingDays = 5
	adapter.quality.MinimumRequiredBars = 1

	result, err := (quantFactorStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.FactorStageCode},
			Parameters:     json.RawMessage(`{"factor_code":"momentum_5d","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":6}`),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.AttemptStatus != researchflowdomain.AttemptDegraded ||
		result.StageStatus != researchflowdomain.StageDegraded ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"quant_factor_insufficient_data"}) ||
		result.ProviderResult == nil {
		t.Fatalf("insufficient Factor sample was not degraded with its evidence: %+v", result)
	}
	if err := quantstage.ValidateFactorStageOutputJSON(result.ProviderResult.Payload); err != nil {
		t.Fatalf("degraded Factor payload failed its validator: %v", err)
	}
}

func TestQuantFactorStageExecutorRejectsWrongProviderPolicyBeforeCallingAdapter(t *testing.T) {
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{"baostock"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &quantFactorStageAdapterStub{}
	result, err := (quantFactorStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.FactorStageCode},
			Parameters:     validQuantFactorStageParameters(),
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

func TestQuantFactorStageExecutorRejectsUnknownParametersAndStage(t *testing.T) {
	adapter := &quantFactorStageAdapterStub{}
	executor := quantFactorStageExecutor{adapter: adapter}
	cases := []struct {
		name       string
		stageCode  string
		parameters json.RawMessage
	}{
		{"unknown parameter", quantstage.FactorStageCode, json.RawMessage(`{"factor_code":"ma20_distance","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20,"extra":true}`)},
		{"trailing JSON", quantstage.FactorStageCode, json.RawMessage(`{"factor_code":"ma20_distance","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}{}`)},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
				Stage:          researchflowdomain.Stage{Code: testCase.stageCode},
				Parameters:     testCase.parameters,
				SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
				ResourcePolicy: validQuantFactorStagePolicy(t),
			})
			if err != nil {
				t.Fatal(err)
			}
			if adapter.calls != 0 || result.StageStatus != researchflowdomain.StageFailed ||
				!reflect.DeepEqual(result.ReasonCodes, []string{"quant_factor_invalid_parameters"}) {
				t.Fatalf("invalid Factor parameters were accepted: adapter=%+v result=%+v", adapter, result)
			}
		})
	}
	if _, err := executor.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage:      researchflowdomain.Stage{Code: quantstage.SMA20StageCode},
		Parameters: validQuantFactorStageParameters(),
	}); err == nil {
		t.Fatal("Factor Stage Executor accepted a foreign Stage code")
	}
}

func TestQuantFactorStageExecutorMapsDataAuthorizationDenialToNonRetryableFailure(t *testing.T) {
	adapter := &quantFactorStageAdapterStub{err: marketdataapplication.ErrDataAuthorizationDenied}
	result, err := (quantFactorStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.FactorStageCode},
			Parameters:     validQuantFactorStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"data_authorization_denied"}) {
		t.Fatalf("Data authorization denial was not classified as a non-retryable Stage failure: %+v", result)
	}
}

func TestBuildQuantFactorWorkflowTemplateFreezesParametersResourcePolicyAndOutputRegistry(t *testing.T) {
	registry, err := newQuantFactorOutputRegistry()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	stage := &quantFactorStageComposition{
		registry: registry, parameterSchemas: schemas,
		applicability: quantFactorApplicabilityEvaluator{},
		providerCodes: []string{quantstage.QuantProviderCode},
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := buildQuantFactorWorkflowTemplate(stage, policies)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Ref != quantFactorWorkflowTemplateRef ||
		workflow.Version != quantFactorWorkflowTemplateVersion ||
		workflow.OutputRegistryVersion != quantFactorStageOutputRegistryVersion ||
		workflow.ParameterSchemaRef != quantFactorParameterSchemaRef ||
		workflow.ResourcePolicyRef != resourcepolicy.QuantDailyBarsProvidersRef ||
		len(workflow.Stages) != 1 || workflow.Stages[0].Code != quantstage.FactorStageCode ||
		workflow.RequiresHardRiskGate {
		t.Fatalf("unexpected Quant Factor workflow template: %+v", workflow)
	}
	if err := template.Validate(workflow, nil); err != nil {
		t.Fatalf("frozen Quant Factor workflow template is invalid: %v", err)
	}
}

func TestQuantFactorParameterSchemaIsStrictImmutableAndDocumented(t *testing.T) {
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := validator.Validate(context.Background(), quantFactorParameterSchemaRef, validQuantFactorStageParameters())
	if err != nil {
		t.Fatalf("valid Quant Factor parameters rejected: %v", err)
	}
	if snapshot.Ref != quantFactorParameterSchemaRef ||
		snapshot.Version != quantFactorParameterSchemaVersion || len(snapshot.Fingerprint) != 64 {
		t.Fatalf("unexpected Quant Factor parameter Schema snapshot: %+v", snapshot)
	}
	invalid := []string{
		`{"factor_code":"volatility_20d","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`,
		`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`,
		`{"factor_code":"ma20_distance","symbol":"600036","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`,
		`{"factor_code":"ma20_distance","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":5}`,
		`{"factor_code":"ma20_distance","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":1001}`,
		`{"factor_code":"ma20_distance","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20,"extra":true}`,
	}
	for _, raw := range invalid {
		if _, err := validator.Validate(context.Background(), quantFactorParameterSchemaRef, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid Quant Factor parameters were accepted: %s", raw)
		}
	}
	documented, err := os.ReadFile("../../../doc/researchflow/schema/quant-factor-parameters-1.0.0.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(documented) != quantFactorParameterSchemaDocument+"\n" {
		t.Fatal("documented Quant Factor parameter Schema drifted from Server registry")
	}
	golden, err := os.ReadFile("../../../doc/researchflow/golden/quant-factor-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), quantFactorParameterSchemaRef, golden); err != nil {
		t.Fatalf("Quant Factor parameter Golden does not satisfy its Schema: %v", err)
	}
	applicable, reason, err := (researchflowApplicabilityEvaluator{}).Evaluate(
		context.Background(),
		template.ApplicabilityRule{RuleRef: quantFactorApplicabilityRuleRef, RuleVersion: quantFactorApplicabilityVersion},
		validQuantFactorStageParameters(),
	)
	if err != nil || !applicable || reason != "" {
		t.Fatalf("Quant Factor applicability rule rejected validated parameters: applicable=%v reason=%q err=%v", applicable, reason, err)
	}
	if _, _, err := (quantFactorApplicabilityEvaluator{}).Evaluate(
		context.Background(),
		template.ApplicabilityRule{RuleRef: quantFactorApplicabilityRuleRef, RuleVersion: "9.9.9"},
		validQuantFactorStageParameters(),
	); err == nil {
		t.Fatal("Quant Factor applicability evaluator accepted an unknown rule version")
	}
}

type recordingQuantStageExecutor struct {
	stageCode string
	calls     int
}

func (executor *recordingQuantStageExecutor) ExecuteStage(
	_ context.Context,
	request researchflowapplication.StageExecutionRequest,
) (researchflowapplication.StageExecutionResult, error) {
	executor.calls++
	executor.stageCode = request.Stage.Code
	return researchflowapplication.StageExecutionResult{StageStatus: researchflowdomain.StageSucceeded}, nil
}

func TestResearchflowStageDispatcherRoutesQuantFactorStage(t *testing.T) {
	factor := &recordingQuantStageExecutor{}
	dispatcher := researchflowStageDispatcher{quantFactor: factor}
	if _, err := dispatcher.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: quantstage.FactorStageCode},
	}); err != nil {
		t.Fatalf("dispatcher rejected the registered Quant Factor Stage: %v", err)
	}
	if factor.calls != 1 || factor.stageCode != quantstage.FactorStageCode {
		t.Fatalf("Quant Factor Stage was not routed to its executor: %+v", factor)
	}
	if _, err := (researchflowStageDispatcher{}).ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: quantstage.FactorStageCode},
	}); err == nil {
		t.Fatal("dispatcher accepted an unconfigured Quant Factor Stage")
	}
	sma := &recordingQuantStageExecutor{}
	if _, err := (researchflowStageDispatcher{quantFactor: factor, quant: sma}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage: researchflowdomain.Stage{Code: quantstage.SMA20StageCode},
		},
	); err != nil || sma.calls != 1 {
		t.Fatalf("registering Quant Factor disturbed the Quant SMA20 route: err=%v sma=%+v", err, sma)
	}
}

func TestQuantFactorOutputRegistryVersionDoesNotCollide(t *testing.T) {
	versions := []string{
		newsSearchStageOutputRegistryVersion,
		newsFeedStageOutputRegistryVersion,
		quantSMA20StageOutputRegistryVersion,
		quantFactorStageOutputRegistryVersion,
		quantTrendStageOutputRegistryVersion,
	}
	seen := make(map[string]struct{}, len(versions))
	for _, version := range versions {
		if _, exists := seen[version]; exists {
			t.Fatalf("Stage Output Registry version %q is registered twice", version)
		}
		seen[version] = struct{}{}
	}
}
