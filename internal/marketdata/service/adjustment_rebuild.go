package service

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
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

	// A factor series is only valid for raw bars from the exact same security,
	// Provider and source version. Cross-Provider factor application is forbidden.
	rawSymbol := strings.TrimSpace(raw[0].Symbol)
	rawSource := strings.TrimSpace(raw[0].Source)
	rawSourceVersion := strings.TrimSpace(raw[0].SourceVersion)
	if rawSymbol == "" || rawSource == "" || !hasIdentifiableSourceVersion(rawSourceVersion) {
		return nil, errors.New("raw bars must identify symbol, source and source_version before adjustment rebuild")
	}
	previousTradeDate := ""
	for index, bar := range raw {
		tradeDate, err := time.Parse("2006-01-02", bar.TradeDate)
		if err != nil || tradeDate.Format("2006-01-02") != bar.TradeDate {
			return nil, fmt.Errorf("raw bar %d has invalid trade date %q", index, bar.TradeDate)
		}
		if strings.TrimSpace(bar.Symbol) != rawSymbol || strings.TrimSpace(bar.Source) != rawSource ||
			strings.TrimSpace(bar.SourceVersion) != rawSourceVersion {
			return nil, errors.New("raw bars contain mixed symbol, source or source_version")
		}
		if bar.AdjustmentBasis != model.AdjustmentNone {
			return nil, errors.New("adjustment rebuild requires unadjusted raw bars")
		}
		if previousTradeDate != "" && bar.TradeDate <= previousTradeDate {
			return nil, errors.New("raw bars must have unique trade dates in ascending order")
		}
		previousTradeDate = bar.TradeDate
	}
	if requestedReferenceDate != "" {
		parsed, err := time.Parse("2006-01-02", requestedReferenceDate)
		if err != nil || parsed.Format("2006-01-02") != requestedReferenceDate {
			return nil, fmt.Errorf("invalid adjustment reference date %q", requestedReferenceDate)
		}
	}
	if basis == model.AdjustmentForward {
		if requestedReferenceDate == "" || requestedReferenceDate < previousTradeDate {
			return nil, errors.New("forward adjustment reference date must not precede the latest raw bar")
		}
	}

	now := time.Now().UTC()
	type factorKey struct {
		source  string
		version string
		basis   model.AdjustmentFactorBasis
	}
	groups := make(map[factorKey]map[string]model.AdjustmentFactor)
	for _, factor := range factors {
		if strings.TrimSpace(factor.Symbol) != rawSymbol ||
			strings.TrimSpace(factor.Source) != rawSource ||
			strings.TrimSpace(factor.SourceVersion) != rawSourceVersion {
			continue
		}
		factorDate, dateErr := time.Parse("2006-01-02", factor.TradeDate)
		if dateErr != nil || factorDate.Format("2006-01-02") != factor.TradeDate ||
			factor.Factor <= 0 || math.IsNaN(factor.Factor) || math.IsInf(factor.Factor, 0) ||
			(factor.QualityStatus != model.QualityValid && factor.QualityStatus != model.QualityDegraded) ||
			factor.FetchedAt.IsZero() || factor.FetchedAt.After(now) ||
			factor.SystemObservedAt == nil || factor.SystemObservedAt.IsZero() ||
			factor.SystemObservedAt.After(now) || factor.FetchedAt.After(*factor.SystemObservedAt) {
			continue
		}
		if factor.AvailableAt != nil && (factor.AvailableAt.IsZero() ||
			factor.AvailableAt.After(now) || factor.AvailableAt.After(factor.FetchedAt)) {
			continue
		}
		if factor.AvailableAt == nil {
			if mode == adjustmentFactorRebuildPIT {
				continue
			}
			// Current reconstruction may consume a persisted factor with unknown
			// Provider availability, but it must never report that result as valid.
			factor.QualityStatus = model.QualityDegraded
			appendReasonOnce(&factor.QualityReasons, "provider_available_at_unknown")
		}
		factorBasis := model.NormalizeAdjustmentFactorBasis(factor.Basis)
		if factorBasis != model.AdjustmentFactorShared && factorBasis != model.AdjustmentFactorForward && factorBasis != model.AdjustmentFactorBackward {
			continue
		}
		if factorBasis != model.AdjustmentFactorShared && factorBasis != factorBasisForAdjustment(basis) {
			continue
		}
		key := factorKey{source: rawSource, version: rawSourceVersion, basis: factorBasis}
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
		if keys[i].basis != keys[j].basis {
			return keys[i].basis == factorBasisForAdjustment(basis)
		}
		return keys[i].version < keys[j].version
	})

	var selected map[string]model.AdjustmentFactor
	var selectedBasis model.AdjustmentFactorBasis
	var reference model.AdjustmentFactor
	for _, key := range keys {
		series := groups[key]
		candidateBars := make(map[string]model.AdjustmentFactor, len(raw))
		complete := true
		for _, bar := range raw {
			var candidate model.AdjustmentFactor
			for _, factor := range series {
				if factor.TradeDate <= bar.TradeDate &&
					(candidate.TradeDate == "" || factor.TradeDate > candidate.TradeDate) {
					candidate = factor
				}
			}
			if candidate.TradeDate == "" {
				complete = false
				break
			}
			candidateBars[bar.TradeDate] = candidate
		}
		if !complete {
			continue
		}

		var referenceCandidate model.AdjustmentFactor
		// Shared factors (for example Tushare adj_factor) need normalization
		// against the reference-date factor. Direction-specific factors (BaoStock
		// fore/back) are already direct multipliers and must not be divided again.
		needsReferenceFactor := basis == model.AdjustmentForward && key.basis == model.AdjustmentFactorShared
		if needsReferenceFactor {
			for _, factor := range series {
				if factor.TradeDate <= requestedReferenceDate &&
					(referenceCandidate.TradeDate == "" || factor.TradeDate > referenceCandidate.TradeDate) {
					referenceCandidate = factor
				}
			}
			if referenceCandidate.TradeDate == "" {
				continue
			}
		}
		selected = candidateBars
		selectedBasis = key.basis
		reference = referenceCandidate
		break
	}
	if selected == nil {
		detail := ""
		if basis == model.AdjustmentForward {
			detail = " and the forward reference date"
		}
		return nil, fmt.Errorf("no same-security/source/version factor series covers every raw bar%s", detail)
	}

	out := make([]model.DailyBar, len(raw))
	for i, bar := range raw {
		factor := selected[bar.TradeDate]
		scale := factor.Factor
		if basis == model.AdjustmentForward && selectedBasis == model.AdjustmentFactorShared {
			scale /= reference.Factor
		}
		bar.Open *= scale
		bar.High *= scale
		bar.Low *= scale
		bar.Close *= scale
		// 原始均线与涨跌幅不能直接复用到复权价格序列，避免混合口径。
		bar.MA5 = nil
		bar.MA20 = nil
		bar.MA120 = nil
		bar.MA250 = nil
		bar.ChangePercent = nil
		bar.AdjustmentBasis = basis
		bar.AdjustmentMethod = "cumulative_adj_factor_v1"
		if basis == model.AdjustmentForward {
			// AdjustmentRefDate 是价格序列的参考锚点；reference.TradeDate 是内部命中的最近因子事件日，两者不能混为一谈。
			bar.AdjustmentRefDate = requestedReferenceDate
		} else {
			bar.AdjustmentRefDate = ""
		}
		bar.AdjustmentFactorSource = factor.Source
		bar.AdjustmentFactorSourceVersion = factor.SourceVersion
		applyAdjustmentFactorQuality(&bar, factor, "bar")
		observed := *factor.SystemObservedAt
		if basis == model.AdjustmentForward && selectedBasis == model.AdjustmentFactorShared {
			applyAdjustmentFactorQuality(&bar, reference, "reference")
			if reference.SystemObservedAt.After(observed) {
				observed = *reference.SystemObservedAt
			}
		}
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
