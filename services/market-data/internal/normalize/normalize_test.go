package normalize

import (
	"math"
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/validation"
)

func TestNormalizeSymbol(t *testing.T) {
	tests := map[string]string{"600519": "600519.SH", "000001": "000001.SZ", "830799": "830799.BJ", "600519.SH": "600519.SH"}
	for in, want := range tests {
		got, err := NormalizeSymbol(in)
		if err != nil || got != want {
			t.Fatalf("NormalizeSymbol(%s)=(%s,%v), want %s", in, got, err, want)
		}
	}
	if _, err := NormalizeSymbol("AAPL"); err == nil {
		t.Fatal("expected invalid symbol")
	}
	for _, raw := range []string{"900901", "900901.SH", "200001.SZ", "600519.SZ", "000001.SH", "830799.SZ"} {
		if _, err := NormalizeSymbol(raw); err == nil {
			t.Errorf("NormalizeSymbol(%q) should reject unsupported B-share or mismatched exchange", raw)
		}
	}
}
func TestValidateAndSortBars(t *testing.T) {
	v1 := int64(100)
	v2 := int64(200)
	bars := []model.DailyBar{{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 12, Low: 9, Close: 11, Volume: &v1, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney"}, {Symbol: "600519.SH", TradeDate: "2026-10-19", Period: "1d", Open: 10, High: 12, Low: 9, Close: 11, Volume: &v2, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney"}, {Symbol: "600519.SH", TradeDate: "2026-10-21", Period: "1d", Open: 10, High: 8, Low: 9, Close: 11, Volume: &v2, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney"}}
	got, warnings := ValidateAndSortBars("600519.SH", bars, "2026-10-19", "2026-10-20", 10)
	if len(got) != 2 {
		t.Fatalf("wanted 2 valid bars, got %d", len(got))
	}
	if got[0].TradeDate != "2026-10-19" || got[1].TradeDate != "2026-10-20" {
		t.Fatalf("bars not sorted: %#v", got)
	}
	if len(warnings) == 0 {
		t.Fatal("expected invalid bar warning")
	}
}

func TestValidateAndSortBarsRejectsMissingInvalidAndMismatchedSymbols(t *testing.T) {
	volume := int64(1000)
	base := model.DailyBar{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume,
		AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney",
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
	}
	mismatched := base
	mismatched.Symbol = "000001.SZ"
	mismatched.TradeDate = "2026-10-21"
	missing := base
	missing.Symbol = ""
	missing.TradeDate = "2026-10-22"
	invalid := base
	invalid.Symbol = "AAPL"
	invalid.TradeDate = "2026-10-23"

	got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{base, mismatched, missing, invalid}, "", "", 10)
	if len(got) != 1 || got[0].TradeDate != base.TradeDate || got[0].Symbol != "600519.SH" {
		t.Fatalf("only the matching security row should survive, got bars=%+v warnings=%v", got, warnings)
	}
	for _, warning := range []string{
		"bar_symbol_mismatch_dropped",
		"bar_with_missing_symbol_dropped",
		"bar_with_invalid_symbol_dropped",
	} {
		if !containsWarning(warnings, warning) {
			t.Errorf("missing warning %q in %v", warning, warnings)
		}
	}
}
func TestInvalidOHLCWarningIncludesDroppedRowContext(t *testing.T) {
	volume := int64(1000)
	bad := model.DailyBar{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 11, High: 10, Low: 9, Close: 9.5, Volume: &volume,
		AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney",
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
	}
	got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{bad}, "", "2026-10-22", 10)
	if len(got) != 0 || !containsWarning(warnings, "invalid_ohlc_bar_dropped") {
		t.Fatalf("invalid OHLC row should be dropped, got bars=%+v warnings=%v", got, warnings)
	}
	wantDetail := "invalid_ohlc_bar_detail:trade_date=2026-10-20,open=11,high=10,low=9,close=9.5"
	if !containsWarning(warnings, wantDetail) {
		t.Fatalf("expected row-level OHLC diagnostic %q, got %v", wantDetail, warnings)
	}
}

