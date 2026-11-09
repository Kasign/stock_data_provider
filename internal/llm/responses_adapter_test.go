package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/config"
)

type fixedTestClock struct {
	now time.Time
}

func (c fixedTestClock) Now() time.Time {
	return c.now
}

type steppingTestClock struct {
	now  time.Time
	step time.Duration
}

func (c *steppingTestClock) Now() time.Time {
	current := c.now
	c.now = c.now.Add(c.step)
	return current
}

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func adapterTestConfig() (ProviderConfig, config.Secret, CapabilityState, Clock) {
	provider := testProviderConfig()
	now := time.Now().UTC().Add(time.Second)
	verifiedAt := now.Add(-time.Minute)
	validUntil := now.Add(time.Hour)
	clock := fixedTestClock{now: now}
	capabilities := CapabilityState{
		ProviderID: provider.ProviderID, ModelID: provider.DefaultModel,
		Protocol: provider.Protocol, GatewayVersion: "gateway-v1",
		VerifiedAt: &verifiedAt, ValidUntil: &validUntil,
		VerificationMethod: "automated_e2e", EvidenceRef: "fixture://gateway-v1/qwen-local",
		MaxOutputTokensLimit: 1024,
		Values: map[Capability]CapabilityStatus{
			CapabilityResponsesJSON: CapabilitySupported,
		},
	}
	secret := config.NewEnvWithLookup(func(key string) (string, bool) {
		return "gateway-caller-secret", key == "LLM_GATEWAY_KEY"
	}).Secret("LLM_GATEWAY_KEY")
	return provider, secret, capabilities, clock
}

func TestResponsesAdapterRejectsCapabilityEvidenceBeyondProviderTTL(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	validUntil := capabilities.VerifiedAt.Add(time.Duration(provider.CapabilityTTLMS)*time.Millisecond + time.Nanosecond)
	capabilities.ValidUntil = &validUntil
	if _, err := NewResponsesAdapter(provider, secret, doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("must not be called")
	}), capabilities, clock); err == nil {
		t.Fatal("adapter must reject capability evidence exceeding configured Provider TTL")
	}
}

func TestResponsesAdapterRechecksCapabilityExpiryBeforeDispatch(t *testing.T) {
	provider, secret, capabilities, _ := adapterTestConfig()
	base := time.Now().UTC().Add(time.Second)
	clock := &steppingTestClock{now: base, step: 2 * time.Hour}
	var dispatched bool
	adapter, err := NewResponsesAdapter(provider, secret, doerFunc(func(*http.Request) (*http.Response, error) {
		dispatched = true
		return nil, errors.New("must not dispatch with expired capability evidence")
	}), capabilities, clock)
	if err != nil {
		t.Fatalf("adapter should accept evidence valid at construction: %v", err)
	}
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	_, err = adapter.Generate(context.Background(), request)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorUnsupportedCapability || dispatched {
		t.Fatalf("expired evidence must block dispatch: err=%v dispatched=%v", err, dispatched)
	}
}

