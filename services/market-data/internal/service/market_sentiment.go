package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// MarketSentiment 由同一交易日的市场宽度和涨跌停池构建情绪快照。
// 任一辅助池不可用时对应计数保持 null，不能按零处理。
func (s *DataService) MarketSentiment(ctx context.Context, rawDate string) (*model.MarketSentimentSnapshot, error) {
	loc := marketLocation()
	requestedDate := time.Now().In(loc).Format("2006-01-02")
	if rawDate != "" {
		parsed, err := time.Parse("2006-01-02", rawDate)
		if err != nil || parsed.Format("2006-01-02") != rawDate {
			return nil, fmt.Errorf("invalid market-sentiment date %q: expected YYYY-MM-DD", rawDate)
		}
		if rawDate != requestedDate {
			return nil, errors.New("historical sentiment snapshots are read-only; live collection only supports the current China market date")
		}
		requestedDate = rawDate
	}
	breadth, _, err := s.MarketBreadth(ctx)
	if err != nil {
		return nil, fmt.Errorf("market sentiment breadth: %w", err)
	}
	tradeDate := breadth.ProviderReportDate
	if tradeDate == "" {
		tradeDate = requestedDate
	}
	now := time.Now().UTC()
	item := &model.MarketSentimentSnapshot{
		Market: "cn_a_share", TradeDate: tradeDate,
		ProviderReportDate: breadth.ProviderReportDate,
		TotalSecurities: breadth.TotalSecurities, CoveredSecurities: breadth.CoveredSecurities,
		AdvancingCount: breadth.AdvancingCount, DecliningCount: breadth.DecliningCount,
		UnchangedCount: breadth.UnchangedCount, UnclassifiedCount: breadth.UnclassifiedCount,
		Source: breadth.Source, SourceVersion: breadth.SourceVersion,
		FetchedAt: now, QualityStatus: breadth.QualityStatus,
		QualityReasons: append([]string(nil), breadth.QualityReasons...),
		PoolSources: make(map[string]string), PoolSourceVersions: make(map[string]string),
	}
	if item.ProviderReportDate == "" {
		item.QualityStatus = model.QualityDegraded
		item.QualityReasons = appendReason(item.QualityReasons, "provider_report_date_unavailable")
	} else if item.ProviderReportDate != tradeDate {
		item.QualityStatus = model.QualityDegraded
		item.QualityReasons = appendReason(item.QualityReasons, "provider_report_date_mismatch")
	}
	for _, poolType := range []string{"up", "down", "broken"} {
		pool, _, poolErr := s.LimitPool(ctx, poolType, tradeDate, 100)
		if poolErr != nil || pool == nil || !pool.HasData {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "limit_pool_"+poolType+"_unavailable")
			continue
		}
		if pool.ProviderReportDate != "" && pool.ProviderReportDate != tradeDate {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "limit_pool_"+poolType+"_report_date_mismatch")
			continue
		}
		count := pool.Total
		item.PoolSources[poolType] = pool.Source
		item.PoolSourceVersions[poolType] = pool.SourceVersion
		switch poolType {
		case "up":
			item.LimitUpCount = &count
			item.LimitUpCountIsLowerBound = pool.TotalIsLowerBound
		case "down":
			item.LimitDownCount = &count
			item.LimitDownCountIsLowerBound = pool.TotalIsLowerBound
		case "broken":
			item.BrokenBoardCount = &count
			item.BrokenBoardCountIsLowerBound = pool.TotalIsLowerBound
		}
		if pool.TotalIsLowerBound {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "limit_pool_"+poolType+"_count_is_lower_bound")
		}
		if pool.QualityStatus != model.QualityValid {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "limit_pool_"+poolType+"_quality_degraded")
		}
	}
	if len(item.PoolSources) > 0 {
		parts := []string{item.Source}
		for _, poolType := range []string{"up", "down", "broken"} {
			if source := item.PoolSources[poolType]; source != "" && !strings.Contains(","+strings.Join(parts, ",")+",", ","+source+",") {
				parts = append(parts, source)
			}
		}
		item.Source = strings.Join(parts, ",")
	}
	if s.store != nil {
		if err := s.store.SaveMarketSentiment(ctx, item); err != nil {
			return item, fmt.Errorf("persist market sentiment snapshot: %w", err)
		}
	}
	return item, nil
}

// MarketSentimentHistory returns persisted daily snapshots; it never synthesizes missing historical observations.
func (s *DataService) MarketSentimentHistory(ctx context.Context, startDate, endDate string, limit int) (*model.MarketSentimentHistory, error) {
	if s.store == nil {
		return nil, errors.New("market-sentiment history storage is not enabled")
	}
	start, err := time.Parse("2006-01-02", startDate)
	if err != nil || start.Format("2006-01-02") != startDate {
		return nil, errors.New("start must use YYYY-MM-DD")
	}
	end, err := time.Parse("2006-01-02", endDate)
	if err != nil || end.Format("2006-01-02") != endDate || end.Before(start) {
		return nil, errors.New("end must use YYYY-MM-DD and not precede start")
	}
	if end.Sub(start) > 366*24*time.Hour {
		return nil, errors.New("market-sentiment history range must not exceed 366 days")
	}
	if limit == 0 {
		limit = 366
	}
	if limit < 1 || limit > 500 {
		return nil, errors.New("market-sentiment history limit must be between 1 and 500")
	}
	items, err := s.store.ListMarketSentiment(ctx, "cn_a_share", startDate, endDate, limit)
	if err != nil {
		return nil, err
	}
	return &model.MarketSentimentHistory{Market: "cn_a_share", StartDate: startDate, EndDate: endDate, Items: items, Total: len(items)}, nil
}
