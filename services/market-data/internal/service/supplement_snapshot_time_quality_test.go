package service

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestSupplementSnapshotsRejectFutureFetchedAt(t *testing.T) {
	now := time.Now().UTC()
	future := now.Add(6 * time.Minute)

	t.Run("announcement snapshot and item", func(t *testing.T) {
		item := model.StockAnnouncement{
			ArticleID: "article-1", Symbol: "600036.SH", Title: "公告标题",
			AnnouncementDate: "2026-11-02", SourceURL: "https://example.com/article",
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
		}
		snapshot := model.StockAnnouncementSnapshot{
			Symbol: "600036.SH", StartDate: "2026-11-02", EndDate: "2026-11-02",
			Page: 1, PageSize: 10, Total: 1, Pages: 1, Returned: 1,
			HasData: true, Items: []model.StockAnnouncement{item},
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
		}
		if err := validateStockAnnouncementSnapshot(snapshot.Symbol, snapshot.StartDate, snapshot.EndDate, 1, 10, &snapshot); err != nil {
			t.Fatalf("valid announcement snapshot rejected: %v", err)
		}
		snapshot.FetchedAt = future
		if err := validateStockAnnouncementSnapshot(snapshot.Symbol, snapshot.StartDate, snapshot.EndDate, 1, 10, &snapshot); err == nil {
			t.Fatal("future announcement snapshot fetched_at must be rejected")
		}
		snapshot.FetchedAt = now
		snapshot.Items[0].FetchedAt = future
		if err := validateStockAnnouncementSnapshot(snapshot.Symbol, snapshot.StartDate, snapshot.EndDate, 1, 10, &snapshot); err == nil {
			t.Fatal("future announcement item fetched_at must be rejected")
		}
	})

	t.Run("dragon tiger snapshot and item", func(t *testing.T) {
		item := model.DragonTigerItem{
			Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", TradeDate: "2026-11-02",
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
		}
		snapshot := model.DragonTigerSnapshot{
			TradeDate: "2026-11-02", ProviderReportDate: "2026-11-02",
			Page: 1, PageSize: 10, Total: 1, Pages: 1, Returned: 1,
			SourceReturned: 1, HasData: true, Items: []model.DragonTigerItem{item},
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
		}
		if err := validateDragonTigerSnapshot(snapshot.TradeDate, 1, 10, &snapshot); err != nil {
			t.Fatalf("valid dragon-tiger snapshot rejected: %v", err)
		}
		snapshot.FetchedAt = future
		if err := validateDragonTigerSnapshot(snapshot.TradeDate, 1, 10, &snapshot); err == nil {
			t.Fatal("future dragon-tiger snapshot fetched_at must be rejected")
		}
		snapshot.FetchedAt = now
		snapshot.Items[0].FetchedAt = future
		if err := validateDragonTigerSnapshot(snapshot.TradeDate, 1, 10, &snapshot); err == nil {
			t.Fatal("future dragon-tiger item fetched_at must be rejected")
		}
	})

	t.Run("limit pool snapshot and item", func(t *testing.T) {
		change := 10.0
		item := model.LimitPoolItem{
			Symbol: "600519.SH", Code: "600519", Name: "贵州茅台", ChangePercent: &change,
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
		}
		snapshot := model.LimitPoolSnapshot{
			PoolType: "up", TradeDate: "2026-11-02", ProviderReportDate: "2026-11-02",
			Total: 1, HasData: true, Items: []model.LimitPoolItem{item},
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded,
		}
		if err := validateLimitPoolSnapshot("up", snapshot.TradeDate, 10, &snapshot); err != nil {
			t.Fatalf("valid limit-pool snapshot rejected: %v", err)
		}
		snapshot.FetchedAt = future
		if err := validateLimitPoolSnapshot("up", snapshot.TradeDate, 10, &snapshot); err == nil {
			t.Fatal("future limit-pool snapshot fetched_at must be rejected")
		}
		snapshot.FetchedAt = now
		snapshot.Items[0].FetchedAt = future
		if err := validateLimitPoolSnapshot("up", snapshot.TradeDate, 10, &snapshot); err == nil {
			t.Fatal("future limit-pool item fetched_at must be rejected")
		}
	})

	t.Run("trading calendar snapshot", func(t *testing.T) {
		snapshot := model.TradingCalendarSnapshot{
			Market: "cn_a_share", StartDate: "2026-11-02", EndDate: "2026-11-02",
			TotalDays: 1, TradingDays: 1,
			Items:  []model.TradingCalendarDay{{Date: "2026-11-02", IsTradingDay: true}},
			Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now,
			QualityStatus: model.QualityValid,
		}
		if err := validateTradingCalendarSnapshot(&snapshot, snapshot.StartDate, snapshot.EndDate); err != nil {
			t.Fatalf("valid trading-calendar snapshot rejected: %v", err)
		}
		snapshot.FetchedAt = future
		if err := validateTradingCalendarSnapshot(&snapshot, snapshot.StartDate, snapshot.EndDate); err == nil {
			t.Fatal("future trading-calendar fetched_at must be rejected")
		}
	})
}
