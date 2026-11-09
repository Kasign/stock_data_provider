package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/bootstrap"
	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel(os.Getenv("LOG_LEVEL"))}))
	slog.SetDefault(logger)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, logger); err != nil {
		logger.Error("market-data service stopped", "error", err)
		os.Exit(1)
	}
}
func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	store, err := repository.OpenMySQL(ctx, cfg.MySQLDSN)
	if err != nil {
		return err
	}
	if store != nil {
		defer store.Close()
		if cfg.AutoMigrate {
			if err := store.Migrate(ctx); err != nil {
				return err
			}
		}
	}
	var c cache.Cache = cache.NewDisabled()
	if cfg.RedisAddr != "" {
		redis := cache.NewRedis(cfg.RedisAddr, cfg.RedisUsername, cfg.RedisPassword, cfg.RedisDB, 3*time.Second)
		pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		err := redis.Ping(pingCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("Redis configured but unavailable: %w", err)
		}
		c = redis
	}
	module, err := bootstrap.New(cfg, bootstrap.Dependencies{
		Store:  store,
		Cache:  c,
		Logger: logger,
	})
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
		defer cancel()
		if err := module.Close(closeCtx); err != nil {
			logger.Warn("关闭 Market Data Module 失败", "error", err)
		}
	}()
	if err := module.Start(ctx); err != nil {
		return err
	}
	handler := module.Handler()
	providerSummary := module.ProviderSummary()
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: cfg.RequestTimeout + 2*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	logger.Info("market-data service starting", "addr", cfg.HTTPAddr, "providers", strings.Join(providerSummary.All, ","), "quote_providers", strings.Join(providerSummary.Quotes, ","), "daily_bar_providers", strings.Join(providerSummary.DailyBars, ","), "mariadb_enabled", store != nil, "redis_enabled", cfg.RedisAddr != "", "adjustment", cfg.Adjustment, "mx_enabled", cfg.MXEnabled)
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	select {
	case e := <-errCh:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if e := server.Shutdown(shutdownCtx); e != nil {
			_ = server.Close()
			return e
		}
		e := <-errCh
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	}
}
func logLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
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
