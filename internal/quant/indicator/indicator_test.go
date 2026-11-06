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
