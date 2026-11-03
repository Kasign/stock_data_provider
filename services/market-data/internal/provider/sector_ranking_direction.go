package provider

import (
	"errors"
	"sort"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// sortSectorRankingSnapshot 保持旧调用兼容，默认按涨跌幅排序。
func sortSectorRankingSnapshot(snapshot *model.SectorRankingSnapshot, direction string) error {
	return sortSectorRankingSnapshotByMetric(snapshot, direction, "change_percent")
}

// sortSectorRankingSnapshotByMetric 按指定指标稳定排序；缺失排序值不允许被当作零。
func sortSectorRankingSnapshotByMetric(snapshot *model.SectorRankingSnapshot, direction, sortBy string) error {
	if snapshot == nil {
		return errors.New("sector-ranking snapshot is empty")
	}
	direction = strings.ToLower(strings.TrimSpace(direction))
	if direction != "up" && direction != "down" {
		return errors.New("sector-ranking direction must be up or down")
	}
	sortBy = strings.ToLower(strings.TrimSpace(sortBy))
	if sortBy == "" {
		sortBy = "change_percent"
	}
	value := func(item model.SectorRanking) (*float64, error) {
		switch sortBy {
		case "change_percent":
			if item.ChangePercent == nil {
				return nil, errors.New("sector-ranking item is missing change percent")
			}
			return item.ChangePercent, nil
		case "change_5d":
			if item.ChangePercent5D == nil {
				return nil, errors.New("sector-ranking item is missing 5-day change percent")
			}
			return item.ChangePercent5D, nil
		case "amount":
			if item.Amount == nil || item.AmountUnit != "CNY" {
				return nil, errors.New("sector-ranking item is missing verified CNY amount")
			}
			return item.Amount, nil
		default:
			return nil, errors.New("sector-ranking sort_by must be change_percent or amount")
		}
	}
	for _, item := range snapshot.Items {
		if _, err := value(item); err != nil {
			return err
		}
	}
	sort.SliceStable(snapshot.Items, func(i, j int) bool {
		left, _ := value(snapshot.Items[i])
		right, _ := value(snapshot.Items[j])
		if *left != *right {
			if direction == "up" {
				return *left > *right
			}
			return *left < *right
		}
		if snapshot.Items[i].Code != snapshot.Items[j].Code {
			return snapshot.Items[i].Code < snapshot.Items[j].Code
		}
		return snapshot.Items[i].Name < snapshot.Items[j].Name
	})
	snapshot.Direction = direction
	snapshot.SortBy = sortBy
	for index := range snapshot.Items {
		snapshot.Items[index].Rank = index + 1
	}
	return nil
}
