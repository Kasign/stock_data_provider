package config

import (
	"fmt"
	"time"

	platformconfig "example.com/stock-analysis-flow/internal/platform/config"
)

// Migration 配置只保留执行超时；Schema 迁移由 Server 启动统一自动执行。
type Migration struct {
	Timeout time.Duration
}

// LoadMigration 读取统一数据库迁移的执行超时。
func LoadMigration(env platformconfig.Env) (Migration, error) {
	cfg := Migration{Timeout: 2 * time.Minute}
	var err error
	if cfg.Timeout, err = env.Duration("MIGRATION_TIMEOUT", cfg.Timeout); err != nil {
		return Migration{}, err
	}
	if cfg.Timeout <= 0 || cfg.Timeout > 30*time.Minute {
		return Migration{}, fmt.Errorf("MIGRATION_TIMEOUT must be greater than 0 and no more than 30m")
	}
	return cfg, nil
}
