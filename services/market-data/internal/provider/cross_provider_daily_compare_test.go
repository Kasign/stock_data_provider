package provider

import "testing"

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
