package llm

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"time"
)

// HTTPStreamResponse 是 LLM 与 Platform 受控流式传输之间的窄响应契约。
// Body 所有权交给 LLM Adapter；Adapter 在解析结束、失败或取消时负责关闭。
type HTTPStreamResponse struct {
	StatusCode int
	Header     http.Header
	Body       io.ReadCloser
}

// HTTPStreamDoer 由组合根包装 Platform BoundDoer.OpenStream 提供。
// LLM 不创建 HTTP Client，也不接收可变 DestinationPolicy。
type HTTPStreamDoer interface {
	OpenStream(context.Context, *http.Request) (*HTTPStreamResponse, error)
}

// HTTPStreamDoerFunc 便于在组合根和单元测试中注入受控流式入口。
type HTTPStreamDoerFunc func(context.Context, *http.Request) (*HTTPStreamResponse, error)

func (f HTTPStreamDoerFunc) OpenStream(ctx context.Context, request *http.Request) (*HTTPStreamResponse, error) {
	return f(ctx, request)
}

// Stream 执行单次 Responses SSE 请求。事件回调同步执行，以调用方消费速度形成背压；本方法不重试。
func (a *ResponsesAdapter) Stream(ctx context.Context, request ModelRequest, onEvent func(ModelEvent) error) (ModelResponse, error) {
	startedAt := a.clock.Now().UTC()
	result := ModelResponse{
		RequestID: request.RequestID, CorrelationID: request.CorrelationID,
		SystemAsOf: request.SystemAsOf, ProviderID: request.ProviderID,
		ModelRequested: request.Model, Status: ResponseIncomplete,
		OutputItems: []OutputItem{}, StartedAt: startedAt,
	}
	fail := func(info *ModelErrorInfo, cause error) (ModelResponse, error) {
		result.Status = ResponseFailed
		result.Error = info
		result.FinishedAt = a.clock.Now().UTC()
		result.DurationMS = result.FinishedAt.Sub(startedAt).Milliseconds()
		return result, &AdapterError{Info: info, cause: cause}
	}
	if ctx == nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.request.invalid_context", false, nil), ErrInvalidResponsesRequest)
	}
	if a.streamDoer == nil || !a.provider.StreamEnabled {
		return fail(adapterError(ErrorUnsupportedCapability, "llm.responses.stream_not_configured", false, nil), ErrInvalidResponsesRequest)
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
	if err := request.ValidateStreamCapabilitiesAt(a.capabilities, startedAt); err != nil {
		return fail(adapterError(ErrorUnsupportedCapability, "llm.provider.stream_capability_unavailable", false, nil), err)
	}

	body, err := MapResponsesStreamRequestAt(request, startedAt)
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
	if err := callCtx.Err(); err != nil {
		return fail(classifyAdapterTransportError(err, callCtx), err)
	}

	endpoint, err := responsesEndpoint(a.provider.BaseURL)
	if err != nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.provider.invalid_endpoint", false, nil), err)
	}
	httpRequest, err := http.NewRequestWithContext(callCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fail(adapterError(ErrorInvalidRequest, "llm.responses.request_build_failed", false, nil), err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	httpRequest.Header.Set("Accept", "text/event-stream")
	httpRequest.Header.Set("Authorization", "Bearer "+a.apiKey.Value())

	stream, err := a.streamDoer.OpenStream(callCtx, httpRequest)
	if err != nil {
		// 遵循 HTTP Doer 约定：若错误同时返回了响应体，仍由 Adapter 释放资源。
		if stream != nil && stream.Body != nil {
			_ = stream.Body.Close()
		}
		info := ModelErrorFromPlatformFault(err)
		if info == nil {
			info = classifyAdapterTransportError(err, callCtx)
		}
		return fail(info, err)
	}
	if stream == nil || stream.Body == nil {
		return fail(adapterError(ErrorInvalidResponse, "llm.responses.empty_stream", false, nil), ErrInvalidResponsesResponse)
	}
	defer stream.Body.Close()

	result.GatewayRequestID = safeProtocolValue(stream.Header.Get("X-Request-ID"), 256)
	result.UpstreamRequestID = safeProtocolValue(stream.Header.Get("Request-Id"), 256)
	if stream.StatusCode < 200 || stream.StatusCode >= 300 {
		return fail(classifyAdapterHTTPError(stream.StatusCode, stream.Header, a.clock.Now()), fmt.Errorf("upstream HTTP status %d", stream.StatusCode))
	}
	mediaType, _, mediaErr := mime.ParseMediaType(stream.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "text/event-stream" {
		return fail(adapterError(ErrorInvalidResponse, "llm.responses.invalid_stream_content_type", false, nil), ErrInvalidResponsesResponse)
	}

	var callbackErr error
	var callback func(ModelEvent) error
	if onEvent != nil {
		callback = func(event ModelEvent) error {
			if err := onEvent(event); err != nil {
				callbackErr = err
				return err
			}
			return nil
		}
	}
	parsed, parseErr := ParseResponsesSSEReader(stream.Body, request, a.clock, callback)
	parsed.StartedAt = startedAt
	parsed.GatewayRequestID = result.GatewayRequestID
	parsed.UpstreamRequestID = result.UpstreamRequestID
	parsed.FinishedAt = a.clock.Now().UTC()
	parsed.DurationMS = parsed.FinishedAt.Sub(startedAt).Milliseconds()

	// 消费者错误必须原样返回，不能因为其类型恰好是 Platform Fault 而重新分类。
	if callbackErr != nil {
		return parsed, callbackErr
	}
	if parseErr != nil {
		var parserErr *AdapterError
		if errors.As(parseErr, &parserErr) {
			if parsed.Error == nil {
				parsed.Error = parserErr.Info
			}
			return parsed, parseErr
		}
		info := ModelErrorFromPlatformFault(parseErr)
		if info == nil {
			info = classifyAdapterTransportError(parseErr, callCtx)
		}
		parsed.Status = ResponseIncomplete
		parsed.Error = info
		return parsed, &AdapterError{Info: info, cause: parseErr}
	}
	return parsed, nil
}