func TestBarWarningsDistinguishTruncationFromQualityLoss(t *testing.T) {
	if HasQualityDegradingBarWarnings([]string{"bars_truncated_to_limit"}) {
		t.Fatal("intentional result cap must not degrade otherwise valid bars")
	}
	if !HasQualityDegradingBarWarnings([]string{"invalid_ohlc_bar_dropped", "bars_truncated_to_limit"}) {
		t.Fatal("invalid OHLC data must still degrade the result")
	}
	if !HasQualityDegradingBarWarnings([]string{"provider_unavailable_served_from_mariadb"}) {
		t.Fatal("stale provider fallback must degrade the result")
	}
}

func TestAdjustedBarsDoNotCompareUnadjustedTurnoverToAdjustedOHLC(t *testing.T) {
	volume := int64(1000)
	amount := 100000.0 // implied raw traded price 100; adjusted candle is around 10.
	bars := []model.DailyBar{{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount,
		AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney",
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
	}}
	got, warnings := ValidateAndSortBars("600519.SH", bars, "", "2026-10-21", 10)
	if len(got) != 1 || got[0].Amount == nil || *got[0].Amount != amount {
		t.Fatalf("valid adjusted candle should not be rejected by raw-price cross-check: bars=%+v warnings=%v", got, warnings)
	}
}

func TestUnadjustedBarsStillRejectAmountVolumeScaleMismatch(t *testing.T) {
	volume := int64(1000)
	amount := 100000.0
	bars := []model.DailyBar{{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: &amount,
		AdjustmentBasis: model.AdjustmentNone, Source: "tencent",
		VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid,
	}}
	got, warnings := ValidateAndSortBars("600519.SH", bars, "", "2026-10-21", 10)
	if len(got) != 0 || !containsWarning(warnings, "bar_amount_volume_scale_mismatch_dropped") {
		t.Fatalf("unadjusted candle should fail impossible amount/volume scale: bars=%+v warnings=%v", got, warnings)
	}
}

func TestQuoteRejectsInconsistentPriceScale(t *testing.T) {
	price, change, pct := 131331.0, -1857.0, -1.394
	open, high, low, prev := 133188.0, 133198.0, 131200.0, 133188.0
	volume := int64(1558000)
	amount := 2054895500.0
	q := &model.Quote{Symbol: "600519.SH", Source: "eastmoney", CurrentPrice: &price, Change: &change, ChangePercent: &pct,
		Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &volume, Amount: &amount,
		VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid}
	if err := ValidateQuote(q); err == nil {
		t.Fatal("price/amount scale mismatch must be rejected instead of guessed and rescaled")
	}
}

func TestValidateQuoteRejectsInconsistentChangeFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*model.Quote)
	}{
		{
			name: "change amount mismatch",
			mutate: func(q *model.Quote) {
				q.Change = ptrFloat(1.5)
			},
		},
		{
			name: "change percentage mismatch",
			mutate: func(q *model.Quote) {
				q.ChangePercent = ptrFloat(10)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			q := validQuoteForNormalizationTest()
			tc.mutate(q)
			if err := ValidateQuote(q); err == nil {
				t.Fatal("inconsistent change fields must be rejected")
			}
		})
	}
}

func TestValidateQuoteAllowsRoundedChangePercentage(t *testing.T) {
	q := validQuoteForNormalizationTest()
	q.ChangePercent = ptrFloat(11.1) // displayed percentage may be rounded to one decimal
	if err := ValidateQuote(q); err != nil {
		t.Fatalf("provider percentage rounding within tolerance should pass: %v", err)
	}
}

func TestValidateQuoteAllowsMissingAmountAsDegradedWithoutClaimingCNY(t *testing.T) {
	q := validQuoteForNormalizationTest()
	q.Amount = nil
	q.AmountUnit = "CNY" // a provider may have hard-coded this despite returning no amount
	q.QualityStatus = model.QualityDegraded
	if err := ValidateQuote(q); err != nil {
		t.Fatalf("missing amount should not invalidate known price/volume fields: %v", err)
	}
	if q.AmountUnit != "unknown" || !containsWarning(q.QualityReasons, "amount_missing_from_provider") {
		t.Fatalf("missing amount unit must be explicit: %+v", q)
	}
}

