package normalize

import (
	"math"
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// TestLegacyPythonBridgeAndGoQualityContract records the intentional contract
// difference: legacy Python bars carry unknown unit/adjustment metadata, while
// native Go normalization requires explicit units and a supported price basis.
func TestLegacyPythonBridgeAndGoQualityContract(t *testing.T) {
	t.Run("Go rejects legacy unknown metadata rather than guessing", func(t *testing.T) {
		volume := int64(1000)
		bar := model.DailyBar{
			TradeDate: "2026-10-20", Period: "1d",
			Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: &volume, VolumeUnit: "unknown", AmountUnit: "unknown",
			AdjustmentBasis: model.AdjustmentUnknown, Source: "legacy-python",
			QualityStatus: model.QualityUnknown,
		}
		got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{bar}, "", "", 10)
		if len(got) != 0 {
			t.Fatalf("legacy row with unverified units/adjustment must not enter normalized Go series: %+v", got)
		}
		if !containsWarning(warnings, "bar_with_nonstandard_volume_unit_dropped") {
			t.Fatalf("expected explicit unit-contract failure, warnings=%v", warnings)
		}
	})

	t.Run("missing amount remains null and degraded", func(t *testing.T) {
		volume := int64(1000)
		bar := model.DailyBar{
			TradeDate: "2026-10-20", Period: "1d",
			Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: &volume, VolumeUnit: "shares", AmountUnit: "CNY",
			AdjustmentBasis: model.AdjustmentNone, Source: "tencent",
			QualityStatus: model.QualityValid,
		}
		got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{bar}, "", "", 10)
		if len(got) != 1 {
			t.Fatalf("expected price/volume row to remain usable, got=%+v warnings=%v", got, warnings)
		}
		if got[0].Amount != nil || got[0].AmountUnit != "unknown" || got[0].QualityStatus != model.QualityDegraded {
			t.Fatalf("missing amount must not be fabricated or labeled CNY: %+v", got[0])
		}
		if !containsWarning(got[0].QualityReasons, "amount_missing_from_provider") {
			t.Fatalf("missing amount reason not recorded: %+v", got[0].QualityReasons)
		}
	})

	t.Run("invalid OHLC and duplicate dates are visible", func(t *testing.T) {
		volume := int64(1000)
		base := model.DailyBar{
			TradeDate: "2026-10-20", Period: "1d",
			Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: &volume, Amount: ptrFloat(10000), VolumeUnit: "shares", AmountUnit: "CNY",
			AdjustmentBasis: model.AdjustmentNone, Source: "eastmoney",
			QualityStatus: model.QualityValid,
		}
		bad := base
		bad.High = 8
		duplicate := base
		got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{bad, base, duplicate}, "", "", 10)
		if len(got) != 1 {
			t.Fatalf("expected invalid OHLC and duplicate date to be excluded, got=%+v warnings=%v", got, warnings)
		}
		if !containsWarning(warnings, "invalid_ohlc_bar_dropped") || !containsWarning(warnings, "duplicate_trade_date_dropped") {
			t.Fatalf("expected both quality reasons, warnings=%v", warnings)
		}
	})

	t.Run("non-finite values and impossible amount scale are rejected", func(t *testing.T) {
		nan := math.NaN()
		q := validQuoteForNormalizationTest()
		q.ChangePercent = &nan
		if err := ValidateQuote(q); err == nil {
			t.Fatal("non-finite quote field must be rejected")
		}

		volume := int64(1000)
		amount := 100000.0
		bar := model.DailyBar{
			TradeDate: "2026-10-20", Period: "1d",
			Open: 10, High: 11, Low: 9, Close: 10.5,
			Volume: &volume, Amount: &amount, VolumeUnit: "shares", AmountUnit: "CNY",
			AdjustmentBasis: model.AdjustmentNone, Source: "tencent",
			QualityStatus: model.QualityValid,
		}
		got, warnings := ValidateAndSortBars("600519.SH", []model.DailyBar{bar}, "", "", 10)
		if len(got) != 0 || !containsWarning(warnings, "bar_amount_volume_scale_mismatch_dropped") {
			t.Fatalf("unadjusted amount/volume scale mismatch must be rejected: bars=%+v warnings=%v", got, warnings)
		}
	})
}
