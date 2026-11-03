package service

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestValidateSectorRankingSnapshotUsesSelectedMetric(t *testing.T) {
	now := time.Now().UTC()
	changeA, changeB := 2.0, 10.0
	amountA, amountB, amountC := 300.0, 200.0, 400.0
	item := func(rank int, code, name string, change, amount *float64) model.SectorRanking {
		return model.SectorRanking{
			Rank: rank, Code: code, Name: name, Category: "industry",
			ChangePercent: change, Amount: amount, AmountUnit: "CNY",
			Source: "eastmoney", SourceVersion: "eastmoney-sector-clist-v1",
			FetchedAt: now, QualityStatus: model.QualityDegraded,
		}
	}
	snapshot := &model.SectorRankingSnapshot{
		Category: "industry", Direction: "up", SortBy: "amount", Total: 2,
		Items: []model.SectorRanking{
			item(1, "BK0001", "板块一", &changeA, &amountA),
			item(2, "BK0002", "板块二", &changeB, &amountB),
		},
		Source: "eastmoney", SourceVersion: "eastmoney-sector-clist-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
	}
	if err := validateSectorRankingSnapshot("industry", 2, snapshot); err != nil {
		t.Fatalf("amount-sorted snapshot rejected: %v", err)
	}
	snapshot.Items[1].Amount = &amountC
	if err := validateSectorRankingSnapshot("industry", 2, snapshot); err == nil {
		t.Fatal("snapshot not ordered by the selected amount metric must be rejected")
	}
}
