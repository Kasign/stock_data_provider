package llm

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

const (
	MaxSSELineBytes  = 64 << 10
	MaxSSEEventBytes = 1 << 20
	MaxSSEEvents     = 10000
)

var ErrInvalidResponsesSSE = errors.New("invalid Responses SSE stream")

type responsesSSEWireEvent struct {
	Type           string          `json:"type"`
	SequenceNumber *int64          `json:"sequence_number"`
	ResponseID     string          `json:"response_id"`
	OutputIndex    *int            `json:"output_index"`
	ContentIndex   *int            `json:"content_index"`
	ItemID         string          `json:"item_id"`
	CallID         string          `json:"call_id"`
	Status         string          `json:"status"`
	Delta          *string         `json:"delta"`
	Text           *string         `json:"text"`
	Refusal        *string         `json:"refusal"`
	Arguments      *string         `json:"arguments"`
	Response       json.RawMessage `json:"response"`
	Item           json.RawMessage `json:"item"`
	ContentPart    json.RawMessage `json:"content_part"`
	Usage          *responsesUsage `json:"usage"`
	Error          *responsesError `json:"error"`
}

// ParseResponsesSSE parses a complete, bounded SSE byte fixture. It does not
// own a network connection or perform streaming HTTP reads. A future Stream
// transport can feed bounded events through the same normalization rules once
// Platform exposes a controlled streaming transport.
func ParseResponsesSSE(data []byte, request ModelRequest, clock Clock, onEvent func(ModelEvent) error) (ModelResponse, error) {
	if clock == nil {
		return ModelResponse{}, errors.New("LLM clock is required")
	}
	startedAt := clock.Now().UTC()
	result := ModelResponse{
		RequestID: request.RequestID, CorrelationID: request.CorrelationID,
		SystemAsOf: request.SystemAsOf, ProviderID: request.ProviderID,
		ModelRequested: request.Model, Status: ResponseIncomplete,
		OutputItems: []OutputItem{}, StartedAt: startedAt,
	}
	finish := func() {
		result.FinishedAt = clock.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(startedAt).Milliseconds()
	}
	fail := func(info *ModelErrorInfo, cause error) (ModelResponse, error) {
		result.Status = ResponseFailed
		result.Error = info
		finish()
		return result, &AdapterError{Info: info, cause: cause}
	}
	interrupted := func(info *ModelErrorInfo, cause error) (ModelResponse, error) {
		result.Status = ResponseIncomplete
		result.Error = info
		finish()
		return result, &AdapterError{Info: info, cause: cause}
	}
	if err := request.ValidateAt(startedAt); err != nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.sse.invalid_request", false, nil), err)
	}
	if len(data) == 0 {
		return interrupted(streamInterruptedError(), io.EOF)
	}
	if len(data) > MaxPayloadBytes {
		return fail(adapterError(ErrorResourceLimit, "llm.sse.total_too_large", false, nil), ErrInvalidResponsesSSE)
	}

	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), MaxSSELineBytes)
	eventName := ""
	eventNameSet := false
	dataLines := make([]string, 0, 1)
	eventBytes := 0
	eventCount := 0
	terminal := false
	sequenceMode := 0 // 1 = all events carry sequence_number; 2 = none do.
	var previousSequence int64
	hasPreviousSequence := false
	var parseErr error
	var terminalErr error
	functionArguments := make(map[string]string)

	resetEvent := func() {
		eventName = ""
		eventNameSet = false
		dataLines = dataLines[:0]
		eventBytes = 0
	}
	dispatch := func() {
		if parseErr != nil || len(dataLines) == 0 {
			resetEvent()
			return
		}
		if terminal {
			parseErr = invalidSSEError("llm.sse.event_after_terminal")
			resetEvent()
			return
		}
		eventCount++
		if eventCount > MaxSSEEvents {
			parseErr = newSSEAdapterError(adapterError(ErrorResourceLimit, "llm.sse.event_count_exceeded", false, nil))
			resetEvent()
			return
		}
		rawData := []byte(strings.Join(dataLines, "\n"))
		if len(rawData) > MaxSSEEventBytes {
			parseErr = newSSEAdapterError(adapterError(ErrorResourceLimit, "llm.sse.event_too_large", false, nil))
			resetEvent()
			return
		}
		if len(rawData) == 0 || !json.Valid(rawData) {
			parseErr = invalidSSEError("llm.sse.invalid_event_data")
			resetEvent()
			return
		}
		var wire responsesSSEWireEvent
		if err := json.Unmarshal(rawData, &wire); err != nil || safeStableCode(wire.Type) == "" {
			parseErr = invalidSSEError("llm.sse.invalid_event_envelope")
			resetEvent()
			return
		}
		if eventNameSet && eventName != wire.Type {
			parseErr = invalidSSEError("llm.sse.event_type_mismatch")
			resetEvent()
			return
		}
		if wire.SequenceNumber == nil {
			if sequenceMode == 1 {
				parseErr = invalidSSEError("llm.sse.mixed_sequence_presence")
				resetEvent()
				return
			}
			sequenceMode = 2
		} else {
			if sequenceMode == 2 {
				parseErr = invalidSSEError("llm.sse.mixed_sequence_presence")
				resetEvent()
				return
			}
			sequenceMode = 1
			if hasPreviousSequence {
				if previousSequence == int64(^uint64(0)>>1) || *wire.SequenceNumber <= previousSequence {
					parseErr = invalidSSEError("llm.sse.invalid_sequence")
					resetEvent()
					return
				}
				if *wire.SequenceNumber != previousSequence+1 {
					parseErr = newSSEAdapterError(adapterError(ErrorStreamSequenceGap, "llm.sse.sequence_gap", false, nil))
					resetEvent()
					return
				}
			}
			previousSequence = *wire.SequenceNumber
			hasPreviousSequence = true
		}

		event, isTerminal, eventErr := normalizeResponsesSSEEvent(wire, request, clock.Now().UTC())
		if eventErr != nil {
			parseErr = eventErr
			resetEvent()
			return
		}
		if event.ResponseID != "" {
			result.ResponseID = event.ResponseID
		}
		if event.Type == "response.output_text.delta" {
			result.OutputText += event.Delta
		}
		if event.Type == "response.output_text.done" {
			if result.OutputText != "" && result.OutputText != event.Delta {
				parseErr = invalidSSEError("llm.sse.output_text_mismatch")
				resetEvent()
				return
			}
			result.OutputText = event.Delta
		}
		if event.Usage != nil {
			result.Usage = event.Usage
		}
		if event.Item != nil {
			if event.Type == "response.output_item.done" && event.Item.ToolCall != nil {
				if accumulated, ok := functionArguments[event.Item.ToolCall.CallID]; ok &&
					accumulated != event.Item.ToolCall.ArgumentsJSON {
					parseErr = invalidSSEError("llm.sse.output_item_arguments_mismatch")
					resetEvent()
					return
				}
			}
			upsertSSEOutputItem(&result, *event.Item)
		}
		if event.Type == "response.function_call_arguments.delta" && event.CallID != "" {
			functionArguments[event.CallID] += event.Delta
			if len(functionArguments[event.CallID]) > MaxSSEEventBytes {
				parseErr = newSSEAdapterError(adapterError(ErrorResourceLimit, "llm.sse.function_arguments_too_large", false, nil))
				resetEvent()
				return
			}
			for i := range result.ToolCalls {
				if result.ToolCalls[i].CallID == event.CallID {
					result.ToolCalls[i].ArgumentsJSON += event.Delta
				}
			}
			for i := range result.OutputItems {
				if result.OutputItems[i].ToolCall != nil && result.OutputItems[i].ToolCall.CallID == event.CallID {
					result.OutputItems[i].ToolCall.ArgumentsJSON += event.Delta
				}
			}
		}
		if event.Type == "response.function_call_arguments.done" && event.CallID != "" {
			if previous, ok := functionArguments[event.CallID]; ok && previous != event.Arguments {
				parseErr = invalidSSEError("llm.sse.function_arguments_mismatch")
				resetEvent()
				return
			}
			functionArguments[event.CallID] = event.Arguments
			for i := range result.ToolCalls {
				if result.ToolCalls[i].CallID == event.CallID {
					result.ToolCalls[i].ArgumentsJSON = event.Arguments
				}
			}
			for i := range result.OutputItems {
				if result.OutputItems[i].ToolCall != nil && result.OutputItems[i].ToolCall.CallID == event.CallID {
					result.OutputItems[i].ToolCall.ArgumentsJSON = event.Arguments
				}
			}
		}
		if isTerminal {
			if event.Type == "response.completed" || event.Type == "response.incomplete" || event.Type == "response.failed" {
				for _, call := range terminalToolCalls(wire.Response, request) {
					if accumulated, ok := functionArguments[call.CallID]; ok && accumulated != call.ArgumentsJSON {
						parseErr = invalidSSEError("llm.sse.terminal_arguments_mismatch")
						resetEvent()
						return
					}
				}
			}
			terminal = true
			if event.Type == "error" {
				terminalErr = &AdapterError{Info: event.Error, cause: ErrInvalidResponsesSSE}
				result.Status = ResponseFailed
				result.Error = event.Error
			} else {
				var terminalParseErr error
				result, terminalParseErr = terminalResponseFromEvent(wire, request, startedAt, result)
				if terminalParseErr != nil {
					parseErr = terminalParseErr
					resetEvent()
					return
				}
			}
		}
		if onEvent != nil {
			if err := onEvent(event); err != nil {
				result.Status = ResponseIncomplete
				result.Error = nil
				terminal = false
				terminalErr = err
				parseErr = err
			}
		}
		resetEvent()
	}

	for scanner.Scan() {
		line := strings.TrimSuffix(scanner.Text(), "\r")
		if line == "" {
			dispatch()
			if parseErr != nil {
				break
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		field, value, found := strings.Cut(line, ":")
		if !found {
			value = ""
		} else {
			value = strings.TrimPrefix(value, " ")
		}
		switch field {
		case "event":
			eventName, eventNameSet = value, true
		case "data":
			if len(dataLines) > 0 {
				eventBytes++ // SSE 多条 data 行拼接时会插入一个换行符。
			}
			eventBytes += len(value)
			if eventBytes > MaxSSEEventBytes {
				parseErr = newSSEAdapterError(adapterError(ErrorResourceLimit, "llm.sse.event_too_large", false, nil))
				break
			}
			dataLines = append(dataLines, value)
		case "id", "retry":
			// Transport cursor/retry fields are not part of ModelEvent.
		default:
			// SSE extension fields are ignored by the protocol contract.
		}
		if parseErr != nil {
			break
		}
	}
	if parseErr == nil && scanner.Err() != nil {
		if strings.Contains(strings.ToLower(scanner.Err().Error()), "token too long") {
			parseErr = newSSEAdapterError(adapterError(ErrorResourceLimit, "llm.sse.line_too_large", false, nil))
		} else {
			parseErr = invalidSSEError("llm.sse.read_failed")
		}
	}
	if parseErr == nil {
		dispatch()
		parseErr = terminalErr
	}
	switch sequenceMode {
	case 1:
		result.SequenceNumberStatus = SequenceNumberAvailable
	case 2:
		result.SequenceNumberStatus = SequenceNumberUnavailable
	}
	if parseErr != nil {
		var adapterErr *AdapterError
		if errors.As(parseErr, &adapterErr) {
			result.Status = ResponseFailed
			result.Error = adapterErr.Info
			finish()
			return result, parseErr
		}
		// Callback errors are returned unchanged with the partial response.
		if terminalErr == parseErr {
			finish()
			return result, parseErr
		}
		return fail(invalidResponsesError("llm.sse.invalid_stream"), parseErr)
	}
	if !terminal {
		return interrupted(streamInterruptedError(), io.EOF)
	}
	finish()
	return result, nil
}

func normalizeResponsesSSEEvent(wire responsesSSEWireEvent, request ModelRequest, receivedAt time.Time) (ModelEvent, bool, error) {
	event := ModelEvent{
		Type: safeStableCode(wire.Type), SequenceNumber: wire.SequenceNumber,
		ResponseID: safeProtocolValue(wire.ResponseID, 256),
		OutputIndex: wire.OutputIndex, ContentIndex: wire.ContentIndex,
		ItemID: safeProtocolValue(wire.ItemID, 256), CallID: safeProtocolValue(wire.CallID, 256),
		Status: safeProtocolValue(wire.Status, 64), ReceivedAt: receivedAt,
	}
	switch wire.Type {
	case "response.created":
		var response struct {
			ID string `json:"id"`
			Model string `json:"model"`
		}
		if len(wire.Response) == 0 || json.Unmarshal(wire.Response, &response) != nil || response.ID == "" {
			return ModelEvent{}, false, invalidSSEError("llm.sse.created_missing_response")
		}
		event.ResponseID = safeProtocolValue(response.ID, 256)
		event.Status = "in_progress"
	case "response.output_item.added", "response.output_item.done":
		item, err := parseResponsesStreamOutputItem(wire.Item)
		if err != nil {
			return ModelEvent{}, false, err
		}
		if wire.Type == "response.output_item.done" && item.Type == OutputItemFunctionCall &&
			(item.ToolCall == nil || item.ToolCall.ArgumentsJSON == "" || !json.Valid([]byte(item.ToolCall.ArgumentsJSON))) {
			return ModelEvent{}, false, invalidSSEError("llm.sse.invalid_function_call_item")
		}
		event.Item = &item
		if event.Status == "" {
			event.Status = item.Status
		}
		if event.ItemID == "" {
			event.ItemID = item.ItemID
		}
		if item.ToolCall != nil && item.ToolCall.CallID != "" {
			event.CallID = item.ToolCall.CallID
		}
	case "response.content_part.added", "response.content_part.done":
		part, err := parseResponsesContentPart(wire.ContentPart)
		if err != nil {
			return ModelEvent{}, false, invalidSSEError("llm.sse.invalid_content_part")
		}
		event.ContentPart = &part
	case "response.output_text.delta", "response.function_call_arguments.delta":
		if wire.Delta == nil {
			return ModelEvent{}, false, invalidSSEError("llm.sse.missing_delta")
		}
		event.Delta = *wire.Delta
	case "response.refusal.delta":
		if wire.Delta == nil {
			return ModelEvent{}, false, invalidSSEError("llm.sse.missing_delta")
		}
		event.Delta = *wire.Delta
	case "response.output_text.done":
		if wire.Text == nil {
			return ModelEvent{}, false, invalidSSEError("llm.sse.missing_text")
		}
		event.Delta = *wire.Text
	case "response.refusal.done":
		if wire.Refusal != nil {
			event.Delta = *wire.Refusal
		} else if wire.Text != nil {
			event.Delta = *wire.Text
		} else {
			return ModelEvent{}, false, invalidSSEError("llm.sse.missing_refusal")
		}
	case "response.function_call_arguments.done":
		if event.CallID == "" || wire.Arguments == nil || !json.Valid([]byte(*wire.Arguments)) {
			return ModelEvent{}, false, invalidSSEError("llm.sse.invalid_arguments")
		}
		event.Arguments = *wire.Arguments
	case "response.completed", "response.incomplete", "response.failed":
		if len(wire.Response) == 0 {
			return ModelEvent{}, false, invalidSSEError("llm.sse.missing_terminal_response")
		}
		var envelope responsesEnvelope
		if json.Unmarshal(wire.Response, &envelope) != nil || envelope.Status != strings.TrimPrefix(wire.Type, "response.") {
			return ModelEvent{}, false, invalidSSEError("llm.sse.terminal_status_mismatch")
		}
		event.ResponseID = safeProtocolValue(envelope.ID, 256)
		event.Status = envelope.Status
		if envelope.Usage != nil {
			event.Usage = normalizeResponsesUsage(envelope.Usage)
		}
		return event, true, nil
	case "error":
		if wire.Error == nil {
			return ModelEvent{}, false, invalidSSEError("llm.sse.invalid_error_event")
		}
		code := safeStableCode(wire.Error.Code)
		if code == "" {
			code = "llm.sse.upstream_error"
		}
		event.Error = &ModelErrorInfo{
			Kind: ErrorUpstream, StableCode: code,
			SafeMessage: "upstream Responses stream failed",
		}
		return event, true, nil
	default:
		// Unknown events retain only the explicit safe metadata allowlist.
		if len(wire.Item) > 0 {
			item, err := parseSSEUnknownItemMetadata(wire.Item)
			if err == nil {
				event.Item = &item
			}
		}
		if len(wire.ContentPart) > 0 {
			part, err := parseSSEUnknownContentMetadata(wire.ContentPart)
			if err == nil {
				event.ContentPart = &part
			}
		}
	}
	return event, false, nil
}

func parseSSEUnknownItemMetadata(raw json.RawMessage) (OutputItem, error) {
	if len(raw) == 0 || len(raw) > MaxSSEEventBytes || !json.Valid(raw) {
		return OutputItem{}, ErrInvalidResponsesSSE
	}
	var wire struct {
		Type string `json:"type"`
		ID string `json:"id"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil || safeStableCode(wire.Type) == "" {
		return OutputItem{}, ErrInvalidResponsesSSE
	}
	item := OutputItem{ItemID: safeProtocolValue(wire.ID, 256), Status: safeProtocolValue(wire.Status, 64)}
	switch wire.Type {
	case "message":
		item.Type = OutputItemMessage
	case "function_call":
		item.Type = OutputItemFunctionCall
	case "refusal":
		item.Type = OutputItemRefusal
	default:
		item.Type = OutputItemUnknown
		item.UnknownType = safeStableCode(wire.Type)
	}
	return item, nil
}

func parseSSEUnknownContentMetadata(raw json.RawMessage) (ContentPart, error) {
	if len(raw) == 0 || len(raw) > MaxSSEEventBytes || !json.Valid(raw) {
		return ContentPart{}, ErrInvalidResponsesSSE
	}
	var wire struct { Type string `json:"type"` }
	if err := json.Unmarshal(raw, &wire); err != nil || safeStableCode(wire.Type) == "" {
		return ContentPart{}, ErrInvalidResponsesSSE
	}
	part := ContentPart{Type: ContentPartUnknown, UnknownType: safeStableCode(wire.Type)}
	switch wire.Type {
	case "output_text":
		part.Type = ContentPartOutputText
	case "refusal":
		part.Type = ContentPartRefusal
	}
	return part, nil
}

func parseResponsesStreamOutputItem(raw json.RawMessage) (OutputItem, error) {
	if len(raw) == 0 || len(raw) > MaxSSEEventBytes || !json.Valid(raw) {
		return OutputItem{}, ErrInvalidResponsesSSE
	}
	var wire struct {
		Type string `json:"type"`
		ID string `json:"id"`
		Status string `json:"status"`
		Content []json.RawMessage `json:"content"`
		CallID string `json:"call_id"`
		Name string `json:"name"`
		Arguments string `json:"arguments"`
		Refusal string `json:"refusal"`
	}
	if err := json.Unmarshal(raw, &wire); err != nil || safeStableCode(wire.Type) == "" {
		return OutputItem{}, ErrInvalidResponsesSSE
	}
	item := OutputItem{ItemID: safeProtocolValue(wire.ID, 256), Status: safeProtocolValue(wire.Status, 64)}
	switch wire.Type {
	case "message":
		item.Type = OutputItemMessage
		for _, rawPart := range wire.Content {
			part, err := parseResponsesContentPart(rawPart)
			if err != nil {
				return OutputItem{}, err
			}
			item.ContentParts = append(item.ContentParts, part)
		}
	case "refusal":
		item.Type = OutputItemRefusal
		item.ContentParts = []ContentPart{{Type: ContentPartRefusal, Text: safeProtocolValue(wire.Refusal, MaxSSEEventBytes)}}
	case "function_call":
		item.Type = OutputItemFunctionCall
		if wire.CallID == "" || wire.Name == "" || len(wire.Arguments) > MaxSSEEventBytes {
			return OutputItem{}, ErrInvalidResponsesSSE
		}
		call := ModelToolCall{
			CallID: safeProtocolValue(wire.CallID, 256), OutputItemID: item.ItemID,
			Name: safeProtocolValue(wire.Name, 128), ArgumentsJSON: wire.Arguments,
			Status: item.Status,
		}
		item.ToolCall = &call
	default:
		item.Type = OutputItemUnknown
		item.UnknownType = safeStableCode(wire.Type)
	}
	return item, nil
}

func upsertSSEOutputItem(response *ModelResponse, item OutputItem) {
	replaced := false
	for i := range response.OutputItems {
		if item.ItemID != "" && response.OutputItems[i].ItemID == item.ItemID {
			response.OutputItems[i] = item
			replaced = true
			break
		}
	}
	if !replaced {
		response.OutputItems = append(response.OutputItems, item)
	}
	if item.ToolCall != nil {
		for i := range response.ToolCalls {
			if response.ToolCalls[i].CallID == item.ToolCall.CallID {
				response.ToolCalls[i] = *item.ToolCall
				return
			}
		}
		response.ToolCalls = append(response.ToolCalls, *item.ToolCall)
	}
}

func terminalResponseFromEvent(wire responsesSSEWireEvent, request ModelRequest, startedAt time.Time, partial ModelResponse) (ModelResponse, error) {
	var parsed ModelResponse
	if len(wire.Response) == 0 {
		return partial, invalidSSEError("llm.sse.missing_terminal_response")
	}
	parsed, err := ParseResponsesResponse(wire.Response, request)
	parsed.StartedAt = startedAt
	parsed.GatewayRequestID = partial.GatewayRequestID
	parsed.UpstreamRequestID = partial.UpstreamRequestID
	if err != nil {
		if errors.Is(err, ErrResponsesSchemaMismatch) {
			return parsed, &AdapterError{Info: parsed.Error, cause: err}
		}
		parsed = partial
		parsed.Status = ResponseFailed
		parsed.Error = invalidResponsesError("llm.sse.invalid_terminal_response")
		return parsed, invalidSSEError("llm.sse.invalid_terminal_response")
	}
	return parsed, nil
}

func terminalToolCalls(raw json.RawMessage, request ModelRequest) []ModelToolCall {
	if len(raw) == 0 {
		return nil
	}
	parsed, err := ParseResponsesResponse(raw, request)
	if err != nil {
		return nil
	}
	return parsed.ToolCalls
}

func streamInterruptedError() *ModelErrorInfo {
	return &ModelErrorInfo{
		Kind: ErrorStreamInterrupted, StableCode: "llm.sse.stream_interrupted",
		SafeMessage: "Responses stream ended before a terminal event",
	}
}

func invalidSSEError(code string) error {
	return newSSEAdapterError(&ModelErrorInfo{
		Kind: ErrorInvalidResponse, StableCode: code,
		SafeMessage: "invalid Responses SSE stream",
	})
}

func newSSEAdapterError(info *ModelErrorInfo) *AdapterError {
	return &AdapterError{Info: info, cause: ErrInvalidResponsesSSE}
}

