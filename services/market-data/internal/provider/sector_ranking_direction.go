package provider

import (
	"errors"
	"sort"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// sortSectorRankingSnapshot 将排名按涨跌幅稳定排序并重排行次。等涨跌幅时按代码、名称排序。
func sortSectorRankingSnapshot(snapshot *model.SectorRankingSnapshot, direction string) error {
	if snapshot == nil {
		return errors.New("sector-ranking snapshot is empty")
	}
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction != "up" && direction != "down" {
		return errors.New("sector-ranking direction must be up or down")
	}
	for _, item := range snapshot.Items {
		if item.ChangePercent == nil {
			return errors.New("sector-ranking item is missing change percent")
		}
	}
	sort.SliceStable(snapshot.Items, func(i, j int) bool {
		left, right := *snapshot.Items[i].ChangePercent, *snapshot.Items[j].ChangePercent
		if left != right {
			if direction == "up" {
				return left > right
			}
			return left < right
		}
		if snapshot.Items[i].Code != snapshot.Items[j].Code {
			return snapshot.Items[i].Code < snapshot.Items[j].Code
		}
		return snapshot.Items[i].Name < snapshot.Items[j].Name
	})
	snapshot.Direction = direction
	for index := range snapshot.Items {
		snapshot.Items[index].Rank = index + 1
	}
	return nil
}
