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

type hotRankingFallbackTestProvider struct {
	name string
	snapshot *model.HotRankingSnapshot
	err error
	calls int
}

func (p *hotRankingFallbackTestProvider) Name() string { return p.name }

func (p *hotRankingFallbackTestProvider) FetchHotStocks(_ context.Context, rankingType string, _ int) (*model.HotRankingSnapshot, error) {
	p.calls++
	if p.err != nil {
		return nil, p.err
	}
	if p.snapshot == nil {
		return nil, errors.New("test hot-ranking snapshot is nil")
	}
	out := *p.snapshot
	out.RankingType = rankingType
	out.Items = append([]model.HotRankingItem(nil), p.snapshot.Items...)
	return &out, nil
}

func emptyHotRankingForFallbackTest(source string) *model.HotRankingSnapshot {
	return &model.HotRankingSnapshot{
		RankingType: "popular", Total: 0, Items: []model.HotRankingItem{},
		Source: source, SourceVersion: "test-v1", FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityUnknown, QualityReasons: []string{"provider_ranking_not_available"},
	}
}

func populatedHotRankingForFallbackTest(source string) *model.HotRankingSnapshot {
	now := time.Now().UTC()
	return &model.HotRankingSnapshot{
		RankingType: "popular", Total: 1,
		Items: []model.HotRankingItem{{
			Rank: 1, Symbol: "600519.SH", Code: "600519", Name: "贵州茅台",
			Source: source, SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
		}},
		Source: source, SourceVersion: "test-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: []string{"provider_snapshot_timestamp_not_exposed"},
	}
}

func hotRankingFallbackService(cacheStore cache.Cache, providers ...provider.HotRankingProvider) *DataService {
	svc := NewWithCapabilities(nil, nil, nil, cacheStore, nil, time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
	svc.SetHotRankingProviders(providers)
	return svc
}

func TestHotRankingFallsBackAfterUnknownEmptyResult(t *testing.T) {
	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: emptyHotRankingForFallbackTest("eastmoney")}
	second := &hotRankingFallbackTestProvider{name: "hithink", snapshot: populatedHotRankingForFallbackTest("hithink")}
	svc := hotRankingFallbackService(cache.NewDisabled(), first, second)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || len(got.Items) != 1 || got.Source != "hithink" {
		t.Fatalf("expected fallback popularity ranking, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected both providers to be called once, eastmoney=%d hithink=%d", first.calls, second.calls)
	}
}

func TestHotRankingCachesUnknownOnlyAfterFallbackExhaustion(t *testing.T) {
	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: emptyHotRankingForFallbackTest("eastmoney")}
	second := &hotRankingFallbackTestProvider{name: "hithink", err: errors.New("test ranking endpoint unavailable")}
	svc := hotRankingFallbackService(newMemoryCache(), first, second)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || len(got.Items) != 0 || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected an unknown empty ranking after fallback exhaustion, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "hot_ranking_fallback_exhausted_without_confirmed_data" {
			found = true
		}
	}
	if !found || first.calls != 1 || second.calls != 1 {
		t.Fatalf("missing fallback-exhaustion marker or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls)
	}
	cached, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || !hit {
		t.Fatalf("exhausted unknown result should use its marked short cache: hit=%v err=%v", hit, err)
	}
	if first.calls != 1 || second.calls != 1 || cached == nil || cached.QualityStatus != model.QualityUnknown {
		t.Fatalf("unexpected cached unknown result: %+v calls=%d/%d", cached, first.calls, second.calls)
	}
}
