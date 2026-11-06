package parameterschema

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type memoryRegistry struct { definition Definition; err error }
func (r memoryRegistry) Get(_ context.Context, _ string) (Definition, error) {
	if r.err != nil { return Definition{}, r.err }
	return r.definition, nil
}

func testDefinition() Definition {
	return Definition{
		Ref:"stock-research-params-v1", Version:"1",
		Document:json.RawMessage("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"type\":\"object\",\"properties\":{\"symbol\":{\"type\":\"string\",\"minLength\":6,\"maxLength\":6},\"lookback_days\":{\"type\":\"integer\",\"minimum\":1,\"maximum\":250}},\"required\":[\"symbol\"],\"additionalProperties\":false}"),
	}
}

func TestValidatorAcceptsTypedParametersAndReturnsImmutableSnapshot(t *testing.T) {
	validator, err := NewValidator(memoryRegistry{definition:testDefinition()})
	if err != nil { t.Fatal(err) }
	snapshot, err := validator.Validate(context.Background(), "stock-research-params-v1", json.RawMessage("{\"symbol\":\"600000\",\"lookback_days\":60}"))
	if err != nil { t.Fatal(err) }
	if snapshot.Ref != "stock-research-params-v1" || snapshot.Version != "1" || snapshot.Fingerprint != "7b82c379f72297871193074939a937b5a4d61ceab879df392d277cd623e0c2c4" {
		t.Fatalf("unexpected schema snapshot: %#v", snapshot)
	}
}

func TestValidatorRejectsMissingWrongTypeAndUnknownFields(t *testing.T) {
	validator, _ := NewValidator(memoryRegistry{definition:testDefinition()})
	cases := []string{
		"{\"lookback_days\":60}",
		"{\"symbol\":600000}",
		"{\"symbol\":\"600000\",\"unknown\":true}",
		"{\"symbol\":\"600000\",\"lookback_days\":0}",
	}
	for _, input := range cases {
		if _, err := validator.Validate(context.Background(), "stock-research-params-v1", json.RawMessage(input)); !errors.Is(err, ErrInvalidParameters) {
			t.Errorf("expected invalid parameters for %s, got %v", input, err)
		}
	}
}

func TestValidatorRejectsMalformedAndOpenRootSchema(t *testing.T) {
	for _, document := range []string{
		"{\"type\":\"object\",\"additionalProperties\":false,\"properties\":",
		"{\"type\":\"object\",\"properties\":{\"symbol\":{\"type\":\"string\"}}}",
		"{\"type\":\"array\",\"additionalProperties\":false}",
	} {
		validator, _ := NewValidator(memoryRegistry{definition:Definition{Ref:"schema-v1",Version:"1",Document:json.RawMessage(document)}})
		if _, err := validator.Validate(context.Background(), "schema-v1", json.RawMessage("{}")); !errors.Is(err, ErrInvalidSchema) {
			t.Errorf("expected invalid schema for %s, got %v", document, err)
		}
	}
}

func TestValidatorRejectsRegistryIdentityMismatch(t *testing.T) {
	validator, _ := NewValidator(memoryRegistry{definition:Definition{Ref:"other",Version:"1",Document:testDefinition().Document}})
	if _, err := validator.Validate(context.Background(), "stock-research-params-v1", json.RawMessage("{\"symbol\":\"600000\"}")); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected registry identity mismatch, got %v", err)
	}
}

func TestValidatorRejectsMutationOfImmutableRef(t *testing.T) {
	registry := mutableRegistry{definition:testDefinition()}
	validator, _ := NewValidator(&registry)
	if _, err := validator.Validate(context.Background(), "stock-research-params-v1", json.RawMessage("{\"symbol\":\"600000\"}")); err != nil { t.Fatal(err) }
	registry.definition.Version = "2"
	if _, err := validator.Validate(context.Background(), "stock-research-params-v1", json.RawMessage("{\"symbol\":\"600000\"}")); !errors.Is(err, ErrImmutableSchemaChanged) {
		t.Fatalf("expected immutable schema change rejection, got %v", err)
	}
}

type mutableRegistry struct { definition Definition }
func (r *mutableRegistry) Get(_ context.Context, _ string) (Definition, error) { return r.definition, nil }

func TestValidatorRejectsRemoteRefs(t *testing.T) {
	definition := testDefinition()
	definition.Document = json.RawMessage("{\"$schema\":\"https://json-schema.org/draft/2020-12/schema\",\"type\":\"object\",\"properties\":{\"nested\":{\"$ref\":\"https://example.com/schema.json\"}},\"additionalProperties\":false}")
	validator, _ := NewValidator(memoryRegistry{definition:definition})
	if _, err := validator.Validate(context.Background(), definition.Ref, json.RawMessage("{}")); !errors.Is(err, ErrInvalidSchema) {
		t.Fatalf("expected external ref rejection, got %v", err)
	}
}
