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
	"example.com/stock-analysis-flow/internal/quant/metric"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	"example.com/stock-analysis-flow/internal/quant/trend"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	quantTrendTestResultID    = "quant_result:sha256:" + strings.Repeat("d", 64)
	quantTrendTestSnapshotRef = "data_snapshot:sha256:" + strings.Repeat("e", 64)
)

// quantTrendEvidence 构造与 D1 周期集合一致的四个证据 Metric。
func quantTrendEvidence(t *testing.T, values [4]string, longAvailable bool) []metric.Metric {
	t.Helper()
	codes := [4]string{"ma20_distance", "momentum_5d", "momentum_20d", "momentum_60d"}
	periods := [4]string{"20d", "5d", "20d", "60d"}
	samples := [4]int{20, 6, 21, 61}
	evidence := make([]metric.Metric, 4)
	for index := range codes {
		value := json.RawMessage(`"` + values[index] + `"`)
		status := metric.StatusAvailable
		sampleCount := samples[index]
		if index == 3 && !longAvailable {
			value = json.RawMessage("null")
			status = metric.StatusInsufficientData
			sampleCount = 30
		}
		evidence[index] = metric.Metric{
			MetricCode:         codes[index],
			Value:              value,
			Unit:               "ratio",
			Period:             periods[index],
			SystemAsOf:         "2026-11-20T08:00:00Z",
			QualityStatus:      metric.QualityValid,
			MetricStatus:       status,
			CalculationVersion: "1.0.0",
			SampleCount:        &sampleCount,
			LowerBound:         json.RawMessage("null"),
			UpperBound:         json.RawMessage("null"),
		}
	}
	return evidence
}

type quantTrendStageAdapterStub struct {
	request quantstage.TrendRequest
	output  quantstage.TrendOutput
	quality quantstage.TrendQualitySummary
	err     error
	calls   int
}

func (stub *quantTrendStageAdapterStub) Execute(
	_ context.Context,
	request quantstage.TrendRequest,
) (quantstage.TrendOutput, quantstage.TrendQualitySummary, error) {
	stub.calls++
	stub.request = request
	return stub.output, stub.quality, stub.err
}

func validQuantTrendStageAdapterStub(t *testing.T) *quantTrendStageAdapterStub {
	t.Helper()
	return &quantTrendStageAdapterStub{
		output: quantstage.TrendOutput{
			SchemaVersion: quantstage.TrendSchemaVersion,
			ResultID:      quantTrendTestResultID,
			SnapshotRef:   quantTrendTestSnapshotRef,
			ResultStatus:  "completed",
			Result: trend.Result{
				TrendCode:        trend.CloseTrendCode,
				Direction:        trend.DirectionBullish,
				Phase:            trend.PhaseUnknown,
				Strength:         nil,
				Period:           trend.CloseTrendPeriod,
				EvidenceMetrics:  quantTrendEvidence(t, [4]string{"0.0325", "0.011", "0.048", "0.092"}, true),
				ConflictRefs:     []string{},
				QualityStatus:    trend.QualityValid,
				SystemAsOf:       "2026-11-20T08:00:00Z",
				AlgorithmVersion: trend.CloseTrendAlgorithmVersion,
			},
		},
		quality: quantstage.TrendQualitySummary{
			SchemaVersion: quantstage.TrendQualitySchemaVersion,
			TrendCode:     trend.CloseTrendCode,
			QualityStatus: "valid", ResultQualityStatus: "valid", CalendarQualityStatus: "valid",
			ExpectedTradingDays: 61, ObservedBars: 61, MinimumRequiredBars: 20, CoverageSufficient: true,
		},
	}
}

func validQuantTrendStageParameters() json.RawMessage {
	return json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":61}`)
}

func TestQuantTrendStageExecutorPersistsOutputContract(t *testing.T) {
	adapter := validQuantTrendStageAdapterStub(t)
	result, err := (quantTrendStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.TrendStageCode},
			Parameters:     validQuantTrendStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 || adapter.request.Symbol != "600036.SH" || adapter.request.Limit != 61 ||
		!reflect.DeepEqual(adapter.request.AllowedProviderCodes, []string{quantstage.QuantProviderCode}) {
		t.Fatalf("Trend adapter did not receive the frozen Stage inputs: %+v", adapter)
	}
	if result.AttemptStatus != researchflowdomain.AttemptSucceeded ||
		result.StageStatus != researchflowdomain.StageSucceeded ||
		len(result.ReasonCodes) != 0 || result.ProviderResult == nil {
		t.Fatalf("unexpected successful Trend Stage result: %+v", result)
	}
	if result.ProviderResult.OutputType != quantstage.TrendOutputType ||
		result.ProviderResult.SchemaVersion != quantstage.TrendSchemaVersion {
		t.Fatalf("Trend Stage Output type/version drifted: %+v", result.ProviderResult)
	}
	if err := quantstage.ValidateTrendStageOutputJSON(result.ProviderResult.Payload); err != nil {
		t.Fatalf("Trend Stage payload failed its validator: %v", err)
	}
	if err := quantstage.ValidateTrendQualitySummaryJSON(result.ProviderResult.QualitySummary); err != nil {
		t.Fatalf("Trend Stage quality summary failed its validator: %v", err)
	}
}

