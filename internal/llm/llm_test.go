package llm

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

func validResponsesRequest() ModelRequest {
	now := time.Now().UTC()
	return ModelRequest{
		ProviderID: "gateway-1", RequestID: "req-1", PurposeCode: "research.summary",
		Model: "qwen-local", Instructions: "Be precise", InputText: "Summarize evidence",
		MaxOutputTokens: 256, DeadlineAt: now.Add(time.Minute),
		CorrelationID: "run-1", SystemAsOf: now,
	}
}

func TestMapResponsesRequest(t *testing.T) {
	temperature, topP := 0.2, 0.8
	request := validResponsesRequest()
	request.Temperature, request.TopP, request.ReasoningEffort = &temperature, &topP, "high"
	request.ResponseSchema = &JSONSchemaSpec{
		Name: "summary", Strict: true,
		SchemaJSON: json.RawMessage(`{"type":"object","required":["summary"],"properties":{"summary":{"type":"string"}}}`),
	}
	request.Tools = []ToolDefinition{{
		Name: "lookup_quote", Description: "Look up a quote", Strict: true,
		ParametersJSON: json.RawMessage(`{"type":"object","properties":{"symbol":{"type":"string"}}}`),
	}}
	request.ToolChoice = "auto"

	body, err := MapResponsesRequest(request)
	if err != nil {
		t.Fatalf("map request: %v", err)
	}
	var got map[string]json.RawMessage
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"model", "instructions", "input", "max_output_tokens", "temperature", "top_p", "reasoning", "text", "tools", "tool_choice"} {
		if _, ok := got[key]; !ok {
			t.Fatalf("wire request missing %q: %s", key, body)
		}
	}
	for _, forbidden := range []string{"provider_id", "request_id", "correlation_id", "system_as_of", "deadline_at", "trace_id"} {
		if _, ok := got[forbidden]; ok {
			t.Fatalf("application metadata leaked into wire request: %q", forbidden)
		}
	}
	var textFormat struct {
		Format struct {
			Type string `json:"type"`
			Name string `json:"name"`
			Strict bool `json:"strict"`
		} `json:"format"`
	}
	if err := json.Unmarshal(got["text"], &textFormat); err != nil {
		t.Fatal(err)
	}
	if textFormat.Format.Type != "json_schema" || textFormat.Format.Name != "summary" || !textFormat.Format.Strict {
		t.Fatalf("unexpected text.format mapping: %+v", textFormat)
	}
}

func TestMapResponsesRequestDefaultsToolChoiceNone(t *testing.T) {
	body, err := MapResponsesRequest(validResponsesRequest())
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		ToolChoice string `json:"tool_choice"`
	}
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got.ToolChoice != "none" {
		t.Fatalf("default tool_choice = %q, want none", got.ToolChoice)
	}
}

func TestParseResponsesCompletedAndUsage(t *testing.T) {
	request := validResponsesRequest()
	body := []byte(`{
		"id":"resp-1","object":"response","model":"qwen-actual","status":"completed",
		"output":[
			{"id":"msg-1","type":"message","status":"completed","role":"assistant","content":[
				{"type":"output_text","text":"Hello "},
				{"type":"refusal","refusal":"not applicable"},
				{"type":"output_text","text":"world"}
			]},
			{"id":"reason-1","type":"reasoning","summary":[{"type":"summary_text","text":"hidden"}]}
		],
		"usage":{"input_tokens":10,"output_tokens":5,"input_tokens_details":{"cached_tokens":2},"output_tokens_details":{"reasoning_tokens":1}}
	}`)
	got, err := ParseResponsesResponse(body, request)
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if got.Status != ResponseCompleted || got.ResponseID != "resp-1" || got.ModelActual != "qwen-actual" {
		t.Fatalf("unexpected response identity/status: %+v", got)
	}
	if got.RequestID != request.RequestID || got.CorrelationID != request.CorrelationID || !got.SystemAsOf.Equal(request.SystemAsOf) {
		t.Fatalf("request correlation metadata not echoed: %+v", got)
	}
	if got.OutputText != "Hello world" || len(got.OutputItems) != 2 || got.OutputItems[1].Type != OutputItemUnknown || got.OutputItems[1].UnknownType != "reasoning" {
		t.Fatalf("output order/type/text not preserved: %+v", got.OutputItems)
	}
	if got.Usage == nil || got.Usage.TotalTokens == nil || *got.Usage.TotalTokens != 15 || got.Usage.Source != UsageSourceDerived {
		t.Fatalf("usage total should be safely derived: %+v", got.Usage)
	}
	if got.Usage.InputCachedTokens == nil || *got.Usage.InputCachedTokens != 2 ||
		got.Usage.OutputReasoningTokens == nil || *got.Usage.OutputReasoningTokens != 1 {
		t.Fatalf("usage details missing: %+v", got.Usage)
	}
}

