// Package bootstrap 负责组装 Market Data 模块的 Provider、Service、Application Port 与 HTTP Handler。
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/application"
	"example.com/stock-analysis-flow/internal/marketdata/baostock"
	"example.com/stock-analysis-flow/internal/marketdata/cache"
	"example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/httpapi"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/marketdata/repository"
	"example.com/stock-analysis-flow/internal/marketdata/service"
)

// Dependencies 是由调用方创建并拥有的基础设施依赖；Module 不负责关闭 Store/Cache。
type Dependencies struct {
	Store  *repository.Store
	Cache  cache.Cache
	Logger *slog.Logger
}

// Diagnostics 暴露模块就绪状态和 Provider 诊断，不暴露具体 DataService。
type Diagnostics interface {
	Ready(ctx context.Context) error
	ProviderDiagnostics() []map[string]any
}

// ProviderSummary 用于启动日志，记录实际启用的 Provider 与能力集合。
type ProviderSummary struct {
	All      []string
	Quotes   []string
	DailyBars []string
}

// Module 封装 Data 应用、HTTP Handler、诊断能力及模块自有资源。
type Module struct {
	handler        http.Handler
	application    app.MarketDataReader
	diagnostics    Diagnostics
	summary        ProviderSummary
	store          *repository.Store
	cache          cache.Cache
	logger         *slog.Logger
	baostockClient *baostock.Client
	closeOnce      sync.Once
	closeErr       error
}

// New 根据已加载配置和调用方提供的基础设施依赖创建 Market Data Module。
// Store/Cache 由调用方持有；Module 只拥有并关闭自身创建的 BaoStock 客户端。
func New(cfg config.Config, deps Dependencies) (*Module, error) {
	if strings.TrimSpace(cfg.MySQLDSN) != "" && deps.Store == nil {
		return nil, errors.New("MariaDB is configured but Store was not injected")
	}
	if strings.TrimSpace(cfg.RedisAddr) != "" && deps.Cache == nil {
		return nil, errors.New("Redis is configured but Cache was not injected")
	}
	if deps.Cache == nil {
		deps.Cache = cache.NewDisabled()
	}
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	module := &Module{
		store:  deps.Store,
		cache:  deps.Cache,
		logger: deps.Logger,
	}
	if err := module.build(cfg); err != nil {
		_ = module.Close()
		return nil, err
	}
	return module, nil
}

// Handler 返回由 Module 组装的 HTTP Handler。
func (m *Module) Handler() http.Handler {
	if m == nil {
		return nil
	}
	return m.handler
}

// Application 返回供模块内消费者调用的窄化 Application Port。
func (m *Module) Application() app.MarketDataReader {
	if m == nil {
		return nil
	}
	return m.application
}

// Diagnostics 返回模块就绪与 Provider 诊断能力。
func (m *Module) Diagnostics() Diagnostics {
	if m == nil {
		return nil
	}
	return m.diagnostics
}

// ProviderSummary 返回启动日志使用的 Provider 名称快照。
func (m *Module) ProviderSummary() ProviderSummary {
	if m == nil {
		return ProviderSummary{}
	}
	return ProviderSummary{
		All:      append([]string(nil), m.summary.All...),
		Quotes:   append([]string(nil), m.summary.Quotes...),
		DailyBars: append([]string(nil), m.summary.DailyBars...),
	}
}

// Close 只关闭 Module 自己创建的 BaoStock 客户端，不关闭调用方注入的 Store/Cache。
func (m *Module) Close() error {
	if m == nil {
		return nil
	}
	m.closeOnce.Do(func() {
		if m.baostockClient != nil {
			m.closeErr = m.baostockClient.Close()
		}
	})
	return m.closeErr
}