func TestResponsesAdapterGenerateUsesInjectedPlatformDoer(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID

	var sawRequest bool
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		sawRequest = true
		if req.Method != http.MethodPost || req.URL.String() != "https://gateway.example.com:8443/v1/responses" {
			t.Fatalf("unexpected request target: %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Authorization") != "Bearer gateway-caller-secret" {
			t.Fatal("Gateway caller key was not injected")
		}
		if req.Header.Get("Content-Type") != "application/json" {
			t.Fatal("missing JSON content type")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"X-Request-Id": []string{"gateway-req-1"},
				"Request-Id":   []string{"upstream-req-1"},
			},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"resp-1","object":"response","model":"qwen-actual","status":"completed",
				"output":[{"id":"msg-1","type":"message","content":[{"type":"output_text","text":"ok"}]}]
			}`)),
		}, nil
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.Generate(context.Background(), request)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}
	if !sawRequest || got.Status != ResponseCompleted || got.OutputText != "ok" {
		t.Fatalf("unexpected adapter result: %+v", got)
	}
	if got.GatewayRequestID != "gateway-req-1" || got.UpstreamRequestID != "upstream-req-1" {
		t.Fatalf("Gateway/upstream request IDs were conflated: %+v", got)
	}
	if got.StartedAt.IsZero() || got.FinishedAt.IsZero() || got.DurationMS < 0 {
		t.Fatalf("call timing metadata missing: %+v", got)
	}
}

func TestResponsesAdapterMapsHTTPStatusAndRetryAfter(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     http.Header{"Retry-After": []string{"30"}},
			Body:       io.NopCloser(strings.NewReader(`{"error":{"type":"rate_limit_error"}}`)),
		}, nil
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Generate(context.Background(), request)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorRateLimited ||
		adapterErr.Info.HTTPStatus == nil || *adapterErr.Info.HTTPStatus != http.StatusTooManyRequests ||
		adapterErr.Info.RetryAfterSeconds == nil || *adapterErr.Info.RetryAfterSeconds != 30 {
		t.Fatalf("HTTP 429 mapping is incomplete: %#v, %v", adapterErr, err)
	}
}

func TestResponsesAdapterDoesNotExposeTransportError(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("Authorization: Bearer gateway-caller-secret")
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	_, err = adapter.Generate(context.Background(), request)
	if err == nil || strings.Contains(err.Error(), "gateway-caller-secret") {
		t.Fatalf("transport error leaked credential: %v", err)
	}
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorUpstreamUnavailable {
		t.Fatalf("transport error not normalized: %v", err)
	}
}

func TestResponsesAdapterUsesEarliestRequestContextAndProviderDeadline(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	request.DeadlineAt = clock.Now().Add(20 * time.Second)

	ctx, cancel := context.WithDeadline(context.Background(), clock.Now().Add(5*time.Second))
	defer cancel()
	var observedDeadline time.Time
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		var ok bool
		observedDeadline, ok = req.Context().Deadline()
		if !ok {
			t.Fatal("request context must have a deadline")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"resp-deadline","object":"response","model":"qwen-local","status":"completed","output":[]
			}`)),
		}, nil
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Generate(ctx, request); err != nil {
		t.Fatalf("later request deadline must be bounded by earlier Context deadline: %v", err)
	}
	if !observedDeadline.Equal(clock.Now().Add(5 * time.Second)) {
		t.Fatalf("Context deadline did not win: got %s", observedDeadline)
	}

	provider.RequestTimeoutMS = 500
	request.DeadlineAt = clock.Now().Add(20 * time.Second)
	adapter, err = NewResponsesAdapter(provider, secret, doerFunc(func(req *http.Request) (*http.Response, error) {
		var ok bool
		observedDeadline, ok = req.Context().Deadline()
		if !ok {
			t.Fatal("request context must have a deadline")
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{
			"id":"resp-provider-deadline","object":"response","model":"qwen-local","status":"completed","output":[]
		}`))}, nil
	}), capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Generate(context.Background(), request); err != nil {
		t.Fatalf("Provider timeout should bound request: %v", err)
	}
	if !observedDeadline.Equal(clock.Now().Add(500 * time.Millisecond)) {
		t.Fatalf("Provider timeout did not win: got %s", observedDeadline)
	}
}

func TestResponsesAdapterUsesRequestDeadlineWhenItIsEarliest(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	request.DeadlineAt = clock.Now().Add(2 * time.Second)
	var observed time.Time
	doer := doerFunc(func(req *http.Request) (*http.Response, error) {
		observed, _ = req.Context().Deadline()
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"resp-request-deadline","object":"response","model":"qwen-local","status":"completed","output":[]
			}`)),
		}, nil
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), clock.Now().Add(5*time.Second))
	defer cancel()
	if _, err := adapter.Generate(ctx, request); err != nil {
		t.Fatal(err)
	}
	if !observed.Equal(request.DeadlineAt) {
		t.Fatalf("request deadline should win: got %s want %s", observed, request.DeadlineAt)
	}
}

func TestResponsesAdapterUsesInjectedClockForTiming(t *testing.T) {
	provider, secret, capabilities, baseClock := adapterTestConfig()
	clock := &steppingTestClock{now: baseClock.Now(), step: 125 * time.Millisecond}
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK, Header: http.Header{},
			Body: io.NopCloser(strings.NewReader(`{
				"id":"resp-clock","object":"response","model":"qwen-local","status":"completed","output":[]
			}`)),
		}, nil
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if got.DurationMS != 125 {
		t.Fatalf("duration must use injected clock: got %dms", got.DurationMS)
	}
}

func TestResponsesAdapterDoesNotDispatchWhenEffectiveDeadlineExpired(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	request.DeadlineAt = clock.Now().Add(time.Minute)
	called := false
	adapter, err := NewResponsesAdapter(provider, secret, doerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not dispatch")
	}), capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), clock.Now().Add(-time.Second))
	defer cancel()
	_, err = adapter.Generate(ctx, request)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorTimeout || called {
		t.Fatalf("expired effective deadline must stop before dispatch: err=%v called=%v", err, called)
	}
}

