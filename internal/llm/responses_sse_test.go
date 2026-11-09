package llm

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

type sseTestClock struct {
	now time.Time
}

func (c sseTestClock) Now() time.Time {
	return c.now
}

func validSSERequest() ModelRequest {
	now := time.Now().UTC()
	return ModelRequest{
		ProviderID: "gateway-1", RequestID: "req-sse-1", PurposeCode: "research.summary",
		Model: "qwen-local", InputText: "Summarize evidence", MaxOutputTokens: 128,
		DeadlineAt: now.Add(time.Minute), CorrelationID: "run-sse-1", SystemAsOf: now,
	}
}

func TestParseResponsesSSECompletedTextAndUsage(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.created",
		"data: {\"type\":\"response.created\",\"sequence_number\":10,\"response\":{\"id\":\"resp-sse-1\",\"model\":\"qwen-actual\",\"status\":\"in_progress\"}}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":11,\"response_id\":\"resp-sse-1\",\"delta\":\"hello \"}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":12,\"response_id\":\"resp-sse-1\",\"delta\":\"world\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"sequence_number\":13,\"response\":{\"id\":\"resp-sse-1\",\"object\":\"response\",\"model\":\"qwen-actual\",\"status\":\"completed\",\"output\":[{\"id\":\"msg-1\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"hello world\"}]}],\"usage\":{\"input_tokens\":8,\"output_tokens\":2,\"total_tokens\":10}}}",
		"",
	}, "\n")
	var gotEvents []ModelEvent
	got, err := ParseResponsesSSE([]byte(data), request, clock, func(event ModelEvent) error {
		gotEvents = append(gotEvents, event)
		return nil
	})
	if err != nil {
		t.Fatalf("ParseResponsesSSE failed: %v", err)
	}
	if got.Status != ResponseCompleted || got.ResponseID != "resp-sse-1" || got.ModelActual != "qwen-actual" {
		t.Fatalf("unexpected terminal response: %+v", got)
	}
	if got.OutputText != "hello world" || got.Usage == nil || got.Usage.Source != UsageSourceUpstream ||
		got.Usage.TotalTokens == nil || *got.Usage.TotalTokens != 10 {
		t.Fatalf("text/usage normalization failed: %+v", got)
	}
	if len(gotEvents) != 4 || gotEvents[1].Type != "response.output_text.delta" ||
		gotEvents[3].Type != "response.completed" {
		t.Fatalf("event order was not preserved: %+v", gotEvents)
	}
	if got.DurationMS != 0 {
		t.Fatalf("fixed clock should produce deterministic zero duration, got %d", got.DurationMS)
	}
}

func TestParseResponsesSSESequencePolicies(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	makeStream := func(sequences []string) []byte {
		var lines []string
		for i, seq := range sequences {
			lines = append(lines, "event: response.output_text.delta")
			sequence := ""
			if seq != "" {
				sequence = ",\"sequence_number\":" + seq
			}
			lines = append(lines, "data: {\"type\":\"response.output_text.delta\""+sequence+",\"delta\":\"x\"}", "")
			_ = i
		}
		terminalSequence := ""
		if len(sequences) > 0 && sequences[0] != "" {
			last, _ := strconv.ParseInt(sequences[len(sequences)-1], 10, 64)
			terminalSequence = fmt.Sprintf(",\"sequence_number\":%d", last+1)
		}
		lines = append(lines,
			"event: response.completed",
			"data: {\"type\":\"response.completed\""+terminalSequence+",\"response\":{\"id\":\"resp-seq\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
			"",
		)
		return []byte(strings.Join(lines, "\n"))
	}
	tests := []struct {
		name string
		seq  []string
		want ModelErrorKind
	}{
		{name: "contiguous", seq: []string{"10", "11"}},
		{name: "all missing", seq: []string{"", ""}},
		{name: "gap", seq: []string{"10", "12"}, want: ErrorStreamSequenceGap},
		{name: "duplicate", seq: []string{"10", "10"}, want: ErrorInvalidResponse},
		{name: "decreasing", seq: []string{"10", "9"}, want: ErrorInvalidResponse},
		{name: "mixed", seq: []string{"10", ""}, want: ErrorInvalidResponse},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseResponsesSSE(makeStream(tt.seq), request, clock, nil)
			if tt.want == "" {
				if err != nil || got.Status != ResponseCompleted {
					t.Fatalf("expected successful stream, got status=%s err=%v", got.Status, err)
				}
				if tt.name == "all missing" && got.SequenceNumberStatus != SequenceNumberUnavailable {
					t.Fatalf("all-missing sequence must be recorded unavailable: %+v", got)
				}
				if tt.name == "contiguous" && got.SequenceNumberStatus != SequenceNumberAvailable {
					t.Fatalf("contiguous sequence must be recorded available: %+v", got)
				}
				return
			}
			var adapterErr *AdapterError
			if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != tt.want || got.Status == ResponseCompleted {
				t.Fatalf("unexpected sequence result: status=%s err=%v", got.Status, err)
			}
		})
	}
}

