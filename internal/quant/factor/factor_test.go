package factor

import (
	"errors"
	"math"
	"testing"
)

func assertNear(t *testing.T, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-12 {
		t.Fatalf("got %.12f, want %.12f", got, want)
	}
}

func TestMomentum20Uses21Closes(t *testing.T) {
	closes := make([]float64, 21)
	for i := range closes {
		closes[i] = 100 + float64(i)
	}
	got, err := Momentum20(closes)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAvailable || got.SampleCount != 21 || got.Unit != "ratio" {
		t.Fatalf("unexpected result: %+v", got)
	}
	assertNear(t, got.Value, 120.0/100.0-1)
}

func TestMomentum60Requires61Closes(t *testing.T) {
	closes := make([]float64, 60)
	for i := range closes {
		closes[i] = float64(i + 1)
	}
	got, err := Momentum60(closes)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusInsufficientData {
		t.Fatalf("unexpected status: %+v", got)
	}

	closes = append(closes, 61)
	got, err = Momentum60(closes)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAvailable || got.SampleCount != 61 {
		t.Fatalf("unexpected result: %+v", got)
	}
	assertNear(t, got.Value, 61.0/1.0-1)
}

func TestVolatility20UsesSampleStdAndIsNotAnnualized(t *testing.T) {
	closes := make([]float64, 21)
	for i := range closes {
		closes[i] = 100 * math.Pow(1.01, float64(i))
	}
	got, err := Volatility20(closes)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusAvailable || got.SampleCount != 20 || got.Unit != "ratio" {
		t.Fatalf("unexpected result: %+v", got)
	}
	assertNear(t, got.Value, 0)
}

func TestVolatility20AvoidsIntermediateOverflowForFiniteReturns(t *testing.T) {
	closes := make([]float64, 21)
	for i := range closes {
		if i%2 == 0 {
			closes[i] = 1e-308
		} else {
			closes[i] = 1
		}
	}

	got, err := Volatility20(closes)
	if err != nil {
		t.Fatalf("Volatility20() rejected finite returns: %v", err)
	}
	want := math.Sqrt(5.0/19.0) * 1e308
	if math.IsNaN(got.Value) || math.IsInf(got.Value, 0) || math.Abs(got.Value-want)/want > 1e-12 {
		t.Fatalf("Volatility20() = %.17g, want approximately %.17g", got.Value, want)
	}
}

func TestAllFactorsReportInsufficientDataBeforeWarmup(t *testing.T) {
	positiveCloses := func(n int) []float64 {
		closes := make([]float64, n)
		for i := range closes {
			closes[i] = 100 + float64(i)
		}
		return closes
	}
	tests := []struct {
		name string
		run  func() (Status, error)
	}{
		{"momentum_20d", func() (Status, error) {
			got, err := Momentum20(positiveCloses(20))
			return got.Status, err
		}},
		{"momentum_60d", func() (Status, error) {
			got, err := Momentum60(positiveCloses(60))
			return got.Status, err
		}},
		{"volatility_20d", func() (Status, error) {
			got, err := Volatility20(positiveCloses(20))
			return got.Status, err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, err := tt.run()
			if err != nil {
				t.Fatal(err)
			}
			if status != StatusInsufficientData {
				t.Fatalf("expected insufficient_data, got %s", status)
			}
		})
	}
}

func TestFactorsRejectOverflowFromFinitePositivePrices(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{"momentum_20d", func() error {
			closes := make([]float64, 21)
			for i := range closes {
				closes[i] = 1
			}
			closes[0] = math.SmallestNonzeroFloat64
			closes[20] = math.MaxFloat64
			_, err := Momentum20(closes)
			return err
		}},
		{"momentum_60d", func() error {
			closes := make([]float64, 61)
			for i := range closes {
				closes[i] = 1
			}
			closes[0] = math.SmallestNonzeroFloat64
			closes[60] = math.MaxFloat64
			_, err := Momentum60(closes)
			return err
		}},
		{"volatility_20d", func() error {
			closes := make([]float64, 21)
			for i := range closes {
				closes[i] = 1
			}
			closes[0] = math.SmallestNonzeroFloat64
			closes[1] = math.MaxFloat64
			_, err := Volatility20(closes)
			return err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(); !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("expected ErrInvalidInput for finite positive inputs, got %v", err)
			}
		})
	}
}

func TestFactorRejectsInvalidPrices(t *testing.T) {
	if _, err := Momentum20([]float64{1, 0}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