func TestResponsesAdapterRejectsUnconfiguredSecretOrDoer(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	if _, err := NewResponsesAdapter(provider, config.Secret{}, doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	}), capabilities, clock); err == nil {
		t.Fatal("missing Secret must be rejected")
	}
	if _, err := NewResponsesAdapter(provider, secret, nil, capabilities, clock); err == nil {
		t.Fatal("missing Platform-bound doer must be rejected")
	}
	if _, err := NewResponsesAdapter(provider, secret, doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	}), capabilities, nil); err == nil {
		t.Fatal("missing Clock must be rejected")
	}
}

func TestResponsesAdapterBlocksOptionalFeaturesWithoutCapabilityEvidence(t *testing.T) {
	cases := []struct {
		name      string
		configure func(*ModelRequest)
	}{
		{
			name: "sampling parameters",
			configure: func(request *ModelRequest) {
				temperature := 0.2
				request.Temperature = &temperature
			},
		},
		{
			name: "reasoning effort",
			configure: func(request *ModelRequest) {
				request.ReasoningEffort = "high"
			},
		},
		{
			name: "structured output",
			configure: func(request *ModelRequest) {
				request.ResponseSchema = &JSONSchemaSpec{
					Name:       "summary",
					Strict:     true,
					SchemaJSON: json.RawMessage(`{"type":"object","properties":{"summary":{"type":"string"}}}`),
				}
			},
		},
		{
			name: "function tool calls",
			configure: func(request *ModelRequest) {
				request.Tools = []ToolDefinition{{
					Name:           "lookup_quote",
					ParametersJSON: json.RawMessage(`{"type":"object","properties":{"symbol":{"type":"string"}}}`),
				}}
				request.ToolChoice = "auto"
			},
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			provider, secret, capabilities, clock := adapterTestConfig()
			request := validResponsesRequest()
			request.ProviderID = provider.ProviderID
			tt.configure(&request)
			dispatched := false
			adapter, err := NewResponsesAdapter(provider, secret, doerFunc(func(*http.Request) (*http.Response, error) {
				dispatched = true
				return nil, errors.New("must not dispatch without capability evidence")
			}), capabilities, clock)
			if err != nil {
				t.Fatal(err)
			}
			_, err = adapter.Generate(context.Background(), request)
			var adapterErr *AdapterError
			if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorUnsupportedCapability || dispatched {
				t.Fatalf("feature without current capability evidence must be blocked: err=%v dispatched=%v", err, dispatched)
			}
		})
	}
}

// TestResponsesAdapterRejectsUnsupportedStrictSchemaBeforeDispatch 验证不支持的 strict Schema 在网络派发前被拒绝。
func TestResponsesAdapterRejectsUnsupportedStrictSchemaBeforeDispatch(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	capabilities.Values[CapabilityStructuredOutput] = CapabilitySupported
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	request.ResponseSchema = &JSONSchemaSpec{
		Name: "summary", Strict: true,
		SchemaJSON: json.RawMessage(`{"type":"object","properties":{},"required":[],"additionalProperties":false,"allOf":[{"type":"object","properties":{},"required":[],"additionalProperties":false}]}`),
	}
	dispatched := false
	adapter, err := NewResponsesAdapter(provider, secret, doerFunc(func(*http.Request) (*http.Response, error) {
		dispatched = true
		return nil, errors.New("must not dispatch")
	}), capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Generate(context.Background(), request)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorInvalidRequest ||
		adapterErr.Info.StableCode != "llm.responses.unsupported_schema_subset" {
		t.Fatalf("unsupported strict schema must map to a stable invalid_request: %v", err)
	}
	if dispatched {
		t.Fatal("unsupported strict schema must be rejected before HTTP dispatch")
	}
}

// closeTrackingReader 用于验证 Adapter 在传输层同时返回响应和错误时释放 Body。
type closeTrackingReader struct {
	io.Reader
	closed bool
}

func (r *closeTrackingReader) Close() error {
	r.closed = true
	return nil
}

func TestResponsesAdapterStreamClosesBodyReturnedWithError(t *testing.T) {
	provider, secret, capabilities, clock := streamAdapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	body := &closeTrackingReader{Reader: strings.NewReader("")}
	streamDoer := HTTPStreamDoerFunc(func(context.Context, *http.Request) (*HTTPStreamResponse, error) {
		return &HTTPStreamResponse{
			StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: body,
		}, errors.New("open stream failed")
	})
	adapter, err := NewResponsesAdapterWithStream(provider, secret,
		doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") }),
		streamDoer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}

	_, err = adapter.Stream(context.Background(), request, nil)
	if err == nil || !body.closed {
		t.Fatalf("body returned with transport error must be closed: err=%v closed=%v", err, body.closed)
	}
}

// cancelAfterChunkReader 在交付首段 SSE 数据后取消上下文，模拟读取期间连接被取消。
type cancelAfterChunkReader struct {
	data    []byte
	sent    bool
	cancel  context.CancelFunc
	closed  bool
}

func (r *cancelAfterChunkReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		n := copy(p, r.data)
		r.cancel()
		return n, nil
	}
	return 0, context.Canceled
}

