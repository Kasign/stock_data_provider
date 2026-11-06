package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	ProtocolOpenAIResponses = "openai_responses"
	MaxPayloadBytes = 8 << 20
	MaxSchemaBytes = 1 << 20
)

type Capability string
type CapabilityStatus string

const (
	CapabilityResponsesJSON Capability = "responses_json"
	CapabilityResponsesStream Capability = "responses_stream"
	CapabilityStructuredOutput Capability = "structured_output_json_schema"
	CapabilityFunctionToolCalls Capability = "function_tool_calls"
	CapabilityReasoningEffort Capability = "reasoning_effort"
	CapabilitySamplingParameters Capability = "sampling_parameters"
	CapabilityUsageDetails Capability = "usage_details"

	CapabilitySupported CapabilityStatus = "supported"
	CapabilityUnsupported CapabilityStatus = "unsupported"
	CapabilityUnknown CapabilityStatus = "unknown"
)

// CapabilityState 将能力绑定到 Provider、Model、Gateway 版本和实测时间。
type CapabilityState struct {
	ProviderID string `json:"provider_id"`
	ModelID string `json:"model_id"`
	Protocol string `json:"protocol"`
	GatewayVersion string `json:"gateway_version"`
	VerifiedAt *time.Time `json:"verified_at,omitempty"`
	ValidUntil *time.Time `json:"valid_until,omitempty"`
	EvidenceRef string `json:"evidence_ref"`
	MaxOutputTokensLimit int `json:"max_output_tokens_limit"`
	Values map[Capability]CapabilityStatus `json:"values,omitempty"`
}

func (s CapabilityState) Status(key Capability) CapabilityStatus {
	if v, ok := s.Values[key]; ok { return v }
	return CapabilityUnknown
}

func (s CapabilityState) Supports(key Capability) bool {
	return s.ProviderID != "" && s.ModelID != "" && s.Protocol == ProtocolOpenAIResponses &&
		s.GatewayVersion != "" && s.EvidenceRef != "" && s.VerifiedAt != nil && !s.VerifiedAt.IsZero() &&
		s.ValidUntil != nil && s.ValidUntil.After(*s.VerifiedAt) &&
		time.Now().Before(*s.ValidUntil) && s.Status(key) == CapabilitySupported
}

// ProviderConfig 仅保存 Secret 引用，不保存凭据明文。
type ProviderConfig struct {
	ProviderID string `json:"provider_id"`
	Protocol string `json:"protocol"`
	BaseURL string `json:"base_url"`
	DefaultModel string `json:"default_model"`
	APIKeySecretRef string `json:"api_key_secret_ref"`
	RequestTimeoutMS int64 `json:"request_timeout_ms"`
	AllowedModels []string `json:"allowed_models"`
	MaxRequestBytes int64 `json:"max_request_bytes"`
	MaxResponseBytes int64 `json:"max_response_bytes"`
	StreamEnabled bool `json:"stream_enabled"`
}

func (c ProviderConfig) Validate() error {
	if strings.TrimSpace(c.ProviderID) == "" || len(c.ProviderID) > 128 { return errors.New("invalid provider_id") }
	if c.Protocol != ProtocolOpenAIResponses { return fmt.Errorf("unsupported protocol %q", c.Protocol) }
	u, err := url.Parse(c.BaseURL)
	if err != nil || u == nil || u.Scheme != "https" || u.Hostname() == "" ||
		u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "/v1" {
		return errors.New("base_url must be an HTTPS Gateway URL ending in /v1")
	}
	if strings.TrimSpace(c.DefaultModel) == "" || strings.TrimSpace(c.APIKeySecretRef) == "" {
		return errors.New("default_model and api_key_secret_ref are required")
	}
	if c.RequestTimeoutMS <= 0 { return errors.New("request_timeout_ms must be positive") }
	if len(c.AllowedModels) == 0 { return errors.New("allowed_models must not be empty") }
	allowed := make(map[string]struct{}, len(c.AllowedModels))
	for _, modelID := range c.AllowedModels {
		if strings.TrimSpace(modelID) == "" { return errors.New("allowed_models contains an empty model") }
		if _, exists := allowed[modelID]; exists { return fmt.Errorf("duplicate allowed model %q", modelID) }
		allowed[modelID] = struct{}{}
	}
	if _, ok := allowed[c.DefaultModel]; !ok { return errors.New("default_model must be included in allowed_models") }
	if c.MaxRequestBytes <= 0 || c.MaxRequestBytes > MaxPayloadBytes ||
		c.MaxResponseBytes <= 0 || c.MaxResponseBytes > MaxPayloadBytes {
		return fmt.Errorf("request and response limits must be between 1 and %d", MaxPayloadBytes)
	}
	return nil
}

