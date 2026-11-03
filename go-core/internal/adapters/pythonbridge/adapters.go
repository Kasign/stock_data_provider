package pythonbridge

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/adapters/marketdatahttp"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/contextpack"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

type MarketDataAdapter struct {
	client         *Client
	native         *marketdatahttp.Client
	nativeRequired bool
}

func NewMarketDataAdapter(c *Client) *MarketDataAdapter { return &MarketDataAdapter{client: c} }
func NewMarketDataAdapterWithNative(c *Client, native *marketdatahttp.Client, required bool) *MarketDataAdapter {
	return &MarketDataAdapter{client: c, native: native, nativeRequired: required}
}
func (a *MarketDataAdapter) FetchStockBundle(ctx context.Context, code string, lookback int, asOf string) (domain.StockBundle, error) {
	legacyRequest := map[string]any{"stock_code": code, "lookback_days": lookback}
	if strings.TrimSpace(asOf) != "" {
		legacyRequest["as_of"] = asOf
	}
	if a.native == nil {
		var out domain.StockBundle
		err := a.client.Post(ctx, "/internal/v1/market-data/stock-bundle", legacyRequest, &out)
		return out, err
	}
	out, err := a.native.FetchStockBundle(ctx, code, lookback, asOf)
	if err != nil {
		if a.nativeRequired {
			return domain.StockBundle{}, fmt.Errorf("required Go market-data service: %w", err)
		}
		var legacy domain.StockBundle
		legacyErr := a.client.Post(ctx, "/internal/v1/market-data/stock-bundle", legacyRequest, &legacy)
		if legacyErr != nil {
			return domain.StockBundle{}, errors.Join(err, legacyErr)
		}
		legacy.Warnings = append(legacy.Warnings, "native_market_data_unavailable_legacy_data_used")
		return legacy, nil
	}
	var aux struct {
		Code         string                   `json:"code"`
		Name         string                   `json:"name"`
		Chip         *domain.ChipDistribution `json:"chip"`
		Fundamentals map[string]any           `json:"fundamentals"`
		Boards       []map[string]any         `json:"boards"`
		Sources      map[string]string        `json:"sources"`
		Warnings     []string                 `json:"warnings"`
	}
	if auxErr := a.client.Post(ctx, "/internal/v1/market-data/stock-auxiliary", legacyRequest, &aux); auxErr != nil {
		out.Warnings = append(out.Warnings, "python_auxiliary_data_unavailable")
	} else {
		if out.Name == "" {
			out.Name = aux.Name
		}
		out.Chip, out.Fundamentals, out.Boards = aux.Chip, aux.Fundamentals, aux.Boards
		for k, v := range aux.Sources {
			if out.Sources == nil {
				out.Sources = map[string]string{}
			}
			out.Sources[k] = v
		}
		out.Warnings = append(out.Warnings, aux.Warnings...)
	}
	if out.Quote != nil && out.Quote.Name == "" {
		out.Quote.Name = out.Name
	}
	return out, nil
}
func (a *MarketDataAdapter) FetchMarketOverview(ctx context.Context, region, asOf string) (domain.MarketOverview, error) {
	var out domain.MarketOverview
	request := map[string]any{"region": region}
	if strings.TrimSpace(asOf) != "" {
		request["as_of"] = asOf
	}
	err := a.client.Post(ctx, "/internal/v1/market-data/market-overview", request, &out)
	return out, err
}

type SearchAdapter struct{ client *Client }

func NewSearchAdapter(c *Client) *SearchAdapter { return &SearchAdapter{client: c} }
func (a *SearchAdapter) SearchStockNews(ctx context.Context, code, name string, limit int) ([]domain.NewsItem, error) {
	var out struct {
		Items    []domain.NewsItem `json:"items"`
		Warnings []string          `json:"warnings,omitempty"`
	}
	err := a.client.Post(ctx, "/internal/v1/search/stock-news", map[string]any{"stock_code": code, "stock_name": name, "max_results": limit}, &out)
	return out.Items, err
}
func (a *SearchAdapter) SearchMarketNews(ctx context.Context, region string) ([]domain.NewsItem, error) {
	var out struct {
		Items    []domain.NewsItem `json:"items"`
		Warnings []string          `json:"warnings,omitempty"`
	}
	err := a.client.Post(ctx, "/internal/v1/search/market-news", map[string]any{"region": region}, &out)
	return out.Items, err
}

type AnalyzerAdapter struct{ client *Client }

