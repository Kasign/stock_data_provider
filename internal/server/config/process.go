package config

import (
	"fmt"
	"strings"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// Process 是 Bootstrap 使用的不可变进程配置快照。
// 领域模块配置仍由模块自己的配置入口解析，避免 Server 接管领域参数所有权。
type Process struct {
	LogLevel          string
	AnalysisTimeout   time.Duration
	AnalysisWorkers   int
	AnalysisMaxQueued int
	Role              Role
	Migration         Migration
	Security          Security
	HTTP              HTTP
}

// LoadProcess 在启动阶段一次性解析 Server 进程级配置。
// Worker 角色不加载 HTTP 和 Owner HTTP 认证配置。
func LoadProcess(env platformconfig.Env) (Process, error) {
	cfg := Process{
		LogLevel: strings.ToLower(strings.TrimSpace(env.String("LOG_LEVEL", "info"))),
	}
	switch cfg.LogLevel {
	case "", "info":
		cfg.LogLevel = "info"
	case "debug", "warn", "warning", "error":
	default:
		return Process{}, fmt.Errorf("LOG_LEVEL 必须是 debug、info、warn 或 error")
	}

	var err error
	if cfg.AnalysisTimeout, err = env.Duration("ANALYSIS_TIMEOUT", 5*time.Minute); err != nil {
		return Process{}, err
	}
	if cfg.AnalysisTimeout <= 0 {
		return Process{}, fmt.Errorf("ANALYSIS_TIMEOUT 必须大于 0")
	}
	if cfg.AnalysisWorkers, err = env.Int("ANALYSIS_WORKERS", 2); err != nil {
		return Process{}, err
	}
	if err := validateIntRange("ANALYSIS_WORKERS", cfg.AnalysisWorkers, 1, 32); err != nil {
		return Process{}, err
	}
	if cfg.AnalysisMaxQueued, err = env.Int("ANALYSIS_MAX_QUEUED", 100); err != nil {
		return Process{}, err
	}
	if err := validateIntRange("ANALYSIS_MAX_QUEUED", cfg.AnalysisMaxQueued, cfg.AnalysisWorkers, 10000); err != nil {
		return Process{}, err
	}
	if cfg.Role, err = LoadRole(env); err != nil {
		return Process{}, err
	}
	if cfg.Migration, err = LoadMigration(env); err != nil {
		return Process{}, err
	}
	if cfg.Role != RoleWorker {
		if cfg.Security, err = LoadSecurity(env); err != nil {
			return Process{}, err
		}
		if cfg.HTTP, err = LoadHTTP(env, cfg.AnalysisTimeout); err != nil {
			return Process{}, err
		}
	}
	return cfg, nil
}
