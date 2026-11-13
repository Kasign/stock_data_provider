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
	newsconfig "example.com/stock-analysis-flow/internal/news/config"
	newsdiagnostics "example.com/stock-analysis-flow/internal/news/diagnostics"
	notificationapplication "example.com/stock-analysis-flow/internal/notification/application"
	notificationschema "example.com/stock-analysis-flow/internal/notification/schema"
	notificationstorage "example.com/stock-analysis-flow/internal/notification/storage"
	"example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpapi"
	platformsecurity "example.com/stock-analysis-flow/internal/platform/security"
	platformstorage "example.com/stock-analysis-flow/internal/platform/storage"
	"example.com/stock-analysis-flow/internal/platform/taskqueue"
	researchapplication "example.com/stock-analysis-flow/internal/research/application"
	researchstorage "example.com/stock-analysis-flow/internal/research/storage"
	researchflowstorage "example.com/stock-analysis-flow/internal/researchflow/storage"
	serverconfig "example.com/stock-analysis-flow/internal/server/config"
	serverdiagnostics "example.com/stock-analysis-flow/internal/server/diagnostics"
	serverhealth "example.com/stock-analysis-flow/internal/server/health"
	servermigration "example.com/stock-analysis-flow/internal/server/migration"
	serverrelay "example.com/stock-analysis-flow/internal/server/relay"
	serverresources "example.com/stock-analysis-flow/internal/server/resources"
	runtime "example.com/stock-analysis-flow/internal/server/runtime"
	serversecurity "example.com/stock-analysis-flow/internal/server/security"
	"example.com/stock-analysis-flow/internal/service"
)

