package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
)

// loadJSONObject reads a JSON document from disk and decodes it into a generic
// value so the test can compare constraint keyword subtrees structurally.
func loadJSONObject(t *testing.T, path string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	return value
}

func TestQuantResearchParameterSchemaRegistryGoldenAndRejections(t *testing.T) {
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("../../../doc/researchflow/golden/quant-research-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := validator.Validate(context.Background(), quantResearchParameterSchemaRef, golden)
	if err != nil {
		t.Fatalf("quant.research golden must satisfy its Schema: %v", err)
	}
	if snapshot.Ref != quantResearchParameterSchemaRef || snapshot.Version != quantResearchParameterSchemaVersion || len(snapshot.Fingerprint) != 64 {
		t.Fatalf("unexpected quant.research schema snapshot: %+v", snapshot)
	}

	invalid := []struct {
		name string
		body string
	}{
		{"root not object", `[]`},
		{"missing stages", `{}`},
		{"extra root key", `{"stages":{},"symbol":"600036.SH"}`},
		{"missing a stage", `{"stages":{"quant.sma20":{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}}}`},
		{"unknown stage", `{"stages":{"quant.sma20":{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20},"quant.factor":{"factor_code":"momentum_5d","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":6},"quant.trend":{"symbol":"600036.SH","start_date":"2026-09-01","end_date":"2026-10-30","limit":61},"quant.risk":{"symbol":"600036.SH","trade_date":"2026-11-25"},"ghost":{}}}`},
		{"sma20 limit below floor", `{"stages":{"quant.sma20":{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":19},"quant.factor":{"factor_code":"momentum_5d","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":6},"quant.trend":{"symbol":"600036.SH","start_date":"2026-09-01","end_date":"2026-10-30","limit":61},"quant.risk":{"symbol":"600036.SH","trade_date":"2026-11-25"}}}`},
		{"risk unknown field", `{"stages":{"quant.sma20":{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20},"quant.factor":{"factor_code":"momentum_5d","symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":6},"quant.trend":{"symbol":"600036.SH","start_date":"2026-09-01","end_date":"2026-10-30","limit":61},"quant.risk":{"symbol":"600036.SH","trade_date":"2026-11-25","start_date":"2026-11-25"}}}`},
	}
	for _, tc := range invalid {
		if _, err := validator.Validate(context.Background(), quantResearchParameterSchemaRef, json.RawMessage(tc.body)); err == nil {
			t.Errorf("%s: invalid parameters were accepted", tc.name)
		}
	}
}

func TestQuantResearchParameterSchemaDocMatchesRegistry(t *testing.T) {
	documented, err := os.ReadFile("../../../doc/researchflow/schema/quant-research-parameters-1.0.0.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(documented) != quantResearchParameterSchemaDocument+"\n" {
		t.Fatal("documented quant.research parameter Schema drifted from Server registry")
	}
}

// TestQuantResearchDefsMatchLeafSchemas enforces B2: the four inlined $defs must
// stay structurally identical to the four frozen leaf parameter schemas. JSON
// Schema forbids external $ref in this validator, so these copies are the only
// link; a one-sided edit to a leaf limit must turn this test red.
func TestQuantResearchDefsMatchLeafSchemas(t *testing.T) {
	composite := loadJSONObject(t, "../../../doc/researchflow/schema/quant-research-parameters-1.0.0.schema.json")
	defs, ok := composite["$defs"].(map[string]any)
	if !ok {
		t.Fatal("quant.research schema is missing $defs")
	}
	leaves := []struct {
		defName string
		path    string
	}{
		{"quantSMA20", "../../../doc/researchflow/schema/quant-sma20-parameters-1.0.0.schema.json"},
		{"quantFactor", "../../../doc/researchflow/schema/quant-factor-parameters-1.0.0.schema.json"},
		{"quantTrend", "../../../doc/researchflow/schema/quant-trend-parameters-1.0.0.schema.json"},
		{"quantRisk", "../../../doc/researchflow/schema/quant-risk-parameters-1.0.0.schema.json"},
	}
	constraintKeys := []string{"type", "properties", "required", "additionalProperties"}
	for _, leaf := range leaves {
		def, ok := defs[leaf.defName].(map[string]any)
		if !ok {
			t.Fatalf("missing $defs.%s", leaf.defName)
		}
		schema := loadJSONObject(t, leaf.path)
		for _, key := range constraintKeys {
			want, wantOK := schema[key]
			got, gotOK := def[key]
			if !wantOK || !gotOK {
				t.Fatalf("%s: constraint key %q presence drifted (leaf=%v def=%v)", leaf.defName, key, wantOK, gotOK)
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("%s: constraint %q drifted from leaf schema\ndef: %v\nleaf: %v", leaf.defName, key, got, want)
			}
		}
	}
	if len(defs) != len(leaves) {
		t.Fatalf("$defs count = %d, want %d", len(defs), len(leaves))
	}
}

func TestQuantResearchSchemaNotFoundIsDistinguishable(t *testing.T) {
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), "unknown/quant-research", json.RawMessage(`{"stages":{}}`)); !errors.Is(err, parameterschema.ErrSchemaNotFound) {
		t.Fatalf("unknown quant.research schema ref must be distinguishable, got %v", err)
	}
}
