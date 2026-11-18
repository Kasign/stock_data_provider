package bootstrap

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	"example.com/stock-analysis-flow/internal/platform/config"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
)

// InitializeOwner consumes a one-time deployment secret without exposing an HTTP initialization route.
// The caller remains responsible for removing/unmounting the secret source after successful execution.
func InitializeOwner(ctx context.Context, initialSecret []byte) (runErr error) {
	defer clear(initialSecret)
	if ctx == nil {
		return errors.New("Owner initialization context is required")
	}
	if len(initialSecret) == 0 || len(initialSecret) > 1024 {
		return errors.New("initial Owner secret must contain 1 to 1024 bytes")
	}
	if err := config.LoadDotEnvIfExists(".env"); err != nil {
		return fmt.Errorf("加载 .env 失败: %w", err)
	}
	env := config.NewEnv()
	processConfig, err := serverconfig.LoadProcess(env)
	if err != nil {
		return fmt.Errorf("Server 进程配置无效: %w", err)
	}
	if !processConfig.OwnerSession.Enabled {
		return errors.New("OWNER_SESSION_ENABLED must be true for owner-init")
	}
	storageCfg, err := mysqlredis.LoadConfig(env)
	if err != nil {
		return fmt.Errorf("存储配置无效: %w", err)
	}
	startupCtx, startupCancel := context.WithTimeout(ctx, 15*time.Second)
	shared, _, err := serverresources.OpenLegacyAnalysisResources(startupCtx, storageCfg)
	startupCancel()
	if err != nil {
		return fmt.Errorf("连接 MariaDB/Redis 失败: %w", err)
	}
	defer func() {
		if closeErr := shared.Close(); closeErr != nil {
			runErr = errors.Join(runErr, fmt.Errorf("关闭 Owner 初始化共享资源失败: %w", closeErr))
		}
	}()

	registry, err := newMigrationRegistry()
	if err != nil {
		return fmt.Errorf("注册模块 Migration 失败: %w", err)
	}
	migrationCtx, migrationCancel := context.WithTimeout(ctx, processConfig.Migration.Timeout)
	if processConfig.Migration.Enabled {
		if _, err := registry.RunWithReport(migrationCtx, shared.DB); err != nil {
			migrationCancel()
			return fmt.Errorf("执行模块 Migration 失败: %w", err)
		}
	} else if err := registry.ValidateApplied(migrationCtx, shared.DB); err != nil {
		migrationCancel()
		return fmt.Errorf("数据库 Schema 尚未满足当前模块版本要求: %w", err)
	}
	migrationCancel()

	composition, err := newOwnerSessionComposition(shared, processConfig.OwnerSession)
	if err != nil {
		return fmt.Errorf("装配 Owner Session 依赖失败: %w", err)
	}
	requestIDBytes := make([]byte, 16)
	if _, err := rand.Read(requestIDBytes); err != nil {
		return fmt.Errorf("生成 Owner 初始化 Request ID 失败: %w", err)
	}
	requestID := hex.EncodeToString(requestIDBytes)
	if err := composition.lifecycle.InitializeOwner(ctx, initialSecret, requestID); err != nil {
		return fmt.Errorf("初始化 Owner Credential 失败: %w", err)
	}
	return nil
}
