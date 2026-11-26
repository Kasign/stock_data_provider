package stage

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/registry"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

const factorStageTestResultID = "quant_result:sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestFactorStageOutputSchemaAndValidatorAcceptProductionCases(t *testing.T) {
	outputSchema := compileStageSchema(t, "QNT-CTR-003-factor-stage-output-1.0.0.schema.json")
	qualitySchema := compileStageSchema(t, "QNT-CTR-003-factor-quality-summary-1.0.0.schema.json")
	cases := []struct {
		code         string
		period       string
		sampleCount  int64
		value        json.RawMessage
		metricStatus string
		resultStatus string
	}{
		{"ma20_distance", "20d", 20, json.RawMessage(`"0.052631578947"`), "available", "completed"},
		{"momentum_5d", "5d", 6, json.RawMessage(`"-0.125"`), "available", "completed"},
		{"momentum_20d", "20d", 21, json.RawMessage(`"0"`), "available", "completed"},
		{"ma20_distance", "20d", 19, json.RawMessage(`null`), "insufficient_data", "insufficient_data"},
		{"momentum_5d", "5d", 5, json.RawMessage(`null`), "insufficient_data", "insufficient_data"},
		{"momentum_20d", "20d", 20, json.RawMessage(`null`), "insufficient_data", "insufficient_data"},
	}
	for _, tc := range cases {
		t.Run(tc.code+"_"+tc.metricStatus, func(t *testing.T) {
			output := FactorOutput{
				SchemaVersion:      FactorSchemaVersion,
				ResultID:           factorStageTestResultID,
				SnapshotRef:        "snapshot:factor-stage-schema",
				MetricCode:         tc.code,
				Value:              tc.value,
				Unit:               "ratio",
				Period:             tc.period,
				MetricStatus:       tc.metricStatus,
				SampleCount:        tc.sampleCount,
				TradeDate:          "2026-11-26",
				SystemAsOf:         "2026-11-26T06:00:00Z",
				QualityStatus:      "valid",
				ResultStatus:       tc.resultStatus,
				CalculationVersion: "1.0.0",
			}
			encoded := mustMarshalStageJSON(t, output)
			if err := ValidateFactorStageOutputJSON(encoded); err != nil {
				t.Fatalf("runtime validator rejected valid output: %v; JSON: %s", err, encoded)
			}
			validateStageSchemaInstance(t, outputSchema, encoded)

			quality := FactorQualitySummary{
				SchemaVersion:         FactorQualitySchemaVersion,
				MetricCode:            tc.code,
				QualityStatus:         "valid",
				CalendarQualityStatus: "valid",
				ExpectedTradingDays:   30,
				ObservedBars:          int(tc.sampleCount),
				MinimumRequiredBars:   1,
				CoverageSufficient:    true,
			}
			qualityJSON := mustMarshalStageJSON(t, quality)
			if err := ValidateFactorQualitySummaryJSON(qualityJSON); err != nil {
				t.Fatalf("runtime validator rejected valid quality summary: %v; JSON: %s", err, qualityJSON)
			}
			validateStageSchemaInstance(t, qualitySchema, qualityJSON)
		})
	}
}

func TestFactorStageOutputValidatorRejectsContractViolations(t *testing.T) {
	base := FactorOutput{
		SchemaVersion:      FactorSchemaVersion,
		ResultID:           factorStageTestResultID,
		SnapshotRef:        "snapshot:factor-stage-schema",
		MetricCode:         "momentum_5d",
		Value:              json.RawMessage(`"0.052631578947"`),
		Unit:               "ratio",
		Period:             "5d",
		MetricStatus:       "available",
		SampleCount:        6,
		TradeDate:          "2026-11-26",
		SystemAsOf:         "2026-11-26T06:00:00Z",
		QualityStatus:      "valid",
		ResultStatus:       "completed",
		CalculationVersion: "1.0.0",
	}
	mutations := []struct {
		name   string
		mutate func(*FactorOutput)
	}{
		{"unknown schema version", func(o *FactorOutput) { o.SchemaVersion = "2.0.0" }},
		{"derived result id", func(o *FactorOutput) { o.ResultID = "factor-result-1" }},
		{"blank snapshot ref", func(o *FactorOutput) { o.SnapshotRef = " snapshot " }},
		{"unsupported metric code", func(o *FactorOutput) { o.MetricCode = "momentum_60d" }},
		{"wrong period", func(o *FactorOutput) { o.Period = "20d" }},
		{"numeric value", func(o *FactorOutput) { o.Value = json.RawMessage(`0.05`) }},
		{"short sample", func(o *FactorOutput) { o.SampleCount = 5 }},
		{"wrong result status", func(o *FactorOutput) { o.ResultStatus = "insufficient_data" }},
		{"degraded quality", func(o *FactorOutput) { o.QualityStatus = "degraded" }},
		{"non-semver calculation version", func(o *FactorOutput) { o.CalculationVersion = "momentum-decimal-v1" }},
		{"non-utc system as of", func(o *FactorOutput) { o.SystemAsOf = "2026-11-26T14:00:00+08:00" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			candidate.Value = append(json.RawMessage(nil), base.Value...)
			tc.mutate(&candidate)
			encoded := mustMarshalStageJSON(t, candidate)
			if err := ValidateFactorStageOutputJSON(encoded); err == nil {
				t.Fatalf("invalid Factor Stage Output unexpectedly passed: %s", encoded)
			}
		})
	}
	if err := ValidateFactorStageOutputJSON([]byte(`{"schema_version":"1.0.0","unexpected":true}`)); err == nil {
		t.Fatal("output with unknown field unexpectedly passed")
	}
	if err := ValidateFactorStageOutputJSON(append(mustMarshalStageJSON(t, base), []byte(`{}`)...)); err == nil {
		t.Fatal("output with trailing JSON unexpectedly passed")
	}
}

