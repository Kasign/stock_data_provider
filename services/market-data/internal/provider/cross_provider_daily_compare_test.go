package provider

import (
	"math"
	"testing"
)

func crossProviderAmount(value float64) *float64 { return &value }

func crossProviderSnapshot(provider, adjustment, quality string, bars ...CrossProviderDailyBar) CrossProviderDailySnapshot {
	return CrossProviderDailySnapshot{
		Provider: provider, SourceVersion: provider + "-v1", Symbol: "600036.SH",
		AdjustmentBasis: adjustment, QualityStatus: quality, Bars: bars,
	}
}

func crossProviderBar(date string) CrossProviderDailyBar {
	return CrossProviderDailyBar{
		TradeDate: date, Open: 42.3, High: 43.06, Low: 42.04, Close: 42.99,
		Volume: 86728303, VolumeUnit: "shares", VolumeGranularity: 1, AmountGranularity: 0.0001,
	}
}

func crossProviderAdjustedSnapshot(provider, method, refDate string, bars ...CrossProviderDailyBar) CrossProviderDailySnapshot {
	for i := range bars {
		bars[i].AdjustmentMethod = method
		bars[i].AdjustmentRefDate = refDate
	}
	return crossProviderSnapshot(provider, "backward_adjusted", "valid", bars...)
}

