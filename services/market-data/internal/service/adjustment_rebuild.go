package service

import (
	"errors"
	"fmt"
	"sort"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// rebuildAdjustedDailyBars 仅使用同一来源版本、且系统观察与市场可用时间均已证明的因子重建价格。
func rebuildAdjustedDailyBars(raw []model.DailyBar, factors []model.AdjustmentFactor, basis model.AdjustmentBasis, requestedReferenceDate string) ([]model.DailyBar, error) {
	if basis != model.AdjustmentForward && basis != model.AdjustmentBackward {
		return nil, errors.New("adjustment rebuild requires forward or backward basis")
	}
	if len(raw) == 0 || len(factors) == 0 {
		return nil, errors.New("raw bars or adjustment factors are empty")
	}

	type factorKey struct {
		source  string
		version string
	}
	groups := make(map[factorKey]map[string]model.AdjustmentFactor)
	for _, factor := range factors {
		if factor.Factor <= 0 || factor.AvailableAt == nil || factor.SystemObservedAt == nil ||
			factor.AvailableAt.IsZero() || factor.SystemObservedAt.IsZero() {
			continue
		}
		key := factorKey{source: factor.Source, version: factor.SourceVersion}
		if groups[key] == nil {
			groups[key] = make(map[string]model.AdjustmentFactor)
		}
		if previous, exists := groups[key][factor.TradeDate]; !exists ||
			factor.SystemObservedAt.After(*previous.SystemObservedAt) {
			groups[key][factor.TradeDate] = factor
		}
	}

	keys := make([]factorKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].source == keys[j].source {
			return keys[i].version < keys[j].version
		}
		return keys[i].source < keys[j].source
	})

	var selected map[string]model.AdjustmentFactor
	var reference model.AdjustmentFactor
	for _, key := range keys {
		series := groups[key]
		var candidate model.AdjustmentFactor
		for _, factor := range series {
			if factor.TradeDate <= requestedReferenceDate &&
				(candidate.TradeDate == "" || factor.TradeDate > candidate.TradeDate) {
				candidate = factor
			}
		}
		if candidate.TradeDate == "" {
			continue
		}
		complete := true
		for _, bar := range raw {
			if _, exists := series[bar.TradeDate]; !exists {
				complete = false
				break
			}
		}
		if complete {
			selected = series
			reference = candidate
			break
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("no single factor source/version covers every raw bar and reference date <= %s", requestedReferenceDate)
	}

	out := make([]model.DailyBar, len(raw))
	for i, bar := range raw {
		factor, exists := selected[bar.TradeDate]
		if !exists {
			return nil, fmt.Errorf("adjustment factor missing for %s", bar.TradeDate)
		}
		scale := factor.Factor
		if basis == model.AdjustmentForward {
			scale /= reference.Factor
		}
		bar.Open *= scale
		bar.High *= scale
		bar.Low *= scale
		bar.Close *= scale
		// 原始均线不能直接复用到复权价格序列，避免混合口径。
		bar.MA5 = nil
		bar.MA20 = nil
		bar.MA120 = nil
		bar.MA250 = nil
		bar.ChangePercent = nil
		bar.AdjustmentBasis = basis
		bar.AdjustmentMethod = "cumulative_adj_factor_v1"
		bar.AdjustmentRefDate = reference.TradeDate
		bar.AdjustmentFactorSource = factor.Source
		bar.AdjustmentFactorSourceVersion = factor.SourceVersion
		observed := *factor.SystemObservedAt
		bar.AdjustmentFactorSystemObservedAt = &observed
		out[i] = bar
	}
	for i := 1; i < len(out); i++ {
		if out[i-1].Close > 0 {
			change := (out[i].Close/out[i-1].Close - 1) * 100
			out[i].ChangePercent = &change
		}
	}
	return out, nil
}
