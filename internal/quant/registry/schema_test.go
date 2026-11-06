package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readQuantSchema(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "quant", "schema", name)
	document, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema %s: %v", path, err)
	}
	return document
}

func TestDraft202012SchemasCompileAndValidateInstances(t *testing.T) {
	cases := []struct {
		name     string
		instance string
		invalid  string
	}{
		{
			name:     "QNT-CTR-001-registry-entry-1.0.0.schema.json",
			instance: mustJSON(t, validEntry("momentum_20d", "1.0.0")),
			invalid:  `{"code":"momentum_20d","schema_version":"1.0.0","unexpected":true}`,
		},
		{
			name:     "QNT-CTR-002-indicator-parameters-1.0.0.schema.json",
			instance: `{"schema_version":"1.0.0","indicator_code":"sma","parameters":{"window":5}}`,
			invalid:  `{"schema_version":"1.0.0","indicator_code":"sma","parameters":{"window":0}}`,
		},
		{
			name:     "QNT-CTR-003-factor-parameters-1.0.0.schema.json",
			instance: `{"schema_version":"1.0.0","factor_code":"momentum_20d","parameters":{}}`,
			invalid:  `{"schema_version":"1.0.0","factor_code":"momentum_20d","parameters":{"window":20}}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compiled, err := compileJSONSchema(readQuantSchema(t, tc.name))
			if err != nil {
				t.Fatalf("compileJSONSchema() error = %v", err)
			}
			if err := validateJSONSchemaInstance(compiled, []byte(tc.instance)); err != nil {
				t.Fatalf("valid instance rejected: %v", err)
			}
			if err := validateJSONSchemaInstance(compiled, []byte(tc.invalid)); err == nil {
				t.Fatalf("invalid instance unexpectedly passed: %s", tc.invalid)
			}
		})
	}
}

func TestCompileJSONSchemaRejectsUnsupportedDraftAndExternalReferences(t *testing.T) {
	cases := []string{
		`{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","additionalProperties":false}`,
		`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"properties":{"value":{"$ref":"https://example.com/value.json"}}}`,
		`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`,
	}
	for _, document := range cases {
		if _, err := compileJSONSchema([]byte(document)); err == nil {
			t.Errorf("expected schema rejection: %s", document)
		}
	}
}

func TestEntryValidateRejectsInvalidParameterSchema(t *testing.T) {
	entry := validEntry("momentum_20d", "1.0.0")
	entry.ParametersSchema = json.RawMessage(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`)
	if err := entry.Validate(); err == nil {
		t.Fatal("expected open-root parameter schema to be rejected")
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}
	return string(encoded)
}
