package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type marketCapitalFlowFake struct {
	snapshot *model.MarketCapitalFlowSnapshot
	err error
}

func (p *marketCapitalFlowFake) Name() string { return "tushare" }

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
