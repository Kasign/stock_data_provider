// Package config owns immutable-at-runtime configuration for the News module.
package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

const (
	BochaEndpointEnv  = "NEWS_PROVIDER_BOCHA_ENDPOINT"
	BochaAPIKeyEnv    = "NEWS_PROVIDER_BOCHA_API_KEY"
	TavilyEndpointEnv = "NEWS_PROVIDER_TAVILY_ENDPOINT"
	TavilyAPIKeyEnv   = "NEWS_PROVIDER_TAVILY_API_KEY"
	BraveEndpointEnv  = "NEWS_PROVIDER_BRAVE_ENDPOINT"
	BraveAPIKeyEnv    = "NEWS_PROVIDER_BRAVE_API_KEY"
	SearXNGEndpointEnv = "NEWS_PROVIDER_SEARXNG_ENDPOINT"
)

// ProviderConfig is a value snapshot. Treat it as immutable after Load returns.
type ProviderConfig struct {
	endpoint string
	apiKey   platformconfig.Secret
	configured bool
}

// Endpoint returns the configured provider endpoint.
func (c ProviderConfig) Endpoint() string { return c.endpoint }

// APIKey returns the redacting Platform Secret wrapper.
func (c ProviderConfig) APIKey() platformconfig.Secret { return c.apiKey }

// Configured reports whether this provider has enough configuration to be used.
func (c ProviderConfig) Configured() bool { return c.configured }

// Config is a startup snapshot of News provider settings.
// Fields are private to prevent accidental mutation after loading.
type Config struct {
	bocha  ProviderConfig
	tavily ProviderConfig
	brave  ProviderConfig
	searxng ProviderConfig
}

func (c Config) Bocha() ProviderConfig { return c.bocha }
func (c Config) Tavily() ProviderConfig { return c.tavily }
func (c Config) Brave() ProviderConfig { return c.brave }
func (c Config) SearXNG() ProviderConfig { return c.searxng }

// Load reads News provider settings from the Platform environment abstraction.
// It performs local validation only: no network access, DNS lookup, or client creation.
func Load(env platformconfig.Env) (Config, error) {
	bocha, err := loadCredentialProvider(env, BochaEndpointEnv, BochaAPIKeyEnv)
	if err != nil { return Config{}, err }
	tavily, err := loadCredentialProvider(env, TavilyEndpointEnv, TavilyAPIKeyEnv)
	if err != nil { return Config{}, err }
	brave, err := loadCredentialProvider(env, BraveEndpointEnv, BraveAPIKeyEnv)
	if err != nil { return Config{}, err }
	searxngEndpoint := strings.TrimSpace(env.String(SearXNGEndpointEnv, ""))
	searxng := ProviderConfig{}
	if searxngEndpoint != "" {
		if err := validateEndpoint(SearXNGEndpointEnv, searxngEndpoint); err != nil { return Config{}, err }
		searxng = ProviderConfig{endpoint: searxngEndpoint, configured: true}
	}
	return Config{bocha: bocha, tavily: tavily, brave: brave, searxng: searxng}, nil
}

func loadCredentialProvider(env platformconfig.Env, endpointKey, secretKey string) (ProviderConfig, error) {
	endpoint := strings.TrimSpace(env.String(endpointKey, ""))
	secret := env.Secret(secretKey)
	hasEndpoint := endpoint != ""
	hasSecret := secret.Configured()
	if !hasEndpoint && !hasSecret {
		return ProviderConfig{}, nil
	}
	if hasEndpoint != hasSecret {
		return ProviderConfig{}, fmt.Errorf("news config requires %s and %s to be configured together", endpointKey, secretKey)
	}
	if err := validateEndpoint(endpointKey, endpoint); err != nil {
		return ProviderConfig{}, err
	}
	return ProviderConfig{endpoint: endpoint, apiKey: secret, configured: true}, nil
}

func validateEndpoint(key, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return fmt.Errorf("news config %s must be an absolute HTTP(S) endpoint without credentials, query, or fragment", key)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("news config %s must use HTTP or HTTPS", key)
	}
	if parsed.Hostname() == "" {
		return fmt.Errorf("news config %s must include a valid host", key)
	}
	if port := parsed.Port(); port != "" {
		number, parseErr := strconv.Atoi(port)
		if parseErr != nil || number < 1 || number > 65535 {
			return fmt.Errorf("news config %s contains an invalid port", key)
		}
	}
	return nil
}
