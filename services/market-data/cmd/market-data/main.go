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

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/baostock"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/config"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/httpapi"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/repository"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
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
		runQuoteRetentionWorker(ctx, store, cfg.QuoteRetention, logger)
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
	baostockClient := baostock.NewClient(baostock.Config{
		Host:           cfg.BaostockHost,
		Port:           cfg.BaostockPort,
		UserID:         cfg.BaostockUserID,
		Password:       cfg.BaostockPassword,
		ConnectTimeout: cfg.BaostockConnectTimeout,
		RequestTimeout: cfg.BaostockRequestTimeout,
	})
	defer func() {
		if err := baostockClient.Close(); err != nil {
			logger.Warn("关闭 BaoStock 客户端失败", "error", err)
		}
	}()
	available := map[string]provider.NamedProvider{
		"eastmoney": provider.NewEastmoneyConfigured(cfg.ProviderTimeout, cfg.EastmoneyQuoteURL, cfg.EastmoneyKlineURL, cfg.EastmoneyUT),
		"baostock":  provider.NewBaostock(baostockClient),
		"tencent":   provider.NewTencentConfigured(cfg.ProviderTimeout, cfg.TencentQuoteURL, cfg.TencentKlineURL, cfg.TencentNoneKlineURL),
		"sina":      provider.NewSina(cfg.ProviderTimeout, cfg.SinaQuoteURL),
	}
	allProviders := make([]provider.NamedProvider, 0, len(available))
	quoteProviders := make([]provider.QuoteProvider, 0, len(available))
	barProviders := make([]provider.DailyBarProvider, 0, len(available))
	for _, name := range cfg.ProviderPriority {
		p, ok := available[name]
		if !ok {
			return fmt.Errorf("unknown provider %q in MARKET_DATA_PROVIDER_PRIORITY", name)
		}
		settings := cfg.ProviderSettings[name]
		if !settings.Enabled {
			continue
		}
		allProviders = append(allProviders, p)
		if settings.Quote {
			quoteProvider, ok := p.(provider.QuoteProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for quotes but does not implement QuoteProvider", name)
			}
			quoteProviders = append(quoteProviders, quoteProvider)
		}
		if settings.DailyBars {
			barProvider, ok := p.(provider.DailyBarProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for daily bars but does not implement DailyBarProvider", name)
			}
			barProviders = append(barProviders, barProvider)
		}
	}
	if len(allProviders) == 0 {
		return errors.New("provider priority has no enabled providers")
	}
	ds := service.NewWithCapabilities(allProviders, quoteProviders, barProviders, c, store, cfg.QuoteCacheTTL, cfg.BarsCacheTTL, cfg.DefaultDailyBars, cfg.MaxDailyBars, service.ResilienceConfig{Retries: cfg.ProviderRetries, CircuitFailureThreshold: cfg.CircuitFailureThreshold, CircuitCooldown: cfg.CircuitCooldown}, logger)
	supplementProviders := make([]provider.SupplementProvider, 0, 1)
	if cfg.MXEnabled {
		mx, mxErr := provider.NewMiaoXiang(cfg.MXAPIKey, cfg.MXAPIURL, cfg.MXAPITimeout)
		if mxErr != nil {
			return fmt.Errorf("configure MiaoXiang supplemental provider: %w", mxErr)
		}
		supplementProviders = append(supplementProviders, mx)
	}
	ds.SetSupplementProviders(supplementProviders)
	handler := httpapi.NewWithCapabilityInterfaces(cfg.APIKey, cfg.Adjustment, ds, allProviders, quoteProviders, barProviders, cfg.MaxDailyBars, cfg.RequestTimeout, logger)
	server := &http.Server{Addr: cfg.HTTPAddr, Handler: handler.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: cfg.RequestTimeout + 2*time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	logger.Info("market-data service starting", "addr", cfg.HTTPAddr, "providers", strings.Join(providerNames(allProviders), ","), "quote_providers", strings.Join(providerNames(quoteProviders), ","), "daily_bar_providers", strings.Join(providerNames(barProviders), ","), "mariadb_enabled", store != nil, "redis_enabled", cfg.RedisAddr != "", "adjustment", cfg.Adjustment, "mx_enabled", cfg.MXEnabled)
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
func runQuoteRetentionWorker(ctx context.Context, store *repository.Store, retention time.Duration, logger *slog.Logger) {
	if store == nil || retention < 24*time.Hour {
		return
	}
	if logger == nil {
		logger = slog.Default()
	}
	go func() {
		purge := func() {
			if err := ctx.Err(); err != nil {
				return
			}
			jobCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			cutoff := time.Now().UTC().Add(-retention)
			deleted, err := store.PurgeQuotesBefore(jobCtx, cutoff)
			if err != nil {
				logger.Error("quote snapshot retention purge failed", "cutoff", cutoff.Format(time.RFC3339Nano), "error", err)
				return
			}
			if deleted > 0 {
				logger.Info("quote snapshot retention purge completed", "deleted_rows", deleted, "retention", retention.String())
			}
		}
		purge()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				purge()
			}
		}
	}()
}

func providerNames[T provider.NamedProvider](ps []T) []string {
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name())
	}
	return out
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
