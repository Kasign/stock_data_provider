package toolpolicy

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/llm"
)

func TestIntentFromModelToolCallOnlyAdaptsCompletedStructuredIntent(t *testing.T) {
	call := llm.ModelToolCall{
		CallID: "call-1", Name: "research.lookup",
		ArgumentsJSON: `{"symbol":"600000.SH"}`, Status: "completed",
	}
	intent, err := IntentFromModelToolCall(call)
	if err != nil {
		t.Fatal(err)
	}
	if intent.CallID != call.CallID || intent.Name != call.Name || intent.ArgumentsJSON != call.ArgumentsJSON {
		t.Fatalf("unexpected adapted intent: %+v", intent)
	}
	call.Status = "in_progress"
	if _, err := IntentFromModelToolCall(call); !errors.Is(err, ErrInvalidModelToolCall) {
		t.Fatalf("non-terminal tool call error = %v", err)
	}
}

func TestIntentsFromModelResponseRequiresCompletedResponseAndUniqueCallIDs(t *testing.T) {
	response := llm.ModelResponse{
		Status: llm.ResponseCompleted,
		ToolCalls: []llm.ModelToolCall{
			{CallID: "call-1", Name: "research.lookup", ArgumentsJSON: `{"symbol":"600000.SH"}`},
			{CallID: "call-2", Name: "research.lookup", ArgumentsJSON: `{"symbol":"000001.SZ"}`},
		},
	}
	intents, err := IntentsFromModelResponse(response)
	if err != nil || len(intents) != 2 {
		t.Fatalf("intents=%+v err=%v", intents, err)
	}
	response.Status = llm.ResponseIncomplete
	if _, err := IntentsFromModelResponse(response); !errors.Is(err, ErrInvalidModelToolCall) {
		t.Fatalf("incomplete response error = %v", err)
	}
	response.Status = llm.ResponseCompleted
	response.ToolCalls[1].CallID = response.ToolCalls[0].CallID
	if _, err := IntentsFromModelResponse(response); !errors.Is(err, ErrInvalidModelToolCall) {
		t.Fatalf("duplicate call ID error = %v", err)
	}
}

func TestNewLLMRegistryUsesBoundedLocalSchemaValidation(t *testing.T) {
	registry, err := NewLLMRegistry([]Definition{{
		Name:           "research.lookup",
		ParametersJSON: []byte(`{"type":"object","required":["symbol"],"properties":{"symbol":{"type":"string"}},"additionalProperties":false}`),
		Timeout:        time.Second, MaxCallsPerRun: 1,
	}})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	budget := Budget{
		MaxCalls: 1, MaxTotalTimeout: time.Second, RunDeadline: now.Add(time.Minute),
		Now: now, IdempotencyKey: "run/stage/call",
	}
	for _, args := range []string{`{"symbol":12}`, `{"symbol":"600000.SH","unexpected":true}`} {
		_, err := registry.Authorize(Intent{CallID: "call-1", Name: "research.lookup", ArgumentsJSON: args}, budget)
		if !errors.Is(err, ErrInvalidToolIntent) {
			t.Fatalf("invalid arguments %s error = %v", args, err)
		}
	}
}
