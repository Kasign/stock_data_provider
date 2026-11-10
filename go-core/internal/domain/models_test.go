package domain

import (
	"encoding/json"
	"testing"
)

func TestAnalysisResultPreservesUnknownPythonFields(t *testing.T) {
	input := []byte(`{"code":"600396","success":true,"dashboard":{"core_conclusion":{"one_sentence":"fixture"}},"technical_analysis":"legacy technical field","operation_advice":"观望","unknown_future_field":{"nested":true}}`)
	var result AnalysisResult
	if err := json.Unmarshal(input, &result); err != nil {
		t.Fatal(err)
	}
	result.QueryID = "go-query"
	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var actual map[string]any
	if err := json.Unmarshal(encoded, &actual); err != nil {
		t.Fatal(err)
	}
	if actual["technical_analysis"] != "legacy technical field" {
		t.Fatalf("legacy Python report field was lost: %s", encoded)
	}
	if actual["unknown_future_field"].(map[string]any)["nested"] != true {
		t.Fatalf("unknown future field was lost: %s", encoded)
	}
	if actual["query_id"] != "go-query" {
		t.Fatalf("modeled Go field was not serialized: %s", encoded)
	}
}
