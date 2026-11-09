package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"
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


func TestAllIndicatorAndFactorGoldenParametersMatchContractSchemas(t *testing.T) {
	contracts := []struct {
		name       string
		directory  string
		schemaFile string
		codeField  string
	}{
		{
			name:       "indicator",
			directory:  filepath.Join("..", "..", "..", "doc", "quant", "golden", "indicator"),
			schemaFile: "QNT-CTR-002-indicator-parameters-1.0.0.schema.json",
			codeField:  "indicator_code",
		},
		{
			name:       "factor",
			directory:  filepath.Join("..", "..", "..", "doc", "quant", "golden", "factor"),
			schemaFile: "QNT-CTR-003-factor-parameters-1.0.0.schema.json",
			codeField:  "factor_code",
		},
	}

	for _, contract := range contracts {
		t.Run(contract.name, func(t *testing.T) {
			schema := compileQuantSchemaFile(t, contract.schemaFile)
			files, err := filepath.Glob(filepath.Join(contract.directory, "*.golden.json"))
			if err != nil {
				t.Fatalf("list Golden fixtures: %v", err)
			}
			if len(files) == 0 {
				t.Fatal("no Golden fixtures found")
			}

			for _, path := range files {
				t.Run(filepath.Base(path), func(t *testing.T) {
					content, err := os.ReadFile(path)
					if err != nil {
						t.Fatalf("read Golden fixture: %v", err)
					}
					var fixture struct {
						SchemaVersion string          `json:"schema_version"`
						IndicatorCode string          `json:"indicator_code"`
						FactorCode    string          `json:"factor_code"`
						Parameters    json.RawMessage `json:"parameters"`
					}
					if err := json.Unmarshal(content, &fixture); err != nil {
						t.Fatalf("decode Golden fixture: %v", err)
					}
					code := fixture.IndicatorCode
					if contract.codeField == "factor_code" {
						code = fixture.FactorCode
					}
					instance, err := json.Marshal(map[string]any{
						"schema_version": fixture.SchemaVersion,
						contract.codeField: code,
						"parameters":       fixture.Parameters,
					})
					if err != nil {
						t.Fatalf("encode parameter contract instance: %v", err)
					}
					if err := validateJSONSchemaInstance(schema, instance); err != nil {
						t.Fatalf("Golden parameters violate %s: %v", contract.schemaFile, err)
					}
				})
			}
		})
	}
}

func compileQuantSchemaFile(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	compiled, err := compileJSONSchema(readQuantSchema(t, name))
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	return compiled
}


func TestRegistryEntrySchemaEnforcesValueTypeConditionals(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-001-registry-entry-1.0.0.schema.json")

	decimalEntry := validEntry("decimal_metric", "1.0.0")
	decimalEntry.ValueType = ValueTypeDecimalString
	decimalEntry.Precision.Tolerance = json.RawMessage(`"0.000001"`)
	decimalEntry.Range = NumericRange{
		Minimum: json.RawMessage(`"-1"`),
		Maximum: json.RawMessage(`"1"`),
	}
	encoded, err := json.Marshal(decimalEntry)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaInstance(schema, encoded); err != nil {
		t.Fatalf("valid decimal_string entry rejected: %v", err)
	}
	decimalEntry.Precision.Tolerance = json.RawMessage("0.000001")
	encoded, err = json.Marshal(decimalEntry)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaInstance(schema, encoded); err == nil {
		t.Fatal("decimal_string entry with numeric tolerance unexpectedly passed")
	}

	integerEntry := validEntry("integer_metric", "1.0.0")
	integerEntry.ValueType = ValueTypeInteger
	integerEntry.Precision.MaxFractionDigits = 0
	integerEntry.Precision.Tolerance = json.RawMessage("1")
	integerEntry.Range = NumericRange{
		Minimum: json.RawMessage("0"),
		Maximum: json.RawMessage("100"),
	}
	encoded, err = json.Marshal(integerEntry)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaInstance(schema, encoded); err != nil {
		t.Fatalf("valid integer entry rejected: %v", err)
	}
	integerEntry.Precision.MaxFractionDigits = 1
	encoded, err = json.Marshal(integerEntry)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateJSONSchemaInstance(schema, encoded); err == nil {
		t.Fatal("integer entry with non-zero fractional scale unexpectedly passed")
	}
}

func TestIndicatorParameterSchemaEnforcesOneOfCodeAndParameters(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-002-indicator-parameters-1.0.0.schema.json")
	valid := []byte(`{"schema_version":"1.0.0","indicator_code":"sma","parameters":{"window":5}}`)
	if err := validateJSONSchemaInstance(schema, valid); err != nil {
		t.Fatalf("valid SMA parameters rejected: %v", err)
	}
	mismatched := []byte(`{"schema_version":"1.0.0","indicator_code":"macd","parameters":{"window":5}}`)
	if err := validateJSONSchemaInstance(schema, mismatched); err == nil {
		t.Fatal("MACD code with SMA parameters unexpectedly passed oneOf")
	}
}





func TestCompileJSONSchemaRejectsInvalidUTF8(t *testing.T) {
	document := append([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"description":"`), 0xff)
	document = append(document, []byte(`"}`)...)
	if _, err := compileJSONSchema(document); err == nil {
		t.Fatal("schema containing invalid UTF-8 unexpectedly compiled")
	}
}

func TestValidateJSONSchemaInstanceRejectsInvalidUTF8(t *testing.T) {
	schema, err := compileJSONSchema([]byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object","additionalProperties":false,"required":["value"],"properties":{"value":{"type":"string"}}}`))
	if err != nil {
		t.Fatal(err)
	}
	instance := append([]byte(`{"value":"`), 0xff)
	instance = append(instance, []byte(`"}`)...)
	if err := validateJSONSchemaInstance(schema, instance); err == nil {
		t.Fatal("instance containing invalid UTF-8 unexpectedly passed")
	}
}
