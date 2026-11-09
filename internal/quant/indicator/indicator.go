// Package indicator 提供无副作用、可复算的首批技术指标计算。
package indicator

import (
	"errors"
	"math"
)

type Status string

const (
	StatusAvailable        Status = "available"
	StatusInsufficientData Status = "insufficient_data"
)

var (
	ErrInvalidParameter = errors.New("invalid indicator parameter")
	ErrInvalidInput     = errors.New("invalid indicator input")
)

type Result struct {
	Value       float64
	Status      Status
	SampleCount int
}

type MACDResult struct {
	DIF         float64
	DEA         float64
	Histogram   float64
	Status      Status
	SampleCount int
}

type BandsResult struct {
	Middle      float64
	Upper       float64
	Lower       float64
	Status      Status
	SampleCount int
}

type SMAParams struct {
	Window int
}

type EMAParams struct {
	Window     int
	SeedMethod string
}

type ReturnParams struct {
	Window      int
	ReturnBasis string
}

type VolatilityParams struct {
	Window              int
	ReturnBasis         string
	AnnualizationFactor int
}

type RSIParams struct {
	Window          int
	SmoothingMethod string
}

type MACDParams struct {
	FastWindow   int
	SlowWindow   int
	SignalWindow int
}

type ATRParams struct {
	Window          int
	SmoothingMethod string
}

type BollingerParams struct {
	Window       int
	Multiplier   float64
	StdDevMethod string
}

func SMA(closes []float64, p SMAParams) (Result, error) {
	if !validWindow(p.Window, 1) {
		return Result{}, ErrInvalidParameter
	}
	if err := validatePrices(closes); err != nil {
		return Result{}, err
	}
	if len(closes) < p.Window {
		return Result{Status: StatusInsufficientData, SampleCount: len(closes)}, nil
	}

	window := closes[len(closes)-p.Window:]
	value := mean(window)
	if !finite(value) {
		return Result{}, ErrInvalidInput
	}
	return Result{
		Value:       value,
		Status:      StatusAvailable,
		SampleCount: len(window),
	}, nil
}

func EMA(closes []float64, p EMAParams) (Result, error) {
	if !validWindow(p.Window, 1) || p.SeedMethod != "sma" {
		return Result{}, ErrInvalidParameter
	}
	if err := validatePrices(closes); err != nil {
		return Result{}, err
	}

	values, err := emaSeries(closes, p.Window)
	if err != nil {
		return Result{}, err
	}
	if len(values) == 0 {
		return Result{Status: StatusInsufficientData, SampleCount: len(closes)}, nil
	}
	value := values[len(values)-1]
	if !finite(value) {
		return Result{}, ErrInvalidInput
	}
	return Result{
		Value:       value,
		Status:      StatusAvailable,
		SampleCount: len(closes),
	}, nil
}

func Return(closes []float64, p ReturnParams) (Result, error) {
	if !validWindow(p.Window, 1) || (p.ReturnBasis != "simple" && p.ReturnBasis != "log") {
		return Result{}, ErrInvalidParameter
	}
	if err := validatePrices(closes); err != nil {
		return Result{}, err
	}
	if len(closes) <= p.Window {
		return Result{Status: StatusInsufficientData, SampleCount: len(closes)}, nil
	}

	start := closes[len(closes)-1-p.Window]
	end := closes[len(closes)-1]
	value := end/start - 1
	if p.ReturnBasis == "log" {
		value = math.Log(end) - math.Log(start)
	}
	if !finite(value) {
		return Result{}, ErrInvalidInput
	}
	return Result{Value: value, Status: StatusAvailable, SampleCount: p.Window}, nil
}

