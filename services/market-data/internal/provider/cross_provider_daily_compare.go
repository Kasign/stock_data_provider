package provider

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	CrossProviderDailyMatched       = "matched"
	CrossProviderDailyPartial       = "partial"
	CrossProviderDailyMismatch      = "mismatch"
	CrossProviderDailyNotComparable = "not_comparable"
)

// CrossProviderDailySnapshot 是跨 Provider 日 K 对账的最小输入契约。
// 所有数值必须已按统一模型归一化，不能在比较器内猜测或转换单位。
type CrossProviderDailySnapshot struct {
	Provider        string
	SourceVersion   string
	Symbol          string
	AdjustmentBasis string
	QualityStatus   string
	Bars            []CrossProviderDailyBar
}

// CrossProviderVolumeGranularity 根据原始成交量单位返回归一化股数粒度。
// hands/hands_inferred 的源值以手计，每手 100 股；shares 的源值以股计，粒度为 1。
// 未知原始单位返回 0，比较器据此判定 not_comparable。
func CrossProviderVolumeGranularity(rawVolumeUnit string) int64 {
	switch strings.ToLower(strings.TrimSpace(rawVolumeUnit)) {
	case "hands", "hands_inferred":
		return 100
	case "shares":
		return 1
	default:
		return 0
	}
}

// CrossProviderDailyBar 表示一个交易日的标准化日 K。
type CrossProviderDailyBar struct {
	TradeDate         string
	Open              float64
	High              float64
	Low               float64
	Close             float64
	Volume            int64
	VolumeUnit        string
	VolumeGranularity int64
	Amount            *float64
	AmountUnit        string
	AdjustmentMethod  string
	AdjustmentRefDate string
}

// CrossProviderFieldResult 记录字段级对账结果。
type CrossProviderFieldResult struct {
	Field      string
	Status     string
	Left       float64
	Right      float64
	Difference float64
	Tolerance  float64
	Reason     string
}

// CrossProviderDailyResult 不可比较与不一致是不同结论，调用方不得将前者统计为通过。
type CrossProviderDailyResult struct {
	Status        string
	Reason        string
	ComparedDates []string
	Fields        []CrossProviderFieldResult
}

// CompareCrossProviderDailyBars 比较两个 Provider 同证券、同交易日、同复权口径的日 K。
// 只有质量状态为 valid 的输入可作为对账基准；未知或降级数据直接拒绝参与。
func CompareCrossProviderDailyBars(left, right CrossProviderDailySnapshot) CrossProviderDailyResult {
	result := CrossProviderDailyResult{Status: CrossProviderDailyNotComparable}
	if left.Provider == "" || right.Provider == "" || left.SourceVersion == "" || right.SourceVersion == "" {
		result.Reason = "provider_or_source_version_missing"
		return result
	}
	if left.Provider == right.Provider {
		result.Reason = "provider_must_be_distinct"
		return result
	}
	if left.Symbol == "" || left.Symbol != right.Symbol {
		result.Reason = "symbol_mismatch"
		return result
	}
	if left.AdjustmentBasis == "" || left.AdjustmentBasis != right.AdjustmentBasis {
		result.Reason = "adjustment_basis_mismatch"
		return result
	}
	if left.QualityStatus != "valid" || right.QualityStatus != "valid" {
		result.Reason = "quality_status_must_be_valid"
		return result
	}
	if reason := validateCrossProviderAdjustmentCompatibility(left, right); reason != "" {
		result.Reason = reason
		return result
	}
	leftBars, leftDates, reason := indexCrossProviderBars(left.Bars)
	if reason != "" {
		result.Reason = "left_" + reason
		return result
	}
	rightBars, rightDates, reason := indexCrossProviderBars(right.Bars)
	if reason != "" {
		result.Reason = "right_" + reason
		return result
	}
	if len(leftDates) == 0 || len(leftDates) != len(rightDates) {
		result.Reason = "trade_date_set_mismatch"
		return result
	}
	for index := range leftDates {
		if leftDates[index] != rightDates[index] {
			result.Reason = "trade_date_set_mismatch"
			return result
		}
	}
	result.ComparedDates = append([]string(nil), leftDates...)

	for _, date := range leftDates {
		a, b := leftBars[date], rightBars[date]
		if a.VolumeUnit != "shares" || b.VolumeUnit != "shares" {
			result.Reason = "volume_unit_must_be_shares"
			return result
		}
		if a.VolumeGranularity <= 0 || b.VolumeGranularity <= 0 {
			result.Reason = "volume_granularity_missing"
			return result
		}
		volumeTolerance := int64(50)
		if a.VolumeGranularity > volumeTolerance {
			volumeTolerance = a.VolumeGranularity
		}
		if b.VolumeGranularity > volumeTolerance {
			volumeTolerance = b.VolumeGranularity
		}
		if reason := validateCrossProviderDailyBar(a); reason != "" {
			result.Reason = "left_" + date + "_" + reason
			return result
		}
		if reason := validateCrossProviderDailyBar(b); reason != "" {
			result.Reason = "right_" + date + "_" + reason
			return result
		}
		for _, field := range []struct {
			name        string
			left, right float64
			tolerance   float64
		}{
			{"open", a.Open, b.Open, 0.001},
			{"high", a.High, b.High, 0.001},
			{"low", a.Low, b.Low, 0.001},
			{"close", a.Close, b.Close, 0.001},
			{"volume", float64(a.Volume), float64(b.Volume), float64(volumeTolerance)},
		} {
			difference := math.Abs(field.left - field.right)
			status := "matched"
			if math.IsNaN(field.left) || math.IsInf(field.left, 0) ||
				math.IsNaN(field.right) || math.IsInf(field.right, 0) {
				status = "not_comparable"
			} else if difference > field.tolerance {
				status = "mismatch"
			}
			result.Fields = append(result.Fields, CrossProviderFieldResult{
				Field: date + "." + field.name, Status: status,
				Left: field.left, Right: field.right, Difference: difference, Tolerance: field.tolerance,
			})
		}
		if a.Amount == nil || b.Amount == nil || a.AmountUnit != "CNY" || b.AmountUnit != "CNY" {
			result.Fields = append(result.Fields, CrossProviderFieldResult{
				Field: date + ".amount", Status: "not_comparable",
				Reason: "amount_missing_or_unit_not_cny",
			})
			continue
		}
		difference := math.Abs(*a.Amount - *b.Amount)
		status := "matched"
		if math.IsNaN(*a.Amount) || math.IsInf(*a.Amount, 0) ||
			math.IsNaN(*b.Amount) || math.IsInf(*b.Amount, 0) {
			status = "not_comparable"
		} else if difference > 1 {
			status = "mismatch"
		}
		result.Fields = append(result.Fields, CrossProviderFieldResult{
			Field: date + ".amount", Status: status, Left: *a.Amount, Right: *b.Amount,
			Difference: difference, Tolerance: 1,
		})
	}

	hasMismatch, hasUncomparable := false, false
	for _, field := range result.Fields {
		hasMismatch = hasMismatch || field.Status == "mismatch"
		hasUncomparable = hasUncomparable || field.Status == "not_comparable"
	}
	switch {
	case hasMismatch:
		result.Status = CrossProviderDailyMismatch
	case hasUncomparable:
		result.Status = CrossProviderDailyPartial
	default:
		result.Status = CrossProviderDailyMatched
	}
	return result
}

