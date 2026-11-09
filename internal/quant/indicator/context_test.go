package indicator

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
)

func validIndicatorContext() domain.CalculationContext {
	return domain.CalculationContext{
		SystemAsOf:        time.Date(2026, 11, 9, 6, 0, 0, 0, time.UTC),
		TradeDate:         "2026-11-06",
		InputSnapshotRefs: []string{"snapshot:daily-bars:fixture"},
		AdjustmentBasis:   domain.AdjustmentBasisForward,
		QualityPolicy:     domain.QualityPolicyStrict,
	}
}

func TestIndicatorWithContextValidatesSharedCalculationContext(t *testing.T) {
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)
	closes := make([]float64, 40)
	highs := make([]float64, 40)
	lows := make([]float64, 40)
	for i := range closes {
		closes[i] = 100 + float64(i)
		highs[i] = closes[i] + 1
		lows[i] = closes[i] - 1
	}

	tests := []struct {
		name string
		run  func(domain.CalculationContext) error
	}{
		{"sma", func(ctx domain.CalculationContext) error {
			_, err := SMAWithContext(ctx, now, closes, SMAParams{Window: 3})
			return err
		}},
		{"ema", func(ctx domain.CalculationContext) error {
			_, err := EMAWithContext(ctx, now, closes, EMAParams{Window: 3, SeedMethod: "sma"})
			return err
		}},
		{"return", func(ctx domain.CalculationContext) error {
			_, err := ReturnWithContext(ctx, now, closes, ReturnParams{Window: 2, ReturnBasis: "simple"})
			return err
		}},
		{"volatility", func(ctx domain.CalculationContext) error {
			_, err := VolatilityWithContext(ctx, now, closes, VolatilityParams{Window: 2, ReturnBasis: "simple", AnnualizationFactor: 252})
			return err
		}},
		{"rsi", func(ctx domain.CalculationContext) error {
			_, err := RSIWithContext(ctx, now, closes, RSIParams{Window: 2, SmoothingMethod: "wilder"})
			return err
		}},
		{"macd", func(ctx domain.CalculationContext) error {
			_, err := MACDWithContext(ctx, now, closes, MACDParams{FastWindow: 2, SlowWindow: 3, SignalWindow: 2})
			return err
		}},
		{"atr", func(ctx domain.CalculationContext) error {
			_, err := ATRWithContext(ctx, now, highs, lows, closes, 100, ATRParams{Window: 2, SmoothingMethod: "wilder"})
			return err
		}},
		{"bollinger", func(ctx domain.CalculationContext) error {
			_, err := BollingerWithContext(ctx, now, closes, BollingerParams{Window: 2, Multiplier: 2, StdDevMethod: "population"})
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.run(validIndicatorContext()); err != nil {
				t.Fatalf("calculation with valid context failed: %v", err)
			}
		})
	}
}

func TestIndicatorWithContextRejectsUnknownAdjustmentBasis(t *testing.T) {
	ctx := validIndicatorContext()
	ctx.AdjustmentBasis = domain.AdjustmentBasisUnknown
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)

	_, err := SMAWithContext(ctx, now, []float64{100}, SMAParams{Window: 1})
	if !errors.Is(err, domain.ErrUnknownAdjustmentBasis) {
		t.Fatalf("expected ErrUnknownAdjustmentBasis, got %v", err)
	}
}

func TestIndicatorWithContextRejectsMissingSnapshotReference(t *testing.T) {
	ctx := validIndicatorContext()
	ctx.InputSnapshotRefs = nil
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)

	_, err := BollingerWithContext(ctx, now, []float64{100}, BollingerParams{Window: 2, Multiplier: 2, StdDevMethod: "population"})
	if !errors.Is(err, domain.ErrMissingSnapshotRefs) {
		t.Fatalf("expected ErrMissingSnapshotRefs, got %v", err)
	}
}

func TestIndicatorWithContextRejectsFutureSystemAsOf(t *testing.T) {
	ctx := validIndicatorContext()
	ctx.SystemAsOf = time.Date(2026, 11, 9, 8, 0, 0, 0, time.UTC)
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)

	_, err := MACDWithContext(ctx, now, []float64{100}, MACDParams{FastWindow: 1, SlowWindow: 2, SignalWindow: 1})
	if !errors.Is(err, domain.ErrSystemAsOfInFuture) {
		t.Fatalf("expected ErrSystemAsOfInFuture, got %v", err)
	}
}
