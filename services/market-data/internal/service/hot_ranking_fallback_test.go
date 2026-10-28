package service

import (
	"context"
	"encoding/json"
	"errors"
	"math"
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

func TestHotRankingFallsBackFromUnorderedRanks(t *testing.T) {
	invalid := populatedHotRankingForFallbackTest("eastmoney")
	first := invalid.Items[0]
	first.Rank = 2
	second := first
	second.Rank = 1
	second.Symbol = "000001.SZ"
	second.Code = "000001"
	second.Name = "平安银行"
	invalid.Items = []model.HotRankingItem{first, second}
	invalid.Total = len(invalid.Items)
	primary := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: invalid}
	fallback := &hotRankingFallbackTestProvider{name: "hithink", snapshot: populatedHotRankingForFallbackTest("hithink")}
	svc := hotRankingFallbackService(cache.NewDisabled(), primary, fallback)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "hithink" || len(got.Items) != 1 {
		t.Fatalf("unordered ranking items must reject the first provider and fall back, got %+v", got)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("unordered ranking must trigger fallback, calls=%d/%d", primary.calls, fallback.calls)
	}
}

func TestHotRankingFallsBackFromDuplicateRank(t *testing.T) {
	invalid := populatedHotRankingForFallbackTest("eastmoney")
	duplicate := invalid.Items[0]
	duplicate.Symbol = "000001.SZ"
	duplicate.Code = "000001"
	duplicate.Name = "平安银行"
	invalid.Items = append(invalid.Items, duplicate)
	invalid.Total = len(invalid.Items)
	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: invalid}
	second := &hotRankingFallbackTestProvider{name: "hithink", snapshot: populatedHotRankingForFallbackTest("hithink")}
	svc := hotRankingFallbackService(cache.NewDisabled(), first, second)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "hithink" || len(got.Items) != 1 {
		t.Fatalf("duplicate rank must reject the first provider and fall back, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("duplicate rank must trigger fallback, calls=%d/%d", first.calls, second.calls)
	}
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

func unknownQualityHotRankingForFallbackTest(source string) *model.HotRankingSnapshot {
	snapshot := populatedHotRankingForFallbackTest(source)
	snapshot.QualityStatus = model.QualityUnknown
	snapshot.QualityReasons = []string{"provider_ranking_quality_unknown"}
	return snapshot
}

func TestHotRankingFallsBackFromItemSourceVersionMismatch(t *testing.T) {
	mismatched := populatedHotRankingForFallbackTest("eastmoney")
	mismatched.Items[0].SourceVersion = "older-version"
	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: mismatched}
	second := &hotRankingFallbackTestProvider{name: "hithink", snapshot: populatedHotRankingForFallbackTest("hithink")}
	svc := hotRankingFallbackService(cache.NewDisabled(), first, second)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "hithink" || got.Items[0].SourceVersion != got.SourceVersion {
		t.Fatalf("expected fallback after source-version mismatch, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("version mismatch must trigger fallback, calls=%d/%d", first.calls, second.calls)
	}
}

func TestHotRankingFallsBackFromUnknownQualityRows(t *testing.T) {
	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: unknownQualityHotRankingForFallbackTest("eastmoney")}
	second := &hotRankingFallbackTestProvider{name: "hithink", snapshot: populatedHotRankingForFallbackTest("hithink")}
	svc := hotRankingFallbackService(cache.NewDisabled(), first, second)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "hithink" || got.QualityStatus != model.QualityDegraded {
		t.Fatalf("expected later degraded provider to outrank unknown rows, got %+v", got)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("expected fallback after unknown-quality rows, first=%d second=%d", first.calls, second.calls)
	}
}

func TestHotRankingLegacyUnknownRowsCacheDoesNotBlockFallback(t *testing.T) {
	legacy := unknownQualityHotRankingForFallbackTest("eastmoney")
	encoded, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	mem := newMemoryCache()
	mem.values["md:v1:hot-ranking:popular:10"] = encoded
	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: legacy}
	second := &hotRankingFallbackTestProvider{name: "hithink", snapshot: populatedHotRankingForFallbackTest("hithink")}
	svc := hotRankingFallbackService(mem, first, second)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("legacy unknown rows cache must not short-circuit fallback: hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "hithink" || second.calls != 1 {
		t.Fatalf("expected fallback source after bypassing old unknown rows cache, got %+v calls=%d", got, second.calls)
	}
}

