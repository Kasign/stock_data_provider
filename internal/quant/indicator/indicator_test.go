package indicator

import (
	"errors"
	"math"
	"testing"
)

func closeTo(t *testing.T, got, want, tolerance float64) {
	t.Helper()
	if math.Abs(got-want) > tolerance {
		t.Fatalf("got %.12f, want %.12f (tol %.2g)", got, want, tolerance)
	}
}

func TestSMAAndEMA(t *testing.T) {
	sma, err := SMA([]float64{1, 2, 3, 4}, SMAParams{Window: 3})
	if err != nil {
		t.Fatal(err)
	}
	if sma.Status != StatusAvailable || sma.SampleCount != 3 {
		t.Fatalf("unexpected SMA metadata: %+v", sma)
	}
	closeTo(t, sma.Value, 3, 1e-12)

	ema, err := EMA([]float64{1, 2, 3, 4}, EMAParams{Window: 3, SeedMethod: "sma"})
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, ema.Value, 3, 1e-12)
}

func TestReturnAndVolatility(t *testing.T) {
	ret, err := Return([]float64{100, 110, 121}, ReturnParams{Window: 2, ReturnBasis: "simple"})
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, ret.Value, .21, 1e-12)

	logRet, err := Return([]float64{100, 110, 121}, ReturnParams{Window: 2, ReturnBasis: "log"})
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, logRet.Value, math.Log(1.21), 1e-12)

	extremeLog, err := Return([]float64{math.SmallestNonzeroFloat64, math.MaxFloat64}, ReturnParams{Window: 1, ReturnBasis: "log"})
	if err != nil {
		t.Fatalf("finite extreme log return was rejected: %v", err)
	}
	closeTo(t, extremeLog.Value, math.Log(math.MaxFloat64)-math.Log(math.SmallestNonzeroFloat64), 1e-12)

	extremeVolatility, err := Volatility([]float64{math.SmallestNonzeroFloat64, math.MaxFloat64, math.SmallestNonzeroFloat64}, VolatilityParams{Window: 2, ReturnBasis: "log", AnnualizationFactor: 1})
	if err != nil {
		t.Fatalf("finite extreme log volatility was rejected: %v", err)
	}
	extremeLogReturn := math.Log(math.MaxFloat64) - math.Log(math.SmallestNonzeroFloat64)
	closeTo(t, extremeVolatility.Value, math.Sqrt(2)*extremeLogReturn, 1e-9)

	vol, err := Volatility([]float64{100, 110, 100}, VolatilityParams{
		Window: 2, ReturnBasis: "simple", AnnualizationFactor: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := math.Sqrt((.1 + .09090909090909091) * (.1 + .09090909090909091) / 2)
	closeTo(t, vol.Value, expected, 1e-9)
}

func TestVolatilityAvoidsIntermediateOverflowForFiniteReturns(t *testing.T) {
	closes := make([]float64, 21)
	for i := range closes {
		if i%2 == 0 {
			closes[i] = 1e-308
		} else {
			closes[i] = 1
		}
	}

	got, err := Volatility(closes, VolatilityParams{Window: 20, ReturnBasis: "simple", AnnualizationFactor: 1})
	if err != nil {
		t.Fatalf("Volatility() rejected finite returns: %v", err)
	}
	want := math.Sqrt(5.0/19.0) * 1e308
	if !finite(got.Value) || math.Abs(got.Value-want)/want > 1e-12 {
		t.Fatalf("Volatility() = %.17g, want approximately %.17g", got.Value, want)
	}
}

func TestMeanAvoidsIntermediateOverflowForFinitePrices(t *testing.T) {
	closes := []float64{1e308, 1e308}

	sma, err := SMA(closes, SMAParams{Window: 2})
	if err != nil {
		t.Fatalf("SMA() rejected a representable mean: %v", err)
	}
	if sma.Status != StatusAvailable || sma.Value != 1e308 {
		t.Fatalf("SMA() = %+v, want available value 1e308", sma)
	}

	ema, err := EMA(closes, EMAParams{Window: 2, SeedMethod: "sma"})
	if err != nil {
		t.Fatalf("EMA() rejected a representable seed mean: %v", err)
	}
	if ema.Status != StatusAvailable || ema.Value != 1e308 {
		t.Fatalf("EMA() = %+v, want available value 1e308", ema)
	}
}

func TestBollingerAvoidsIntermediateOverflowForFinitePrices(t *testing.T) {
	closes := []float64{1e308, 5e307, 5e307}
	got, err := Bollinger(closes, BollingerParams{Window: 3, Multiplier: 2, StdDevMethod: "population"})
	if err != nil {
		t.Fatalf("Bollinger() rejected representable statistics: %v", err)
	}

	wantMiddle := (2.0 / 3.0) * 1e308
	wantStdDev := math.Sqrt(1.0/18.0) * 1e308
	wantUpper := wantMiddle + 2*wantStdDev
	wantLower := wantMiddle - 2*wantStdDev
	for _, pair := range []struct {
		name string
		got  float64
		want float64
	}{
		{"middle", got.Middle, wantMiddle},
		{"upper", got.Upper, wantUpper},
		{"lower", got.Lower, wantLower},
	} {
		if !finite(pair.got) || math.Abs(pair.got-pair.want)/pair.want > 1e-12 {
			t.Fatalf("Bollinger() %s = %.17g, want approximately %.17g", pair.name, pair.got, pair.want)
		}
	}
	if got.Status != StatusAvailable || got.SampleCount != 3 {
		t.Fatalf("unexpected Bollinger metadata: %+v", got)
	}
}

func TestRSISeedAvoidsIntermediateOverflowForFiniteChanges(t *testing.T) {
	max := math.MaxFloat64
	got, err := RSI([]float64{1, max, 1, max, 1}, RSIParams{Window: 3, SmoothingMethod: "wilder"})
	if err != nil {
		t.Fatalf("RSI() rejected representable Wilder averages: %v", err)
	}
	want := 100 - 100/(1+4.0/5.0)
	if !finite(got.Value) || math.Abs(got.Value-want) > 1e-12 {
		t.Fatalf("RSI() = %.17g, want %.17g", got.Value, want)
	}
}

func TestATRSmoothingAvoidsIntermediateOverflow(t *testing.T) {
	max := math.MaxFloat64
	got, err := ATR(
		[]float64{max, max, max, 1},
		[]float64{1, 1, 1, 1},
		[]float64{1, 1, 1, 1},
		1,
		ATRParams{Window: 3, SmoothingMethod: "wilder"},
	)
	if err != nil {
		t.Fatalf("ATR() rejected representable Wilder average: %v", err)
	}
	want := (2.0 / 3.0) * max
	if !finite(got.Value) || math.Abs(got.Value-want)/want > 1e-12 {
		t.Fatalf("ATR() = %.17g, want approximately %.17g", got.Value, want)
	}
}

func TestRSIWilderSeedAndSmoothing(t *testing.T) {
	got, err := RSI([]float64{1, 2, 3, 2, 1}, RSIParams{Window: 2, SmoothingMethod: "wilder"})
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, got.Value, 25, 1e-12)

	flat, err := RSI([]float64{5, 5, 5}, RSIParams{Window: 2, SmoothingMethod: "wilder"})
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, flat.Value, 50, 1e-12)
}

