package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
)

// StockUniverse 使用已配置的证券目录来源获取完整 A 股证券池。
//
// 候选来源分两组：显式注册的 StockUniverseProvider（如 BaoStock 全量目录）优先，
// 其次是实现了该能力的 StockSearchProvider（Eastmoney 的 clist 目录）。两组按名称去重，
// 顺序即兜底顺序；只要有一条通过完整性校验就返回，全部失败才报错。
func (s *DataService) StockUniverse(ctx context.Context) (*model.StockUniverseSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var failures []string
	for _, candidate := range s.stockUniverseCandidates() {
		snapshot, err := candidate.ListStockUniverse(ctx)
		if err != nil {
			failures = append(failures, candidate.Name()+": "+err.Error())
			continue
		}
		if err := validateStockUniverseSnapshot(snapshot); err != nil {
			failures = append(failures, candidate.Name()+": "+err.Error())
			continue
		}
		return snapshot, nil
	}
	if len(failures) == 0 {
		return nil, errors.New("no configured provider exposes a complete stock universe")
	}
	return nil, fmt.Errorf("all stock-universe providers failed: %v", failures)
}

// stockUniverseCandidates 合并显式目录来源与搜索来源中的目录实现，按名称去重并保持优先级。
func (s *DataService) stockUniverseCandidates() []provider.StockUniverseProvider {
	candidates := make([]provider.StockUniverseProvider, 0, len(s.stockUniverseProviders)+len(s.stockSearchProviders))
	seen := make(map[string]struct{}, cap(candidates))
	for _, candidate := range s.stockUniverseProviders {
		if candidate == nil {
			continue
		}
		if _, duplicate := seen[candidate.Name()]; duplicate {
			continue
		}
		seen[candidate.Name()] = struct{}{}
		candidates = append(candidates, candidate)
	}
	for _, searchProvider := range s.stockSearchProviders {
		universeProvider, ok := searchProvider.(provider.StockUniverseProvider)
		if !ok {
			continue
		}
		if _, duplicate := seen[universeProvider.Name()]; duplicate {
			continue
		}
		seen[universeProvider.Name()] = struct{}{}
		candidates = append(candidates, universeProvider)
	}
	return candidates
}

// validateStockUniverseSnapshot 校验目录的完整性与身份一致性。
// Total 必须等于条目数，条目必须是规范代码、非空名称且不重复，否则不得作为证券池使用。
func validateStockUniverseSnapshot(snapshot *model.StockUniverseSnapshot) error {
	if snapshot == nil || snapshot.Total <= 0 || snapshot.Total != len(snapshot.Items) ||
		snapshot.Source == "" || snapshot.SourceVersion == "" || snapshot.FetchedAt.IsZero() ||
		(snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded && snapshot.QualityStatus != model.QualityStale) {
		return errors.New("invalid or incomplete stock-universe snapshot")
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		symbol, symbolErr := normalize.NormalizeSymbol(item.Symbol)
		if symbolErr != nil || symbol != item.Symbol || item.Code == "" || item.Name == "" {
			return errors.New("stock-universe contains invalid or duplicate securities")
		}
		if _, exists := seen[symbol]; exists {
			return errors.New("stock-universe contains invalid or duplicate securities")
		}
		seen[symbol] = struct{}{}
	}
	return nil
}

// MarketStocks returns one bounded page of the complete A-share universe with quote and optional MA enrichment.
func (s *DataService) MarketStocks(ctx context.Context, page, pageSize int, includeMA bool, basis model.AdjustmentBasis) (*model.MarketStocksSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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
		UniverseFetchedAt: universe.FetchedAt, FetchedAt: s.nowUTC(), Items: items,
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
