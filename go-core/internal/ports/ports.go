package ports

import (
	"context"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

type MarketDataProvider interface {
	FetchStockBundle(ctx context.Context, code string, lookbackDays int, asOf string) (domain.StockBundle, error)
	FetchMarketOverview(ctx context.Context, region, asOf string) (domain.MarketOverview, error)
}

type NewsSearcher interface {
	SearchStockNews(ctx context.Context, code, name string, limit int) ([]domain.NewsItem, error)
	SearchMarketNews(ctx context.Context, region string) ([]domain.NewsItem, error)
}

type Analyzer interface {
	AnalyzeStock(ctx context.Context, input domain.AnalysisContext, news []domain.NewsItem, reportType, queryID, reportLanguage string) (domain.AnalysisResult, error)
	GenerateMarketReview(ctx context.Context, overview domain.MarketOverview, news []domain.NewsItem, region, queryID, reportLanguage string) (string, error)
}

type Persistence interface {
	SaveDailyBars(ctx context.Context, code string, bars []domain.DailyBar, source string) (int, error)
	SaveStockAnalysis(ctx context.Context, result domain.AnalysisResult, context domain.AnalysisContext, reportType string, queryID string) (int64, error)
	SaveMarketReview(ctx context.Context, report, region, queryID string, overview domain.MarketOverview) (int64, error)
}

type Notifier interface {
	NotifyStockAnalysis(ctx context.Context, result domain.AnalysisResult, reportType string) error
	NotifyMarketReview(ctx context.Context, report, region, queryID string) error
}