func Volatility(closes []float64, p VolatilityParams) (Result, error) {
	if !validWindow(p.Window, 2) ||
		(p.ReturnBasis != "simple" && p.ReturnBasis != "log") ||
		p.AnnualizationFactor <= 0 {
		return Result{}, ErrInvalidParameter
	}
	if err := validatePrices(closes); err != nil {
		return Result{}, err
	}
	if len(closes) <= p.Window {
		return Result{
			Status:      StatusInsufficientData,
			SampleCount: max(0, len(closes)-1),
		}, nil
	}

	start := len(closes) - p.Window - 1
	returns := make([]float64, p.Window)
	for i := range returns {
		previous, current := closes[start+i], closes[start+i+1]
		if p.ReturnBasis == "simple" {
			returns[i] = current/previous - 1
		} else {
			returns[i] = math.Log(current) - math.Log(previous)
		}
	}
	average := mean(returns)
	var sum float64
	for _, value := range returns {
		delta := value - average
		sum += delta * delta
	}
	value := math.Sqrt(sum/float64(len(returns)-1)) * math.Sqrt(float64(p.AnnualizationFactor))
	if !finite(value) {
		return Result{}, ErrInvalidInput
	}
	return Result{Value: value, Status: StatusAvailable, SampleCount: len(returns)}, nil
}

func RSI(closes []float64, p RSIParams) (Result, error) {
	if !validWindow(p.Window, 1) || p.SmoothingMethod != "wilder" {
		return Result{}, ErrInvalidParameter
	}
	if err := validatePrices(closes); err != nil {
		return Result{}, err
	}
	if len(closes) <= p.Window {
		return Result{
			Status:      StatusInsufficientData,
			SampleCount: max(0, len(closes)-1),
		}, nil
	}

	var gains, losses float64
	for i := 1; i <= p.Window; i++ {
		change := closes[i] - closes[i-1]
		if change > 0 {
			gains += change
		} else {
			losses -= change
		}
	}
	averageGain := gains / float64(p.Window)
	averageLoss := losses / float64(p.Window)
	if !finite(averageGain) || !finite(averageLoss) {
		return Result{}, ErrInvalidInput
	}

	for i := p.Window + 1; i < len(closes); i++ {
		change := closes[i] - closes[i-1]
		gain, loss := 0.0, 0.0
		if change > 0 {
			gain = change
		} else {
			loss = -change
		}
		averageGain = (averageGain*float64(p.Window-1) + gain) / float64(p.Window)
		averageLoss = (averageLoss*float64(p.Window-1) + loss) / float64(p.Window)
		if !finite(averageGain) || !finite(averageLoss) {
			return Result{}, ErrInvalidInput
		}
	}

	value := rsiValue(averageGain, averageLoss)
	if !finite(value) {
		return Result{}, ErrInvalidInput
	}
	return Result{
		Value:       value,
		Status:      StatusAvailable,
		SampleCount: len(closes) - 1,
	}, nil
}

func MACD(closes []float64, p MACDParams) (MACDResult, error) {
	if !validWindow(p.FastWindow, 1) ||
		!validWindow(p.SlowWindow, 2) ||
		p.FastWindow >= p.SlowWindow ||
		!validWindow(p.SignalWindow, 1) {
		return MACDResult{}, ErrInvalidParameter
	}
	if err := validatePrices(closes); err != nil {
		return MACDResult{}, err
	}
	if len(closes) < p.SlowWindow+p.SignalWindow-1 {
		return MACDResult{Status: StatusInsufficientData, SampleCount: len(closes)}, nil
	}

	fast, err := emaSeries(closes, p.FastWindow)
	if err != nil {
		return MACDResult{}, err
	}
	slow, err := emaSeries(closes, p.SlowWindow)
	if err != nil {
		return MACDResult{}, err
	}
	dif := make([]float64, 0, len(closes)-p.SlowWindow+1)
	for i := p.SlowWindow - 1; i < len(closes); i++ {
		dif = append(dif, fast[i]-slow[i])
	}
	dea, err := emaSeries(dif, p.SignalWindow)
	if err != nil {
		return MACDResult{}, err
	}
	if len(dea) == 0 {
		return MACDResult{Status: StatusInsufficientData, SampleCount: len(closes)}, nil
	}

	lastDIF := dif[len(dif)-1]
	lastDEA := dea[len(dea)-1]
	histogram := 2 * (lastDIF - lastDEA)
	if !finite(lastDIF) || !finite(lastDEA) || !finite(histogram) {
		return MACDResult{}, ErrInvalidInput
	}
	return MACDResult{
		DIF:         lastDIF,
		DEA:         lastDEA,
		Histogram:   histogram,
		Status:      StatusAvailable,
		SampleCount: len(closes),
	}, nil
}