func TestQuantTrendStageExecutorMapsUnknownDirectionToDegraded(t *testing.T) {
	adapter := validQuantTrendStageAdapterStub(t)
	adapter.output.Direction = trend.DirectionUnknown
	adapter.output.ResultStatus = "insufficient_data"
	adapter.output.QualityStatus = trend.QualityUnknown
	for index := range adapter.output.EvidenceMetrics {
		adapter.output.EvidenceMetrics[index].MetricStatus = metric.StatusInsufficientData
		adapter.output.EvidenceMetrics[index].Value = json.RawMessage("null")
		sampleCount := []int{19, 5, 20, 30}[index]
		adapter.output.EvidenceMetrics[index].SampleCount = &sampleCount
	}
	adapter.quality.ResultQualityStatus = "unknown"

	result, err := (quantTrendStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.TrendStageCode},
			Parameters:     validQuantTrendStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.AttemptStatus != researchflowdomain.AttemptDegraded ||
		result.StageStatus != researchflowdomain.StageDegraded ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"quant_trend_insufficient_data"}) ||
		result.ProviderResult == nil {
		t.Fatalf("unable-to-assess trend was not degraded with its evidence: %+v", result)
	}
	if err := quantstage.ValidateTrendStageOutputJSON(result.ProviderResult.Payload); err != nil {
		t.Fatalf("degraded Trend payload failed its validator: %v", err)
	}
}

func TestQuantTrendStageExecutorRejectsWrongProviderPolicyAndParameters(t *testing.T) {
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	policy, err := policies.Resolve(resourcepolicy.QuantDailyBarsProvidersRef, []string{"baostock"})
	if err != nil {
		t.Fatal(err)
	}
	adapter := &quantTrendStageAdapterStub{}
	result, err := (quantTrendStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.TrendStageCode},
			Parameters:     validQuantTrendStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: policy,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 0 || result.StageStatus != researchflowdomain.StageFailed ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"quant_resource_policy_invalid"}) {
		t.Fatalf("invalid provider policy reached the adapter: adapter=%+v result=%+v", adapter, result)
	}
	for _, raw := range []string{
		`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":61,"extra":true}`,
		`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":61}{}`,
		`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":61,"period":"5d"}`,
	} {
		result, err := (quantTrendStageExecutor{adapter: adapter}).ExecuteStage(
			context.Background(), researchflowapplication.StageExecutionRequest{
				Stage:          researchflowdomain.Stage{Code: quantstage.TrendStageCode},
				Parameters:     json.RawMessage(raw),
				SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
				ResourcePolicy: validQuantFactorStagePolicy(t),
			},
		)
		if err != nil {
			t.Fatal(err)
		}
		if adapter.calls != 0 || result.StageStatus != researchflowdomain.StageFailed ||
			!reflect.DeepEqual(result.ReasonCodes, []string{"quant_trend_invalid_parameters"}) {
			t.Fatalf("invalid Trend parameters were accepted: %s -> %+v", raw, result)
		}
	}
}

func TestQuantTrendStageExecutorMapsDataAuthorizationDenialToNonRetryableFailure(t *testing.T) {
	adapter := &quantTrendStageAdapterStub{err: marketdataapplication.ErrDataAuthorizationDenied}
	result, err := (quantTrendStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.TrendStageCode},
			Parameters:     validQuantTrendStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed ||
		!reflect.DeepEqual(result.ReasonCodes, []string{"data_authorization_denied"}) {
		t.Fatalf("Data authorization denial was not classified as non-retryable: %+v", result)
	}
}

