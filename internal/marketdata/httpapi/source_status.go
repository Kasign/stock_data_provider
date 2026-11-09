package httpapi

import (
	"sort"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

// SourceCapabilities 是 Bootstrap 已完成 Provider 配置和能力校验后的只读能力集合。
// BuildSourceStatuses 只把这些已启用能力映射为既有 HTTP SourceStatus，不执行探测或 Provider 调用。
type SourceCapabilities struct {
	All                    []provider.NamedProvider
	Quotes                 []provider.QuoteProvider
	DailyBars              []provider.DailyBarProvider
	StockBasic             []provider.StockBasicProvider
	StockSearch            []provider.StockSearchProvider
	SectorRankings         []provider.SectorRankingProvider
	IndexQuotes            []provider.IndexQuoteProvider
	MarketBreadth          []provider.MarketBreadthProvider
	LimitPools             []provider.LimitPoolProvider
	DragonTiger            []provider.DragonTigerProvider
	Announcements          []provider.AnnouncementProvider
	StockBoards            []provider.StockBoardsProvider
	TradingCalendar        []provider.TradingCalendarProvider
	SecurityTradingStatus  []provider.SecurityTradingStatusProvider
	Fundamentals            []provider.FundamentalsProvider
	Valuations              []provider.ValuationProvider
	IndexCatalog            []provider.IndexCatalogProvider
	ChipDistribution        []provider.ChipDistributionProvider
	CapitalFlow             []provider.CapitalFlowProvider
	MarketCapitalFlow       []provider.MarketCapitalFlowProvider
	HotRankings             []provider.HotRankingProvider
	Supplements             []provider.SupplementProvider
}

// BuildSourceStatuses 从已注册的 Provider 能力集合生成 HTTP 数据源能力快照。
// 数据来源是 Bootstrap 校验后实际启用的 Provider Name 和对应能力接口，不读取配置推测能力。
func BuildSourceStatuses(capabilities SourceCapabilities) []model.SourceStatus {
	enabled := func(names []string) map[string]bool {
		result := make(map[string]bool, len(names))
		for _, name := range names {
			result[name] = true
		}
		return result
	}
	providerNames := func[T interface{ Name() string }](providers []T) []string {
		names := make([]string, 0, len(providers))
		for _, p := range providers {
			names = append(names, p.Name())
		}
		return names
	}
	known := enabled(providerNames(capabilities.All))
	quoteEnabled := enabled(providerNames(capabilities.Quotes))
	barsEnabled := enabled(providerNames(capabilities.DailyBars))
	stockBasicEnabled := enabled(providerNames(capabilities.StockBasic))
	stockSearchEnabled := enabled(providerNames(capabilities.StockSearch))
	sectorRankingEnabled := enabled(providerNames(capabilities.SectorRankings))
	indexQuoteEnabled := enabled(providerNames(capabilities.IndexQuotes))
	marketBreadthEnabled := enabled(providerNames(capabilities.MarketBreadth))
	limitPoolEnabled := enabled(providerNames(capabilities.LimitPools))
	dragonTigerEnabled := enabled(providerNames(capabilities.DragonTiger))
	announcementsEnabled := enabled(providerNames(capabilities.Announcements))
	stockBoardsEnabled := enabled(providerNames(capabilities.StockBoards))
	tradingCalendarEnabled := enabled(providerNames(capabilities.TradingCalendar))
	securityTradingStatusEnabled := enabled(providerNames(capabilities.SecurityTradingStatus))
	fundamentalsEnabled := enabled(providerNames(capabilities.Fundamentals))
	valuationEnabled := enabled(providerNames(capabilities.Valuations))
	indexCatalogEnabled := enabled(providerNames(capabilities.IndexCatalog))
	chipEnabled := enabled(providerNames(capabilities.ChipDistribution))
	capitalFlowEnabled := enabled(providerNames(capabilities.CapitalFlow))
	marketCapitalFlowEnabled := enabled(providerNames(capabilities.MarketCapitalFlow))
	hotRankingEnabled := enabled(providerNames(capabilities.HotRankings))
	historicalIndexConstituentsEnabled := make(map[string]bool)
	for _, p := range capabilities.IndexCatalog {
		if _, ok := p.(provider.HistoricalIndexConstituentProvider); ok {
			historicalIndexConstituentsEnabled[p.Name()] = true
		}
	}

	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)

	sources := make([]model.SourceStatus, 0, len(names)+len(capabilities.Supplements))
	for _, name := range names {
		sources = append(sources, model.SourceStatus{
			Name: name, Enabled: true, QuoteCapable: quoteEnabled[name], DailyBarCapable: barsEnabled[name],
			StockBasicCapable: stockBasicEnabled[name], StockSearchCapable: stockSearchEnabled[name],
			SectorRankingCapable: sectorRankingEnabled[name], IndexQuoteCapable: indexQuoteEnabled[name],
			MarketBreadthCapable: marketBreadthEnabled[name], LimitPoolCapable: limitPoolEnabled[name],
			StockBoardsCapable: stockBoardsEnabled[name], TradingCalendarCapable: tradingCalendarEnabled[name],
			SecurityTradingStatusCapable: securityTradingStatusEnabled[name], FundamentalsCapable: fundamentalsEnabled[name],
			ValuationCapable: valuationEnabled[name], IndexCatalogCapable: indexCatalogEnabled[name],
			HistoricalIndexConstituentsCapable: historicalIndexConstituentsEnabled[name],
			ChipCapable: chipEnabled[name], CapitalFlowCapable: capitalFlowEnabled[name],
			MarketCapitalFlowCapable: marketCapitalFlowEnabled[name], HotRankingCapable: hotRankingEnabled[name],
			DragonTigerCapable: dragonTigerEnabled[name], AnnouncementsCapable: announcementsEnabled[name],
		})
	}
	for _, p := range capabilities.Supplements {
		sources = append(sources, model.SourceStatus{
			Name: p.Name(), Enabled: true, ChipCapable: true, CapitalFlowCapable: true,
		})
	}
	return sources
}
