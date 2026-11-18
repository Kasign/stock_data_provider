package indicator

import (
	"math"
	"math/big"
	"testing"
)

const referencePrecision uint = 256

func referenceFloat(value float64) *big.Float {
	return new(big.Float).SetPrec(referencePrecision).SetMode(big.ToNearestEven).SetFloat64(value)
}

func referenceAdd(a, b *big.Float) *big.Float {
	return new(big.Float).SetPrec(referencePrecision).Add(a, b)
}

func referenceSub(a, b *big.Float) *big.Float {
	return new(big.Float).SetPrec(referencePrecision).Sub(a, b)
}

func referenceMul(a, b *big.Float) *big.Float {
	return new(big.Float).SetPrec(referencePrecision).Mul(a, b)
}

func referenceDiv(a, b *big.Float) *big.Float {
	return new(big.Float).SetPrec(referencePrecision).Quo(a, b)
}

// referenceLn 用 atanh 展开独立计算自然对数，不调用生产代码使用的 math.Log。
func referenceLn(value *big.Float) *big.Float {
	mantissa := new(big.Float).SetPrec(referencePrecision)
	exponent := value.MantExp(mantissa)
	one := referenceFloat(1)
	z := referenceDiv(referenceSub(mantissa, one), referenceAdd(mantissa, one))
	zSquared := referenceMul(z, z)
	term := new(big.Float).SetPrec(referencePrecision).Set(z)
	sum := new(big.Float).SetPrec(referencePrecision)
	for denominator := int64(1); denominator < 10000; denominator += 2 {
		sum = referenceAdd(sum, referenceDiv(term, referenceFloat(float64(denominator))))
		term = referenceMul(term, zSquared)
		if term.Sign() == 0 || term.MantExp(nil) < -referencePrecision {
			break
		}
	}
	lnMantissa := referenceMul(referenceFloat(2), sum)
	// ln(2) 使用 z=(2-1)/(2+1)=1/3 的同一收敛级数。
	third := referenceDiv(one, referenceFloat(3))
	thirdSquared := referenceMul(third, third)
	term = new(big.Float).SetPrec(referencePrecision).Set(third)
	lnTwoSum := new(big.Float).SetPrec(referencePrecision)
	for denominator := int64(1); denominator < 10000; denominator += 2 {
		lnTwoSum = referenceAdd(lnTwoSum, referenceDiv(term, referenceFloat(float64(denominator))))
		term = referenceMul(term, thirdSquared)
		if term.Sign() == 0 || term.MantExp(nil) < -referencePrecision {
			break
		}
	}
	lnTwo := referenceMul(referenceFloat(2), lnTwoSum)
	return referenceAdd(lnMantissa, referenceMul(referenceFloat(float64(exponent)), lnTwo))
}

func referenceReturn(start, end float64, basis string) *big.Float {
	a, b := referenceFloat(start), referenceFloat(end)
	if basis == "log" {
		return referenceSub(referenceLn(b), referenceLn(a))
	}
	return referenceSub(referenceDiv(b, a), referenceFloat(1))
}

func referenceVolatility(closes []float64, window, annualization int, basis string) *big.Float {
	returns := make([]*big.Float, window)
	start := len(closes) - window - 1
	for i := range returns {
		returns[i] = referenceReturn(closes[start+i], closes[start+i+1], basis)
	}
	mean := new(big.Float).SetPrec(referencePrecision)
	for _, value := range returns {
		mean = referenceAdd(mean, referenceDiv(value, referenceFloat(float64(window))))
	}
	sumSquares := new(big.Float).SetPrec(referencePrecision)
	for _, value := range returns {
		delta := referenceSub(value, mean)
		sumSquares = referenceAdd(sumSquares, referenceMul(delta, delta))
	}
	variance := referenceDiv(sumSquares, referenceFloat(float64(window-1)))
	stddev := new(big.Float).SetPrec(referencePrecision).Sqrt(variance)
	return referenceMul(stddev, new(big.Float).SetPrec(referencePrecision).Sqrt(referenceFloat(float64(annualization))))
}

