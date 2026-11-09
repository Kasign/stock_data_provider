package llm

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func readLLMFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", path))
	if err != nil {
		t.Fatalf("read fixture %s: %v", path, err)
	}
	return data
}

func normalizeGoldenResponse(response ModelResponse) ModelResponse {
	response.RequestID = ""
	response.CorrelationID = ""
	response.SystemAsOf = time.Time{}
	response.ProviderID = ""
	response.ModelRequested = ""
	response.StartedAt = time.Time{}
	response.FinishedAt = time.Time{}
	response.DurationMS = 0
	return response
}

func assertResponseGolden(t *testing.T, got ModelResponse, expectedPath string) {
	t.Helper()
	var want ModelResponse
	if err := json.Unmarshal(readLLMFixture(t, expectedPath), &want); err != nil {
		t.Fatalf("decode expected DTO %s: %v", expectedPath, err)
	}
	got = normalizeGoldenResponse(got)
	if !reflect.DeepEqual(got, want) {
		gotJSON, _ := json.MarshalIndent(got, "", "  ")
		wantJSON, _ := json.MarshalIndent(want, "", "  ")
		t.Fatalf("DTO does not match %s\n got: %s\nwant: %s", expectedPath, gotJSON, wantJSON)
	}
}

func TestParseResponsesJSONGoldenFixtures(t *testing.T) {
	request := validResponsesRequest()
	cases := []struct {
		name string
		expected string
	}{
		{name: "response_completed_text.json", expected: "json/response_completed_text.expected.json"},
		{name: "response_completed_function_call.json", expected: "json/response_completed_function_call.expected.json"},
		{name: "response_incomplete_max_output_tokens.json", expected: "json/response_incomplete_max_output_tokens.expected.json"},
		{name: "response_usage_inconsistent_total.json", expected: "json/response_usage_inconsistent_total.expected.json"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResponsesResponse(readLLMFixture(t, "json/"+tt.name), request)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			assertResponseGolden(t, got, tt.expected)
		})
	}
}

func TestParseResponsesSSEGoldenFixtures(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	cases := []struct {
		name string
		expected string
	}{
		{name: "stream_completed_text.sse", expected: "sse/stream_completed_text.expected.json"},
		{name: "stream_function_call.sse", expected: "sse/stream_function_call.expected.json"},
		{name: "stream_incomplete.sse", expected: "sse/stream_incomplete.expected.json"},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResponsesSSE(readLLMFixture(t, "sse/"+tt.name), request, clock, nil)
			if err != nil {
				t.Fatalf("parse fixture: %v", err)
			}
			assertResponseGolden(t, got, tt.expected)
		})
	}
}

func TestParseResponsesSSEGoldenErrorFixtures(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	t.Run("sequence gap", func(t *testing.T) {
		got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_sequence_gap.sse"), request, clock, nil)
		var adapterErr *AdapterError
		if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorStreamSequenceGap || got.Status == ResponseCompleted {
			t.Fatalf("expected stream_sequence_gap, got response=%+v err=%v", got, err)
		}
	})
	t.Run("EOF without terminal", func(t *testing.T) {
		got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_eof_without_terminal.sse"), request, clock, nil)
		var adapterErr *AdapterError
		if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorStreamInterrupted ||
			got.Status != ResponseIncomplete || got.OutputText != "partial" {
			t.Fatalf("expected partial stream_interrupted response, got response=%+v err=%v", got, err)
		}
	})
	t.Run("unknown event allowlist", func(t *testing.T) {
		var unknown ModelEvent
		got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_unknown_event.sse"), request, clock, func(event ModelEvent) error {
			if event.Type == "response.future_event" {
				unknown = event
			}
			return nil
		})
		if err != nil || got.Status != ResponseCompleted {
			t.Fatalf("unknown event must not prevent later terminal response: response=%+v err=%v", got, err)
		}
		unknown.ReceivedAt = time.Time{}
		encoded, _ := json.Marshal(unknown)
		if unknown.Type != "response.future_event" || unknown.Item == nil ||
			unknown.Item.ItemID != "item_unknown_001" ||
			strings.Contains(string(encoded), "must-not-survive") {
			t.Fatalf("unknown event retained unsafe payload: %s", encoded)
		}
	})
	t.Run("terminal then extra event", func(t *testing.T) {
		got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_terminal_then_extra_event.sse"), request, clock, nil)
		var adapterErr *AdapterError
		if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorInvalidResponse || got.Status == ResponseCompleted {
			t.Fatalf("extra event after terminal must invalidate stream: response=%+v err=%v", got, err)
		}
	})
	t.Run("error event", func(t *testing.T) {
		got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_error_event.sse"), request, clock, nil)
		var adapterErr *AdapterError
		if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorUpstream ||
			got.Status != ResponseFailed || strings.Contains(err.Error(), "sensitive upstream detail") {
			t.Fatalf("error event must return sanitized upstream_error: response=%+v err=%v", got, err)
		}
	})
}