// ATR 的 previousClose 是输入窗口前一交易日的收盘价；window 个 TR 需要 window 根 OHLC bar。
func ATR(highs, lows, closes []float64, previousClose float64, p ATRParams) (Result, error) {
	if !validWindow(p.Window, 1) || p.SmoothingMethod != "wilder" {
		return Result{}, ErrInvalidParameter
	}
	if len(highs) != len(lows) || len(lows) != len(closes) {
		return Result{}, ErrInvalidInput
	}
	if !finite(previousClose) || previousClose <= 0 {
		return Result{}, ErrInvalidInput
	}
	if err := validatePrices(highs); err != nil {
		return Result{}, err
	}
	if err := validatePrices(lows); err != nil {
		return Result{}, err
	}
	if err := validatePrices(closes); err != nil {
		return Result{}, err
	}
	for i := range highs {
		if highs[i] < lows[i] || closes[i] > highs[i] || closes[i] < lows[i] {
			return Result{}, ErrInvalidInput
		}
	}
	if len(closes) < p.Window {
		return Result{Status: StatusInsufficientData, SampleCount: len(closes)}, nil
	}

	trueRanges := make([]float64, len(closes))
	previous := previousClose
	for i := range closes {
		trueRanges[i] = math.Max(
			highs[i]-lows[i],
			math.Max(math.Abs(highs[i]-previous), math.Abs(lows[i]-previous)),
		)
		previous = closes[i]
	}
	average := mean(trueRanges[:p.Window])
	for i := p.Window; i < len(trueRanges); i++ {
		average = (average*float64(p.Window-1) + trueRanges[i]) / float64(p.Window)
	}
	if !finite(average) {
		return Result{}, ErrInvalidInput
	}
	return Result{Value: average, Status: StatusAvailable, SampleCount: len(trueRanges)}, nil
}

func Bollinger(closes []float64, p BollingerParams) (BandsResult, error) {
	if !validWindow(p.Window, 2) || !finite(p.Multiplier) || p.Multiplier <= 0 || p.StdDevMethod != "population" {
		return BandsResult{}, ErrInvalidParameter
	}
	if err := validatePrices(closes); err != nil {
		return BandsResult{}, err
	}
	if len(closes) < p.Window {
		return BandsResult{Status: StatusInsufficientData, SampleCount: len(closes)}, nil
	}

	window := closes[len(closes)-p.Window:]
	middle := mean(window)
	var sum float64
	for _, value := range window {
		delta := value - middle
		sum += delta * delta
	}
	standardDeviation := math.Sqrt(sum / float64(p.Window))
	band := p.Multiplier * standardDeviation
	upper, lower := middle+band, middle-band
	if !finite(middle) || !finite(standardDeviation) || !finite(upper) || !finite(lower) {
		return BandsResult{}, ErrInvalidInput
	}
	return BandsResult{
		Middle:      middle,
		Upper:       upper,
		Lower:       lower,
		Status:      StatusAvailable,
		SampleCount: p.Window,
	}, nil
}

func emaSeries(values []float64, window int) ([]float64, error) {
	if !validWindow(window, 1) {
		return nil, ErrInvalidParameter
	}
	if err := validateFinite(values); err != nil {
		return nil, err
	}
	if len(values) < window {
		return nil, nil
	}

	out := make([]float64, len(values))
	out[window-1] = mean(values[:window])
	alpha := 2 / float64(window+1)
	for i := window; i < len(values); i++ {
		out[i] = alpha*values[i] + (1-alpha)*out[i-1]
	}
	return out, nil
}

const maxIndicatorWindow = 1000

func validWindow(window, minimum int) bool {
	return window >= minimum && window <= maxIndicatorWindow
}

func validatePrices(values []float64) error {
	for _, value := range values {
		if !finite(value) || value <= 0 {
			return ErrInvalidInput
		}
	}
	return nil
}

func validateFinite(values []float64) error {
	for _, value := range values {
		if !finite(value) {
			return ErrInvalidInput
		}
	}
	return nil
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func mean(values []float64) float64 {
	var sum float64
	for _, value := range values {
		sum += value
	}
	return sum / float64(len(values))
}

func rsiValue(gain, loss float64) float64 {
	if gain == 0 && loss == 0 {
		return 50
	}
	if loss == 0 {
		return 100
	}
	if gain == 0 {
		return 0
	}
	return 100 - 100/(1+gain/loss)
}
