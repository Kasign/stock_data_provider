package llmtransport

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/llm"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
)

type fakeBoundDoer struct {
	response       *http.Response
	doErr          error
	streamResponse *platformhttpclient.StreamResponse
	streamErr      error
}

func (f *fakeBoundDoer) Do(*http.Request) (*http.Response, error) {
	return f.response, f.doErr
}
func (f *fakeBoundDoer) OpenStream(context.Context, *http.Request) (*platformhttpclient.StreamResponse, error) {
	return f.streamResponse, f.streamErr
}

func TestValidateProviderPolicyRequiresExactHTTPSDestination(t *testing.T) {
	provider := llm.ProviderConfig{
		ProviderID: "gateway", Protocol: llm.ProtocolOpenAIResponses,
		BaseURL: "https://ai.example.com/v1", DefaultModel: "model",
		APIKeySecretRef: "LLM_API_KEY", RequestTimeoutMS: 1000,
		CapabilityTTLMS: 1000, AllowedModels: []string{"model"},
		MaxRequestBytes: 1024, MaxResponseBytes: 1024,
	}
	valid := platformhttpclient.DestinationPolicy{
		Dependency: "llm.gateway", AllowedHosts: []string{"ai.example.com"},
		AllowedPorts: []uint16{443},
	}
	if err := validateProviderPolicy(provider, valid); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*platformhttpclient.DestinationPolicy)
	}{
		{"host mismatch", func(p *platformhttpclient.DestinationPolicy) { p.AllowedHosts = []string{"other.example.com"} }},
		{"multiple hosts", func(p *platformhttpclient.DestinationPolicy) { p.AllowedHosts = append(p.AllowedHosts, "other.example.com") }},
		{"port mismatch", func(p *platformhttpclient.DestinationPolicy) { p.AllowedPorts = []uint16{8443} }},
		{"redirects enabled", func(p *platformhttpclient.DestinationPolicy) { p.AllowRedirects = true }},
		{"transport retries enabled", func(p *platformhttpclient.DestinationPolicy) { p.Idempotent = true }},
		{"http enabled", func(p *platformhttpclient.DestinationPolicy) { p.AllowHTTP = true }},
		{"dependency missing", func(p *platformhttpclient.DestinationPolicy) { p.Dependency = "" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := valid
			policy.AllowedHosts = append([]string(nil), valid.AllowedHosts...)
			policy.AllowedPorts = append([]uint16(nil), valid.AllowedPorts...)
			tt.mutate(&policy)
			if err := validateProviderPolicy(provider, policy); err == nil {
				t.Fatal("expected policy validation error")
			}
		})
	}
}

func TestValidateProviderPolicyRequiresExplicitNonStandardPort(t *testing.T) {
	provider := llm.ProviderConfig{
		ProviderID: "gateway", Protocol: llm.ProtocolOpenAIResponses,
		BaseURL: "https://ai.example.com:8443/v1", DefaultModel: "model",
		APIKeySecretRef: "LLM_API_KEY", RequestTimeoutMS: 1000,
		CapabilityTTLMS: 1000, AllowedModels: []string{"model"},
		MaxRequestBytes: 1024, MaxResponseBytes: 1024,
	}
	policy := platformhttpclient.DestinationPolicy{Dependency: "llm.gateway", AllowedHosts: []string{"ai.example.com"}}
	if err := validateProviderPolicy(provider, policy); err == nil {
		t.Fatal("non-standard port without allowlist was accepted")
	}
	policy.AllowedPorts = []uint16{8443}
	if err := validateProviderPolicy(provider, policy); err != nil {
		t.Fatalf("explicit non-standard port rejected: %v", err)
	}
}

func TestTransportOpenStreamTransfersBodyOwnershipAndHeaders(t *testing.T) {
	body := io.NopCloser(strings.NewReader("event: done\n\n"))
	header := http.Header{"Content-Type": []string{"text/event-stream"}, "X-Request-Id": []string{"req-1"}}
	fake := &fakeBoundDoer{streamResponse: &platformhttpclient.StreamResponse{
		StatusCode: http.StatusOK, Header: header, Body: body,
	}}
	transport := &Transport{bound: fake}
	got, err := transport.OpenStream(context.Background(), httptestRequest(t))
	if err != nil {
		t.Fatalf("OpenStream() error = %v", err)
	}
	if got == nil || got.StatusCode != http.StatusOK || got.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("unexpected stream response: %#v", got)
	}
	if got.Body != body {
		t.Fatal("stream body ownership was not transferred unchanged")
	}
	header.Set("X-Request-Id", "mutated")
	if got.Header.Get("X-Request-Id") != "req-1" {
		t.Fatal("response headers were not isolated from the Platform response")
	}
}

func TestTransportOpenStreamPreservesResponseAndError(t *testing.T) {
	body := io.NopCloser(strings.NewReader("partial"))
	openErr := errors.New("stream open failed")
	transport := &Transport{bound: &fakeBoundDoer{
		streamResponse: &platformhttpclient.StreamResponse{StatusCode: http.StatusBadGateway, Body: body},
		streamErr: openErr,
	}}
	got, err := transport.OpenStream(context.Background(), httptestRequest(t))
	if !errors.Is(err, openErr) {
		t.Fatalf("OpenStream() error = %v, want wrapped open error", err)
	}
	if got == nil || got.Body != body {
		t.Fatal("response body must be preserved so the LLM Adapter can close it")
	}
}

func TestTransportDoDelegatesToBoundDoer(t *testing.T) {
	response := &http.Response{StatusCode: http.StatusNoContent, Body: http.NoBody}
	fake := &fakeBoundDoer{response: response}
	transport := &Transport{bound: fake}
	got, err := transport.Do(httptestRequest(t))
	if err != nil || got != response {
		t.Fatalf("Do() = (%v, %v), want delegated response", got, err)
	}
}

func TestTransportRejectsUnconfiguredUse(t *testing.T) {
	var transport *Transport
	if _, err := transport.Do(httptestRequest(t)); err == nil {
		t.Fatal("nil transport Do should fail")
	}
	if _, err := transport.OpenStream(context.Background(), httptestRequest(t)); err == nil {
		t.Fatal("nil transport OpenStream should fail")
	}
}

func httptestRequest(t *testing.T) *http.Request {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, "https://ai.example.com/v1/responses", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	return request
}
