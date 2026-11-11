package httpclient

import (
	"net/http"

	"example.com/stock-analysis-flow/internal/platform/fault"
)

// BoundDoer adapts a Client to the narrow Do(*http.Request) interface.
// Its destination policy is fixed at construction so callers cannot omit or
// replace the per-endpoint SSRF policy on individual requests.
type BoundDoer struct {
	client *Client
	policy DestinationPolicy
}

// Bind creates a request doer bound to one destination policy.
// Callers should create one BoundDoer for each configured external endpoint.
func (c *Client) Bind(policy DestinationPolicy) (*BoundDoer, error) {
	if c == nil || !validDependency(policy.Dependency) || validatePolicy(policy) != nil {
		return nil, fault.New(
			fault.ClassValidation,
			"platform.httpclient.invalid_policy",
			"invalid HTTP destination policy",
			nil,
		)
	}

	policy.AllowedHosts = append([]string(nil), policy.AllowedHosts...)
	policy.AllowedPorts = append([]uint16(nil), policy.AllowedPorts...)
	return &BoundDoer{client: c, policy: policy}, nil
}

// Do executes the request using its Context and the immutable bound policy.
func (d *BoundDoer) Do(request *http.Request) (*http.Response, error) {
	if d == nil || d.client == nil {
		return nil, fault.New(
			fault.ClassValidation,
			"platform.httpclient.invalid_client",
			"HTTP client is not configured",
			nil,
		)
	}
	if request == nil {
		return nil, fault.New(
			fault.ClassValidation,
			"platform.httpclient.invalid_request",
			"invalid HTTP request",
			nil,
		)
	}
	return d.client.Do(request.Context(), request, d.policy)
}
