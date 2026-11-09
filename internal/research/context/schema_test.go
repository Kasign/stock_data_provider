package context

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
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
