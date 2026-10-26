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
		"eastmoney": provider.NewEastmoneyConfiguredWithAnnouncements(cfg.ProviderTimeout, cfg.EastmoneyQuoteURL, cfg.EastmoneyKlineURL, cfg.EastmoneyStockBasicURL, cfg.EastmoneySectorRankingURL, cfg.EastmoneyIndexURL, cfg.EastmoneyMarketStocksURL, cfg.EastmoneyLimitPoolBaseURL, cfg.EastmoneyStockBoardsURL, cfg.EastmoneyDragonTigerURL, cfg.EastmoneyAnnouncementsURL, cfg.EastmoneyUT),
		"hithink":   provider.NewHithinkConfigured(cfg.ProviderTimeout, cfg.HithinkBaseURL, cfg.HithinkAPIKey),
		"baostock":  provider.NewBaostock(baostockClient),
		"tencent":   provider.NewTencentConfigured(cfg.ProviderTimeout, cfg.TencentQuoteURL, cfg.TencentKlineURL, cfg.TencentNoneKlineURL),
		"sina":      provider.NewSinaConfigured(cfg.ProviderTimeout, cfg.SinaQuoteURL, cfg.SinaKlineURL),
		"yahoo":     provider.NewYahooFinance(cfg.ProviderTimeout, cfg.YahooChartURL),
		"netease":   provider.NewNeteaseConfigured(cfg.ProviderTimeout, cfg.NeteaseQuoteURL, cfg.NeteaseHistoryURL),
		"tushare":   provider.NewTushareConfigured(cfg.ProviderTimeout, cfg.TushareAPIURL, cfg.TushareToken),
		"tickflow":  provider.NewTickFlowConfigured(cfg.ProviderTimeout, cfg.TickFlowAPIURL, cfg.TickFlowFreeAPIURL, cfg.TickFlowAPIKey),
		"pytdx":     provider.NewPytdx(cfg.ProviderTimeout, cfg.PytdxHosts...),
	}
	allProviders := make([]provider.NamedProvider, 0, len(available))
	quoteProviders := make([]provider.QuoteProvider, 0, len(available))
	barProviders := make([]provider.DailyBarProvider, 0, len(available))
	stockBasicProviders := make([]provider.StockBasicProvider, 0, len(available))
	stockSearchProviders := make([]provider.StockSearchProvider, 0, len(available))
	activeProviders := make(map[string]provider.NamedProvider, len(available))
	sectorRankingProviders := make([]provider.SectorRankingProvider, 0, len(available))
	indexQuoteProviders := make([]provider.IndexQuoteProvider, 0, len(available))
	marketBreadthProviders := make([]provider.MarketBreadthProvider, 0, len(available))
	limitPoolProviders := make([]provider.LimitPoolProvider, 0, len(available))
	dragonTigerProviders := make([]provider.DragonTigerProvider, 0, len(available))
	announcementProviders := make([]provider.AnnouncementProvider, 0, len(available))
	stockBoardsProviders := make([]provider.StockBoardsProvider, 0, len(available))
	tradingCalendarProviders := make([]provider.TradingCalendarProvider, 0, len(available))
	fundamentalsProviders := make([]provider.FundamentalsProvider, 0, len(available))
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
		activeProviders[name] = p
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
		if settings.StockBasic {
			stockBasicProvider, ok := p.(provider.StockBasicProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for stock basics but does not implement StockBasicProvider", name)
			}
			stockBasicProviders = append(stockBasicProviders, stockBasicProvider)
		}
		if settings.SectorRankings {
			sectorRankingProvider, ok := p.(provider.SectorRankingProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for sector rankings but does not implement SectorRankingProvider", name)
			}
			sectorRankingProviders = append(sectorRankingProviders, sectorRankingProvider)
		}
		if settings.IndexQuotes {
			indexQuoteProvider, ok := p.(provider.IndexQuoteProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for index quotes but does not implement IndexQuoteProvider", name)
			}
			indexQuoteProviders = append(indexQuoteProviders, indexQuoteProvider)
		}
		if settings.MarketBreadth {
			capability, ok := p.(provider.MarketBreadthProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for market breadth but does not implement MarketBreadthProvider", name)
			}
			marketBreadthProviders = append(marketBreadthProviders, capability)
		}
		if settings.LimitPools {
			capability, ok := p.(provider.LimitPoolProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for limit pools but does not implement LimitPoolProvider", name)
			}
			limitPoolProviders = append(limitPoolProviders, capability)
		}
		if settings.Announcements {
			capability, ok := p.(provider.AnnouncementProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for announcements but does not implement AnnouncementProvider", name)
			}
			announcementProviders = append(announcementProviders, capability)
		}
		if settings.DragonTiger {
			capability, ok := p.(provider.DragonTigerProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for dragon-tiger but does not implement DragonTigerProvider", name)
			}
			dragonTigerProviders = append(dragonTigerProviders, capability)
		}
		if settings.StockBoards {
			capability, ok := p.(provider.StockBoardsProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for stock boards but does not implement StockBoardsProvider", name)
			}
			stockBoardsProviders = append(stockBoardsProviders, capability)
		}
		if settings.TradingCalendar {
			capability, ok := p.(provider.TradingCalendarProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for trading calendar but does not implement TradingCalendarProvider", name)
			}
			tradingCalendarProviders = append(tradingCalendarProviders, capability)
		}
		if settings.Fundamentals {
			capability, ok := p.(provider.FundamentalsProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for fundamentals but does not implement FundamentalsProvider", name)
			}
			fundamentalsProviders = append(fundamentalsProviders, capability)
		}
	}
	// 股票搜索使用独立的源优先级：腾讯 Smartbox 按关键词检索，避免冷启动时扫描全市场目录；东方财富保留为后备。
	searchPriority := []string{"tencent", "eastmoney"}
	searchPriority = append(searchPriority, cfg.ProviderPriority...)
	seenSearchProviders := make(map[string]struct{}, len(searchPriority))
	for _, name := range searchPriority {
		if _, seen := seenSearchProviders[name]; seen {
			continue
		}
		seenSearchProviders[name] = struct{}{}
		p, enabled := activeProviders[name]
		if !enabled {
			continue
		}
		searchProvider, ok := p.(provider.StockSearchProvider)
		if !ok {
			continue
		}
		stockSearchProviders = append(stockSearchProviders, searchProvider)
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
	ds.SetStockBasicProviders(stockBasicProviders)
	ds.SetStockSearchProviders(stockSearchProviders)
	ds.SetSectorRankingProviders(sectorRankingProviders)
	ds.SetIndexQuoteProviders(indexQuoteProviders)
	ds.SetMarketBreadthProviders(marketBreadthProviders)
	ds.SetLimitPoolProviders(limitPoolProviders)
	ds.SetDragonTigerProviders(dragonTigerProviders)
	ds.SetAnnouncementProviders(announcementProviders)
	ds.SetStockBoardsProviders(stockBoardsProviders)
	ds.SetTradingCalendarProviders(tradingCalendarProviders)
	ds.SetFundamentalsProviders(fundamentalsProviders)
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
