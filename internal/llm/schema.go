package llm

import (
	"bytes"
	"encoding/json"
	"errors"

	jsonschema "github.com/santhosh-tekuri/jsonschema/v6"
)

const (
	MaxJSONDepth = 64
	draft2020SchemaURI = "https://json-schema.org/draft/2020-12/schema"
	responseSchemaResourceURI = "https://llm.invalid/schema/root.json"
)

var (
	ErrInvalidJSONSchema = errors.New("invalid JSON Schema Draft 2020-12")
	ErrJSONSchemaMismatch = errors.New("JSON value does not match schema")
)

// denyExternalSchemaLoader prevents caller-supplied $ref values from causing
// file or network access. Built-in JSON Schema meta-schemas are embedded by
// the validator and do not pass through this loader.
type denyExternalSchemaLoader struct{}

func (denyExternalSchemaLoader) Load(resourceURL string) (any, error) {
	return nil, errors.New("external JSON Schema resources are disabled")
}

// CompileJSONSchema compiles one bounded Draft 2020-12 schema. A missing
// $schema selects Draft 2020-12; any explicitly different dialect is rejected.
func CompileJSONSchema(raw []byte) (*jsonschema.Schema, error) {
	if len(raw) == 0 || len(raw) > MaxSchemaBytes || !json.Valid(raw) {
		return nil, ErrInvalidJSONSchema
	}
	if err := validateJSONDepth(raw, MaxJSONDepth); err != nil {
		return nil, ErrInvalidJSONSchema
	}

	document, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return nil, ErrInvalidJSONSchema
	}
	if object, ok := document.(map[string]any); ok {
		if declared, exists := object["$schema"]; exists {
			dialect, ok := declared.(string)
			if !ok || dialect != draft2020SchemaURI {
				return nil, ErrInvalidJSONSchema
			}
		}
	}

	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseLoader(denyExternalSchemaLoader{})
	if err := compiler.AddResource(responseSchemaResourceURI, document); err != nil {
		return nil, ErrInvalidJSONSchema
	}
	compiled, err := compiler.Compile(responseSchemaResourceURI)
	if err != nil {
		return nil, ErrInvalidJSONSchema
	}
	return compiled, nil
}

// ValidateJSONAgainstSchema validates a bounded JSON instance against a
// locally compiled Draft 2020-12 schema with format assertions enabled.
func ValidateJSONAgainstSchema(schemaJSON, valueJSON []byte) error {
	if len(valueJSON) == 0 || len(valueJSON) > MaxPayloadBytes || !json.Valid(valueJSON) {
		return ErrJSONSchemaMismatch
	}
	if err := validateJSONDepth(valueJSON, MaxJSONDepth); err != nil {
		return ErrJSONSchemaMismatch
	}
	compiled, err := CompileJSONSchema(schemaJSON)
	if err != nil {
		return ErrInvalidJSONSchema
	}
	instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(valueJSON))
	if err != nil {
		return ErrJSONSchemaMismatch
	}
	if err := compiled.Validate(instance); err != nil {
		return ErrJSONSchemaMismatch
	}
	return nil
}

// validateJSONDepth counts structural nesting while ignoring delimiters inside
// JSON strings. The caller must validate JSON syntax before invoking it.
func validateJSONDepth(raw []byte, maxDepth int) error {
	depth := 0
	inString := false
	escaped := false
	for _, char := range raw {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch char {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{', '[':
			depth++
			if depth > maxDepth {
				return errors.New("JSON nesting depth exceeds limit")
			}
		case '}', ']':
			depth--
		}
	}
	return nil
}