func TestMapResponsesRequestGoldenFixture(t *testing.T) {
	got, err := MapResponsesRequest(validResponsesRequest())
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal(got, &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(readLLMFixture(t, "json/request_minimal_text.expected.json"), &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("request mapping differs from golden: got=%s want=%s", got, readLLMFixture(t, "json/request_minimal_text.expected.json"))
	}
}

func TestParseResponsesJSONCoverageFixtures(t *testing.T) {
	request := validResponsesRequest()
	t.Run("refusal remains typed", func(t *testing.T) {
		got, err := ParseResponsesResponse(readLLMFixture(t, "json/response_completed_refusal.json"), request)
		if err != nil || got.Status != ResponseCompleted || len(got.OutputItems) != 1 ||
			len(got.OutputItems[0].ContentParts) != 1 || got.OutputItems[0].ContentParts[0].Type != ContentPartRefusal ||
			got.OutputItems[0].ContentParts[0].Text != "I cannot assist with that request." || got.OutputText != "" {
			t.Fatalf("refusal must not be merged into output text: response=%+v err=%v", got, err)
		}
	})
	t.Run("unknown output item is bounded", func(t *testing.T) {
		got, err := ParseResponsesResponse(readLLMFixture(t, "json/response_unknown_output_item.json"), request)
		encoded, _ := json.Marshal(got)
		if err != nil || len(got.OutputItems) != 1 || got.OutputItems[0].Type != OutputItemUnknown ||
			got.OutputItems[0].UnknownType != "future_item" || strings.Contains(string(encoded), "must-not-survive") {
			t.Fatalf("unknown output item retained unbounded content: response=%+v err=%v json=%s", got, err, encoded)
		}
	})
	t.Run("failed response is a status and sanitized error", func(t *testing.T) {
		got, err := ParseResponsesResponse(readLLMFixture(t, "json/response_failed_upstream.json"), request)
		if err != nil || got.Status != ResponseFailed || got.Error == nil ||
			strings.Contains(got.Error.SafeMessage, "sensitive upstream detail") {
			t.Fatalf("failed response must remain a safe response status: response=%+v err=%v", got, err)
		}
	})
	t.Run("usage missing", func(t *testing.T) {
		got, err := ParseResponsesResponse(readLLMFixture(t, "json/response_usage_missing.json"), request)
		if err != nil || got.Usage != nil {
			t.Fatalf("missing usage must remain unknown: usage=%+v err=%v", got.Usage, err)
		}
	})
	t.Run("usage partial", func(t *testing.T) {
		got, err := ParseResponsesResponse(readLLMFixture(t, "json/response_usage_partial.json"), request)
		if err != nil || got.Usage == nil || got.Usage.InputTokens == nil ||
			got.Usage.OutputTokens != nil || got.Usage.TotalTokens != nil {
			t.Fatalf("partial usage must not synthesize missing fields: usage=%+v err=%v", got.Usage, err)
		}
	})
	t.Run("usage derived total", func(t *testing.T) {
		got, err := ParseResponsesResponse(readLLMFixture(t, "json/response_usage_derived_total.json"), request)
		if err != nil || got.Usage == nil || got.Usage.TotalTokens == nil ||
			*got.Usage.TotalTokens != 9 || got.Usage.Source != UsageSourceDerived {
			t.Fatalf("complete input/output counts should derive total: usage=%+v err=%v", got.Usage, err)
		}
	})
	t.Run("usage overflow", func(t *testing.T) {
		got, err := ParseResponsesResponse(readLLMFixture(t, "json/response_usage_overflow.json"), request)
		if err != nil || got.Usage == nil || got.Usage.TotalTokens != nil ||
			!containsString(got.Usage.Diagnostics, "total_tokens_overflow") {
			t.Fatalf("overflow must not derive total: usage=%+v err=%v", got.Usage, err)
		}
	})
	for _, fixture := range []string{"response_missing_status.json", "response_completed_missing_id.json"} {
		t.Run(fixture, func(t *testing.T) {
			got, err := ParseResponsesResponse(readLLMFixture(t, "json/"+fixture), request)
			if err == nil || got.Status == ResponseCompleted {
				t.Fatalf("malformed response envelope must not succeed: response=%+v err=%v", got, err)
			}
		})
	}
}

func TestParseResponsesSSECoverageFixtures(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	t.Run("failed terminal remains a response status", func(t *testing.T) {
		got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_failed.sse"), request, clock, nil)
		if err != nil || got.Status != ResponseFailed || got.Error == nil ||
			strings.Contains(got.Error.SafeMessage, "sensitive upstream detail") {
			t.Fatalf("failed SSE terminal must remain a sanitized response status: response=%+v err=%v", got, err)
		}
	})
	t.Run("all sequence numbers missing", func(t *testing.T) {
		got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_sequence_all_missing.sse"), request, clock, nil)
		if err != nil || got.Status != ResponseCompleted || got.SequenceNumberStatus != SequenceNumberUnavailable {
			t.Fatalf("all-missing sequence stream must be accepted and marked unavailable: response=%+v err=%v", got, err)
		}
	})
	for _, fixture := range []string{"stream_sequence_duplicate.sse", "stream_sequence_decreasing.sse", "stream_sequence_mixed_missing.sse"} {
		t.Run(fixture, func(t *testing.T) {
			got, err := ParseResponsesSSE(readLLMFixture(t, "sse/"+fixture), request, clock, nil)
			var adapterErr *AdapterError
			if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorInvalidResponse || got.Status == ResponseCompleted {
				t.Fatalf("invalid sequence stream must not complete: response=%+v err=%v", got, err)
			}
		})
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestParseResponsesSSECallbackErrorReturnsPartialResponse(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	callbackErr := errors.New("consumer stopped")
	var callbackCount int

	got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_eof_without_terminal.sse"), request, clock, func(event ModelEvent) error {
		callbackCount++
		if event.Type == "response.output_text.delta" {
			return callbackErr
		}
		return nil
	})
	if err != callbackErr {
		t.Fatalf("callback error must be returned unchanged: got=%v want=%v", err, callbackErr)
	}
	if callbackCount != 2 || got.Status != ResponseIncomplete || got.OutputText != "partial" || got.Error != nil {
		t.Fatalf("callback interruption must preserve partial output without claiming completion: callbacks=%d response=%+v", callbackCount, got)
	}
	if got.FinishedAt.IsZero() || got.DurationMS < 0 {
		t.Fatalf("interrupted response timing metadata missing: %+v", got)
	}
}
