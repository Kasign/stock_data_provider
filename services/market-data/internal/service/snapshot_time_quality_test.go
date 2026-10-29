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

type snapshotTimeStockBasicProvider struct{ item *model.StockBasic }

func (p snapshotTimeStockBasicProvider) Name() string { return "eastmoney" }

func (p snapshotTimeStockBasicProvider) FetchStockBasic(context.Context, string) (*model.StockBasic, error) {
	out := *p.item
	return &out, nil
}

type snapshotTimeStockBoardsProvider struct{ item *model.StockBoardSnapshot }

func (p snapshotTimeStockBoardsProvider) Name() string { return "eastmoney" }

func (p snapshotTimeStockBoardsProvider) FetchStockBoards(context.Context, string) (*model.StockBoardSnapshot, error) {
	out := *p.item
	out.Items = append([]model.StockBoard(nil), p.item.Items...)
	return &out, nil
}

type snapshotTimeSectorRankingProvider struct{ item *model.SectorRankingSnapshot }

func (p snapshotTimeSectorRankingProvider) Name() string { return "eastmoney" }

func (p snapshotTimeSectorRankingProvider) FetchSectorRankings(context.Context, string, int) (*model.SectorRankingSnapshot, error) {
	out := *p.item
	out.Items = append([]model.SectorRanking(nil), p.item.Items...)
	return &out, nil
}

func snapshotTimeQualityService() *DataService {
	return NewWithCapabilities(nil, nil, nil, cache.NewDisabled(), nil,
		time.Second, time.Second, 120, 2000, ResilienceConfig{}, nil)
}

func hasSnapshotQualityReason(reasons []string, target string) bool {
	for _, reason := range reasons {
		if reason == target {
			return true
		}
	}
	return false
}

func TestStockBasicCannotClaimValidWithoutProviderSnapshotTime(t *testing.T) {
	now := time.Now().UTC()
	item := &model.StockBasic{
		Symbol: "600519.SH", Name: "贵州茅台", SecurityType: 1, SecurityTypeLabel: "主板股票",
		ListingStatus: 1, ListingStatusLabel: "上市", Source: "eastmoney", SourceVersion: "test-v1",
		FetchedAt: now, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version,
	}
	svc := snapshotTimeQualityService()
	svc.stockBasicProviders = []provider.StockBasicProvider{snapshotTimeStockBasicProvider{item: item}}
	got, cacheHit, err := svc.StockBasic(context.Background(), "600519.SH")
	if err != nil || cacheHit {
		t.Fatalf("StockBasic returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got.QualityStatus != model.QualityDegraded || !hasSnapshotQualityReason(got.QualityReasons, "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("missing provider snapshot time must degrade stock-basic data: %+v", got)
	}
}

func TestStockBoardsCannotClaimValidWithoutProviderSnapshotTime(t *testing.T) {
	now := time.Now().UTC()
	change := 1.2
	item := &model.StockBoardSnapshot{
		Symbol: "600519.SH", Total: 1, HasData: true,
		Items: []model.StockBoard{{Code: "BK0477", Name: "酿酒行业", ChangePercent: &change,
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid}},
		Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
	}
	svc := snapshotTimeQualityService()
	svc.stockBoardsProviders = []provider.StockBoardsProvider{snapshotTimeStockBoardsProvider{item: item}}
	got, cacheHit, err := svc.StockBoards(context.Background(), "600519.SH")
	if err != nil || cacheHit {
		t.Fatalf("StockBoards returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got.QualityStatus != model.QualityDegraded || got.Items[0].QualityStatus != model.QualityDegraded ||
		!hasSnapshotQualityReason(got.QualityReasons, "provider_snapshot_timestamp_not_exposed") ||
		!hasSnapshotQualityReason(got.Items[0].QualityReasons, "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("missing provider snapshot time must degrade stock-board data: %+v", got)
	}
}

func TestSectorRankingCannotClaimValidWithoutProviderSnapshotTime(t *testing.T) {
	now := time.Now().UTC()
	change := 1.2
	item := &model.SectorRankingSnapshot{
		Category: "industry", Total: 1,
		Items: []model.SectorRanking{{Rank: 1, Code: "BK0477", Name: "酿酒行业", Category: "industry", ChangePercent: &change,
			AmountUnit: "unknown", Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid}},
		Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: model.QualityValid,
	}
	svc := snapshotTimeQualityService()
	svc.sectorRankingProviders = []provider.SectorRankingProvider{snapshotTimeSectorRankingProvider{item: item}}
	got, cacheHit, err := svc.SectorRankings(context.Background(), "industry", 10)
	if err != nil || cacheHit {
		t.Fatalf("SectorRankings returned cacheHit=%v err=%v", cacheHit, err)
	}
	if got.QualityStatus != model.QualityDegraded || got.Items[0].QualityStatus != model.QualityDegraded ||
		!hasSnapshotQualityReason(got.QualityReasons, "provider_snapshot_timestamp_not_exposed") ||
		!hasSnapshotQualityReason(got.Items[0].QualityReasons, "provider_snapshot_timestamp_not_exposed") {
		t.Fatalf("missing provider snapshot time must degrade sector rankings: %+v", got)
	}
}
