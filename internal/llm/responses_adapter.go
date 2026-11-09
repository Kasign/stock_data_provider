package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/platform/config"
)

// HTTPDoer is intentionally narrow so the composition root can inject a
// Platform httpclient.BoundDoer without exposing a raw HTTP client here.
type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// AdapterError exposes only a stable, sanitized ModelErrorInfo. The underlying
// cause remains available for internal errors.As/errors.Is handling.
type AdapterError struct {
	Info  *ModelErrorInfo
	cause error
}

func (e *AdapterError) Error() string {
	if e == nil || e.Info == nil {
		return "LLM request failed"
	}
	return e.Info.SafeMessage
}

func (e *AdapterError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

// ResponsesAdapter implements non-streaming Responses calls only. Streaming
// must use a separately approved Platform streaming transport.
type ResponsesAdapter struct {
	provider     ProviderConfig
	apiKey       config.Secret
	doer         HTTPDoer
	capabilities CapabilityState
	clock        Clock
}

func NewResponsesAdapter(provider ProviderConfig, apiKey config.Secret, doer HTTPDoer, capabilities CapabilityState, clock Clock) (*ResponsesAdapter, error) {
	if err := provider.Validate(); err != nil {
		return nil, fmt.Errorf("invalid LLM provider configuration: %w", err)
	}
	if !apiKey.Configured() {
		return nil, errors.New("LLM Gateway caller key is required")
	}
	if doer == nil {
		return nil, errors.New("Platform-bound HTTP doer is required")
	}
	if clock == nil {
		return nil, errors.New("LLM clock is required")
	}
	if err := provider.ValidateCapabilityStateAt(capabilities, clock.Now()); err != nil {
		return nil, fmt.Errorf("invalid LLM capability evidence: %w", err)
	}
	return &ResponsesAdapter{
		provider: provider, apiKey: apiKey, doer: doer, capabilities: capabilities, clock: clock,
	}, nil
}

// Generate performs one non-streaming Responses request. It never retries.
// The supplied doer must be a Platform BoundDoer configured for this provider.
func (a *ResponsesAdapter) Generate(ctx context.Context, request ModelRequest) (ModelResponse, error) {
	startedAt := a.clock.Now().UTC()
	result := ModelResponse{
		RequestID: request.RequestID, CorrelationID: request.CorrelationID,
		SystemAsOf: request.SystemAsOf, ProviderID: request.ProviderID,
		ModelRequested: request.Model, StartedAt: startedAt,
		OutputItems: []OutputItem{},
	}
	fail := func(info *ModelErrorInfo, cause error) (ModelResponse, error) {
		finished := a.clock.Now().UTC()
		result.FinishedAt = finished
		result.DurationMS = finished.Sub(startedAt).Milliseconds()
		result.Error = info
		return result, &AdapterError{Info: info, cause: cause}
	}
	if ctx == nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.request.invalid_context", false, nil), ErrInvalidResponsesRequest)
	}
	if err := request.ValidateAt(startedAt); err != nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.request.invalid", false, nil), err)
	}
	if err := request.ValidateAgainstProvider(a.provider); err != nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.provider.request_mismatch", false, nil), err)
	}
	if err := a.provider.ValidateCapabilityStateAt(a.capabilities, startedAt); err != nil {
		return fail(adapterError(ErrorUnsupportedCapability, "llm.provider.capability_unavailable", false, nil), err)
	}
	if err := request.ValidateCapabilitiesAt(a.capabilities, startedAt); err != nil {
		return fail(adapterError(ErrorUnsupportedCapability, "llm.provider.capability_unavailable", false, nil), err)
	}

	body, err := MapResponsesRequestAt(request, startedAt)
	if err != nil {
		code := "llm.responses.request_mapping_failed"
		if errors.Is(err, ErrUnsupportedResponsesSchemaSubset) {
			code = "llm.responses.unsupported_schema_subset"
		}
		return fail(adapterError(ErrorInvalidRequest, code, false, nil), err)
	}
	if int64(len(body)) > a.provider.MaxRequestBytes {
		return fail(adapterError(ErrorResourceLimit, "llm.responses.request_too_large", false, nil), ErrInvalidResponsesRequest)
	}

	deadline := a.effectiveDeadline(ctx, request.DeadlineAt, startedAt)
	if !deadline.After(startedAt) {
		return fail(adapterError(ErrorTimeout, "llm.request.deadline_exceeded", true, nil), context.DeadlineExceeded)
	}
	callCtx, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	endpoint, err := responsesEndpoint(a.provider.BaseURL)
	if err != nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.provider.invalid_endpoint", false, nil), err)
	}
	httpRequest, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.responses.request_build_failed", false, nil), err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "application/json")
	httpRequest.Header.Set("Authorization", "Bearer "+a.apiKey.Value())

	httpResponse, err := a.doer.Do(httpRequest)
	if err != nil {
		return fail(classifyAdapterTransportError(err, callCtx), err)
	}
	if httpResponse == nil {
		return fail(adapterError(ErrorUpstream, "llm.responses.empty_http_response", false, nil), ErrInvalidResponsesResponse)
	}
	if httpResponse.Body == nil {
		return fail(adapterError(ErrorInvalidResponse, "llm.responses.empty_body", false, nil), ErrInvalidResponsesResponse)
	}
	defer httpResponse.Body.Close()
	result.GatewayRequestID = safeProtocolValue(httpResponse.Header.Get("X-Request-ID"), 256)
	result.UpstreamRequestID = safeProtocolValue(httpResponse.Header.Get("Request-Id"), 256)

	responseBody, readErr := io.ReadAll(io.LimitReader(httpResponse.Body, a.provider.MaxResponseBytes+1))
	if readErr != nil {
		return fail(classifyAdapterTransportError(readErr, callCtx), readErr)
	}
	if int64(len(responseBody)) > a.provider.MaxResponseBytes {
		return fail(adapterError(ErrorResourceLimit, "llm.responses.response_too_large", false, nil), ErrInvalidResponsesResponse)
	}
	if httpResponse.StatusCode < 200 || httpResponse.StatusCode >= 300 {
		return fail(classifyAdapterHTTPError(httpResponse.StatusCode, httpResponse.Header, a.clock.Now()), fmt.Errorf("upstream HTTP status %d", httpResponse.StatusCode))
	}

	parsed, parseErr := ParseResponsesResponse(responseBody, request)
	parsed.StartedAt = startedAt
	parsed.FinishedAt = a.clock.Now().UTC()
	parsed.DurationMS = parsed.FinishedAt.Sub(startedAt).Milliseconds()
	parsed.GatewayRequestID = result.GatewayRequestID
	parsed.UpstreamRequestID = result.UpstreamRequestID
	if parseErr != nil {
		if errors.Is(parseErr, ErrResponsesSchemaMismatch) {
			info := schemaMismatchError()
			parsed.Error = info
			return parsed, &AdapterError{Info: info, cause: parseErr}
		}
		info := invalidResponsesError("llm.responses.invalid_payload")
		parsed.Error = info
		return parsed, &AdapterError{Info: info, cause: parseErr}
	}
	return parsed, nil
}

