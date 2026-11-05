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
		{Symbol: "600036.SH", TradeDate: "2025-06-30", Period: "1d", Open: 10, High: 10, Low: 10, Close: 10, Source: "baostock"},
		{Symbol: "600036.SH", TradeDate: "2025-07-01", Period: "1d", Open: 20, High: 20, Low: 20, Close: 20, Source: "baostock"},
	}
	factor := func(date string, value float64, factorBasis model.AdjustmentFactorBasis) model.AdjustmentFactor {
		return model.AdjustmentFactor{
			Symbol: "600036.SH", TradeDate: date, Factor: value, Basis: factorBasis,
			Source: "baostock", SourceVersion: "baostock-test-v1",
			AvailableAt: &available, SystemObservedAt: &observed,
			QualityStatus: model.QualityValid,
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
	if err != nil { t.Fatal(err) }
	if forward[0].Close != 5 || forward[1].Close != 20 {
		t.Fatalf("forward rebuild used wrong factor basis: got closes %.4f, %.4f", forward[0].Close, forward[1].Close)
	}
	backward, err := rebuildAdjustedDailyBars(raw, factors, model.AdjustmentBackward, "2025-07-01")
	if err != nil { t.Fatal(err) }
	if backward[0].Close != 50 || backward[1].Close != 200 {
		t.Fatalf("backward rebuild used wrong factor basis: got closes %.4f, %.4f", backward[0].Close, backward[1].Close)
	}
}

func TestRebuildAdjustedDailyBarsKeepsSharedFactorFallback(t *testing.T) {
	available := time.Date(2025, 7, 1, 0, 0, 0, 0, time.UTC)
	observed := available.Add(time.Hour)
	raw := []model.DailyBar{{Symbol: "600036.SH", TradeDate: "2025-07-01", Open: 10, High: 10, Low: 10, Close: 10}}
	factors := []model.AdjustmentFactor{{
		Symbol: "600036.SH", TradeDate: "2025-07-01", Factor: 2,
		Source: "tushare", SourceVersion: "tushare-test-v1",
		AvailableAt: &available, SystemObservedAt: &observed, QualityStatus: model.QualityValid,
	}}
	got, err := rebuildAdjustedDailyBars(raw, factors, model.AdjustmentForward, "2025-07-01")
	if err != nil { t.Fatal(err) }
	if got[0].Close != 10 { t.Fatalf("shared factor fallback produced close %.4f, want 10", got[0].Close) }
}
