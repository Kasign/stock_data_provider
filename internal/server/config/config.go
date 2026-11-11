// Package config owns process-level Server configuration.
package config

import (
	"fmt"
	"strings"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

type Role string

const (
	RoleAPI    Role = "api"
	RoleWorker Role = "worker"
	RoleAll    Role = "all"
)

// LoadRole parses the process role. The default preserves the historical combined runtime.
func LoadRole(env platformconfig.Env) (Role, error) {
	raw := strings.ToLower(strings.TrimSpace(env.String("SERVER_ROLE", string(RoleAll))))
	role := Role(raw)
	switch role {
	case RoleAPI, RoleWorker, RoleAll:
		return role, nil
	default:
		return "", fmt.Errorf("SERVER_ROLE 必须是 api、worker 或 all")
	}
}

// HTTP contains the HTTP listener, admission limits and graceful-shutdown settings.
type HTTP struct {
	Address               string
	ReadHeaderTimeout     time.Duration
	ReadTimeout           time.Duration
	WriteTimeout          time.Duration
	IdleTimeout           time.Duration
	ShutdownTimeout       time.Duration
	MaxRequestBodyBytes   int64
	MaxConcurrentRequests int
}

// LoadHTTP reads and validates Server-owned HTTP settings. The legacy Analysis
// timeout is supplied only to preserve the existing HTTP write-timeout default.
func LoadHTTP(env platformconfig.Env, analysisTimeout time.Duration) (HTTP, error) {
	if analysisTimeout <= 0 {
		return HTTP{}, fmt.Errorf("analysis timeout must be positive")
	}
	cfg := HTTP{
		Address:               strings.TrimSpace(env.String("HTTP_ADDR", ":8092")),
		ReadHeaderTimeout:     5 * time.Second,
		ShutdownTimeout:       10 * time.Second,
		MaxRequestBodyBytes:   4 << 20,
		MaxConcurrentRequests: 64,
	}
	if cfg.Address == "" {
		cfg.Address = ":8092"
	}
	var err error
	if cfg.ReadTimeout, err = env.Duration("HTTP_READ_TIMEOUT", 15*time.Second); err != nil {
		return HTTP{}, err
	}
	if cfg.WriteTimeout, err = env.Duration("HTTP_WRITE_TIMEOUT", analysisTimeout+10*time.Second); err != nil {
		return HTTP{}, err
	}
	if cfg.IdleTimeout, err = env.Duration("HTTP_IDLE_TIMEOUT", 60*time.Second); err != nil {
		return HTTP{}, err
	}
	if cfg.ShutdownTimeout, err = env.Duration("HTTP_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout); err != nil {
		return HTTP{}, err
	}
	if cfg.MaxRequestBodyBytes, err = loadBoundedInt64(env, "HTTP_MAX_REQUEST_BYTES", cfg.MaxRequestBodyBytes, 1024, 64<<20); err != nil {
		return HTTP{}, err
	}
	if cfg.MaxConcurrentRequests, err = env.Int("HTTP_MAX_CONCURRENT_REQUESTS", cfg.MaxConcurrentRequests); err != nil {
		return HTTP{}, err
	}
	if err := platformconfig.JoinValidationErrors(
		platformconfig.ValidatePositiveDuration("HTTP_READ_TIMEOUT", cfg.ReadTimeout),
		platformconfig.ValidatePositiveDuration("HTTP_WRITE_TIMEOUT", cfg.WriteTimeout),
		platformconfig.ValidatePositiveDuration("HTTP_IDLE_TIMEOUT", cfg.IdleTimeout),
		platformconfig.ValidatePositiveDuration("HTTP_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout),
		validateIntRange("HTTP_MAX_CONCURRENT_REQUESTS", cfg.MaxConcurrentRequests, 1, 1024),
	); err != nil {
		return HTTP{}, err
	}
	return cfg, nil
}

func loadBoundedInt64(env platformconfig.Env, key string, fallback, minimum, maximum int64) (int64, error) {
	value, err := env.Int(key, int(fallback))
	if err != nil {
		return 0, err
	}
	if int64(value) < minimum || int64(value) > maximum {
		return 0, fmt.Errorf("configuration %s must be between %d and %d", key, minimum, maximum)
	}
	return int64(value), nil
}

func validateIntRange(key string, value, minimum, maximum int) error {
	if value < minimum || value > maximum {
		return fmt.Errorf("configuration %s must be between %d and %d", key, minimum, maximum)
	}
	return nil
}
