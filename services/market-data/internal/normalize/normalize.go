package normalize

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

const Version = "4"

const (
	quoteChangeToleranceCNY       = 0.02
	quoteChangePercentTolerancePP = 0.15
)

var symbolPattern = regexp.MustCompile(`^[0-9]{6}(\.(SH|SZ|BJ))?$`)

func NormalizeSymbol(raw string) (string, error) {
	s := strings.ToUpper(strings.TrimSpace(raw))
	if !symbolPattern.MatchString(s) {
		return "", fmt.Errorf("invalid A-share symbol %q", raw)
	}
	parts := strings.Split(s, ".")
	code := parts[0]
	// This service's normalized monetary contract is CNY and its lot conversion
	// rules are for A-shares. Shanghai 900xxx and Shenzhen 200xxx are B-shares;
	// accepting them would silently misstate their currency/volume semantics.
	if strings.HasPrefix(code, "900") || strings.HasPrefix(code, "200") {
		return "", fmt.Errorf("B-share code %q is outside the supported A-share data contract", code)
	}
	exchange := ""
	switch {
	case strings.HasPrefix(code, "92"):
		exchange = "BJ"
	case strings.HasPrefix(code, "4"), strings.HasPrefix(code, "8"):
		exchange = "BJ"
	case strings.HasPrefix(code, "6"), strings.HasPrefix(code, "5"), strings.HasPrefix(code, "9"):
		exchange = "SH"
	case strings.HasPrefix(code, "0"), strings.HasPrefix(code, "1"), strings.HasPrefix(code, "2"), strings.HasPrefix(code, "3"):
		exchange = "SZ"
	default:
		return "", fmt.Errorf("cannot infer exchange for %q", raw)
	}
	if len(parts) == 2 && parts[1] != exchange {
		return "", fmt.Errorf("symbol %q has exchange suffix %s but code prefix maps to %s", raw, parts[1], exchange)
	}
	return code + "." + exchange, nil
}

func Adjustment(v string) (model.AdjustmentBasis, error) {
	switch model.AdjustmentBasis(strings.ToLower(strings.TrimSpace(v))) {
	case "", model.AdjustmentForward:
		return model.AdjustmentForward, nil
	case model.AdjustmentNone:
		return model.AdjustmentNone, nil
	case model.AdjustmentBackward:
		return model.AdjustmentBackward, nil
	case model.AdjustmentUnknown:
		return model.AdjustmentUnknown, errors.New("unknown is not a requestable adjustment basis")
	default:
		return "", fmt.Errorf("unsupported adjustment basis %q", v)
	}
}

// HandsToShares converts the commonly used A-share lot unit to shares while
// refusing non-finite, negative, or overflowing values. Conversion is performed
// only in provider adapters whose source contract has been identified as lots.
func HandsToShares(raw float64) (int64, error) {
	if !finite(raw) || raw < 0 {
		return 0, errors.New("raw volume in hands must be finite and non-negative")
	}
	if raw >= float64(math.MaxInt64)/100 {
		return 0, errors.New("raw volume in hands overflows normalized shares")
	}
	return int64(math.Round(raw * 100)), nil
}

// RawSharesToInt64 validates a provider field that is already expressed in
// shares. Some Tencent STAR Market daily K-line responses use shares while
// their other A-share K-line responses use hands; the adapter must choose the
// correct conversion from the provider's documented per-market contract.
func RawSharesToInt64(raw float64) (int64, error) {
	if !finite(raw) || raw < 0 {
		return 0, errors.New("raw volume in shares must be finite and non-negative")
	}
	if raw >= float64(math.MaxInt64) {
		return 0, errors.New("raw volume in shares overflows int64")
	}
	return int64(math.Round(raw)), nil
}

