package validation

import (
	"math"
	"strings"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/symbol"
)

const (
	quoteChangeToleranceCNY       = 0.02
	quoteChangePercentTolerancePP = 0.15
)

func ValidateQuote(q *model.Quote) Report {
	report := NewReport(DefaultMaxIssues)
	if q == nil {
		report.Add(issue(StageValidate, "quote", "required", "non-nil quote", "quote is nil", SeverityError))
		return report
	}

	if strings.TrimSpace(q.Symbol) == "" {
		report.Add(issue(StageValidate, "symbol", "required", "canonical A-share symbol", "quote symbol is required", SeverityError))
	} else if canonical, err := symbol.Normalize(q.Symbol); err != nil || canonical != q.Symbol {
		report.Add(issue(StageValidate, "symbol", "invalid_symbol", "canonical six-digit code with exchange suffix", "quote symbol is not canonical", SeverityError))
	}
	if strings.TrimSpace(q.Source) == "" {
		report.Add(issue(StageValidate, "source", "required", "non-empty provider identifier", "quote source is required", SeverityError))
	}
	switch q.QualityStatus {
	case model.QualityValid, model.QualityDegraded, model.QualityStale, model.QualityUnknown:
	case model.QualityInvalid:
		report.Add(issue(StageValidate, "quality_status", "invalid_enum", "valid|degraded|stale|unknown", "quote quality status is invalid", SeverityError))
	default:
		report.Add(issue(StageValidate, "quality_status", "invalid_enum", "valid|degraded|stale|unknown", "quote quality status is missing or unsupported", SeverityError))
	}
	switch q.AdjustmentBasis {
	case model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward, model.AdjustmentUnknown:
	default:
		report.Add(issue(StageValidate, "adjustment_basis", "invalid_enum", "none|forward_adjusted|backward_adjusted|unknown", "quote adjustment basis is missing or unsupported", SeverityError))
	}

	requiredPositive := []struct {
		name  string
		value *float64
	}{
		{"current_price", q.CurrentPrice}, {"open", q.Open}, {"high", q.High},
		{"low", q.Low}, {"prev_close", q.PrevClose},
	}
	for _, item := range requiredPositive {
		if item.value == nil {
			report.Add(issue(StageValidate, item.name, "required", "positive finite number", "quote field is missing", SeverityError))
		} else if !finite(*item.value) || *item.value <= 0 {
			report.Add(issue(StageValidate, item.name, "out_of_range", "positive finite number", "quote field must be positive and finite", SeverityError))
		}
	}
	if q.Low != nil && q.High != nil && q.Open != nil && q.CurrentPrice != nil &&
		finite(*q.Low) && finite(*q.High) && finite(*q.Open) && finite(*q.CurrentPrice) &&
		(*q.Low > *q.High || *q.Open < *q.Low || *q.Open > *q.High || *q.CurrentPrice < *q.Low || *q.CurrentPrice > *q.High) {
		report.Add(issue(StageValidate, "ohlc", "ohlc_relationship", "low <= open/current_price <= high and low <= high", "quote OHLC invariant failed", SeverityError))
	}

	optionalNumbers := []struct {
		name        string
		value       *float64
		nonNegative bool
	}{
		{"change", q.Change, false}, {"change_percent", q.ChangePercent, false},
		{"volume_ratio", q.VolumeRatio, true}, {"turnover_rate", q.TurnoverRate, true},
	}
	for _, item := range optionalNumbers {
		if item.value != nil && (!finite(*item.value) || (item.nonNegative && *item.value < 0)) {
			report.Add(issue(StageValidate, item.name, "out_of_range", "finite number", "quote field must be finite and within its allowed range", SeverityError))
		}
	}

	if q.CurrentPrice != nil && q.PrevClose != nil && finite(*q.CurrentPrice) && finite(*q.PrevClose) && *q.PrevClose > 0 {
		expected := *q.CurrentPrice - *q.PrevClose
		if q.Change != nil && finite(*q.Change) && math.Abs(*q.Change-expected) > quoteChangeToleranceCNY {
			report.Add(issue(StageValidate, "change", "cross_field_consistency", "current_price - prev_close (±0.02 CNY)", "quote change is inconsistent with current_price minus prev_close", SeverityError))
		}
		if q.ChangePercent != nil && finite(*q.ChangePercent) {
			expectedPercent := expected / *q.PrevClose * 100
			if !finite(expectedPercent) || math.Abs(*q.ChangePercent-expectedPercent) > quoteChangePercentTolerancePP {
				report.Add(issue(StageValidate, "change_percent", "cross_field_consistency", "calculated percentage points (±0.15 pp)", "quote change_percent is inconsistent with current_price and prev_close", SeverityError))
			}
		}
	}

	if q.Volume == nil {
		report.Add(issue(StageValidate, "volume", "required", "known non-negative shares", "quote volume is unknown", SeverityError))
	} else if *q.Volume < 0 {
		report.Add(issue(StageValidate, "volume", "out_of_range", "non-negative shares", "quote volume must not be negative", SeverityError))
	}
	if q.VolumeUnit != "shares" {
		report.Add(issue(StageValidate, "volume_unit", "unit_mismatch", "shares", "quote normalized volume unit must be shares", SeverityError))
	}
	if q.Amount == nil {
		if q.QualityStatus == model.QualityValid {
			report.Add(issue(StageValidate, "amount", "required", "CNY amount or degraded/unknown quality", "quote with unknown amount must not be marked valid", SeverityError))
		} else {
			report.Add(issue(StageValidate, "amount", "missing_optional", "optional CNY amount", "provider did not supply quote amount", SeverityWarning))
		}
	} else if !finite(*q.Amount) || *q.Amount < 0 {
		report.Add(issue(StageValidate, "amount", "out_of_range", "finite non-negative CNY amount", "quote amount must be finite and non-negative", SeverityError))
	} else if q.AmountUnit != "CNY" {
		report.Add(issue(StageValidate, "amount_unit", "unit_mismatch", "CNY", "quote amount unit must be CNY when amount is present", SeverityError))
	}
	if q.Amount != nil && q.Volume != nil && *q.Amount >= 0 && *q.Volume >= 0 {
		if (*q.Volume == 0 && *q.Amount > 0) || (*q.Volume > 0 && *q.Amount == 0) {
			report.Add(issue(StageValidate, "amount,volume", "cross_field_consistency", "both zero or both non-zero", "quote amount and volume are inconsistent", SeverityError))
		} else if *q.Amount > 0 && *q.Volume > 0 && q.Low != nil && q.High != nil && finite(*q.Low) && finite(*q.High) && *q.Low > 0 && *q.High >= *q.Low {
			average := *q.Amount / float64(*q.Volume)
			if average < *q.Low*0.5 || average > *q.High*2 {
				report.Add(issue(StageValidate, "amount,volume", "unit_mismatch", "implied average price within a broad OHLC range", "amount/volume implied average price is inconsistent with OHLC", SeverityError))
			}
		}
	}
	return report
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}