func TestBuildQuantTrendWorkflowTemplateFreezesParametersAndRegistry(t *testing.T) {
	registry, err := newQuantTrendOutputRegistry()
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	stage := &quantTrendStageComposition{
		registry: registry, parameterSchemas: schemas,
		applicability: quantTrendApplicabilityEvaluator{},
		providerCodes: []string{quantstage.QuantProviderCode},
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := buildQuantTrendWorkflowTemplate(stage, policies)
	if err != nil {
		t.Fatal(err)
	}
	if workflow.Ref != quantTrendWorkflowTemplateRef ||
		workflow.Version != quantTrendWorkflowTemplateVersion ||
		workflow.OutputRegistryVersion != quantTrendStageOutputRegistryVersion ||
		workflow.ParameterSchemaRef != quantTrendParameterSchemaRef ||
		workflow.ResourcePolicyRef != resourcepolicy.QuantDailyBarsProvidersRef ||
		len(workflow.Stages) != 1 || workflow.Stages[0].Code != quantstage.TrendStageCode ||
		workflow.Stages[0].Propagation.OnDegraded != researchflowdomain.PropagateAccept ||
		workflow.RequiresHardRiskGate {
		t.Fatalf("unexpected Quant Trend workflow template: %+v", workflow)
	}
	if err := template.Validate(workflow, nil); err != nil {
		t.Fatalf("frozen Quant Trend workflow template is invalid: %v", err)
	}
}

func TestQuantTrendParameterSchemaIsStrictImmutableAndDocumented(t *testing.T) {
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := validator.Validate(context.Background(), quantTrendParameterSchemaRef, validQuantTrendStageParameters())
	if err != nil {
		t.Fatalf("valid Quant Trend parameters rejected: %v", err)
	}
	if snapshot.Ref != quantTrendParameterSchemaRef ||
		snapshot.Version != quantTrendParameterSchemaVersion || len(snapshot.Fingerprint) != 64 {
		t.Fatalf("unexpected Quant Trend parameter Schema snapshot: %+v", snapshot)
	}
	for _, raw := range []string{
		`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30"}`,
		`{"symbol":"600036","start_date":"2026-05-01","end_date":"2026-10-30","limit":61}`,
		`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":60}`,
		`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":1001}`,
		`{"symbol":"600036.SH","start_date":"2026-05-01","end_date":"2026-10-30","limit":61,"window":60}`,
	} {
		if _, err := validator.Validate(context.Background(), quantTrendParameterSchemaRef, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid Quant Trend parameters were accepted: %s", raw)
		}
	}
	documented, err := os.ReadFile("../../../doc/researchflow/schema/quant-trend-parameters-1.0.0.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(documented) != quantTrendParameterSchemaDocument+"\n" {
		t.Fatal("documented Quant Trend parameter Schema drifted from Server registry")
	}
	golden, err := os.ReadFile("../../../doc/researchflow/golden/quant-trend-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), quantTrendParameterSchemaRef, golden); err != nil {
		t.Fatalf("Quant Trend parameter Golden does not satisfy its Schema: %v", err)
	}
	applicable, reason, err := (researchflowApplicabilityEvaluator{}).Evaluate(
		context.Background(),
		template.ApplicabilityRule{RuleRef: quantTrendApplicabilityRuleRef, RuleVersion: quantTrendApplicabilityVersion},
		validQuantTrendStageParameters(),
	)
	if err != nil || !applicable || reason != "" {
		t.Fatalf("Quant Trend applicability rule rejected validated parameters: applicable=%v reason=%q err=%v", applicable, reason, err)
	}
	if _, _, err := (quantTrendApplicabilityEvaluator{}).Evaluate(
		context.Background(),
		template.ApplicabilityRule{RuleRef: quantTrendApplicabilityRuleRef, RuleVersion: "9.9.9"},
		validQuantTrendStageParameters(),
	); err == nil {
		t.Fatal("Quant Trend applicability evaluator accepted an unknown rule version")
	}
}

func TestResearchflowStageDispatcherRoutesQuantTrendStage(t *testing.T) {
	trendExecutor := &recordingQuantStageExecutor{}
	dispatcher := researchflowStageDispatcher{quantTrend: trendExecutor}
	if _, err := dispatcher.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: quantstage.TrendStageCode},
	}); err != nil {
		t.Fatalf("dispatcher rejected the registered Quant Trend Stage: %v", err)
	}
	if trendExecutor.calls != 1 || trendExecutor.stageCode != quantstage.TrendStageCode {
		t.Fatalf("Quant Trend Stage was not routed to its executor: %+v", trendExecutor)
	}
	if _, err := (researchflowStageDispatcher{}).ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: quantstage.TrendStageCode},
	}); err == nil {
		t.Fatal("dispatcher accepted an unconfigured Quant Trend Stage")
	}
}