func TestValidateQuoteRejectsNonFiniteOptionalValues(t *testing.T) {
	q := validQuoteForNormalizationTest()
	nan := math.NaN()
	q.ChangePercent = &nan
	if err := ValidateQuote(q); err == nil {
		t.Fatal("non-finite change percent must be rejected before JSON encoding")
	}
}

func TestValidateAndSortBarsRejectsNonFiniteChangePercentAndDegradesMissingAmount(t *testing.T) {
	volume := int64(1000)
	nan := math.NaN()
	bars := []model.DailyBar{
		{Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: nil, ChangePercent: nil, AdjustmentBasis: model.AdjustmentForward, Source: "tencent", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
		{Symbol: "600519.SH", TradeDate: "2026-10-19", Period: "1d", Open: 10, High: 11, Low: 9, Close: 10.5, Volume: &volume, Amount: ptrFloat(10000), ChangePercent: &nan, AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid},
	}
	got, warnings := ValidateAndSortBars("600519.SH", bars, "", "", 10)
	if len(got) != 1 || got[0].TradeDate != "2026-10-20" {
		t.Fatalf("unexpected normalized bars: %+v", got)
	}
	if got[0].QualityStatus != model.QualityDegraded {
		t.Fatalf("missing amount must degrade quality, got %s", got[0].QualityStatus)
	}
	if got[0].Amount != nil || got[0].AmountUnit != "unknown" || !containsWarning(got[0].QualityReasons, "amount_missing_from_provider") {
		t.Fatalf("missing amount should preserve usable OHLC/volume while marking amount unknown: %+v", got[0])
	}
	if !containsWarning(warnings, "bar_with_invalid_change_percent_dropped") {
		t.Fatalf("missing invalid percentage warning: %v", warnings)
	}
}

func TestValidateAndSortBarsRejectsVolumeAmountAndContractBoundaries(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*model.DailyBar)
		wantWarning string
	}{
		{
			name:        "missing volume",
			mutate:      func(bar *model.DailyBar) { bar.Volume = nil },
			wantWarning: "bar_with_unknown_volume_dropped",
		},
		{
			name:        "negative volume",
			mutate:      func(bar *model.DailyBar) { bar.Volume = ptrInt64(-1) },
			wantWarning: "bar_with_unknown_volume_dropped",
		},
		{
			name:        "non share volume unit",
			mutate:      func(bar *model.DailyBar) { bar.VolumeUnit = "hands" },
			wantWarning: "bar_with_nonstandard_volume_unit_dropped",
		},
		{
			name:        "negative amount",
			mutate:      func(bar *model.DailyBar) { bar.Amount = ptrFloat(-1) },
			wantWarning: "bar_with_invalid_amount_dropped",
		},
		{
			name:        "zero amount with nonzero volume",
			mutate:      func(bar *model.DailyBar) { bar.Amount = ptrFloat(0) },
			wantWarning: "bar_amount_volume_zero_mismatch_dropped",
		},
		{
			name:        "nonzero amount with zero volume",
			mutate:      func(bar *model.DailyBar) { bar.Volume = ptrInt64(0) },
			wantWarning: "bar_amount_volume_zero_mismatch_dropped",
		},
		{
			name:        "non CNY amount unit",
			mutate:      func(bar *model.DailyBar) { bar.AmountUnit = "USD" },
			wantWarning: "bar_with_nonstandard_amount_unit_dropped",
		},
		{
			name:        "unknown adjustment basis",
			mutate:      func(bar *model.DailyBar) { bar.AdjustmentBasis = model.AdjustmentUnknown },
			wantWarning: "bar_with_unknown_or_unsupported_adjustment_dropped",
		},
		{
			name:        "unsupported period",
			mutate:      func(bar *model.DailyBar) { bar.Period = "1m" },
			wantWarning: "bar_with_unsupported_period_dropped",
		},
		{
			name:        "non finite change percent",
			mutate:      func(bar *model.DailyBar) { bar.ChangePercent = ptrFloat(math.NaN()) },
			wantWarning: "bar_with_invalid_change_percent_dropped",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			bar := validDailyBarForNormalizationTest()
			tc.mutate(&bar)
			got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{bar}, "2026-10-20", "2026-10-20", 10)
			if len(got) != 0 || !containsWarning(warnings, tc.wantWarning) {
				t.Fatalf("invalid bar must be rejected with %q: bars=%+v warnings=%v", tc.wantWarning, got, warnings)
			}
		})
	}
}

