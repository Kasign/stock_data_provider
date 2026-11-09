package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"unicode/utf8"
)

// ErrUnsupportedResponsesSchemaSubset 表示 Schema 不属于 Responses strict 模式支持的子集。
var ErrUnsupportedResponsesSchemaSubset = errors.New("schema is outside the Responses strict JSON Schema subset")

const (
	maxResponsesSchemaObjectDepth = 10
	maxResponsesSchemaProperties  = 5000
	maxResponsesSchemaStringChars = 120000
	maxResponsesSchemaEnumValues  = 1000
)

type responsesSchemaSubsetBudget struct {
	properties  int
	stringChars int
	enumValues  int
}

var responsesSchemaSubsetKeywords = map[string]struct{}{
	"$schema": {}, "$defs": {}, "$ref": {}, "title": {}, "description": {},
	"type": {}, "enum": {}, "const": {}, "anyOf": {},
	"properties": {}, "required": {}, "additionalProperties": {}, "items": {},
	"pattern": {}, "format": {}, "multipleOf": {}, "maximum": {},
	"exclusiveMaximum": {}, "minimum": {}, "exclusiveMinimum": {},
	"minItems": {}, "maxItems": {},
}

var responsesSchemaSubsetFormats = map[string]struct{}{
	"date-time": {}, "time": {}, "date": {}, "duration": {},
	"email": {}, "hostname": {}, "ipv4": {}, "ipv6": {}, "uuid": {},
}

var responsesSchemaSubsetTypes = map[string]struct{}{
	"string": {}, "number": {}, "integer": {}, "boolean": {},
	"object": {}, "array": {}, "null": {},
}

// validateResponsesStrictSchemaSubset validates the documented strict-mode
// subset before a schema is sent to the Responses API. Local validation keeps
// the full Draft 2020-12 dialect and does not call this function.
func validateResponsesStrictSchemaSubset(raw []byte) error {
	if len(raw) == 0 || !json.Valid(raw) {
		return fmt.Errorf("%w: invalid JSON", ErrUnsupportedResponsesSchemaSubset)
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return fmt.Errorf("%w: root must be an object schema", ErrUnsupportedResponsesSchemaSubset)
	}
	if rootType, ok := root["type"].(string); !ok || rootType != "object" {
		return fmt.Errorf("%w: root schema must have type object", ErrUnsupportedResponsesSchemaSubset)
	}
	budget := responsesSchemaSubsetBudget{}
	if err := validateResponsesSchemaNode(root, 0, true, &budget); err != nil {
		return fmt.Errorf("%w: %v", ErrUnsupportedResponsesSchemaSubset, err)
	}
	return nil
}

