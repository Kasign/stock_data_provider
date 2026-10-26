package provider

import (
	"context"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// NamedProvider 是数据源发现与诊断所需的最小接口。新增 Provider 不需要实现与自身无关的数据能力。
type NamedProvider interface {
	Name() string
}

// QuoteProvider 与 DailyBarProvider 是独立的能力接口。聚合 Provider 为原有实现保留兼容性；服务也可以分别注册仅支持行情或仅支持日 K 的适配器。
type QuoteProvider interface {
	NamedProvider
	FetchQuote(ctx context.Context, symbol string) (*model.Quote, error)
}

type DailyBarProvider interface {
	NamedProvider
	FetchDailyBars(ctx context.Context, symbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error)
}

type Provider interface {
	QuoteProvider
	DailyBarProvider
}

// StockBasicProvider 获取证券身份、上市日期等低频基础资料，不参与实时行情和日 K 兜底链。
type StockBasicProvider interface {
	NamedProvider
	FetchStockBasic(ctx context.Context, symbol string) (*model.StockBasic, error)
}

// StockSearchProvider 从真实证券目录按名称或代码搜索股票，不承担实时行情查询。
type StockSearchProvider interface {
	NamedProvider
	SearchStocks(ctx context.Context, query string, limit int) (*model.StockSearchSnapshot, error)
}

// SectorRankingProvider 提供行业/概念板块的实时排名快照，不参与个股行情回退链。
type SectorRankingProvider interface {
	NamedProvider
	FetchSectorRankings(ctx context.Context, category string, limit int) (*model.SectorRankingSnapshot, error)
}

// IndexQuoteProvider provides quotes for a separately allow-listed core index set.
type IndexQuoteProvider interface {
	NamedProvider
	FetchIndexQuotes(ctx context.Context) (*model.IndexQuoteSnapshot, error)
}

type MarketBreadthProvider interface {
	NamedProvider
	FetchMarketBreadth(ctx context.Context) (*model.MarketBreadthSnapshot, error)
}

// StockBoardsProvider returns board membership associated with one A-share.
type StockBoardsProvider interface {
	NamedProvider
	FetchStockBoards(ctx context.Context, symbol string) (*model.StockBoardSnapshot, error)
}

type LimitPoolProvider interface {
	NamedProvider
	FetchLimitPool(ctx context.Context, poolType, tradeDate string, limit int) (*model.LimitPoolSnapshot, error)
}

// TradingCalendarProvider returns explicit trading-day flags for an inclusive range.
type DragonTigerProvider interface {
	NamedProvider
	FetchDragonTiger(ctx context.Context, tradeDate string, page, pageSize int) (*model.DragonTigerSnapshot, error)
}

// AnnouncementProvider 获取单只 A 股在明确公告日期区间内的公告列表。
type AnnouncementProvider interface {
	NamedProvider
	FetchStockAnnouncements(ctx context.Context, symbol, startDate, endDate string, page, pageSize int) (*model.StockAnnouncementSnapshot, error)
}

type TradingCalendarProvider interface {
	NamedProvider
	FetchTradingCalendar(ctx context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error)
}

// FundamentalsProvider returns financial indicators for an explicit report period.
type FundamentalsProvider interface {
	NamedProvider
	FetchFundamentals(ctx context.Context, symbol string, year, quarter int) (*model.FundamentalsSnapshot, error)
}

 // IndexCatalogProvider 提供同花顺指数/板块目录、当前成分股和指数历史日线。
// 该接口使用独立的指数代码空间（.TI/.SH/.SZ），不进入个股 K 线回退链。
type IndexCatalogProvider interface {
	NamedProvider
	FetchIndexCatalog(ctx context.Context, tag string) (*model.IndexCatalogSnapshot, error)
	FetchIndexConstituents(ctx context.Context, indexSymbol string) (*model.IndexConstituentSnapshot, error)
	FetchIndexDailyBars(ctx context.Context, indexSymbol, startDate, endDate string) (*model.IndexDailyBarsSnapshot, error)
}

// ValuationProvider returns the latest PE/PB/PS/PCF snapshot for a bounded list of A-share symbols.
type ValuationProvider interface {
	NamedProvider
	FetchValuations(ctx context.Context, symbols []string) (*model.ValuationSnapshot, error)
}

// SupplementProvider 用于承载可选补充数据// SupplementProvider 用于承载可选补充数据，这类能力不会进入实时行情或日 K 的兜底链。
type SupplementProvider interface {
	NamedProvider
	FetchChipDistribution(ctx context.Context, symbol string) (*model.ChipDistribution, error)
	FetchCapitalFlow(ctx context.Context, symbol string) (*model.CapitalFlow, error)
}
