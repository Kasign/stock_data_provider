package stage

import (
	"encoding/json"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/risk"
)

func validRiskStageOutput() RiskOutput {
	snapshotRef := "data_snapshot:sha256:" + strings.Repeat("a", 64)
	authorizationRef := "data_authorization:opaque-token"
	orderBookRef := "data_snapshot:sha256:" + strings.Repeat("c", 64)
	orderBookAuthorizationRef := "data_authorization:order-book-opaque-token"
	refs := []string{snapshotRef, authorizationRef, orderBookRef, orderBookAuthorizationRef}
	return RiskOutput{
		SchemaVersion: RiskSchemaVersion,
		ResultID:      "quant_result:sha256:" + strings.Repeat("b", 64),
		ResultStatus:  "rejected",
		MissingFacts:  []string{"price_limit"},
		BlockingFacts: []string{},
		Decision: risk.Decision{
			DecisionID:     "risk_decision_1",
			Status:         risk.DecisionReject,
			RuleSetVersion: risk.TradabilityRuleSetVersion,
			EvaluatedAt:    "2026-11-25T15:00:00Z",
			SystemAsOf:     "2026-11-25T15:00:00Z",
			Reasons: []risk.Reason{{
				Code: risk.TradabilityGateCode, Severity: risk.SeverityBlock, Status: risk.ReasonTriggered,
				SafeMessage:  "可交易性事实不完整，缺失：涨跌停状态；无法确认可交易性",
				EvidenceRefs: append([]string{}, refs...),
			}},
			InputRefs: append([]string{}, refs...),
		},
	}
}

func TestRiskStageOutputAcceptsTraceableRejection(t *testing.T) {
	payload, err := json.Marshal(validRiskStageOutput())
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRiskStageOutputJSON(payload); err != nil {
		t.Fatalf("valid risk output rejected: %v", err)
	}
	// 输出必须能被解析回同一份决策，而不是只有校验通过。
	var decoded RiskOutput
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Status != risk.DecisionReject || decoded.Reasons[0].Code != risk.TradabilityGateCode ||
		len(decoded.MissingFacts) != 1 || decoded.MissingFacts[0] != "price_limit" ||
		decoded.ResultStatus != "rejected" {
		t.Fatalf("unexpected decoded output: %+v", decoded)
	}
}

// 篡改输出必须被拒绝：这些是最容易在适配层"顺手改一下"的字段。
func TestRiskStageOutputRejectsTampering(t *testing.T) {
	snapshotRef := "data_snapshot:sha256:" + strings.Repeat("a", 64)
	authorizationRef := "data_authorization:opaque-token"
	for name, mutate := range map[string]func(*RiskOutput){
		"result id":           func(output *RiskOutput) { output.ResultID = "quant_result:sha256:short" },
		"schema version":      func(output *RiskOutput) { output.SchemaVersion = "9.9.9" },
		"rule set version":    func(output *RiskOutput) { output.RuleSetVersion = "qnt-risk-tradability/9.9.9" },
		"result status":       func(output *RiskOutput) { output.ResultStatus = "completed" },
		"evaluated at":        func(output *RiskOutput) { output.EvaluatedAt = "2026-11-25T15:00:01Z" },
		"reason status":       func(output *RiskOutput) { output.Reasons[0].Status = risk.ReasonNotTriggered },
		"reason code":         func(output *RiskOutput) { output.Reasons[0].Code = "another_rule" },
		"reason count":        func(output *RiskOutput) { output.Reasons = append(output.Reasons, output.Reasons[0]) },
		"missing facts empty": func(output *RiskOutput) { output.MissingFacts = []string{} },
		"unknown missing fact": func(output *RiskOutput) {
			output.MissingFacts = append(output.MissingFacts, "halted")
		},
		"duplicate fact": func(output *RiskOutput) { output.MissingFacts = []string{"price_limit", "price_limit"} },
		"input refs order": func(output *RiskOutput) {
			output.InputRefs = []string{output.InputRefs[1], output.InputRefs[0], output.InputRefs[3], output.InputRefs[2]}
		},
		"order book evidence omitted": func(output *RiskOutput) {
			output.InputRefs = append([]string{}, output.InputRefs[:2]...)
			output.Reasons[0].EvidenceRefs = append([]string{}, output.InputRefs...)
		},
		"evidence refs diverged": func(output *RiskOutput) {
			output.Reasons[0].EvidenceRefs = []string{
				snapshotRef, authorizationRef, "data_snapshot:sha256:" + strings.Repeat("d", 64),
				output.InputRefs[3],
			}
		},
		"status": func(output *RiskOutput) { output.Status = risk.DecisionPass },
	} {
		t.Run(name, func(t *testing.T) {
			output := validRiskStageOutput()
			mutate(&output)
			payload, err := json.Marshal(output)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateRiskStageOutputJSON(payload); err == nil {
				t.Fatalf("tampered risk output was accepted: %s", payload)
			}
		})
	}
}

func TestRiskStageOutputRejectsUnknownFields(t *testing.T) {
	payload, err := json.Marshal(validRiskStageOutput())
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(payload, &object); err != nil {
		t.Fatal(err)
	}
	object["confidence"] = json.RawMessage("0.98")
	extended, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRiskStageOutputJSON(extended); err == nil {
		t.Fatal("risk output accepted an undeclared field")
	}
}

func TestRiskQualitySummaryContract(t *testing.T) {
	valid := RiskQualitySummary{
		SchemaVersion: RiskQualitySchemaVersion, RuleSetVersion: risk.TradabilityRuleSetVersion,
		QualityStatus: "valid", DecisionStatus: "reject", ResultStatus: "rejected",
		MissingFacts: []string{"price_limit"}, BlockingFacts: []string{},
	}
	payload, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateRiskQualitySummaryJSON(payload); err != nil {
		t.Fatalf("valid quality summary rejected: %v", err)
	}
	for name, mutate := range map[string]func(*RiskQualitySummary){
		"quality status":   func(summary *RiskQualitySummary) { summary.QualityStatus = "unknown" },
		"rule set version": func(summary *RiskQualitySummary) { summary.RuleSetVersion = "other/1.0.0" },
		"result status":    func(summary *RiskQualitySummary) { summary.ResultStatus = "completed" },
		"pass with missing facts": func(summary *RiskQualitySummary) {
			summary.DecisionStatus = "pass"
			summary.ResultStatus = "completed"
		},
		"reject without gaps": func(summary *RiskQualitySummary) { summary.MissingFacts = []string{} },
		"decision status":     func(summary *RiskQualitySummary) { summary.DecisionStatus = "maybe" },
	} {
		t.Run(name, func(t *testing.T) {
			summary := valid
			summary.MissingFacts = append([]string{}, valid.MissingFacts...)
			summary.BlockingFacts = append([]string{}, valid.BlockingFacts...)
			mutate(&summary)
			payload, err := json.Marshal(summary)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateRiskQualitySummaryJSON(payload); err == nil {
				t.Fatalf("tampered quality summary was accepted: %s", payload)
			}
		})
	}
}