func (r *cancelAfterChunkReader) Close() error {
	r.closed = true
	return nil
}

func TestResponsesAdapterStreamCancellationPreservesPartialOutput(t *testing.T) {
	provider, secret, capabilities, clock := streamAdapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	body := &cancelAfterChunkReader{
		data: []byte("event: response.output_text.delta\ndata: {\\"type\\":\\"response.output_text.delta\\",\\"delta\\":\\"partial\\"}\n\n"),
		cancel: cancel,
	}
	streamDoer := HTTPStreamDoerFunc(func(context.Context, *http.Request) (*HTTPStreamResponse, error) {
		return &HTTPStreamResponse{
			StatusCode: http.StatusOK,
			Header: http.Header{"Content-Type": []string{"text/event-stream"}},
			Body: body,
		}, nil
	})
	adapter, err := NewResponsesAdapterWithStream(provider, secret,
		doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") }),
		streamDoer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}

	got, err := adapter.Stream(ctx, request, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorCancelled ||
		adapterErr.Info.StableCode != "llm.http.cancelled" ||
		got.Status != ResponseIncomplete || got.OutputText != "partial" || !body.closed {
		t.Fatalf("mid-stream cancellation must preserve partial output and close body: response=%+v err=%v closed=%v", got, err, body.closed)
	}
}

func streamAdapterTestConfig() (ProviderConfig, config.Secret, CapabilityState, Clock) {
	provider, secret, capabilities, clock := adapterTestConfig()
	provider.StreamEnabled = true
	capabilities.Values[CapabilityResponsesStream] = CapabilitySupported
	return provider, secret, capabilities, clock
}

func TestResponsesAdapterStreamUsesPlatformStreamAndParsesIncrementally(t *testing.T) {
	provider, secret, capabilities, clock := streamAdapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	sse := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"streamed\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-stream\",\"object\":\"response\",\"model\":\"qwen-local\",\"status\":\"completed\",\"output\":[{\"id\":\"msg-stream\",\"type\":\"message\",\"content\":[{\"type\":\"output_text\",\"text\":\"streamed\"}]}]}}",
		"",
	}, "\n")
	var opened bool
	streamDoer := HTTPStreamDoerFunc(func(ctx context.Context, req *http.Request) (*HTTPStreamResponse, error) {
		opened = true
		if req.Method != http.MethodPost || req.URL.String() != "https://gateway.example.com:8443/v1/responses" {
			t.Fatalf("unexpected stream request target: %s %s", req.Method, req.URL)
		}
		if req.Header.Get("Accept") != "text/event-stream" ||
			req.Header.Get("Authorization") != "Bearer gateway-caller-secret" {
			t.Fatalf("stream headers are incomplete: %#v", req.Header)
		}
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		var wire map[string]any
		if err := json.Unmarshal(body, &wire); err != nil || wire["stream"] != true {
			t.Fatalf("stream request body must include stream=true: %s err=%v", body, err)
		}
		return &HTTPStreamResponse{
			StatusCode: http.StatusOK,
			Header: http.Header{
				"Content-Type": []string{"text/event-stream; charset=utf-8"},
				"X-Request-ID": []string{"gateway-stream-1"},
				"Request-Id":   []string{"upstream-stream-1"},
			},
			Body: io.NopCloser(strings.NewReader(sse)),
		}, nil
	})
	adapter, err := NewResponsesAdapterWithStream(provider, secret,
		doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") }),
		streamDoer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	var sawDelta bool
	got, err := adapter.Stream(context.Background(), request, func(event ModelEvent) error {
		if event.Type == "response.output_text.delta" && event.Delta == "streamed" {
			sawDelta = true
		}
		return nil
	})
	if err != nil || !opened || !sawDelta || got.Status != ResponseCompleted || got.OutputText != "streamed" {
		t.Fatalf("unexpected stream result: opened=%v delta=%v response=%+v err=%v", opened, sawDelta, got, err)
	}
	if got.GatewayRequestID != "gateway-stream-1" || got.UpstreamRequestID != "upstream-stream-1" {
		t.Fatalf("stream request IDs were not preserved: %+v", got)
	}
}