func TestParseResponsesFunctionCallDoesNotExecute(t *testing.T) {
	request := validResponsesRequest()
	body := []byte(`{
		"id":"resp-tool","object":"response","model":"qwen-local","status":"completed",
		"output":[{"id":"fc-1","type":"function_call","call_id":"call-1","name":"lookup_quote","arguments":"{\"symbol\":\"600000\"}","status":"completed"}]
	}`)
	got, err := ParseResponsesResponse(body, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ToolCalls) != 1 || got.ToolCalls[0].Name != "lookup_quote" || got.ToolCalls[0].OutputItemID != "fc-1" {
		t.Fatalf("function call not normalized: %+v", got.ToolCalls)
	}
}

func TestParseResponsesIncompleteAndFailedAreStatuses(t *testing.T) {
	request := validResponsesRequest()
	incomplete, err := ParseResponsesResponse([]byte(`{
		"id":"resp-inc","object":"response","model":"qwen-local","status":"incomplete",
		"incomplete_details":{"reason":"max_output_tokens"},"output":[]
	}`), request)
	if err != nil || incomplete.Status != ResponseIncomplete || incomplete.IncompleteReason != "max_output_tokens" {
		t.Fatalf("incomplete must be a response status, got %+v, %v", incomplete, err)
	}
	failed, err := ParseResponsesResponse([]byte(`{
		"id":"resp-fail","object":"response","model":"qwen-local","status":"failed",
		"error":{"type":"rate_limit_error","code":"rate_limit_exceeded","message":"sensitive upstream details"},"output":[]
	}`), request)
	if err != nil || failed.Status != ResponseFailed || failed.Error == nil || failed.Error.Kind != ErrorRateLimited {
		t.Fatalf("failed must be a response status, got %+v, %v", failed, err)
	}
	encoded, _ := json.Marshal(failed)
	if strings.Contains(string(encoded), "sensitive upstream details") {
		t.Fatalf("upstream message leaked: %s", encoded)
	}
}

func TestParseResponsesSchemaValidation(t *testing.T) {
	request := validResponsesRequest()
	request.ResponseSchema = &JSONSchemaSpec{
		Name: "summary", SchemaJSON: json.RawMessage(`{"type":"object","required":["summary"],"properties":{"summary":{"type":"string"}},"additionalProperties":false}`),
	}
	body := []byte(`{"id":"resp-json","object":"response","model":"qwen-local","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"summary\":\"ok\"}"}]}]}`)
	got, err := ParseResponsesResponse(body, request)
	if err != nil || string(got.StructuredJSON) != `{"summary":"ok"}` {
		t.Fatalf("valid structured output rejected: %+v, %v", got, err)
	}
	bad := []byte(`{"id":"resp-json","object":"response","model":"qwen-local","status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"{\"summary\":7}"}]}]}`)
	got, err = ParseResponsesResponse(bad, request)
	if !errors.Is(err, ErrResponsesSchemaMismatch) || got.Error == nil || got.Error.Kind != ErrorSchemaValidationFailed {
		t.Fatalf("schema mismatch must be explicit: %+v, %v", got, err)
	}
}


func TestParseResponsesRefusalAndUnknownContentPreserveTypes(t *testing.T) {
	request := validResponsesRequest()
	body := []byte(`{
		"id":"resp-mixed","object":"response","model":"qwen-local","status":"completed",
		"output":[
			{"id":"ref-1","type":"refusal","refusal":"cannot help"},
			{"id":"msg-1","type":"message","content":[
				{"type":"future_content","opaque":"must not be retained"}
			]}
		]
	}`)
	got, err := ParseResponsesResponse(body, request)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.OutputItems) != 2 || got.OutputItems[0].Type != OutputItemRefusal ||
		got.OutputItems[1].ContentParts[0].Type != ContentPartUnknown ||
		got.OutputItems[1].ContentParts[0].UnknownType != "future_content" {
		t.Fatalf("refusal/unknown wire types were not preserved: %+v", got.OutputItems)
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "must not be retained") {
		t.Fatalf("unknown payload leaked into normalized DTO: %s", encoded)
	}
}

func TestParseResponsesUpstreamTotalTokensTakesPrecedence(t *testing.T) {
	request := validResponsesRequest()
	body := []byte(`{
		"id":"resp-usage","object":"response","model":"qwen-local","status":"completed",
		"output":[],"usage":{"input_tokens":10,"output_tokens":5,"total_tokens":99}
	}`)
	got, err := ParseResponsesResponse(body, request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Usage == nil || got.Usage.TotalTokens == nil || *got.Usage.TotalTokens != 99 ||
		got.Usage.Source != UsageSourceUpstream {
		t.Fatalf("upstream total must take precedence: %+v", got.Usage)
	}
}

func TestParseResponsesRejectsMalformedEnvelopeAndFunctionCall(t *testing.T) {
	request := validResponsesRequest()
	cases := [][]byte{
		[]byte(`{"id":"resp-1","object":"response","status":"queued","output":[]}`),
		[]byte(`{"object":"response","status":"completed","output":[]}`),
		[]byte(`{"id":"resp-1","object":"response","model":"qwen","status":"completed","output":[{"type":"function_call","name":"tool","arguments":"{}"}]}`),
	}
	for i, body := range cases {
		if _, err := ParseResponsesResponse(body, request); !errors.Is(err, ErrInvalidResponsesResponse) {
			t.Fatalf("case %d should fail as invalid response, got %v", i, err)
		}
	}
}
