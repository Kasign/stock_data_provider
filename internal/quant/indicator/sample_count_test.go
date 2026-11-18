package indicator

import "testing"

// insufficient_data 时，sample_count 仍应按该指标的计数单位返回已观察样本数，
// 不能一律使用输入数组长度。
func TestInsufficientIndicatorSampleCountsUseMetricCountingUnit(t *testing.T) {
	tests := []struct {
		name string
		run  func() (Status, int, error)
		want int
	}{
		{
			name: "sma counts close bars",
			run: func() (Status, int, error) {
				got, err := SMA([]float64{1, 2}, SMAParams{Window: 3})
				return got.Status, got.SampleCount, err
			},
			want: 2,
		},
		{
			name: "ema counts close bars",
			run: func() (Status, int, error) {
				got, err := EMA([]float64{1, 2}, EMAParams{Window: 3, SeedMethod: "sma"})
				return got.Status, got.SampleCount, err
			},
			want: 2,
		},
		{
			name: "return counts observed close changes",
			run: func() (Status, int, error) {
				got, err := Return([]float64{1, 2, 3}, ReturnParams{Window: 3, ReturnBasis: "simple"})
				return got.Status, got.SampleCount, err
			},
			want: 2,
		},
		{
			name: "volatility counts observed returns",
			run: func() (Status, int, error) {
				got, err := Volatility([]float64{1, 2, 3}, VolatilityParams{Window: 3, ReturnBasis: "simple", AnnualizationFactor: 1})
				return got.Status, got.SampleCount, err
			},
			want: 2,
		},
		{
			name: "rsi counts observed close changes",
			run: func() (Status, int, error) {
				got, err := RSI([]float64{1, 2, 3}, RSIParams{Window: 3, SmoothingMethod: "wilder"})
				return got.Status, got.SampleCount, err
			},
			want: 2,
		},
		{
			name: "macd counts input close bars",
			run: func() (Status, int, error) {
				got, err := MACD([]float64{1, 2, 3}, MACDParams{FastWindow: 2, SlowWindow: 3, SignalWindow: 2})
				return got.Status, got.SampleCount, err
			},
			want: 3,
		},
		{
			name: "atr counts input ohlc bars",
			run: func() (Status, int, error) {
				got, err := ATR([]float64{2, 3}, []float64{1, 2}, []float64{1.5, 2.5}, 1, ATRParams{Window: 3, SmoothingMethod: "wilder"})
				return got.Status, got.SampleCount, err
			},
			want: 2,
		},
		{
			name: "bollinger counts close bars",
			run: func() (Status, int, error) {
				got, err := Bollinger([]float64{1, 2}, BollingerParams{Window: 3, Multiplier: 2, StdDevMethod: "population"})
				return got.Status, got.SampleCount, err
			},
			want: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, count, err := tc.run()
			if err != nil {
				t.Fatal(err)
			}
			if status != StatusInsufficientData {
				t.Fatalf("status=%s, want %s", status, StatusInsufficientData)
			}
			if count != tc.want {
				t.Fatalf("sample_count=%d, want %d", count, tc.want)
			}
		})
	}
}

func TestInsufficientIndicatorSampleCountsAreZeroForEmptyInputs(t *testing.T) {
	tests := []struct {
		name string
		run  func() (Status, int, error)
	}{
		{"sma", func() (Status, int, error) {
			got, err := SMA(nil, SMAParams{Window: 1})
			return got.Status, got.SampleCount, err
		}},
		{"ema", func() (Status, int, error) {
			got, err := EMA(nil, EMAParams{Window: 1, SeedMethod: "sma"})
			return got.Status, got.SampleCount, err
		}},
		{"return", func() (Status, int, error) {
			got, err := Return(nil, ReturnParams{Window: 1, ReturnBasis: "simple"})
			return got.Status, got.SampleCount, err
		}},
		{"volatility", func() (Status, int, error) {
			got, err := Volatility(nil, VolatilityParams{Window: 2, ReturnBasis: "simple", AnnualizationFactor: 1})
			return got.Status, got.SampleCount, err
		}},
		{"rsi", func() (Status, int, error) {
			got, err := RSI(nil, RSIParams{Window: 1, SmoothingMethod: "wilder"})
			return got.Status, got.SampleCount, err
		}},
		{"macd", func() (Status, int, error) {
			got, err := MACD(nil, MACDParams{FastWindow: 1, SlowWindow: 2, SignalWindow: 1})
			return got.Status, got.SampleCount, err
		}},
		{"atr", func() (Status, int, error) {
			got, err := ATR(nil, nil, nil, 1, ATRParams{Window: 1, SmoothingMethod: "wilder"})
			return got.Status, got.SampleCount, err
		}},
		{"bollinger", func() (Status, int, error) {
			got, err := Bollinger(nil, BollingerParams{Window: 2, Multiplier: 2, StdDevMethod: "population"})
			return got.Status, got.SampleCount, err
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			status, count, err := tc.run()
			if err != nil {
				t.Fatal(err)
			}
			if status != StatusInsufficientData {
				t.Fatalf("status=%s, want %s", status, StatusInsufficientData)
			}
			if count != 0 {
				t.Fatalf("sample_count=%d, want 0 for empty input", count)
			}
		})
	}
}
