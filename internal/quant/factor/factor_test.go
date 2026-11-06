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

func TestFactorRejectsInvalidPrices(t *testing.T) {
	if _, err := Momentum20([]float64{1, 0}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