func NewAnalyzerAdapter(c *Client) *AnalyzerAdapter { return &AnalyzerAdapter{client: c} }
func (a *AnalyzerAdapter) AnalyzeStock(ctx context.Context, input domain.AnalysisContext, news []domain.NewsItem, reportType, queryID, reportLanguage string) (domain.AnalysisResult, error) {
	var out domain.AnalysisResult
	legacyContext := contextpack.ToLegacyAnalyzerContext(input)
	err := a.client.Post(ctx, "/internal/v1/llm/analyze-stock", map[string]any{
		"context": legacyContext, "news_context": contextpack.FormatNews(news), "report_type": reportType,
		"query_id": queryID, "analysis_context_pack_summary": contextpack.QualitySummary(input.DataQuality),
		"report_language": reportLanguage, "search_performed": input.NewsSearchPerformed,
		"news_result_count": input.NewsResultCount, "news_evidence_present": len(news) > 0,
	}, &out)
	return out, err
}
func (a *AnalyzerAdapter) GenerateMarketReview(ctx context.Context, overview domain.MarketOverview, news []domain.NewsItem, region, queryID, language string) (string, error) {
	var out struct {
		Report string `json:"report"`
	}
	err := a.client.Post(ctx, "/internal/v1/llm/generate-market-review", map[string]any{"overview": overview, "news": news, "region": region, "query_id": queryID, "report_language": language}, &out)
	return out.Report, err
}

type PersistenceAdapter struct{ client *Client }

func NewPersistenceAdapter(c *Client) *PersistenceAdapter { return &PersistenceAdapter{client: c} }
func (a *PersistenceAdapter) SaveDailyBars(ctx context.Context, code string, bars []domain.DailyBar, source string) (int, error) {
	var out struct {
		SavedCount int `json:"saved_count"`
	}
	err := a.client.Post(ctx, "/internal/v1/persistence/daily-bars", map[string]any{
		"stock_code": code, "source": source, "bars": bars,
	}, &out)
	return out.SavedCount, err
}

func (a *PersistenceAdapter) SaveStockAnalysis(ctx context.Context, result domain.AnalysisResult, input domain.AnalysisContext, reportType, queryID string) (int64, error) {
	var out struct {
		SavedHistoryID int64 `json:"saved_history_id"`
	}
	err := a.client.Post(ctx, "/internal/v1/persistence/stock-analysis", map[string]any{
		"result": result, "context": input, "legacy_context": contextpack.ToLegacyAnalyzerContext(input),
		"analysis_context_pack_summary": contextpack.QualitySummary(input.DataQuality),
		"report_type":                   reportType, "query_id": queryID,
	}, &out)
	return out.SavedHistoryID, err
}
func (a *PersistenceAdapter) SaveMarketReview(ctx context.Context, report, region, queryID string, overview domain.MarketOverview) (int64, error) {
	var out struct {
		SavedHistoryID int64 `json:"saved_history_id"`
	}
	err := a.client.Post(ctx, "/internal/v1/persistence/market-review", map[string]any{"report": report, "region": region, "query_id": queryID, "overview": overview}, &out)
	return out.SavedHistoryID, err
}

type NotificationAdapter struct{ client *Client }

func NewNotificationAdapter(c *Client) *NotificationAdapter { return &NotificationAdapter{client: c} }
func (a *NotificationAdapter) NotifyStockAnalysis(ctx context.Context, result domain.AnalysisResult, reportType string) error {
	var out struct {
		Sent bool `json:"sent"`
	}
	dedupKey := fmt.Sprintf("%s:%s", result.QueryID, result.Code)
	if err := a.client.Post(ctx, "/internal/v1/notification/stock-analysis", map[string]any{
		"result": result, "report_type": reportType, "query_id": result.QueryID, "dedup_key": dedupKey,
	}, &out); err != nil {
		return err
	}
	if !out.Sent {
		return errors.New("python notification dispatcher reported no successful channel")
	}
	return nil
}
func (a *NotificationAdapter) NotifyMarketReview(ctx context.Context, report, region, queryID string) error {
	var out struct {
		Sent bool `json:"sent"`
	}
	if err := a.client.Post(ctx, "/internal/v1/notification/market-review", map[string]any{"report": report, "region": region, "query_id": queryID}, &out); err != nil {
		return err
	}
	if !out.Sent {
		return errors.New("python notification dispatcher reported no successful channel")
	}
	return nil
}

// Compile-time checks ensure the bridge adapters completely implement the ports.
var _ interface {
	FetchStockBundle(context.Context, string, int, string) (domain.StockBundle, error)
	FetchMarketOverview(context.Context, string, string) (domain.MarketOverview, error)
} = (*MarketDataAdapter)(nil)
var _ interface {
	SearchStockNews(context.Context, string, string, int) ([]domain.NewsItem, error)
	SearchMarketNews(context.Context, string) ([]domain.NewsItem, error)
} = (*SearchAdapter)(nil)
var _ interface {
	AnalyzeStock(context.Context, domain.AnalysisContext, []domain.NewsItem, string, string, string) (domain.AnalysisResult, error)
	GenerateMarketReview(context.Context, domain.MarketOverview, []domain.NewsItem, string, string, string) (string, error)
} = (*AnalyzerAdapter)(nil)
var _ interface {
	SaveDailyBars(context.Context, string, []domain.DailyBar, string) (int, error)
	SaveStockAnalysis(context.Context, domain.AnalysisResult, domain.AnalysisContext, string, string) (int64, error)
	SaveMarketReview(context.Context, string, string, string, domain.MarketOverview) (int64, error)
} = (*PersistenceAdapter)(nil)
var _ interface {
	NotifyStockAnalysis(context.Context, domain.AnalysisResult, string) error
	NotifyMarketReview(context.Context, string, string, string) error
} = (*NotificationAdapter)(nil)
