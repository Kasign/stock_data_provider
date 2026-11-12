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
	goldenPath := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "indicator-payload-1.0.0.golden.json")
	document, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Cases []struct {
			Name    string          `json:"name"`
			Payload json.RawMessage `json:"payload"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(document, &golden); err != nil {
		t.Fatal(err)
	}
	var validMACD json.RawMessage
	for _, item := range golden.Cases {
		if item.Name == "macd" {
			validMACD = item.Payload
			break
		}
	}
	if len(validMACD) == 0 {
		t.Fatal("MACD Golden fixture is missing")
	}

	mutations := []struct {
		name   string
		mutate func(map[string]json.RawMessage) error
	}{
		{
			name: "wrong metric mapping",
			mutate: func(payload map[string]json.RawMessage) error {
				var metrics []map[string]json.RawMessage
				if err := json.Unmarshal(payload["metrics"], &metrics); err != nil {
					return err
				}
				metrics[0]["metric_code"] = json.RawMessage("\"macd_dea\"")
				encoded, err := json.Marshal(metrics)
				payload["metrics"] = encoded
				return err
			},
		},
		{
			name: "wrong metric order",
			mutate: func(payload map[string]json.RawMessage) error {
				var metrics []json.RawMessage
				if err := json.Unmarshal(payload["metrics"], &metrics); err != nil {
					return err
				}
				metrics[0], metrics[1] = metrics[1], metrics[0]
				encoded, err := json.Marshal(metrics)
				payload["metrics"] = encoded
				return err
			},
		},
		{
			name: "too few metrics",
			mutate: func(payload map[string]json.RawMessage) error {
				var metrics []json.RawMessage
				if err := json.Unmarshal(payload["metrics"], &metrics); err != nil {
					return err
				}
				payload["metrics"], _ = json.Marshal(metrics[:2])
				return nil
			},
		},
		{
			name: "unknown indicator",
			mutate: func(payload map[string]json.RawMessage) error {
				payload["indicator_code"] = json.RawMessage("\"unknown\"")
				return nil
			},
		},
		{
			name: "unknown payload field",
			mutate: func(payload map[string]json.RawMessage) error {
				payload["extra"] = json.RawMessage("true")
				return nil
			},
		},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(validMACD, &payload); err != nil {
				t.Fatal(err)
			}
			if err := tc.mutate(payload); err != nil {
				t.Fatal(err)
			}
			instance, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateJSONSchemaInstance(schema, instance); err == nil {
				t.Fatalf("Indicator Payload Schema unexpectedly accepted: %s", instance)
			}
		})
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
