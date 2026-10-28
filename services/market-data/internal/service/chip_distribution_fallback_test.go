package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

type chipFallbackTestProvider struct {
	name  string
	item  *model.ChipDistribution
	err   error
	calls int
}

func (p *chipFallbackTestProvider) Name() string { return p.name }

func (p *chipFallbackTestProvider) FetchChipDistribution(_ context.Context, symbol string) (*model.ChipDistribution, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.item == nil {
		return nil, errors.New("test chip snapshot is nil")
	}
	out := *p.item
	out.Symbol = symbol
	return &out, nil
}

func chipFallbackService(providers ...provider.ChipDistributionProvider) *DataService {
	svc := NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetChipDistributionProviders(providers)
	return svc
}

func chipFallbackItem(source string, quality model.QualityStatus) *model.ChipDistribution {
	return &model.ChipDistribution{
		Symbol: "600519.SH", AverageCost: 1500, Date: "2026-10-27",
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: quality, QualityReasons: []string{"test_quality"},
	}
}

func TestChipDistributionFallsBackFromUnknownQualitySnapshot(t *testing.T) {
	unknown := &chipFallbackTestProvider{name: "eastmoney", item: chipFallbackItem("eastmoney", model.QualityUnknown)}
	fallback := &chipFallbackTestProvider{name: "miaoxiang", item: chipFallbackItem("miaoxiang", model.QualityDegraded)}
	svc := chipFallbackService(unknown, fallback)

	got, hit, err := svc.ChipDistribution(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("ChipDistribution returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "miaoxiang" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected degraded fallback snapshot, got %+v", got)
	}
	if unknown.calls != 1 || fallback.calls != 1 {
		t.Fatalf("expected both chip sources to be tried, first=%d fallback=%d", unknown.calls, fallback.calls)
	}
}

func TestChipDistributionKeepsUnknownCandidateWithMoreMetrics(t *testing.T) {
	partial := chipFallbackItem("eastmoney", model.QualityUnknown)
	full := chipFallbackItem("miaoxiang", model.QualityUnknown)
	cost90Low, cost90High, cost70Low, cost70High := 1300.0, 1700.0, 1400.0, 1600.0
	profit, concentration90, concentration70 := 0.82, 0.15, 0.11
	full.Cost90Low, full.Cost90High = &cost90Low, &cost90High
	full.Cost70Low, full.Cost70High = &cost70Low, &cost70High
	full.ProfitRatio, full.Concentration90, full.Concentration70 = &profit, &concentration90, &concentration70
	first := &chipFallbackTestProvider{name: "eastmoney", item: partial}
	second := &chipFallbackTestProvider{name: "miaoxiang", item: full}
	failed := &chipFallbackTestProvider{name: "third-source", err: errors.New("test source unavailable")}
	svc := chipFallbackService(first, second, failed)

	got, hit, err := svc.ChipDistribution(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("ChipDistribution returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "miaoxiang" || got.Cost90Low == nil || got.ProfitRatio == nil ||
		got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown candidate with more chip metrics, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 || failed.calls != 1 {
		t.Fatalf("expected all Providers to be tried, calls=%d/%d/%d", first.calls, second.calls, failed.calls)
	}
}

func TestChipDistributionUnknownQualityIsReturnedOnlyAfterFallbackExhaustion(t *testing.T) {
	unknown := &chipFallbackTestProvider{name: "eastmoney", item: chipFallbackItem("eastmoney", model.QualityUnknown)}
	failed := &chipFallbackTestProvider{name: "miaoxiang", err: errors.New("test source unavailable")}
	svc := chipFallbackService(unknown, failed)

	got, hit, err := svc.ChipDistribution(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("ChipDistribution returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected unknown snapshot after fallback exhaustion, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "chip_distribution_fallback_exhausted_without_usable_quality" {
			found = true
		}
	}
	if !found || unknown.calls != 1 || failed.calls != 1 {
		t.Fatalf("missing fallback marker or source was not attempted: reasons=%v unknown=%d failed=%d", got.QualityReasons, unknown.calls, failed.calls)
	}
}

func TestChipDistributionDeduplicatesProviderNamesIgnoringCaseAndWhitespace(t *testing.T) {
	first := &chipFallbackTestProvider{name: "eastmoney", item: chipFallbackItem("eastmoney", model.QualityUnknown)}
	duplicate := &chipFallbackTestProvider{name: " EASTMONEY ", err: errors.New("duplicate provider should not be called")}
	fallback := &chipFallbackTestProvider{name: "miaoxiang", item: chipFallbackItem("miaoxiang", model.QualityDegraded)}
	svc := chipFallbackService(nil, first, duplicate, fallback)
	svc.SetSupplementProviders([]provider.SupplementProvider{nil})

	got, hit, err := svc.ChipDistribution(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("ChipDistribution returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "miaoxiang" || first.calls != 1 || duplicate.calls != 0 || fallback.calls != 1 {
		t.Fatalf("expected case-insensitive duplicate provider to be skipped: got=%+v calls=%d/%d/%d", got, first.calls, duplicate.calls, fallback.calls)
	}
}

func TestChipDistributionLegacyUnknownCacheDoesNotBlockFallback(t *testing.T) {
	legacy := chipFallbackItem("eastmoney", model.QualityUnknown)
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemoryCache()
	mem.values["md:v1:chip:600519.SH"] = encoded
	first := &chipFallbackTestProvider{name: "eastmoney", item: legacy}
	fallback := &chipFallbackTestProvider{name: "miaoxiang", item: chipFallbackItem("miaoxiang", model.QualityDegraded)}
	svc := NewWithCapabilities(nil, nil, nil, mem, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetChipDistributionProviders([]provider.ChipDistributionProvider{first, fallback})

	got, hit, err := svc.ChipDistribution(context.Background(), "600519.SH")
	if err != nil || hit {
		t.Fatalf("legacy unknown cache must not short-circuit fallback: hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "miaoxiang" || fallback.calls != 1 {
		t.Fatalf("expected fallback provider after bypassing unknown cache, got %+v calls=%d", got, fallback.calls)
	}
}
