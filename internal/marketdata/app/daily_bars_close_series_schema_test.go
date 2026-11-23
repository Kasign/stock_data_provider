package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func TestDailyBarsCloseSeriesPayloadAndManifestSchemasMatchBuilder(t *testing.T) {
	input := closeSeriesSnapshotTestInput(t)
	snapshot, err := BuildDailyBarsCloseSeriesSnapshot(input, "principal:test-user", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() error = %v", err)
	}

	payloadSchema := compileDailyBarsCloseSeriesSchema(t, "DATA-CTR-007-daily-bars-close-series-payload-1.0.0.schema.json")
	payloadJSON, err := json.Marshal(snapshot.Payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDailyBarsCloseSeriesSchemaInstance(payloadSchema, payloadJSON); err != nil {
		t.Fatalf("Builder Payload violates DATA-CTR-007 Schema: %v; JSON: %s", err, payloadJSON)
	}

	manifestSchema := compileDailyBarsCloseSeriesSchema(t, "DATA-CTR-008-snapshot-manifest-2.0.0.schema.json")
	manifestJSON, err := json.Marshal(snapshot.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDailyBarsCloseSeriesSchemaInstance(manifestSchema, manifestJSON); err != nil {
		t.Fatalf("Builder Manifest violates DATA-CTR-008 2.0.0 Schema: %v; JSON: %s", err, manifestJSON)
	}
}

func TestDailyBarsCloseSeriesPayloadSchemaRejectsNonValidQuality(t *testing.T) {
	schema := compileDailyBarsCloseSeriesSchema(t, "DATA-CTR-007-daily-bars-close-series-payload-1.0.0.schema.json")
	input := closeSeriesSnapshotTestInput(t)
	snapshot, err := BuildDailyBarsCloseSeriesSnapshot(input, "principal:test-user", "quant_analysis")
	if err != nil {
		t.Fatal(err)
	}
	for _, status := range []QualityStatus{QualityDegraded, QualityStale, QualityInvalid, QualityUnknown} {
		t.Run(string(status), func(t *testing.T) {
			payload := cloneDailyBarsCloseSeriesPayload(snapshot.Payload)
			payload.QualityStatus = status
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDailyBarsCloseSeriesSchemaInstance(schema, encoded); err == nil {
				t.Fatalf("Payload Schema accepted aggregate quality %q", status)
			}

			payload = cloneDailyBarsCloseSeriesPayload(snapshot.Payload)
			payload.Bars[0].QualityStatus = status
			encoded, err = json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateDailyBarsCloseSeriesSchemaInstance(schema, encoded); err == nil {
				t.Fatalf("Payload Schema accepted bar quality %q", status)
			}
		})
	}
}

func compileDailyBarsCloseSeriesSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "schema", name)
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	compiler.LoadURL = func(uri string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external schema reference is disabled: %s", uri)
	}
	const schemaURL = "https://market-data.invalid/snapshot-schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		t.Fatalf("add schema %s: %v", name, err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	return schema
}

func validateDailyBarsCloseSeriesSchemaInstance(schema *jsonschema.Schema, encoded []byte) error {
	if schema == nil {
		return fmt.Errorf("compiled schema is required")
	}
	var instance any
	if err := json.Unmarshal(encoded, &instance); err != nil {
		return fmt.Errorf("decode schema instance: %w", err)
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("schema validation failed: %w", err)
	}
	return nil
}
