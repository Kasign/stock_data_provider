package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/ports"
)

type MarketReviewFlow struct {
	marketData  ports.MarketDataProvider
	search      ports.NewsSearcher
	analyzer    ports.Analyzer
	persistence ports.Persistence
	notifier    ports.Notifier
	now         func() time.Time
}

func NewMarketReviewFlow(m ports.MarketDataProvider, s ports.NewsSearcher, a ports.Analyzer, p ports.Persistence, n ports.Notifier) *MarketReviewFlow {
	return &MarketReviewFlow{marketData: m, search: s, analyzer: a, persistence: p, notifier: n, now: time.Now}
}

func (f *MarketReviewFlow) Run(ctx context.Context, request domain.MarketReviewRequest) (response domain.MarketWorkflowResponse, err error) {
	started := f.now().UTC()
	if f.marketData == nil {
		return response, errors.New("market data provider is not configured")
	}
	if f.analyzer == nil {
		return response, errors.New("market analyzer is not configured")
	}
	region := strings.ToLower(strings.TrimSpace(request.Region))
	if region == "" {
		region = "cn"
	}
	switch region {
	case "cn", "hk", "us", "jp", "kr":
	default:
		return response, &InputError{Message: fmt.Sprintf("unsupported market review region %q", region)}
	}
	asOf := request.AsOf
	asOfExplicit := !asOf.IsZero()
	if asOfExplicit {
		delta := started.Sub(asOf)
		if delta < 0 || delta > 90*time.Second {
			return response, &InputError{Message: "the current Python market-overview adapter only supports a live snapshot; an explicit as_of must be at or before execution and within 90 seconds"}
		}
	}
	queryID := strings.TrimSpace(request.QueryID)
	if queryID == "" {
		queryID = newQueryID("market", region, started)
	}
	trace := domain.WorkflowTrace{QueryID: queryID, StartedAt: started}
	warnings := make([]string, 0)
	asOfArgument := ""
	if asOfExplicit {
		asOfArgument = asOf.Format(time.RFC3339Nano)
	}
	overview, err := runStage(ctx, &trace, f.now, "fetch_market_overview", func() (domain.MarketOverview, error) {
		return f.marketData.FetchMarketOverview(ctx, region, asOfArgument)
	})
	if err != nil {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, fmt.Errorf("fetch market overview: %w", err)
	}
	overview.Region = region
	if !asOfExplicit {
		asOf = f.now().UTC()
	}
	warnings = append(warnings, overview.Warnings...)
	if !hasUsableMarketOverview(overview) {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, errors.New("market overview contains no usable index, breadth, turnover, or sector data; analysis was not generated")
	}
	var news []domain.NewsItem
	if f.search != nil {
		var newsErr error
		news, newsErr = runStage(ctx, &trace, f.now, "search_market_news", func() ([]domain.NewsItem, error) {
			return f.search.SearchMarketNews(ctx, region)
		})
		if newsErr != nil {
			warnings = append(warnings, "market news search failed: "+newsErr.Error())
		} else {
			filtered, newsWarnings := filterNewsByReference(news, asOf, asOfExplicit)
			news = filtered
			warnings = append(warnings, newsWarnings...)
		}
	} else {
		trace.Stages = append(trace.Stages, domain.StageResult{Name: "search_market_news", Status: "skipped", Message: "no search adapter configured"})
	}
	report, genErr := runStage(ctx, &trace, f.now, "generate_market_review", func() (string, error) {
		return f.analyzer.GenerateMarketReview(ctx, overview, news, region, queryID, request.ReportLanguage)
	})
	if genErr != nil {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, fmt.Errorf("generate market review: %w", genErr)
	}
	if strings.TrimSpace(report) == "" {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, errors.New("market review generation returned an empty report")
	}
	id := int64(0)
	if f.persistence != nil {
		saved, saveErr := runStage(ctx, &trace, f.now, "persist_market_review", func() (int64, error) { return f.persistence.SaveMarketReview(ctx, report, region, queryID, overview) })
		if saveErr != nil {
			warnings = append(warnings, "persistence failed: "+saveErr.Error())
		} else {
			id = saved
		}
	} else {
		trace.Stages = append(trace.Stages, domain.StageResult{Name: "persist_market_review", Status: "skipped", Message: "no persistence adapter configured"})
	}
	if request.Notify && f.notifier != nil {
		_, nerr := runStage(ctx, &trace, f.now, "notify_market_review", func() (struct{}, error) {
			return struct{}{}, f.notifier.NotifyMarketReview(ctx, report, region, queryID)
		})
		if nerr != nil {
			warnings = append(warnings, "notification failed: "+nerr.Error())
		}
	} else {
		trace.Stages = append(trace.Stages, domain.StageResult{Name: "notify_market_review", Status: "skipped", Message: "notification disabled or adapter unavailable"})
	}
	trace.FinishedAt = f.now().UTC()
	response = domain.MarketWorkflowResponse{Result: domain.MarketReviewResult{Region: region, QueryID: queryID, Report: report, Overview: overview, News: news, Warnings: dedupe(warnings), SavedHistoryID: id}, Trace: trace}
	return response, nil
}

func hasUsableMarketOverview(overview domain.MarketOverview) bool {
	for _, index := range overview.Indices {
		if strings.TrimSpace(index.Code) == "" && strings.TrimSpace(index.Name) == "" {
			continue
		}
		if !finiteMarketNumber(index.Current) || index.Current <= 0 || !finiteMarketNumber(index.Change) || !finiteMarketNumber(index.ChangePct) {
			continue
		}
		return true
	}
	if overview.UpCount > 0 || overview.DownCount > 0 || overview.FlatCount > 0 || overview.LimitUpCount > 0 || overview.LimitDownCount > 0 {
		return true
	}
	if finiteMarketNumber(overview.TotalAmount) && overview.TotalAmount > 0 {
		return true
	}
	return hasNamedEntry(overview.TopSectors) || hasNamedEntry(overview.BottomSectors) || hasNamedEntry(overview.TopConcepts) || hasNamedEntry(overview.BottomConcepts)
}

func hasNamedEntry(entries []map[string]any) bool {
	for _, item := range entries {
		for _, key := range []string{"name", "title", "code", "板块名称", "名称"} {
			if value, ok := item[key].(string); ok && strings.TrimSpace(value) != "" {
				return true
			}
		}
	}
	return false
}

func finiteMarketNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