func ValidateQuote(q *model.Quote) error {
	if q == nil {
		return errors.New("quote is nil")
	}
	if q.Symbol == "" || q.Source == "" {
		return errors.New("quote symbol and source are required")
	}
	canonicalSymbol, err := NormalizeSymbol(q.Symbol)
	if err != nil || canonicalSymbol != q.Symbol {
		return errors.New("quote symbol must be a canonical six-digit code with exchange suffix")
	}
	switch q.QualityStatus {
	case model.QualityValid, model.QualityDegraded, model.QualityStale, model.QualityUnknown:
	case model.QualityInvalid:
		return errors.New("quote quality status is invalid")
	default:
		return fmt.Errorf("quote quality status %q is missing or unsupported", q.QualityStatus)
	}
	switch q.AdjustmentBasis {
	case model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward, model.AdjustmentUnknown:
	default:
		return fmt.Errorf("quote adjustment basis %q is missing or unsupported", q.AdjustmentBasis)
	}
	for name, p := range map[string]*float64{"current_price": q.CurrentPrice, "open": q.Open, "high": q.High, "low": q.Low, "prev_close": q.PrevClose} {
		if p == nil || !finite(*p) || *p <= 0 {
			return fmt.Errorf("quote %s must be a positive finite value", name)
		}
	}
	if *q.Low > *q.High || *q.Open < *q.Low || *q.Open > *q.High || *q.CurrentPrice < *q.Low || *q.CurrentPrice > *q.High {
		return errors.New("quote OHLC invariant failed")
	}
	for name, p := range map[string]*float64{"change": q.Change, "change_percent": q.ChangePercent, "volume_ratio": q.VolumeRatio, "turnover_rate": q.TurnoverRate} {
		if p != nil && (!finite(*p) || ((name == "volume_ratio" || name == "turnover_rate") && *p < 0)) {
			return fmt.Errorf("quote %s must be finite and non-negative when present", name)
		}
	}
	expectedChange := *q.CurrentPrice - *q.PrevClose
	if q.Change != nil && math.Abs(*q.Change-expectedChange) > quoteChangeToleranceCNY {
		return errors.New("quote change is inconsistent with current_price minus prev_close")
	}
	if q.ChangePercent != nil {
		expectedChangePercent := expectedChange / *q.PrevClose * 100
		if !finite(expectedChangePercent) || math.Abs(*q.ChangePercent-expectedChangePercent) > quoteChangePercentTolerancePP {
			return errors.New("quote change_percent is inconsistent with current_price and prev_close")
		}
	}
	if q.Volume == nil || *q.Volume < 0 {
		return errors.New("quote volume must be known and non-negative")
	}
	if q.Amount != nil && (!finite(*q.Amount) || *q.Amount < 0) {
		return errors.New("quote amount must be finite and non-negative when present")
	}
	if q.Amount == nil {
		if q.QualityStatus == model.QualityValid {
			return errors.New("quote with unknown amount must be marked degraded or unknown")
		}
		// A missing amount does not invalidate otherwise sound price/volume data,
		// but its unit must not be advertised as CNY when no amount value exists.
		q.AmountUnit = "unknown"
		q.QualityReasons = appendReasonUnique(q.QualityReasons, "amount_missing_from_provider")
		q.QualityReasons = appendReasonUnique(q.QualityReasons, "amount_unit_unknown_because_amount_missing")
	} else if q.AmountUnit != "CNY" {
		return errors.New("quote amount unit must be CNY when amount is present")
	}
	if q.Amount != nil && q.Volume != nil {
		if (*q.Volume == 0 && *q.Amount > 0) || (*q.Volume > 0 && *q.Amount == 0) {
			return errors.New("quote amount and volume are inconsistent: one is zero while the other is non-zero")
		}
		if *q.Amount > 0 && *q.Volume > 0 {
			// Amount (CNY) / volume (shares) approximates the average traded price.
			// It should be in the same order of magnitude as the quote's intraday
			// range. This catches common 100x/10000x source unit and price-scale errors
			// without guessing a correction factor.
			averagePrice := *q.Amount / float64(*q.Volume)
			if averagePrice < *q.Low*0.5 || averagePrice > *q.High*2 {
				return errors.New("amount/volume implied average price is inconsistent with OHLC; provider units or price scale may be wrong")
			}
		}
	}
	if q.VolumeUnit != "shares" {
		return errors.New("quote normalized volume unit must be shares")
	}
	return nil
}

