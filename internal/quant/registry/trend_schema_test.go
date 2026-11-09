package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestTrendResultSchemaValidatesGoldenAndRejectsInvalidDiscriminators(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-trend-result-1.0.0.schema.json")
	path := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "trend-result-1.0.0.golden.json")
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read TrendResult Golden: %v", err)
	}
	if err := validateJSONSchemaInstance(schema, golden); err != nil {
		t.Fatalf("TrendResult Golden violates schema: %v", err)
	}

	invalid := []string{
		`{"trend_code":"price_trend","direction":"unknownish","phase":"emerging","strength":0.2,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`,
		`{"trend_code":"price_trend","direction":"bullish","phase":"sideways","strength":0.2,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`,
		`{"trend_code":"price_trend","direction":"unknown","phase":"emerging","strength":1.01,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`,
		`{"trend_code":"price_trend","direction":"unknown","phase":"emerging","strength":0.2,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00.000Z","algorithm_version":"1.0.0"}`,
	}
	for _, instance := range invalid {
		if err := validateJSONSchemaInstance(schema, []byte(instance)); err == nil {
			t.Errorf("invalid TrendResult unexpectedly passed: %s", instance)
		}
	}

	invalidMetrics := []string{
		`{"metric_code":"sample","value":null,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`,
		`{"metric_code":"sample","value":0.2,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"insufficient_sample","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`,
		`{"metric_code":"sample","value":0.2,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"unknown","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`,
		`{"metric_code":"sample","value":0.2,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":0.1,"upper_bound":null}`,
		`{"metric_code":"sample","value":"0.2","unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":0.1,"upper_bound":0.3}`,
	}
	for _, metric := range invalidMetrics {
		instance := `{"trend_code":"price_trend","direction":"unknown","phase":"emerging","strength":0.2,"period":"20d","evidence_metrics":[` + metric + `],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`
		if err := validateJSONSchemaInstance(schema, []byte(instance)); err == nil {
			t.Errorf("invalid evidence metric unexpectedly passed: %s", metric)
		}
	}
}


func TestTrendAndRiskSchemasRejectSurroundingWhitespace(t *testing.T) {
	trendSchema := compileQuantSchemaFile(t, "QNT-CTR-004-trend-result-1.0.0.schema.json")
	trendInstance := map[string]any{
		"trend_code": "price_trend",
		"direction":  "unknown",
		"phase":      "emerging",
		"strength":   0.2,
		"period":     "20d",
		"evidence_metrics": []any{
			map[string]any{
				"metric_code":         "momentum_20d",
				"value":               0.2,
				"unit":                "ratio",
				"period":              "20d",
				"system_as_of":        "2026-11-09T01:00:00Z",
				"quality_status":      "valid",
				"metric_status":       "available",
				"calculation_version": "1.0.0",
				"sample_count":        21,
				"lower_bound":         nil,
				"upper_bound":         nil,
			},
		},
		"conflict_refs":     []any{"conflict:1"},
		"quality_status":    "valid",
		"system_as_of":      "2026-11-09T01:00:00Z",
		"algorithm_version": "1.0.0",
	}
	trendMutations := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"trend_code leading whitespace", func(v map[string]any) { v["trend_code"] = " price_trend" }},
		{"trend_code trailing whitespace", func(v map[string]any) { v["trend_code"] = "price_trend " }},
		{"period leading whitespace", func(v map[string]any) { v["period"] = " 20d" }},
		{"period trailing whitespace", func(v map[string]any) { v["period"] = "20d " }},
		{"algorithm_version leading whitespace", func(v map[string]any) { v["algorithm_version"] = " 1.0.0" }},
		{"conflict ref trailing whitespace", func(v map[string]any) { v["conflict_refs"] = []any{"conflict:1 "} }},
		{"metric code leading whitespace", func(v map[string]any) { v["evidence_metrics"].([]any)[0].(map[string]any)["metric_code"] = " momentum_20d" }},
		{"metric unit trailing whitespace", func(v map[string]any) { v["evidence_metrics"].([]any)[0].(map[string]any)["unit"] = "ratio " }},
		{"metric period leading whitespace", func(v map[string]any) { v["evidence_metrics"].([]any)[0].(map[string]any)["period"] = " 20d" }},
		{"metric calculation version trailing whitespace", func(v map[string]any) { v["evidence_metrics"].([]any)[0].(map[string]any)["calculation_version"] = "1.0.0 " }},
	}
	for _, tc := range trendMutations {
		t.Run("trend/"+tc.name, func(t *testing.T) {
			instance := cloneSchemaMap(trendInstance)
			tc.mutate(instance)
			if err := trendSchema.Validate(instance); err == nil {
				t.Fatal("TrendResult Schema unexpectedly accepted non-canonical text")
			}
		})
	}

	riskSchema := compileQuantSchemaFile(t, "QNT-CTR-004-risk-decision-1.0.0.schema.json")
	riskInstance := map[string]any{
		"decision_id":      "decision-1",
		"status":           "unknown",
		"rule_set_version": "1.0.0",
		"evaluated_at":     "2026-11-09T01:00:00Z",
		"system_as_of":     "2026-11-09T00:59:00Z",
		"reasons": []any{
			map[string]any{
				"code":          "INPUT_UNKNOWN",
				"severity":      "block",
				"status":        "unknown",
				"safe_message": "input unavailable",
				"evidence_refs": []any{"evidence:1"},
			},
		},
		"input_refs": []any{"input:1"},
	}
	riskMutations := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"decision_id leading whitespace", func(v map[string]any) { v["decision_id"] = " decision-1" }},
		{"rule_set_version trailing whitespace", func(v map[string]any) { v["rule_set_version"] = "1.0.0 " }},
		{"reason code leading whitespace", func(v map[string]any) { v["reasons"].([]any)[0].(map[string]any)["code"] = " INPUT_UNKNOWN" }},
		{"safe message trailing whitespace", func(v map[string]any) { v["reasons"].([]any)[0].(map[string]any)["safe_message"] = "input unavailable " }},
		{"evidence ref leading whitespace", func(v map[string]any) { v["reasons"].([]any)[0].(map[string]any)["evidence_refs"] = []any{" evidence:1"} }},
		{"input ref trailing whitespace", func(v map[string]any) { v["input_refs"] = []any{"input:1 "} }},
	}
	for _, tc := range riskMutations {
		t.Run("risk/"+tc.name, func(t *testing.T) {
			instance := cloneSchemaMap(riskInstance)
			tc.mutate(instance)
			if err := riskSchema.Validate(instance); err == nil {
				t.Fatal("RiskDecision Schema unexpectedly accepted non-canonical text")
			}
		})
	}
}

func cloneSchemaMap(source map[string]any) map[string]any {
	encoded, err := json.Marshal(source)
	if err != nil {
		panic(err)
	}
	var clone map[string]any
	if err := json.Unmarshal(encoded, &clone); err != nil {
		panic(err)
	}
	return clone
}
