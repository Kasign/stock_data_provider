package service

import (
	"testing"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

func TestCalculateMovingAveragesUsesTradingSessionWindows(t *testing.T) {
	bars := make([]model.DailyBar, 250)
	for i := range bars {
		bars[i] = model.DailyBar{
			TradeDate:       "2026-01-01",
			Close:           float64(i + 1),
			AdjustmentBasis: model.AdjustmentForward,
			QualityStatus:   model.QualityValid,
		}
	}

	CalculateMovingAverages(bars)
	if bars[3].MA5 != nil || bars[18].MA20 != nil || bars[118].MA120 != nil || bars[248].MA250 != nil {
		t.Fatal("moving averages must remain null until their full windows are available")
	}
	assertAverage := func(name string, got *float64, want float64) {
		t.Helper()
		if got == nil || *got != want {
			t.Fatalf("%s got=%v want=%v", name, got, want)
		}
	}
	assertAverage("MA5", bars[4].MA5, 3)
	assertAverage("MA20", bars[19].MA20, 10.5)
	assertAverage("MA120", bars[119].MA120, 60.5)
	assertAverage("MA250", bars[249].MA250, 125.5)
}

func TestCalculateMovingAveragesRejectsInvalidOrMixedAdjustmentWindow(t *testing.T) {
	bars := make([]model.DailyBar, 250)
	for i := range bars {
		bars[i] = model.DailyBar{Close: float64(i + 1), AdjustmentBasis: model.AdjustmentForward, QualityStatus: model.QualityValid}
	}
	bars[100].Close = 0
	CalculateMovingAverages(bars)
	if bars[119].MA120 != nil {
		t.Fatal("MA120 must be null when its window contains an invalid close")
	}
	if bars[220].MA120 == nil {
		t.Fatal("MA120 should recover after the invalid close leaves its window")
	}
	bars[249].AdjustmentBasis = model.AdjustmentBackward
	CalculateMovingAverages(bars)
	if bars[249].MA250 != nil {
		t.Fatal("MA250 must be null when its window mixes adjustment bases")
	}
}
