package registry

import (
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/domain"
)

// TestCalculationContextAndRegistryGate 验证价格计算先通过上下文门禁，再解析明确的活动算法版本。
// 该测试只验证两个领域组件的组合边界，不定义新的 Application API 或版本选择策略。
func TestCalculationContextAndRegistryGate(t *testing.T) {
	r := New()
	entry := validEntry("momentum_20d", "1.0.0")
	if err := r.Register(entry, true); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	now := time.Date(2026, 11, 6, 8, 1, 0, 0, time.UTC)
	validContext := domain.CalculationContext{
		SystemAsOf:        now.Add(-time.Minute),
		TradeDate:         "2026-11-06",
		InputSnapshotRefs: []string{"data_snapshot:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		AdjustmentBasis:   domain.AdjustmentBasisForward,
		QualityPolicy:     domain.QualityPolicyStrict,
	}
	resolve := func(ctx domain.CalculationContext, code, schemaVersion string) (Entry, error) {
		if err := ctx.Validate(now, true); err != nil {
			return Entry{}, err
		}
		return r.LookupActive(code, schemaVersion)
	}

	resolved, err := resolve(validContext, "momentum_20d", "1.0.0")
	if err != nil {
		t.Fatalf("resolve() error = %v", err)
	}
	if resolved.Code != entry.Code || resolved.SchemaVersion != entry.SchemaVersion ||
		resolved.AlgorithmVersion != entry.AlgorithmVersion {
		t.Fatalf("resolved entry = %+v, want code/schema/algorithm %s/%s/%s",
			resolved, entry.Code, entry.SchemaVersion, entry.AlgorithmVersion)
	}

	invalidContext := validContext
	invalidContext.AdjustmentBasis = domain.AdjustmentBasisUnknown
	if _, err := resolve(invalidContext, "momentum_20d", "1.0.0"); !errors.Is(err, domain.ErrUnknownAdjustmentBasis) {
		t.Fatalf("expected invalid price context to fail before registry lookup, got %v", err)
	}

	if _, err := resolve(validContext, "momentum_20d", "9.0.0"); !errors.Is(err, ErrNoActiveVersion) {
		t.Fatalf("expected unknown schema version to have no active metric version, got %v", err)
	}
}

func TestCalculationContextAndRegistryGateRequiresExplicitActiveVersion(t *testing.T) {
	r := New()
	if err := r.Register(validEntry("momentum_20d", "1.0.0"), false); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	now := time.Date(2026, 11, 6, 8, 1, 0, 0, time.UTC)
	ctx := domain.CalculationContext{
		SystemAsOf:        now.Add(-time.Minute),
		TradeDate:         "2026-11-06",
		InputSnapshotRefs: []string{"snapshot-a"},
		AdjustmentBasis:   domain.AdjustmentBasisForward,
		QualityPolicy:     domain.QualityPolicyStrict,
	}
	if err := ctx.Validate(now, true); err != nil {
		t.Fatalf("valid context rejected: %v", err)
	}
	if _, err := r.LookupActive("momentum_20d", "1.0.0"); !errors.Is(err, ErrNoActiveVersion) {
		t.Fatalf("expected explicit active version to be required, got %v", err)
	}
}
