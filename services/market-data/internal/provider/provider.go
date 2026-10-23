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

type LimitPoolProvider interface {
	NamedProvider
	FetchLimitPool(ctx context.Context, poolType, tradeDate string, limit int) (*model.LimitPoolSnapshot, error)
}

// SupplementProvider 用于承载可选补充数据// SupplementProvider 用于承载可选补充数据，这类能力不会进入实时行情或日 K 的兜底链。
type SupplementProvider interface {
	NamedProvider
	FetchChipDistribution(ctx context.Context, symbol string) (*model.ChipDistribution, error)
	FetchCapitalFlow(ctx context.Context, symbol string) (*model.CapitalFlow, error)
}
