package httpapi

import (
	"context"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

// QuoteApplication 承载行情快照和日线读取用例。
type QuoteApplication interface {
	Bundle(context.Context, string, int, time.Time, bool, model.AdjustmentBasis) (*model.Bundle, error)
	DailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, bool, []string, error)
	DailyBarsAsOf(context.Context, string, string, string, int, model.AdjustmentBasis, time.Time) ([]model.DailyBar, []string, error)
	Quote(context.Context, string) (*model.Quote, bool, error)
}

// MarketOverviewApplication 承载市场概览、板块、资金流和情绪用例。
type MarketOverviewApplication interface {
	DragonTiger(context.Context, string, int, int) (*model.DragonTigerSnapshot, bool, error)
	IndexQuotes(context.Context) (*model.IndexQuoteSnapshot, bool, error)
	LimitPool(context.Context, string, string, int) (*model.LimitPoolSnapshot, bool, error)
	MarketBreadth(context.Context) (*model.MarketBreadthSnapshot, bool, error)
	SectorRankingsByMetric(context.Context, string, string, string, int) (*model.SectorRankingSnapshot, bool, error)
	HotStocks(context.Context, string, int) (*model.HotRankingSnapshot, bool, error)
	MarketCapitalFlow(context.Context, string, string) (*model.MarketCapitalFlowSnapshot, bool, error)
	MarketSentiment(context.Context, string) (*model.MarketSentimentSnapshot, error)
	MarketSentimentHistory(context.Context, string, string, int) (*model.MarketSentimentHistory, error)
	MarketSession(context.Context, time.Time) (*model.MarketSessionSnapshot, error)
	MarketStocks(context.Context, int, int, bool, model.AdjustmentBasis) (*model.MarketStocksSnapshot, error)
}

// SecurityApplication 承载证券检索、基础信息和个股扩展数据用例。
type SecurityApplication interface {
	CapitalFlow(context.Context, string) (*model.CapitalFlow, bool, error)
	ChipDistribution(context.Context, string) (*model.ChipDistribution, bool, error)
	SearchStocks(context.Context, string, int) (*model.StockSearchSnapshot, error)
	StockAnnouncements(context.Context, string, string, string, int, int) (*model.StockAnnouncementSnapshot, bool, error)
	StockBasic(context.Context, string) (*model.StockBasic, bool, error)
	StockBoards(context.Context, string) (*model.StockBoardSnapshot, bool, error)
}

// FundamentalsApplication 承载财务与估值读取用例。
type FundamentalsApplication interface {
	Fundamentals(context.Context, string, int, int) (*model.FundamentalsSnapshot, bool, error)
	FundamentalsAsOf(context.Context, string, int, int, time.Time) (*model.FundamentalsSnapshot, error)
	Valuations(context.Context, []string) (*model.ValuationSnapshot, bool, error)
}

// IndexApplication 承载指数目录、成分和指数历史行情用例。
type IndexApplication interface {
	IndexCatalog(context.Context, string) (*model.IndexCatalogSnapshot, bool, error)
	IndexConstituentsAsOfFromSource(context.Context, string, string, string) (*model.IndexConstituentSnapshot, bool, error)
	IndexConstituentsFromSource(context.Context, string, string) (*model.IndexConstituentSnapshot, bool, error)
	IndexDailyBarsFromSource(context.Context, string, string, string, string) (*model.IndexDailyBarsSnapshot, bool, error)
}

// TradingApplication 承载交易日历、交易状态和涨跌停证据用例。
type TradingApplication interface {
	PriceLimitEventEvidence(context.Context, string, string) (*model.PriceLimitEventEvidence, error)
	SecurityTradingStatus(context.Context, string, string) (*model.SecurityTradingStatusSnapshot, error)
	SecurityTradingStatusAsOf(context.Context, string, string, time.Time) (*model.SecurityTradingStatusSnapshot, error)
	TradingCalendar(context.Context, string, string) (*model.TradingCalendarSnapshot, bool, error)
}

// DiagnosticsApplication 承载就绪、Provider 运行诊断和探测历史用例。
type DiagnosticsApplication interface {
	ProviderDiagnostics() []map[string]any
	Ready(context.Context) error
	ProbeProviderCapabilitiesWithOptions(context.Context, model.ProviderProbeOptions) (*model.ProviderProbeRun, error)
	ProviderProbeHistory(context.Context, int) (*model.ProviderProbeHistory, error)
}

// ApplicationDependencies 按用例域注入接口，避免使用单一巨型 Application 接口。
type ApplicationDependencies struct {
	Quotes         QuoteApplication
	MarketOverview MarketOverviewApplication
	Securities     SecurityApplication
	Fundamentals   FundamentalsApplication
	Index          IndexApplication
	Trading        TradingApplication
	Diagnostics    DiagnosticsApplication
}
