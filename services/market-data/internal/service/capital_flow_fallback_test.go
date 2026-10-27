package service

import (
	"context"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type capitalFlowFallbackTestProvider struct {
	name string
	item *model.CapitalFlow
	calls int
}

func (p *capitalFlowFallbackTestProvider) Name() string { return p.name }

func (p *capitalFlowFallbackTestProvider) FetchCapitalFlow(context.Context, string) (*model.CapitalFlow, error) {
	p.calls++
	if p.item == nil {
		return nil, nil
	}
	out := *p.item
	return &out, nil
}

func capitalFlowServiceForFallbackTest(providers ...provider.CapitalFlowProvider) *DataService {
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetCapitalFlowProviders(providers)
	return svc
}

func TestCapitalFlowFallsBackWhenFirstProviderHasNoUsableMetrics(t *testing.T) {
	now := time.Now().UTC()
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	empty := &capitalFlowFallbackTestProvider{
		name: "eastmoney",
		item: &model.CapitalFlow{
			Symbol: "600519.SH", ProviderDate: today, Observations: 10,
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_amount_metrics_missing"},
		},
	}
	main := 120.0
	five := 500.0
	fallback := &capitalFlowFallbackTestProvider{
		name: "miaoxiang",
		item: &model.CapitalFlow{
			Symbol: "600519.SH", ProviderDate: today, MainNetInflow: &main, Inflow5D: &five,
			Observations: 5, Source: "miaoxiang", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		},
	}
	svc := capitalFlowServiceForFallbackTest(empty, fallback)

	got, cacheHit, err := svc.CapitalFlow(context.Background(), "600519.SH")
	if err != nil || cacheHit {
		t.Fatalf("CapitalFlow returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got == nil || got.Source != "miaoxiang" || got.MainNetInflow == nil || *got.MainNetInflow != main {
		t.Fatalf("expected usable fallback capital-flow metrics, got %+v", got)
	}
	if empty.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected both Providers to be tried, eastmoney=%d miaoxiang=%d", empty.calls, fallback.calls)
	}
}

func TestValidateCapitalFlowRejectsSnapshotWithoutAnyUsableMetric(t *testing.T) {
	item := &model.CapitalFlow{
		Symbol: "600519.SH", ProviderDate: time.Now().In(marketLocation()).Format("2006-01-02"),
		Observations: 10, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_amount_metrics_missing"},
	}
	if err := validateCapitalFlowSnapshot("600519.SH", item); err == nil {
		t.Fatal("capital-flow snapshot without any usable amount metric must be rejected")
	}
}
