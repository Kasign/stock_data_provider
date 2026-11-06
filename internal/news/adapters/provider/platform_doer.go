package provider

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/platform/fault"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

// platformHTTPDoer binds one Provider endpoint to one immutable destination policy.
// It deliberately does not own the shared Platform Client lifecycle.
type platformHTTPDoer struct {
	client *httpclient.Client
	policy httpclient.DestinationPolicy
}

// NewPlatformHTTPDoer creates a Provider-specific HTTPDoer.
//
// Call it once per configured Provider endpoint. Do not reuse the returned
// adapter for Source endpoints or for another Provider with a different origin.
// The Platform Client remains owned by the application composition root.
func NewPlatformHTTPDoer(client *httpclient.Client, endpoint, dependency string) (app.HTTPDoer, error) {
	if client == nil {
		return nil, app.ErrProviderConfig
	}
	policy, err := providerDestinationPolicy(endpoint, dependency)
	if err != nil {
		return nil, err
	}
	return &platformHTTPDoer{client: client, policy: policy}, nil
}

func providerDestinationPolicy(endpoint, dependency string) (httpclient.DestinationPolicy, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Opaque != "" || parsed.User != nil ||
		parsed.Hostname() == "" || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || strings.HasSuffix(parsed.Host, ":") {
		return httpclient.DestinationPolicy{}, app.ErrProviderConfig
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "https" && scheme != "http" {
		return httpclient.DestinationPolicy{}, app.ErrProviderConfig
	}
	host := strings.TrimSuffix(strings.ToLower(parsed.Hostname()), ".")
	if host == "" || strings.ContainsAny(host, "%/\\") {
		return httpclient.DestinationPolicy{}, app.ErrProviderConfig
	}

	port := uint64(0)
	if parsed.Port() != "" {
		port, err = strconv.ParseUint(parsed.Port(), 10, 16)
		if err != nil || port == 0 {
			return httpclient.DestinationPolicy{}, app.ErrProviderConfig
		}
	} else if scheme == "https" {
		port = 443
	} else {
		port = 80
	}

	dependency = strings.TrimSpace(dependency)
	if !validProviderDependency(dependency) {
		return httpclient.DestinationPolicy{}, app.ErrProviderConfig
	}

	return httpclient.DestinationPolicy{
		Dependency:     dependency,
		AllowedHosts:   []string{host},
		AllowedPorts:   []uint16{uint16(port)},
		AllowHTTP:      scheme == "http",
		AllowRedirects: false,
		MaxRedirects:   0,
		Idempotent:     false,
	}, nil
}

func validProviderDependency(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return false
	}
	return true
}

func (d *platformHTTPDoer) Do(request *http.Request) (*http.Response, error) {
	if d == nil || d.client == nil || request == nil || request.URL == nil {
		return nil, app.ErrProviderConfig
	}
	response, err := d.client.Do(request.Context(), request, d.policy)
	if err != nil {
		return nil, providerErrorFromPlatform(err)
	}
	return response, nil
}

func providerErrorFromPlatform(err error) error {
	var platformFault *fault.Fault
	if errors.As(err, &platformFault) && platformFault != nil {
		code := strings.TrimSpace(platformFault.Code)
		if code == "" {
			code = "provider_transport_failed"
		}
		return &app.ProviderError{
			Code:      code,
			Retryable: platformFault.Retryable,
		}
	}
	return &app.ProviderError{Code: "provider_transport_failed"}
}
