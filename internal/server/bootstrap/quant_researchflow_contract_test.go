package bootstrap

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	marketdatabootstrap "example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/resourcepolicy"
	"example.com/stock-analysis-flow/internal/researchflow/stageoutput"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// Quant → Researchflow 消费方契约测试。
//
// 与 Quant 模块内的 DTO 测试不同，这里站在 Researchflow 与 Server 组合根的角度，
// 断言四件事必须同时成立，任何一侧漂移都失败：
//  1. 生产 Output Registry 的 Entry 与 Quant 结果关联规格表指向同一份 Schema/Validator；
//  2. Researchflow 的 Registry.Validate 接受 Quant Stage 真实产出的 Envelope；
//  3. 文档化 JSON Schema 与运行时 Validator 对同一 payload 的判定一致；
//  4. 模板声明的 degraded 传播语义与 Stage Executor 的状态映射一致。
//
// 只使用 Server 组合根与 Researchflow 的生产构造函数，不复制 Entry 或 Schema 约束。

// Data Snapshot Port 必须由真实 Data Module 满足；本断言防止组合根把 Quant 侧
// 自己的接口写成只有测试替身才实现的形状。
var _ quantDataSnapshotPort = (*marketdatabootstrap.Module)(nil)

func quantContractFactorRegistry(t *testing.T) *stageoutput.Registry {
	t.Helper()
	registry, err := newQuantFactorOutputRegistry()
	if err != nil {
		t.Fatalf("create Quant Factor Output Registry: %v", err)
	}
	return registry
}

func quantContractSMA20Registry(t *testing.T) *stageoutput.Registry {
	t.Helper()
	registry, err := newQuantSMA20OutputRegistry()
	if err != nil {
		t.Fatalf("create Quant SMA20 Output Registry: %v", err)
	}
	return registry
}

// quantContractFixtures 返回关联规格表与 Output Registry 都应接受的真实契约样本。
func quantContractFixtures(t *testing.T) map[string]struct {
	payload  []byte
	quality  []byte
	registry *stageoutput.Registry
} {
	t.Helper()
	return map[string]struct {
		payload  []byte
		quality  []byte
		registry *stageoutput.Registry
	}{
		quantstage.SMA20StageCode: {
			payload: []byte(quantAssociationSMA20Payload), quality: []byte(quantAssociationSMA20Quality),
			registry: quantContractSMA20Registry(t),
		},
		quantstage.FactorStageCode: {
			payload: quantFactorAssociationPayload(t), quality: mustQuantFactorAssociationQuality(t),
			registry: quantContractFactorRegistry(t),
		},
	}
}

func TestQuantResearchflowContractOutputRegistryMatchesAssociationSpec(t *testing.T) {
	fixtures := quantContractFixtures(t)
	for _, spec := range quantStageAssociationSpecs {
		fixture, ok := fixtures[spec.StageCode]
		if !ok {
			t.Fatalf("Quant Stage %q has no contract fixture", spec.StageCode)
		}
		entry, present := fixture.registry.Entry(spec.StageCode, spec.OutputType, spec.SchemaVersion)
		if !present {
			t.Fatalf("Researchflow Output Registry has no entry for %s/%s/%s registered by the association spec",
				spec.StageCode, spec.OutputType, spec.SchemaVersion)
		}
		if entry.PayloadSchemaRef != spec.PayloadSchemaRef ||
			entry.QualitySummarySchemaRef != spec.QualitySummarySchemaRef ||
			entry.PayloadValidator == nil || entry.QualitySummaryValidator == nil {
			t.Fatalf("Output Registry entry and association spec for %q disagree: entry=%+v spec=%+v", spec.StageCode, entry, spec)
		}
		if err := entry.PayloadValidator.ValidateJSON(fixture.payload); err != nil {
			t.Fatalf("Output Registry payload validator for %q rejected the contract fixture: %v", spec.StageCode, err)
		}
		if err := entry.QualitySummaryValidator.ValidateJSON(fixture.quality); err != nil {
			t.Fatalf("Output Registry quality validator for %q rejected the contract fixture: %v", spec.StageCode, err)
		}
	}
}

func quantContractFactorIdentity() stageoutput.ExecutionIdentity {
	return stageoutput.ExecutionIdentity{
		RunRef: "run:researchflow:contract-001", StageRef: "stage:quant.factor", AttemptRef: "attempt:001",
		StageCode: quantstage.FactorStageCode, TemplateVersion: quantFactorWorkflowTemplateVersion,
		RegistryVersion: quantFactorStageOutputRegistryVersion,
	}
}

