package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"example.com/stock-analysis-flow/internal/adapters/marketdata"
	"example.com/stock-analysis-flow/internal/adapters/storage/mysqlredis"
	"example.com/stock-analysis-flow/internal/platform/config"
	"example.com/stock-analysis-flow/internal/platform/httpapi"
	"example.com/stock-analysis-flow/internal/platform/taskqueue"
	"example.com/stock-analysis-flow/internal/service"
)

func main() {
	// 先安装默认 JSON logger，确保读取 .env 失败时也有结构化错误记录。
	slog.SetDefault(newJSONLogger("info"))
	if err := config.LoadDotEnvIfExists(".env"); err != nil {
		fatal("加载 .env 失败", err)
	}
	// .env 加载后再读取 LOG_LEVEL；进程环境变量仍然优先。
	slog.SetDefault(newJSONLogger(os.Getenv("LOG_LEVEL")))
	storageCfg, err := mysqlredis.LoadConfigFromEnv()
	if err != nil {
		fatal("存储配置无效", err)
	}
	marketDataCfg, err := marketdata.ConfigFromEnv()
	if err != nil {
		fatal("Go 行情服务配置无效", err)
	}

	startupCtx, startupCancel := context.WithTimeout(context.Background(), 15*time.Second)
	store, err := mysqlredis.Open(startupCtx, storageCfg)
	startupCancel()
	if err != nil {
		fatal("连接 MariaDB/Redis 失败（检查 DB_HOST/DB_PASSWORD/REDIS_ADDR 和 DB_AUTO_MIGRATE）", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			slog.Warn("关闭数据库/缓存依赖失败", "service", "stock-analysis-api", "error", err.Error())
		}
	}()

	runner, err := service.NewRunnerWithMarketDataStores(marketDataCfg, store, store)
	if err != nil {
		fatal("创建分析 Runner 失败", err)
	}
	analysisTimeout, err := service.EnvDuration("ANALYSIS_TIMEOUT", 5*time.Minute)
	if err != nil {
		fatal("读取超时配置失败", err)
	}
	workers, err := intEnv("ANALYSIS_WORKERS", 2, 1, 32)
	if err != nil {
		fatal("读取 Worker 数量配置失败", err)
	}
	maxQueued, err := intEnv("ANALYSIS_MAX_QUEUED", 100, workers, 10000)
	if err != nil {
		fatal("读取最大排队任务数配置失败", err)
	}
	tasks, err := taskqueue.New(store.DB, runner, workers, maxQueued, analysisTimeout, 250*time.Millisecond)
	if err != nil {
		fatal("创建异步任务队列失败", err)
	}
	if err := tasks.Start(context.Background()); err != nil {
		fatal("启动异步任务 Worker 失败", err)
	}
	defer tasks.Close()

	api, err := httpapi.NewWithTasks(runner, store, analysisTimeout, tasks)
	if err != nil {
		fatal("创建 HTTP API 失败", err)
	}
	addr := strings.TrimSpace(os.Getenv("HTTP_ADDR"))
	if addr == "" {
		addr = ":8092"
	}
	readTimeout, err := service.EnvDuration("HTTP_READ_TIMEOUT", 15*time.Second)
	if err != nil {
		fatal("读取超时配置失败", err)
	}
	writeTimeout, err := service.EnvDuration("HTTP_WRITE_TIMEOUT", analysisTimeout+10*time.Second)
	if err != nil {
		fatal("读取超时配置失败", err)
	}
	idleTimeout, err := service.EnvDuration("HTTP_IDLE_TIMEOUT", 60*time.Second)
	if err != nil {
		fatal("读取超时配置失败", err)
	}
	server := &http.Server{Addr: addr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: readTimeout, WriteTimeout: writeTimeout, IdleTimeout: idleTimeout}

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("股票分析 Go API 已启动", "service", "stock-analysis-api", "addr", addr, "market_data_base_url", marketDataCfg.BaseURL, "db_auto_migrate", storageCfg.DBAutoMigrate)
		serveErr <- server.ListenAndServe()
	}()
	signalCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		if !errors.Is(err, http.ErrServerClosed) {
			fatal("HTTP 服务异常退出", err)
		}
	case <-signalCtx.Done():
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			slog.Warn("HTTP 服务优雅关闭失败", "service", "stock-analysis-api", "error", err.Error())
		}
		if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) && err != nil {
			slog.Warn("HTTP 服务停止时返回错误", "service", "stock-analysis-api", "error", err.Error())
		}
	}
}

func fatal(message string, err error) {
	if err != nil {
		slog.Error(message, "service", "stock-analysis-api", "error", err.Error())
	} else {
		slog.Error(message, "service", "stock-analysis-api")
	}
	os.Exit(1)
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

func intEnv(name string, fallback, minimum, maximum int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	if value < minimum || value > maximum {
		return 0, fmt.Errorf("%s must be between %d and %d", name, minimum, maximum)
	}
	return value, nil
}
