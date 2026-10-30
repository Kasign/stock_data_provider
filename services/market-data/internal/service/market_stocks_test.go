package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type marketStocksFake struct {
	universe *model.StockUniverseSnapshot
}

func (p marketStocksFake) Name() string { return "eastmoney" }
func (p marketStocksFake) ListStockUniverse(context.Context) (*model.StockUniverseSnapshot, error) {
	return p.universe, nil
}
func (p marketStocksFake) SearchStocks(_ context.Context, query string, limit int) (*model.StockSearchSnapshot, error) {
	return &model.StockSearchSnapshot{Query: query, Total: len(p.universe.Items), Items: p.universe.Items, Source: "eastmoney", SourceVersion: "test", FetchedAt: p.universe.FetchedAt, QualityStatus: model.QualityValid}, nil
}
func (p marketStocksFake) FetchQuote(_ context.Context, symbol string) (*model.Quote, error) {
	now := time.Now().UTC()
	price, change, pct, open, high, low, prev, amount := 10.0, 1.0, 10.0, 9.0, 11.0, 8.0, 9.0, 100000.0
	volume := int64(10000)
	return &model.Quote{
		Symbol: symbol, Name: "测试", CurrentPrice: &price, Change: &change, ChangePercent: &pct,
		Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &volume, Amount: &amount,
		QuoteTime: &now, QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: now, NormalizedAt: now,
		Source: "eastmoney", SourceVersion: "test-quote-v1", VolumeUnit: "shares", AmountUnit: "CNY",
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version,
	}, nil
}

func TestMarketStocksPaginatesCompleteUniverse(t *testing.T) {
	now := time.Now().UTC()
	universe := &model.StockUniverseSnapshot{
		Items: []model.StockSearchItem{
			{Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", Exchange: "SH"},
			{Symbol: "000001.SZ", Code: "000001", Name: "平安银行", Exchange: "SZ"},
		},
		Total: 2, Source: "eastmoney", SourceVersion: "test-universe-v1", FetchedAt: now, QualityStatus: model.QualityValid,
	}
	p := marketStocksFake{universe: universe}
	ds := NewWithCapabilities([]provider.NamedProvider{p}, []provider.QuoteProvider{p}, nil,
		cache.NewDisabled(), nil, time.Second, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	ds.SetStockSearchProviders([]provider.StockSearchProvider{p})
	result, err := ds.MarketStocks(context.Background(), 2, 1, false, model.AdjustmentNone)
	if err != nil {
		t.Fatalf("MarketStocks() error = %v", err)
	}
	if result.Total != 2 || result.Returned != 1 || result.Items[0].Symbol != "000001.SZ" {
		t.Fatalf("unexpected second page: %+v", result)
	}
	if result.QuoteSucceeded != 1 || result.QuoteFailed != 0 || result.CoverageRatio != 1 {
		t.Fatalf("unexpected page coverage: %+v", result)
	}
}
