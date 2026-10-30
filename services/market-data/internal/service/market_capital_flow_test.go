package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type marketCapitalFlowFake struct {
	name     string
	snapshot *model.MarketCapitalFlowSnapshot
	err      error
}

func (p *marketCapitalFlowFake) Name() string {
	if p.name != "" {
		return p.name
	}
	return "tushare"
}

func (p *marketCapitalFlowFake) FetchMarketCapitalFlow(_ context.Context, startDate, endDate string) (*model.MarketCapitalFlowSnapshot, error) {
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, nil
	}
	copy := *p.snapshot
	copy.StartDate = startDate
	copy.EndDate = endDate
	return &copy, nil
}

func TestMarketCapitalFlowPreservesProviderSeriesAndCache(t *testing.T) {
	netAmount := 123.0
	now := time.Now().UTC()
	fake := &marketCapitalFlowFake{snapshot: &model.MarketCapitalFlowSnapshot{
		Total: 1, Items: []model.MarketCapitalFlowItem{{TradeDate: "2026-10-29", NetAmountCNY: &netAmount}},
		Source: "tushare", SourceVersion: "tushare-moneyflow-mkt-dc-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
	}}
	svc := NewWithCapabilities(nil, nil, nil, nil, nil, time.Minute, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	svc.SetMarketCapitalFlowProviders([]provider.MarketCapitalFlowProvider{fake})
	got, cacheHit, err := svc.MarketCapitalFlow(context.Background(), "2026-10-29", "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if cacheHit || got.Total != 1 || got.Items[0].NetAmountCNY == nil || *got.Items[0].NetAmountCNY != 123 {
		t.Fatalf("unexpected market capital-flow result: hit=%v snapshot=%+v", cacheHit, got)
	}
}

type marketCapitalFlowWrongSourceFake struct {
	marketCapitalFlowFake
}

func (p *marketCapitalFlowWrongSourceFake) Name() string { return "configured-provider" }

func TestMarketCapitalFlowRejectsMismatchedProviderIdentity(t *testing.T) {
	netAmount := 1.0
	fake := &marketCapitalFlowWrongSourceFake{marketCapitalFlowFake{snapshot: &model.MarketCapitalFlowSnapshot{
		Total: 1, Items: []model.MarketCapitalFlowItem{{TradeDate: "2026-10-29", NetAmountCNY: &netAmount}},
		Source: "unrelated-provider", SourceVersion: "unrelated-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
	}}}
	svc := NewWithCapabilities(nil, nil, nil, nil, nil, time.Minute, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	svc.SetMarketCapitalFlowProviders([]provider.MarketCapitalFlowProvider{fake})
	if _, _, err := svc.MarketCapitalFlow(context.Background(), "2026-10-29", "2026-10-29"); err == nil {
		t.Fatal("provider response source must match the configured provider identity")
	}
}

type marketCapitalFlowMemoryCache struct {
	items map[string][]byte
}

func (c *marketCapitalFlowMemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	value, ok := c.items[key]
	return append([]byte(nil), value...), ok, nil
}

func (c *marketCapitalFlowMemoryCache) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	if c.items == nil {
		c.items = make(map[string][]byte)
	}
	c.items[key] = append([]byte(nil), value...)
	return nil
}

func (c *marketCapitalFlowMemoryCache) Ping(context.Context) error { return nil }

func TestMarketCapitalFlowCacheIsScopedToConfiguredProviderChain(t *testing.T) {
	newFake := func(name string, amount float64) *marketCapitalFlowFake {
		return &marketCapitalFlowFake{
			name: name,
			snapshot: &model.MarketCapitalFlowSnapshot{
				Total: 1, Items: []model.MarketCapitalFlowItem{{TradeDate: "2026-10-29", NetAmountCNY: &amount}},
				Source: name, SourceVersion: name + "-market-flow-v1",
				FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
			},
		}
	}
	memory := &marketCapitalFlowMemoryCache{}
	first := newFake("tushare", 100)
	firstService := NewWithCapabilities(nil, nil, nil, memory, nil, time.Minute, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	firstService.SetMarketCapitalFlowProviders([]provider.MarketCapitalFlowProvider{first})
	if _, hit, err := firstService.MarketCapitalFlow(context.Background(), "2026-10-29", "2026-10-29"); err != nil || hit {
		t.Fatalf("initial provider request: hit=%v err=%v", hit, err)
	}

	second := newFake("eastmoney", 200)
	secondService := NewWithCapabilities(nil, nil, nil, memory, nil, time.Minute, time.Minute, 120, 2000, ResilienceConfig{}, nil)
	secondService.SetMarketCapitalFlowProviders([]provider.MarketCapitalFlowProvider{second})
	got, hit, err := secondService.MarketCapitalFlow(context.Background(), "2026-10-29", "2026-10-29")
	if err != nil {
		t.Fatal(err)
	}
	if hit || got.Source != "eastmoney" || got.Items[0].NetAmountCNY == nil || *got.Items[0].NetAmountCNY != 200 {
		t.Fatalf("provider chain change must not reuse stale cached source: hit=%v snapshot=%+v", hit, got)
	}
}

func TestMarketCapitalFlowRejectsUnorderedOrOutOfRangeRows(t *testing.T) {
	now := time.Now().UTC()
	snapshot := &model.MarketCapitalFlowSnapshot{
		StartDate: "2026-10-28", EndDate: "2026-10-29", Total: 1,
		Items:  []model.MarketCapitalFlowItem{{TradeDate: "2026-10-30"}},
		Source: "tushare", SourceVersion: "tushare-moneyflow-mkt-dc-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
	if err := validateMarketCapitalFlowSnapshot("2026-10-28", "2026-10-29", snapshot); err == nil {
		t.Fatal("out-of-range trade date must be rejected")
	}
}