func TestCompareCrossProviderDailyBarsRequiresComparableIdentity(t *testing.T) {
	left := crossProviderSnapshot("sina", "none", "valid", crossProviderBar("2026-10-29"))
	right := crossProviderSnapshot("tencent", "forward", "valid", crossProviderBar("2026-10-29"))
	if got := CompareCrossProviderDailyBars(left, right); got.Status != CrossProviderDailyNotComparable || got.Reason != "adjustment_basis_mismatch" {
		t.Fatalf("unexpected result: %+v", got)
	}
	right.AdjustmentBasis = "none"
	right.Symbol = "600519.SH"
	if got := CompareCrossProviderDailyBars(left, right); got.Reason != "symbol_mismatch" {
		t.Fatalf("expected symbol mismatch, got %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsRejectsDegradedBaseline(t *testing.T) {
	left := crossProviderSnapshot("sina", "none", "valid", crossProviderBar("2026-10-29"))
	right := crossProviderSnapshot("tencent", "none", "degraded", crossProviderBar("2026-10-29"))
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyNotComparable || got.Reason != "quality_status_must_be_valid" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsComparesCommonContract(t *testing.T) {
	leftBar, rightBar := crossProviderBar("2026-10-29"), crossProviderBar("2026-10-29")
	rightBar.Volume += 1
	leftBar.Amount, rightBar.Amount = crossProviderAmount(1000000), crossProviderAmount(1000000.0001)
	leftBar.AmountUnit, rightBar.AmountUnit = "CNY", "CNY"
	left := crossProviderSnapshot("sina", "none", "valid", leftBar)
	right := crossProviderSnapshot("tencent", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyMatched {
		t.Fatalf("expected matched, got %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsUsesRawVolumeGranularity(t *testing.T) {
	leftBar := crossProviderBar("2026-10-29")
	rightBar := crossProviderBar("2026-10-29")
	leftBar.Volume, rightBar.Volume = 100000, 100096
	leftBar.VolumeGranularity, rightBar.VolumeGranularity = 100, 1
	amount := 1000000.0
	leftBar.Amount, rightBar.Amount = &amount, &amount
	leftBar.AmountUnit, rightBar.AmountUnit = "CNY", "CNY"
	leftBar.AmountGranularity, rightBar.AmountGranularity = 0.0001, 0.0001
	left := crossProviderSnapshot("pytdx", "none", "valid", leftBar)
	right := crossProviderSnapshot("baostock", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyMatched {
		t.Fatalf("one TDX lot (100 shares) must be within source granularity: %+v", got)
	}
	for _, field := range got.Fields {
		if field.Field == "2026-10-29.volume" && (field.Status != "matched" || field.Tolerance != 100) {
			t.Fatalf("volume comparison must expose 100-share source granularity: %+v", field)
		}
	}
	right.Bars[0].Volume = 100101
	got = CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyMismatch {
		t.Fatalf("difference above one TDX lot must remain mismatch: %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsRejectsDifferenceAboveSharePrecision(t *testing.T) {
	leftBar := crossProviderBar("2026-10-29")
	rightBar := crossProviderBar("2026-10-29")
	leftBar.Volume, rightBar.Volume = 100000, 100002
	leftBar.VolumeGranularity, rightBar.VolumeGranularity = 1, 1
	amount := 1000000.0
	leftBar.Amount, rightBar.Amount = &amount, &amount
	leftBar.AmountUnit, rightBar.AmountUnit = "CNY", "CNY"
	left := crossProviderSnapshot("sina", "none", "valid", leftBar)
	right := crossProviderSnapshot("tencent", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyMismatch {
		t.Fatalf("two share-precision sources must not tolerate a two-share difference: %+v", got)
	}
	for _, field := range got.Fields {
		if field.Field == "2026-10-29.volume" && field.Tolerance != 1 {
			t.Fatalf("share-precision comparison must use one-share tolerance: %+v", field)
		}
	}
}
func TestCrossProviderAmountGranularityUsesProviderContracts(t *testing.T) {
	tdxAmount := 2672021248.0
	tdxStep := CrossProviderAmountGranularity("pytdx", "CNY", "amount", &tdxAmount)
	if tdxStep != 256 {
		t.Fatalf("expected float32 ULP 256 CNY at 2.67B, got %v", tdxStep)
	}
	baoStep := CrossProviderAmountGranularity("baostock", "CNY", "amount", &tdxAmount)
	if baoStep != 0.0001 {
		t.Fatalf("expected BaoStock 4-decimal precision, got %v", baoStep)
	}
	if got := CrossProviderAmountGranularity("unknown", "CNY", "amount", &tdxAmount); got != 0 {
		t.Fatalf("unknown source precision must remain unknown, got %v", got)
	}
}

func TestCompareCrossProviderDailyBarsUsesAmountSourcePrecision(t *testing.T) {
	tests := []struct {
		name          string
		tdxAmount     float64
		baoAmount     float64
		wantStatus    string
		wantTolerance float64
	}{
		{name: "600036 large amount float32 rounding", tdxAmount: 2672021248, baoAmount: 2672021127.26, wantStatus: CrossProviderDailyMatched, wantTolerance: 128.00005},
		{name: "002982 mid amount float32 rounding", tdxAmount: 190000000, baoAmount: 189999992.1, wantStatus: CrossProviderDailyMatched, wantTolerance: 8.00005},
		{name: "beyond float32 rounding boundary", tdxAmount: 2672021248, baoAmount: 2672021119.9, wantStatus: CrossProviderDailyMismatch, wantTolerance: 128.00005},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tdxBar, baoBar := crossProviderBar("2025-07-07"), crossProviderBar("2025-07-07")
			tdxBar.Amount, baoBar.Amount = &tt.tdxAmount, &tt.baoAmount
			tdxBar.AmountUnit, baoBar.AmountUnit = "CNY", "CNY"
			tdxBar.AmountGranularity = CrossProviderAmountGranularity("pytdx", "CNY", "amount", &tt.tdxAmount)
			baoBar.AmountGranularity = CrossProviderAmountGranularity("baostock", "CNY", "amount", &tt.baoAmount)
			left := crossProviderSnapshot("pytdx", "none", "valid", tdxBar)
			right := crossProviderSnapshot("baostock", "none", "valid", baoBar)
			got := CompareCrossProviderDailyBars(left, right)
			if got.Status != tt.wantStatus {
				t.Fatalf("status = %s, want %s: %+v", got.Status, tt.wantStatus, got)
			}
			for _, field := range got.Fields {
				if field.Field == "2025-07-07.amount" && field.Tolerance != tt.wantTolerance {
					t.Fatalf("amount tolerance = %v, want %v", field.Tolerance, tt.wantTolerance)
				}
			}
		})
	}
}

func TestCompareCrossProviderDailyBarsKeepsUnknownAmountPrecisionPartial(t *testing.T) {
	leftBar, rightBar := crossProviderBar("2025-07-07"), crossProviderBar("2025-07-07")
	amount := 1000000.0
	leftBar.Amount, rightBar.Amount = &amount, &amount
	leftBar.AmountUnit, rightBar.AmountUnit = "CNY", "CNY"
	leftBar.AmountGranularity = 0
	left := crossProviderSnapshot("unknown-left", "none", "valid", leftBar)
	right := crossProviderSnapshot("baostock", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyPartial {
		t.Fatalf("unknown amount precision must not be accepted as matched: %+v", got)
	}
}
func TestCompareCrossProviderDailyBarsRequiresVolumeGranularity(t *testing.T) {
	leftBar := crossProviderBar("2026-10-29")
	rightBar := crossProviderBar("2026-10-29")
	leftBar.VolumeGranularity = 0
	left := crossProviderSnapshot("left", "none", "valid", leftBar)
	right := crossProviderSnapshot("right", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyNotComparable || got.Reason != "volume_granularity_missing" {
		t.Fatalf("missing source volume granularity must be not_comparable: %+v", got)
	}
}
func TestCompareCrossProviderDailyBarsKeepsMissingAmountPartial(t *testing.T) {
	leftBar, rightBar := crossProviderBar("2026-10-29"), crossProviderBar("2026-10-29")
	left := crossProviderSnapshot("sina", "none", "valid", leftBar)
	right := crossProviderSnapshot("tencent", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyPartial {
		t.Fatalf("expected partial when amount is unavailable, got %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsDoesNotTreatDifferentDatesAsMismatch(t *testing.T) {
	left := crossProviderSnapshot("sina", "none", "valid", crossProviderBar("2026-10-28"))
	right := crossProviderSnapshot("tencent", "none", "valid", crossProviderBar("2026-10-29"))
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyNotComparable || got.Reason != "trade_date_set_mismatch" {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsDetectsFieldMismatch(t *testing.T) {
	leftBar, rightBar := crossProviderBar("2026-10-29"), crossProviderBar("2026-10-29")
	rightBar.Close += 0.02
	left := crossProviderSnapshot("sina", "none", "valid", leftBar)
	right := crossProviderSnapshot("tencent", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyMismatch {
		t.Fatalf("expected mismatch, got %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsRejectsInvalidMarketValues(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CrossProviderDailyBar)
		reason string
	}{
		{name: "zero close", mutate: func(bar *CrossProviderDailyBar) { bar.Close = 0 }, reason: "invalid_ohlc_value"},
		{name: "nan open", mutate: func(bar *CrossProviderDailyBar) { bar.Open = math.NaN() }, reason: "invalid_ohlc_value"},
		{name: "infinite high", mutate: func(bar *CrossProviderDailyBar) { bar.High = math.Inf(1) }, reason: "invalid_ohlc_value"},
		{name: "invalid high low relation", mutate: func(bar *CrossProviderDailyBar) { bar.High = 40 }, reason: "invalid_ohlc_relationship"},
		{name: "negative volume", mutate: func(bar *CrossProviderDailyBar) { bar.Volume = -1 }, reason: "negative_volume"},
		{name: "negative amount", mutate: func(bar *CrossProviderDailyBar) { bar.Amount = crossProviderAmount(-1); bar.AmountUnit = "CNY" }, reason: "invalid_amount_value"},
		{name: "nan amount", mutate: func(bar *CrossProviderDailyBar) { bar.Amount = crossProviderAmount(math.NaN()); bar.AmountUnit = "CNY" }, reason: "invalid_amount_value"},
		{name: "infinite amount", mutate: func(bar *CrossProviderDailyBar) {
			bar.Amount = crossProviderAmount(math.Inf(1))
			bar.AmountUnit = "CNY"
		}, reason: "invalid_amount_value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			leftBar, rightBar := crossProviderBar("2026-10-29"), crossProviderBar("2026-10-29")
			test.mutate(&rightBar)
			left := crossProviderSnapshot("sina", "none", "valid", leftBar)
			right := crossProviderSnapshot("tencent", "none", "valid", rightBar)
			got := CompareCrossProviderDailyBars(left, right)
			if got.Status != CrossProviderDailyNotComparable || got.Reason != "right_2026-10-29_"+test.reason {
				t.Fatalf("unexpected result: %+v", got)
			}
		})
	}
}

func TestCompareCrossProviderDailyBarsRejectsEmptyAndInvalidDateSets(t *testing.T) {
	left := crossProviderSnapshot("sina", "none", "valid")
	right := crossProviderSnapshot("tencent", "none", "valid")
	if got := CompareCrossProviderDailyBars(left, right); got.Status != CrossProviderDailyNotComparable || got.Reason != "trade_date_set_mismatch" {
		t.Fatalf("empty bars must be not comparable, got %+v", got)
	}
	left.Bars = []CrossProviderDailyBar{crossProviderBar("2026-02-30")}
	right.Bars = []CrossProviderDailyBar{crossProviderBar("2026-02-30")}
	if got := CompareCrossProviderDailyBars(left, right); got.Status != CrossProviderDailyNotComparable || got.Reason != "left_invalid_trade_date" {
		t.Fatalf("invalid calendar date must be not comparable, got %+v", got)
	}
	left.Bars = []CrossProviderDailyBar{crossProviderBar("2026-10-29"), crossProviderBar("2026-10-29")}
	right.Bars = []CrossProviderDailyBar{crossProviderBar("2026-10-29")}
	if got := CompareCrossProviderDailyBars(left, right); got.Status != CrossProviderDailyNotComparable || got.Reason != "left_duplicate_trade_date" {
		t.Fatalf("duplicate trade date must be not comparable, got %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsRejectsDifferentAdjustedMethods(t *testing.T) {
	left := crossProviderAdjustedSnapshot("eastmoney", "eastmoney_fqt_2", "2026-11-03", crossProviderBar("2026-10-29"))
	right := crossProviderAdjustedSnapshot("baostock", "baostock_pct_change", "2026-11-03", crossProviderBar("2026-10-29"))
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyNotComparable || got.Reason != "adjustment_method_mismatch" {
		t.Fatalf("different adjusted methods must not be compared: %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsRequiresAdjustedReferenceDate(t *testing.T) {
	left := crossProviderAdjustedSnapshot("eastmoney", "same_method", "2026-11-03", crossProviderBar("2026-10-29"))
	right := crossProviderAdjustedSnapshot("other", "same_method", "", crossProviderBar("2026-10-29"))
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyNotComparable || got.Reason != "right_adjustment_reference_date_missing" {
		t.Fatalf("missing adjusted reference date must be rejected: %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsRequiresSameAdjustedReferenceDate(t *testing.T) {
	left := crossProviderAdjustedSnapshot("eastmoney", "same_method", "2026-11-03", crossProviderBar("2026-10-29"))
	right := crossProviderAdjustedSnapshot("other", "same_method", "2026-10-30", crossProviderBar("2026-10-29"))
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyNotComparable || got.Reason != "adjustment_reference_date_mismatch" {
		t.Fatalf("different adjusted reference dates must be rejected: %+v", got)
	}
}

func TestCompareCrossProviderDailyBarsRejectsInconsistentRowAdjustmentMetadata(t *testing.T) {
	left := crossProviderAdjustedSnapshot("eastmoney", "same_method", "2026-11-03", crossProviderBar("2026-10-28"), crossProviderBar("2026-10-29"))
	right := crossProviderAdjustedSnapshot("other", "same_method", "2026-11-03", crossProviderBar("2026-10-28"), crossProviderBar("2026-10-29"))
	left.Bars[1].AdjustmentRefDate = "2026-10-30"
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyNotComparable || got.Reason != "left_adjustment_metadata_inconsistent" {
		t.Fatalf("inconsistent row metadata must be rejected: %+v", got)
	}
}