func TestParseResponsesSSEEOFAndCallbackErrorReturnPartialResponse(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	partial := []byte("event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}\n\n")
	got, err := ParseResponsesSSE(partial, request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorStreamInterrupted ||
		got.Status != ResponseIncomplete || got.OutputText != "partial" {
		t.Fatalf("EOF without terminal must preserve partial output and fail: %+v err=%v", got, err)
	}

	callbackErr := errors.New("callback stopped")
	got, err = ParseResponsesSSE(partial, request, clock, func(ModelEvent) error { return callbackErr })
	if !errors.Is(err, callbackErr) || got.OutputText != "partial" || got.Status == ResponseCompleted {
		t.Fatalf("callback error must be returned unchanged with partial response: %+v err=%v", got, err)
	}
}

func TestParseResponsesSSEFunctionCallAggregationAndConsistency(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.output_item.added",
		"data: {\"type\":\"response.output_item.added\",\"sequence_number\":1,\"output_index\":0,\"item\":{\"id\":\"fc-1\",\"type\":\"function_call\",\"status\":\"in_progress\",\"call_id\":\"call-1\",\"name\":\"lookup_quote\",\"arguments\":\"\"}}",
		"",
		"event: response.function_call_arguments.delta",
		"data: {\"type\":\"response.function_call_arguments.delta\",\"sequence_number\":2,\"item_id\":\"fc-1\",\"call_id\":\"call-1\",\"delta\":\"{\\\"symbol\\\":\"}",
		"",
		"event: response.function_call_arguments.delta",
		"data: {\"type\":\"response.function_call_arguments.delta\",\"sequence_number\":3,\"item_id\":\"fc-1\",\"call_id\":\"call-1\",\"delta\":\"\\\"600000.SH\\\"}\"}",
		"",
		"event: response.function_call_arguments.done",
		"data: {\"type\":\"response.function_call_arguments.done\",\"sequence_number\":4,\"item_id\":\"fc-1\",\"call_id\":\"call-1\",\"arguments\":\"{\\\"symbol\\\":\\\"600000.SH\\\"}\"}",
		"",
		"event: response.output_item.done",
		"data: {\"type\":\"response.output_item.done\",\"sequence_number\":5,\"output_index\":0,\"item\":{\"id\":\"fc-1\",\"type\":\"function_call\",\"status\":\"completed\",\"call_id\":\"call-1\",\"name\":\"lookup_quote\",\"arguments\":\"{\\\"symbol\\\":\\\"600000.SH\\\"}\"}}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"sequence_number\":6,\"response\":{\"id\":\"resp-tool\",\"object\":\"response\",\"status\":\"completed\",\"output\":[{\"id\":\"fc-1\",\"type\":\"function_call\",\"status\":\"completed\",\"call_id\":\"call-1\",\"name\":\"lookup_quote\",\"arguments\":\"{\\\"symbol\\\":\\\"600000.SH\\\"}\"}]}}",
		"",
	}, "\n")
	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	if err != nil {
		t.Fatalf("valid function call stream rejected: %v", err)
	}
	if got.Status != ResponseCompleted || len(got.ToolCalls) != 1 ||
		got.ToolCalls[0].ArgumentsJSON != `{"symbol":"600000.SH"}` {
		t.Fatalf("function call aggregation failed: %+v", got.ToolCalls)
	}
}

func TestParseResponsesSSERejectsTerminalExtraEventAndArgumentMismatch(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	terminalThenExtra := []byte(strings.Join([]string{
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-end\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"late\"}",
		"",
	}, "\n"))
	if got, err := ParseResponsesSSE(terminalThenExtra, request, clock, nil); err == nil || got.Status == ResponseCompleted {
		t.Fatalf("event after terminal must invalidate stream: %+v err=%v", got, err)
	}
}

