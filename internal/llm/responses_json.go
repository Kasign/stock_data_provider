package llm

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrInvalidResponsesRequest  = errors.New("invalid Responses request")
	ErrInvalidResponsesResponse = errors.New("invalid Responses response")
	ErrResponsesSchemaMismatch  = errors.New("Responses output does not match the requested schema")
)

type responsesRequest struct {
	Model           string          `json:"model"`
	Instructions    string          `json:"instructions"`
	Input           string          `json:"input"`
	MaxOutputTokens int             `json:"max_output_tokens"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	Reasoning       *responsesReasoning `json:"reasoning,omitempty"`
	Text            *responsesText `json:"text,omitempty"`
	Tools           []responsesTool `json:"tools,omitempty"`
	ToolChoice      string          `json:"tool_choice"`
}

type responsesReasoning struct {
	Effort string `json:"effort"`
}

type responsesText struct {
	Format responsesTextFormat `json:"format"`
}

type responsesTextFormat struct {
	Type   string          `json:"type"`
	Name   string          `json:"name"`
	Strict bool            `json:"strict"`
	Schema json.RawMessage `json:"schema"`
}

type responsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters"`
	Strict      bool            `json:"strict"`
}

// MapResponsesRequest maps the application DTO to the first-party Responses JSON
// wire shape. It never accepts caller-supplied URLs, headers, or arbitrary JSON.
func MapResponsesRequest(request ModelRequest) ([]byte, error) {
	return MapResponsesRequestAt(request, SystemClock{}.Now())
}

func MapResponsesRequestAt(request ModelRequest, now time.Time) ([]byte, error) {
	if err := request.ValidateAt(now); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidResponsesRequest, err)
	}

	wire := responsesRequest{
		Model: request.Model, Instructions: request.Instructions,
		Input: request.InputText, MaxOutputTokens: request.MaxOutputTokens,
		Temperature: request.Temperature, TopP: request.TopP,
		ToolChoice: request.ToolChoice,
	}
	if wire.ToolChoice == "" {
		wire.ToolChoice = "none"
	}
	if request.ReasoningEffort != "" {
		wire.Reasoning = &responsesReasoning{Effort: request.ReasoningEffort}
	}
	if request.ResponseSchema != nil && !request.LocalValidationOnly {
		wire.Text = &responsesText{Format: responsesTextFormat{
			Type: "json_schema", Name: request.ResponseSchema.Name,
			Strict: request.ResponseSchema.Strict, Schema: request.ResponseSchema.SchemaJSON,
		}}
	}
	if len(request.Tools) > 0 {
		wire.Tools = make([]responsesTool, 0, len(request.Tools))
		for _, tool := range request.Tools {
			wire.Tools = append(wire.Tools, responsesTool{
				Type: "function", Name: tool.Name, Description: tool.Description,
				Parameters: tool.ParametersJSON, Strict: tool.Strict,
			})
		}
	}
	encoded, err := json.Marshal(wire)
	if err != nil {
		return nil, fmt.Errorf("%w: encode failed", ErrInvalidResponsesRequest)
	}
	if len(encoded) > MaxPayloadBytes {
		return nil, fmt.Errorf("%w: payload exceeds limit", ErrInvalidResponsesRequest)
	}
	return encoded, nil
}

type responsesEnvelope struct {
	ID               string          `json:"id"`
	Object           string          `json:"object"`
	Model            string          `json:"model"`
	Status           string          `json:"status"`
	Output           []json.RawMessage `json:"output"`
	Usage            *responsesUsage `json:"usage"`
	IncompleteDetails *struct {
		Reason string `json:"reason"`
	} `json:"incomplete_details"`
	Error            *responsesError `json:"error"`
}

