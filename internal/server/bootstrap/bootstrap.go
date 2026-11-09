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
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/marketdata"
	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	newsstorage "example.com/stock-analysis-flow/internal/news/adapters/storage"
	notificationstorage "example.com/stock-analysis-flow/internal/notification/storage"
	"example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpapi"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/platform/taskqueue"
	researchstorage "example.com/stock-analysis-flow/internal/research/storage"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverdiagnostics "example.com/stock-analysis-flow/internal/server/diagnostics"
	serverhealth "example.com/stock-analysis-flow/internal/server/health"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
	runtime "example.com/stock-analysis-flow/internal/server/runtime"
	"example.com/stock-analysis-flow/internal/service"
)

// Run loads process configuration, assembles the currently supported runtime,
// and serves until the parent context is canceled or the HTTP server exits.
func Run(ctx context.Context) error {
	if ctx == nil {
		return errors.New("bootstrap context is required")
	}

	if err := config.LoadDotEnvIfExists(".env"); err != nil {
		return fmt.Errorf("加载 .env 失败: %w", err)
	}

	processConfig, err := serverconfig.LoadProcess(config.NewEnv())
	if err != nil {
		return fmt.Errorf("Server 进程配置无效: %w", err)
	}
	slog.SetDefault(newJSONLogger(processConfig.LogLevel))

	// 存储与行情配置仍由各自兼容 Adapter 负责解析；Server 不复制其领域配置 Schema。
	storageCfg, err := mysqlredis.LoadConfigFromEnv()
	if err != nil {
		return fmt.Errorf("存储配置无效: %w", err)
	}
	marketDataCfg, err := marketdata.ConfigFromEnv()
	if err != nil {
		return fmt.Errorf("Go 行情服务配置无效: %w", err)
	}
	migrationCfg := processConfig.Migration
	serverRole := processConfig.Role
	analysisTimeout := processConfig.AnalysisTimeout
	workers := processConfig.AnalysisWorkers
	maxQueued := processConfig.AnalysisMaxQueued

	var authenticator *platformsecurity.Authenticator
	if serverRole != serverconfig.RoleWorker {
		authenticator, err = platformsecurity.NewAuthenticator(processConfig.Security.OwnerToken, nil)
		if err != nil {
			return fmt.Errorf("创建 HTTP 认证器失败: %w", err)
		}
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
	httpConfig := processConfig.HTTP

	tasks, err := taskqueue.New(store.DB, runner, workers, maxQueued, analysisTimeout, 250*time.Millisecond)
	if err != nil {
		return fmt.Errorf("创建旧 Analysis 异步任务队列失败: %w", err)
	}

	lifecycle := runtime.NewLifecycle()
	if err := lifecycle.Register(legacyAnalysisWorker{name: "legacy-analysis-worker", queue: tasks, enabled: serverRole == serverconfig.RoleWorker || serverRole == serverconfig.RoleAll}); err != nil {
		return fmt.Errorf("注册旧 Analysis Worker 生命周期失败: %w", err)
	}
	var httpComponent *httpServerComponent
	if serverRole != serverconfig.RoleWorker {
		api, err := httpapi.NewWithTasks(runner, store, analysisTimeout, tasks)
		if err != nil {
			return fmt.Errorf("创建旧 Analysis HTTP API 失败: %w", err)
		}
		diagnosticsService, err := serverdiagnostics.NewService(storageResources)
		if err != nil {
			return fmt.Errorf("创建 Server 诊断服务失败: %w", err)
		}
		healthHandler, err := serverhealth.NewHandler(diagnosticsService)
		if err != nil {
			return fmt.Errorf("创建 Health/Readiness Handler 失败: %w", err)
		}
		handler, err := newHTTPHandler(authenticator, healthHandler, api.Handler())
		if err != nil {
			return fmt.Errorf("注册 HTTP 路由失败: %w", err)
		}
		handler, err = newHTTPMiddleware(handler, httpMiddlewareConfig{
			maxRequestBodyBytes:   httpConfig.MaxRequestBodyBytes,
			maxConcurrentRequests: httpConfig.MaxConcurrentRequests,
		})
		if err != nil {
			return fmt.Errorf("配置 HTTP 请求限制失败: %w", err)
		}
		server := &http.Server{
			Addr:              httpConfig.Address,
			Handler:           handler,
			ReadHeaderTimeout: httpConfig.ReadHeaderTimeout,
			ReadTimeout:       httpConfig.ReadTimeout,
			WriteTimeout:      httpConfig.WriteTimeout,
			IdleTimeout:       httpConfig.IdleTimeout,
		}
		httpComponent, err = newHTTPServerComponent(server, httpConfig.Address, healthHandler)
		if err != nil {
			return fmt.Errorf("创建 HTTP 生命周期组件失败: %w", err)
		}
		if err := lifecycle.Register(httpComponent); err != nil {
			return fmt.Errorf("注册 HTTP Server 生命周期失败: %w", err)
		}
	}

	shutdownTimeout := 10 * time.Second
	if httpComponent != nil {
		shutdownTimeout = httpConfig.ShutdownTimeout
	}
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := lifecycle.Stop(stopCtx); err != nil {
			slog.Warn("停止 Server 运行组件失败", "error", err.Error())
		}
	}()

	if err := lifecycle.Start(ctx); err != nil {
		return fmt.Errorf("启动 Server 运行组件失败: %w", err)
	}
	if serverRole == serverconfig.RoleWorker {
		slog.Info("股票分析 Worker 已启动", "service", "stock-analysis-worker", "role", serverRole)
		<-ctx.Done()
		return nil
	}

	select {
	case <-ctx.Done():
		return nil
	case <-httpComponent.Done():
		return httpComponent.Result()
	}
}

type legacyAnalysisWorker struct {
	name    string
	queue   *taskqueue.Queue
	enabled bool
}

func (w legacyAnalysisWorker) Name() string { return w.name }
func (w legacyAnalysisWorker) Start(ctx context.Context) error {
	if !w.enabled {
		return nil
	}
	// Worker 的停止由 Lifecycle 统一控制，避免 SIGTERM 先取消 Worker、
	// 反而早于 HTTP 在途请求完成；Queue.Close 会取消内部运行 Context。
	return w.queue.Start(context.WithoutCancel(ctx))
}
func (w legacyAnalysisWorker) Stop(context.Context) error {
	w.queue.Close()
	return nil
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
