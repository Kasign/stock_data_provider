package normalize

import (
	"math"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
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
	bars := []model.DailyBar{{TradeDate: "2026-10-20", Period: "1d", Open: 10, High: 12, Low: 9, Close: 11, Volume: &v1, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney"}, {TradeDate: "2026-10-19", Period: "1d", Open: 10, High: 12, Low: 9, Close: 11, Volume: &v2, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney"}, {TradeDate: "2026-10-21", Period: "1d", Open: 10, High: 8, Low: 9, Close: 11, Volume: &v2, VolumeUnit: "shares", AmountUnit: "CNY", AdjustmentBasis: model.AdjustmentForward, Source: "eastmoney"}}
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
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.MaxFloat64} {
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
	for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.MaxFloat64, float64(math.MaxInt64)} {
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
