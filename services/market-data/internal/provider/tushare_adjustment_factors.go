package provider

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// FetchAdjustmentFactors 暴露 Tushare 原始 adj_factor，不在 Provider 层将因子折算成价格。
func (p *Tushare) FetchAdjustmentFactors(ctx context.Context, rawSymbol, startDate, endDate string) ([]model.AdjustmentFactor, error) {
	if p == nil || p.client == nil {
		return nil, fmt.Errorf("Tushare HTTP client is not configured")
	}
	if p.token == "" {
		return nil, fmt.Errorf("Tushare token is not configured")
	}
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tushare adjustment factors only support common A-share equities, got %q", rawSymbol)
	}
	startCompact, err := tushareCompactDate(startDate)
	if err != nil {
		return nil, fmt.Errorf("Tushare adjustment-factor start date: %w", err)
	}
	endCompact, err := tushareCompactDate(endDate)
	if err != nil {
		return nil, fmt.Errorf("Tushare adjustment-factor end date: %w", err)
	}
	startISO, err := tushareISODate(startCompact)
	if err != nil {
		return nil, err
	}
	endISO, err := tushareISODate(endCompact)
	if err != nil {
		return nil, err
	}
	if startISO > endISO {
		return nil, fmt.Errorf("adjustment-factor start date must not be after end date")
	}

	values, err := p.fetchAdjustmentFactors(ctx, symbol, startISO, endISO)
	if err != nil {
		return nil, err
	}
	fetchedAt := time.Now().UTC()
	out := make([]model.AdjustmentFactor, 0, len(values))
	for tradeDate, factor := range values {
		out = append(out, model.AdjustmentFactor{
			Symbol: symbol, TradeDate: tradeDate, Factor: factor,
			Source: p.Name(), SourceVersion: tushareVersion, FetchedAt: fetchedAt,
			QualityStatus:  model.QualityDegraded,
			QualityReasons: []string{"provider_available_at_not_exposed_by_adj_factor"},
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TradeDate < out[j].TradeDate })
	return out, nil
}
