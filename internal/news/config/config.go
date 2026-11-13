// Package config owns immutable-at-runtime configuration for the News module.
package config

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

const (
	BochaEndpointEnv   = "NEWS_PROVIDER_BOCHA_ENDPOINT"
	BochaAPIKeyEnv     = "NEWS_PROVIDER_BOCHA_API_KEY"
	TavilyEndpointEnv  = "NEWS_PROVIDER_TAVILY_ENDPOINT"
	TavilyAPIKeyEnv    = "NEWS_PROVIDER_TAVILY_API_KEY"
	BraveEndpointEnv   = "NEWS_PROVIDER_BRAVE_ENDPOINT"
	BraveAPIKeyEnv     = "NEWS_PROVIDER_BRAVE_API_KEY"
	SearXNGEndpointEnv = "NEWS_PROVIDER_SEARXNG_ENDPOINT"
	EvidenceCursorActiveKeyIDEnv = "NEWS_EVIDENCE_CURSOR_ACTIVE_KEY_ID"
	EvidenceCursorKeyRingEnv = "NEWS_EVIDENCE_CURSOR_KEYRING_JSON"
	maximumEvidenceCursorKeys = 8
)

// ProviderConfig is a value snapshot. Treat it as immutable after Load returns.
type ProviderConfig struct {
	endpoint   string
	apiKey     platformconfig.Secret
	configured bool
}

// Endpoint returns the configured provider endpoint.
func (c ProviderConfig) Endpoint() string {
	return c.endpoint
}

// APIKey returns the redacting Platform Secret wrapper.
func (c ProviderConfig) APIKey() platformconfig.Secret {
	return c.apiKey
}

// Configured reports whether this provider has enough configuration to be used.
func (c ProviderConfig) Configured() bool {
	return c.configured
}

// Config is a startup snapshot of News provider settings.
// Fields are private to prevent accidental mutation after loading.
type Config struct {
	bocha                    ProviderConfig
	tavily                   ProviderConfig
	brave                    ProviderConfig
	searxng                  ProviderConfig
	evidenceCursorActiveKeyID string
	evidenceCursorKeys        map[string]platformconfig.Secret
}

func (c Config) Bocha() ProviderConfig {
	return c.bocha
}

func (c Config) Tavily() ProviderConfig {
	return c.tavily
}

func (c Config) Brave() ProviderConfig {
	return c.brave
}

func (c Config) SearXNG() ProviderConfig {
	return c.searxng
}

// EvidenceCursorKeyRing returns a defensive copy of the optional cursor signing key ring.
// Keep previous keys configured until all cursors signed by them have expired.
func (c Config) EvidenceCursorKeyRing() (string, map[string]platformconfig.Secret, bool) {
	if c.evidenceCursorActiveKeyID == "" || len(c.evidenceCursorKeys) == 0 {
		return "", nil, false
	}
	keys := make(map[string]platformconfig.Secret, len(c.evidenceCursorKeys))
	for keyID, secret := range c.evidenceCursorKeys {
		keys[keyID] = secret
	}
	return c.evidenceCursorActiveKeyID, keys, true
}

// Load reads News provider settings from the Platform environment abstraction.
// It performs local validation only: no network access, DNS lookup, or client creation.
func Load(env platformconfig.Env) (Config, error) {
	bocha, err := loadCredentialProvider(env, BochaEndpointEnv, BochaAPIKeyEnv)
	if err != nil {
		return Config{}, err
	}
	tavily, err := loadCredentialProvider(env, TavilyEndpointEnv, TavilyAPIKeyEnv)
	if err != nil {
		return Config{}, err
	}
	brave, err := loadCredentialProvider(env, BraveEndpointEnv, BraveAPIKeyEnv)
	if err != nil {
		return Config{}, err
	}

	searxngEndpoint := strings.TrimSpace(env.String(SearXNGEndpointEnv, ""))
	searxng := ProviderConfig{}
	if searxngEndpoint != "" {
		if err := validateEndpoint(SearXNGEndpointEnv, searxngEndpoint); err != nil {
			return Config{}, err
		}
		searxng = ProviderConfig{endpoint: searxngEndpoint, configured: true}
	}

	activeCursorKeyID, cursorKeys, err := loadEvidenceCursorKeyRing(env)
	if err != nil {
		return Config{}, err
	}

	return Config{
		bocha: bocha, tavily: tavily, brave: brave, searxng: searxng,
		evidenceCursorActiveKeyID: activeCursorKeyID,
		evidenceCursorKeys: cursorKeys,
	}, nil
}

func loadEvidenceCursorKeyRing(env platformconfig.Env) (string, map[string]platformconfig.Secret, error) {
	activeKeyID := strings.TrimSpace(env.String(EvidenceCursorActiveKeyIDEnv, ""))
	rawKeyRing := env.Secret(EvidenceCursorKeyRingEnv)
	if activeKeyID == "" && !rawKeyRing.Configured() {
		return "", nil, nil
	}
	if activeKeyID == "" || !rawKeyRing.Configured() {
		return "", nil, fmt.Errorf("news config requires %s and %s to be configured together",
			EvidenceCursorActiveKeyIDEnv, EvidenceCursorKeyRingEnv)
	}
	if !validEvidenceCursorKeyID(activeKeyID) {
		return "", nil, fmt.Errorf("news config %s is invalid", EvidenceCursorActiveKeyIDEnv)
	}

	var rawKeys map[string]string
	if err := json.Unmarshal([]byte(rawKeyRing.Value()), &rawKeys); err != nil ||
		len(rawKeys) == 0 || len(rawKeys) > maximumEvidenceCursorKeys {
		return "", nil, fmt.Errorf("news config %s must be a JSON object containing 1..%d keys",
			EvidenceCursorKeyRingEnv, maximumEvidenceCursorKeys)
	}
	if _, exists := rawKeys[activeKeyID]; !exists {
		return "", nil, fmt.Errorf("news config %s must reference a key in %s",
			EvidenceCursorActiveKeyIDEnv, EvidenceCursorKeyRingEnv)
	}

	secretEnv := platformconfig.NewEnvWithLookup(func(key string) (string, bool) {
		value, exists := rawKeys[key]
		return value, exists
	})
	keys := make(map[string]platformconfig.Secret, len(rawKeys))
	for keyID, value := range rawKeys {
		if !validEvidenceCursorKeyID(keyID) || len(value) < 32 || len(value) > 256 ||
			strings.TrimSpace(value) == "" {
			return "", nil, fmt.Errorf("news config %s contains an invalid key entry", EvidenceCursorKeyRingEnv)
		}
		keys[keyID] = secretEnv.Secret(keyID)
	}
	return activeKeyID, keys, nil
}

func validEvidenceCursorKeyID(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '-' || char == '.' {
			continue
		}
		return false
	}
	return true
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
		return ProviderConfig{}, fmt.Errorf(
			"news config requires %s and %s to be configured together",
			endpointKey,
			secretKey,
		)
	}
	if err := validateEndpoint(endpointKey, endpoint); err != nil {
		return ProviderConfig{}, err
	}

	return ProviderConfig{
		endpoint:   endpoint,
		apiKey:     secret,
		configured: true,
	}, nil
}

func validateEndpoint(key, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Opaque != "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.HasSuffix(parsed.Host, ":") {
		return fmt.Errorf(
			"news config %s must be an absolute HTTP(S) endpoint without credentials, query, or fragment",
			key,
		)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
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