func TestMACDUsesDoubledHistogramAndSupportsNegativeDIF(t *testing.T) {
	got, err := MACD([]float64{4, 3, 2, 1}, MACDParams{
		FastWindow: 2, SlowWindow: 3, SignalWindow: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAvailable {
		t.Fatalf("unexpected status: %+v", got)
	}
	closeTo(t, got.DIF, -.5, 1e-12)
	closeTo(t, got.DEA, -.5, 1e-12)
	closeTo(t, got.Histogram, 0, 1e-12)

	positive, err := MACD([]float64{1, 2, 3, 4, 5, 7}, MACDParams{
		FastWindow: 2, SlowWindow: 3, SignalWindow: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, positive.DIF, 2.0/3.0, 1e-12)
	closeTo(t, positive.DEA, 11.0/18.0, 1e-12)
	closeTo(t, positive.Histogram, 1.0/9.0, 1e-12)
	if math.Abs(positive.Histogram-(positive.DIF-positive.DEA)) < 1e-12 {
		t.Fatal("histogram must be twice DIF-DEA, not DIF-DEA")
	}
}

func TestATRUsesExplicitPreviousClose(t *testing.T) {
	got, err := ATR(
		[]float64{11, 12, 13},
		[]float64{9, 10, 11},
		[]float64{10, 11, 12},
		10,
		ATRParams{Window: 3, SmoothingMethod: "wilder"},
	)
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, got.Value, 2, 1e-12)
}

func TestBollingerUsesPopulationDeviation(t *testing.T) {
	got, err := Bollinger([]float64{1, 2, 3}, BollingerParams{
		Window: 3, Multiplier: 2, StdDevMethod: "population",
	})
	if err != nil {
		t.Fatal(err)
	}
	closeTo(t, got.Middle, 2, 1e-12)
	closeTo(t, got.Upper, 2+2*math.Sqrt(2.0/3), 1e-12)
	closeTo(t, got.Lower, 2-2*math.Sqrt(2.0/3), 1e-12)
}

func TestAllIndicatorsReportInsufficientDataBeforeWarmup(t *testing.T) {
	tests := []struct {
		name string
		run  func() (Status, int, error)
	}{
		{"sma", func() (Status, int, error) {
			got, err := SMA([]float64{1, 2}, SMAParams{Window: 3})
			return got.Status, got.SampleCount, err
		}},
		{"ema", func() (Status, int, error) {
			got, err := EMA([]float64{1, 2}, EMAParams{Window: 3, SeedMethod: "sma"})
			return got.Status, got.SampleCount, err
		}},
		{"return", func() (Status, int, error) {
			got, err := Return([]float64{1, 2}, ReturnParams{Window: 2, ReturnBasis: "simple"})
			return got.Status, got.SampleCount, err
		}},
		{"volatility", func() (Status, int, error) {
			got, err := Volatility([]float64{1, 2}, VolatilityParams{Window: 2, ReturnBasis: "simple", AnnualizationFactor: 1})
			return got.Status, got.SampleCount, err
		}},
		{"rsi", func() (Status, int, error) {
			got, err := RSI([]float64{1, 2}, RSIParams{Window: 2, SmoothingMethod: "wilder"})
			return got.Status, got.SampleCount, err
		}},
		{"macd", func() (Status, int, error) {
			got, err := MACD([]float64{1, 2, 3}, MACDParams{FastWindow: 2, SlowWindow: 3, SignalWindow: 2})
			return got.Status, got.SampleCount, err
		}},
		{"atr", func() (Status, int, error) {
			got, err := ATR([]float64{2, 3}, []float64{1, 2}, []float64{1.5, 2.5}, 1, ATRParams{Window: 3, SmoothingMethod: "wilder"})
			return got.Status, got.SampleCount, err
		}},
		{"bollinger", func() (Status, int, error) {
			got, err := Bollinger([]float64{1}, BollingerParams{Window: 2, Multiplier: 2, StdDevMethod: "population"})
			return got.Status, got.SampleCount, err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, err := tt.run()
			if err != nil {
				t.Fatal(err)
			}
			if status != StatusInsufficientData {
				t.Fatalf("expected insufficient_data, got %s", status)
			}
		})
	}
}

func TestATRRejectsInvalidOHLCBoundaries(t *testing.T) {
	validHigh := []float64{11, 12}
	validLow := []float64{9, 10}
	validClose := []float64{10, 11}
	params := ATRParams{Window: 2, SmoothingMethod: "wilder"}
	cases := []struct {
		name string
		high []float64
		low []float64
		close []float64
		previous float64
	}{
		{"length mismatch", []float64{11}, validLow, validClose, 10},
		{"close above high", []float64{11, 12}, validLow, []float64{10, 13}, 10},
		{"close below low", validHigh, validLow, []float64{8, 11}, 10},
		{"zero previous close", validHigh, validLow, validClose, 0},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ATR(tt.high, tt.low, tt.close, tt.previous, params); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestIndicatorsRejectNonFiniteCalculatedResults(t *testing.T) {
	max := math.MaxFloat64
	tests := []struct {
		name string
		run  func() error
	}{
		{"sma overflow", func() error {
			_, err := SMA([]float64{max, max}, SMAParams{Window: 2})
			return err
		}},
		{"ema overflow", func() error {
			_, err := EMA([]float64{max, max}, EMAParams{Window: 2, SeedMethod: "sma"})
			return err
		}},
		{"simple return overflow", func() error {
			_, err := Return([]float64{1, max}, ReturnParams{Window: 1, ReturnBasis: "simple"})
			return err
		}},
		{"volatility overflow", func() error {
			_, err := Volatility([]float64{1, max, 1}, VolatilityParams{Window: 2, ReturnBasis: "simple", AnnualizationFactor: 1})
			return err
		}},
		{"rsi intermediate overflow", func() error {
			_, err := RSI([]float64{1, max, 1, max}, RSIParams{Window: 3, SmoothingMethod: "wilder"})
			return err
		}},
		{"macd overflow", func() error {
			_, err := MACD([]float64{1, max, 1, max}, MACDParams{FastWindow: 1, SlowWindow: 2, SignalWindow: 1})
			return err
		}},
		{"atr overflow", func() error {
			_, err := ATR([]float64{max, max}, []float64{1, 1}, []float64{1, 1}, 1, ATRParams{Window: 2, SmoothingMethod: "wilder"})
			return err
		}},
		{"bollinger overflow", func() error {
			_, err := Bollinger([]float64{max, max}, BollingerParams{Window: 2, Multiplier: 2, StdDevMethod: "population"})
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput, got %v", err)
			}
		})
	}
}

func TestIndicatorsRejectWindowsAboveContractMaximum(t *testing.T) {
	cases := []struct {
		name string
		run  func() error
	}{
		{"sma", func() error { _, err := SMA(nil, SMAParams{Window: 1001}); return err }},
		{"ema", func() error { _, err := EMA(nil, EMAParams{Window: 1001, SeedMethod: "sma"}); return err }},
		{"return", func() error { _, err := Return(nil, ReturnParams{Window: 1001, ReturnBasis: "simple"}); return err }},
		{"volatility", func() error { _, err := Volatility(nil, VolatilityParams{Window: 1001, ReturnBasis: "simple", AnnualizationFactor: 252}); return err }},
		{"rsi", func() error { _, err := RSI(nil, RSIParams{Window: 1001, SmoothingMethod: "wilder"}); return err }},
		{"macd slow", func() error { _, err := MACD(nil, MACDParams{FastWindow: 1, SlowWindow: 1001, SignalWindow: 1}); return err }},
		{"macd signal", func() error { _, err := MACD(nil, MACDParams{FastWindow: 1, SlowWindow: 2, SignalWindow: 1001}); return err }},
		{"atr", func() error { _, err := ATR(nil, nil, nil, 1, ATRParams{Window: 1001, SmoothingMethod: "wilder"}); return err }},
		{"bollinger", func() error { _, err := Bollinger(nil, BollingerParams{Window: 1001, Multiplier: 2, StdDevMethod: "population"}); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.run(); !errors.Is(err, ErrInvalidParameter) {
				t.Fatalf("expected ErrInvalidParameter, got %v", err)
			}
		})
	}
}

func TestIndicatorsAcceptContractMaximumWindow(t *testing.T) {
	prices := make([]float64, 1999)
	for i := range prices {
		prices[i] = 1
	}

	if got, err := SMA(prices[:1000], SMAParams{Window: 1000}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("SMA window=1000: result=%+v err=%v", got, err)
	}
	if got, err := EMA(prices[:1000], EMAParams{Window: 1000, SeedMethod: "sma"}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("EMA window=1000: result=%+v err=%v", got, err)
	}
	if got, err := Return(prices[:1001], ReturnParams{Window: 1000, ReturnBasis: "simple"}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("Return window=1000: result=%+v err=%v", got, err)
	}
	if got, err := Volatility(prices[:1001], VolatilityParams{Window: 1000, ReturnBasis: "simple", AnnualizationFactor: 252}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("Volatility window=1000: result=%+v err=%v", got, err)
	}
	if got, err := RSI(prices[:1001], RSIParams{Window: 1000, SmoothingMethod: "wilder"}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("RSI window=1000: result=%+v err=%v", got, err)
	}
	if got, err := MACD(prices, MACDParams{FastWindow: 999, SlowWindow: 1000, SignalWindow: 1000}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("MACD maximum windows: result=%+v err=%v", got, err)
	}
	if got, err := ATR(prices[:1000], prices[:1000], prices[:1000], 1, ATRParams{Window: 1000, SmoothingMethod: "wilder"}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("ATR window=1000: result=%+v err=%v", got, err)
	}
	if got, err := Bollinger(prices[:1000], BollingerParams{Window: 1000, Multiplier: 2, StdDevMethod: "population"}); err != nil || got.Status != StatusAvailable {
		t.Fatalf("Bollinger window=1000: result=%+v err=%v", got, err)
	}
}

func TestInsufficientDataAndInvalidInputs(t *testing.T) {
	got, err := SMA([]float64{1, 2}, SMAParams{Window: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusInsufficientData {
		t.Fatalf("unexpected status: %+v", got)
	}
	if _, err := EMA([]float64{1, 2, 3}, EMAParams{Window: 2, SeedMethod: "implicit"}); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("expected invalid seed method, got %v", err)
	}
	if _, err := SMA([]float64{1, math.NaN()}, SMAParams{Window: 2}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid price, got %v", err)
	}
	if _, err := EMA([]float64{1, -2, 3}, EMAParams{Window: 2, SeedMethod: "sma"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected non-positive close to fail, got %v", err)
	}
	if _, err := MACD([]float64{1, 2, 3}, MACDParams{FastWindow: 3, SlowWindow: 2, SignalWindow: 1}); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("expected invalid MACD windows, got %v", err)
	}
}
