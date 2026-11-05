package service

import (
	"testing"
	"time"

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
)

func TestRebuildAdjustedDailyBarsPrefersMatchingFactorBasis(t *testing.T) {
	available := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	observed := available.Add(time.Hour)
	raw := []model.DailyBar{
		{Symbol: "600036.SH", TradeDate: "2025-06-30", Period: "1d", Open: 10, High: 10, Low: 10, Close: 10, Source: "baostock", SourceVersion: "baostock-test-v1", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid},
		{Symbol: "600036.SH", TradeDate: "2025-07-01", Period: "1d", Open: 20, High: 20, Low: 20, Close: 20, Source: "baostock", SourceVersion: "baostock-test-v1", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid},
		{Symbol: "600036.SH", TradeDate: "2025-07-02", Period: "1d", Open: 30, High: 30, Low: 30, Close: 30, Source: "baostock", SourceVersion: "baostock-test-v1", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid},
	}
	factor := func(date string, value float64, factorBasis model.AdjustmentFactorBasis) model.AdjustmentFactor {
		return model.AdjustmentFactor{
			Symbol:           "600036.SH",
			TradeDate:        date,
			Factor:           value,
			Basis:            factorBasis,
			Source:           "baostock",
			SourceVersion:    "baostock-test-v1",
			FetchedAt:        available,
			AvailableAt:      &available,
			SystemObservedAt: &observed,
			QualityStatus:    model.QualityValid,
		}
	}
	factors := []model.AdjustmentFactor{
		factor("2025-06-30", 10, model.AdjustmentFactorShared),
		factor("2025-07-01", 20, model.AdjustmentFactorShared),
		factor("2025-06-30", 2, model.AdjustmentFactorForward),
		factor("2025-07-01", 4, model.AdjustmentFactorForward),
		factor("2025-06-30", 5, model.AdjustmentFactorBackward),
		factor("2025-07-01", 10, model.AdjustmentFactorBackward),
	}
	forward, err := rebuildAdjustedDailyBars(raw, factors, model.AdjustmentForward, "2025-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if forward[0].Close != 5 || forward[1].Close != 20 || forward[2].Close != 30 {
		t.Fatalf("forward rebuild must carry the latest event factor across dates: got closes %.4f, %.4f, %.4f", forward[0].Close, forward[1].Close, forward[2].Close)
	}
	backward, err := rebuildAdjustedDailyBars(raw, factors, model.AdjustmentBackward, "2025-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if backward[0].Close != 50 || backward[1].Close != 200 || backward[2].Close != 300 {
		t.Fatalf("backward rebuild must carry the latest event factor across dates: got closes %.4f, %.4f, %.4f", backward[0].Close, backward[1].Close, backward[2].Close)
	}
}

func TestRebuildAdjustedDailyBarsKeepsSharedFactorFallback(t *testing.T) {
	available := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	observed := available.Add(time.Hour)
	raw := []model.DailyBar{{Symbol: "600036.SH", TradeDate: "2025-07-01", Open: 10, High: 10, Low: 10, Close: 10, Source: "tushare", SourceVersion: "tushare-test-v1", AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid}}
	factors := []model.AdjustmentFactor{{
		Symbol:           "600036.SH",
		TradeDate:        "2025-07-01",
		Factor:           2,
		Source:           "tushare",
		SourceVersion:    "tushare-test-v1",
		FetchedAt:        available,
		AvailableAt:      &available,
		SystemObservedAt: &observed,
		QualityStatus:    model.QualityValid,
	}}
	got, err := rebuildAdjustedDailyBars(raw, factors, model.AdjustmentForward, "2025-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Close != 10 {
		t.Fatalf("shared factor fallback produced close %.4f, want 10", got[0].Close)
	}
}


