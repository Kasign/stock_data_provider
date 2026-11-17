// Package factor 实现首批单证券时间序列因子；不承担横截面排序或中性化。
package factor

import (
	"errors"
	"math"
)

var (
	ErrInvalidInput  = errors.New("invalid factor input")
	ErrInvalidWindow = errors.New("invalid factor window")
)

type Status string

const (
	StatusAvailable        Status = "available"
	StatusInsufficientData Status = "insufficient_data"
)

type Result struct {
	Code        string
	Value       float64
	Unit        string
	Status      Status
	SampleCount int
}

func Momentum20(closes []float64) (Result, error) {
	return momentum("momentum_20d", closes, 20)
}

func Momentum60(closes []float64) (Result, error) {
	return momentum("momentum_60d", closes, 60)
}

func momentum(code string, closes []float64, window int) (Result, error) {
	if window < 1 {
		return Result{}, ErrInvalidWindow
	}
	if err := validate(closes); err != nil {
		return Result{}, err
	}
	if len(closes) < window+1 {
		return Result{
			Code:        code,
			Unit:        "ratio",
			Status:      StatusInsufficientData,
			SampleCount: len(closes),
		}, nil
	}

	start := closes[len(closes)-window-1]
	end := closes[len(closes)-1]
	value := end/start - 1
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Result{}, ErrInvalidInput
	}
	return Result{
		Code:        code,
		Value:       value,
		Unit:        "ratio",
		Status:      StatusAvailable,
		SampleCount: window + 1,
	}, nil
}

// Volatility20 使用最近 20 个 simple daily returns 的样本标准差（ddof=1），不年化。
func Volatility20(closes []float64) (Result, error) {
	const window = 20
	if err := validate(closes); err != nil {
		return Result{}, err
	}
	if len(closes) < window+1 {
		return Result{
			Code:        "volatility_20d",
			Unit:        "ratio",
			Status:      StatusInsufficientData,
			SampleCount: max(0, len(closes)-1),
		}, nil
	}

	start := len(closes) - window - 1
	returns := make([]float64, window)
	for i := range returns {
		returns[i] = closes[start+i+1]/closes[start+i] - 1
	}
	value := sampleStdDev(returns)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return Result{}, ErrInvalidInput
	}
	return Result{
		Code:        "volatility_20d",
		Value:       value,
		Unit:        "ratio",
		Status:      StatusAvailable,
		SampleCount: window,
	}, nil
}

// sampleStdDev 先按最大绝对值缩放，再计算样本标准差，避免中间求和或平方溢出。
func sampleStdDev(values []float64) float64 {
	var scale float64
	for _, value := range values {
		if absolute := math.Abs(value); absolute > scale {
			scale = absolute
		}
	}
	if scale == 0 {
		return 0
	}

	var average float64
	for _, value := range values {
		average += (value / scale) / float64(len(values))
	}
	var sum float64
	for _, value := range values {
		delta := value/scale - average
		sum += delta * delta
	}
	return scale * math.Sqrt(sum/float64(len(values)-1))
}

func validate(closes []float64) error {
	for _, value := range closes {
		if math.IsNaN(value) || math.IsInf(value, 0) || value <= 0 {
			return ErrInvalidInput
		}
	}
	return nil
}
