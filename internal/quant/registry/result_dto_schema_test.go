package registry

import (
	"encoding/json"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/factor"
	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/risk"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

// TestResultDTOJSONMatchesQNTCTR004Schemas 确保实际 Go DTO 的 JSON 输出
// 与 QNT-CTR-004 Schema 一致，而不只验证静态 Golden 文件。
func TestResultDTOJSONMatchesQNTCTR004Schemas(t *testing.T) {
	asOf := time.Date(2026, 11, 9, 1, 0, 0, 0, time.UTC)

	factorResult, err := factor.NewQuantMetric(
		factor.Result{
			Code: "momentum_20d", Value: 0.2, Unit: "ratio",
			Status: factor.StatusAvailable, SampleCount: 21,
		},
		asOf, factor.QualityValid, "1.0.0",
	)
	if err != nil {
		t.Fatalf("build Factor QuantMetric: %v", err)
	}

	strength := 0.35
	trendResult := trend.Result{
		TrendCode: "price_trend", Direction: trend.DirectionUnknown,
		Phase: trend.PhaseEmerging, Strength: &strength, Period: "20d",
		EvidenceMetrics: []metric.Metric{}, ConflictRefs: []string{},
		QualityStatus: trend.QualityValid, SystemAsOf: "2026-11-09T01:00:00Z",
		AlgorithmVersion: "1.0.0",
	}

	riskDecision := risk.Decision{
		DecisionID: "risk-decision-001", Status: risk.DecisionUnknown,
		RuleSetVersion: "1.0.0", EvaluatedAt: "2026-11-09T01:00:00Z",
		SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []risk.Reason{{
			Code: "INPUT_QUALITY_UNKNOWN", Severity: risk.SeverityBlock,
			Status: risk.ReasonUnknown, SafeMessage: "输入质量无法确认",
			EvidenceRefs: []string{"evidence:quality:001"},
		}},
		InputRefs: []string{"quant_result:001"},
	}

	tests := []struct {
		name       string
		schemaFile string
		value      any
		validate   func() error
	}{
		{
			name: "Factor QuantMetric", schemaFile: "QNT-CTR-004-factor-quant-metric-1.0.0.schema.json",
			value: factorResult,
		},
		{
			name: "TrendResult", schemaFile: "QNT-CTR-004-trend-result-1.0.0.schema.json",
			value: trendResult, validate: trendResult.Validate,
		},
		{
			name: "RiskDecision", schemaFile: "QNT-CTR-004-risk-decision-1.0.0.schema.json",
			value: riskDecision, validate: riskDecision.Validate,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.validate != nil {
				if err := tc.validate(); err != nil {
					t.Fatalf("DTO validation failed: %v", err)
				}
			}
			encoded, err := json.Marshal(tc.value)
			if err != nil {
				t.Fatalf("marshal DTO: %v", err)
			}
			schema := compileQuantSchemaFile(t, tc.schemaFile)
			if err := validateJSONSchemaInstance(schema, encoded); err != nil {
				t.Fatalf("serialized DTO violates QNT-CTR-004 Schema: %v; JSON: %s", err, encoded)
			}
		})
	}
}

func TestRiskDecisionSchemaRejectsEmptyReasonsAndAllowsEmptyInputRefs(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-risk-decision-1.0.0.schema.json")
	instance := []byte(`{
		"decision_id":"risk-decision-schema-001",
		"status":"unknown",
		"rule_set_version":"1.0.0",
		"evaluated_at":"2026-11-09T01:00:00Z",
		"system_as_of":"2026-11-09T00:59:00Z",
		"reasons":[{"code":"INPUT_QUALITY","severity":"block","status":"unknown","safe_message":"输入质量未知","evidence_refs":[]}],
		"input_refs":[]
	}`)
	if err := validateJSONSchemaInstance(schema, instance); err != nil {
		t.Fatalf("Schema rejected empty input_refs with a non-empty reasons array: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(instance, &decoded); err != nil {
		t.Fatal(err)
	}
	decoded["reasons"] = []any{}
	emptyReasons, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaInstance(schema, emptyReasons); err == nil {
		t.Fatal("Schema accepted RiskDecision with empty reasons")
	}
}