type responsesError struct {
	Type    string `json:"type"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type responsesUsage struct {
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	TotalTokens  *int64 `json:"total_tokens"`
	InputDetails *struct {
		CachedTokens *int64 `json:"cached_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails *struct {
		ReasoningTokens *int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// ParseResponsesResponse normalizes a non-streaming Responses envelope. A
// valid upstream incomplete/failed status is represented in ModelResponse and
// is not duplicated as a Go transport error.
func ParseResponsesResponse(body []byte, request ModelRequest) (ModelResponse, error) {
	result := ModelResponse{
		RequestID: request.RequestID, CorrelationID: request.CorrelationID,
		SystemAsOf: request.SystemAsOf, ProviderID: request.ProviderID,
		ModelRequested: request.Model, Status: ResponseFailed,
		OutputItems: []OutputItem{},
	}
	if len(body) == 0 || len(body) > MaxPayloadBytes || !json.Valid(body) {
		result.Error = invalidResponsesError("llm.responses.invalid_json")
		return result, ErrInvalidResponsesResponse
	}

	var envelope responsesEnvelope
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&envelope); err != nil {
		result.Error = invalidResponsesError("llm.responses.invalid_envelope")
		return result, ErrInvalidResponsesResponse
	}
	if envelope.Object != "response" {
		result.Error = invalidResponsesError("llm.responses.invalid_object")
		return result, ErrInvalidResponsesResponse
	}
	result.ResponseID = safeProtocolValue(envelope.ID, 256)
	result.ModelActual = safeProtocolValue(envelope.Model, 128)
	result.IncompleteReason = ""
	if envelope.IncompleteDetails != nil {
		result.IncompleteReason = safeIncompleteReason(envelope.IncompleteDetails.Reason)
	}
	switch envelope.Status {
	case "completed":
		result.Status = ResponseCompleted
	case "incomplete":
		result.Status = ResponseIncomplete
	case "failed":
		result.Status = ResponseFailed
	default:
		result.Error = invalidResponsesError("llm.responses.invalid_status")
		return result, ErrInvalidResponsesResponse
	}

	for _, rawItem := range envelope.Output {
		item, calls, err := parseResponsesOutputItem(rawItem)
		if err != nil {
			result.Error = invalidResponsesError("llm.responses.invalid_output_item")
			return result, ErrInvalidResponsesResponse
		}
		result.OutputItems = append(result.OutputItems, item)
		if item.Type == OutputItemMessage {
			for _, part := range item.ContentParts {
				if part.Type == ContentPartOutputText {
					result.OutputText += part.Text
				}
			}
		}
		result.ToolCalls = append(result.ToolCalls, calls...)
	}
	if envelope.Usage != nil {
		result.Usage = normalizeResponsesUsage(envelope.Usage)
	}
	if result.Status == ResponseCompleted && result.ResponseID == "" {
		result.Error = invalidResponsesError("llm.responses.missing_response_id")
		return result, ErrInvalidResponsesResponse
	}
	if result.Status == ResponseFailed {
		result.Error = normalizeResponsesError(envelope.Error)
	}
	if result.Status == ResponseIncomplete && result.IncompleteReason == "" {
		result.IncompleteReason = "unspecified"
	}

	if request.ResponseSchema != nil && result.Status == ResponseCompleted {
		trimmed := strings.TrimSpace(result.OutputText)
		if trimmed == "" || !json.Valid([]byte(trimmed)) {
			result.Error = schemaMismatchError()
			return result, ErrResponsesSchemaMismatch
		}
		if err := ValidateJSONAgainstSchema(request.ResponseSchema.SchemaJSON, []byte(trimmed)); err != nil {
			result.Error = schemaMismatchError()
			return result, ErrResponsesSchemaMismatch
		}
		result.StructuredJSON = json.RawMessage(trimmed)
	}
	return result, nil
}

func parseResponsesOutputItem(raw json.RawMessage) (OutputItem, []ModelToolCall, error) {
	var wire struct {
		Type      string            `json:"type"`
		ID        string            `json:"id"`
		Status    string            `json:"status"`
		Role      string            `json:"role"`
		Content   []json.RawMessage `json:"content"`
		CallID    string            `json:"call_id"`
		Name      string            `json:"name"`
		Arguments string            `json:"arguments"`
		Refusal   string            `json:"refusal"`
	}
	if len(raw) == 0 || len(raw) > MaxPayloadBytes || !json.Valid(raw) {
		return OutputItem{}, nil, ErrInvalidResponsesResponse
	}
	if err := json.Unmarshal(raw, &wire); err != nil || wire.Type == "" {
		return OutputItem{}, nil, ErrInvalidResponsesResponse
	}
	item := OutputItem{ItemID: safeProtocolValue(wire.ID, 256), Status: safeProtocolValue(wire.Status, 64)}
	if wire.Type != "message" && wire.Type != "function_call" && wire.Type != "refusal" {
		item.UnknownType = safeStableCode(wire.Type)
	}
	switch wire.Type {
	case "message":
		item.Type = OutputItemMessage
		for _, rawPart := range wire.Content {
			part, err := parseResponsesContentPart(rawPart)
			if err != nil {
				return OutputItem{}, nil, err
			}
			item.ContentParts = append(item.ContentParts, part)
		}
		return item, nil, nil
	case "refusal":
		item.Type = OutputItemRefusal
		item.ContentParts = []ContentPart{{Type: ContentPartRefusal, Text: wire.Refusal}}
		return item, nil, nil
	case "function_call":
		item.Type = OutputItemFunctionCall
		if wire.CallID == "" || wire.Name == "" || wire.Arguments == "" || !json.Valid([]byte(wire.Arguments)) {
			return OutputItem{}, nil, ErrInvalidResponsesResponse
		}
		call := ModelToolCall{
			CallID: safeProtocolValue(wire.CallID, 256), OutputItemID: item.ItemID,
			Name: safeProtocolValue(wire.Name, 128), ArgumentsJSON: wire.Arguments,
			Status: item.Status,
		}
		item.ToolCall = &call
		return item, []ModelToolCall{call}, nil
	case "reasoning", "file_search_call", "web_search_call", "computer_call", "code_interpreter_call":
		item.Type = OutputItemUnknown
		return item, nil, nil
	default:
		item.Type = OutputItemUnknown
		return item, nil, nil
	}
}