func ValidateAndSortBars(symbol string, bars []model.DailyBar, start, end string, limit int) ([]model.DailyBar, []string) {
	warnings := make([]string, 0)
	if start != "" {
		if _, err := time.Parse("2006-01-02", start); err != nil {
			return nil, []string{"invalid_start_date"}
		}
	}
	if end != "" {
		if _, err := time.Parse("2006-01-02", end); err != nil {
			return nil, []string{"invalid_end_date"}
		}
	}
	seen := map[string]bool{}
	valid := make([]model.DailyBar, 0, len(bars))
	ohlcDiagnosticCount := 0
	for _, b := range bars {
		if _, err := time.Parse("2006-01-02", b.TradeDate); err != nil {
			warnings = append(warnings, "invalid_trade_date_dropped")
			continue
		}
		if start != "" && b.TradeDate < start {
			continue
		}
		if end != "" && b.TradeDate > end {
			warnings = append(warnings, "bar_after_end_date_dropped")
			continue
		}
		if seen[b.TradeDate] {
			warnings = append(warnings, "duplicate_trade_date_dropped")
			continue
		}
		if !finite(b.Open) || !finite(b.High) || !finite(b.Low) || !finite(b.Close) || b.Open <= 0 || b.High <= 0 || b.Low <= 0 || b.Close <= 0 || b.Low > b.High || b.Open < b.Low || b.Open > b.High || b.Close < b.Low || b.Close > b.High {
			warnings = append(warnings, "invalid_ohlc_bar_dropped")
			// Keep the stable warning code and add bounded context so operators can
			// identify the source row without logging or returning the raw response.
			if ohlcDiagnosticCount < 5 {
				warnings = append(warnings, fmt.Sprintf("invalid_ohlc_bar_detail:trade_date=%s,open=%v,high=%v,low=%v,close=%v", b.TradeDate, b.Open, b.High, b.Low, b.Close))
			} else if ohlcDiagnosticCount == 5 {
				warnings = append(warnings, "invalid_ohlc_bar_details_truncated")
			}
			ohlcDiagnosticCount++
			continue
		}
		if b.Volume == nil || *b.Volume < 0 {
			warnings = append(warnings, "bar_with_unknown_volume_dropped")
			continue
		}
		if b.VolumeUnit != "shares" {
			warnings = append(warnings, "bar_with_nonstandard_volume_unit_dropped")
			continue
		}
		if b.Amount != nil && (!finite(*b.Amount) || *b.Amount < 0) {
			warnings = append(warnings, "bar_with_invalid_amount_dropped")
			continue
		}
		if b.Amount != nil && ((*b.Volume == 0 && *b.Amount > 0) || (*b.Volume > 0 && *b.Amount == 0)) {
			warnings = append(warnings, "bar_amount_volume_zero_mismatch_dropped")
			continue
		}
		// Amount/volume is a traded-price cross-check only when the OHLC prices
		// are unadjusted. On forward/backward-adjusted candles, price factors can
		// legitimately differ from the raw traded-price basis while turnover and
		// volume remain unadjusted; applying this test would drop valid history.
		if b.AdjustmentBasis == model.AdjustmentNone && b.Amount != nil && *b.Amount > 0 && *b.Volume > 0 {
			averagePrice := *b.Amount / float64(*b.Volume)
			if averagePrice < b.Low*0.5 || averagePrice > b.High*2 {
				warnings = append(warnings, "bar_amount_volume_scale_mismatch_dropped")
				continue
			}
		}
		if b.ChangePercent != nil && !finite(*b.ChangePercent) {
			warnings = append(warnings, "bar_with_invalid_change_percent_dropped")
			continue
		}
		switch b.AdjustmentBasis {
		case model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward:
		default:
			warnings = append(warnings, "bar_with_unknown_or_unsupported_adjustment_dropped")
			continue
		}
		if b.Period != "1d" {
			warnings = append(warnings, "bar_with_unsupported_period_dropped")
			continue
		}
		if b.Source == "" {
			b.Source = "unknown"
			b.QualityStatus = model.QualityUnknown
			b.QualityReasons = append(b.QualityReasons, "source_missing")
		}
		b.Symbol = symbol
		if b.Amount == nil {
			if b.QualityStatus == model.QualityValid || b.QualityStatus == "" {
				b.QualityStatus = model.QualityDegraded
			}
			// AmountUnit describes the actual amount value. A provider cannot assert CNY when amount is null.
			b.AmountUnit = "unknown"
			b.QualityReasons = appendReasonUnique(b.QualityReasons, "amount_missing_from_provider")
			b.QualityReasons = appendReasonUnique(b.QualityReasons, "amount_unit_unknown_because_amount_missing")
		} else if b.AmountUnit != "CNY" {
			warnings = append(warnings, "bar_with_nonstandard_amount_unit_dropped")
			continue
		}
		if b.QualityStatus == "" {
			b.QualityStatus = model.QualityUnknown
			b.QualityReasons = appendReasonUnique(b.QualityReasons, "quality_status_missing")
		}
		switch b.QualityStatus {
		case model.QualityValid, model.QualityDegraded, model.QualityStale, model.QualityUnknown:
		case model.QualityInvalid:
			warnings = append(warnings, "invalid_quality_status_bar_dropped")
			continue
		default:
			warnings = append(warnings, "unsupported_quality_status_bar_dropped")
			continue
		}
		seen[b.TradeDate] = true
		valid = append(valid, b)
	}
	sort.Slice(valid, func(i, j int) bool { return valid[i].TradeDate < valid[j].TradeDate })
	if limit > 0 && len(valid) > limit {
		valid = valid[len(valid)-limit:]
		warnings = append(warnings, "bars_truncated_to_limit")
	}
	return valid, unique(warnings)
}

// HasQualityDegradingBarWarnings distinguishes data-quality warnings from the
// informational notice emitted when a response is capped to the requested limit.
func HasQualityDegradingBarWarnings(warnings []string) bool {
	for _, warning := range warnings {
		if warning == "" || warning == "bars_truncated_to_limit" {
			continue
		}
		return true
	}
	return false
}

func finite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func appendReasonUnique(in []string, value string) []string {
	for _, existing := range in {
		if existing == value {
			return in
		}
	}
	return append(in, value)
}
func unique(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, v := range in {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