func (a *ResponsesAdapter) effectiveDeadline(parent context.Context, requestDeadline time.Time, startedAt time.Time) time.Time {
	maxDuration := time.Duration(1<<63 - 1)
	maxTimeoutMS := int64(maxDuration / time.Millisecond)
	timeout := maxDuration
	if a.provider.RequestTimeoutMS <= maxTimeoutMS {
		timeout = time.Duration(a.provider.RequestTimeoutMS) * time.Millisecond
	}
	deadline := startedAt.Add(timeout)
	if requestDeadline.Before(deadline) {
		deadline = requestDeadline
	}
	if parentDeadline, ok := parent.Deadline(); ok && parentDeadline.Before(deadline) {
		deadline = parentDeadline
	}
	return deadline
}

func responsesEndpoint(baseURL string) (string, error) {
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "/v1" {
		return "", errors.New("invalid Responses Gateway base URL")
	}
	parsed.Path = "/v1/responses"
	return parsed.String(), nil
}

func classifyAdapterTransportError(err error, ctx context.Context) *ModelErrorInfo {
	if mapped := ModelErrorFromPlatformFault(err); mapped != nil {
		return mapped
	}
	if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
		return adapterError(ErrorCancelled, "llm.http.cancelled", false, nil)
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return adapterError(ErrorTimeout, "llm.http.timeout", true, nil)
	}
	return adapterError(ErrorUpstreamUnavailable, "llm.http.request_failed", true, nil)
}

func classifyAdapterHTTPError(status int, headers http.Header, now time.Time) *ModelErrorInfo {
	kind := ErrorUpstream
	retryable := false
	switch status {
	case http.StatusUnauthorized, http.StatusForbidden:
		kind = ErrorUnauthorized
	case http.StatusTooManyRequests:
		kind, retryable = ErrorRateLimited, true
	case http.StatusRequestEntityTooLarge:
		kind = ErrorResourceLimit
	case http.StatusRequestTimeout, http.StatusGatewayTimeout:
		kind, retryable = ErrorTimeout, true
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		kind = ErrorInvalidRequest
	case http.StatusServiceUnavailable:
		kind, retryable = ErrorUpstreamUnavailable, true
	case http.StatusBadGateway:
		kind, retryable = ErrorUpstream, true
	default:
		if status >= 500 {
			retryable = true
		} else if status >= 400 {
			kind = ErrorInvalidRequest
		}
	}
	info := adapterError(kind, fmt.Sprintf("llm.http.status_%d", status), retryable, &status)
	if retryAfter := parseRetryAfter(headers.Get("Retry-After"), now); retryAfter > 0 {
		info.RetryAfterSeconds = &retryAfter
	}
	return info
}

func parseRetryAfter(value string, now time.Time) int64 {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
		return seconds
	}
	if retryAt, err := http.ParseTime(value); err == nil {
		seconds := int64(retryAt.Sub(now).Seconds())
		if seconds > 0 {
			return seconds
		}
	}
	return 0
}

func adapterError(kind ModelErrorKind, code string, retryable bool, status *int) *ModelErrorInfo {
	return &ModelErrorInfo{
		Kind: kind, StableCode: code, Retryable: retryable,
		HTTPStatus: status, SafeMessage: "LLM provider request failed",
	}
}
