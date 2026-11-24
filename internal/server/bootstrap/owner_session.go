package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"sync"

	marketdataauthorization "example.com/stock-analysis-flow/internal/marketdata/authorization"
	platformfault "example.com/stock-analysis-flow/internal/platform/fault"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	quantapplication "example.com/stock-analysis-flow/internal/quant/application"
	quantstorage "example.com/stock-analysis-flow/internal/quant/storage"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
	serversecurity "example.com/stock-analysis-flow/internal/server/security"
)

// quantResultAccessPolicyManager is the Server-facing Owner management port.
type quantResultAccessPolicyManager interface {
	Current(context.Context) (quantapplication.ResultAccessPolicyState, error)
	Publish(context.Context, string, uint64) (quantapplication.ResultAccessPolicyState, error)
	Revoke(context.Context, string, uint64) (quantapplication.ResultAccessPolicyState, error)
}

// ownerSessionComposition 仅组合依赖，不拥有共享 DB/Redis 的生命周期。
type dataAuthorizationPolicyManager interface {
	Current(context.Context) (marketdataauthorization.CurrentPolicy, error)
	Publish(context.Context, string, string, marketdataauthorization.DataAccessPolicy) (marketdataauthorization.CurrentPolicy, error)
}

type ownerSessionResetState struct {
	mu       sync.RWMutex
	required bool
}

func (s *ownerSessionResetState) Required() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.required
}

func (s *ownerSessionResetState) Complete() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.required = false
	s.mu.Unlock()
}

// ownerSessionCredentialGate prevents previously issued Sessions from being
// accepted while this process is waiting for a configured password reset.
type ownerSessionCredentialGate struct {
	credentials platformsecurity.CredentialStore
	resetState  *ownerSessionResetState
}

func (g ownerSessionCredentialGate) GetOwnerCredential(ctx context.Context) (platformsecurity.OwnerCredential, error) {
	if g.resetState.Required() {
		return platformsecurity.OwnerCredential{}, platformfault.New(platformfault.ClassUnauthenticated, "server.owner_password_reset_required", "Owner password reset is required", nil)
	}
	return g.credentials.GetOwnerCredential(ctx)
}

func (g ownerSessionCredentialGate) InitializeOwnerCredential(ctx context.Context, hash string) (platformsecurity.OwnerCredential, error) {
	return g.credentials.InitializeOwnerCredential(ctx, hash)
}

func (g ownerSessionCredentialGate) UpdateOwnerPassword(ctx context.Context, version uint64, hash string) (platformsecurity.OwnerCredential, error) {
	return g.credentials.UpdateOwnerPassword(ctx, version, hash)
}

type ownerSessionComposition struct {
	lifecycle         *serversecurity.SessionLifecycleService
	credentials       platformsecurity.CredentialStore
	resetState        *ownerSessionResetState
	researchflow      *researchflowOwnerComposition
	authenticator     *serversecurity.SessionAuthenticator
	policyManager     quantResultAccessPolicyManager
	dataPolicyManager dataAuthorizationPolicyManager
	cookie            platformsecurity.SessionCookieConfig
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
	resetState := &ownerSessionResetState{required: cfg.ResetRequired}
	sessionCredentials := ownerSessionCredentialGate{credentials: credentials, resetState: resetState}
	authenticator, err := serversecurity.NewSessionAuthenticator(
		sessions, sessionCredentials, serversecurity.SessionAuthenticatorConfig{
			Cookie:       cfg.Cookie,
			PublicOrigin: cfg.PublicOrigin,
			CSRFHeader:   cfg.CSRFHeader,
			IdleTTL:      cfg.IdleTTL,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建 Owner Session Authenticator 失败: %w", err)
	}
	policyStore, err := quantstorage.NewMySQLResultAccessPolicyStore(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("创建 Quant Result Access Policy Store 失败: %w", err)
	}
	policyManager, err := quantapplication.NewResultAccessPolicyManager(policyStore)
	if err != nil {
		return nil, fmt.Errorf("创建 Quant Result Access Policy Manager 失败: %w", err)
	}
	return &ownerSessionComposition{
		lifecycle: lifecycle, credentials: credentials, resetState: resetState,
		authenticator: authenticator, policyManager: policyManager, cookie: cfg.Cookie,
	}, nil
}
