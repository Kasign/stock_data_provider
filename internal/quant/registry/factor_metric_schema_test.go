package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/factor"
)

func TestFactorQuantMetricSchemaMatchesGoOutputAndGoldens(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-factor-quant-metric-1.0.0.schema.json")
	asOf := time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name   string
		result factor.Result
	}{
		{"momentum_20d-available", factor.Result{Code: "momentum_20d", Value: 0.2, Unit: "ratio", Status: factor.StatusAvailable, SampleCount: 21}},
		{"momentum_20d-insufficient", factor.Result{Code: "momentum_20d", Unit: "ratio", Status: factor.StatusInsufficientData, SampleCount: 20}},
		{"momentum_60d-available", factor.Result{Code: "momentum_60d", Value: 0.6, Unit: "ratio", Status: factor.StatusAvailable, SampleCount: 61}},
		{"momentum_60d-insufficient", factor.Result{Code: "momentum_60d", Unit: "ratio", Status: factor.StatusInsufficientData, SampleCount: 60}},
		{"volatility_20d-available", factor.Result{Code: "volatility_20d", Value: 0.012, Unit: "ratio", Status: factor.StatusAvailable, SampleCount: 20}},
		{"volatility_20d-insufficient", factor.Result{Code: "volatility_20d", Unit: "ratio", Status: factor.StatusInsufficientData, SampleCount: 19}},
	}
	goldenDir := filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "factor")
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := factor.NewQuantMetric(tc.result, asOf, factor.QualityValid, "1.0.0")
			if err != nil {
				t.Fatalf("NewQuantMetric: %v", err)
			}
			encoded, err := json.Marshal(got)
			if err != nil {
				t.Fatalf("marshal Factor QuantMetric: %v", err)
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
		})
	}
}