func TestValidateAndSortBarsDoesNotOverrideInvalidQualityWhenSourceIsMissing(t *testing.T) {
	bar := validDailyBarForNormalizationTest()
	bar.Source = ""
	bar.QualityStatus = model.QualityInvalid
	got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{bar}, "2026-10-20", "2026-10-20", 10)
	if len(got) != 0 || !containsWarning(warnings, "invalid_quality_status_bar_dropped") {
		t.Fatalf("missing source must not downgrade invalid quality into an accepted unknown bar: bars=%+v warnings=%v", got, warnings)
	}
}

func TestValidateAndSortBarsUsesInclusiveDateBoundsDeduplicatesAndKeepsNewestLimit(t *testing.T) {
	before := validDailyBarForNormalizationTest()
	before.TradeDate = "2026-10-19"
	first := validDailyBarForNormalizationTest()
	first.TradeDate = "2026-10-20"
	duplicate := first
	next := validDailyBarForNormalizationTest()
	next.TradeDate = "2026-10-21"
	last := validDailyBarForNormalizationTest()
	last.TradeDate = "2026-10-22"

	got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{before, first, duplicate, next, last}, "2026-10-20", "2026-10-22", 2)
	if len(got) != 2 || got[0].TradeDate != "2026-10-21" || got[1].TradeDate != "2026-10-22" {
		t.Fatalf("date bounds must be inclusive and limit must retain newest bars: %+v", got)
	}
	if !containsWarning(warnings, "duplicate_trade_date_dropped") || !containsWarning(warnings, "bars_truncated_to_limit") {
		t.Fatalf("expected duplicate and truncation evidence, got %v", warnings)
	}
}

func validDailyBarForNormalizationTest() model.DailyBar {
	volume := int64(1000)
	amount := 10000.0
	return model.DailyBar{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: &volume, VolumeUnit: "shares", Amount: &amount, AmountUnit: "CNY",
		AdjustmentBasis: model.AdjustmentNone, Source: "eastmoney", QualityStatus: model.QualityValid,
	}
}

func ptrInt64(value int64) *int64 { return &value }

func validQuoteForNormalizationTest() *model.Quote {
	now := time.Now().UTC()
	price, change, pct, open, high, low, prev, amount := 10.0, 1.0, 100.0/9.0, 9.0, 11.0, 8.0, 9.0, 100000.0
	volume := int64(10000)
	return &model.Quote{Symbol: "600519.SH", Source: "eastmoney", CurrentPrice: &price, Change: &change, ChangePercent: &pct, Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &volume, Amount: &amount, QuoteTime: &now, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid}
}

