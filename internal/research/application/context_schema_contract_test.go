package application

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

// Context JSON Schema is consumed through the Application DTO, not the
// internal Domain model. Keep both representations aligned at this boundary.
func TestResearchContextApplicationDTOMatchesSchemaAndGolden(t *testing.T) {
	schemaData, err := os.ReadFile("../context/schema/context-v1.schema.json")
	if err != nil {
		t.Fatalf("read Context JSON Schema: %v", err)
	}
	var document map[string]any
	if err := json.Unmarshal(schemaData, &document); err != nil {
		t.Fatalf("decode Context JSON Schema: %v", err)
	}
	properties := applicationSchemaObject(t, document["properties"])
	definitions := applicationSchemaObject(t, document["$defs"])

	assertApplicationSchemaStruct(t, "ResearchContext", reflect.TypeOf(ResearchContext{}), document)
	assertApplicationSchemaStruct(t, "ContextSectionRef", reflect.TypeOf(ContextSectionRef{}), applicationSchemaObject(t, definitions["ContextSectionRef"]))
	assertApplicationSchemaStruct(t, "ContextOmittedSection", reflect.TypeOf(ContextOmittedSection{}), applicationSchemaObject(t, definitions["OmittedSection"]))
	assertApplicationSchemaStruct(t, "ContextQualitySummary", reflect.TypeOf(ContextQualitySummary{}), applicationSchemaObject(t, definitions["QualitySummary"]))

	for _, field := range []string{"system_as_of", "created_at"} {
		node := applicationSchemaObject(t, properties[field])
		if node["format"] != "date-time" || node["pattern"] != "Z$" {
			t.Errorf("Application Context field %q must require UTC date-time: %v", field, node)
		}
	}

	golden, err := os.ReadFile("../context/testdata/context_v1.golden.json")
	if err != nil {
		t.Fatalf("read Context v1 golden: %v", err)
	}
	var dto ResearchContext
	if err := json.Unmarshal(golden, &dto); err != nil {
		t.Fatalf("decode Context v1 golden into Application DTO: %v", err)
	}
	encoded, err := json.Marshal(dto)
	if err != nil {
		t.Fatalf("encode Application Context DTO: %v", err)
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
	if err != nil {
		t.Fatalf("parse Application Context DTO for Schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.AssertFormat()
	if err := compiler.AddResource("research-context-v1.schema.json", mustApplicationSchemaDocument(t, schemaData)); err != nil {
		t.Fatalf("add Context JSON Schema: %v", err)
	}
	schema, err := compiler.Compile("research-context-v1.schema.json")
	if err != nil {
		t.Fatalf("compile Context JSON Schema: %v", err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("Application Context DTO encoded from Golden does not satisfy Schema: %v", err)
	}
}


func assertContextSchemaValueType(t *testing.T, name string, typ reflect.Type, schema map[string]any) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	switch typ.Kind() {
	case reflect.String:
		if schema["type"] != "string" {
			t.Errorf("%s schema type = %v, want string", name, schema["type"])
		}
	case reflect.Bool:
		if schema["type"] != "boolean" {
			t.Errorf("%s schema type = %v, want boolean", name, schema["type"])
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
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
		wantRef := "#/$defs/" + typ.Name()
		if schema["$ref"] != wantRef {
			t.Errorf("%s schema ref = %v, want %s", name, schema["$ref"], wantRef)
		}
	default:
		t.Errorf("%s has unsupported DTO type %s; add an explicit Schema mapping", name, typ)
	}
}

func mustApplicationSchemaDocument(t *testing.T, data []byte) *jsonschema.Schema {
	t.Helper()
	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("parse Context JSON Schema: %v", err)
	}
	return document
}

func applicationSchemaObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("schema node has type %T, want object", value)
	}
	return object
}

func assertApplicationSchemaStruct(t *testing.T, name string, typ reflect.Type, schema map[string]any) {
	t.Helper()
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		t.Fatalf("%s schema must be a closed object", name)
	}
	properties := applicationSchemaObject(t, schema["properties"])
	requiredValues, ok := schema["required"].([]any)
	if !ok {
		t.Fatalf("%s schema required must be an array", name)
	}
	required := make(map[string]struct{}, len(requiredValues))
	for _, value := range requiredValues {
		field, ok := value.(string)
		if !ok {
			t.Fatalf("%s schema required contains %T", name, value)
		}
		if _, exists := required[field]; exists {
			t.Errorf("%s schema repeats required field %q", name, field)
		}
		required[field] = struct{}{}
	}
	fields := make(map[string]struct{}, typ.NumField())
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		jsonName := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonName == "" || jsonName == "-" {
			continue
		}
		fields[jsonName] = struct{}{}
		assertContextSchemaValueType(t, name+"."+jsonName, field.Type, schemaObject(t, properties[jsonName]))
		if _, exists := properties[jsonName]; !exists {
			t.Errorf("%s schema is missing Application DTO field %q", name, jsonName)
		}
		if _, exists := required[jsonName]; !exists {
			t.Errorf("%s schema does not require Application DTO field %q", name, jsonName)
		}
		delete(required, jsonName)
	}
	for field := range required {
		t.Errorf("%s schema requires field %q absent from Application DTO", name, field)
	}
	for field := range properties {
		if _, exists := fields[field]; !exists {
			t.Errorf("%s schema defines field %q absent from Application DTO", name, field)
		}
	}
}
