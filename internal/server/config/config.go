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
	RoleAPI Role = "api"
	RoleWorker Role = "worker"
	RoleAll Role = "all"
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
// HTTP contains the HTTP listener and graceful-shutdown settings owned by Server.
type HTTP struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
}

// LoadHTTP reads and validates Server-owned HTTP settings. The legacy Analysis
// timeout is supplied only to preserve the existing HTTP write-timeout default.
func LoadHTTP(env platformconfig.Env, analysisTimeout time.Duration) (HTTP, error) {
	if analysisTimeout <= 0 {
		return HTTP{}, fmt.Errorf("analysis timeout must be positive")
	}
	cfg := HTTP{
		Address:           strings.TrimSpace(env.String("HTTP_ADDR", ":8092")),
		ReadHeaderTimeout: 5 * time.Second,
		ShutdownTimeout:   10 * time.Second,
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
	if err := platformconfig.JoinValidationErrors(
		platformconfig.ValidatePositiveDuration("HTTP_READ_TIMEOUT", cfg.ReadTimeout),
		platformconfig.ValidatePositiveDuration("HTTP_WRITE_TIMEOUT", cfg.WriteTimeout),
		platformconfig.ValidatePositiveDuration("HTTP_IDLE_TIMEOUT", cfg.IdleTimeout),
		platformconfig.ValidatePositiveDuration("HTTP_SHUTDOWN_TIMEOUT", cfg.ShutdownTimeout),
	); err != nil {
		return HTTP{}, err
	}
	return cfg, nil
}
