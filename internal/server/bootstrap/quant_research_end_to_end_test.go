package bootstrap

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/risk"
	quantstage "example.com/stock-analysis-flow/internal/quant/stage"
	researchflowapplication "example.com/stock-analysis-flow/internal/researchflow/application"
	researchflowdomain "example.com/stock-analysis-flow/internal/researchflow/domain"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

// 阶段 2.1 端到端验收：D1 参数信封与四份冻结参数契约闭环，且 Stage 在真实
// Data/Quant 代码上执行后，result_id 可作为同一 Run 的授权读取关联锚点。
// 只替换持久化实现（沿用既有 e2e 的内存 Store 模式）；Data manifest/payload
// 指纹、授权复核、Quant 规则与 result 内容寻址全部由生产代码执行。

// TestQuantResearchGoldenSplitsIntoFourValidLeafParameters 证明信封 → 子树
// 的分流结果与四份既有单 Stage 参数 Schema 完全一致：Worker 交付给每个
// Stage 的子树既能通过合并 Schema（G2），也能通过该 Stage 自己的冻结 Schema。
func TestQuantResearchGoldenSplitsIntoFourValidLeafParameters(t *testing.T) {
	golden, err := os.ReadFile("../../../doc/researchflow/golden/quant-research-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	split, err := template.SplitPerStageParameters(json.RawMessage(golden))
	if err != nil {
		t.Fatalf("golden envelope is not a valid per-Stage document: %v", err)
	}
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	refs := map[string]string{
		quantstage.SMA20StageCode:  quantSMA20ParameterSchemaRef,
		quantstage.FactorStageCode: quantFactorParameterSchemaRef,
		quantstage.TrendStageCode:  quantTrendParameterSchemaRef,
		quantstage.RiskStageCode:   quantRiskParameterSchemaRef,
	}
	if len(split) != len(refs) {
		t.Fatalf("split stage count = %d, want %d", len(split), len(refs))
	}
	for code, ref := range refs {
		subtree, ok := split[code]
		if !ok {
			t.Fatalf("golden envelope is missing stage %s", code)
		}
		if _, err := validator.Validate(context.Background(), ref, subtree); err != nil {
			t.Errorf("stage %s subtree %s does not satisfy its frozen parameter schema: %v", code, subtree, err)
		}
	}
}

// TestQuantResearchEndToEndFactorAndRiskResultsAssociateToSameRun 用两个真实
// e2e chain（Data 授权快照 → Quant Facade → 落库）执行 factor 与 risk Stage，
// 验证：两者都产出可内容寻址的 result；risk 为可追溯 reject；两份 result_id
// 都能作为同一 Run 的 Stage Output 关联锚点被授权读取验证（测试点 10/11）。
func TestQuantResearchEndToEndFactorAndRiskResultsAssociateToSameRun(t *testing.T) {
	factorChain := newQuantFactorEndToEndChain(t, 20)
	factorOutput, factorQuality, err := factorChain.adapter.Execute(factorChain.context, factorChain.request)
	if err != nil {
		t.Fatalf("Factor Stage over real data failed: %v", err)
	}
	factorPayload, err := json.Marshal(factorOutput)
	if err != nil {
		t.Fatal(err)
	}
	factorQualityPayload, err := json.Marshal(factorQuality)
	if err != nil {
		t.Fatal(err)
	}

	riskChain := newQuantTradingFactsChain(t, newQuantTradingFactsStore())
	riskAdapter, _ := newQuantRiskEndToEndAdapter(t, riskChain)
	riskOutput, riskQuality, err := riskAdapter.Execute(riskChain.context, quantstage.RiskRequest{
		Symbol: tradingFactsSymbol, TradeDate: tradingFactsDate, SystemAsOf: riskChain.systemAsOf,
	})
	if err != nil {
		t.Fatalf("Risk Stage over real facts failed: %v", err)
	}
	if riskOutput.Status != risk.DecisionPass {
		t.Fatalf("risk decision = %q, want pass with all three facts verified", riskOutput.Status)
	}
	riskPayload, err := json.Marshal(riskOutput)
	if err != nil {
		t.Fatal(err)
	}
	riskQualityPayload, err := json.Marshal(riskQuality)
	if err != nil {
		t.Fatal(err)
	}

	// 同一 Run 下两个 succeeded Stage 各自携带 Stage Output 与 Quality ref。
	runStages := []researchflowdomain.Stage{
		quantAssociationStageStage(quantAssociationTestRunRef, quantstage.FactorStageCode, researchflowdomain.StageSucceeded),
		quantAssociationStageStage(quantAssociationTestRunRef, quantstage.RiskStageCode, researchflowdomain.StageSucceeded),
	}
	runs := &quantAssociationRunReaderStub{result: quantAssociationRun(quantAssociationTestRunRef, runStages...)}
	outputs := &quantAssociationOutputReaderStub{resources: map[string]researchflowapplication.StageOutputResource{
		quantstage.FactorStageCode: {Content: factorPayload},
		quantstage.RiskStageCode:   {Content: riskPayload},
	}}
	verifier := quantResultAssociationVerifier{runs: runs, outputs: outputs}

	for _, tc := range []struct {
		stageCode string
		resultID  string
		payload   json.RawMessage
		quality   json.RawMessage
	}{
		{quantstage.FactorStageCode, factorOutput.ResultID, factorPayload, factorQualityPayload},
		{quantstage.RiskStageCode, riskOutput.ResultID, riskPayload, riskQualityPayload},
	} {
		spec, supported := quantStageAssociationSpecFor(tc.stageCode)
		if !supported {
			t.Fatalf("stage %s missing from association spec table", tc.stageCode)
		}
		extracted, err := spec.ResultID(tc.payload)
		if err != nil || extracted != tc.resultID {
			t.Errorf("stage %s: association result_id = %q (err=%v), want %q", tc.stageCode, extracted, err, tc.resultID)
		}
		if err := spec.PayloadValidator(tc.payload); err != nil {
			t.Errorf("stage %s: association spec rejected payload: %v", tc.stageCode, err)
		}
		if err := spec.QualitySummaryValidator(tc.quality); err != nil {
			t.Errorf("stage %s: association spec rejected quality summary: %v", tc.stageCode, err)
		}
		if err := verifier.VerifyResultAssociation(context.Background(), quantAssociationTestRunRef, tc.resultID); err != nil {
			t.Errorf("stage %s: result_id %s not readable as Run association: %v", tc.stageCode, tc.resultID, err)
		}
	}

	// risk 的结论必须由真实 Quality Summary 承载；三类事实齐备后应为 pass。
	if riskQuality.DecisionStatus != "pass" || riskQuality.ResultStatus != "completed" {
		t.Fatalf("unexpected risk quality summary: %+v", riskQuality)
	}
}