func TestParseResponsesSSEUnknownEventKeepsOnlyAllowlistedMetadata(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := []byte(strings.Join([]string{
		"event: response.future_event",
		"data: {\"type\":\"response.future_event\",\"sequence_number\":1,\"response_id\":\"resp-u\",\"status\":\"future\",\"item\":{\"id\":\"item-u\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"secret body\"}]},\"opaque\":\"must-not-survive\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"sequence_number\":2,\"response\":{\"id\":\"resp-u\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
	}, "\n"))
	var unknown ModelEvent
	got, err := ParseResponsesSSE(data, request, clock, func(event ModelEvent) error {
		if event.Type == "response.future_event" {
			unknown = event
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(unknown)
	if unknown.Type != "response.future_event" || unknown.Item == nil || unknown.Item.ItemID != "item-u" ||
		strings.Contains(string(encoded), "secret body") || strings.Contains(string(encoded), "must-not-survive") {
		t.Fatalf("unknown event metadata was not safely bounded: %s", encoded)
	}
	if got.Status != ResponseCompleted {
		t.Fatalf("terminal response not recognized: %+v", got)
	}
}

func TestParseResponsesSSETerminalIncompleteAndFailedAreResponseStates(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	cases := []struct {
		name  string
		event string
		body  string
		want  ResponseStatus
	}{
		{
			name: "incomplete", event: "response.incomplete",
			body: `{"type":"response.incomplete","response":{"id":"resp-inc","object":"response","status":"incomplete","incomplete_details":{"reason":"max_output_tokens"},"output":[]}}`,
			want: ResponseIncomplete,
		},
		{
			name: "failed", event: "response.failed",
			body: `{"type":"response.failed","response":{"id":"resp-fail","object":"response","status":"failed","error":{"type":"server_error","code":"model_failed","message":"sensitive upstream detail"},"output":[]}}`,
			want: ResponseFailed,
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			data := []byte("event: " + tt.event + "\ndata: " + tt.body + "\n\n")
			got, err := ParseResponsesSSE(data, request, clock, nil)
			if err != nil || got.Status != tt.want {
				t.Fatalf("valid non-completed terminal must be a Response state: status=%s err=%v", got.Status, err)
			}
			if tt.want == ResponseFailed && (got.Error == nil || strings.Contains(got.Error.SafeMessage, "sensitive upstream detail")) {
				t.Fatalf("failed response error must be safely normalized: %+v", got.Error)
			}
		})
	}
}

func TestParseResponsesSSEErrorEventIsUpstreamError(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := []byte("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"server_error\",\"code\":\"provider_failed\",\"message\":\"secret details\"}}\n\n")
	got, err := ParseResponsesSSE(data, request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorUpstream ||
		got.Status != ResponseFailed || strings.Contains(err.Error(), "secret details") {
		t.Fatalf("error event must map to a safe upstream error: %+v err=%v", got, err)
	}
}

func TestParseResponsesSSEEnforcesLineAndEventLimits(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	largeLine := []byte("data: " + strings.Repeat("x", MaxSSELineBytes+1) + "\n")
	got, err := ParseResponsesSSE(largeLine, request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorResourceLimit || got.Status == ResponseCompleted {
		t.Fatalf("oversized line must be rejected as resource limit: %+v err=%v", got, err)
	}
}

func TestParseResponsesSSESupportsCRLFAndMultilineData(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		": keepalive",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",",
		"data: \"response\":{\"id\":\"resp-multiline\",\"object\":\"response\",",
		"data: \"status\":\"completed\",\"output\":[]}}",
		"",
	}, "\r\n")
	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	if err != nil || got.Status != ResponseCompleted || got.ResponseID != "resp-multiline" {
		t.Fatalf("CRLF and multiline SSE data must be parsed: response=%+v err=%v", got, err)
	}
}

func TestParseResponsesSSEEnforcesEventCountLimit(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	var data strings.Builder
	for i := 0; i <= MaxSSEEvents; i++ {
		data.WriteString("event: response.future_event\n")
		data.WriteString("data: {\"type\":\"response.future_event\"}\n\n")
	}
	got, err := ParseResponsesSSE([]byte(data.String()), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorResourceLimit ||
		got.Status == ResponseCompleted {
		t.Fatalf("event count above limit must be rejected: response=%+v err=%v", got, err)
	}
}

func TestParseResponsesSSEClassifiesMultilineEventLimitAsResourceLimit(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	var data strings.Builder
	data.WriteString("event: response.future_event\n")
	data.WriteString("data: {\"type\":\"response.future_event\",\"padding\":[\n")
	for i := 0; i < 40; i++ {
		data.WriteString("data: ")
		data.WriteString(strings.Repeat("\"padding\",", 3000))
		data.WriteByte('\n')
	}
	data.WriteString("data: null]}\n\n")

	got, err := ParseResponsesSSE([]byte(data.String()), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorResourceLimit ||
		got.Status == ResponseCompleted {
		t.Fatalf("oversized multiline event must be resource_limit: response=%+v err=%v", got, err)
	}
}

func TestParseResponsesSSERejectsTotalPayloadAboveLimit(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	line := ":" + strings.Repeat("x", 60<<10) + "\n"
	data := []byte(strings.Repeat(line, MaxPayloadBytes/(60<<10)+2))

	got, err := ParseResponsesSSE(data, request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorResourceLimit ||
		adapterErr.Info.StableCode != "llm.sse.total_too_large" || got.Status == ResponseCompleted {
		t.Fatalf("oversized total payload must be resource_limit: response=%+v err=%v", got, err)
	}
}

func TestParseResponsesSSERejectsEventAfterTerminal(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-terminal-extra\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
		"event: response.future_event",
		"data: {\"type\":\"response.future_event\"}",
		"",
	}, "\n")

	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.StableCode != "llm.sse.event_after_terminal" ||
		adapterErr.Info.Kind != ErrorInvalidResponse || got.Status != ResponseFailed {
		t.Fatalf("event after terminal must reject the stream: response=%+v err=%v", got, err)
	}
}

func TestParseResponsesSSERejectsTerminalOutputTextMismatch(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"streamed text\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-text-mismatch\",\"object\":\"response\",\"status\":\"completed\",\"output\":[{\"id\":\"msg-1\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"terminal text\"}]}]}}",
		"",
	}, "\n")

	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorInvalidResponse ||
		adapterErr.Info.StableCode != "llm.sse.terminal_output_text_mismatch" ||
		got.Status != ResponseFailed {
		t.Fatalf("terminal output differing from streamed text must be rejected: response=%+v err=%v", got, err)
	}
}

// TestParseResponsesSSECallbackAdapterErrorRemainsConsumerError 验证消费者错误不会被重新归类为 Parser 错误。
func TestParseResponsesSSECallbackAdapterErrorRemainsConsumerError(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	callbackErr := &AdapterError{
		Info:  adapterError(ErrorCancelled, "consumer.cancelled", false, nil),
		cause: errors.New("consumer cancellation"),
	}

	got, err := ParseResponsesSSE(readLLMFixture(t, "sse/stream_eof_without_terminal.sse"), request, clock, func(event ModelEvent) error {
		if event.Type == "response.output_text.delta" {
			return callbackErr
		}
		return nil
	})
	if err != callbackErr {
		t.Fatalf("callback AdapterError must be returned unchanged: got=%v want=%v", err, callbackErr)
	}
	if got.Status != ResponseIncomplete || got.Error != nil || got.OutputText != "partial" {
		t.Fatalf("consumer error must preserve an incomplete partial response: %+v", got)
	}
}

// TestParseResponsesSSERejectsNegativeSequenceNumber 验证首个事件的序号也必须是非负数。
func TestParseResponsesSSERejectsNegativeSequenceNumber(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"sequence_number\":-1,\"delta\":\"x\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"sequence_number\":0,\"response\":{\"id\":\"resp-seq-negative\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
	}, "\n")

	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorInvalidResponse ||
		adapterErr.Info.StableCode != "llm.sse.invalid_sequence" || got.Status == ResponseCompleted {
		t.Fatalf("negative sequence number must be rejected: response=%+v err=%v", got, err)
	}
}

