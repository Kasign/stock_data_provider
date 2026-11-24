package indicator

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
)

func TestDecimalSMAExactFiniteResult(t *testing.T) {
	closes := make([]string, 20)
	for index := range closes {
		closes[index] = fmt.Sprintf("0.%02d", index+1)
	}
	got, err := DecimalSMA(closes, SMAParams{Window: 20})
	if err != nil {
		t.Fatalf("DecimalSMA() error = %v", err)
	}
	if got.Status != StatusAvailable || got.Value != "0.105" || got.SampleCount != 20 {
		t.Fatalf("DecimalSMA() = %+v, want exact 0.105 from 20 bars", got)
	}
}

func TestDecimalSMAUsesOnlyLastWindow(t *testing.T) {
	got, err := DecimalSMA([]string{"999", "1.00", "2.00", "3.00"}, SMAParams{Window: 3})
	if err != nil {
		t.Fatalf("DecimalSMA() error = %v", err)
	}
	if got.Value != "2" || got.SampleCount != 3 {
		t.Fatalf("DecimalSMA() = %+v, want value 2 and sample_count 3", got)
	}
}

func TestDecimalSMARejectsImplicitRounding(t *testing.T) {
	_, err := DecimalSMA([]string{"1", "1", "2"}, SMAParams{Window: 3})
	if !errors.Is(err, ErrNonTerminatingDecimal) {
		t.Fatalf("DecimalSMA() error = %v, want ErrNonTerminatingDecimal", err)
	}
}

func TestDecimalSMAInsufficientData(t *testing.T) {
	got, err := DecimalSMA([]string{"1.2", "1.4"}, SMAParams{Window: 3})
	if err != nil {
		t.Fatalf("DecimalSMA() error = %v", err)
	}
	if got.Status != StatusInsufficientData || got.SampleCount != 2 || got.Value != "" {
		t.Fatalf("DecimalSMA() = %+v, want insufficient_data with two observed bars and no value", got)
	}
}

func TestDecimalSMARejectsInvalidPrices(t *testing.T) {
	cases := [][]string{
		{"1", "0", "2"},
		{"1", "-2", "3"},
		{"1", "01.2", "3"},
		{"1", "1e2", "3"},
		{"1", "NaN", "3"},
		{"1", "", "3"},
		{"1", "1.2 ", "3"},
	}
	for _, closes := range cases {
		if _, err := DecimalSMA(closes, SMAParams{Window: 3}); !errors.Is(err, ErrInvalidInput) {
			t.Errorf("DecimalSMA(%q) error = %v, want ErrInvalidInput", closes, err)
		}
	}
}

func TestDecimalSMARejectsInvalidWindow(t *testing.T) {
	if _, err := DecimalSMA([]string{"1"}, SMAParams{Window: maxIndicatorWindow + 1}); !errors.Is(err, ErrInvalidParameter) {
		t.Fatalf("DecimalSMA() error = %v, want ErrInvalidParameter", err)
	}
}

func TestDecimalSMAWithContextRequiresTrustedSnapshotContext(t *testing.T) {
	now := time.Date(2026, 11, 24, 8, 0, 0, 0, time.UTC)
	calculation := domain.CalculationContext{
		SystemAsOf: now.Add(-time.Hour), TradeDate: "2026-11-23",
		InputSnapshotRefs: []string{"data_snapshot:sha256:" + strings.Repeat("a", 64)},
		AdjustmentBasis: domain.AdjustmentBasisNone,
		QualityPolicy: domain.QualityPolicyStrict,
	}
	got, err := DecimalSMAWithContext(calculation, now, []string{"1.01", "1.02", "1.03"}, SMAParams{Window: 3})
	if err != nil {
		t.Fatalf("DecimalSMAWithContext() error = %v", err)
	}
	if got.Value != "1.02" {
		t.Fatalf("DecimalSMAWithContext() value = %q, want 1.02", got.Value)
	}

	calculation.InputSnapshotRefs = nil
	if _, err := DecimalSMAWithContext(calculation, now, []string{"1.01", "1.02", "1.03"}, SMAParams{Window: 3}); !errors.Is(err, domain.ErrMissingSnapshotRefs) {
		t.Fatalf("DecimalSMAWithContext() error = %v, want ErrMissingSnapshotRefs", err)
	}
}