func parseResponsesContentPart(raw json.RawMessage) (ContentPart, error) {
	var wire struct {
		Type        string          `json:"type"`
		Text        string          `json:"text"`
		Annotations json.RawMessage `json:"annotations"`
	}
	if len(raw) == 0 || len(raw) > MaxPayloadBytes || !json.Valid(raw) {
		return ContentPart{}, ErrInvalidResponsesResponse
	}
	if err := json.Unmarshal(raw, &wire); err != nil || wire.Type == "" {
		return ContentPart{}, ErrInvalidResponsesResponse
	}
	part := ContentPart{}
	if wire.Type != "output_text" && wire.Type != "refusal" {
		part.UnknownType = safeStableCode(wire.Type)
	}
	switch wire.Type {
	case "output_text":
		part.Type = ContentPartOutputText
		part.Text = wire.Text
	case "refusal":
		part.Type = ContentPartRefusal
		part.Text = wire.Text
	default:
		part.Type = ContentPartUnknown
	}
	return part, nil
}

func normalizeResponsesUsage(wire *responsesUsage) *Usage {
	if wire == nil {
		return nil
	}
	usage := &Usage{
		InputTokens: wire.InputTokens, OutputTokens: wire.OutputTokens,
		TotalTokens: wire.TotalTokens, Source: UsageSourceUpstream,
	}
	if wire.InputDetails != nil {
		usage.InputCachedTokens = wire.InputDetails.CachedTokens
	}
	if wire.OutputDetails != nil {
		usage.OutputReasoningTokens = wire.OutputDetails.ReasoningTokens
	}
	if usage.TotalTokens != nil && *usage.TotalTokens < 0 {
		usage.Diagnostics = append(usage.Diagnostics, "total_tokens_negative")
	}
	if usage.InputTokens != nil && usage.OutputTokens != nil {
		if *usage.InputTokens < 0 || *usage.OutputTokens < 0 {
			usage.Diagnostics = append(usage.Diagnostics, "token_count_negative")
		} else if *usage.InputTokens > int64(^uint64(0)>>1)-*usage.OutputTokens {
			usage.Diagnostics = append(usage.Diagnostics, "total_tokens_overflow")
		} else {
			derived := *usage.InputTokens + *usage.OutputTokens
			if usage.TotalTokens == nil {
				usage.TotalTokens = &derived
				usage.Source = UsageSourceDerived
			} else if *usage.TotalTokens != derived {
				usage.Diagnostics = append(usage.Diagnostics, "total_tokens_mismatch")
			}
		}
	}
	return usage
}

func normalizeResponsesError(wire *responsesError) *ModelErrorInfo {
	if wire == nil {
		return &ModelErrorInfo{
			Kind: ErrorUpstream, StableCode: "llm.responses.failed",
			SafeMessage: "upstream response failed",
		}
	}
	code := safeStableCode(wire.Code)
	if code == "" {
		code = "llm.responses.failed"
	}
	kind := ErrorUpstream
	retryable := false
	switch strings.ToLower(wire.Type) {
	case "rate_limit_error":
		kind, retryable = ErrorRateLimited, true
	case "server_error", "overloaded_error":
		kind, retryable = ErrorOverloaded, true
	case "authentication_error", "permission_error":
		kind = ErrorUnauthorized
	case "invalid_request_error":
		kind = ErrorInvalidRequest
	}
	return &ModelErrorInfo{
		Kind: kind, StableCode: code, Retryable: retryable,
		SafeMessage: "upstream response failed",
	}
}

func invalidResponsesError(code string) *ModelErrorInfo {
	return &ModelErrorInfo{Kind: ErrorInvalidResponse, StableCode: code, SafeMessage: "invalid upstream Responses payload"}
}

func schemaMismatchError() *ModelErrorInfo {
	return &ModelErrorInfo{Kind: ErrorSchemaValidationFailed, StableCode: "llm.responses.schema_validation_failed", SafeMessage: "model output does not match the requested schema"}
}

func safeProtocolValue(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) > max {
		return value[:max]
	}
	return value
}

func safeStableCode(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-') {
			return ""
		}
	}
	return value
}

func safeIncompleteReason(value string) string {
	switch value {
	case "max_output_tokens", "content_filter":
		return value
	default:
		return "unspecified"
	}
}