func (m *Module) build(cfg config.Config) error {
	store := m.store
	c := m.cache
	logger := m.logger
	baostockClient := baostock.NewClient(baostock.Config{
		Host:           cfg.BaostockHost,
		Port:           cfg.BaostockPort,
		UserID:         cfg.BaostockUserID,
		Password:       cfg.BaostockPassword,
		ConnectTimeout: cfg.BaostockConnectTimeout,
		RequestTimeout: cfg.BaostockRequestTimeout,
	})
	m.baostockClient = baostockClient
	available := map[string]provider.NamedProvider{
		"eastmoney": provider.NewEastmoneyConfiguredWithAnnouncements(cfg.ProviderTimeout, cfg.EastmoneyQuoteURL, cfg.EastmoneyKlineURL, cfg.EastmoneyStockBasicURL, cfg.EastmoneySectorRankingURL, cfg.EastmoneyIndexURL, cfg.EastmoneyMarketStocksURL, cfg.EastmoneyLimitPoolBaseURL, cfg.EastmoneyStockBoardsURL, cfg.EastmoneyDragonTigerURL, cfg.EastmoneyAnnouncementsURL, cfg.EastmoneyUT).WithHotRankingURLs(cfg.EastmoneyHotRankingsURL, cfg.EastmoneyHotSurgeURL).WithCapitalFlowURL(cfg.EastmoneyCapitalFlowURL),
		"hithink":   provider.NewHithinkConfigured(cfg.ProviderTimeout, cfg.HithinkBaseURL, cfg.HithinkAPIKey),
		"baostock":  provider.NewBaostock(baostockClient),
		"tencent":   provider.NewTencentConfiguredWithSearchURL(cfg.ProviderTimeout, cfg.TencentQuoteURL, cfg.TencentKlineURL, cfg.TencentNoneKlineURL, cfg.TencentStockSearchURL),
		"sina":      provider.NewSinaConfigured(cfg.ProviderTimeout, cfg.SinaQuoteURL, cfg.SinaKlineURL),
		"yahoo":     provider.NewYahooFinance(cfg.ProviderTimeout, cfg.YahooChartURL),
		"netease":   provider.NewNeteaseConfigured(cfg.ProviderTimeout, cfg.NeteaseQuoteURL, cfg.NeteaseHistoryURL),
		"tushare":   provider.NewTushareConfigured(cfg.ProviderTimeout, cfg.TushareAPIURL, cfg.TushareToken),
		"tickflow":  provider.NewTickFlowConfigured(cfg.ProviderTimeout, cfg.TickFlowAPIURL, cfg.TickFlowFreeAPIURL, cfg.TickFlowAPIKey),
		"pytdx":     provider.NewPytdx(cfg.ProviderTimeout, cfg.PytdxHosts...),
		"xueqiu":    provider.NewXueqiuConfigured(cfg.ProviderTimeout, cfg.XueqiuBaseURL, cfg.XueqiuCookie),
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
	securityTradingStatusProviders := make([]provider.SecurityTradingStatusProvider, 0, len(available))
	fundamentalsProviders := make([]provider.FundamentalsProvider, 0, len(available))
	valuationProviders := make([]provider.ValuationProvider, 0, len(available))
	indexCatalogProviders := make([]provider.IndexCatalogProvider, 0, len(available))
	chipDistributionProviders := make([]provider.ChipDistributionProvider, 0, len(available))
	capitalFlowProviders := make([]provider.CapitalFlowProvider, 0, len(available))
	marketCapitalFlowProviders := make([]provider.MarketCapitalFlowProvider, 0, len(available))
	hotRankingProviders := make([]provider.HotRankingProvider, 0, len(available))
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
		if settings.SecurityTradingStatus {
			if !settings.StockBasic {
				return fmt.Errorf("provider %q security_trading_status requires stock_basic to be enabled for listing-date verification", name)
			}
			if !settings.TradingCalendar {
				return fmt.Errorf("provider %q security_trading_status requires trading_calendar to be enabled", name)
			}
			capability, ok := p.(provider.SecurityTradingStatusProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for security trading status but does not implement SecurityTradingStatusProvider", name)
			}
			securityTradingStatusProviders = append(securityTradingStatusProviders, capability)
		}
		if settings.Fundamentals {
			capability, ok := p.(provider.FundamentalsProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for fundamentals but does not implement FundamentalsProvider", name)
			}
			fundamentalsProviders = append(fundamentalsProviders, capability)
		}
		if settings.Valuations {
			capability, ok := p.(provider.ValuationProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for valuations but does not implement ValuationProvider", name)
			}
			valuationProviders = append(valuationProviders, capability)
		}
		if settings.IndexCatalog {
			capability, ok := p.(provider.IndexCatalogProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for index catalog but does not implement IndexCatalogProvider", name)
			}
			indexCatalogProviders = append(indexCatalogProviders, capability)
		}
		if settings.ChipDistribution {
			capability, ok := p.(provider.ChipDistributionProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for chip distribution but does not implement ChipDistributionProvider", name)
			}
			chipDistributionProviders = append(chipDistributionProviders, capability)
		}
		if settings.CapitalFlow {
			capability, ok := p.(provider.CapitalFlowProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for capital flow but does not implement CapitalFlowProvider", name)
			}
			capitalFlowProviders = append(capitalFlowProviders, capability)
		}
		if settings.MarketCapitalFlow {
			capability, ok := p.(provider.MarketCapitalFlowProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for market capital flow but does not implement MarketCapitalFlowProvider", name)
			}
			marketCapitalFlowProviders = append(marketCapitalFlowProviders, capability)
		}
		if settings.HotRankings {
			capability, ok := p.(provider.HotRankingProvider)
			if !ok {
				return fmt.Errorf("provider %q is configured for hot rankings but does not implement HotRankingProvider", name)
			}
			hotRankingProviders = append(hotRankingProviders, capability)
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
		if !cfg.ProviderSettings[name].StockSearch {
			continue
		}
		searchProvider, ok := p.(provider.StockSearchProvider)
		if !ok {
			return fmt.Errorf("provider %q is configured for stock search but does not implement StockSearchProvider", name)
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
	ds.SetSecurityTradingStatusProviders(securityTradingStatusProviders)
	ds.SetFundamentalsProviders(fundamentalsProviders)
	ds.SetValuationProviders(valuationProviders)
	ds.SetIndexCatalogProviders(indexCatalogProviders)
	ds.SetChipDistributionProviders(chipDistributionProviders)
	ds.SetCapitalFlowProviders(capitalFlowProviders)
	ds.SetMarketCapitalFlowProviders(marketCapitalFlowProviders)
	ds.SetHotRankingProviders(hotRankingProviders)
	handler := httpapi.NewWithCapabilityInterfaces(cfg.APIKey, cfg.Adjustment, ds, allProviders, quoteProviders, barProviders, cfg.MaxDailyBars, cfg.RequestTimeout, logger)
	m.diagnostics = ds
	reader, err := application.NewReader(ds)
	if err != nil {
		return err
	}
	m.application = reader
	m.handler = handler.Handler()
	m.summary = ProviderSummary{
		All:      providerNames(allProviders),
		Quotes:   providerNames(quoteProviders),
		DailyBars: providerNames(barProviders),
	}
	return nil

}

func providerNames[T provider.NamedProvider](providers []T) []string {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name())
	}
	return names
}
