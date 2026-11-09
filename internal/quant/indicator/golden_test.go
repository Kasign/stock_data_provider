package indicator

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
)

type indicatorGolden struct {
	FixtureType      string `json:"fixture_type"`
	SchemaVersion    string `json:"schema_version"`
	IndicatorCode    string `json:"indicator_code"`
	AlgorithmVersion string `json:"algorithm_version"`
	Parameters       struct {
		Window              int     `json:"window"`
		SeedMethod          string  `json:"seed_method"`
		ReturnBasis         string  `json:"return_basis"`
		AnnualizationFactor int `json:"annualization_factor"`
		SmoothingMethod     string  `json:"smoothing_method"`
		FastWindow          int     `json:"fast_window"`
		SlowWindow          int     `json:"slow_window"`
		SignalWindow        int     `json:"signal_window"`
		Multiplier          float64 `json:"multiplier"`
		StdDevMethod        string  `json:"stddev_method"`
	} `json:"parameters"`
	Input struct {
		Close         []float64 `json:"close"`
		High          []float64 `json:"high"`
		Low           []float64 `json:"low"`
		PreviousClose float64   `json:"previous_close"`
	} `json:"input"`
	Expected struct {
		Value       float64 `json:"value"`
		SampleCount int     `json:"sample_count"`
		DIF       float64 `json:"dif"`
		DEA       float64 `json:"dea"`
		Histogram float64 `json:"histogram"`
		Middle    float64 `json:"middle"`
		Upper     float64 `json:"upper"`
		Lower     float64 `json:"lower"`
	} `json:"expected"`
	Tolerance float64 `json:"tolerance"`
}

func TestIndicatorGoldenFixtures(t *testing.T) {
	codes := []string{"sma", "ema", "return", "volatility", "rsi", "macd", "atr", "bollinger"}
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			path := filepath.Join("../../../doc/quant/golden/indicator", code+".golden.json")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read golden fixture: %v", err)
			}
			var fixture indicatorGolden
			if err := json.Unmarshal(raw, &fixture); err != nil {
				t.Fatalf("decode golden fixture: %v", err)
			}
			if fixture.FixtureType != "synthetic_fixture" || fixture.SchemaVersion != "1.0.0" ||
				fixture.AlgorithmVersion != "1.0.0" || fixture.IndicatorCode != code {
				t.Fatalf("unexpected fixture metadata: %+v", fixture)
			}
			if fixture.Tolerance <= 0 || math.IsNaN(fixture.Tolerance) || math.IsInf(fixture.Tolerance, 0) {
				t.Fatalf("invalid tolerance: %v", fixture.Tolerance)
			}

			closeExpected := func(name string, got, want float64) {
				t.Helper()
				if math.IsNaN(got) || math.IsInf(got, 0) || math.Abs(got-want) > fixture.Tolerance {
					t.Fatalf("%s: got %.15g, want %.15g (tolerance %.3g)", name, got, want, fixture.Tolerance)
				}
			}
			assertAvailable := func(status Status, sampleCount, wantSampleCount int) {
				t.Helper()
				if status != StatusAvailable {
					t.Fatalf("status: got %q, want %q", status, StatusAvailable)
				}
				if sampleCount != wantSampleCount {
					t.Fatalf("sample_count: got %d, want %d", sampleCount, wantSampleCount)
				}
			}

			switch code {
			case "sma":
				got, err := SMA(fixture.Input.Close, SMAParams{Window: fixture.Parameters.Window})
				if err != nil {
					t.Fatal(err)
				}
				assertAvailable(got.Status, got.SampleCount, fixture.Parameters.Window)
				closeExpected("value", got.Value, fixture.Expected.Value)
			case "ema":
				got, err := EMA(fixture.Input.Close, EMAParams{Window: fixture.Parameters.Window, SeedMethod: fixture.Parameters.SeedMethod})
				if err != nil {
					t.Fatal(err)
				}
				assertAvailable(got.Status, got.SampleCount, len(fixture.Input.Close))
				closeExpected("value", got.Value, fixture.Expected.Value)
			case "return":
				got, err := Return(fixture.Input.Close, ReturnParams{Window: fixture.Parameters.Window, ReturnBasis: fixture.Parameters.ReturnBasis})
				if err != nil {
					t.Fatal(err)
				}
				assertAvailable(got.Status, got.SampleCount, fixture.Parameters.Window)
				closeExpected("value", got.Value, fixture.Expected.Value)
			case "volatility":
				got, err := Volatility(fixture.Input.Close, VolatilityParams{Window: fixture.Parameters.Window, ReturnBasis: fixture.Parameters.ReturnBasis, AnnualizationFactor: fixture.Parameters.AnnualizationFactor})
				if err != nil {
					t.Fatal(err)
				}
				assertAvailable(got.Status, got.SampleCount, fixture.Parameters.Window)
				closeExpected("value", got.Value, fixture.Expected.Value)
			case "rsi":
				got, err := RSI(fixture.Input.Close, RSIParams{Window: fixture.Parameters.Window, SmoothingMethod: fixture.Parameters.SmoothingMethod})
				if err != nil {
					t.Fatal(err)
				}
				assertAvailable(got.Status, got.SampleCount, len(fixture.Input.Close)-1)
				closeExpected("value", got.Value, fixture.Expected.Value)
			case "macd":
				got, err := MACD(fixture.Input.Close, MACDParams{FastWindow: fixture.Parameters.FastWindow, SlowWindow: fixture.Parameters.SlowWindow, SignalWindow: fixture.Parameters.SignalWindow})
				if err != nil {
					t.Fatal(err)
				}
				closeExpected("dif", got.DIF, fixture.Expected.DIF)
				closeExpected("dea", got.DEA, fixture.Expected.DEA)
				assertAvailable(got.Status, got.SampleCount, len(fixture.Input.Close))
				closeExpected("histogram", got.Histogram, fixture.Expected.Histogram)
			case "atr":
				got, err := ATR(fixture.Input.High, fixture.Input.Low, fixture.Input.Close, fixture.Input.PreviousClose, ATRParams{Window: fixture.Parameters.Window, SmoothingMethod: fixture.Parameters.SmoothingMethod})
				if err != nil {
					t.Fatal(err)
				}
				assertAvailable(got.Status, got.SampleCount, len(fixture.Input.Close))
				closeExpected("value", got.Value, fixture.Expected.Value)
			case "bollinger":
				got, err := Bollinger(fixture.Input.Close, BollingerParams{Window: fixture.Parameters.Window, Multiplier: fixture.Parameters.Multiplier, StdDevMethod: fixture.Parameters.StdDevMethod})
				if err != nil {
					t.Fatal(err)
				}
				closeExpected("middle", got.Middle, fixture.Expected.Middle)
				closeExpected("upper", got.Upper, fixture.Expected.Upper)
				assertAvailable(got.Status, got.SampleCount, fixture.Parameters.Window)
				closeExpected("lower", got.Lower, fixture.Expected.Lower)
			default:
				t.Fatal(fmt.Errorf("unsupported indicator code %q", code))
			}
		})
	}
}
