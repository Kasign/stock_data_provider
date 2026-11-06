package registry

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

const draft202012SchemaURI = "https://json-schema.org/draft/2020-12/schema"

func compileJSONSchema(document []byte) (*jsonschema.Schema, error) {
	document = bytes.TrimSpace(document)
	if len(document) == 0 || !json.Valid(document) {
		return nil, fmt.Errorf("schema document must be valid JSON")
	}

	var root map[string]json.RawMessage
	if err := json.Unmarshal(document, &root); err != nil || root == nil {
		return nil, fmt.Errorf("schema root must be an object")
	}
	var draftURI string
	if err := json.Unmarshal(root["$schema"], &draftURI); err != nil || draftURI != draft202012SchemaURI {
		return nil, fmt.Errorf("schema must declare Draft 2020-12")
	}
	var rootType string
	if err := json.Unmarshal(root["type"], &rootType); err != nil || rootType != "object" {
		return nil, fmt.Errorf("schema root type must be object")
	}
	var additionalProperties bool
	if err := json.Unmarshal(root["additionalProperties"], &additionalProperties); err != nil || additionalProperties {
		return nil, fmt.Errorf("schema root additionalProperties must be false")
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	compiler.LoadURL = func(uri string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external schema reference is disabled: %s", uri)
	}
	const schemaURL = "https://quant.invalid/schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		return nil, err
	}
	return compiler.Compile(schemaURL)
}

func validateJSONSchemaInstance(schema *jsonschema.Schema, instanceJSON []byte) error {
	if schema == nil {
		return fmt.Errorf("compiled schema is required")
	}
	decoder := json.NewDecoder(bytes.NewReader(instanceJSON))
	decoder.UseNumber()
	var instance any
	if err := decoder.Decode(&instance); err != nil {
		return fmt.Errorf("decode schema instance: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("schema instance contains trailing JSON")
	}
	if err := schema.Validate(instance); err != nil {
		return fmt.Errorf("schema instance validation failed: %w", err)
	}
	return nil
}
