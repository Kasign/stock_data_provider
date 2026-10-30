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
		Volume: 86728303, VolumeUnit: "shares",
	}
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
	rightBar.Volume += 46
	leftBar.Amount, rightBar.Amount = crossProviderAmount(1000000), crossProviderAmount(1000000.5)
	leftBar.AmountUnit, rightBar.AmountUnit = "CNY", "CNY"
	left := crossProviderSnapshot("sina", "none", "valid", leftBar)
	right := crossProviderSnapshot("tencent", "none", "valid", rightBar)
	got := CompareCrossProviderDailyBars(left, right)
	if got.Status != CrossProviderDailyMatched {
		t.Fatalf("expected matched, got %+v", got)
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
