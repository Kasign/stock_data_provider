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

	invalidMetrics := []string{
		`{"metric_code":"sample","value":null,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`,
		`{"metric_code":"sample","value":0.2,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"insufficient_sample","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`,
		`{"metric_code":"sample","value":0.2,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"unknown","metric_status":"available","calculation_version":"1.0.0","lower_bound":null,"upper_bound":null}`,
		`{"metric_code":"sample","value":0.2,"unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":0.1,"upper_bound":null}`,
		`{"metric_code":"sample","value":"0.2","unit":"ratio","period":"20d","system_as_of":"2026-11-09T01:00:00Z","quality_status":"valid","metric_status":"available","calculation_version":"1.0.0","lower_bound":0.1,"upper_bound":0.3}`,
	}
	for _, metric := range invalidMetrics {
		instance := `{"trend_code":"price_trend","direction":"unknown","phase":"emerging","strength":0.2,"period":"20d","evidence_metrics":[` + metric + `],"conflict_refs":[],"quality_status":"valid","system_as_of":"2026-11-09T01:00:00Z","algorithm_version":"1.0.0"}`
		if err := validateJSONSchemaInstance(schema, []byte(instance)); err == nil {
			t.Errorf("invalid evidence metric unexpectedly passed: %s", metric)
		}
	}
}
