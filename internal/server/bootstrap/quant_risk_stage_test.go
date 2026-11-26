package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	marketdataapplication "example.com/stock-analysis-flow/internal/marketdata/application"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	"example.com/stock-analysis-flow/internal/quant/risk"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	quantRiskTestResultID    = "quant_result:sha256:" + strings.Repeat("f", 64)
	quantRiskTestSnapshotRef = "data_snapshot:sha256:" + strings.Repeat("c", 64)
	quantRiskTestAuthRef     = "data_authorization:opaque-token"
)

type quantRiskStageAdapterStub struct {
	request quantstage.RiskRequest
	output  quantstage.RiskOutput
	quality quantstage.RiskQualitySummary
	err     error
	calls   int
}

func (stub *quantRiskStageAdapterStub) Execute(
	_ context.Context, request quantstage.RiskRequest,
) (quantstage.RiskOutput, quantstage.RiskQualitySummary, error) {
	stub.calls++
	stub.request = request
	return stub.output, stub.quality, stub.err
}

// validQuantRiskStageAdapterStub 构造"事实齐全但缺涨跌停/可成交量"的真实拒绝形态：
// 这是当前事实边界下唯一可能出现的结论，测试不得伪造 pass。
func validQuantRiskStageAdapterStub() *quantRiskStageAdapterStub {
	return &quantRiskStageAdapterStub{
		output: quantstage.RiskOutput{
			SchemaVersion: quantstage.RiskSchemaVersion,
			ResultID:      quantRiskTestResultID,
			ResultStatus:  "rejected",
			MissingFacts:  []string{"price_limit", "tradable_volume"},
			BlockingFacts: []string{},
			Decision: risk.Decision{
				DecisionID:     "risk_decision_1",
				Status:         risk.DecisionReject,
				RuleSetVersion: risk.TradabilityRuleSetVersion,
				EvaluatedAt:    "2026-11-25T15:00:00Z",
				SystemAsOf:     "2026-11-25T15:00:00Z",
				Reasons: []risk.Reason{{
					Code: risk.TradabilityGateCode, Severity: risk.SeverityBlock, Status: risk.ReasonTriggered,
					SafeMessage:  "可交易性事实不完整，缺失：涨跌停状态、可成交量；无法确认可交易性",
					EvidenceRefs: []string{quantRiskTestSnapshotRef, quantRiskTestAuthRef},
				}},
				InputRefs: []string{quantRiskTestSnapshotRef, quantRiskTestAuthRef},
			},
		},
		quality: quantstage.RiskQualitySummary{
			SchemaVersion: quantstage.RiskQualitySchemaVersion, RuleSetVersion: risk.TradabilityRuleSetVersion,
			QualityStatus: "valid", DecisionStatus: "reject", ResultStatus: "rejected",
			MissingFacts: []string{"price_limit", "tradable_volume"}, BlockingFacts: []string{},
		},
	}
}

func validQuantRiskStageParameters() json.RawMessage {
	return json.RawMessage(`{"symbol":"600519.SH","trade_date":"2026-11-25"}`)
}

