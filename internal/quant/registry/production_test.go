package registry

import "testing"

func TestNewProductionRegistryFreezesExactSMA20Entry(t *testing.T) {
	registry, err := NewProductionRegistry()
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}
	if !registry.IsFrozen() {
		t.Fatal("production Quant Registry must be frozen")
	}
	entry, err := registry.LookupActive(ProductionSMA20Code, ProductionSMA20SchemaVersion)
	if err != nil {
		t.Fatalf("LookupActive() error = %v", err)
	}
	if entry.AlgorithmVersion != ProductionSMA20AlgorithmVersion ||
		entry.ValueType != ValueTypeDecimalString || entry.Unit != ProductionSMA20Unit ||
		entry.Precision.RoundingMode != "exact" || string(entry.Precision.Tolerance) != "\"0\"" ||
		entry.Precision.MaxIntegerDigits != 128 || entry.Precision.MaxFractionDigits != 128 ||
		entry.SampleCount.CountingRule != "window_bars" || !entry.SampleCount.RequiredInMetric {
		t.Fatalf("production SMA20 Registry Entry is incomplete: %+v", entry)
	}
	if err := registry.Register(entry, false); err != ErrRegistryFrozen {
		t.Fatalf("Register() error = %v, want ErrRegistryFrozen", err)
	}
}
