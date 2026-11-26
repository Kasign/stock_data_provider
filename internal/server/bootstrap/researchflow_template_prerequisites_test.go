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

func TestQuantSMA20ParameterSchemaIsStrictImmutableAndDocumented(t *testing.T) {
	validator, err := newQuantSMA20ParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	valid := json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`)
	snapshot, err := validator.Validate(context.Background(), quantSMA20ParameterSchemaRef, valid)
	if err != nil {
		t.Fatalf("valid Quant SMA20 parameters rejected: %v", err)
	}
	if snapshot.Ref != quantSMA20ParameterSchemaRef ||
		snapshot.Version != quantSMA20ParameterSchemaVersion || len(snapshot.Fingerprint) != 64 {
		t.Fatalf("unexpected Quant SMA20 parameter Schema snapshot: %+v", snapshot)
	}
	invalid := []string{
		`{"symbol":"600036","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`,
		`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":19}`,
		`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":1001}`,
		`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20,"extra":true}`,
	}
	for _, raw := range invalid {
		if _, err := validator.Validate(context.Background(), quantSMA20ParameterSchemaRef, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid Quant SMA20 parameters were accepted: %s", raw)
		}
	}
	documented, err := os.ReadFile("../../../doc/researchflow/schema/quant-sma20-parameters-1.0.0.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(documented) != quantSMA20ParameterSchemaDocument+"\n" {
		t.Fatal("documented Quant SMA20 parameter Schema drifted from Server registry")
	}
	golden, err := os.ReadFile("../../../doc/researchflow/golden/quant-sma20-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), quantSMA20ParameterSchemaRef, golden); err != nil {
		t.Fatalf("Quant SMA20 parameter Golden does not satisfy its Schema: %v", err)
	}
}

func TestResearchflowCompositeSchemaAndApplicabilityRegistrySupportsNewsAndQuant(t *testing.T) {
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), newsSearchParameterSchemaRef, json.RawMessage(`{"query":"market","limit":10}`)); err != nil {
		t.Fatalf("composite registry rejected News parameters: %v", err)
	}
	quantParameters := json.RawMessage(`{"symbol":"600036.SH","start_date":"2026-10-01","end_date":"2026-10-30","limit":20}`)
	if _, err := validator.Validate(context.Background(), quantSMA20ParameterSchemaRef, quantParameters); err != nil {
		t.Fatalf("composite registry rejected Quant parameters: %v", err)
	}
	evaluator := researchflowApplicabilityEvaluator{}
	for _, rule := range []template.ApplicabilityRule{
		{RuleRef: newsSearchApplicabilityRuleRef, RuleVersion: newsSearchApplicabilityVersion},
		{RuleRef: quantSMA20ApplicabilityRuleRef, RuleVersion: quantSMA20ApplicabilityVersion},
	} {
		parameters := json.RawMessage(`{"query":"market","limit":10}`)
		if rule.RuleRef == quantSMA20ApplicabilityRuleRef {
			parameters = quantParameters
		}
		applicable, reason, err := evaluator.Evaluate(context.Background(), rule, parameters)
		if err != nil || !applicable || reason != "" {
			t.Fatalf("composite applicability rejected %s: applicable=%v reason=%q err=%v", rule.RuleRef, applicable, reason, err)
		}
	}
}

func TestNewsFeedParameterSchemaIsStrictImmutableAndDocumented(t *testing.T) {
	validator, err := newResearchflowParameterSchemaValidator()
	if err != nil {
		t.Fatal(err)
	}
	valid := json.RawMessage("{}")
	snapshot, err := validator.Validate(context.Background(), newsFeedParameterSchemaRef, valid)
	if err != nil {
		t.Fatalf("empty News Feed parameters rejected: %v", err)
	}
	if snapshot.Ref != newsFeedParameterSchemaRef || snapshot.Version != newsFeedParameterSchemaVersion || len(snapshot.Fingerprint) != 64 {
		t.Fatalf("unexpected News Feed parameter Schema snapshot: %+v", snapshot)
	}
	for _, raw := range []string{"[]", "null", "{\"source_id\":\"source-a\"}", "{\"unexpected\":true}"} {
		if _, err := validator.Validate(context.Background(), newsFeedParameterSchemaRef, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid News Feed parameters were accepted: %s", raw)
		}
	}

	registry := researchflowParameterSchemaRegistry{}
	definition, err := registry.Get(context.Background(), newsFeedParameterSchemaRef)
	if err != nil {
		t.Fatal(err)
	}
	definition.Document[0] = 'x'
	again, err := registry.Get(context.Background(), newsFeedParameterSchemaRef)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(again.Document) {
		t.Fatal("mutating a returned News Feed schema definition changed the immutable registry")
	}
	documented, err := os.ReadFile("../../../doc/researchflow/schema/news-feed-collection-parameters-1.0.0.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(documented) != newsFeedParameterSchemaDocument+"\n" {
		t.Fatal("documented News Feed parameter Schema drifted from Server registry")
	}
	golden, err := os.ReadFile("../../../doc/researchflow/golden/news-feed-collection-parameters-1.0.0.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := validator.Validate(context.Background(), newsFeedParameterSchemaRef, golden); err != nil {
		t.Fatalf("News Feed parameter Golden does not satisfy its Schema: %v", err)
	}
}

func TestNewsFeedApplicabilityRuleRequiresExactVersionAndEmptyParameters(t *testing.T) {
	evaluator := newsFeedApplicabilityEvaluator{}
	rule := template.ApplicabilityRule{RuleRef: newsFeedApplicabilityRuleRef, RuleVersion: newsFeedApplicabilityVersion}
	applicable, reason, err := evaluator.Evaluate(context.Background(), rule, json.RawMessage("{}"))
	if err != nil || !applicable || reason != "" {
		t.Fatalf("valid News Feed rule should include Stage: applicable=%v reason=%q err=%v", applicable, reason, err)
	}
	if _, _, err := evaluator.Evaluate(context.Background(), template.ApplicabilityRule{
		RuleRef: newsFeedApplicabilityRuleRef, RuleVersion: "9.9.9",
	}, json.RawMessage("{}")); err == nil {
		t.Fatal("unknown News Feed applicability version was accepted")
	}
	if _, _, err := evaluator.Evaluate(context.Background(), rule, json.RawMessage("{\"source_id\":\"source-a\"}")); err == nil {
		t.Fatal("News Feed applicability accepted a caller-selected source")
	}
}
