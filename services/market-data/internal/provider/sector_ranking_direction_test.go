package provider

import (
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestSortSectorRankingSnapshotSupportsUpAndDown(t *testing.T) {
	now := time.Now().UTC()
	makeSnapshot := func() *model.SectorRankingSnapshot {
		high, low, middle := 5.0, -2.0, 0.5
		return &model.SectorRankingSnapshot{
			Category: "industry", Total: 3, Source: "fixture", FetchedAt: now, QualityStatus: model.QualityDegraded,
			Items: []model.SectorRanking{
				{Code: "BK3", Name: "高", Rank: 3, ChangePercent: &high},
				{Code: "BK1", Name: "低", Rank: 1, ChangePercent: &low},
				{Code: "BK2", Name: "中", Rank: 2, ChangePercent: &middle},
			},
		}
	}
	t.Run("up", func(t *testing.T) {
		got := makeSnapshot()
		if err := sortSectorRankingSnapshot(got, "up"); err != nil {
			t.Fatal(err)
		}
		if got.Direction != "up" || got.Items[0].Code != "BK3" || got.Items[1].Code != "BK2" || got.Items[2].Code != "BK1" {
			t.Fatalf("unexpected up ordering: %+v", got)
		}
	})
	t.Run("down", func(t *testing.T) {
		got := makeSnapshot()
		if err := sortSectorRankingSnapshot(got, "down"); err != nil {
			t.Fatal(err)
		}
		if got.Direction != "down" || got.Items[0].Code != "BK1" || got.Items[1].Code != "BK2" || got.Items[2].Code != "BK3" {
			t.Fatalf("unexpected down ordering: %+v", got)
		}
		for index, item := range got.Items {
			if item.Rank != index+1 {
				t.Fatalf("rank at %d = %d, want %d", index, item.Rank, index+1)
			}
		}
	})
}
