package llm

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestValidateResponsesStrictSchemaSubsetAcceptsSupportedSchema(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"properties":{
			"summary":{"type":"string","format":"email"},
			"tags":{"type":"array","items":{"type":"string"},"minItems":1,"maxItems":5},
			"optional_note":{"type":["string","null"]},
			"status":{"anyOf":[{"type":"string","enum":["ok","partial"]},{"type":"null"}]}
		},
		"required":["summary","tags","optional_note","status"],
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
			name: "root anyOf",
			schema: `{"anyOf":[{"type":"object","properties":{},"required":[],"additionalProperties":false}]}`,
		},
		{
			name: "allOf",
			schema: `{"type":"object","properties":{},"required":[],"additionalProperties":false,"allOf":[]}`,
		},
		{
			name: "missing required property",
			schema: `{"type":"object","properties":{"name":{"type":"string"}},"required":[],"additionalProperties":false}`,
		},
		{
			name: "additional properties enabled",
			schema: `{"type":"object","properties":{},"required":[],"additionalProperties":true}`,
		},
		{
			name: "unsupported string format",
			schema: `{"type":"object","properties":{"value":{"type":"string","format":"ulid"}},"required":["value"],"additionalProperties":false}`,
		},
		{
			name: "unsupported keyword",
			schema: `{"type":"object","properties":{},"required":[],"additionalProperties":false,"minProperties":1}`,
		},
		{
			name: "external reference",
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
