package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/factor"
)

// TestDecimalFactorQuantMetricSchemaMatchesGoOutputAndGoldens 验证新增 Decimal QuantMetric 的
// Go 输出与结果级 Golden 都符合 additive 修订后的 QNT-CTR-004 Schema，且值以 JSON string 承载。
func TestDecimalFactorQuantMetricSchemaMatchesGoOutputAndGoldens(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-factor-quant-metric-1.0.0.schema.json")
	asOf := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)

	cases := []struct {
		name   string
		result factor.DecimalResult
	}{
		{
			name:   "ma20_distance-available",
			result: factor.DecimalResult{Code: "ma20_distance", Value: "0.904761904762", Status: factor.StatusAvailable, SampleCount: 20},
		},
		{
			name:   "ma20_distance-insufficient",
			result: factor.DecimalResult{Code: "ma20_distance", Status: factor.StatusInsufficientData, SampleCount: 19},
		},
		{
			name:   "momentum_5d-available",
			result: factor.DecimalResult{Code: "momentum_5d", Value: "0.052631578947", Status: factor.StatusAvailable, SampleCount: 6},
		},
		{
			name:   "momentum_5d-insufficient",
			result: factor.DecimalResult{Code: "momentum_5d", Status: factor.StatusInsufficientData, SampleCount: 5},
		},
		{
			name:   "momentum_20d-available",
			result: factor.DecimalResult{Code: "momentum_20d", Value: "0.2", Status: factor.StatusAvailable, SampleCount: 21},
		},
		{
			name:   "momentum_20d-insufficient",
			result: factor.DecimalResult{Code: "momentum_20d", Status: factor.StatusInsufficientData, SampleCount: 20},
		},
	}
	goldenDir := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "factor", "decimal")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			metric, err := factor.NewDecimalQuantMetric(tc.result, asOf, factor.QualityValid, "1.0.0")
			if err != nil {
				t.Fatalf("NewDecimalQuantMetric: %v", err)
			}
			encoded, err := json.Marshal(metric)
			if err != nil {
				t.Fatalf("marshal Decimal QuantMetric: %v", err)
			}
			if err := validateJSONSchemaInstance(schema, encoded); err != nil {
				t.Fatalf("Go output violates QNT-CTR-004 Factor QuantMetric Schema: %v; JSON: %s", err, encoded)
			}

			goldenPath := filepath.Join(goldenDir, tc.name+".golden.json")
			golden, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read Golden %s: %v", goldenPath, err)
			}
			if err := validateJSONSchemaInstance(schema, golden); err != nil {
				t.Fatalf("Golden violates QNT-CTR-004 Factor QuantMetric Schema: %v; JSON: %s", err, golden)
			}

			// Go 输出与 Golden 语义一致（比较解码后的对象，避免时间格式字节差异）。
			var gotObject, goldenObject map[string]any
			if err := json.Unmarshal(encoded, &gotObject); err != nil {
				t.Fatalf("decode Go output: %v", err)
			}
			if err := json.Unmarshal(golden, &goldenObject); err != nil {
				t.Fatalf("decode Golden: %v", err)
			}
			for key, want := range goldenObject {
				got, ok := gotObject[key]
				if !ok {
					t.Errorf("Go output missing field %q", key)
					continue
				}
				if got != want {
					t.Errorf("field %q: got %v (%T), want %v (%T)", key, got, got, want, want)
				}
			}
		})
	}
}