type JSONSchemaSpec struct {
	Name string `json:"name"`
	SchemaJSON json.RawMessage `json:"schema_json"`
	Strict bool `json:"strict"`
}

type ToolDefinition struct {
	Name string `json:"name"`
	Description string `json:"description,omitempty"`
	ParametersJSON json.RawMessage `json:"parameters_json"`
	Strict bool `json:"strict"`
}

type ModelRequest struct {
	ProviderID string `json:"provider_id"`
	RequestID string `json:"request_id"`
	PurposeCode string `json:"purpose_code"`
	Model string `json:"model"`
	Instructions string `json:"instructions"`
	InputText string `json:"input_text"`
	MaxOutputTokens int `json:"max_output_tokens"`
	Temperature *float64 `json:"temperature,omitempty"`
	TopP *float64 `json:"top_p,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	ResponseSchema *JSONSchemaSpec `json:"response_schema,omitempty"`
	Tools []ToolDefinition `json:"tools,omitempty"`
	ToolChoice string `json:"tool_choice,omitempty"`
	DeadlineAt time.Time `json:"deadline_at"`
	CorrelationID string `json:"correlation_id"`
	SystemAsOf time.Time `json:"system_as_of"`
	TraceID string `json:"trace_id,omitempty"`
	LocalValidationOnly bool `json:"local_validation_only,omitempty"`
}

func (r ModelRequest) Validate() error {
	if strings.TrimSpace(r.ProviderID) == "" || strings.TrimSpace(r.RequestID) == "" ||
		strings.TrimSpace(r.CorrelationID) == "" || strings.TrimSpace(r.PurposeCode) == "" ||
		strings.TrimSpace(r.Model) == "" {
		return errors.New("provider_id, request_id, correlation_id, purpose_code and model are required")
	}
	if strings.TrimSpace(r.InputText) == "" || r.MaxOutputTokens <= 0 {
		return errors.New("input_text must not be empty and max_output_tokens must be positive")
	}
	if r.DeadlineAt.IsZero() || r.SystemAsOf.IsZero() { return errors.New("deadline_at and system_as_of are required") }
	if !r.DeadlineAt.After(time.Now()) { return errors.New("deadline_at must be in the future") }
	if r.SystemAsOf.After(time.Now().UTC()) { return errors.New("system_as_of must not be in the future") }
	_, deadlineOffset := r.DeadlineAt.Zone()
	_, systemOffset := r.SystemAsOf.Zone()
	if deadlineOffset != 0 || systemOffset != 0 { return errors.New("deadline_at and system_as_of must use UTC") }
	if r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 2) { return errors.New("temperature out of range") }
	if r.TopP != nil && (*r.TopP <= 0 || *r.TopP > 1) { return errors.New("top_p out of range") }
	if r.ReasoningEffort != "" && r.ReasoningEffort != "low" &&
		r.ReasoningEffort != "medium" && r.ReasoningEffort != "high" { return errors.New("invalid reasoning_effort") }
	if r.ToolChoice != "" && r.ToolChoice != "none" && r.ToolChoice != "auto" { return errors.New("unsupported tool_choice") }
	if r.ToolChoice == "auto" && len(r.Tools) == 0 { return errors.New("tool_choice auto requires tools") }
	if r.ResponseSchema != nil {
		if strings.TrimSpace(r.ResponseSchema.Name) == "" || len(r.ResponseSchema.SchemaJSON) == 0 ||
			len(r.ResponseSchema.SchemaJSON) > MaxSchemaBytes || !json.Valid(r.ResponseSchema.SchemaJSON) {
			return errors.New("response_schema requires a name and valid bounded JSON")
		}
	}
	seen := make(map[string]struct{}, len(r.Tools))
	for _, tool := range r.Tools {
		if strings.TrimSpace(tool.Name) == "" || len(tool.Name) > 128 { return errors.New("invalid tool name") }
		if _, exists := seen[tool.Name]; exists { return fmt.Errorf("duplicate tool name %q", tool.Name) }
		seen[tool.Name] = struct{}{}
		if len(tool.ParametersJSON) == 0 || !json.Valid(tool.ParametersJSON) {
			return fmt.Errorf("tool %q parameters_json must be valid JSON", tool.Name)
		}
	}
	return nil
}

// ValidateAgainstProvider 检查请求只能使用服务端 ProviderConfig 中允许的 Provider 和模型。
// ValidateCapabilityState 将 Provider 级开关与指定模型的版本化实测证据绑定。
func (c ProviderConfig) ValidateCapabilityState(state CapabilityState) error {
	if err := c.Validate(); err != nil { return err }
	if state.ProviderID != c.ProviderID || state.Protocol != c.Protocol {
		return errors.New("capability evidence does not match provider configuration")
	}
	allowed := false
	for _, modelID := range c.AllowedModels {
		if modelID == state.ModelID { allowed = true; break }
	}
	if !allowed { return errors.New("capability evidence model is outside provider allowlist") }
	if !state.Supports(CapabilityResponsesJSON) { return errors.New("responses_json capability is not currently supported") }
	if c.StreamEnabled && !state.Supports(CapabilityResponsesStream) {
		return errors.New("stream_enabled requires current responses_stream evidence")
	}
	return nil
}

func (r ModelRequest) ValidateAgainstProvider(config ProviderConfig) error {
	if err := config.Validate(); err != nil { return err }
	if r.ProviderID != config.ProviderID { return errors.New("request provider_id does not match configured provider") }
	for _, allowed := range config.AllowedModels {
		if r.Model == allowed { return nil }
	}
	return fmt.Errorf("model %q is not allowed by provider %q", r.Model, config.ProviderID)
}

// ValidateCapabilities 不允许把 unknown 或旧版本能力当作当前 supported。
func (r ModelRequest) ValidateCapabilities(s CapabilityState) error {
	if s.ModelID != r.Model || s.ProviderID != r.ProviderID || s.Protocol != ProtocolOpenAIResponses ||
		s.GatewayVersion == "" || s.EvidenceRef == "" || s.VerifiedAt == nil || s.VerifiedAt.IsZero() ||
		s.ValidUntil == nil || !time.Now().Before(*s.ValidUntil) {
		return errors.New("capability evidence does not match a current verified provider/model/protocol/gateway version")
	}
	if s.MaxOutputTokensLimit <= 0 || r.MaxOutputTokens > s.MaxOutputTokensLimit {
		return errors.New("max_output_tokens exceeds the verified model capability limit")
	}
	require := func(key Capability) error {
		if !s.Supports(key) { return fmt.Errorf("unsupported_capability: %s", key) }
		return nil
	}
	if err := require(CapabilityResponsesJSON); err != nil { return err }
	if r.Temperature != nil || r.TopP != nil {
		if err := require(CapabilitySamplingParameters); err != nil { return err }
	}
	if r.ReasoningEffort != "" {
		if err := require(CapabilityReasoningEffort); err != nil { return err }
	}
	if r.ResponseSchema != nil && !r.LocalValidationOnly {
		if err := require(CapabilityStructuredOutput); err != nil { return err }
	}
	if len(r.Tools) > 0 || r.ToolChoice == "auto" {
		if err := require(CapabilityFunctionToolCalls); err != nil { return err }
	}
	return nil
}

// ValidateStreamCapabilities 在基础 Responses JSON 能力之外，单独要求真实 SSE 能力证据。
func (r ModelRequest) ValidateStreamCapabilities(s CapabilityState) error {
	if err := r.ValidateCapabilities(s); err != nil { return err }
	if !s.Supports(CapabilityResponsesStream) {
		return fmt.Errorf("unsupported_capability: %s", CapabilityResponsesStream)
	}
	return nil
}

type ResponseStatus string
const (
	ResponseCompleted ResponseStatus = "completed"
	ResponseIncomplete ResponseStatus = "incomplete"
	ResponseFailed ResponseStatus = "failed"
)

type OutputItemType string
const (
	OutputItemMessage OutputItemType = "message"
	OutputItemFunctionCall OutputItemType = "function_call"
	OutputItemRefusal OutputItemType = "refusal"
	OutputItemUnknown OutputItemType = "unknown"
)

type ContentPartType string
const (
	ContentPartOutputText ContentPartType = "output_text"
	ContentPartRefusal ContentPartType = "refusal"
	ContentPartUnknown ContentPartType = "unknown"
)

type OutputItem struct {
	Type OutputItemType `json:"type"`
	ItemID string `json:"item_id,omitempty"`
	Status string `json:"status,omitempty"`
	ContentParts []ContentPart `json:"content_parts,omitempty"`
	ToolCall *ModelToolCall `json:"tool_call,omitempty"`
}
type ContentPart struct {
	Type ContentPartType `json:"type"`
	Text string `json:"text,omitempty"`
	Annotations json.RawMessage `json:"annotations,omitempty"`
}
type ModelToolCall struct {
	CallID string `json:"call_id"`
	OutputItemID string `json:"output_item_id,omitempty"`
	Name string `json:"name"`
	ArgumentsJSON string `json:"arguments_json"`
	Status string `json:"status,omitempty"`
}

type UsageSource string
const (
	UsageSourceUpstream UsageSource = "upstream"
	UsageSourceDerived UsageSource = "derived"
	UsageSourceUnknown UsageSource = "unknown"
)
type Usage struct {
	InputTokens *int64 `json:"input_tokens,omitempty"`
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	TotalTokens *int64 `json:"total_tokens,omitempty"`
	InputCachedTokens *int64 `json:"input_cached_tokens,omitempty"`
	OutputReasoningTokens *int64 `json:"output_reasoning_tokens,omitempty"`
	Source UsageSource `json:"source"`
}

type ModelErrorKind string
const (
	ErrorInvalidRequest ModelErrorKind = "invalid_request"
	ErrorUnauthorized ModelErrorKind = "unauthorized"
	ErrorRateLimited ModelErrorKind = "rate_limited"
	ErrorOverloaded ModelErrorKind = "overloaded"
	ErrorTimeout ModelErrorKind = "timeout"
	ErrorCancelled ModelErrorKind = "cancelled"
	ErrorUpstreamUnavailable ModelErrorKind = "upstream_unavailable"
	ErrorUpstream ModelErrorKind = "upstream_error"
	ErrorInvalidResponse ModelErrorKind = "invalid_response"
	ErrorIncompleteResponse ModelErrorKind = "incomplete_response"
	ErrorSchemaValidationFailed ModelErrorKind = "schema_validation_failed"
	ErrorUnsupportedCapability ModelErrorKind = "unsupported_capability"
	ErrorStreamInterrupted ModelErrorKind = "stream_interrupted"
	ErrorStreamSequenceGap ModelErrorKind = "stream_sequence_gap"
	ErrorResourceLimit ModelErrorKind = "resource_limit"
)
type ModelErrorInfo struct {
	Kind ModelErrorKind `json:"kind"`
	StableCode string `json:"stable_code"`
	Retryable bool `json:"retryable"`
	HTTPStatus *int `json:"http_status,omitempty"`
	RetryAfterSeconds *int64 `json:"retry_after_seconds,omitempty"`
	SafeMessage string `json:"safe_message"`
}

type ModelResponse struct {
	RequestID string `json:"request_id"`
	CorrelationID string `json:"correlation_id"`
	SystemAsOf time.Time `json:"system_as_of"`
	ProviderID string `json:"provider_id"`
	ResponseID string `json:"response_id,omitempty"`
	Status ResponseStatus `json:"status"`
	OutputItems []OutputItem `json:"output_items"`
	OutputText string `json:"output_text,omitempty"`
	StructuredJSON json.RawMessage `json:"structured_json,omitempty"`
	ToolCalls []ModelToolCall `json:"tool_calls,omitempty"`
	Usage *Usage `json:"usage,omitempty"`
	ModelRequested string `json:"model_requested"`
	ModelActual string `json:"model_actual,omitempty"`
	ModelVersion string `json:"model_version,omitempty"`
	GatewayRequestID string `json:"gateway_request_id,omitempty"`
	UpstreamRequestID string `json:"upstream_request_id,omitempty"`
	IncompleteReason string `json:"incomplete_reason,omitempty"`
	Error *ModelErrorInfo `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	DurationMS int64 `json:"duration_ms"`
}

type ModelEvent struct {
	Type string `json:"type"`
	SequenceNumber *int64 `json:"sequence_number,omitempty"`
	ResponseID string `json:"response_id,omitempty"`
	OutputIndex *int `json:"output_index,omitempty"`
	ContentIndex *int `json:"content_index,omitempty"`
	ItemID string `json:"item_id,omitempty"`
	CallID string `json:"call_id,omitempty"`
	Item *OutputItem `json:"item,omitempty"`
	ContentPart *ContentPart `json:"content_part,omitempty"`
	Delta string `json:"delta,omitempty"`
	Arguments string `json:"arguments,omitempty"`
	Usage *Usage `json:"usage,omitempty"`
	Error *ModelErrorInfo `json:"error,omitempty"`
	ReceivedAt time.Time `json:"received_at"`
}

// Provider 是协议适配边界。工具授权与执行不属于该接口。
type Provider interface {
	Generate(ctx context.Context, request ModelRequest) (ModelResponse, error)
	// Stream 使用同步回调提供背压；无 terminal event 的 EOF 必须返回 stream_interrupted。
	Stream(ctx context.Context, request ModelRequest, onEvent func(ModelEvent) error) (ModelResponse, error)
}
