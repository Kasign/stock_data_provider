package indicators

import (
	"errors"
	"math"
	"sort"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

var ErrInsufficientBars = errors.New("at least one valid daily bar is required")

func Calculate(bars []domain.DailyBar) (domain.TechnicalSnapshot, error) {
	ordered := append([]domain.DailyBar(nil), bars...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Date < ordered[j].Date })
	closes := make([]float64, 0, len(ordered))
	for _, bar := range ordered {
		if bar.Close > 0 && !math.IsNaN(bar.Close) && !math.IsInf(bar.Close, 0) {
			closes = append(closes, bar.Close)
		}
	}
	if len(closes) == 0 {
		return domain.TechnicalSnapshot{}, ErrInsufficientBars
	}
	out := domain.TechnicalSnapshot{}
	out.MA5 = movingAverage(closes, 5)
	out.MA10 = movingAverage(closes, 10)
	out.MA20 = movingAverage(closes, 20)
	out.MA60 = movingAverage(closes, 60)
	if out.MA60 == nil {
		// The Python legacy implementation uses MA20 as its MA60 fallback when
		// fewer than 60 bars are available.
		out.MA60 = out.MA20
	}
	out.RSI14 = rsi(closes, 14)
	dif, dea, hist := macd(closes, 12, 26, 9)
	out.MACDDIF, out.MACDDEA, out.MACDHist = dif, dea, hist
	if len(closes) >= 20 && out.MA5 != nil && out.MA10 != nil && out.MA20 != nil {
		switch {
		case *out.MA5 > *out.MA10 && *out.MA10 > *out.MA20:
			out.MAAlignment, out.TrendStatus = "bullish", "uptrend"
		case *out.MA5 < *out.MA10 && *out.MA10 < *out.MA20:
			out.MAAlignment, out.TrendStatus = "bearish", "downtrend"
		default:
			out.MAAlignment, out.TrendStatus = "mixed", "sideways"
		}
	}
	last := closes[len(closes)-1]
	if last > 0 {
		out.BiasMA5Pct = bias(last, out.MA5)
		out.BiasMA10Pct = bias(last, out.MA10)
		out.BiasMA20Pct = bias(last, out.MA20)
	}
	return out, nil
}

func movingAverage(values []float64, period int) *float64 {
	if len(values) < period {
		return nil
	}
	var total float64
	for _, v := range values[len(values)-period:] {
		total += v
	}
	result := total / float64(period)
	return &result
}

func bias(price float64, average *float64) *float64 {
	if average == nil || *average == 0 {
		return nil
	}
	v := (price - *average) / *average * 100
	return &v
}

func rsi(values []float64, period int) *float64 {
	if len(values) == 0 || period <= 0 {
		return nil
	}
	// pandas Series.ewm(alpha=1/period, adjust=False) is seeded with the first
	// observation. The legacy implementation converts the initial NaN delta to
	// zero, so avg_gain/avg_loss begin at zero and use Wilder smoothing thereafter.
	alpha := 1.0 / float64(period)
	avgGain, avgLoss := 0.0, 0.0
	for i := 1; i < len(values); i++ {
		delta := values[i] - values[i-1]
		gain, loss := 0.0, 0.0
		if delta > 0 {
			gain = delta
		} else if delta < 0 {
			loss = -delta
		}
		avgGain = alpha*gain + (1-alpha)*avgGain
		avgLoss = alpha*loss + (1-alpha)*avgLoss
	}
	if avgLoss == 0 {
		v := 100.0
		if avgGain == 0 {
			v = 50
		}
		return &v
	}
	rs := avgGain / avgLoss
	v := 100 - (100 / (1 + rs))
	return &v
}

func macd(values []float64, fast, slow, signal int) (*float64, *float64, *float64) {
	if len(values) == 0 {
		return nil, nil, nil
	}
	fastSeries := emaSeries(values, fast)
	slowSeries := emaSeries(values, slow)
	difSeries := make([]float64, len(values))
	for i := range values {
		difSeries[i] = fastSeries[i] - slowSeries[i]
	}
	deaSeries := emaSeries(difSeries, signal)
	i := len(values) - 1
	dif, dea := difSeries[i], deaSeries[i]
	hist := (dif - dea) * 2
	return &dif, &dea, &hist
}

func emaSeries(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	if len(values) == 0 {
		return out
	}
	alpha := 2.0 / (float64(period) + 1)
	out[0] = values[0]
	for i := 1; i < len(values); i++ {
		out[i] = values[i]*alpha + out[i-1]*(1-alpha)
	}
	return out
}
