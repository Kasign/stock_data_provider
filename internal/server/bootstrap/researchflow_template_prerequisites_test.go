package bootstrap

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"example.com/stock-analysis-flow/internal/researchflow/parameterschema"
	"example.com/stock-analysis-flow/internal/researchflow/template"
)

func TestNewsSearchParameterSchemaRegistryIsStrictAndImmutable(t *testing.T) {
	validator, err := newNewsSearchParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	valid := json.RawMessage(" \n{\"query\":\"market news\",\"query_dimension\":\"company\",\"limit\":20}")
	snapshot, err := validator.Validate(context.Background(), newsSearchParameterSchemaRef, valid)
	if err != nil {
		t.Fatalf("valid News Search parameters rejected: %v", err)
	}
	if snapshot.Ref != newsSearchParameterSchemaRef || snapshot.Version != newsSearchParameterSchemaVersion || len(snapshot.Fingerprint) != 64 {
		t.Fatalf("unexpected immutable schema snapshot: %+v", snapshot)
	}

	invalid := []string{
		`{"query":"market","limit":0}`,
		`{"query":"market","limit":101}`,
		`{"query":"   ","limit":10}`,
		`{"query":"market"}`,
		`{"query":12,"limit":10}`,
		`{"query":"market","limit":10.5}`,
		`{"query":"market","limit":10,"unexpected":true}`,
	}
	for _, raw := range invalid {
		if _, err := validator.Validate(context.Background(), newsSearchParameterSchemaRef, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid News Search parameters were accepted: %s", raw)
		}
	}
	if _, err := validator.Validate(context.Background(), "unknown/schema", valid); !errors.Is(err, parameterschema.ErrSchemaNotFound) {
		t.Fatalf("unknown schema ref must remain distinguishable, got %v", err)
	}

	registry := newsSearchParameterSchemaRegistry{}
	definition, err := registry.Get(context.Background(), newsSearchParameterSchemaRef)
	if err != nil {
		t.Fatal(err)
	}
	definition.Document[0] = 'x'
	again, err := registry.Get(context.Background(), newsSearchParameterSchemaRef)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(again.Document) {
		t.Fatal("mutating a returned schema definition changed the immutable registry")
	}
	documentedSchema, err := os.ReadFile("../../../doc/researchflow/schema/news-search-parameters-1.0.0.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(documentedSchema) != newsSearchParameterSchemaDocument+"\n" {
		t.Fatal("documented News Search parameter schema drifted from the Server registry definition")
	}
	golden, err := os.ReadFile("../../../doc/researchflow/golden/news-search-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), newsSearchParameterSchemaRef, golden); err != nil {
		t.Fatalf("News Search parameter Golden does not satisfy its Schema: %v", err)
	}
}

func TestNewsSearchApplicabilityRuleRequiresExactVersion(t *testing.T) {
	evaluator := newsSearchApplicabilityEvaluator{}
	parameters := json.RawMessage(`{"query":"market","limit":10}`)
	applicable, reason, err := evaluator.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: newsSearchApplicabilityRuleRef, RuleVersion: newsSearchApplicabilityVersion,
	}, parameters)
	if err != nil || !applicable || reason != "" {
		t.Fatalf("exact News Search rule should include the stage: applicable=%v reason=%q err=%v", applicable, reason, err)
	}
	_, _, err = evaluator.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: newsSearchApplicabilityRuleRef, RuleVersion: "9.9.9",
	}, parameters)
	if !errors.Is(err, ErrUnsupportedNewsSearchApplicabilityRule) {
		t.Fatalf("unknown rule version must fail closed, got %v", err)
	}
	_, _, err = evaluator.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: newsSearchApplicabilityRuleRef, RuleVersion: newsSearchApplicabilityVersion,
	}, json.RawMessage(`[]`))
	if err == nil {
		t.Fatal("non-object parameters must be rejected")
	}
}
