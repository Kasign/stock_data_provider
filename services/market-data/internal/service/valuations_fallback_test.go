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

type valuationFallbackTestProvider struct {
	name string
	snapshot *model.ValuationSnapshot
	err error
	calls int
}

func (p *valuationFallbackTestProvider) Name() string { return p.name }

func (p *valuationFallbackTestProvider) FetchValuations(context.Context, []string) (*model.ValuationSnapshot, error) {
	p.calls++
	if p.err != nil { return nil, p.err }
	if p.snapshot == nil { return nil, errors.New("test valuation snapshot is nil") }
	out := *p.snapshot
	out.RequestedSymbols = append([]string(nil), p.snapshot.RequestedSymbols...)
	out.Items = append([]model.ValuationItem(nil), p.snapshot.Items...)
	out.MissingSymbols = append([]string(nil), p.snapshot.MissingSymbols...)
	return &out, nil
}

func unknownValuationsForFallbackTest(source string) *model.ValuationSnapshot {
	return &model.ValuationSnapshot{
		RequestedSymbols: []string{"600519.SH"}, Total: 0, Items: []model.ValuationItem{},
		MissingSymbols: []string{"600519.SH"},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_valuation_not_available"},
	}
}

func validValuationsForFallbackTest(source string) *model.ValuationSnapshot {
	now := time.Now().UTC()
	value := 25.0
	return &model.ValuationSnapshot{
		RequestedSymbols: []string{"600519.SH"}, Total: 1,
		Items: []model.ValuationItem{{
			Symbol: "600519.SH", Code: "600519", Name: "贵州茅台",
			PETTM: &value, PEMRQ: &value, PEStatic: &value, PBMRQ: &value, PSTTM: &value, PCFTTM: &value,
			Source: source, SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityDegraded,
			QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
}

func valuationFallbackService(providers ...provider.ValuationProvider) *DataService {
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetValuationProviders(providers)
	return svc
}

func TestValuationsFallsBackAfterUnknownProviderSnapshot(t *testing.T) {
	first := &valuationFallbackTestProvider{name: "eastmoney", snapshot: unknownValuationsForFallbackTest("eastmoney")}
	second := &valuationFallbackTestProvider{name: "tushare", snapshot: validValuationsForFallbackTest("tushare")}
	got, hit, err := valuationFallbackService(first, second).Valuations(context.Background(), []string{"600519.SH"})
	if err != nil || hit { t.Fatalf("Valuations returned hit=%v err=%v", hit, err) }
	if got == nil || got.QualityStatus != model.QualityDegraded || got.Source != "tushare" || first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected fallback valuation data, snapshot=%+v calls=%d/%d", got, first.calls, second.calls)
	}
}

func TestValuationsReturnsUnknownOnlyAfterFallbackExhaustion(t *testing.T) {
	first := &valuationFallbackTestProvider{name: "eastmoney", snapshot: unknownValuationsForFallbackTest("eastmoney")}
	second := &valuationFallbackTestProvider{name: "tushare", err: errors.New("test provider unavailable")}
	mem := newMemoryCache()
	svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetValuationProviders([]provider.ValuationProvider{first, second})
	got, hit, err := svc.Valuations(context.Background(), []string{"600519.SH"})
	if err != nil || hit { t.Fatalf("Valuations returned hit=%v err=%v", hit, err) }
	if got == nil || got.QualityStatus != model.QualityUnknown { t.Fatalf("expected unknown valuation after fallback exhaustion, got %+v", got) }
	found := false
	for _, reason := range got.QualityReasons { if reason == "valuations_fallback_exhausted_without_confirmed_data" { found = true } }
	if !found || first.calls != 1 || second.calls != 1 { t.Fatalf("missing fallback exhaustion marker or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls) }
}
