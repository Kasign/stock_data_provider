package service

import (
	"math"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
)

// CalculateMovingAverages adds close-price moving averages without changing the source bars.
// Windows are measured in trading sessions: MA120 approximates half a year and MA250 a full year.
// A value is null when the window is incomplete, contains an invalid close, or mixes adjustment bases.
func CalculateMovingAverages(bars []model.DailyBar) []model.DailyBar {
	for i := range bars {
		bars[i].MA5 = movingAverageAt(bars, i, 5)
		bars[i].MA20 = movingAverageAt(bars, i, 20)
		bars[i].MA120 = movingAverageAt(bars, i, 120)
		bars[i].MA250 = movingAverageAt(bars, i, 250)
	}
	return bars
}

func movingAverageAt(bars []model.DailyBar, end, window int) *float64 {
	if end+1 < window {
		return nil
	}

	basis := bars[end].AdjustmentBasis
	if basis == model.AdjustmentUnknown {
		return nil
	}

	sum := 0.0
	for i := end - window + 1; i <= end; i++ {
		bar := bars[i]
		if bar.AdjustmentBasis != basis || bar.QualityStatus == model.QualityInvalid || bar.QualityStatus == model.QualityUnknown {
			return nil
		}
		if bar.Close <= 0 || math.IsNaN(bar.Close) || math.IsInf(bar.Close, 0) {
			return nil
		}
		sum += bar.Close
	}

	average := sum / float64(window)
	if math.IsNaN(average) || math.IsInf(average, 0) {
		return nil
	}
	return &average
}
