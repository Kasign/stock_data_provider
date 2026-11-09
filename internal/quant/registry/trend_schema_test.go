package registry

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTrendResultSchemaValidatesGoldenAndRejectsInvalidDiscriminators(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-trend-result-1.0.0.schema.json")
	path := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "trend-result-1.0.0.golden.json")
	golden, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read TrendResult Golden: %v", err)
	}
	if err := validateJSONSchemaInstance(schema, golden); err != nil {
		t.Fatalf("TrendResult Golden violates schema: %v", err)
	}

	invalid := []string{
		`{"trend_code":"price_trend","direction":"unknownish","phase":"emerging","strength":0.2,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`,
		`{"trend_code":"price_trend","direction":"bullish","phase":"sideways","strength":0.2,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`,
		`{"trend_code":"price_trend","direction":"unknown","phase":"emerging","strength":1.01,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`,
		`{"trend_code":"price_trend","direction":"unknown","phase":"emerging","strength":0.2,"period":"20d","evidence_metrics":[],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00.000Z","algorithm_version":"1.0.0"}`,
	}
	for _, instance := range invalid {
		if err := validateJSONSchemaInstance(schema, []byte(instance)); err == nil {
			t.Errorf("invalid TrendResult unexpectedly passed: %s", instance)
		}
	}
}
