package context

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestContextJSONSchemaMatchesDTOFields(t *testing.T) {
	data, err := os.ReadFile("schema/context-v1.schema.json")
	if err != nil {
		t.Fatalf("read Context JSON Schema: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatalf("parse Context JSON Schema: %v", err)
	}
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected JSON Schema dialect: %v", document["$schema"])
	}
	properties := schemaObject(t, document["properties"])
	assertSchemaStructFields(t, "Context", reflect.TypeOf(Context{}), properties)

	definitions := schemaObject(t, document["$defs"])
	assertSchemaStructFields(t, "ContextSectionRef", reflect.TypeOf(SectionRef{}), schemaObject(t, definitions["ContextSectionRef"]))
	assertSchemaStructFields(t, "OmittedSection", reflect.TypeOf(OmittedSection{}), schemaObject(t, definitions["OmittedSection"]))
	assertSchemaStructFields(t, "QualitySummary", reflect.TypeOf(QualitySummary{}), schemaObject(t, definitions["QualitySummary"]))

	timeFields := []map[string]any{
		schemaObject(t, properties["system_as_of"]),
		schemaObject(t, properties["created_at"]),
		schemaObject(t, schemaObject(t, definitions["ContextSectionRef"])["properties"].(map[string]any)["assembled_at"]),
		schemaObject(t, schemaObject(t, definitions["ContextSectionRef"])["properties"].(map[string]any)["system_as_of"]),
	}
	for _, field := range timeFields {
		if field["format"] != "date-time" || field["pattern"] != "Z$" {
			t.Errorf("Context time field must require RFC3339 date-time ending in UTC Z: %v", field)
		}
	}

	version := schemaObject(t, properties["schema_version"])
	if version["const"] != "1.0.0" {
		t.Fatalf("schema_version const = %v, want 1.0.0", version["const"])
	}
}

func schemaObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("schema node has type %T, want object", value)
	}
	return object
}

func assertSchemaStructFields(t *testing.T, name string, typ reflect.Type, schema map[string]any) {
	t.Helper()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("%s schema must be a closed object", name)
	}
	properties := schemaObject(t, schema["properties"])
	requiredValues, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("%s schema required must be an array", name)
	}
	required := make(map[string]struct{}, len(requiredValues))
	for _, value := range requiredValues {
		field, ok := value.(string)
		if !ok {
			t.Fatalf("%s schema required contains non-string value %T", name, value)
		}
		required[field] = struct{}{}
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			continue
		}
		if _, exists := properties[jsonName]; !exists {
			t.Errorf("%s schema is missing DTO field %q", name, jsonName)
		}
		if _, exists := required[jsonName]; !exists {
			t.Errorf("%s schema does not require DTO field %q", name, jsonName)
		}
		delete(required, jsonName)
	}
	for field := range required {
		t.Errorf("%s schema requires field %q absent from DTO", name, field)
	}
	for field := range properties {
		if _, exists := requiredFieldNames(typ)[field]; !exists {
			t.Errorf("%s schema defines field %q absent from DTO", name, field)
		}
	}
}

func requiredFieldNames(typ reflect.Type) map[string]struct{} {
	fields := make(map[string]struct{}, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			fields[name] = struct{}{}
		}
	}
	return fields
}


func TestContextJSONSchemaValidatesGoldenAndRejectsInvalidPayloads(t *testing.T) {
	schemaData, err := os.ReadFile("schema/context-v1.schema.json")
	if err != nil {
		t.Fatalf("read Context JSON Schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("context-v1.schema.json", strings.NewReader(string(schemaData))); err != nil {
		t.Fatalf("add Context JSON Schema resource: %v", err)
	}
	schema, err := compiler.Compile("context-v1.schema.json")
	if err != nil {
		t.Fatalf("compile Context JSON Schema: %v", err)
	}

	golden, err := os.ReadFile("testdata/context_v1.golden.json")
	if err != nil {
		t.Fatalf("read Context v1 golden fixture: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(golden, &payload); err != nil {
		t.Fatalf("decode Context v1 golden fixture: %v", err)
	}
	if err := schema.Validate(payload); err != nil {
		t.Fatalf("Context v1 golden fixture does not satisfy JSON Schema: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"missing required field", func(value map[string]any) { delete(value, "context_id") }},
		{"null required array", func(value map[string]any) { value["subject_refs"] = nil }},
		{"unknown scope enum", func(value map[string]any) { value["scope"] = "portfolio" }},
		{"unknown top-level field", func(value map[string]any) { value["unexpected"] = true }},
		{"unknown section owner enum", func(value map[string]any) {
			sections := value["section_refs"].([]any)
			sections[0].(map[string]any)["source_owner"] = "unknown_owner"
		}},
		{"unknown omission reason enum", func(value map[string]any) {
			omitted := value["omitted_sections"].([]any)
			omitted[0].(map[string]any)["reason_code"] = "unknown_reason"
		}},
		{"non UTC timestamp", func(value map[string]any) {
			value["system_as_of"] = "2026-11-06T16:00:00+08:00"
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			encoded, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("clone fixture: %v", err)
			}
			var candidate map[string]any
			if err := json.Unmarshal(encoded, &candidate); err != nil {
				t.Fatalf("decode cloned fixture: %v", err)
			}
			tt.mutate(candidate)
			if err := schema.Validate(candidate); err == nil {
				t.Fatal("invalid payload unexpectedly satisfied Context JSON Schema")
			}
		})
	}
}

func TestContextJSONSchemaRejectsNullNestedObject(t *testing.T) {
	schemaData, err := os.ReadFile("schema/context-v1.schema.json")
	if err != nil {
		t.Fatalf("read Context JSON Schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("context-v1.schema.json", strings.NewReader(string(schemaData))); err != nil {
		t.Fatalf("add Context JSON Schema resource: %v", err)
	}
	schema, err := compiler.Compile("context-v1.schema.json")
	if err != nil {
		t.Fatalf("compile Context JSON Schema: %v", err)
	}
	golden, err := os.ReadFile("testdata/context_v1.golden.json")
	if err != nil {
		t.Fatalf("read Context v1 golden fixture: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(golden, &payload); err != nil {
		t.Fatalf("decode Context v1 golden fixture: %v", err)
	}
	payload["quality_summary"] = nil
	if err := schema.Validate(payload); err == nil {
		t.Fatal("null quality_summary unexpectedly satisfied Context JSON Schema")
	}
}
