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
