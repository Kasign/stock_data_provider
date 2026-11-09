package context

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

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
	if strings.Count(string(data), `"maximum": 9223372036854775807`) != 5 {
		t.Fatal("all five Context integer fields must use the exact Go int64 maximum")
	}
	if document["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		t.Fatalf("unexpected JSON Schema dialect: %v", document["$schema"])
	}
	properties := schemaObject(t, document["properties"])
	assertSchemaStructFields(t, "Context", reflect.TypeOf(Context{}), document)

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
		if _, exists := required[field]; exists {
			t.Errorf("%s schema required contains duplicate field %q", name, field)
		}
		required[field] = struct{}{}
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			continue
		}
		property, exists := properties[jsonName]
		if !exists {
			t.Errorf("%s schema is missing DTO field %q", name, jsonName)
			continue
		}
		assertContextSchemaValueType(t, name+"."+jsonName, field.Type, schemaObject(t, property))
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

func assertContextSchemaValueType(t *testing.T, name string, typ reflect.Type, schema map[string]any) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		t.Errorf("%s pointer DTO type requires an explicit nullable Schema mapping", name)
		return
	}
	switch typ.Kind() {
	case reflect.String:
		if schema["type"] == "string" {
			return
		}
		ref, _ := schema["$ref"].(string)
		if ref != "#/$defs/sourceOwner" && ref != "#/$defs/qualityStatus" {
			t.Errorf("%s schema must be a string or a known string enum reference: %v", name, schema)
		}
	case reflect.Bool:
		if schema["type"] != "boolean" {
			t.Errorf("%s schema type = %v, want boolean", name, schema["type"])
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32,
		reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64:
		if schema["type"] != "integer" {
			t.Errorf("%s schema type = %v, want integer", name, schema["type"])
		}
	case reflect.Slice, reflect.Array:
		if schema["type"] != "array" {
			t.Errorf("%s schema type = %v, want array", name, schema["type"])
			return
		}
		items, ok := schema["items"].(map[string]any)
		if !ok {
			t.Errorf("%s schema items must be an object", name)
			return
		}
		assertContextSchemaValueType(t, name+"[]", typ.Elem(), items)
	case reflect.Struct:
		if typ == reflect.TypeOf(time.Time{}) {
			if schema["type"] != "string" || schema["format"] != "date-time" {
				t.Errorf("%s schema must be a date-time string: %v", name, schema)
			}
			return
		}
		definition := typ.Name()
		switch definition {
		case "SectionRef", "ContextSectionRef":
			definition = "ContextSectionRef"
		case "OmittedSection", "ContextOmittedSection":
			definition = "OmittedSection"
		case "QualitySummary", "ContextQualitySummary":
			definition = "QualitySummary"
		}
		wantRef := "#/$defs/" + definition
		if schema["$ref"] != wantRef {
			t.Errorf("%s schema ref = %v, want %s", name, schema["$ref"], wantRef)
		}
	default:
		t.Errorf("%s has unsupported DTO type %s; add an explicit Schema mapping", name, typ)
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
	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatalf("parse Context JSON Schema for validator: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("context-v1.schema.json", schemaDocument); err != nil {
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
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(golden))
	if err != nil {
		t.Fatalf("parse Context v1 golden for validator: %v", err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("Context v1 golden fixture does not satisfy JSON Schema: %v", err)
	}

	maxInt64Fixture := strings.Replace(string(golden), `"context_version": 1`, `"context_version": 9223372036854775807`, 1)
	maxInt64Fixture = strings.Replace(maxInt64Fixture, `"conflict_count": 0`, `"conflict_count": 9223372036854775807`, 1)
	maxInt64Instance, err := jsonschema.UnmarshalJSON(strings.NewReader(maxInt64Fixture))
	if err != nil {
		t.Fatalf("parse max int64 boundary fixture: %v", err)
	}
	if err := schema.Validate(maxInt64Instance); err != nil {
		t.Fatalf("Context int64 maximum boundary must be accepted: %v", err)
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
		{"non UTC nested timestamp", func(value map[string]any) {
			sections := value["section_refs"].([]any)
			sections[0].(map[string]any)["assembled_at"] = "2026-11-06T16:00:00+08:00"
		}},
		{"invalid date-time format", func(value map[string]any) {
			value["created_at"] = "not-a-date"
		}},
		{"unknown nested section field", func(value map[string]any) {
			sections := value["section_refs"].([]any)
			sections[0].(map[string]any)["unexpected"] = true
		}},
		{"context version below minimum", func(value map[string]any) {
			value["context_version"] = 0
		}},
		{"context version exceeds Go int64", func(value map[string]any) {
			value["context_version"] = json.Number("9223372036854775808")
		}},
		{"quality count exceeds Go int64", func(value map[string]any) {
			quality := value["quality_summary"].(map[string]any)
			quality["conflict_count"] = json.Number("9223372036854775808")
		}},
		{"whitespace-only research question", func(value map[string]any) {
			value["research_question"] = "   "
		}},
		{"padded context identifier", func(value map[string]any) {
			value["context_id"] = " context-1 "
		}},
		{"whitespace-only subject reference", func(value map[string]any) {
			value["subject_refs"] = []any{"   "}
		}},
		{"padded builder version", func(value map[string]any) {
			value["builder_version"] = " builder "
		}},
		{"padded nested section identifier", func(value map[string]any) {
			sections := value["section_refs"].([]any)
			sections[0].(map[string]any)["section_id"] = " section "
		}},
		{"whitespace-only nested safe message", func(value map[string]any) {
			omitted := value["omitted_sections"].([]any)
			omitted[0].(map[string]any)["safe_message"] = "   "
		}},
		{"padded omitted section identifier", func(value map[string]any) {
			omitted := value["omitted_sections"].([]any)
			omitted[0].(map[string]any)["section_id"] = " section "
		}},
		{"padded omitted source reference", func(value map[string]any) {
			omitted := value["omitted_sections"].([]any)
			omitted[0].(map[string]any)["source_refs"] = []any{" source "}
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
			candidateJSON, err := json.Marshal(candidate)
			if err != nil {
				t.Fatalf("encode invalid fixture: %v", err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(candidateJSON))
			if err != nil {
				t.Fatalf("parse invalid fixture for validator: %v", err)
			}
			if err := schema.Validate(instance); err == nil {
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
	schemaDocument, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemaData))
	if err != nil {
		t.Fatalf("parse Context JSON Schema for validator: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("context-v1.schema.json", schemaDocument); err != nil {
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
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("encode null nested object fixture: %v", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("parse null nested object fixture: %v", err)
	}
	if err := schema.Validate(instance); err == nil {
		t.Fatal("null quality_summary unexpectedly satisfied Context JSON Schema")
	}
}
