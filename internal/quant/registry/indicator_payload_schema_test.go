package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/result"
)

func TestIndicatorResultPayloadSchemaAndGolden(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-indicator-result-payload-1.0.0.schema.json")
	goldenPath := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "indicator-payload-1.0.0.golden.json")
	document, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read Indicator Payload Golden: %v", err)
	}
	var golden struct {
		FixtureKind   string `json:"fixture_kind"`
		SchemaVersion string `json:"schema_version"`
		Cases         []struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(document, &golden); err != nil {
		t.Fatalf("decode Indicator Payload Golden: %v", err)
	}
	if golden.FixtureKind != "synthetic_structure_only" || golden.SchemaVersion != "1.0.0" || len(golden.Cases) != 8 {
		t.Fatalf("unexpected Indicator Payload Golden metadata: %+v", golden)
	}
	for _, tc := range golden.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			payload, err := result.DecodeIndicatorPayload(tc.Payload)
			if err != nil {
				t.Fatalf("Go Validator rejected Golden: %v", err)
			}
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateJSONSchemaInstance(schema, encoded); err != nil {
				t.Fatalf("Indicator Payload violates Schema: %v; JSON: %s", err, encoded)
			}
		})
	}
}

func TestIndicatorResultPayloadSchemaRejectsWrongMappingAndUnknownFields(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-indicator-result-payload-1.0.0.schema.json")
	cases := []string{
		"{\"indicator_code\":\"sma\",\"metrics\":[{\"metric_code\":\"ema\"}]}",
		"{\"indicator_code\":\"macd\",\"metrics\":[{\"metric_code\":\"macd_dea\"},{\"metric_code\":\"macd_dif\"},{\"metric_code\":\"macd_histogram\"}]}",
		"{\"indicator_code\":\"bollinger\",\"metrics\":[{\"metric_code\":\"bollinger_middle\"},{\"metric_code\":\"bollinger_upper\"}]}",
		"{\"indicator_code\":\"unknown\",\"metrics\":[]}",
		"{\"indicator_code\":\"sma\",\"metrics\":[{\"metric_code\":\"sma\"}],\"extra\":true}",
	}
	for _, instance := range cases {
		if err := validateJSONSchemaInstance(schema, []byte(instance)); err == nil {
			t.Errorf("Indicator Payload Schema unexpectedly accepted: %s", instance)
		}
	}
}

func TestIndicatorResultPayloadGoValidatorRejectsContextDrift(t *testing.T) {
	base := []string{"macd_dif", "macd_dea", "macd_histogram"}
	makeMetric := func(code string) map[string]json.RawMessage {
		return map[string]json.RawMessage{
			"metric_code":         json.RawMessage("\"" + code + "\""),
			"value":               json.RawMessage("null"),
			"unit":                json.RawMessage("\"fixture_unit\""),
			"period":              json.RawMessage("\"20d\""),
			"system_as_of":        json.RawMessage("\"2026-11-12T00:00:00Z\""),
			"quality_status":      json.RawMessage("\"unknown\""),
			"metric_status":       json.RawMessage("\"insufficient_sample\""),
			"calculation_version": json.RawMessage("\"fixture-v1\""),
			"lower_bound":         json.RawMessage("null"),
			"upper_bound":         json.RawMessage("null"),
		}
	}
	for _, field := range []string{"period", "system_as_of", "quality_status", "metric_status", "calculation_version"} {
		t.Run(field, func(t *testing.T) {
			items := []map[string]json.RawMessage{makeMetric(base[0]), makeMetric(base[1]), makeMetric(base[2])}
			items[2][field] = json.RawMessage("\"drift\"")
			raw, err := json.Marshal(map[string]any{"indicator_code": "macd", "metrics": items})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := result.DecodeIndicatorPayload(raw); err == nil {
				t.Fatalf("Go Validator accepted %s drift", field)
			}
		})
	}
	t.Run("sample_count", func(t *testing.T) {
		items := []map[string]json.RawMessage{makeMetric(base[0]), makeMetric(base[1]), makeMetric(base[2])}
		items[0]["sample_count"] = json.RawMessage("3")
		items[2]["sample_count"] = json.RawMessage("4")
		raw, err := json.Marshal(map[string]any{"indicator_code": "macd", "metrics": items})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := result.DecodeIndicatorPayload(raw); err == nil {
			t.Fatal("Go Validator accepted sample_count drift")
		}
	})
}