func validateCrossProviderAdjustmentCompatibility(left, right CrossProviderDailySnapshot) string {
	if left.AdjustmentBasis == "none" {
		return ""
	}
	if left.AdjustmentBasis != "forward_adjusted" && left.AdjustmentBasis != "backward_adjusted" {
		return "adjustment_basis_unsupported"
	}
	leftMethod, leftRefDate, reason := crossProviderAdjustmentMetadata(left)
	if reason != "" {
		return "left_" + reason
	}
	rightMethod, rightRefDate, reason := crossProviderAdjustmentMetadata(right)
	if reason != "" {
		return "right_" + reason
	}
	if leftMethod != rightMethod {
		return "adjustment_method_mismatch"
	}
	if leftRefDate != rightRefDate {
		return "adjustment_reference_date_mismatch"
	}
	return ""
}

func crossProviderAdjustmentMetadata(snapshot CrossProviderDailySnapshot) (string, string, string) {
	if len(snapshot.Bars) == 0 {
		return "", "", "adjustment_metadata_missing"
	}
	method, refDate := strings.TrimSpace(snapshot.Bars[0].AdjustmentMethod), strings.TrimSpace(snapshot.Bars[0].AdjustmentRefDate)
	if method == "" {
		return "", "", "adjustment_method_missing"
	}
	if refDate == "" {
		return "", "", "adjustment_reference_date_missing"
	}
	for _, bar := range snapshot.Bars[1:] {
		if strings.TrimSpace(bar.AdjustmentMethod) != method || strings.TrimSpace(bar.AdjustmentRefDate) != refDate {
			return "", "", "adjustment_metadata_inconsistent"
		}
	}
	return method, refDate, ""
}

func validateCrossProviderDailyBar(bar CrossProviderDailyBar) string {
	prices := []float64{bar.Open, bar.High, bar.Low, bar.Close}
	for _, value := range prices {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return "invalid_ohlc_value"
		}
	}
	if bar.Low > bar.High || bar.Open < bar.Low || bar.Open > bar.High ||
		bar.Close < bar.Low || bar.Close > bar.High {
		return "invalid_ohlc_relationship"
	}
	if bar.Volume < 0 {
		return "negative_volume"
	}
	if bar.Amount != nil && (math.IsNaN(*bar.Amount) || math.IsInf(*bar.Amount, 0) || *bar.Amount < 0) {
		return "invalid_amount_value"
	}
	return ""
}

func indexCrossProviderBars(bars []CrossProviderDailyBar) (map[string]CrossProviderDailyBar, []string, string) {
	indexed := make(map[string]CrossProviderDailyBar, len(bars))
	dates := make([]string, 0, len(bars))
	for _, bar := range bars {
		if _, err := time.Parse("2006-01-02", bar.TradeDate); err != nil {
			return nil, nil, "invalid_trade_date"
		}
		if _, exists := indexed[bar.TradeDate]; exists {
			return nil, nil, "duplicate_trade_date"
		}
		indexed[bar.TradeDate] = bar
		dates = append(dates, bar.TradeDate)
	}
	sort.Strings(dates)
	return indexed, dates, ""
}
