package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type dragonTigerFallbackTestProvider struct {
	name     string
	snapshot *model.DragonTigerSnapshot
	err      error
	calls    int
}

func (p *dragonTigerFallbackTestProvider) Name() string { return p.name }

func (p *dragonTigerFallbackTestProvider) FetchDragonTiger(_ context.Context, tradeDate string, page, pageSize int) (*model.DragonTigerSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test dragon-tiger snapshot is nil")
	}
	out := *p.snapshot
	out.TradeDate = tradeDate
	out.ProviderReportDate = tradeDate
	out.Page = page
	out.PageSize = pageSize
	out.Items = append([]model.DragonTigerItem(nil), p.snapshot.Items...)
	for index := range out.Items {
		out.Items[index].TradeDate = tradeDate
	}
	return &out, nil
}

func unknownDragonTigerForFallbackTest(source string) *model.DragonTigerSnapshot {
	return &model.DragonTigerSnapshot{
		TradeDate: "2026-10-26", ProviderReportDate: "2026-10-26",
		Page: 1, PageSize: 100, Total: 0, Pages: 0, Returned: 0, SourceReturned: 0,
		ExcludedNonAShareCount: 0, HasData: false, HasMore: false, Items: []model.DragonTigerItem{},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_report_not_confirmed"},
	}
}

func dataDragonTigerForFallbackTest(source string) *model.DragonTigerSnapshot {
	now := time.Now().UTC()
	change, closePrice, netBuy := 1.5, 10.0, -100.0
	return &model.DragonTigerSnapshot{
		TradeDate: "2026-10-26", ProviderReportDate: "2026-10-26",
		Page: 1, PageSize: 100, Total: 1, Pages: 1, Returned: 1, SourceReturned: 1,
		ExcludedNonAShareCount: 0, HasData: true, HasMore: false,
		Items: []model.DragonTigerItem{{
			Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", TradeDate: "2026-10-26",
			Reason: "test", ChangePercent: &change, ClosePrice: &closePrice, NetBuyAmountCNY: &netBuy,
			Source: source, SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
}

func dragonTigerFallbackService(providers ...provider.DragonTigerProvider) *DataService {
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetDragonTigerProviders(providers)
	return svc
}

func TestDragonTigerFallsBackAfterUnknownEmptyReport(t *testing.T) {
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	empty := &dragonTigerFallbackTestProvider{name: "eastmoney", snapshot: unknownDragonTigerForFallbackTest("eastmoney")}
	fallback := &dragonTigerFallbackTestProvider{name: "hithink", snapshot: dataDragonTigerForFallbackTest("hithink")}
	svc := dragonTigerFallbackService(empty, fallback)

	got, hit, err := svc.DragonTiger(context.Background(), date, 1, 100)
	if err != nil || hit {
		t.Fatalf("DragonTiger returned hit=%v err=%v", hit, err)
	}
	if got == nil || !got.HasData || got.Source != "hithink" || len(got.Items) != 1 {
		t.Fatalf("expected fallback provider A-share data, got %+v", got)
	}
	if empty.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected both providers to be tried once, eastmoney=%d hithink=%d", empty.calls, fallback.calls)
	}
}

func TestDragonTigerCachesUnknownOnlyAfterFallbackExhaustion(t *testing.T) {
	date := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	empty := &dragonTigerFallbackTestProvider{name: "eastmoney", snapshot: unknownDragonTigerForFallbackTest("eastmoney")}
	failed := &dragonTigerFallbackTestProvider{name: "hithink", err: errors.New("test endpoint unavailable")}
	mem := newMemoryCache()
	svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetDragonTigerProviders([]provider.DragonTigerProvider{empty, failed})

	got, hit, err := svc.DragonTiger(context.Background(), date, 1, 100)
	if err != nil || hit {
		t.Fatalf("DragonTiger returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.HasData || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown empty page after fallback exhaustion, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "dragon_tiger_fallback_exhausted_without_confirmed_data" {
			found = true
		}
	}
	if !found || empty.calls != 1 || failed.calls != 1 {
		t.Fatalf("missing exhaustion marker or incomplete fallback: reasons=%v emptyCalls=%d failedCalls=%d", got.QualityReasons, empty.calls, failed.calls)
	}
}
