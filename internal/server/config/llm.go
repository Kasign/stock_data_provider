package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"example.com/stock-analysis-flow/internal/llm"
	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformhttpclient "example.com/stock-analysis-flow/internal/platform/httpclient"
)

const (
	llmProviderJSONEnv       = "SERVER_LLM_PROVIDER_JSON"
	llmCapabilityJSONEnv     = "SERVER_LLM_CAPABILITY_JSON"
	llmDestinationDependency = "llm.provider"
	maxLLMConfigJSONBytes    = 32 << 10
)

// LLM is the Server-owned, immutable LLM Gateway configuration. Configured
// stays false when no provider is present; Server must never synthesize a
// default provider, capability evidence or credential.
type LLM struct {
	Provider     llm.ProviderConfig
	Capabilities llm.CapabilityState
	APIKey       platformconfig.Secret
	Policy       platformhttpclient.DestinationPolicy
	Configured   bool
}

// LoadLLM reads optional LLM Gateway configuration. Provider and capability
// evidence must be configured together; a secret reference is resolved only to
// an opaque platformconfig.Secret and is never logged or copied into errors.
func LoadLLM(env platformconfig.Env) (LLM, error) {
	providerRaw := strings.TrimSpace(env.String(llmProviderJSONEnv, ""))
	capabilityRaw := strings.TrimSpace(env.String(llmCapabilityJSONEnv, ""))
	if providerRaw == "" && capabilityRaw == "" {
		return LLM{}, nil
	}
	if providerRaw == "" || capabilityRaw == "" {
		return LLM{}, errors.New("LLM 配置无效: provider 与 capability 证据必须同时配置")
	}

	var provider llm.ProviderConfig
	if err := decodeStrictLLMJSON([]byte(providerRaw), &provider); err != nil {
		return LLM{}, fmt.Errorf("LLM 配置无效: provider JSON: %w", err)
	}
	if err := provider.Validate(); err != nil {
		return LLM{}, fmt.Errorf("LLM 配置无效: %w", err)
	}

	var capabilities llm.CapabilityState
	if err := decodeStrictLLMJSON([]byte(capabilityRaw), &capabilities); err != nil {
		return LLM{}, fmt.Errorf("LLM 配置无效: capability JSON: %w", err)
	}
	if err := provider.ValidateCapabilityState(capabilities); err != nil {
		return LLM{}, fmt.Errorf("LLM 配置无效: %w", err)
	}

	apiKey := env.Secret(provider.APIKeySecretRef)
	if !apiKey.Configured() {
		return LLM{}, fmt.Errorf("LLM 配置无效: 凭据环境变量 %s 未配置", provider.APIKeySecretRef)
	}
	policy, err := llmDestinationPolicy(provider)
	if err != nil {
		return LLM{}, fmt.Errorf("LLM 配置无效: %w", err)
	}
	return LLM{
		Provider:     provider,
		Capabilities: capabilities,
		APIKey:       apiKey,
		Policy:       policy,
		Configured:   true,
	}, nil
}

func decodeStrictLLMJSON(raw []byte, target any) error {
	if len(raw) == 0 || len(raw) > maxLLMConfigJSONBytes {
		return errors.New("配置为空或超过大小限制")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("必须是有效 JSON")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("包含多余内容")
	}
	return nil
}

func llmDestinationPolicy(provider llm.ProviderConfig) (platformhttpclient.DestinationPolicy, error) {
	target, err := url.Parse(provider.BaseURL)
	if err != nil || target == nil || target.Scheme != "https" || target.Hostname() == "" {
		return platformhttpclient.DestinationPolicy{}, errors.New("base_url 必须是 HTTPS Gateway URL")
	}
	host := strings.TrimSuffix(strings.ToLower(target.Hostname()), ".")
	if host == "" {
		return platformhttpclient.DestinationPolicy{}, errors.New("base_url 缺少主机")
	}
	policy := platformhttpclient.DestinationPolicy{
		Dependency:     llmDestinationDependency,
		AllowedHosts:   []string{host},
		AllowHTTP:      false,
		AllowRedirects: false,
		MaxRedirects:   0,
		Idempotent:     false,
	}
	portText := target.Port()
	if portText == "" {
		return policy, nil
	}
	port, err := strconv.ParseUint(portText, 10, 16)
	if err != nil || port == 0 || port > 65535 {
		return platformhttpclient.DestinationPolicy{}, errors.New("base_url 端口无效")
	}
	if port != 443 {
		policy.AllowedPorts = []uint16{uint16(port)}
	}
	return policy, nil
}
