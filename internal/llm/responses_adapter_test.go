package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/platform/config"
)

type doerFunc func(*http.Request) (*http.Response, error)

func (f doerFunc) Do(request *http.Request) (*http.Response, error) {
	return f(request)
}

func adapterTestConfig() (ProviderConfig, config.Secret, CapabilityState) {
	provider := testProviderConfig()
	now := time.Now().UTC()
	validUntil := now.Add(time.Hour)
	capabilities := CapabilityState{
		ProviderID: provider.ProviderID, ModelID: provider.DefaultModel,
		Protocol: provider.Protocol, GatewayVersion: "gateway-v1",
		VerifiedAt: &now, ValidUntil: &validUntil,
		EvidenceRef: "fixture://gateway-v1/qwen-local",
		MaxOutputTokensLimit: 1024,
		Values: map[Capability]CapabilityStatus{
			CapabilityResponsesJSON: CapabilitySupported,
		},
	}
	secret := config.NewEnvWithLookup(func(key string) (string, bool) {
		return "gateway-caller-secret", key == "LLM_GATEWAY_KEY"
	}).Secret("LLM_GATEWAY_KEY")
	return provider, secret, capabilities
}

func TestResponsesAdapterGenerateUsesInjectedPlatformDoer(t *testing.T) {
	provider, secret, capabilities := adapterTestConfig()
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
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities)
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
	provider, secret, capabilities := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header: http.Header{"Retry-After": []string{"30"}},
			Body: io.NopCloser(strings.NewReader(`{"error":{"type":"rate_limit_error"}}`)),
		}, nil
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities)
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
	provider, secret, capabilities := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("Authorization: Bearer gateway-caller-secret")
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities)
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


func TestResponsesAdapterRejectsDeadlineBeyondCallerContext(t *testing.T) {
	provider, secret, capabilities := adapterTestConfig()
	request := validResponsesRequest()
	request.ProviderID = provider.ProviderID
	request.DeadlineAt = time.Now().UTC().Add(time.Minute)
	doerCalled := false
	doer := doerFunc(func(*http.Request) (*http.Response, error) {
		doerCalled = true
		return nil, errors.New("must not be called")
	})
	adapter, err := NewResponsesAdapter(provider, secret, doer, capabilities)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(10*time.Second))
	defer cancel()
	_, err = adapter.Generate(ctx, request)
	var adapterErr *AdapterError
	if !errors.As(err, &adapterErr) || adapterErr.Info.StableCode != "llm.request.deadline_exceeds_context" || doerCalled {
		t.Fatalf("request deadline beyond caller context was not rejected: %v", err)
	}
}

func TestResponsesAdapterRejectsUnconfiguredSecretOrDoer(t *testing.T) {
	provider, secret, capabilities := adapterTestConfig()
	if _, err := NewResponsesAdapter(provider, config.Secret{}, doerFunc(func(*http.Request) (*http.Response, error) {
		return nil, nil
	}), capabilities); err == nil {
		t.Fatal("missing Secret must be rejected")
	}
	if _, err := NewResponsesAdapter(provider, secret, nil, capabilities); err == nil {
		t.Fatal("missing Platform-bound doer must be rejected")
	}
}