func referenceRSI(closes []float64, window int) *big.Float {
	gainSum, lossSum := new(big.Float).SetPrec(referencePrecision), new(big.Float).SetPrec(referencePrecision)
	for i := 1; i <= window; i++ {
		change := referenceSub(referenceFloat(closes[i]), referenceFloat(closes[i-1]))
		if change.Sign() > 0 {
			gainSum = referenceAdd(gainSum, change)
		} else {
			lossSum = referenceAdd(lossSum, new(big.Float).SetPrec(referencePrecision).Neg(change))
		}
	}
	gain := referenceDiv(gainSum, referenceFloat(float64(window)))
	loss := referenceDiv(lossSum, referenceFloat(float64(window)))
	for i := window + 1; i < len(closes); i++ {
		change := referenceSub(referenceFloat(closes[i]), referenceFloat(closes[i-1]))
		currentGain, currentLoss := new(big.Float).SetPrec(referencePrecision), new(big.Float).SetPrec(referencePrecision)
		if change.Sign() > 0 {
			currentGain.Set(change)
		} else {
			currentLoss.Neg(change)
		}
		gain = referenceDiv(referenceAdd(referenceMul(referenceFloat(float64(window-1)), gain), currentGain), referenceFloat(float64(window)))
		loss = referenceDiv(referenceAdd(referenceMul(referenceFloat(float64(window-1)), loss), currentLoss), referenceFloat(float64(window)))
	}
	if gain.Sign() == 0 && loss.Sign() == 0 {
		return referenceFloat(50)
	}
	if loss.Sign() == 0 {
		return referenceFloat(100)
	}
	if gain.Sign() == 0 {
		return referenceFloat(0)
	}
	rs := referenceDiv(gain, loss)
	return referenceSub(referenceFloat(100), referenceDiv(referenceFloat(100), referenceAdd(referenceFloat(1), rs)))
}

func assertReferenceClose(t *testing.T, name string, got float64, want *big.Float) {
	t.Helper()
	want64, _ := want.Float64()
	if math.IsNaN(got) || math.IsInf(got, 0) {
		t.Fatalf("%s produced non-finite result: %g", name, got)
	}
	// 该阈值只用于验证 synthetic float64 算法实现与 256-bit 参考计算的一致性，
	// 不是生产输入范围、API 精度声明或 Registry precision.tolerance。
	scale := math.Max(1, math.Abs(want64))
	if math.Abs(got-want64) > 5e-13*scale {
		t.Fatalf("%s: float64=%.17g reference=%.17g relative_error=%.3g", name, got, want64, math.Abs(got-want64)/scale)
	}
}

func TestStatisticalIndicatorsAgainstHighPrecisionReference(t *testing.T) {
	cases := []struct {
		name   string
		closes []float64
	}{
		{
			name:   "small changes around large base",
			closes: []float64{1e12, 1e12 + 0.01, 1e12 - 0.02, 1e12 + 0.03, 1e12 + 0.01, 1e12 + 0.04},
		},
		{
			name:   "alternating scale",
			closes: []float64{1e-100, 1e100, 1.000000000000001e100, 1e-100, 1.000000000000002e100, 1e-100},
		},
		{
			name:   "near flat long sequence",
			closes: func() []float64 {
				values := make([]float64, 2049)
				for i := range values {
					values[i] = 100 + float64((i%7)-3)*1e-10
				}
				return values
			}(),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			window := 5
			for _, basis := range []string{"simple", "log"} {
				t.Run("return_"+basis, func(t *testing.T) {
					got, err := Return(tc.closes, ReturnParams{Window: window, ReturnBasis: basis})
					if err != nil {
						t.Fatal(err)
					}
					want := referenceReturn(tc.closes[len(tc.closes)-1-window], tc.closes[len(tc.closes)-1], basis)
					assertReferenceClose(t, "Return/"+basis, got.Value, want)
				})
				t.Run("volatility_"+basis, func(t *testing.T) {
					got, err := Volatility(tc.closes, VolatilityParams{Window: window, ReturnBasis: basis, AnnualizationFactor: 252})
					if err != nil {
						t.Fatal(err)
					}
					want := referenceVolatility(tc.closes, window, 252, basis)
					assertReferenceClose(t, "Volatility/"+basis, got.Value, want)
				})
			}
		})
	}
}

func TestRSIAgainstHighPrecisionWilderReference(t *testing.T) {
	closes := make([]float64, 5001)
	closes[0] = 100
	for i := 1; i < len(closes); i++ {
		switch i % 5 {
		case 0:
			closes[i] = closes[i-1] + 0.0003
		case 1:
			closes[i] = closes[i-1] - 0.0001
		case 2:
			closes[i] = closes[i-1]
		case 3:
			closes[i] = closes[i-1] + 0.0002
		default:
			closes[i] = closes[i-1] - 0.0004
		}
	}
	const window = 14
	got, err := RSI(closes, RSIParams{Window: window, SmoothingMethod: "wilder"})
	if err != nil {
		t.Fatal(err)
	}
	assertReferenceClose(t, "RSI/Wilder/long-sequence", got.Value, referenceRSI(closes, window))
}

func TestStatisticalIndicatorMathematicalBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name   string
		closes []float64
		want   float64
	}{
		{"all gains", []float64{1, 2, 3, 4, 5}, 100},
		{"all losses", []float64{5, 4, 3, 2, 1}, 0},
		{"flat", []float64{5, 5, 5, 5, 5}, 50},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := RSI(tc.closes, RSIParams{Window: 2, SmoothingMethod: "wilder"})
			if err != nil {
				t.Fatal(err)
			}
			if got.Value != tc.want || got.Value < 0 || got.Value > 100 {
				t.Fatalf("RSI=%g, want %g within [0,100]", got.Value, tc.want)
			}
		})
	}
}
