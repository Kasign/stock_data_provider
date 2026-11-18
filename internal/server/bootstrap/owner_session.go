package bootstrap

import (
	"errors"
	"fmt"

	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
	serversecurity "example.com/stock-analysis-flow/internal/server/security"
)

// ownerSessionComposition 仅组合依赖，不拥有共享 DB/Redis 的生命周期。
type ownerSessionComposition struct {
	lifecycle     *serversecurity.SessionLifecycleService
	researchflow  *researchflowOwnerComposition
	authenticator *serversecurity.SessionAuthenticator
	cookie        platformsecurity.SessionCookieConfig
}

// newOwnerSessionComposition 将 Platform 安全存储和 Server 审计适配器组合为 Owner Session 用例。
func newOwnerSessionComposition(
	shared *serverresources.Resources,
	cfg serverconfig.OwnerSession,
) (*ownerSessionComposition, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if shared == nil || shared.DB == nil || shared.Redis == nil {
		return nil, errors.New("Owner Session requires the shared MariaDB and Redis resources")
	}

	credentials, err := platformsecurity.NewSQLCredentialStore(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Credential Store 失败: %w", err)
	}
	sessions, err := platformsecurity.NewRedisSessionStore(shared.Redis, cfg.RedisKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Session Store 失败: %w", err)
	}
	rateLimiter, err := platformsecurity.NewRedisLoginRateLimiter(shared.Redis, cfg.RedisKeyPrefix)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Login Rate Limiter 失败: %w", err)
	}
	hasher, err := platformsecurity.NewArgon2idHasher(cfg.Argon2id)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Password Hasher 失败: %w", err)
	}
	audit, err := serversecurity.NewSQLSecurityAuditWriter(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Security Audit Writer 失败: %w", err)
	}

	lifecycle, err := serversecurity.NewSessionLifecycleService(
		serversecurity.SessionLifecycleDependencies{
			Credentials: credentials,
			Sessions:    sessions,
			RateLimiter: rateLimiter,
			Hasher:      hasher,
			Audit:       audit,
		},
		serversecurity.SessionLifecycleConfig{
			Cookie:          cfg.Cookie,
			IdleTTL:         cfg.IdleTTL,
			AbsoluteTTL:     cfg.AbsoluteTTL,
			RateLimitPolicy: cfg.RateLimitPolicy,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Session Lifecycle Service 失败: %w", err)
	}
	authenticator, err := serversecurity.NewSessionAuthenticator(
		sessions, credentials, serversecurity.SessionAuthenticatorConfig{
			Cookie:       cfg.Cookie,
			PublicOrigin: cfg.PublicOrigin,
			CSRFHeader:   cfg.CSRFHeader,
			IdleTTL:      cfg.IdleTTL,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Session Authenticator 失败: %w", err)
	}
	return &ownerSessionComposition{lifecycle: lifecycle, authenticator: authenticator, cookie: cfg.Cookie}, nil
}
