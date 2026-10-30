package service

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestValidateSectorRankingSnapshotAcceptsFiveDayMetricOnlyWhenPresent(t *testing.T) {
	now := time.Now().UTC()
	change, fiveDay := 1.2, 5.6
	snapshot := &model.SectorRankingSnapshot{
		Category: "industry", Direction: "up", SortBy: "change_5d", Total: 1,
		Source: "eastmoney", SourceVersion: "eastmoney-sector-5d-kline-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		Items: []model.SectorRanking{{
			Rank: 1, Code: "BK0001", Name: "行业一", Category: "industry",
			ChangePercent: &change, ChangePercent5D: &fiveDay, AmountUnit: "unknown",
			Source: "eastmoney", SourceVersion: "eastmoney-sector-5d-kline-v1",
			FetchedAt: now, QualityStatus: model.QualityDegraded,
		}},
	}
	if err := validateSectorRankingSnapshot("industry", 10, snapshot); err != nil {
		t.Fatalf("valid five-day ranking rejected: %v", err)
	}
	snapshot.Items[0].ChangePercent5D = nil
	if err := validateSectorRankingSnapshot("industry", 10, snapshot); err == nil {
		t.Fatal("missing five-day metric must be rejected")
	}
}
