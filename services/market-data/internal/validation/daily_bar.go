package validation

import (
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/symbol"
)

type DailyBarContext struct {
	RequestedSymbol string
	StartDate       string
	EndDate         string
	Period          string
}

func ValidateDailyBar(bar model.DailyBar, ctx DailyBarContext) Report {
	report := NewReport(DefaultMaxIssues)
	add := func(field, code, expected, message string, severity Severity) {
		item := issue(StageValidate, field, code, expected, message, severity)
		item.Symbol = bar.Symbol
		item.TradeDate = bar.TradeDate
		report.Add(item)
	}

	if strings.TrimSpace(ctx.RequestedSymbol) != "" {
		requested, err := symbol.Normalize(ctx.RequestedSymbol)
		if err != nil || requested != ctx.RequestedSymbol {
			add("requested_symbol", "invalid_symbol", "canonical A-share symbol", "requested symbol is not canonical", SeverityError)
		} else if strings.TrimSpace(bar.Symbol) == "" {
			add("symbol", "required", "provider symbol matching requested symbol", "daily bar symbol is missing", SeverityError)
		} else {
			actual, actualErr := symbol.Normalize(bar.Symbol)
			if actualErr != nil || actual != requested {
				add("symbol", "identity_mismatch", requested, "provider daily bar symbol does not match requested symbol", SeverityError)
			}
		}
	} else if strings.TrimSpace(bar.Symbol) == "" {
		add("symbol", "required", "canonical A-share symbol", "daily bar symbol is required", SeverityError)
	} else if canonical, err := symbol.Normalize(bar.Symbol); err != nil || canonical != bar.Symbol {
		add("symbol", "invalid_symbol", "canonical A-share symbol", "daily bar symbol is not canonical", SeverityError)
	}

	parsed, err := time.Parse("2006-01-02", bar.TradeDate)
	if err != nil || parsed.Format("2006-01-02") != bar.TradeDate {
		add("trade_date", "invalid_date", "YYYY-MM-DD", "daily bar trade_date is invalid", SeverityError)
	} else {
		if ctx.StartDate != "" && validISODate(ctx.StartDate) && bar.TradeDate < ctx.StartDate {
			add("trade_date", "out_of_range", "on or after "+ctx.StartDate, "daily bar is before requested start date", SeverityWarning)
		}
		if ctx.EndDate != "" && validISODate(ctx.EndDate) && bar.TradeDate > ctx.EndDate {
			add("trade_date", "out_of_range", "on or before "+ctx.EndDate, "daily bar is after requested end date", SeverityWarning)
		}
	}
	if ctx.StartDate != "" && !validISODate(ctx.StartDate) {
		add("start_date", "invalid_date", "YYYY-MM-DD", "requested start date is invalid", SeverityError)
	}
	if ctx.EndDate != "" && !validISODate(ctx.EndDate) {
		add("end_date", "invalid_date", "YYYY-MM-DD", "requested end date is invalid", SeverityError)
	}
	expectedPeriod := ctx.Period
	if expectedPeriod == "" {
		expectedPeriod = "1d"
	}
	if bar.Period != expectedPeriod {
		add("period", "invalid_enum", expectedPeriod, "daily bar period does not match requested period", SeverityError)
	}
	if !finite(bar.Open) || !finite(bar.High) || !finite(bar.Low) || !finite(bar.Close) ||
		bar.Open <= 0 || bar.High <= 0 || bar.Low <= 0 || bar.Close <= 0 {
		add("open,high,low,close", "out_of_range", "positive finite prices", "daily bar OHLC values must be positive and finite", SeverityError)
	} else if bar.Low > bar.High || bar.Open < bar.Low || bar.Open > bar.High || bar.Close < bar.Low || bar.Close > bar.High {
		add("open,high,low,close", "ohlc_relationship", "low <= open/close <= high and low <= high", "daily bar OHLC invariant failed", SeverityError)
	}
	if bar.Volume == nil {
		add("volume", "required", "known non-negative shares", "daily bar volume is unknown", SeverityError)
	} else if *bar.Volume < 0 {
		add("volume", "out_of_range", "non-negative shares", "daily bar volume must not be negative", SeverityError)
	}
	if bar.VolumeUnit != "shares" {
		add("volume_unit", "unit_mismatch", "shares", "daily bar normalized volume unit must be shares", SeverityError)
	}
	if bar.Amount == nil {
		add("amount", "missing_optional", "optional CNY amount", "provider did not supply daily bar amount", SeverityWarning)
	} else if !finite(*bar.Amount) || *bar.Amount < 0 {
		add("amount", "out_of_range", "finite non-negative CNY amount", "daily bar amount must be finite and non-negative", SeverityError)
	} else if bar.AmountUnit != "CNY" {
		add("amount_unit", "unit_mismatch", "CNY", "daily bar amount unit must be CNY when amount is present", SeverityError)
	}
	if bar.Amount != nil && bar.Volume != nil && *bar.Amount >= 0 && *bar.Volume >= 0 {
		if (*bar.Volume == 0 && *bar.Amount > 0) || (*bar.Volume > 0 && *bar.Amount == 0) {
			add("amount,volume", "cross_field_consistency", "both zero or both non-zero", "daily bar amount and volume are inconsistent", SeverityError)
		} else if bar.AdjustmentBasis == model.AdjustmentNone && *bar.Amount > 0 && *bar.Volume > 0 &&
			finite(bar.Low) && finite(bar.High) && bar.Low > 0 && bar.High >= bar.Low {
			average := *bar.Amount / float64(*bar.Volume)
			if average < bar.Low*0.5 || average > bar.High*2 {
				add("amount,volume", "unit_mismatch", "implied average price within a broad OHLC range", "daily bar amount/volume implied price is inconsistent with unadjusted OHLC", SeverityError)
			}
		}
	}
	if bar.ChangePercent != nil && !finite(*bar.ChangePercent) {
		add("change_percent", "invalid_number", "finite percentage points", "daily bar change_percent must be finite when present", SeverityError)
	}
	switch bar.AdjustmentBasis {
	case model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward:
	default:
		add("adjustment_basis", "invalid_enum", "none|forward_adjusted|backward_adjusted", "daily bar adjustment basis is unknown or unsupported", SeverityError)
	}
	if strings.TrimSpace(bar.Source) == "" {
		add("source", "required", "non-empty provider identifier", "daily bar source is required", SeverityError)
	}
	switch bar.QualityStatus {
	case model.QualityValid, model.QualityDegraded, model.QualityStale, model.QualityUnknown:
	case model.QualityInvalid:
		add("quality_status", "invalid_enum", "valid|degraded|stale|unknown", "daily bar quality status is invalid", SeverityError)
	default:
		add("quality_status", "invalid_enum", "valid|degraded|stale|unknown", "daily bar quality status is missing or unsupported", SeverityError)
	}
	return report
}

func validISODate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
