package bootstrap

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v5"

	quantresult "example.com/stock-analysis-flow/internal/quant/result"
)

// TestResultReadResponseMatchesDocumentedSchema 用真实 handler 响应校验
// SRV-CTR-001 1.1.0 的文档 Schema，确保契约文档不漂移（规则 4/14）。
func TestResultReadResponseMatchesDocumentedSchema(t *testing.T) {
	port := &resultReadHTTPPortStub{envelopes: []quantresult.Envelope{
		resultReadEnvelope("factor", resultReadFactorPayload(t)),
		resultReadEnvelope("risk", nil),
	}}
	handler := &researchflowHTTPHandler{resultRead: port}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/researchflow/runs/run-1/results", nil)
	request.SetPathValue("run_ref", "run-1")
	response := httptest.NewRecorder()
	handler.getResults(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", response.Code, response.Body.String())
	}

	document, err := os.ReadFile(filepath.Join("..", "..", "..",
		"doc/server/schema/SRV-CTR-001/result-read-response-1.1.0.schema.json"))
	if err != nil {
		t.Fatalf("read documented schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	const schemaURL = "https://server.invalid/result-read-response-1.1.0.schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	var payload any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if err := compiled.Validate(payload); err != nil {
		t.Fatalf("response does not match documented schema: %v\nresponse=%s", err, response.Body.String())
	}
}

// TestResultReadSchemaRejectsRiskPayload 证明文档 Schema 硬禁 risk 的 payload，
// 与 Go 投影的白名单语义双侧一致（Rule 4：实现与契约同时一致）。
func TestResultReadSchemaRejectsRiskPayload(t *testing.T) {
	document, err := os.ReadFile(filepath.Join("..", "..", "..",
		"doc/server/schema/SRV-CTR-001/result-read-response-1.1.0.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	const schemaURL = "https://server.invalid/result-read-response-1.1.0-risk.schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	bad := map[string]any{
		"schema_version": "1.1.0",
		"run_ref":        "run-1",
		"resource_kind":  "authorized_domain_results",
		"results": []any{
			map[string]any{
				"schema_version":         "1.0.0",
				"result_id":              "quant_result:sha256:aaa",
				"result_type":            "risk",
				"scope":                  "security",
				"system_as_of":           "2026-11-20T08:00:00Z",
				"quality_status":         "valid",
				"result_status":          "rejected",
				"authorization_ref":      "quant_auth:sha256:bbb",
				"payload_schema_version": "1.0.0",
				"payload":                map[string]any{"decision_id": "secret"},
			},
		},
	}
	if err := compiled.Validate(bad); err == nil {
		t.Fatal("documented schema must reject risk result with payload")
	}
}