func TestResponsesAdapterStreamReturnsConsumerErrorUnchanged(t *testing.T) {
	provider, secret, capabilities, clock := streamAdapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	sse := strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}",
		"",
		"event: response.completed",
		"data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp-callback\",\"object\":\"response\",\"status\":\"completed\",\"output\":[]}}",
		"",
	}, "\n")
	callbackErr := &AdapterError{Info: adapterError(ErrorCancelled, "consumer.cancelled", false, nil)}
	streamDoer := HTTPStreamDoerFunc(func(context.Context, *http.Request) (*HTTPStreamResponse, error) {
		return &HTTPStreamResponse{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(sse))}, nil
	})
	adapter, err := NewResponsesAdapterWithStream(provider, secret,
		doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") }),
		streamDoer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.Stream(context.Background(), request, func(ModelEvent) error { return callbackErr })
	if err != callbackErr || got.Status != ResponseIncomplete || got.OutputText != "partial" {
		t.Fatalf("consumer error must pass through and preserve partial output: response=%+v err=%v", got, err)
	}
}

func TestResponsesAdapterStreamMapsPlatformReadFailureAndPreservesPartialOutput(t *testing.T) {
	provider, secret, capabilities, clock := streamAdapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	readErr := errors.New("upstream socket closed")
	partial := []byte(strings.Join([]string{
		"event: response.output_text.delta",
		"data: {\"type\":\"response.output_text.delta\",\"delta\":\"partial\"}",
		"",
	}, "\n"))
	streamDoer := HTTPStreamDoerFunc(func(context.Context, *http.Request) (*HTTPStreamResponse, error) {
		return &HTTPStreamResponse{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, Body: io.NopCloser(&errorAfterSSEDataReader{data: partial, err: readErr})}, nil
	})
	adapter, err := NewResponsesAdapterWithStream(provider, secret,
		doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") }),
		streamDoer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	got, err := adapter.Stream(context.Background(), request, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorUpstreamUnavailable ||
		got.Status != ResponseIncomplete || got.OutputText != "partial" {
		t.Fatalf("read failure must be normalized without losing partial output: response=%+v err=%v", got, err)
	}
}

// TestResponsesAdapterStreamDoesNotDispatchWithCancelledContext 验证已取消的调用上下文不会进入传输层。
func TestResponsesAdapterStreamDoesNotDispatchWithCancelledContext(t *testing.T) {
	provider, secret, capabilities, clock := streamAdapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	called := false
	streamDoer := HTTPStreamDoerFunc(func(context.Context, *http.Request) (*HTTPStreamResponse, error) {
		called = true
		return nil, errors.New("cancelled request must not be dispatched")
	})
	adapter, err := NewResponsesAdapterWithStream(provider, secret,
		doerFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("unused") }),
		streamDoer, capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := adapter.Stream(ctx, request, nil)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorCancelled ||
		adapterErr.Info.StableCode != "llm.http.cancelled" || got.Status != ResponseFailed || called {
		t.Fatalf("cancelled context must stop before transport dispatch: err=%v called=%v", err, called)
	}
}

// TestResponsesAdapterGenerateDoesNotDispatchWithCancelledContext 验证已取消的调用上下文不会进入传输层。
func TestResponsesAdapterGenerateDoesNotDispatchWithCancelledContext(t *testing.T) {
	provider, secret, capabilities, clock := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	called := false
	adapter, err := NewResponsesAdapter(provider, secret, doerFunc(func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("cancelled request must not be dispatched")
	}), capabilities, clock)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	got, err := adapter.Generate(ctx, request)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.Kind != ErrorCancelled ||
		adapterErr.Info.StableCode != "llm.http.cancelled" || got.Status != ResponseFailed || called {
		t.Fatalf("cancelled context must stop before transport dispatch: err=%v called=%v", err, called)
	}
}
