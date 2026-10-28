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
