package source

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

// platformHTTPDoer binds one registered Source endpoint to one exact destination policy.
// The shared Platform Client lifecycle remains owned by the composition root.
type platformHTTPDoer struct {
	client *httpclient.Client
	policy httpclient.DestinationPolicy
}

// NewPlatformHTTPDoer creates one transport adapter for one Source/Endpoint registration.
// Source query parameters are retained because some feeds use them as part of their URL;
// userinfo and fragments remain forbidden.
func NewPlatformHTTPDoer(client *httpclient.Client, endpoint, dependency string) (app.HTTPDoer, error) {
	if client == nil {
		return nil, app.ErrProviderConfig
	}
	policy, err := sourceDestinationPolicy(endpoint, dependency)
	if err != nil {
		return nil, err
	}
	return &platformHTTPDoer{client: client, policy: policy}, nil
}

func sourceDestinationPolicy(endpoint, dependency string) (httpclient.DestinationPolicy, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Opaque != "" || parsed.User != nil ||
		parsed.Hostname() == "" || parsed.Fragment != "" ||
		strings.HasSuffix(parsed.Host, ":") {
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
	if !validSourceDependency(dependency) {
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

func validSourceDependency(value string) bool {
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
		return nil, sourceTransportErrorFromPlatform(err)
	}
	return response, nil
}

type sourceTransportError struct {
	retryable bool
}

func (e *sourceTransportError) Error() string {
	return "news: source transport failed"
}

func (e *sourceTransportError) Retryable() bool {
	return e != nil && e.retryable
}

func sourceTransportErrorFromPlatform(err error) error {
	var platformFault *fault.Fault
	if errors.As(err, &platformFault) && platformFault != nil {
		return &sourceTransportError{retryable: platformFault.Retryable}
	}
	return &sourceTransportError{}
}
