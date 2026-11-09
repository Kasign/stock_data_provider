package bootstrap

import (
	"fmt"
	"strings"

	"example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

func newEastmoneyProvider(cfg config.Config) *provider.Eastmoney {
	return provider.NewEastmoneyConfiguredWithAnnouncements(
		cfg.ProviderTimeout, cfg.EastmoneyQuoteURL, cfg.EastmoneyKlineURL,
		cfg.EastmoneyStockBasicURL, cfg.EastmoneySectorRankingURL, cfg.EastmoneyIndexURL,
		cfg.EastmoneyMarketStocksURL, cfg.EastmoneyLimitPoolBaseURL, cfg.EastmoneyStockBoardsURL,
		cfg.EastmoneyDragonTigerURL, cfg.EastmoneyAnnouncementsURL, cfg.EastmoneyUT,
	).WithHotRankingURLs(cfg.EastmoneyHotRankingsURL, cfg.EastmoneyHotSurgeURL).WithCapitalFlowURL(cfg.EastmoneyCapitalFlowURL)
}

// validateEgressDependencies 仅校验实际启用 Provider 的出站注入是否完整。
// 具体 BoundDoer/BoundDialer 的目标地址、DNS/IP 和重定向策略由 Root 负责。
func validateEgressDependencies(cfg config.Config, deps Dependencies) error {
	for _, name := range cfg.ProviderPriority {
		settings := cfg.ProviderSettings[name]
		if !settings.Enabled || !hasConfiguredProviderCapability(settings) {
			continue
		}
		if name == "eastmoney" {
			origins, err := newEastmoneyProvider(cfg).HTTPOrigins()
			if err != nil {
				return fmt.Errorf("controlled egress for eastmoney: %w", err)
			}
			for _, origin := range origins {
				if deps.EastmoneyHTTPDoers[origin] == nil {
					return fmt.Errorf("controlled egress dependency missing: eastmoney origin %q", origin)
				}
			}
			continue
		}
		switch name {
		case "hithink":
			if deps.HithinkHTTPDoer == nil {
				return missingEgress("hithink HTTP")
			}
		case "baostock":
			if deps.BaoStockDialContext == nil {
				return missingEgress("baostock TCP")
			}
		case "tencent":
			if settings.Quote && deps.TencentQuoteHTTPDoer == nil {
				return missingEgress("tencent quote HTTP")
			}
			if settings.DailyBars && (deps.TencentKlineHTTPDoer == nil || deps.TencentNoneKlineHTTPDoer == nil) {
				return missingEgress("tencent daily-bars HTTP endpoints")
			}
			if settings.StockSearch && deps.TencentSearchHTTPDoer == nil {
				return missingEgress("tencent stock-search HTTP")
			}
		case "sina":
			if settings.Quote && deps.SinaQuoteHTTPDoer == nil {
				return missingEgress("sina quote HTTP")
			}
			if settings.DailyBars && deps.SinaKlineHTTPDoer == nil {
				return missingEgress("sina daily-bars HTTP")
			}
		case "yahoo":
			if deps.YahooHTTPDoer == nil {
				return missingEgress("yahoo HTTP")
			}
		case "netease":
			if settings.Quote && deps.NeteaseQuoteHTTPDoer == nil {
				return missingEgress("netease quote HTTP")
			}
			if settings.DailyBars && deps.NeteaseHistoryHTTPDoer == nil {
				return missingEgress("netease history HTTP")
			}
		case "tushare":
			if deps.TushareHTTPDoer == nil {
				return missingEgress("tushare HTTP")
			}
		case "tickflow":
			// Quote、指数、广度和行业排名走授权 API；日 K 在配置 API Key 时也走授权 API，
			// 未配置 Key 时才走 free API。只校验本次配置可能实际使用的出口。
			tickflowAuthenticated := settings.Quote || settings.IndexQuotes ||
				settings.MarketBreadth || settings.SectorRankings ||
				(settings.DailyBars && strings.TrimSpace(cfg.TickFlowAPIKey) != "")
			tickflowPublic := settings.DailyBars && strings.TrimSpace(cfg.TickFlowAPIKey) == ""
			if tickflowAuthenticated && deps.TickFlowAuthenticatedHTTPDoer == nil {
				return missingEgress("tickflow authenticated HTTP")
			}
			if tickflowPublic && deps.TickFlowPublicHTTPDoer == nil {
				return missingEgress("tickflow public HTTP")
			}
		case "pytdx":
			if deps.PytdxDialContext == nil {
				return missingEgress("pytdx TCP")
			}
		case "xueqiu":
			if deps.XueqiuHTTPDoer == nil {
				return missingEgress("xueqiu HTTP")
			}
		default:
			return fmt.Errorf("controlled egress validation does not recognize provider %q", name)
		}
	}
	if cfg.MXEnabled && deps.MiaoXiangHTTPDoer == nil {
		return missingEgress("miao-xiang HTTP")
	}
	return nil
}

// hasConfiguredProviderCapability 避免仅启用 Provider 开关、但未启用任何业务能力时，
// 对不会被调用的网络出口提出装配要求。
func hasConfiguredProviderCapability(settings config.ProviderSettings) bool {
	return settings.Quote || settings.DailyBars || settings.StockBasic || settings.StockSearch ||
		settings.SectorRankings || settings.IndexQuotes || settings.MarketBreadth ||
		settings.LimitPools || settings.StockBoards || settings.TradingCalendar ||
		settings.SecurityTradingStatus || settings.Fundamentals || settings.Valuations ||
		settings.IndexCatalog || settings.ChipDistribution || settings.CapitalFlow ||
		settings.MarketCapitalFlow || settings.HotRankings || settings.DragonTiger ||
		settings.Announcements
}

func missingEgress(name string) error {
	return fmt.Errorf("controlled egress dependency missing: %s", name)
}