func TestFactorQualitySummaryValidatorRejectsContractViolations(t *testing.T) {
	base := FactorQualitySummary{
		SchemaVersion:         FactorQualitySchemaVersion,
		MetricCode:            "momentum_20d",
		QualityStatus:         "valid",
		CalendarQualityStatus: "valid",
		ExpectedTradingDays:   30,
		ObservedBars:          21,
		MinimumRequiredBars:   21,
		CoverageSufficient:    true,
	}
	mutations := []struct {
		name   string
		mutate func(*FactorQualitySummary)
	}{
		{"minimum above observed", func(q *FactorQualitySummary) { q.MinimumRequiredBars = 22 }},
		{"observed above expected", func(q *FactorQualitySummary) { q.ObservedBars = 31 }},
		{"unsupported metric code", func(q *FactorQualitySummary) { q.MetricCode = "momentum_60d" }},
		{"coverage not sufficient", func(q *FactorQualitySummary) { q.CoverageSufficient = false }},
		{"degraded calendar", func(q *FactorQualitySummary) { q.CalendarQualityStatus = "degraded" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			tc.mutate(&candidate)
			encoded := mustMarshalStageJSON(t, candidate)
			if err := ValidateFactorQualitySummaryJSON(encoded); err == nil {
				t.Fatalf("invalid Factor Quality Summary unexpectedly passed: %s", encoded)
			}
		})
	}
}

func TestFactorStageValidationPoliciesMatchProductionRegistry(t *testing.T) {
	expectedPeriods := map[string]string{
		registry.ProductionMADistanceCode: "20d",
		registry.ProductionMomentum5Code:  "5d",
		registry.ProductionMomentum20Code: "20d",
	}
	if len(factorStageMinimumBarsByCode) != len(expectedPeriods) || len(factorStagePeriodByCode) != len(expectedPeriods) {
		t.Fatalf("Factor Stage policy tables must contain exactly three production codes: bars=%d periods=%d",
			len(factorStageMinimumBarsByCode), len(factorStagePeriodByCode))
	}
	for code, expectedPeriod := range expectedPeriods {
		minimum, ok := factorStageMinimumBarsByCode[code]
		if !ok {
			t.Fatalf("Factor Stage policy missing production code %s", code)
		}
		if factorStagePeriodByCode[code] != expectedPeriod {
			t.Fatalf("Factor Stage period for %s = %s, want %s", code, factorStagePeriodByCode[code], expectedPeriod)
		}
		if minimum < 2 {
			t.Fatalf("Factor Stage minimum bars for %s = %d, want >= 2", code, minimum)
		}
	}
}

func compileStageSchema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	document, err := os.ReadFile(filepath.Join("..", "..", "..", "doc", "quant", "schema", name))
	if err != nil {
		t.Fatalf("read schema %s: %v", name, err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	const schemaURL = "https://quant.invalid/factor-stage-schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(document)); err != nil {
		t.Fatalf("add schema resource %s: %v", name, err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatalf("compile schema %s: %v", name, err)
	}
	return compiled
}

func validateStageSchemaInstance(t *testing.T, schema *jsonschema.Schema, payload []byte) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var instance any
	if err := decoder.Decode(&instance); err != nil {
		t.Fatalf("decode schema instance: %v", err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("schema rejected valid Stage instance: %v; JSON: %s", err, payload)
	}
}

func mustMarshalStageJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal Stage fixture: %v", err)
	}
	return encoded
}
