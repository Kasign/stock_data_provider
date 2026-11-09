package domain

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func validCalculationContext() CalculationContext {
	return CalculationContext{
		SystemAsOf:        time.Date(2026, 11, 6, 8, 0, 0, 0, time.UTC),
		TradeDate:         "2026-11-06",
		InputSnapshotRefs: []string{"data_snapshot:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		AdjustmentBasis:   AdjustmentBasisForward,
		QualityPolicy:     QualityPolicyStrict,
	}
}

func TestCalculationContextValidateAcceptsValidPriceContext(t *testing.T) {
	ctx := validCalculationContext()
	now := time.Date(2026, 11, 6, 8, 1, 0, 0, time.UTC)
	if err := ctx.Validate(now, true); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestCalculationContextValidateRejectsMissingOrFutureSystemAsOf(t *testing.T) {
	now := time.Date(2026, 11, 6, 8, 1, 0, 0, time.UTC)

	ctx := validCalculationContext()
	ctx.SystemAsOf = time.Time{}
	if err := ctx.Validate(now, false); !errors.Is(err, ErrMissingSystemAsOf) {
		t.Fatalf("expected ErrMissingSystemAsOf, got %v", err)
	}

	ctx = validCalculationContext()
	ctx.SystemAsOf = now.Add(time.Second)
	if err := ctx.Validate(now, false); !errors.Is(err, ErrSystemAsOfInFuture) {
		t.Fatalf("expected ErrSystemAsOfInFuture, got %v", err)
	}
}

func TestCalculationContextValidateRequiresUTC(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	ctx := validCalculationContext()
	ctx.SystemAsOf = time.Date(2026, 11, 6, 16, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	if err := ctx.Validate(now, false); !errors.Is(err, ErrSystemAsOfNotUTC) {
		t.Fatalf("expected ErrSystemAsOfNotUTC, got %v", err)
	}

	ctx.SystemAsOf = time.Date(2026, 11, 6, 8, 0, 0, 0, time.FixedZone("UTC+0", 0))
	if err := ctx.Validate(now, false); !errors.Is(err, ErrSystemAsOfNotUTC) {
		t.Fatalf("expected zero-offset non-UTC location to be rejected, got %v", err)
	}
}

func TestCalculationContextValidateChecksTradeDateFormat(t *testing.T) {
	ctx := validCalculationContext()
	ctx.TradeDate = "2026-2-06"
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	if err := ctx.Validate(now, false); !errors.Is(err, ErrInvalidTradeDate) {
		t.Fatalf("expected ErrInvalidTradeDate, got %v", err)
	}
}

func TestCalculationContextValidateRequiresUniqueSnapshotRefs(t *testing.T) {
	ctx := validCalculationContext()
	ctx.InputSnapshotRefs = []string{"snapshot-a", "snapshot-a"}
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	if err := ctx.Validate(now, false); !errors.Is(err, ErrDuplicateSnapshotRef) {
		t.Fatalf("expected ErrDuplicateSnapshotRef, got %v", err)
	}

	ctx.InputSnapshotRefs = []string{"  "}
	if err := ctx.Validate(now, false); !errors.Is(err, ErrInvalidSnapshotRef) {
		t.Fatalf("expected ErrInvalidSnapshotRef, got %v", err)
	}
}

func TestCalculationContextValidateEnforcesSnapshotRefBounds(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	ctx := validCalculationContext()

	ctx.InputSnapshotRefs = []string{" snapshot-a"}
	if err := ctx.Validate(now, false); !errors.Is(err, ErrInvalidSnapshotRef) {
		t.Fatalf("expected leading whitespace to be rejected, got %v", err)
	}

	ctx.InputSnapshotRefs = []string{"snapshot-a "}
	if err := ctx.Validate(now, false); !errors.Is(err, ErrInvalidSnapshotRef) {
		t.Fatalf("expected trailing whitespace to be rejected, got %v", err)
	}

	ctx.InputSnapshotRefs = []string{strings.Repeat("a", 129)}
	if err := ctx.Validate(now, false); !errors.Is(err, ErrInvalidSnapshotRef) {
		t.Fatalf("expected reference longer than 128 bytes to be rejected, got %v", err)
	}

	ctx.InputSnapshotRefs = []string{strings.Repeat("a", 128)}
	if err := ctx.Validate(now, false); err != nil {
		t.Fatalf("expected 128-byte reference to be accepted, got %v", err)
	}
}

func TestCalculationContextValidateRequiresComparablePriceBasis(t *testing.T) {
	ctx := validCalculationContext()
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

	ctx.AdjustmentBasis = ""
	if err := ctx.Validate(now, true); !errors.Is(err, ErrMissingAdjustmentBasis) {
		t.Fatalf("expected ErrMissingAdjustmentBasis, got %v", err)
	}

	ctx.AdjustmentBasis = AdjustmentBasisUnknown
	if err := ctx.Validate(now, true); !errors.Is(err, ErrUnknownAdjustmentBasis) {
		t.Fatalf("expected ErrUnknownAdjustmentBasis, got %v", err)
	}
}

func TestCalculationContextValidateRequiresQualityPolicy(t *testing.T) {
	ctx := validCalculationContext()
	ctx.QualityPolicy = ""
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	if err := ctx.Validate(now, false); !errors.Is(err, ErrInvalidQualityPolicy) {
		t.Fatalf("expected ErrInvalidQualityPolicy, got %v", err)
	}
}
