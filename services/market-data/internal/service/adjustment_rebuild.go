package service

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

type adjustmentFactorRebuildMode uint8

const (
	adjustmentFactorRebuildCurrent adjustmentFactorRebuildMode = iota + 1
	adjustmentFactorRebuildPIT
)

// rebuildAdjustedDailyBars 保留历史调用语义：严格按 PIT 规则筛选因子。
func rebuildAdjustedDailyBars(raw []model.DailyBar, factors []model.AdjustmentFactor, basis model.AdjustmentBasis, requestedReferenceDate string) ([]model.DailyBar, error) {
	return rebuildAdjustedDailyBarsPIT(raw, factors, basis, requestedReferenceDate)
}

// rebuildAdjustedDailyBarsPIT 仅使用系统当时已观察且 Provider 可用时间已证明的因子。
func rebuildAdjustedDailyBarsPIT(raw []model.DailyBar, factors []model.AdjustmentFactor, basis model.AdjustmentBasis, requestedReferenceDate string) ([]model.DailyBar, error) {
	return rebuildAdjustedDailyBarsWithMode(raw, factors, basis, requestedReferenceDate, adjustmentFactorRebuildPIT)
}

// rebuildAdjustedDailyBarsCurrent 用于当前数据重建，可使用系统已观察但 Provider AvailableAt 未知的因子。
// 该入口不得用于任何历史 PIT 查询；SystemObservedAt 仍是必要条件，防止消费未持久化的 Provider 临时结果。
func rebuildAdjustedDailyBarsCurrent(raw []model.DailyBar, factors []model.AdjustmentFactor, basis model.AdjustmentBasis, requestedReferenceDate string) ([]model.DailyBar, error) {
	return rebuildAdjustedDailyBarsWithMode(raw, factors, basis, requestedReferenceDate, adjustmentFactorRebuildCurrent)
}

func rebuildAdjustedDailyBarsWithMode(raw []model.DailyBar, factors []model.AdjustmentFactor, basis model.AdjustmentBasis, requestedReferenceDate string, mode adjustmentFactorRebuildMode) ([]model.DailyBar, error) {
	if mode != adjustmentFactorRebuildCurrent && mode != adjustmentFactorRebuildPIT {
		return nil, errors.New("unsupported adjustment factor rebuild mode")
	}
	if basis != model.AdjustmentForward && basis != model.AdjustmentBackward {
		return nil, errors.New("adjustment rebuild requires forward or backward basis")
	}
	if len(raw) == 0 || len(factors) == 0 {
		return nil, errors.New("raw bars or adjustment factors are empty")
	}

	type factorKey struct {
		source  string
		version string
		basis   model.AdjustmentFactorBasis
	}
	groups := make(map[factorKey]map[string]model.AdjustmentFactor)
	for _, factor := range factors {
		if factor.Factor <= 0 || (factor.QualityStatus != model.QualityValid && factor.QualityStatus != model.QualityDegraded) ||
			factor.SystemObservedAt == nil || factor.SystemObservedAt.IsZero() ||
			factor.SystemObservedAt.After(time.Now().UTC()) {
			continue
		}
		if mode == adjustmentFactorRebuildPIT &&
			(factor.AvailableAt == nil || factor.AvailableAt.IsZero()) {
			continue
		}
		factorBasis := model.NormalizeAdjustmentFactorBasis(factor.Basis)
		if factorBasis != model.AdjustmentFactorShared && factorBasis != model.AdjustmentFactorForward && factorBasis != model.AdjustmentFactorBackward {
			continue
		}
		if factorBasis != model.AdjustmentFactorShared && factorBasis != factorBasisForAdjustment(basis) {
			continue
		}
		key := factorKey{source: factor.Source, version: factor.SourceVersion, basis: factorBasis}
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
			if keys[i].basis != keys[j].basis {
				return keys[i].basis == factorBasisForAdjustment(basis)
			}
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
		if basis == model.AdjustmentForward {
			// 前复权必须暴露实际采用的参考因子日期；累计因子后复权使用固定基准，不冒充请求结束日。
			bar.AdjustmentRefDate = reference.TradeDate
		} else {
			bar.AdjustmentRefDate = ""
		}
		bar.AdjustmentFactorSource = factor.Source
		bar.AdjustmentFactorSourceVersion = factor.SourceVersion
		applyAdjustmentFactorQuality(&bar, factor, "bar")
		if basis == model.AdjustmentForward {
			applyAdjustmentFactorQuality(&bar, reference, "reference")
		}
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

func factorBasisForAdjustment(basis model.AdjustmentBasis) model.AdjustmentFactorBasis {
	if basis == model.AdjustmentForward {
		return model.AdjustmentFactorForward
	}
	return model.AdjustmentFactorBackward
}

func applyAdjustmentFactorQuality(bar *model.DailyBar, factor model.AdjustmentFactor, role string) {
	if bar == nil || factor.QualityStatus != model.QualityDegraded {
		return
	}
	degradeQualityStatus(&bar.QualityStatus)
	appendReasonOnce(&bar.QualityReasons, "adjustment_factor_"+role+"_quality_degraded")
	for _, reason := range factor.QualityReasons {
		if reason != "" {
			appendReasonOnce(&bar.QualityReasons, "adjustment_factor_"+role+":"+reason)
		}
	}
}