// TestParseResponsesSSERejectsResponseIDChange 验证同一条流中的响应 ID 必须保持一致。
func TestParseResponsesSSERejectsResponseIDChange(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.created",
		"data: {\"type\":\"response.created\",\"response\":{\"id\":\"resp-first\",\"model\":\"qwen-local\"}}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"response_id\":\"resp-second\",\"delta\":\"x\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-first\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
	}, "\n")

	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorInvalidResponse ||
		adapterErr.Info.StableCode != "llm.sse.response_id_mismatch" || got.Status == ResponseCompleted {
		t.Fatalf("changing response ID must reject the stream: response=%+v err=%v", got, err)
	}
}

// TestParseResponsesSSERejectsTerminalTextAfterEmptyTextEvent 验证空文本事件也必须参与终态文本一致性校验。
func TestParseResponsesSSERejectsTerminalTextAfterEmptyTextEvent(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	for _, event := range []struct {
		name string
		kind string
		body string
	}{
		{
			name: "empty delta",
			kind: "response.output_text.delta",
			body: `{"type":"response.output_text.delta","delta":""}`,
		},
		{
			name: "empty done",
			kind: "response.output_text.done",
			body: `{"type":"response.output_text.done","text":""}`,
		},
	} {
		t.Run(event.name, func(t *testing.T) {
			data := strings.Join([]string{
				"event: " + event.kind,
				"data: " + event.body,
				"",
				"event: response.completed",
				"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-empty-text\",\"object\":\"response\",\"status\":\"completed\",\"output\":[{\"id\":\"msg-1\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"nonempty terminal text\"}]}]}}",
				"",
			}, "\n")

			got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
			var adapterErr *AdapterError
			if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorInvalidResponse ||
				adapterErr.Info.StableCode != "llm.sse.terminal_output_text_mismatch" ||
				got.Status != ResponseFailed {
				t.Fatalf("terminal text after an empty text event must be rejected: response=%+v err=%v", got, err)
			}
		})
	}
}

