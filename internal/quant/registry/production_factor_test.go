package registry

import "testing"

func TestNewProductionRegistryRegistersDecimalFactors(t *testing.T) {
	registry, err := NewProductionRegistry()
	if err != nil {
		t.Fatalf("NewProductionRegistry() error = %v", err)
	}
	if !registry.IsFrozen() {
		t.Fatal("production Quant Registry must be frozen")
	}

	cases := []struct {
		code             string
		algorithmVersion string
		countingRule     string
	}{
		{ProductionMADistanceCode, ProductionMADistanceAlgorithmVersion, ProductionMADistanceCountingRule},
		{ProductionMomentum5Code, ProductionMomentum5AlgorithmVersion, ProductionMomentum5CountingRule},
		{ProductionMomentum20Code, ProductionMomentum20AlgorithmVersion, ProductionMomentum20CountingRule},
	}
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			entry, err := registry.LookupActive(tc.code, ProductionFactorSchemaVersion)
			if err != nil {
				t.Fatalf("LookupActive(%s) error = %v", tc.code, err)
			}
			if entry.AlgorithmVersion != tc.algorithmVersion ||
				entry.ValueType != ValueTypeDecimalString || entry.Unit != ProductionFactorUnit {
				t.Fatalf("%s identity metadata incomplete: %+v", tc.code, entry)
			}
			if entry.Precision.MaxIntegerDigits != 128 || entry.Precision.MaxFractionDigits != 12 ||
				entry.Precision.RoundingMode != "half_even" || string(entry.Precision.Tolerance) != productionFactorTolerance {
				t.Fatalf("%s precision strategy incomplete: %+v", tc.code, entry.Precision)
			}
			if !entry.Range.Unbounded || len(entry.Range.Minimum) != 0 || len(entry.Range.Maximum) != 0 {
				t.Fatalf("%s ratio range must be unbounded: %+v", tc.code, entry.Range)
			}
			if len(entry.InputRequirements) != 1 {
				t.Fatalf("%s input_requirements length = %d, want 1", tc.code, len(entry.InputRequirements))
			}
			requirement := entry.InputRequirements[0]
			if len(requirement.Fields) != 1 || requirement.Fields[0] != "close" ||
				requirement.Interval != "1d" || requirement.Adjustment != "none" ||
				!requirement.PIT || !requirement.RequiresSnapshot ||
				len(requirement.Quality) != 1 || requirement.Quality[0] != "valid" {
				t.Fatalf("%s input requirement must bind authorized close snapshot: %+v", tc.code, requirement)
			}
			if !entry.SampleCount.RequiredInMetric || entry.SampleCount.Unit != "bar" ||
				entry.SampleCount.CountingRule != tc.countingRule {
				t.Fatalf("%s sample_count policy incomplete: %+v", tc.code, entry.SampleCount)
			}
			if entry.QualityPolicy.MinimumCoverage != 1 || entry.QualityPolicy.MissingHandling != "reject" ||
				len(entry.QualityPolicy.UnavailableConditions) != 1 || entry.QualityPolicy.UnavailableConditions[0] != "insufficient_data" {
				t.Fatalf("%s quality policy incomplete: %+v", tc.code, entry.QualityPolicy)
			}
			if entry.SupportsInterval {
				t.Fatalf("%s must not claim multi-interval support", tc.code)
			}
		})
	}
}

func TestProductionFactorEntriesValidateIndividually(t *testing.T) {
	for _, entry := range productionFactorEntries() {
		if err := entry.Validate(); err != nil {
			t.Errorf("Entry %s failed Validate(): %v", entry.Code, err)
		}
	}
}
