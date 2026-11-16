package config

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
)

// OwnerSession 配置浏览器 Owner Session；默认关闭以保持现有 Bearer 路径兼容。
type OwnerSession struct {
	Enabled         bool
	PublicOrigin    string
	Cookie          platformsecurity.SessionCookieConfig
	CSRFHeader      string
	IdleTTL         time.Duration
	AbsoluteTTL     time.Duration
	RedisKeyPrefix  string
	Argon2id        platformsecurity.Argon2idParams
	RateLimitPolicy platformsecurity.LoginRateLimitPolicy
}

// LoadOwnerSession 仅在显式启用时解析配置。Argon2id 成本必须由目标硬件基准测试决定。
func LoadOwnerSession(env platformconfig.Env) (OwnerSession, error) {
	enabled, err := env.Bool("OWNER_SESSION_ENABLED", false)
	if err != nil {
		return OwnerSession{}, err
	}
	if !enabled {
		return OwnerSession{}, nil
	}
	cfg := OwnerSession{
		Enabled:      true,
		PublicOrigin: strings.TrimSpace(env.String("OWNER_SESSION_PUBLIC_ORIGIN", "")),
		Cookie: platformsecurity.SessionCookieConfig{
			Name:     strings.TrimSpace(env.String("OWNER_SESSION_COOKIE_NAME", "__Secure-owner_session")),
			Path:     strings.TrimSpace(env.String("OWNER_SESSION_COOKIE_PATH", "/api")),
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
		},
		CSRFHeader:     strings.TrimSpace(env.String("OWNER_SESSION_CSRF_HEADER", "X-CSRF-Token")),
		IdleTTL:        12 * time.Hour,
		AbsoluteTTL:    7 * 24 * time.Hour,
		RedisKeyPrefix: strings.TrimSpace(env.String("OWNER_SESSION_REDIS_KEY_PREFIX", "stock-analysis:owner-session")),
		RateLimitPolicy: platformsecurity.LoginRateLimitPolicy{
			AccountMaxFailures: 5,
			IPMaxFailures:      10,
			Window:             15 * time.Minute,
			BaseBackoff:        time.Second,
			MaxBackoff:         15 * time.Minute,
		},
	}
	if cfg.Cookie.Secure, err = env.Bool("OWNER_SESSION_COOKIE_SECURE", true); err != nil {
		return OwnerSession{}, err
	}
	if cfg.IdleTTL, err = env.Duration("OWNER_SESSION_IDLE_TTL", cfg.IdleTTL); err != nil {
		return OwnerSession{}, err
	}
	if cfg.AbsoluteTTL, err = env.Duration("OWNER_SESSION_ABSOLUTE_TTL", cfg.AbsoluteTTL); err != nil {
		return OwnerSession{}, err
	}
	if cfg.RateLimitPolicy.Window, err = env.Duration("OWNER_SESSION_LOGIN_WINDOW", cfg.RateLimitPolicy.Window); err != nil {
		return OwnerSession{}, err
	}
	if cfg.RateLimitPolicy.BaseBackoff, err = env.Duration("OWNER_SESSION_LOGIN_BASE_BACKOFF", cfg.RateLimitPolicy.BaseBackoff); err != nil {
		return OwnerSession{}, err
	}
	if cfg.RateLimitPolicy.MaxBackoff, err = env.Duration("OWNER_SESSION_LOGIN_MAX_BACKOFF", cfg.RateLimitPolicy.MaxBackoff); err != nil {
		return OwnerSession{}, err
	}
	if cfg.RateLimitPolicy.AccountMaxFailures, err = loadPositiveInt(env, "OWNER_SESSION_LOGIN_ACCOUNT_MAX_FAILURES", 5); err != nil {
		return OwnerSession{}, err
	}
	if cfg.RateLimitPolicy.IPMaxFailures, err = loadPositiveInt(env, "OWNER_SESSION_LOGIN_IP_MAX_FAILURES", 10); err != nil {
		return OwnerSession{}, err
	}

	memory, err := loadRequiredPositiveInt(env, "OWNER_SESSION_ARGON2_MEMORY_KIB")
	if err != nil {
		return OwnerSession{}, err
	}
	iterations, err := loadRequiredPositiveInt(env, "OWNER_SESSION_ARGON2_ITERATIONS")
	if err != nil {
		return OwnerSession{}, err
	}
	parallelism, err := loadRequiredPositiveInt(env, "OWNER_SESSION_ARGON2_PARALLELISM")
	if err != nil {
		return OwnerSession{}, err
	}
	if memory > 256*1024 || iterations > 10 || parallelism > 8 {
		return OwnerSession{}, fmt.Errorf("Owner Session Argon2id 参数超出 Platform 安全上限")
	}
	cfg.Argon2id = platformsecurity.Argon2idParams{
		Memory:      uint32(memory),
		Iterations:  uint32(iterations),
		Parallelism: uint8(parallelism),
		SaltLength:  16,
		KeyLength:   32,
	}
	if cfg.PublicOrigin == "" {
		return OwnerSession{}, fmt.Errorf("OWNER_SESSION_PUBLIC_ORIGIN 必须配置")
	}
	origin, parseErr := url.Parse(cfg.PublicOrigin)
	if parseErr != nil || origin.Scheme == "" || origin.Host == "" || origin.User != nil ||
		origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" ||
		(origin.Scheme != "http" && origin.Scheme != "https") {
		return OwnerSession{}, fmt.Errorf("OWNER_SESSION_PUBLIC_ORIGIN 必须是无路径的 HTTP(S) Origin")
	}
	if origin.Scheme == "https" && !cfg.Cookie.Secure {
		return OwnerSession{}, fmt.Errorf("HTTPS Origin 必须启用 OWNER_SESSION_COOKIE_SECURE")
	}
	if err := cfg.Cookie.Validate(); err != nil {
		return OwnerSession{}, fmt.Errorf("Owner Session Cookie 配置无效: %w", err)
	}
	if cfg.IdleTTL < time.Second || cfg.IdleTTL > platformsecurity.DefaultSessionIdleTTL ||
		cfg.AbsoluteTTL < cfg.IdleTTL || cfg.AbsoluteTTL > platformsecurity.DefaultSessionAbsoluteTTL {
		return OwnerSession{}, fmt.Errorf("Owner Session TTL 超出 Platform 支持范围")
	}
	if cfg.CSRFHeader == "" || strings.ContainsAny(cfg.CSRFHeader, " \t\r\n:") {
		return OwnerSession{}, fmt.Errorf("OWNER_SESSION_CSRF_HEADER 无效")
	}
	if cfg.RedisKeyPrefix == "" || strings.ContainsAny(cfg.RedisKeyPrefix, "{} \t\r\n") || len(cfg.RedisKeyPrefix) > 128 {
		return OwnerSession{}, fmt.Errorf("OWNER_SESSION_REDIS_KEY_PREFIX 无效")
	}
	if _, err := platformsecurity.NewArgon2idHasher(cfg.Argon2id); err != nil {
		return OwnerSession{}, fmt.Errorf("Owner Session Argon2id 参数无效: %w", err)
	}
	p := cfg.RateLimitPolicy
	if p.AccountMaxFailures < 1 || p.AccountMaxFailures > 100000 || p.IPMaxFailures < 1 || p.IPMaxFailures > 100000 ||
		p.Window < time.Millisecond || p.Window > 24*time.Hour || p.BaseBackoff < time.Millisecond ||
		p.MaxBackoff < p.BaseBackoff || p.MaxBackoff > 24*time.Hour {
		return OwnerSession{}, fmt.Errorf("Owner Session 登录限流策略无效")
	}
	return cfg, nil
}

func loadPositiveInt(env platformconfig.Env, key string, fallback int) (int64, error) {
	value, err := env.Int(key, fallback)
	if err != nil {
		return 0, err
	}
	if value < 1 {
		return 0, fmt.Errorf("configuration %s must be positive", key)
	}
	return int64(value), nil
}

func loadRequiredPositiveInt(env platformconfig.Env, key string) (int, error) {
	raw, err := env.RequiredString(key)
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value < 1 {
		return 0, fmt.Errorf("configuration %s must be a positive integer", key)
	}
	return value, nil
}