func ptrFloat(value float64) *float64 { return &value }
func containsWarning(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestHandsToSharesRejectsInvalidAndOverflowValues(t *testing.T) {
	got, err := HandsToShares(34136)
	if err != nil || got != 3413600 {
		t.Fatalf("HandsToShares(34136)=(%d,%v), want 3413600", got, err)
	}
	for _, value := range []float64{-1, 1.5, math.NaN(), math.Inf(1), math.MaxFloat64} {
		if _, err := HandsToShares(value); err == nil {
			t.Errorf("HandsToShares(%v) should fail", value)
		}
	}
}

func TestRawSharesToInt64RejectsInvalidAndOverflowValues(t *testing.T) {
	got, err := RawSharesToInt64(4421640)
	if err != nil || got != 4421640 {
		t.Fatalf("RawSharesToInt64=(%d,%v), want 4421640", got, err)
	}
	for _, value := range []float64{-1, 1.5, math.NaN(), math.Inf(1), math.MaxFloat64, float64(math.MaxInt64)} {
		if _, err := RawSharesToInt64(value); err == nil {
			t.Errorf("RawSharesToInt64(%v) should fail", value)
		}
	}
}

func TestValidateAndSortBarsRejectsNonCanonicalDates(t *testing.T) {
	volume := int64(1000)
	bars := []model.DailyBar{{
		Symbol: "600519.SH", TradeDate: "2026-10-2", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10.5,
		Volume: &volume, AdjustmentBasis: model.AdjustmentForward,
		Source: "eastmoney", VolumeUnit: "shares", QualityStatus: model.QualityValid,
	}}
	got, warnings := ValidateAndSortBars("600519.SH", bars, "", "2026-10-31", 10)
	if len(got) != 0 || !containsWarning(warnings, "invalid_trade_date_dropped") {
		t.Fatalf("non-canonical trade date must be dropped: bars=%+v warnings=%v", got, warnings)
	}

	if got, warnings := ValidateAndSortBars("600519.SH", bars, "2026-1-01", "2026-10-31", 10); len(got) != 0 || !containsWarning(warnings, "invalid_start_date") {
		t.Fatalf("non-canonical start date must be rejected: bars=%+v warnings=%v", got, warnings)
	}
	if got, warnings := ValidateAndSortBars("600519.SH", bars, "", "2026-10-2", 10); len(got) != 0 || !containsWarning(warnings, "invalid_end_date") {
		t.Fatalf("non-canonical end date must be rejected: bars=%+v warnings=%v", got, warnings)
	}
}

func TestValidateQuoteWithReportPreservesLegacyAmountMutation(t *testing.T) {
	quote := validQuoteForNormalizationTest()
	quote.Amount = nil
	quote.AmountUnit = "CNY"
	quote.QualityStatus = model.QualityDegraded
	report, err := ValidateQuoteWithReport(quote)
	if err != nil {
		t.Fatalf("legacy validation should accept degraded quote with missing amount: %v", err)
	}
	if !containsValidationIssue(report.Issues, "amount", "missing_optional") {
		t.Fatalf("expected structured missing amount warning, got %+v", report)
	}
	if quote.AmountUnit != "unknown" || len(quote.QualityReasons) == 0 {
		t.Fatalf("legacy amount-missing mutation was not preserved: %+v", quote)
	}
}

func TestValidateAndSortBarsWithReportPreservesLegacyOutputAndAddsRowIndex(t *testing.T) {
	volume := int64(100)
	valid := model.DailyBar{
		Symbol: "600519.SH", TradeDate: "2026-10-20", Period: "1d",
		Open: 10, High: 11, Low: 9, Close: 10, Volume: &volume,
		VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentForward,
		Source: "eastmoney", QualityStatus: model.QualityValid,
	}
	invalid := valid
	invalid.TradeDate = "2026-10-21"
	invalid.High = 8
	legacyBars, legacyWarnings := ValidateAndSortBars("600519.SH", []model.DailyBar{valid, invalid}, "", "2026-10-22", 10)
	reportedBars, reportedWarnings, report := ValidateAndSortBarsWithReport("600519.SH", []model.DailyBar{valid, invalid}, "", "2026-10-22", 10)
	if len(legacyBars) != len(reportedBars) || len(legacyWarnings) != len(reportedWarnings) {
		t.Fatalf("reporting changed legacy result shape: legacy=%+v/%v reported=%+v/%v", legacyBars, legacyWarnings, reportedBars, reportedWarnings)
	}
	if len(reportedBars) != 1 || reportedBars[0].TradeDate != "2026-10-20" {
		t.Fatalf("legacy filtering behavior changed: %+v", reportedBars)
	}
	found := false
	for _, item := range report.Issues {
		if item.Field == "open,high,low,close" && item.RowIndex != nil && *item.RowIndex == 1 {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected row-indexed OHLC issue, got %+v", report.Issues)
	}
}

func containsValidationIssue(issues []validation.FieldIssue, field, code string) bool {
	for _, item := range issues {
		if item.Field == field && item.Code == code {
			return true
		}
	}
	return false
}
