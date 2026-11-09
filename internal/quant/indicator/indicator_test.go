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

	vol, err := Volatility([]float64{100, 110, 100}, VolatilityParams{
		Window: 2, ReturnBasis: "simple", AnnualizationFactor: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	expected := math.Sqrt((.1 + .09090909090909091) * (.1 + .09090909090909091) / 2)
	closeTo(t, vol.Value, expected, 1e-9)
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
