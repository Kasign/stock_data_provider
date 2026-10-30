package provider

import (
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestSortSectorRankingSnapshotByAmountRequiresVerifiedAmounts(t *testing.T) {
	amount1, amount2 := 200.0, 100.0
	change1, change2 := 1.0, 9.0
	snapshot := &model.SectorRankingSnapshot{Items: []model.SectorRanking{
		{Code: "BK0002", Name: "B", ChangePercent: &change1, Amount: &amount1, AmountUnit: "CNY"},
		{Code: "BK0001", Name: "A", ChangePercent: &change2, Amount: &amount2, AmountUnit: "CNY"},
	}}
	if err := sortSectorRankingSnapshotByMetric(snapshot, "up", "amount"); err != nil {
		t.Fatalf("sort by amount: %v", err)
	}
	if snapshot.Items[0].Code != "BK0002" || snapshot.Items[0].Rank != 1 || snapshot.SortBy != "amount" {
		t.Fatalf("unexpected amount ranking: %+v", snapshot)
	}
	snapshot.Items[1].Amount = nil
	if err := sortSectorRankingSnapshotByMetric(snapshot, "up", "amount"); err == nil {
		t.Fatal("missing amount must not be treated as zero")
	}
}
