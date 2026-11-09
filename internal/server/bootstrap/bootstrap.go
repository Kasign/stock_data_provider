// Package bootstrap assembles the transitional Root Server runtime.
// Legacy Analysis components remain behind this boundary until their owning
// modules expose production-ready Application Ports and lifecycle contracts.
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/marketdata"
	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	"example.com/stock-analysis-flow/internal/platform/config"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/platform/httpapi"
	"example.com/stock-analysis-flow/internal/platform/taskqueue"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	notificationstorage "example.com/stock-analysis-flow/internal/notification/storage"
	researchstorage "example.com/stock-analysis-flow/internal/research/storage"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
	"example.com/stock-analysis-flow/internal/service"
)

// Run loads process configuration, assembles the currently supported runtime,
// and serves until the parent context is canceled or the HTTP server exits.
func Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("bootstrap context is required")
	}

	slog.SetDefault(newJSONLogger("info"))
	if err := config.LoadDotEnvIfExists(".env"); err != nil {
		return fmt.Errorf("加载 .env 失败: %w", err)
	}
	slog.SetDefault(newJSONLogger(os.Getenv("LOG_LEVEL")))

	storageCfg, err := mysqlredis.LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("存储配置无效: %w", err)
	}
	marketDataCfg, err := marketdata.ConfigFromEnv()
	if err != nil {
		return fmt.Errorf("Go 行情服务配置无效: %w", err)
	}
	migrationCfg, err := serverconfig.LoadMigration(config.NewEnv())
	if err != nil {
		return fmt.Errorf("Migration 配置无效: %w", err)
	}

	startupCtx, startupCancel := context.WithTimeout(ctx, 15*time.Second)
	storageResources, store, err := serverresources.OpenLegacyAnalysisResources(startupCtx, storageCfg)
	startupCancel()
	if err != nil {
		return fmt.Errorf("连接 MariaDB/Redis 失败（检查 DB_HOST/DB_PASSWORD/REDIS_ADDR 和 DB_AUTO_MIGRATE）: %w", err)
	}
	defer func() {
		if closeErr := storageResources.Close(); closeErr != nil {
			slog.Warn("关闭数据库/缓存依赖失败", "service", "stock-analysis-api", "error", closeErr.Error())
		}
	}()

	if migrationCfg.Enabled {
		migrationRegistry, err := newMigrationRegistry()
		if err != nil {
			return fmt.Errorf("注册模块 Migration 失败: %w", err)
		}
		migrationCtx, migrationCancel := context.WithTimeout(ctx, migrationCfg.Timeout)
		err = migrationRegistry.Run(migrationCtx, storageResources.DB)
		migrationCancel()
		if err != nil {
			return fmt.Errorf("执行模块 Migration 失败: %w", err)
		}
		slog.Info("模块 Migration 执行完成", "namespaces", migrationRegistry.Namespaces())
	}

	runner, err := service.NewRunnerWithMarketDataStores(marketDataCfg, store, store)
	if err != nil {
		return fmt.Errorf("创建分析 Runner 失败: %w", err)
	}
	analysisTimeout, err := service.EnvDuration("ANALYSIS_TIMEOUT", 5*time.Minute)
	if err != nil {
		return fmt.Errorf("读取 ANALYSIS_TIMEOUT 失败: %w", err)
	}
	workers, err := boundedIntEnv("ANALYSIS_WORKERS", 2, 1, 32)
	if err != nil {
		return err
	}
	maxQueued, err := boundedIntEnv("ANALYSIS_MAX_QUEUED", 100, workers, 10000)
	if err != nil {
		return err
	}
	httpConfig, err := serverconfig.LoadHTTP(config.NewEnv(), analysisTimeout)
	if err != nil {
		return fmt.Errorf("HTTP 服务配置无效: %w", err)
	}

	tasks, err := taskqueue.New(store.DB, runner, workers, maxQueued, analysisTimeout, 250*time.Millisecond)
	if err != nil {
		return fmt.Errorf("创建旧 Analysis 异步任务队列失败: %w", err)
	}
	if err := tasks.Start(ctx); err != nil {
		return fmt.Errorf("启动旧 Analysis Worker 失败: %w", err)
	}
	defer tasks.Close()

	api, err := httpapi.NewWithTasks(runner, store, analysisTimeout, tasks)
	if err != nil {
		return fmt.Errorf("创建旧 Analysis HTTP API 失败: %w", err)
	}
	server := &http.Server{
		Addr:              httpConfig.Address,
		Handler:           api.Handler(),
		ReadHeaderTimeout: httpConfig.ReadHeaderTimeout,
		ReadTimeout:       httpConfig.ReadTimeout,
		WriteTimeout:      httpConfig.WriteTimeout,
		IdleTimeout:       httpConfig.IdleTimeout,
	}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("股票分析 Go API 已启动",
			"service", "stock-analysis-api",
			"addr", httpConfig.Address,
			"market_data_base_url", marketDataCfg.BaseURL,
			"db_auto_migrate", storageCfg.DBAutoMigrate)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("HTTP 服务异常退出: %w", err)
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), httpConfig.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
			return fmt.Errorf("HTTP 服务优雅关闭失败: %w", err)
		}
		if err := <-serveErr; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("HTTP 服务停止时返回错误: %w", err)
		}
		return nil
	}
}

func boundedIntEnv(name string, fallback, minimum, maximum int) (int, error) {
	return boundedIntValue(os.Getenv(name), name, fallback, minimum, maximum)
}

func boundedIntValue(raw, name string, fallback, minimum, maximum int) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s 必须是整数: %w", name, err)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%s 必须在 %d 到 %d 之间", name, minimum, maximum)
	}
	return value, nil
}

func newJSONLogger(rawLevel string) *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(rawLevel)}))
}

func logLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}


// newMigrationRegistry assembles only module migrations already present in this
// root Go module. Module SQL remains in each module's storage package.
func newMigrationRegistry() (*servermigration.Registry, error) {
	registry := servermigration.NewRegistry()
	registrations := []struct {
		namespace  string
		migrations []platformstorage.Migration
	}{
		{namespace: "news", migrations: newsstorage.Migrations()},
		{namespace: researchstorage.MigrationNamespace, migrations: researchstorage.Migrations()},
		{namespace: "notification", migrations: notificationstorage.Migrations()},
	}
	for _, registration := range registrations {
		if err := registry.Register(registration.namespace, registration.migrations); err != nil {
			return nil, fmt.Errorf("注册 %s Migration: %w", registration.namespace, err)
		}
	}
	return registry, nil
}
