package bootstrap

import (
	"fmt"
	"reflect"
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
			origins, err := newEastmoneyProvider(cfg).HTTPOriginsForCapabilities(provider.EastmoneyCapabilities{
				Quote: settings.Quote, DailyBars: settings.DailyBars, StockBasic: settings.StockBasic,
				StockSearch: settings.StockSearch, SectorRankings: settings.SectorRankings, IndexQuotes: settings.IndexQuotes,
				MarketBreadth: settings.MarketBreadth, LimitPools: settings.LimitPools, StockBoards: settings.StockBoards,
				Announcements: settings.Announcements, DragonTiger: settings.DragonTiger, ChipDistribution: settings.ChipDistribution,
				CapitalFlow: settings.CapitalFlow, HotRankings: settings.HotRankings,
			})
			if err != nil {
				return fmt.Errorf("controlled egress for eastmoney: %w", err)
			}
			for _, origin := range origins {
				if isNilEgressDependency(deps.EastmoneyHTTPDoers[origin]) {
					return fmt.Errorf("controlled egress dependency missing: eastmoney origin %q", origin)
				}
			}
			continue
		}
		switch name {
		case "hithink":
			if isNilEgressDependency(deps.HithinkHTTPDoer) {
				return missingEgress("hithink HTTP")
			}
		case "baostock":
			if isNilEgressDependency(deps.BaoStockDialContext) {
				return missingEgress("baostock TCP")
			}
		case "tencent":
			if settings.Quote && isNilEgressDependency(deps.TencentQuoteHTTPDoer) {
				return missingEgress("tencent quote HTTP")
			}
			if settings.DailyBars && (isNilEgressDependency(deps.TencentKlineHTTPDoer) || isNilEgressDependency(deps.TencentNoneKlineHTTPDoer)) {
				return missingEgress("tencent daily-bars HTTP endpoints")
			}
			if settings.StockSearch && isNilEgressDependency(deps.TencentSearchHTTPDoer) {
				return missingEgress("tencent stock-search HTTP")
			}
		case "sina":
			if settings.Quote && isNilEgressDependency(deps.SinaQuoteHTTPDoer) {
				return missingEgress("sina quote HTTP")
			}
			if settings.DailyBars && isNilEgressDependency(deps.SinaKlineHTTPDoer) {
				return missingEgress("sina daily-bars HTTP")
			}
		case "yahoo":
			if isNilEgressDependency(deps.YahooHTTPDoer) {
				return missingEgress("yahoo HTTP")
			}
		case "netease":
			if settings.Quote && isNilEgressDependency(deps.NeteaseQuoteHTTPDoer) {
				return missingEgress("netease quote HTTP")
			}
			if settings.DailyBars && isNilEgressDependency(deps.NeteaseHistoryHTTPDoer) {
				return missingEgress("netease history HTTP")
			}
		case "tushare":
			if isNilEgressDependency(deps.TushareHTTPDoer) {
				return missingEgress("tushare HTTP")
			}
		case "tickflow":
			// Quote、指数、广度和行业排名走授权 API；日 K 在配置 API Key 时也走授权 API，
			// 未配置 Key 时才走 free API。只校验本次配置可能实际使用的出口。
			tickflowAuthenticated := settings.Quote || settings.IndexQuotes ||
				settings.MarketBreadth || settings.SectorRankings ||
				(settings.DailyBars && strings.TrimSpace(cfg.TickFlowAPIKey) != "")
			tickflowPublic := settings.DailyBars && strings.TrimSpace(cfg.TickFlowAPIKey) == ""
			if tickflowAuthenticated && isNilEgressDependency(deps.TickFlowAuthenticatedHTTPDoer) {
				return missingEgress("tickflow authenticated HTTP")
			}
			if tickflowPublic && isNilEgressDependency(deps.TickFlowPublicHTTPDoer) {
				return missingEgress("tickflow public HTTP")
			}
		case "pytdx":
			if isNilEgressDependency(deps.PytdxDialContext) {
				return missingEgress("pytdx TCP")
			}
		case "xueqiu":
			if isNilEgressDependency(deps.XueqiuHTTPDoer) {
				return missingEgress("xueqiu HTTP")
			}
		default:
			return fmt.Errorf("controlled egress validation does not recognize provider %q", name)
		}
	}
	if cfg.MXEnabled && isNilEgressDependency(deps.MiaoXiangHTTPDoer) {
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

// isNilEgressDependency 同时识别 nil interface 与承载 nil 指针/函数的 typed nil interface。
func isNilEgressDependency(value any) bool {
	if value == nil {
		return true
	}
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflected.IsNil()
	default:
		return false
	}
}