func TestQuantRiskStageExecutorProducesTraceableRejection(t *testing.T) {
	adapter := validQuantRiskStageAdapterStub()
	result, err := (quantRiskStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:      researchflowdomain.Stage{Code: quantstage.RiskStageCode},
			Parameters: validQuantRiskStageParameters(),
			SystemAsOf: time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if adapter.calls != 1 || adapter.request.Symbol != "600519.SH" || adapter.request.TradeDate != "2026-11-25" {
		t.Fatalf("Risk adapter did not receive the frozen Stage inputs: %+v", adapter)
	}
	// 拒绝不是 Stage 失败：评估完成即成功，结论在 payload 里。
	if result.AttemptStatus != researchflowdomain.AttemptSucceeded ||
		result.StageStatus != researchflowdomain.StageSucceeded ||
		len(result.ReasonCodes) != 0 || result.ProviderResult == nil {
		t.Fatalf("unexpected Risk Stage result: %+v", result)
	}
	if result.ProviderResult.OutputType != quantstage.RiskOutputType ||
		result.ProviderResult.SchemaVersion != quantstage.RiskSchemaVersion {
		t.Fatalf("Risk Stage Output type/version drifted: %+v", result.ProviderResult)
	}
	if err := quantstage.ValidateRiskStageOutputJSON(result.ProviderResult.Payload); err != nil {
		t.Fatalf("Risk Stage payload failed its validator: %v", err)
	}
	if err := quantstage.ValidateRiskQualitySummaryJSON(result.ProviderResult.QualitySummary); err != nil {
		t.Fatalf("Risk Stage quality summary failed its validator: %v", err)
	}
	var payload quantstage.RiskOutput
	if err := json.Unmarshal(result.ProviderResult.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Status != risk.DecisionReject || payload.DecisionID != "risk_decision_1" ||
		len(payload.MissingFacts) != 2 || payload.ResultStatus != "rejected" {
		t.Fatalf("Risk Stage payload lost its decision: %+v", payload)
	}
}

func TestQuantRiskStageExecutorMapsDegradedFactsToDegradedStage(t *testing.T) {
	adapter := validQuantRiskStageAdapterStub()
	adapter.quality.QualityStatus = "degraded"
	result, err := (quantRiskStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:      researchflowdomain.Stage{Code: quantstage.RiskStageCode},
			Parameters: validQuantRiskStageParameters(),
			SystemAsOf: time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.AttemptStatus != researchflowdomain.AttemptDegraded ||
		result.StageStatus != researchflowdomain.StageDegraded ||
		len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "quant_risk_degraded_facts" {
		t.Fatalf("degraded facts did not degrade the stage: %+v", result)
	}
	if result.ProviderResult == nil {
		t.Fatal("degraded facts must still persist the decision")
	}
}

// 规则执行失败（unknown）不产出可读 Stage Output：否则下游会把 failed 的决策当正常结论。
func TestQuantRiskStageExecutorFailsClosedOnUnknownDecision(t *testing.T) {
	adapter := validQuantRiskStageAdapterStub()
	adapter.output.Status = risk.DecisionUnknown
	adapter.output.ResultStatus = "failed"
	adapter.output.Reasons[0].Status = risk.ReasonUnknown
	adapter.quality.DecisionStatus = "unknown"
	adapter.quality.ResultStatus = "failed"
	result, err := (quantRiskStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:      researchflowdomain.Stage{Code: quantstage.RiskStageCode},
			Parameters: validQuantRiskStageParameters(),
			SystemAsOf: time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed || result.ProviderResult != nil ||
		len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "quant_risk_decision_unknown" {
		t.Fatalf("unknown decision was published as a result: %+v", result)
	}
}

func TestQuantRiskStageExecutorRejectsUnknownParametersAndStage(t *testing.T) {
	for name, request := range map[string]researchflowapplication.StageExecutionRequest{
		"unknown parameter": {
			Stage:      researchflowdomain.Stage{Code: quantstage.RiskStageCode},
			Parameters: json.RawMessage(`{"symbol":"600519.SH","trade_date":"2026-11-25","window":20}`),
		},
		"missing trade date": {
			Stage:      researchflowdomain.Stage{Code: quantstage.RiskStageCode},
			Parameters: json.RawMessage(`{"symbol":"600519.SH"}`),
		},
		"wrong stage": {
			Stage:      researchflowdomain.Stage{Code: quantstage.TrendStageCode},
			Parameters: validQuantRiskStageParameters(),
		},
	} {
		t.Run(name, func(t *testing.T) {
			adapter := validQuantRiskStageAdapterStub()
			result, err := (quantRiskStageExecutor{adapter: adapter}).ExecuteStage(context.Background(), request)
			if name == "wrong stage" {
				if err == nil {
					t.Fatal("executor accepted a foreign Stage code")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if adapter.calls != 0 {
				t.Fatalf("invalid parameters reached the adapter %d times", adapter.calls)
			}
			if result.StageStatus != researchflowdomain.StageFailed ||
				len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "quant_risk_invalid_parameters" {
				t.Fatalf("unexpected invalid-parameter result: %+v", result)
			}
		})
	}
}

func TestQuantRiskStageExecutorMapsDataAuthorizationDenialToNonRetryableFailure(t *testing.T) {
	adapter := validQuantRiskStageAdapterStub()
	adapter.err = marketdataapplication.ErrDataAuthorizationDenied
	result, err := (quantRiskStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:      researchflowdomain.Stage{Code: quantstage.RiskStageCode},
			Parameters: validQuantRiskStageParameters(),
			SystemAsOf: time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed || len(result.ReasonCodes) != 1 ||
		result.ReasonCodes[0] != "data_authorization_denied" {
		t.Fatalf("authorization denial was not mapped: %+v", result)
	}

	adapter = validQuantRiskStageAdapterStub()
	adapter.err = quantapplication.ErrInvalidRiskSnapshot
	result, err = (quantRiskStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:      researchflowdomain.Stage{Code: quantstage.RiskStageCode},
			Parameters: validQuantRiskStageParameters(),
			SystemAsOf: time.Date(2026, 11, 25, 15, 0, 0, 0, time.UTC),
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.StageStatus != researchflowdomain.StageFailed || len(result.ReasonCodes) != 1 ||
		result.ReasonCodes[0] != "quant_risk_invalid_facts_snapshot" {
		t.Fatalf("invalid facts snapshot was not mapped: %+v", result)
	}
}

func TestBuildQuantRiskWorkflowTemplateFreezesParametersRegistryAndPropagation(t *testing.T) {
	stage := &quantRiskStageComposition{
		registry:         mustQuantRiskOutputRegistry(t),
		parameterSchemas: mustResearchflowParameterValidator(t),
		applicability:    quantRiskApplicabilityEvaluator{},
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	value, err := buildQuantRiskWorkflowTemplate(stage, policies)
	if err != nil {
		t.Fatal(err)
	}
	if value.Ref != quantRiskWorkflowTemplateRef || value.Version != quantRiskWorkflowTemplateVersion ||
		value.ParameterSchemaRef != quantRiskParameterSchemaRef ||
		value.OutputRegistryVersion != quantRiskStageOutputRegistryVersion ||
		value.ResourcePolicyRef != resourcepolicy.QuantDailyBarsProvidersRef {
		t.Fatalf("unexpected Risk workflow template: %+v", value)
	}
	// 本轮不声明 hard_risk：组合根 gates 为 nil，且恒 reject 会把所有 Run 打成失败。
	if value.RequiresHardRiskGate {
		t.Fatal("Quant Risk template must not require a hard risk gate in this slice")
	}
	if len(value.Stages) != 1 {
		t.Fatalf("Risk template stages = %d, want 1", len(value.Stages))
	}
	definition := value.Stages[0]
	if definition.Code != quantstage.RiskStageCode || !definition.Required ||
		definition.GateType != "" ||
		definition.Applicability.RuleRef != quantRiskApplicabilityRuleRef ||
		definition.Applicability.RuleVersion != quantRiskApplicabilityVersion ||
		definition.Retry.Ref != quantRiskRetryPolicyRef || definition.Retry.MaxAttempts != 1 {
		t.Fatalf("unexpected Risk Stage definition: %+v", definition)
	}
	if definition.Propagation.OnDegraded != researchflowdomain.PropagateAccept ||
		definition.Propagation.OnFailed != researchflowdomain.PropagateFail ||
		definition.Propagation.OnTimedOut != researchflowdomain.PropagateFail ||
		definition.Propagation.OnBlocked != researchflowdomain.PropagateFail ||
		definition.Propagation.OnSkipped != researchflowdomain.PropagateFail {
		t.Fatalf("unexpected Risk propagation policy: %+v", definition.Propagation)
	}
}

func TestQuantRiskParameterSchemaIsStrictImmutableAndDocumented(t *testing.T) {
	validator := mustResearchflowParameterValidator(t)
	snapshot, err := validator.Validate(context.Background(), quantRiskParameterSchemaRef, validQuantRiskStageParameters())
	if err != nil {
		t.Fatalf("valid Quant Risk parameters rejected: %v", err)
	}
	if snapshot.Ref != quantRiskParameterSchemaRef ||
		snapshot.Version != quantRiskParameterSchemaVersion || len(snapshot.Fingerprint) != 64 {
		t.Fatalf("unexpected Quant Risk parameter Schema snapshot: %+v", snapshot)
	}
	for _, raw := range []string{
		`{"symbol":"600519","trade_date":"2026-11-25"}`,
		`{"symbol":"600519.SH"}`,
		`{"trade_date":"2026-11-25"}`,
		`{"symbol":"600519.SH","trade_date":"2026/11/25"}`,
		`{"symbol":"600519.SH","trade_date":"2026-11-25","window":20}`,
		`{"symbol":"600519.SH","trade_date":"2026-11-25","threshold":0.05}`,
	} {
		if _, err := validator.Validate(context.Background(), quantRiskParameterSchemaRef, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid Quant Risk parameters were accepted: %s", raw)
		}
	}
	documented, err := os.ReadFile("../../../doc/researchflow/schema/quant-risk-parameters-1.0.0.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(documented) != quantRiskParameterSchemaDocument+"\n" {
		t.Fatal("documented Quant Risk parameter Schema drifted from Server registry")
	}
	golden, err := os.ReadFile("../../../doc/researchflow/golden/quant-risk-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), quantRiskParameterSchemaRef, golden); err != nil {
		t.Fatalf("Quant Risk parameter Golden does not satisfy its Schema: %v", err)
	}
	applicable, reason, err := (researchflowApplicabilityEvaluator{}).Evaluate(
		context.Background(),
		template.ApplicabilityRule{RuleRef: quantRiskApplicabilityRuleRef, RuleVersion: quantRiskApplicabilityVersion},
		validQuantRiskStageParameters(),
	)
	if err != nil || !applicable || reason != "" {
		t.Fatalf("Quant Risk applicability rule rejected validated parameters: applicable=%v reason=%q err=%v", applicable, reason, err)
	}
	if _, _, err := (quantRiskApplicabilityEvaluator{}).Evaluate(
		context.Background(),
		template.ApplicabilityRule{RuleRef: quantRiskApplicabilityRuleRef, RuleVersion: "9.9.9"},
		validQuantRiskStageParameters(),
	); err == nil {
		t.Fatal("Quant Risk applicability evaluator accepted an unknown rule version")
	}
}

func TestResearchflowStageDispatcherRoutesQuantRiskStage(t *testing.T) {
	riskExecutor := &recordingQuantStageExecutor{}
	dispatcher := researchflowStageDispatcher{quantRisk: riskExecutor}
	if _, err := dispatcher.ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: quantstage.RiskStageCode},
	}); err != nil {
		t.Fatalf("dispatcher did not route %s: %v", quantstage.RiskStageCode, err)
	}
	if riskExecutor.calls != 1 || riskExecutor.stageCode != quantstage.RiskStageCode {
		t.Fatalf("dispatcher routed %s elsewhere: %+v", quantstage.RiskStageCode, riskExecutor)
	}
	if _, err := (researchflowStageDispatcher{}).ExecuteStage(context.Background(), researchflowapplication.StageExecutionRequest{
		Stage: researchflowdomain.Stage{Code: quantstage.RiskStageCode},
	}); err == nil {
		t.Fatal("dispatcher executed the Risk Stage without its executor")
	}
}

func TestQuantRiskOutputRegistryVersionDoesNotCollide(t *testing.T) {
	riskRegistry := mustQuantRiskOutputRegistry(t)
	for name, version := range map[string]string{
		"news search":  "1.0.0",
		"quant sma20":  "1.1.0",
		"news feed":    "1.2.0",
		"quant factor": "1.3.0",
		"quant trend":  "1.4.0",
	} {
		if riskRegistry.Version() == version {
			t.Fatalf("Quant Risk Output Registry version collides with %s (%s)", name, version)
		}
	}
	if riskRegistry.Version() != quantRiskStageOutputRegistryVersion {
		t.Fatalf("Quant Risk Output Registry version = %q", riskRegistry.Version())
	}
}

func mustQuantRiskOutputRegistry(t *testing.T) *stageoutput.Registry {
	t.Helper()
	registry, err := newQuantRiskOutputRegistry()
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func mustResearchflowParameterValidator(t *testing.T) *parameterschema.Validator {
	t.Helper()
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	return validator
}

// 关联规格表的存在性必须显式断言：契约测试按规格表遍历，
// 漏登记只会让遍历变短，不会失败——这正是"结果落库但读不回来"的成因。
func TestQuantRiskStageIsRegisteredForResultAssociation(t *testing.T) {
	spec, ok := quantStageAssociationSpecFor(quantstage.RiskStageCode)
	if !ok {
		t.Fatalf("%s is missing from the Quant result association spec table", quantstage.RiskStageCode)
	}
	if spec.OutputType != quantstage.RiskOutputType || spec.SchemaVersion != quantstage.RiskSchemaVersion ||
		spec.PayloadSchemaRef != quantRiskPayloadSchemaRef ||
		spec.QualitySummarySchemaRef != quantRiskQualitySchemaRef ||
		spec.PayloadValidator == nil || spec.QualitySummaryValidator == nil || spec.ResultID == nil {
		t.Fatalf("Risk association spec is incomplete: %+v", spec)
	}
	raw, err := json.Marshal(validQuantRiskStageAdapterStub().output)
	if err != nil {
		t.Fatal(err)
	}
	resultID, err := spec.ResultID(raw)
	if err != nil {
		t.Fatalf("Risk association spec cannot extract result_id: %v", err)
	}
	if resultID != quantRiskTestResultID {
		t.Fatalf("Risk association result_id = %q, want %q", resultID, quantRiskTestResultID)
	}
}