// Run loads process configuration, assembles the currently supported runtime,
// and serves until the parent context is canceled or the HTTP server exits.
func Run(ctx context.Context) (runErr error) {
	if ctx == nil {
		return errors.New("bootstrap context is required")
	}

	if err := config.LoadDotEnvIfExists(".env"); err != nil {
		return fmt.Errorf("加载 .env 失败: %w", err)
	}

	env := config.NewEnv()
	processConfig, err := serverconfig.LoadProcess(env)
	if err != nil {
		return fmt.Errorf("Server 进程配置无效: %w", err)
	}
	slog.SetDefault(newJSONLogger(processConfig.LogLevel))

	// 存储与行情配置仍由各自兼容 Adapter 负责解析；Server 不复制其领域配置 Schema。
	storageCfg, err := mysqlredis.LoadConfig(env)
	if err != nil {
		return fmt.Errorf("存储配置无效: %w", err)
	}
	marketDataCfg, err := marketdata.LoadConfig(env)
	if err != nil {
		return fmt.Errorf("Go 行情服务配置无效: %w", err)
	}
	newsConfig, err := newsconfig.Load(env)
	if err != nil {
		return fmt.Errorf("News 配置无效: %w", err)
	}
	migrationCfg := processConfig.Migration
	serverRole := processConfig.Role
	shutdownTimeout := 10 * time.Second
	if serverRole != serverconfig.RoleWorker {
		shutdownTimeout = processConfig.HTTP.ShutdownTimeout
	}
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
	var lifecycle *runtime.Lifecycle
	defer func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if shutdownErr := stopConsumersThenCloseResources(
			stopCtx,
			func(ctx context.Context) error {
				if lifecycle == nil {
					return nil
				}
				return lifecycle.Stop(ctx)
			},
			storageResources.Close,
		); shutdownErr != nil {
			slog.Error("停止 Server 并关闭共享资源失败", "error", shutdownErr.Error())
			runErr = joinShutdownError(runErr, shutdownErr)
		}
	}()

	migrationRegistry, err := newMigrationRegistry()
	if err != nil {
		return fmt.Errorf("注册模块 Migration 失败: %w", err)
	}
	migrationCtx, migrationCancel := context.WithTimeout(ctx, migrationCfg.Timeout)
	if migrationCfg.Enabled {
		migrationReport, migrationErr := migrationRegistry.RunWithReport(migrationCtx, storageResources.DB)
		if migrationErr != nil {
			migrationCancel()
			slog.Error("模块 Migration 执行失败", "report", migrationReport, "error", migrationErr.Error())
			return fmt.Errorf("执行模块 Migration 失败: %w", migrationErr)
		}
		slog.Info("模块 Migration 执行完成", "report", migrationReport)
	} else {
		if err := migrationRegistry.ValidateApplied(migrationCtx, storageResources.DB); err != nil {
			migrationCancel()
			return fmt.Errorf("数据库 Schema 尚未满足当前模块版本要求（MIGRATION_ENABLED=false）: %w", err)
		}
		slog.Info("模块 Schema 预检通过", "migration_enabled", false)
	}
	migrationCancel()

	// Key Ring 配置为可选项；未配置时不创建分页 Facade，也不生成任何默认密钥。
	// 已配置时在 API/Worker 两种角色下使用同一共享 DB 构造真实 News/Research 依赖。
	var researchNews *researchNewsComposition
	if _, _, configured := newsConfig.EvidenceCursorKeyRing(); configured {
		researchNews, err = newResearchNewsComposition(storageResources.DB, newsConfig)
		if err != nil {
			return fmt.Errorf("装配 News→Research 依赖失败: %w", err)
		}
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

	lifecycle, err = runtime.NewLifecycleWithRollbackTimeout(shutdownTimeout)
	if err != nil {
		return fmt.Errorf("创建 Server Lifecycle 失败: %w", err)
	}
	if err := lifecycle.Register(legacyAnalysisWorker{name: "legacy-analysis-worker", queue: tasks, enabled: serverRole == serverconfig.RoleWorker || serverRole == serverconfig.RoleAll}); err != nil {
		return fmt.Errorf("注册旧 Analysis Worker 生命周期失败: %w", err)
	}

	if serverRole != serverconfig.RoleAPI {
		relay, relayErr := newResearchNotificationRelay(storageResources, processConfig.ResearchNotificationRelay)
		if relayErr != nil {
			return fmt.Errorf("创建 Research→Notification Outbox Relay 失败: %w", relayErr)
		}
		if err := lifecycle.Register(relay); err != nil {
			return fmt.Errorf("注册 Research→Notification Outbox Relay 生命周期失败: %w", err)
		}
		if processConfig.ResearchflowCapacity.Configured {
			researchflowRelay, relayErr := newResearchflowNotificationRelay(
				storageResources, processConfig.ResearchNotificationRelay, processConfig.ResearchflowCapacity,
			)
			if relayErr != nil {
				return fmt.Errorf("创建 Researchflow→Notification Outbox Relay 失败: %w", relayErr)
			}
			if err := lifecycle.Register(researchflowRelay); err != nil {
				return fmt.Errorf("注册 Researchflow→Notification Outbox Relay 生命周期失败: %w", err)
			}
		}
	}
	var httpComponent *httpServerComponent
	if serverRole != serverconfig.RoleWorker {
		api, err := httpapi.NewWithTasks(runner, store, analysisTimeout, tasks)
		if err != nil {
			return fmt.Errorf("创建旧 Analysis HTTP API 失败: %w", err)
		}
		diagnosticsService, err := newDiagnosticsService(storageResources, migrationRegistry, newsConfig)
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

	if err := lifecycle.Start(ctx); err != nil {
		return fmt.Errorf("启动 Server 运行组件失败: %w", err)
	}
	if researchNews != nil {
		// 目前没有 Researchflow Stage/HTTP 用例消费该组合；这里只确认生产依赖装配成功。
		slog.Info("News→Research 生产依赖已装配，等待业务消费端接入")
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

type notificationClock struct{}

func (notificationClock) Now() time.Time { return time.Now().UTC() }

type notificationReceiverAdapter struct {
	receiver *notificationapplication.EventReceiver
}

func (a notificationReceiverAdapter) Receive(ctx context.Context, raw []byte) error {
	if a.receiver == nil {
		return errors.New("Notification EventReceiver is not configured")
	}
	_, err := a.receiver.Receive(ctx, raw)
	return err
}

// newNotificationEventReceiver constructs Notification's validated Inbox acceptance path.
// Both Research and Researchflow relays share this stateless receiver over the same
// module-owned Inbox and FanoutWorkItem repositories.
func newNotificationEventReceiver(shared *serverresources.Resources, cfg serverconfig.ResearchNotificationRelay) (*notificationapplication.EventReceiver, error) {
	if shared == nil || shared.DB == nil {
		return nil, errors.New("shared MariaDB resource is required for Notification EventReceiver")
	}
	fanoutWriter, err := notificationstorage.NewMySQLFanoutWorkItemRepository(shared.DB)
	if err != nil {
		return nil, fmt.Errorf("创建 Notification Fanout Writer 失败: %w", err)
	}
	inbox, err := notificationstorage.NewMySQLInboxRepository(shared.DB, fanoutWriter)
	if err != nil {
		return nil, fmt.Errorf("创建 Notification Inbox Repository 失败: %w", err)
	}
	registry, err := notificationschema.NewRegistry(os.DirFS("doc/notification/schema"), cfg.EventLimits)
	if err != nil {
		return nil, fmt.Errorf("加载 Notification Event Schema Registry 失败: %w", err)
	}
	receiver, err := notificationapplication.NewEventReceiver(registry, inbox, notificationClock{})
	if err != nil {
		return nil, fmt.Errorf("创建 Notification EventReceiver 失败: %w", err)
	}
	return receiver, nil
}

// newResearchNotificationRelay connects the Research-owned Outbox Port to
// Notification's validated Inbox acceptance path. Both modules keep their own
// persistence and transaction boundaries.
func newResearchNotificationRelay(shared *serverresources.Resources, cfg serverconfig.ResearchNotificationRelay) (*serverrelay.ResearchOutboxRelay, error) {
	if shared == nil || shared.DB == nil {
		return nil, errors.New("shared MariaDB resource is required for Research Notification relay")
	}
	outboxRepository, err := researchstorage.NewOutboxRepository(shared.DB, func() time.Time { return time.Now().UTC() })
	if err != nil {
		return nil, fmt.Errorf("创建 Research Outbox Repository 失败: %w", err)
	}
	outbox, err := researchapplication.NewOutboxService(outboxRepository, func() time.Time { return time.Now().UTC() })
	if err != nil {
		return nil, fmt.Errorf("创建 Research Outbox Service 失败: %w", err)
	}
	receiver, err := newNotificationEventReceiver(shared, cfg)
	if err != nil {
		return nil, err
	}
	return serverrelay.NewResearchOutboxRelay(outbox, notificationReceiverAdapter{receiver: receiver}, serverrelay.Config{
		WorkerID: cfg.WorkerID, BatchSize: cfg.BatchSize,
		PollInterval: cfg.PollInterval, LeaseDuration: cfg.LeaseDuration,
		RetryDelay: cfg.RetryDelay,
	}, slog.Default())
}

// newResearchflowNotificationRelay connects the Researchflow terminal Outbox
// to the same Notification EventReceiver. Capacity is explicit because the
// Run repository constructor requires the same database-wide admission limits
// used by CreateRun and Worker claims.
func newResearchflowNotificationRelay(
	shared *serverresources.Resources,
	cfg serverconfig.ResearchNotificationRelay,
	capacity serverconfig.ResearchflowCapacity,
) (*serverrelay.ResearchflowNotificationRelay, error) {
	if shared == nil || shared.DB == nil || !capacity.Configured {
		return nil, errors.New("shared MariaDB resource and explicit Researchflow capacity are required")
	}
	outbox, err := researchflowstorage.NewMySQLRunRepository(shared.DB, researchflowstorage.CapacityLimits{
		MaxQueuedRuns: capacity.MaxQueuedRuns,
		MaxWorkers:    capacity.MaxWorkers,
	})
	if err != nil {
		return nil, fmt.Errorf("创建 Researchflow Outbox Store 失败: %w", err)
	}
	receiver, err := newNotificationEventReceiver(shared, cfg)
	if err != nil {
		return nil, err
	}
	return serverrelay.NewResearchflowNotificationRelay(outbox, notificationReceiverAdapter{receiver: receiver}, serverrelay.ResearchflowNotificationConfig{
		WorkerID: cfg.WorkerID, BatchSize: cfg.BatchSize,
		PollInterval: cfg.PollInterval, LeaseDuration: cfg.LeaseDuration,
		PublishTimeout: cfg.LeaseDuration / 2,
	}, slog.Default())
}

// newDiagnosticsService composes Server resource/schema checks with the
// deterministic News parser self-checks. Domain checks remain owned by News.
func newDiagnosticsService(shared *serverresources.Resources, registry *servermigration.Registry, newsConfig newsconfig.Config) (*serverdiagnostics.Service, error) {
	return serverdiagnostics.NewServiceWithMigration(
		shared, registry,
		newsdiagnostics.ParserChecks(),
		newsdiagnostics.ProviderConfigChecks(newsConfig),
	)
}

func joinShutdownError(runErr, shutdownErr error) error {
	if shutdownErr == nil {
		return runErr
	}
	return errors.Join(runErr, fmt.Errorf("Server shutdown: %w", shutdownErr))
}

// stopConsumersThenCloseResources enforces the ownership boundary: all
// lifecycle-managed consumers stop before shared DB/Redis/HTTP resources close.
// Resource closure still runs when consumer shutdown reports an error.
func stopConsumersThenCloseResources(
	ctx context.Context,
	stopConsumers func(context.Context) error,
	closeResources func() error,
) error {
	var stopErr, closeErr error
	if stopConsumers != nil {
		stopErr = stopConsumers(ctx)
	}
	if closeResources != nil {
		closeErr = closeResources()
	}
	return errors.Join(stopErr, closeErr)
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
		{namespace: platformsecurity.OwnerCredentialMigrationNamespace, migrations: platformsecurity.OwnerCredentialMigrations()},
		{namespace: researchstorage.MigrationNamespace, migrations: researchstorage.Migrations()},
		{namespace: researchflowstorage.MigrationNamespace, migrations: researchflowstorage.Migrations()},
		{namespace: "notification", migrations: notificationstorage.Migrations()},
		{namespace: serversecurity.SecurityAuditMigrationNamespace, migrations: serversecurity.SecurityAuditMigrations()},
	}
	for _, registration := range registrations {
		if err := registry.Register(registration.namespace, registration.migrations); err != nil {
			return nil, fmt.Errorf("注册 %s Migration: %w", registration.namespace, err)
		}
	}
	return registry, nil
}
