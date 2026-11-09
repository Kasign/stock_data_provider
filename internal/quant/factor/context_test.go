package factor

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
)

func validFactorContext() domain.CalculationContext {
	return domain.CalculationContext{
		SystemAsOf:        time.Date(2026, 11, 9, 6, 0, 0, 0, time.UTC),
		TradeDate:         "2026-11-06",
		InputSnapshotRefs: []string{"snapshot:daily-bars:fixture"},
		AdjustmentBasis:   domain.AdjustmentBasisForward,
		QualityPolicy:     domain.QualityPolicyStrict,
	}
}

func TestFactorWithContextValidatesSharedCalculationContext(t *testing.T) {
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)
	closes21 := make([]float64, 21)
	for i := range closes21 {
		closes21[i] = 100 + float64(i)
	}
	closes61 := make([]float64, 61)
	for i := range closes61 {
		closes61[i] = 100 + float64(i)
	}

	tests := []struct {
		name string
		run  func(domain.CalculationContext) (Result, error)
		want string
	}{
		{"momentum_20d", func(ctx domain.CalculationContext) (Result, error) {
			return Momentum20WithContext(ctx, now, closes21)
		}, "momentum_20d"},
		{"momentum_60d", func(ctx domain.CalculationContext) (Result, error) {
			return Momentum60WithContext(ctx, now, closes61)
		}, "momentum_60d"},
		{"volatility_20d", func(ctx domain.CalculationContext) (Result, error) {
			return Volatility20WithContext(ctx, now, closes21)
		}, "volatility_20d"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.run(validFactorContext())
			if err != nil {
				t.Fatal(err)
			}
			if got.Code != tt.want || got.Status != StatusAvailable {
				t.Fatalf("unexpected result: %+v", got)
			}
		})
	}
}

func TestFactorWithContextRejectsUnknownAdjustmentBasis(t *testing.T) {
	ctx := validFactorContext()
	ctx.AdjustmentBasis = domain.AdjustmentBasisUnknown
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)

	_, err := Momentum20WithContext(ctx, now, []float64{100})
	if !errors.Is(err, domain.ErrUnknownAdjustmentBasis) {
		t.Fatalf("expected ErrUnknownAdjustmentBasis, got %v", err)
	}
}

func TestFactorWithContextRejectsMissingSnapshotReference(t *testing.T) {
	ctx := validFactorContext()
	ctx.InputSnapshotRefs = nil
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)

	_, err := Volatility20WithContext(ctx, now, []float64{100})
	if !errors.Is(err, domain.ErrMissingSnapshotRefs) {
		t.Fatalf("expected ErrMissingSnapshotRefs, got %v", err)
	}
}

func TestFactorWithContextRejectsFutureSystemAsOf(t *testing.T) {
	ctx := validFactorContext()
	ctx.SystemAsOf = time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)

	_, err := Momentum60WithContext(ctx, now, []float64{100})
	if !errors.Is(err, domain.ErrSystemAsOfInFuture) {
		t.Fatalf("expected ErrSystemAsOfInFuture, got %v", err)
	}
}
