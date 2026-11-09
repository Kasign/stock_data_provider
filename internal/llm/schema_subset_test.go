package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestValidateResponsesStrictSchemaSubsetAcceptsSupportedSchema(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"summary":{"type":"string","format":"email"},
			"tags":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":5},
			"label":{"$ref":"#/$defs/label"},
			"optional_note":{"type":["string","null"]},
			"status":{"anyOf":[{"type":"string","enum":["ok","partial"]},{"type":"null"}]}
		},
		"required":["summary","tags","label","optional_note","status"],
		"additionalProperties":false,
		"$defs":{"label":{"type":"string"}}
	}`)
	if err := validateResponsesStrictSchemaSubset(schema); err != nil {
		t.Fatalf("supported strict schema rejected: %v", err)
	}
}

func TestValidateResponsesStrictSchemaSubsetRejectsUnsupportedFeatures(t *testing.T) {
	cases := []struct {
		name   string
		schema string
	}{
		{
			name:   "root anyOf",
			schema: `{"anyOf":[{"type":"object","properties":{},"required":[],"additionalProperties":false}]}`,
		},
		{
			name:   "allOf",
			schema: `{"type":"object","properties":{},"required":[],"additionalProperties":false,"allOf":[]}`,
		},
		{
			name:   "missing required property",
			schema: `{"type":"object","properties":{"name":{"type":"string"}},"required":[],"additionalProperties":false}`,
		},
		{
			name:   "additional properties enabled",
			schema: `{"type":"object","properties":{},"required":[],"additionalProperties":true}`,
		},
		{
			name:   "unsupported string format",
			schema: `{"type":"object","properties":{"value":{"type":"string","format":"ulid"}},"required":["value"],"additionalProperties":false}`,
		},
		{
			name:   "unsupported keyword",
			schema: `{"type":"object","properties":{},"required":[],"additionalProperties":false,"minProperties":1}`,
		},
		{
			name:   "external reference",
			schema: `{"type":"object","properties":{"value":{"$ref":"https://example.com/schema.json"}},"required":["value"],"additionalProperties":false}`,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResponsesStrictSchemaSubset([]byte(tt.schema))
			if !errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
				t.Fatalf("unsupported strict schema must be rejected: %v", err)
			}
		})
	}
}

func TestMapResponsesRequestRejectsUnsupportedStrictSchemaButAllowsLocalValidation(t *testing.T) {
	request := validResponsesRequest()
	request.ResponseSchema = &JSONSchemaSpec{
		Name: "summary", Strict: true,
		SchemaJSON: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false,"allOf":[]}`),
	}
	if _, err := MapResponsesRequest(request); !errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
		t.Fatalf("unsupported upstream schema must be rejected: %v", err)
	}

	request.LocalValidationOnly = true
	if _, err := MapResponsesRequest(request); err != nil {
		t.Fatalf("local_validation_only must retain full local Draft 2020-12 validation: %v", err)
	}
}

func TestMapResponsesRequestRejectsUnsupportedStrictToolSchema(t *testing.T) {
	request := validResponsesRequest()
	request.Tools = []ToolDefinition{{
		Name: "lookup_quote", Strict: true,
		ParametersJSON: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false,"allOf":[]}`),
	}}
	if _, err := MapResponsesRequest(request); !errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
		t.Fatalf("unsupported strict tool schema must be rejected: %v", err)
	}
}


// TestValidateResponsesStrictSchemaSubsetEnforcesDocumentedLimits 验证上游 strict Schema 的结构预算。
func TestValidateResponsesStrictSchemaSubsetEnforcesDocumentedLimits(t *testing.T) {
	t.Run("object nesting", func(t *testing.T) {
		var nested any = map[string]any{"type": "string"}
		for i := 0; i < 10; i++ {
			nested = map[string]any{
				"type": "object", "properties": map[string]any{"next": nested},
				"required": []string{"next"}, "additionalProperties": false,
			}
		}
		schema := map[string]any{
			"type": "object", "properties": map[string]any{"nested": nested},
			"required": []string{"nested"}, "additionalProperties": false,
		}
		raw, _ := json.Marshal(schema)
		if err := validateResponsesStrictSchemaSubset(raw); !errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
			t.Fatalf("more than 10 object levels must be rejected: %v", err)
		}
	})

	t.Run("object property count", func(t *testing.T) {
		properties := make(map[string]any, maxResponsesSchemaProperties+1)
		required := make([]string, 0, maxResponsesSchemaProperties+1)
		for i := 0; i <= maxResponsesSchemaProperties; i++ {
			name := fmt.Sprintf("field_%04d", i)
			properties[name] = map[string]any{"type": "string"}
			required = append(required, name)
		}
		raw, _ := json.Marshal(map[string]any{
			"type": "object", "properties": properties, "required": required,
			"additionalProperties": false,
		})
		if err := validateResponsesStrictSchemaSubset(raw); !errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
			t.Fatalf("more than 5000 properties must be rejected: %v", err)
		}
	})

	t.Run("enum count", func(t *testing.T) {
		values := make([]string, maxResponsesSchemaEnumValues+1)
		for i := range values {
			values[i] = fmt.Sprintf("value_%04d", i)
		}
		raw, _ := json.Marshal(map[string]any{
			"type": "object",
			"properties": map[string]any{"state": map[string]any{"type": "string", "enum": values}},
			"required": []string{"state"}, "additionalProperties": false,
		})
		if err := validateResponsesStrictSchemaSubset(raw); !errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
			t.Fatalf("more than 1000 enum values must be rejected: %v", err)
		}
	})

	t.Run("string metadata size", func(t *testing.T) {
		name := strings.Repeat("x", maxResponsesSchemaStringChars+1)
		raw, _ := json.Marshal(map[string]any{
			"type": "object", "properties": map[string]any{name: map[string]any{"type": "string"}},
			"required": []string{name}, "additionalProperties": false,
		})
		if err := validateResponsesStrictSchemaSubset(raw); !errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
			t.Fatalf("more than 120000 schema metadata characters must be rejected: %v", err)
		}
	})
}
