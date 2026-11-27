package registry

import (
	"os"
	"path/filepath"
	"testing"
)

// TestRiskDecisionV2GoldenMatchesSchema 证明 2.0.0 Golden 样本与文档 Schema 一致。
func TestRiskDecisionV2GoldenMatchesSchema(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-risk-decision-2.0.0.schema.json")
	goldenPath := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "risk-decision-2.0.0.golden.json")
	golden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read 2.0.0 Golden: %v", err)
	}
	if err := validateJSONSchemaInstance(schema, golden); err != nil {
		t.Fatalf("2.0.0 Golden violates schema: %v", err)
	}
}

// TestRiskDecisionV2SchemaAcceptsExplanationFields 证明 2.0.0 文档 Schema
// 接纳权威 Decision 的四个解释字段。
func TestRiskDecisionV2SchemaAcceptsExplanationFields(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-risk-decision-2.0.0.schema.json")
	valid := `{"decision_id":"risk-v2","status":"reject","rule_set_version":"v2","evaluated_at":"2026-11-09T01:00:00Z","system_as_of":"2026-11-09T00:59:00Z","reasons":[{"code":"HARD_GATE","severity":"block","status":"triggered","safe_message":"blocked","evidence_refs":[],"rule_ref":"rule:v2","observed_value":12.5,"expected_range":"0-10","remediation_code":"REDUCE"}],"input_refs":[]}`
	if err := validateJSONSchemaInstance(schema, []byte(valid)); err != nil {
		t.Fatalf("2.0.0 schema rejected valid explanation fields: %v", err)
	}
}

// TestRiskDecisionV1SchemaRejectsExplanationFields 证明冻结的 1.0.0 Schema
// 拒绝携带解释字段的实例，防止旧版本被静默扩写。
func TestRiskDecisionV1SchemaRejectsExplanationFields(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-risk-decision-1.0.0.schema.json")
	withExplanation := `{"decision_id":"risk-v2","status":"reject","rule_set_version":"v2","evaluated_at":"2026-11-09T01:00:00Z","system_as_of":"2026-11-09T00:59:00Z","reasons":[{"code":"HARD_GATE","severity":"block","status":"triggered","safe_message":"blocked","evidence_refs":[],"rule_ref":"rule:v2"}],"input_refs":[]}`
	if err := validateJSONSchemaInstance(schema, []byte(withExplanation)); err == nil {
		t.Fatal("frozen 1.0.0 schema must reject explanation fields")
	}
}

// TestRiskDecisionV2SchemaRejectsNonCanonicalExplanation 证明 2.0.0 Schema
// 对解释字段执行与 Go 侧一致的 canonical 约束。
func TestRiskDecisionV2SchemaRejectsNonCanonicalExplanation(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-risk-decision-2.0.0.schema.json")
	invalids := []string{
		`{"decision_id":"d","status":"reject","rule_set_version":"v2","evaluated_at":"2026-11-09T01:00:00Z","system_as_of":"2026-11-09T00:59:00Z","reasons":[{"code":"R","severity":"block","status":"triggered","safe_message":"x","evidence_refs":[],"rule_ref":" "}],"input_refs":[]}`,
		`{"decision_id":"d","status":"reject","rule_set_version":"v2","evaluated_at":"2026-11-09T01:00:00Z","system_as_of":"2026-11-09T00:59:00Z","reasons":[{"code":"R","severity":"block","status":"triggered","safe_message":"x","evidence_refs":[],"expected_range":" "}],"input_refs":[]}`,
		`{"decision_id":"d","status":"reject","rule_set_version":"v2","evaluated_at":"2026-11-09T01:00:00Z","system_as_of":"2026-11-09T00:59:00Z","reasons":[{"code":"R","severity":"block","status":"triggered","safe_message":"x","evidence_refs":[],"observed_value":{"nested":1}}],"input_refs":[]}`,
	}
	for _, instance := range invalids {
		if err := validateJSONSchemaInstance(schema, []byte(instance)); err == nil {
			t.Errorf("2.0.0 schema must reject non-canonical explanation: %s", instance)
		}
	}
}
