package parameterschema

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"github.com/gowebpki/jcs"
	"github.com/santhosh-tekuri/jsonschema/v5"

	"example.com/stock-analysis-flow/internal/researchflow/template"
)

var (
	ErrSchemaNotFound         = errors.New("parameter schema not found")
	ErrInvalidSchema          = errors.New("invalid parameter schema")
	ErrInvalidParameters      = errors.New("invalid template parameters")
	ErrSchemaNotFound = errors.New("parameter schema not found")
	ErrInvalidSchema = errors.New("invalid parameter schema")
	ErrInvalidParameters = errors.New("invalid template parameters")
	ErrImmutableSchemaChanged = errors.New("immutable parameter schema changed")
	Ref      string
	Version  string
)

type Definition struct {
type Registry interface {
	Get(context.Context, string) (Definition, error)
}

type Validator struct {
	registry Registry
	mu       sync.RWMutex
	mu sync.RWMutex
	compiled map[string]compiledSchema
}

	version     string
type compiledSchema struct {
	schema      *jsonschema.Schema
	version string
	fingerprint string
	schema *jsonschema.Schema
	if registry == nil {
		return nil, fmt.Errorf("%w: registry is required", ErrInvalidSchema)
	}
	return &Validator{registry: registry, compiled: make(map[string]compiledSchema)}, nil
}

func NewValidator(registry Registry) (*Validator, error) {
	if strings.TrimSpace(ref) == "" {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: schema ref is empty", ErrInvalidSchema)
	}
	if registry == nil { return nil, fmt.Errorf("%w: registry is required", ErrInvalidSchema) }
	return &Validator{registry:registry, compiled:make(map[string]compiledSchema)}, nil
}
	}
	if _, err := jcs.Transform(trimmed); err != nil {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: parameters are not canonicalizable JSON: %v", ErrInvalidParameters, err)
	}
	definition, err := v.registry.Get(ctx, ref)
	if err != nil {
		if errors.Is(err, ErrSchemaNotFound) {
			return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: %s", ErrSchemaNotFound, ref)
		}
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("load parameter schema %q: %w", ref, err)
	}
	if definition.Ref != ref || strings.TrimSpace(definition.Version) == "" {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: registry returned mismatched ref or empty version", ErrInvalidSchema)
	}
	document := bytes.TrimSpace(definition.Document)
	if len(document) == 0 || !json.Valid(document) {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: schema document must be valid JSON", ErrInvalidSchema)
	}
	canonical, err := jcs.Transform(document)
	if err != nil {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: canonicalize schema: %v", ErrInvalidSchema, err)
	}
	if err != nil { return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: canonicalize schema: %v", ErrInvalidSchema, err) }
	sum := sha256.Sum256(canonical)
	if err := validateRootSchema(document); err != nil {
		return template.ParameterSchemaSnapshot{}, err
	}
	fingerprint := hex.EncodeToString(sum[:])
	if err := validateRootSchema(document); err != nil { return template.ParameterSchemaSnapshot{}, err }

	v.mu.RUnlock()
	if exists {
		if cached.version != definition.Version || cached.fingerprint != fingerprint {
			return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: %s", ErrImmutableSchemaChanged, ref)
		}
		return v.validateInstance(ref, fingerprint, cached, trimmed)
	}

	compiled, err := compileSchema(ref, document)
	if err != nil {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: %s: %v", ErrInvalidSchema, ref, err)
	}
	if err != nil { return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: %s: %v", ErrInvalidSchema, ref, err) }
	v.mu.Lock()
	if previous, ok := v.compiled[ref]; ok {
			v.mu.Unlock()
			return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: %s", ErrImmutableSchemaChanged, ref)
		}
		compiled = previous
	} else {
		v.compiled[ref] = compiledSchema{version: definition.Version, fingerprint: fingerprint, schema: compiled}
		v.compiled[ref] = compiledSchema{version:definition.Version, fingerprint:fingerprint, schema:compiled}
	}
	v.mu.Unlock()
}

func (v *Validator) validateInstance(ref, fingerprint string, compiled compiledSchema, parameters []byte) (template.ParameterSchemaSnapshot, error) {
	decoder := json.NewDecoder(bytes.NewReader(parameters))
	decoder.UseNumber()
	var instance any
	if err := decoder.Decode(&instance); err != nil {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: decode parameters: %v", ErrInvalidParameters, err)
	}
	if err := compiled.schema.Validate(instance); err != nil {
		return template.ParameterSchemaSnapshot{}, fmt.Errorf("%w: %v", ErrInvalidParameters, err)
	}
	return template.ParameterSchemaSnapshot{Ref: ref, Version: compiled.version, Fingerprint: fingerprint}, nil
	return template.ParameterSchemaSnapshot{Ref:ref, Version:compiled.version, Fingerprint:fingerprint}, nil
}

	schemaURL := "https://researchflow.invalid/parameter-schemas/" + url.PathEscape(ref)
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	compiler.LoadURL = func(uri string) (io.ReadCloser, error) {
		return nil, fmt.Errorf("external schema reference is disabled: %s", uri)
	}
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		return nil, err
	}
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil { return nil, err }
	return compiler.Compile(schemaURL)
}

func validateRootSchema(document []byte) error {
	if err := json.Unmarshal(document, &root); err != nil {
		return fmt.Errorf("%w: decode schema root: %v", ErrInvalidSchema, err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(document, &root); err != nil { return fmt.Errorf("%w: decode schema root: %v", ErrInvalidSchema, err) }
	var rootType string
	}
	var additionalProperties bool
	if err := json.Unmarshal(root["additionalProperties"], &additionalProperties); err != nil || additionalProperties {
		return fmt.Errorf("%w: root additionalProperties must be false", ErrInvalidSchema)
	}
	return nil
}