func validateResponsesSchemaNode(node map[string]any, parentObjectDepth int, root bool, budget *responsesSchemaSubsetBudget) error {
	for key := range node {
		if _, ok := responsesSchemaSubsetKeywords[key]; !ok {
			return fmt.Errorf("unsupported keyword %q", key)
		}
	}

	if dialect, exists := node["$schema"]; exists {
		if dialect != draft2020SchemaURI {
			return errors.New("only Draft 2020-12 is supported")
		}
	}
	if title, exists := node["title"]; exists {
		if _, ok := title.(string); !ok {
			return errors.New("title must be a string")
		}
	}
	if description, exists := node["description"]; exists {
		if _, ok := description.(string); !ok {
			return errors.New("description must be a string")
		}
	}
	if ref, exists := node["$ref"]; exists {
		value, ok := ref.(string)
		if !ok || value == "" || value[0] != '#' {
			return errors.New("only local JSON Schema references are supported")
		}
	}

	types := make(map[string]struct{})
	if rawType, exists := node["type"]; exists {
		switch value := rawType.(type) {
		case string:
			if _, ok := responsesSchemaSubsetTypes[value]; !ok {
				return fmt.Errorf("unsupported type %q", value)
			}
			types[value] = struct{}{}
		case []any:
			if len(value) == 0 {
				return errors.New("type arrays must contain at least two types")
			}
			for _, item := range value {
				typeName, ok := item.(string)
				if !ok {
					return errors.New("type array values must be strings")
				}
				if _, ok := responsesSchemaSubsetTypes[typeName]; !ok {
					return fmt.Errorf("unsupported type %q", typeName)
				}
				if _, duplicate := types[typeName]; duplicate {
					return fmt.Errorf("duplicate type %q", typeName)
				}
				types[typeName] = struct{}{}
			}
		default:
			return errors.New("type must be a string or a string array")
		}
	}
	hasType := len(types) > 0
	isObject := hasType && hasResponsesSchemaType(types, "object")
	isArray := hasType && hasResponsesSchemaType(types, "array")
	isString := hasType && hasResponsesSchemaType(types, "string")
	isNumber := hasType && (hasResponsesSchemaType(types, "number") || hasResponsesSchemaType(types, "integer"))

	objectDepth := parentObjectDepth
	if isObject {
		objectDepth++
		if objectDepth > maxResponsesSchemaObjectDepth {
			return fmt.Errorf("object nesting exceeds %d levels", maxResponsesSchemaObjectDepth)
		}
	}
	if root && (len(types) != 1 || !isObject) {
		return errors.New("root schema must be an object and cannot be a union")
	}

	if propertiesValue, exists := node["properties"]; exists {
		if !isObject {
			return errors.New("properties is only supported on object schemas")
		}
		properties, ok := propertiesValue.(map[string]any)
		if !ok {
			return errors.New("properties must be an object")
		}
		budget.properties += len(properties)
		if budget.properties > maxResponsesSchemaProperties {
			return fmt.Errorf("schema has more than %d object properties", maxResponsesSchemaProperties)
		}
		for name, rawProperty := range properties {
			budget.addString(name)
			property, ok := rawProperty.(map[string]any)
			if !ok {
				return fmt.Errorf("property %q must be a schema object", name)
			}
			if err := validateResponsesSchemaNode(property, objectDepth, false, budget); err != nil {
				return fmt.Errorf("property %q: %w", name, err)
			}
		}
	}
	if isObject {
		properties, hasProperties := node["properties"].(map[string]any)
		if !hasProperties {
			return errors.New("object schemas require properties")
		}
		required, ok := node["required"].([]any)
		if !ok {
			return errors.New("object schemas require a required array")
		}
		requiredSet := make(map[string]struct{}, len(required))
		for _, rawName := range required {
			name, ok := rawName.(string)
			if !ok {
				return errors.New("required values must be strings")
			}
			if _, duplicate := requiredSet[name]; duplicate {
				return fmt.Errorf("duplicate required property %q", name)
			}
			requiredSet[name] = struct{}{}
		}
		if len(requiredSet) != len(properties) {
			return errors.New("all object properties must be required")
		}
		for name := range properties {
			if _, ok := requiredSet[name]; !ok {
				return fmt.Errorf("property %q is not listed in required", name)
			}
		}
		additional, ok := node["additionalProperties"].(bool)
		if !ok || additional {
			return errors.New("object schemas require additionalProperties=false")
		}
	} else if _, exists := node["required"]; exists {
		return errors.New("required is only supported on object schemas")
	}
	if additional, exists := node["additionalProperties"]; exists {
		if !isObject || additional != false {
			return errors.New("additionalProperties is only supported as false on object schemas")
		}
	}

	if items, exists := node["items"]; exists {
		if !isArray {
			return errors.New("items is only supported on array schemas")
		}
		itemSchema, ok := items.(map[string]any)
		if !ok {
			return errors.New("items must be a schema object")
		}
		if err := validateResponsesSchemaNode(itemSchema, objectDepth, false, budget); err != nil {
			return fmt.Errorf("items: %w", err)
		}
	}
	if isArray {
		if _, exists := node["items"]; !exists {
			return errors.New("array schemas require items")
		}
	}
	for _, key := range []string{"minItems", "maxItems"} {
		if _, exists := node[key]; exists && !isArray {
			return fmt.Errorf("%s is only supported on array schemas", key)
		}
	}
	for _, key := range []string{"pattern", "format"} {
		value, exists := node[key]
		if !exists {
			continue
		}
		if !isString {
			return fmt.Errorf("%s is only supported on string schemas", key)
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", key)
		}
		if key == "format" {
			if _, ok := responsesSchemaSubsetFormats[text]; !ok {
				return fmt.Errorf("unsupported string format %q", text)
			}
		}
	}
	for _, key := range []string{"multipleOf", "maximum", "exclusiveMaximum", "minimum", "exclusiveMinimum"} {
		if _, exists := node[key]; exists && !isNumber {
			return fmt.Errorf("%s is only supported on number/integer schemas", key)
		}
	}

	if enumValue, exists := node["enum"]; exists {
		values, ok := enumValue.([]any)
		if !ok || len(values) == 0 {
			return errors.New("enum must be a non-empty array")
		}
		budget.enumValues += len(values)
		if budget.enumValues > maxResponsesSchemaEnumValues {
			return fmt.Errorf("schema has more than %d enum values", maxResponsesSchemaEnumValues)
		}
		stringChars := 0
		for _, value := range values {
			if text, ok := value.(string); ok {
				stringChars += utf8.RuneCountInString(text)
				budget.addString(text)
			}
		}
		if len(values) > 250 && stringChars > 15000 {
			return errors.New("a string enum with more than 250 values exceeds 15000 characters")
		}
	}
	if constant, exists := node["const"]; exists {
		if text, ok := constant.(string); ok {
			budget.addString(text)
		}
	}

	if anyOfValue, exists := node["anyOf"]; exists {
		if root {
			return errors.New("root schema cannot use anyOf")
		}
		variants, ok := anyOfValue.([]any)
		if !ok || len(variants) == 0 {
			return errors.New("anyOf must be a non-empty array")
		}
		for index, rawVariant := range variants {
			variant, ok := rawVariant.(map[string]any)
			if !ok {
				return fmt.Errorf("anyOf variant %d must be a schema object", index)
			}
			if err := validateResponsesSchemaNode(variant, objectDepth, false, budget); err != nil {
				return fmt.Errorf("anyOf variant %d: %w", index, err)
			}
		}
	}

	if definitionsValue, exists := node["$defs"]; exists {
		definitions, ok := definitionsValue.(map[string]any)
		if !ok {
			return errors.New("$defs must be an object")
		}
		for name, rawDefinition := range definitions {
			budget.addString(name)
			definition, ok := rawDefinition.(map[string]any)
			if !ok {
				return fmt.Errorf("$defs entry %q must be a schema object", name)
			}
			if err := validateResponsesSchemaNode(definition, 0, false, budget); err != nil {
				return fmt.Errorf("$defs entry %q: %w", name, err)
			}
		}
	}

	if budget.stringChars > maxResponsesSchemaStringChars {
		return fmt.Errorf("schema string metadata exceeds %d characters", maxResponsesSchemaStringChars)
	}
	return nil
}

func hasResponsesSchemaType(types map[string]struct{}, typeName string) bool {
	_, ok := types[typeName]
	return ok
}

func (budget *responsesSchemaSubsetBudget) addString(value string) {
	budget.stringChars += utf8.RuneCountInString(value)
}