// TestParseResponsesSSEAggregatesMultipleTextPartsByIndex 验证多个输出项的文本按索引聚合且分别校验 done。
func TestParseResponsesSSEAggregatesMultipleTextPartsByIndex(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":1,\"content_index\":0,\"delta\":\"second\"}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"first\"}",
		"",
		"event: response.output_text.done",
		"data: {\"type\":\"response.output_text.done\",\"output_index\":0,\"content_index\":0,\"text\":\"first\"}",
		"",
		"event: response.output_text.done",
		"data: {\"type\":\"response.output_text.done\",\"output_index\":1,\"content_index\":0,\"text\":\"second\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-multi-text\",\"object\":\"response\",\"status\":\"completed\",\"output\":[{\"id\":\"msg-1\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"first\"}]},{\"id\":\"msg-2\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"second\"}]}]}}",
		"",
	}, "\n")

	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	if err != nil || got.Status != ResponseCompleted || got.OutputText != "firstsecond" {
		t.Fatalf("multiple text parts must aggregate in output/content index order: response=%+v err=%v", got, err)
	}
}

// TestParseResponsesSSERejectsTextDeltaAfterDone 验证同一文本分段完成后不再接受增量。
func TestParseResponsesSSERejectsTextDeltaAfterDone(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.output_text.done",
		"data: {\"type\":\"response.output_text.done\",\"output_index\":0,\"content_index\":0,\"text\":\"final\"}",
		"",
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"late\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-text-late\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
	}, "\n")

	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.StableCode != "llm.sse.output_text_after_done" ||
		got.Status != ResponseFailed {
		t.Fatalf("text delta after done must be rejected: response=%+v err=%v", got, err)
	}
}

// TestParseResponsesSSERejectsFunctionArgumentDeltaAfterDone 验证函数参数完成后不再接受增量。
func TestParseResponsesSSERejectsFunctionArgumentDeltaAfterDone(t *testing.T) {
	request := validSSERequest()
	clock := sseTestClock{now: request.SystemAsOf.Add(time.Second)}
	data := strings.Join([]string{
		"event: response.output_item.added",
		"data: {\"type\":\"response.output_item.added\",\"output_index\":0,\"item\":{\"id\":\"fc-after-done\",\"type\":\"function_call\",\"status\":\"in_progress\",\"call_id\":\"call-after-done\",\"name\":\"lookup_quote\",\"arguments\":\"\"}}",
		"",
		"event: response.function_call_arguments.done",
		"data: {\"type\":\"response.function_call_arguments.done\",\"call_id\":\"call-after-done\",\"arguments\":\"{}\"}",
		"",
		"event: response.function_call_arguments.delta",
		"data: {\"type\":\"response.function_call_arguments.delta\",\"call_id\":\"call-after-done\",\"delta\":\"late\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-function-after-done\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
	}, "\n")

	got, err := ParseResponsesSSE([]byte(data), request, clock, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.StableCode != "llm.sse.function_arguments_after_done" ||
		got.Status != ResponseFailed {
		t.Fatalf("function argument delta after done must be rejected: response=%+v err=%v", got, err)
	}
}
