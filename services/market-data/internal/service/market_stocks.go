package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

// StockUniverse 使用已配置的证券搜索目录 Provider 获取完整 A 股证券池。
func (s *DataService) StockUniverse(ctx context.Context) (*model.StockUniverseSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var failures []string
	for _, candidate := range s.stockSearchProviders {
		universeProvider, ok := candidate.(provider.StockUniverseProvider)
		if !ok {
			continue
		}
		snapshot, err := universeProvider.ListStockUniverse(ctx)
		if err != nil {
			failures = append(failures, candidate.Name()+": "+err.Error())
			continue
		}
		if snapshot == nil || snapshot.Total <= 0 || snapshot.Total != len(snapshot.Items) ||
			snapshot.Source == "" || snapshot.SourceVersion == "" || snapshot.FetchedAt.IsZero() ||
			(snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded && snapshot.QualityStatus != model.QualityStale) {
			failures = append(failures, candidate.Name()+": invalid or incomplete stock-universe snapshot")
			continue
		}
		seen := make(map[string]struct{}, len(snapshot.Items))
		valid := true
		for _, item := range snapshot.Items {
			symbol, symbolErr := normalize.NormalizeSymbol(item.Symbol)
			if symbolErr != nil || symbol != item.Symbol || item.Code == "" || item.Name == "" {
				valid = false
				break
			}
			if _, exists := seen[symbol]; exists {
				valid = false
				break
			}
			seen[symbol] = struct{}{}
		}
		if !valid {
			failures = append(failures, candidate.Name()+": stock-universe contains invalid or duplicate securities")
			continue
		}
		return snapshot, nil
	}
	if len(failures) == 0 {
		return nil, errors.New("no configured provider exposes a complete stock universe")
	}
	return nil, fmt.Errorf("all stock-universe providers failed: %v", failures)
}

// MarketStocks returns one bounded page of the complete A-share universe with quote and optional MA enrichment.
func (s *DataService) MarketStocks(ctx context.Context, page, pageSize int, includeMA bool, basis model.AdjustmentBasis) (*model.MarketStocksSnapshot, error) {
	if ctx == nil { ctx = context.Background() }
	if page < 1 || page > 10000 {
		return nil, errors.New("market-stocks page must be between 1 and 10000")
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, errors.New("market-stocks page_size must be between 1 and 100")
	}
	if basis != model.AdjustmentNone && basis != model.AdjustmentForward && basis != model.AdjustmentBackward {
		return nil, errors.New("market-stocks adjustment basis must be none, forward_adjusted or backward_adjusted")
	}
	universe, err := s.StockUniverse(ctx)
	if err != nil {
		return nil, err
	}
	start := (page - 1) * pageSize
	if start > universe.Total {
		start = universe.Total
	}
	end := start + pageSize
	if end > universe.Total {
		end = universe.Total
	}
	selected := universe.Items[start:end]
	items := make([]model.MarketStockOverviewItem, len(selected))
	sem := make(chan struct{}, 5)
	var wg sync.WaitGroup
	for i, security := range selected {
		wg.Add(1)
		go func(index int, security model.StockSearchItem) {
			defer wg.Done()
			item := model.MarketStockOverviewItem{
				Symbol: security.Symbol, Code: security.Code, Name: security.Name, Exchange: security.Exchange,
				MAAdjustmentBasis: basis, QualityStatus: model.QualityUnknown,
			}
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				item.ErrorCode = "REQUEST_CANCELLED"
				items[index] = item
				return
			}
			defer func() { <-sem }()
			quote, _, quoteErr := s.Quote(ctx, security.Symbol)
			if quoteErr != nil || quote == nil {
				item.ErrorCode = "QUOTE_UNAVAILABLE"
				item.QualityReasons = appendReason(item.QualityReasons, "market_overview_quote_unavailable")
				items[index] = item
				return
			}
			item.Quote = quote
			item.QualityStatus = quote.QualityStatus
			for _, reason := range quote.QualityReasons {
				item.QualityReasons = appendReason(item.QualityReasons, reason)
			}
			if includeMA {
				bars, _, warnings, barsErr := s.DailyBars(ctx, security.Symbol, "", "", 20, basis)
				if barsErr != nil {
					item.QualityStatus = model.QualityDegraded
					item.QualityReasons = appendReason(item.QualityReasons, "market_overview_daily_bars_unavailable")
				} else {
					sort.Slice(bars, func(i, j int) bool { return bars[i].TradeDate < bars[j].TradeDate })
					bars = CalculateMovingAverages(bars)
					if len(bars) > 0 {
						latest := bars[len(bars)-1]
						item.MA5, item.MA20, item.MAProvider = latest.MA5, latest.MA20, latest.Source
						if latest.QualityStatus != model.QualityValid || len(warnings) > 0 {
							item.QualityStatus = model.QualityDegraded
							item.QualityReasons = appendReason(item.QualityReasons, "market_overview_daily_bars_quality_degraded")
						}
					}
					if item.MA5 == nil {
						item.QualityStatus = model.QualityDegraded
						item.QualityReasons = appendReason(item.QualityReasons, "ma5_unavailable_or_insufficient_history")
					}
					if item.MA20 == nil {
						item.QualityStatus = model.QualityDegraded
						item.QualityReasons = appendReason(item.QualityReasons, "ma20_unavailable_or_insufficient_history")
					}
				}
			}
			items[index] = item
		}(i, security)
	}
	wg.Wait()
	result := &model.MarketStocksSnapshot{
		Page: page, PageSize: pageSize, Total: universe.Total, TotalIsLowerBound: universe.TotalIsLowerBound,
		Returned: len(items), IncludeMA: includeMA, MAAdjustmentBasis: basis,
		UniverseSource: universe.Source, UniverseSourceVersion: universe.SourceVersion,
		UniverseFetchedAt: universe.FetchedAt, FetchedAt: time.Now().UTC(), Items: items,
		QualityStatus: model.QualityValid, QualityReasons: append([]string(nil), universe.QualityReasons...),
	}
	if universe.QualityStatus != model.QualityValid || universe.TotalIsLowerBound {
		result.QualityStatus = model.QualityDegraded
		result.QualityReasons = appendReason(result.QualityReasons, "stock_universe_quality_degraded_or_lower_bound")
	}
	for _, item := range items {
		if item.Quote == nil {
			result.QuoteFailed++
		} else {
			result.QuoteSucceeded++
		}
		if item.QualityStatus != model.QualityValid {
			result.QualityStatus = model.QualityDegraded
		}
		for _, reason := range item.QualityReasons {
			result.QualityReasons = appendReason(result.QualityReasons, reason)
		}
	}
	if result.Returned > 0 {
		result.CoverageRatio = float64(result.QuoteSucceeded) / float64(result.Returned)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return result, nil
}
