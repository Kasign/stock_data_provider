package factor

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
)

func TestNewQuantMetricWithContextAppliesQualityPolicy(t *testing.T) {
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)
	result := Result{
		Code: "momentum_20d", Value: 0.12, Unit: "ratio",
		Status: StatusAvailable, SampleCount: 21,
	}
	ctx := validFactorContext()

	metric, err := NewQuantMetricWithContext(ctx, now, result, QualityValid, "1.0.0")
	if err != nil {
		t.Fatalf("valid strict result rejected: %v", err)
	}
	if metric.Value == nil || *metric.Value != result.Value || metric.SystemAsOf != ctx.SystemAsOf.Format(time.RFC3339Nano) {
		t.Fatalf("unexpected metric projection: %+v", metric)
	}

	_, err = NewQuantMetricWithContext(ctx, now, result, QualityDegraded, "1.0.0")
	if !errors.Is(err, ErrQualityPolicyRejected) {
		t.Fatalf("strict policy should reject degraded result, got %v", err)
	}

	ctx.QualityPolicy = domain.QualityPolicyAllowDegraded
	metric, err = NewQuantMetricWithContext(ctx, now, result, QualityDegraded, "1.0.0")
	if err != nil {
		t.Fatalf("allow_degraded policy rejected degraded result: %v", err)
	}
	if metric.QualityStatus != QualityDegraded || metric.Value == nil {
		t.Fatalf("degraded result quality was not preserved: %+v", metric)
	}
}

func TestNewQuantMetricWithContextKeepsInsufficientDataUnavailable(t *testing.T) {
	ctx := validFactorContext()
	now := time.Date(2026, 11, 9, 7, 0, 0, 0, time.UTC)
	result := Result{
		Code: "momentum_20d", Unit: "ratio",
		Status: StatusInsufficientData, SampleCount: 12,
	}

	metric, err := NewQuantMetricWithContext(ctx, now, result, QualityUnknown, "1.0.0")
	if err != nil {
		t.Fatalf("insufficient data should remain representable: %v", err)
	}
	if metric.Value != nil || metric.MetricStatus != StatusInsufficientData || metric.QualityStatus != QualityUnknown {
		t.Fatalf("insufficient data was not preserved as unavailable: %+v", metric)
	}
}
