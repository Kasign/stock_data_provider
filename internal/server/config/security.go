package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

// Security contains credentials required by the Root Server HTTP boundary.
type Security struct {
	OwnerToken         platformconfig.Secret
	ServiceCredentials []platformsecurity.ServiceCredential
}

type serviceCredentialConfig struct {
	Name     string   `json:"name"`
	TokenEnv string   `json:"token_env"`
	Scopes   []string `json:"scopes"`
}

// LoadSecurity requires an explicit owner credential. Service credentials are
// optional and reference separately managed environment secrets. Server never
// synthesizes a service identity or grants an implicit scope.
func LoadSecurity(env platformconfig.Env) (Security, error) {
	token, err := env.RequiredSecret("PLATFORM_ADMIN_TOKEN")
	if err != nil {
		return Security{}, fmt.Errorf("安全配置无效: %w", err)
	}
	services, err := LoadServiceCredentials(env)
	if err != nil {
		return Security{}, err
	}
	return Security{OwnerToken: token, ServiceCredentials: services}, nil
}

// LoadServiceCredentials 只加载独立服务凭据，不读取或要求 Owner HTTP Token。
// Worker 进程使用该入口，避免将 Owner HTTP 认证耦合到后台执行身份。
func LoadServiceCredentials(env platformconfig.Env) ([]platformsecurity.ServiceCredential, error) {
	raw := strings.TrimSpace(env.String("PLATFORM_SERVICE_CREDENTIALS", ""))
	if raw == "" {
		return nil, nil
	}
	if len(raw) > 32*1024 {
		return nil, errors.New("安全配置无效: PLATFORM_SERVICE_CREDENTIALS 超过大小限制")
	}
	decoder := json.NewDecoder(bytes.NewBufferString(raw))
	decoder.DisallowUnknownFields()
	var entries []serviceCredentialConfig
	if err := decoder.Decode(&entries); err != nil {
		return nil, errors.New("安全配置无效: PLATFORM_SERVICE_CREDENTIALS 必须是有效的服务凭据 JSON 数组")
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, errors.New("安全配置无效: PLATFORM_SERVICE_CREDENTIALS 包含多余内容")
	}
	if len(entries) == 0 || len(entries) > 128 {
		return nil, errors.New("安全配置无效: PLATFORM_SERVICE_CREDENTIALS 必须包含 1 到 128 个服务")
	}
	credentials := make([]platformsecurity.ServiceCredential, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name, tokenEnv := strings.TrimSpace(entry.Name), strings.TrimSpace(entry.TokenEnv)
		if name == "" || tokenEnv == "" || len(entry.Scopes) == 0 {
			return nil, errors.New("安全配置无效: 每个服务凭据都必须配置 name、token_env 和 scopes")
		}
		if _, exists := seen[name]; exists {
			return nil, errors.New("安全配置无效: 服务凭据 name 必须唯一")
		}
		seen[name] = struct{}{}
		secret := env.Secret(tokenEnv)
		if !secret.Configured() {
			return nil, fmt.Errorf("安全配置无效: 服务凭据环境变量 %s 未配置", tokenEnv)
		}
		credentials = append(credentials, platformsecurity.ServiceCredential{
			Name: name, Secret: secret, Scopes: append([]string(nil), entry.Scopes...),
		})
	}
	return credentials, nil
}
