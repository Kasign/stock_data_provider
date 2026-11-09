// Package llmtransport adapts the Platform-bound HTTP transport to the LLM ports.
package llmtransport

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"example.com/stock-analysis-flow/internal/llm"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

type boundDoer interface {
	Do(*http.Request) (*http.Response, error)
	OpenStream(context.Context, *http.Request) (*platformhttpclient.StreamResponse, error)
}

// Transport exposes one immutable Platform destination binding through both
// LLM HTTP ports. It does not own or close the shared Platform Client.
type Transport struct {
	bound boundDoer
}

var (
	_ llm.HTTPDoer       = (*Transport)(nil)
	_ llm.HTTPStreamDoer = (*Transport)(nil)
)

// New validates that the fixed destination policy matches the configured LLM
// Gateway, then binds it to the shared Platform Client owned by Server.
func New(resources *serverresources.Resources, provider llm.ProviderConfig, policy platformhttpclient.DestinationPolicy) (*Transport, error) {
	if err := validateProviderPolicy(provider, policy); err != nil {
		return nil, err
	}
	if resources == nil {
		return nil, errors.New("Server resources are required for LLM transport")
	}
	bound, err := resources.BindHTTPDestination(policy)
	if err != nil {
		return nil, fmt.Errorf("bind LLM Gateway destination: %w", err)
	}
	return &Transport{bound: bound}, nil
}

func validateProviderPolicy(provider llm.ProviderConfig, policy platformhttpclient.DestinationPolicy) error {
	if err := provider.Validate(); err != nil {
		return fmt.Errorf("invalid LLM provider configuration: %w", err)
	}
	if strings.TrimSpace(policy.Dependency) == "" {
		return errors.New("LLM Gateway dependency identifier is required")
	}
	if policy.AllowHTTP || policy.AllowRedirects || policy.MaxRedirects != 0 || policy.Idempotent {
		return errors.New("LLM Gateway policy must require HTTPS, reject redirects, and disable transport retries")
	}
	if len(policy.AllowedHosts) != 1 {
		return errors.New("LLM Gateway policy must allow exactly the configured host")
	}
	target, err := url.Parse(provider.BaseURL)
	if err != nil || target == nil {
		return errors.New("invalid LLM Gateway base URL")
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	allowedHost := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(policy.AllowedHosts[0])), ".")
	if host == "" || host != allowedHost {
		return errors.New("LLM Gateway policy host does not match provider configuration")
	}
	port := target.Port()
	if port == "" {
		port = "443"
	}
	portNumber, err := strconv.ParseUint(port, 10, 16)
	if err != nil || portNumber == 0 {
		return errors.New("invalid LLM Gateway destination port")
	}
	if len(policy.AllowedPorts) > 1 {
		return errors.New("LLM Gateway policy must allow at most one destination port")
	}
	if len(policy.AllowedPorts) == 1 && uint64(policy.AllowedPorts[0]) != portNumber {
		return errors.New("LLM Gateway policy port does not match provider configuration")
	}
	if len(policy.AllowedPorts) == 0 && portNumber != 443 {
		return errors.New("non-standard LLM Gateway port must be explicitly allowlisted")
	}
	return nil
}

// Do delegates non-streaming requests to the same immutable destination binding.
func (t *Transport) Do(request *http.Request) (*http.Response, error) {
	if t == nil || t.bound == nil {
		return nil, errors.New("LLM Platform HTTP transport is not configured")
	}
	return t.bound.Do(request)
}

// OpenStream adapts Platform's stream response without buffering or taking
// ownership away from the LLM Adapter. The LLM Adapter must close Body.
func (t *Transport) OpenStream(ctx context.Context, request *http.Request) (*llm.HTTPStreamResponse, error) {
	if t == nil || t.bound == nil {
		return nil, errors.New("LLM Platform HTTP transport is not configured")
	}
	response, err := t.bound.OpenStream(ctx, request)
	if response == nil {
		return nil, err
	}
	return &llm.HTTPStreamResponse{
		StatusCode: response.StatusCode,
		Header:     response.Header.Clone(),
		Body:       response.Body,
	}, err
}

// NewResponsesAdapter binds one destination once and injects the same
// transport into both JSON and SSE paths. Provider credentials and capability
// evidence remain explicit inputs owned by Server configuration/policy wiring.
func NewResponsesAdapter(
	resources *serverresources.Resources,
	provider llm.ProviderConfig,
	apiKey platformconfig.Secret,
	capabilities llm.CapabilityState,
	clock llm.Clock,
	policy platformhttpclient.DestinationPolicy,
) (*llm.ResponsesAdapter, error) {
	transport, err := New(resources, provider, policy)
	if err != nil {
		return nil, err
	}
	adapter, err := llm.NewResponsesAdapterWithStream(provider, apiKey, transport, transport, capabilities, clock)
	if err != nil {
		return nil, fmt.Errorf("construct LLM Responses adapter: %w", err)
	}
	return adapter, nil
}