func TestRebuildAdjustedDailyBarsRejectsCrossProviderOrSymbolFactors(t *testing.T) {
	available := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	observed := available.Add(time.Hour)
	raw := []model.DailyBar{{
		Symbol: "600036.SH", TradeDate: "2025-07-01", Open: 10, High: 10, Low: 10, Close: 10,
		Source: "eastmoney", SourceVersion: "eastmoney-test-v1", AdjustmentBasis: model.AdjustmentNone,
	}}
	factor := model.AdjustmentFactor{
		Symbol: "600036.SH", TradeDate: "2025-07-01", Factor: 2, Basis: model.AdjustmentFactorForward,
		Source: "baostock", SourceVersion: "baostock-test-v1", FetchedAt: available,
		AvailableAt: &available, SystemObservedAt: &observed, QualityStatus: model.QualityValid,
	}
	if _, err := rebuildAdjustedDailyBarsPIT(raw, []model.AdjustmentFactor{factor}, model.AdjustmentForward, "2025-07-01"); err == nil {
		t.Fatal("cross-Provider adjustment factors must never be applied to raw bars")
	}
	factor.Source, factor.SourceVersion = "eastmoney", "eastmoney-test-v1"
	factor.Symbol = "000001.SZ"
	if _, err := rebuildAdjustedDailyBarsPIT(raw, []model.AdjustmentFactor{factor}, model.AdjustmentForward, "2025-07-01"); err == nil {
		t.Fatal("adjustment factors from another security must never be applied")
	}
}

func TestRebuildAdjustedDailyBarsRejectsRawBarsBeforeFirstFactorEvent(t *testing.T) {
	available := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	observed := available.Add(time.Hour)
	raw := []model.DailyBar{{
		Symbol: "600036.SH", TradeDate: "2025-06-30", Open: 10, High: 10, Low: 10, Close: 10,
		Source: "baostock", SourceVersion: "baostock-test-v1", AdjustmentBasis: model.AdjustmentNone,
	}}
	factor := model.AdjustmentFactor{
		Symbol: "600036.SH", TradeDate: "2025-07-01", Factor: 2, Basis: model.AdjustmentFactorForward,
		Source: "baostock", SourceVersion: "baostock-test-v1", FetchedAt: available,
		AvailableAt: &available, SystemObservedAt: &observed, QualityStatus: model.QualityValid,
	}
	if _, err := rebuildAdjustedDailyBarsPIT(raw, []model.AdjustmentFactor{factor}, model.AdjustmentForward, "2025-07-01"); err == nil {
		t.Fatal("no factor on or before the first raw bar must not be extrapolated")
	}
}

func TestRebuildAdjustedDailyBarsCurrentAllowsUnknownAvailabilityButKeepsDegradedQuality(t *testing.T) {
	fetched := time.Now().UTC().Add(-time.Minute)
	observed := fetched.Add(time.Second)
	raw := []model.DailyBar{{
		Symbol: "600036.SH", TradeDate: "2025-07-01", Open: 10, High: 10, Low: 10, Close: 10,
		Source: "baostock", SourceVersion: "baostock-test-v1", AdjustmentBasis: model.AdjustmentNone,
		QualityStatus: model.QualityValid,
	}}
	factor := model.AdjustmentFactor{
		Symbol: "600036.SH", TradeDate: "2025-07-01", Factor: 2, Basis: model.AdjustmentFactorForward,
		Source: "baostock", SourceVersion: "baostock-test-v1", FetchedAt: fetched,
		SystemObservedAt: &observed, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_available_at_not_exposed_by_query_adjust_factor"},
	}
	got, err := rebuildAdjustedDailyBarsCurrent(raw, []model.AdjustmentFactor{factor}, model.AdjustmentForward, "2025-07-01")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Close != 10 || got[0].AdjustmentRefDate != "2025-07-01" ||
		got[0].QualityStatus != model.QualityDegraded ||
		!containsReason(got[0].QualityReasons, "adjustment_factor_reference_quality_degraded") {
		t.Fatalf("current rebuild must preserve unknown availability as degraded: %+v", got[0])
	}
	if _, err := rebuildAdjustedDailyBarsPIT(raw, []model.AdjustmentFactor{factor}, model.AdjustmentForward, "2025-07-01"); err == nil {
		t.Fatal("PIT rebuild must reject factors without proven AvailableAt")
	}
	factor.SystemObservedAt = nil
	if _, err := rebuildAdjustedDailyBarsCurrent(raw, []model.AdjustmentFactor{factor}, model.AdjustmentForward, "2025-07-01"); err == nil {
		t.Fatal("current rebuild must require persisted SystemObservedAt")
	}
}