func TestQuantResearchflowContractRegistryAcceptsRealFactorStageEnvelope(t *testing.T) {
	adapter := validQuantFactorStageAdapterStub()
	result, err := (quantFactorStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.FactorStageCode},
			Parameters:     validQuantFactorStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil || result.ProviderResult == nil {
		t.Fatalf("Quant Factor Stage Executor did not produce a ProviderResult: %+v (err=%v)", result, err)
	}
	identity := quantContractFactorIdentity()
	completedAt := time.Date(2026, 11, 20, 8, 1, 0, 0, time.UTC)
	envelope, err := stageoutput.BuildEnvelope(identity, *result.ProviderResult, completedAt)
	if err != nil {
		t.Fatalf("build Stage Output envelope from the Quant ProviderResult: %v", err)
	}
	registry := quantContractFactorRegistry(t)
	if err := registry.Validate(envelope, stageoutput.ValidationContext{Identity: identity}); err != nil {
		t.Fatalf("Researchflow Output Registry rejected the Quant Factor Stage output: %v", err)
	}

	// 文档化 Schema 必须与运行时 Validator 判定一致：同一 payload 被两者同时接受。
	payloadSchema := compileQuantContractSchema(t, quantFactorPayloadSchemaRef)
	qualitySchema := compileQuantContractSchema(t, quantFactorQualitySchemaRef)
	for name, testCase := range map[string]struct {
		schema  *jsonschema.Schema
		payload []byte
	}{
		"payload":         {payloadSchema, envelope.Payload},
		"quality_summary": {qualitySchema, envelope.QualitySummary},
	} {
		decoder := json.NewDecoder(bytes.NewReader(testCase.payload))
		decoder.UseNumber()
		var instance any
		if err := decoder.Decode(&instance); err != nil {
			t.Fatalf("decode %s instance: %v", name, err)
		}
		if err := testCase.schema.Validate(instance); err != nil {
			t.Fatalf("documented %s Schema rejected the runtime-valid output: %v", name, err)
		}
	}

	// 值类型漂移必须被契约两侧同时拒绝，否则文档与运行时存在双重标准。
	var decoded map[string]any
	if err := json.Unmarshal(envelope.Payload, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["value"] = 0.904761904762
	drifted, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	driftedEnvelope := envelope
	driftedEnvelope.Payload = drifted
	if err := registry.Validate(driftedEnvelope, stageoutput.ValidationContext{Identity: identity}); err == nil {
		t.Fatal("Researchflow Output Registry accepted a numeric Factor value that the contract requires as decimal string")
	}
	decoder := json.NewDecoder(bytes.NewReader(drifted))
	decoder.UseNumber()
	var driftedInstance any
	if err := decoder.Decode(&driftedInstance); err != nil {
		t.Fatal(err)
	}
	if err := payloadSchema.Validate(driftedInstance); err == nil {
		t.Fatal("documented Factor payload Schema accepted a numeric value")
	}
}

func TestQuantResearchflowContractDegradedFactorStaysAcceptedAndPropagates(t *testing.T) {
	registry := quantContractFactorRegistry(t)
	schemas, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	policies, err := resourcepolicy.NewRegistry()
	if err != nil {
		t.Fatal(err)
	}
	workflow, err := buildQuantFactorWorkflowTemplate(&quantFactorStageComposition{
		registry: registry, parameterSchemas: schemas,
		applicability: quantFactorApplicabilityEvaluator{},
		providerCodes: []string{quantstage.QuantProviderCode},
	}, policies)
	if err != nil {
		t.Fatalf("build frozen Quant Factor workflow template: %v", err)
	}
	if len(workflow.Stages) != 1 || workflow.Stages[0].Propagation.OnDegraded != researchflowdomain.PropagateAccept {
		t.Fatalf("frozen Quant Factor workflow template no longer accepts degraded Stage results: %+v", workflow.Stages)
	}

	adapter := validQuantFactorStageAdapterStub()
	adapter.output.MetricStatus = "insufficient_data"
	adapter.output.ResultStatus = "insufficient_data"
	adapter.output.Value = json.RawMessage("null")
	adapter.output.SampleCount = 6
	adapter.quality.ObservedBars = 6
	adapter.quality.ExpectedTradingDays = 6
	adapter.quality.MinimumRequiredBars = 4
	result, err := (quantFactorStageExecutor{adapter: adapter}).ExecuteStage(
		context.Background(), researchflowapplication.StageExecutionRequest{
			Stage:          researchflowdomain.Stage{Code: quantstage.FactorStageCode},
			Parameters:     validQuantFactorStageParameters(),
			SystemAsOf:     time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC),
			ResourcePolicy: validQuantFactorStagePolicy(t),
		},
	)
	if err != nil || result.ProviderResult == nil {
		t.Fatalf("degraded Factor Stage did not produce a ProviderResult: %+v (err=%v)", result, err)
	}
	if result.AttemptStatus != researchflowdomain.AttemptDegraded ||
		result.StageStatus != researchflowdomain.StageDegraded ||
		len(result.ReasonCodes) != 1 || result.ReasonCodes[0] != "quant_factor_insufficient_data" {
		t.Fatalf("degraded Factor Stage lost its status or reason code: %+v", result)
	}
	identity := quantContractFactorIdentity()
	envelope, err := stageoutput.BuildEnvelope(identity, *result.ProviderResult, time.Date(2026, 11, 20, 8, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("build degraded Stage Output envelope: %v", err)
	}
	if err := registry.Validate(envelope, stageoutput.ValidationContext{Identity: identity}); err != nil {
		t.Fatalf("Researchflow Output Registry rejected a degraded-but-valid Factor output: %v", err)
	}
}

// compileQuantContractSchema 直接编译文档中登记的 Schema 文件。
// Schema 引用是仓库相对路径，测试从 internal/server/bootstrap 出发回到仓库根。
func compileQuantContractSchema(t *testing.T, ref string) *jsonschema.Schema {
	t.Helper()
	document, err := os.ReadFile(filepath.Join("..", "..", "..", ref))
	if err != nil {
		t.Fatalf("read documented schema %s: %v", ref, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	schemaURL := "https://quant.invalid/" + filepath.Base(ref)
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		t.Fatalf("add documented schema %s: %v", ref, err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatalf("compile documented schema %s: %v", ref, err)
	}
	return compiled
}