func TestHotRankingKeepsUnknownCandidateWithMoreMetricCoverage(t *testing.T) {
	sparse := unknownQualityHotRankingForFallbackTest("eastmoney")
	rich := unknownQualityHotRankingForFallbackTest("hithink")
	latestPrice, change, changePercent, heat := 1500.0, 10.0, 0.67, 88.0
	rankChange := 2
	rich.Items[0].LatestPrice = &latestPrice
	rich.Items[0].Change = &change
	rich.Items[0].ChangePercent = &changePercent
	rich.Items[0].Heat = &heat
	rich.Items[0].RankChange = &rankChange
	rich.Items[0].RankTrend = "rising"

	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: sparse}
	second := &hotRankingFallbackTestProvider{name: "hithink", snapshot: rich}
	failed := &hotRankingFallbackTestProvider{name: "third-source", err: errors.New("test ranking endpoint unavailable")}
	svc := hotRankingFallbackService(newMemoryCache(), first, second, failed)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "hithink" || got.QualityStatus != model.QualityUnknown {
		t.Fatalf("expected the unknown ranking with richer item fields, got %+v", got)
	}
	if got.Items[0].LatestPrice == nil || got.Items[0].Heat == nil ||
		first.calls != 1 || second.calls != 1 || failed.calls != 1 {
		t.Fatalf("expected richer item fields and all Providers attempted: snapshot=%+v calls=%d/%d/%d", got, first.calls, second.calls, failed.calls)
	}
}

func TestHotRankingReturnsUnknownRowsOnlyAfterFallbackExhaustion(t *testing.T) {
	first := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: unknownQualityHotRankingForFallbackTest("eastmoney")}
	second := &hotRankingFallbackTestProvider{name: "hithink", err: errors.New("test provider unavailable")}
	svc := hotRankingFallbackService(newMemoryCache(), first, second)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.QualityStatus != model.QualityUnknown || len(got.Items) != 1 {
		t.Fatalf("expected unknown rows after fallback exhaustion, got %+v", got)
	}
	found := false
	for _, reason := range got.QualityReasons {
		if reason == "hot_ranking_fallback_exhausted_without_usable_quality" {
			found = true
			break
		}
	}
	if !found || first.calls != 1 || second.calls != 1 {
		t.Fatalf("missing unknown-quality exhaustion marker or incomplete fallback: reasons=%v calls=%d/%d", got.QualityReasons, first.calls, second.calls)
	}
	cached, cacheHit, cacheErr := svc.HotStocks(context.Background(), "popular", 10)
	if cacheErr != nil || !cacheHit || cached == nil || cached.QualityStatus != model.QualityUnknown {
		t.Fatalf("marked unknown rows should use their short cache: hit=%v err=%v cached=%+v", cacheHit, cacheErr, cached)
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


func TestHotRankingFallsBackFromNonFiniteHeat(t *testing.T) {
	invalid := populatedHotRankingForFallbackTest("eastmoney")
	badHeat := math.NaN()
	invalid.Items[0].Heat = &badHeat
	primary := &hotRankingFallbackTestProvider{name: "eastmoney", snapshot: invalid}
	fallback := &hotRankingFallbackTestProvider{name: "hithink", snapshot: populatedHotRankingForFallbackTest("hithink")}
	svc := hotRankingFallbackService(cache.NewDisabled(), primary, fallback)

	got, hit, err := svc.HotStocks(context.Background(), "popular", 10)
	if err != nil || hit {
		t.Fatalf("HotStocks returned hit=%v err=%v", hit, err)
	}
	if got == nil || got.Source != "hithink" {
		t.Fatalf("non-finite heat must reject the first provider and use fallback, got %+v", got)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("non-finite heat must trigger provider fallback, calls=%d/%d", primary.calls, fallback.calls)
	}
}
