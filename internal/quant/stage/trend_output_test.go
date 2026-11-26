package stage

import (
	"bytes"
	"encoding/json"
	"testing"

	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/trend"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

// 五类 Golden：三正、三负、符号冲突、长周期证据缺失、方向不可判定。
// 每个用例同时经运行时 Validator 与文档化 Draft 2020-12 Schema 判定，
// 避免"文档写一套、运行时判另一套"。

func trendEvidenceFixture(t *testing.T, maDistance, momentum5, momentum20, momentum60 string, longAvailable bool) []metric.Metric {
	t.Helper()
	values := []string{maDistance, momentum5, momentum20, momentum60}
	codes := []string{"ma20_distance", "momentum_5d", "momentum_20d", "momentum_60d"}
	periods := []string{"20d", "5d", "20d", "60d"}
	samples := []int{20, 6, 21, 61}
	evidence := make([]metric.Metric, len(codes))
	for index := range codes {
		sampleCount := samples[index]
		value := json.RawMessage(`"` + values[index] + `"`)
		status := metric.StatusAvailable
		if index == 3 && !longAvailable {
			status = metric.StatusInsufficientData
			value = json.RawMessage("null")
			sampleCount = 30
		}
		evidence[index] = metric.Metric{
			MetricCode:         codes[index],
			Value:              value,
			Unit:               "ratio",
			Period:             periods[index],
			SystemAsOf:         "2026-11-26T06:00:00Z",
			QualityStatus:      metric.QualityValid,
			MetricStatus:       status,
			CalculationVersion: "1.0.0",
			SampleCount:        &sampleCount,
			LowerBound:         json.RawMessage("null"),
			UpperBound:         json.RawMessage("null"),
		}
	}
	return evidence
}

func trendOutputFixture(
	t *testing.T,
	evidence []metric.Metric,
	direction trend.Direction,
	quality trend.QualityStatus,
	resultStatus string,
) TrendOutput {
	t.Helper()
	strength := (*float64)(nil)
	return TrendOutput{
		SchemaVersion: TrendSchemaVersion,
		ResultID:      factorStageTestResultID,
		SnapshotRef:   "snapshot:trend-stage-schema",
		ResultStatus:  resultStatus,
		Result: trend.Result{
			TrendCode:        trend.CloseTrendCode,
			Direction:        direction,
			Phase:            trend.PhaseUnknown,
			Strength:         strength,
			Period:           trend.CloseTrendPeriod,
			EvidenceMetrics:  evidence,
			ConflictRefs:     []string{},
			QualityStatus:    quality,
			SystemAsOf:       "2026-11-26T06:00:00Z",
			AlgorithmVersion: trend.CloseTrendAlgorithmVersion,
		},
	}
}

func trendQualityFixture(resultQuality string) TrendQualitySummary {
	return TrendQualitySummary{
		SchemaVersion:         TrendQualitySchemaVersion,
		TrendCode:             trend.CloseTrendCode,
		QualityStatus:         "valid",
		ResultQualityStatus:   resultQuality,
		CalendarQualityStatus: "valid",
		ExpectedTradingDays:   61,
		ObservedBars:          61,
		MinimumRequiredBars:   20,
		CoverageSufficient:    true,
	}
}

func TestTrendStageOutputSchemaAndValidatorAcceptAllGoldenCases(t *testing.T) {
	outputSchema := compileStageSchema(t, "QNT-CTR-003-trend-stage-output-1.0.0.schema.json")
	qualitySchema := compileStageSchema(t, "QNT-CTR-003-trend-quality-summary-1.0.0.schema.json")
	cases := []struct {
		name         string
		ma           string
		m5           string
		m20          string
		m60          string
		longOK       bool
		direction    trend.Direction
		quality      trend.QualityStatus
		resultStatus string
	}{
		{"bullish", "0.0325", "0.011", "0.048", "0.092", true, trend.DirectionBullish, trend.QualityValid, "completed"},
		{"bearish", "-0.0325", "-0.011", "-0.048", "-0.092", true, trend.DirectionBearish, trend.QualityValid, "completed"},
		{"sideways", "-0.0325", "0.011", "0.048", "0.092", true, trend.DirectionSideways, trend.QualityValid, "completed"},
		{"degraded_missing_long_period", "0.0325", "0.011", "0.048", "", false, trend.DirectionBullish, trend.QualityDegraded, "completed"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			evidence := trendEvidenceFixture(t, testCase.ma, testCase.m5, testCase.m20, testCase.m60, testCase.longOK)
			output := trendOutputFixture(t, evidence, testCase.direction, testCase.quality, testCase.resultStatus)
			encoded := mustMarshalStageJSON(t, output)
			if err := ValidateTrendStageOutputJSON(encoded); err != nil {
				t.Fatalf("runtime validator rejected valid output: %v; JSON: %s", err, encoded)
			}
			validateStageSchemaInstance(t, outputSchema, encoded)

			quality := trendQualityFixture(string(testCase.quality))
			qualityJSON := mustMarshalStageJSON(t, quality)
			if err := ValidateTrendQualitySummaryJSON(qualityJSON); err != nil {
				t.Fatalf("runtime validator rejected valid quality summary: %v; JSON: %s", err, qualityJSON)
			}
			validateStageSchemaInstance(t, qualitySchema, qualityJSON)
		})
	}
}

func TestTrendStageOutputSchemaAndValidatorAcceptUnknownDirection(t *testing.T) {
	outputSchema := compileStageSchema(t, "QNT-CTR-003-trend-stage-output-1.0.0.schema.json")
	evidence := trendEvidenceFixture(t, "", "", "", "", false)
	// 全部证据都不可用：样本数必须低于各自下限，否则运行时 Validator 会拒绝。
	for index := range evidence {
		evidence[index].MetricStatus = metric.StatusInsufficientData
		evidence[index].Value = json.RawMessage("null")
		sampleCount := []int{19, 5, 20, 30}[index]
		evidence[index].SampleCount = &sampleCount
	}
	output := trendOutputFixture(t, evidence, trend.DirectionUnknown, trend.QualityUnknown, "insufficient_data")
	encoded := mustMarshalStageJSON(t, output)
	if err := ValidateTrendStageOutputJSON(encoded); err != nil {
		t.Fatalf("runtime validator rejected unknown-direction output: %v; JSON: %s", err, encoded)
	}
	validateStageSchemaInstance(t, outputSchema, encoded)
}

// 文档化 Schema 必须与运行时 Validator 对同一漂移给出同样结论：
// 数值型 value、具体 phase、非 null strength、缺失的 result_quality_status。
func TestTrendStageDocumentedSchemaRejectsSameDriftAsValidator(t *testing.T) {
	outputSchema := compileStageSchema(t, "QNT-CTR-003-trend-stage-output-1.0.0.schema.json")
	qualitySchema := compileStageSchema(t, "QNT-CTR-003-trend-quality-summary-1.0.0.schema.json")
	evidence := trendEvidenceFixture(t, "0.0325", "0.011", "0.048", "0.092", true)
	baseline := mustMarshalStageJSON(t, trendOutputFixture(t, evidence, trend.DirectionBullish, trend.QualityValid, "completed"))
	qualityBaseline := mustMarshalStageJSON(t, trendQualityFixture("valid"))

	assertBothReject := func(t *testing.T, payload []byte, schema *jsonschema.Schema) {
		t.Helper()
		if err := ValidateTrendStageOutputJSON(payload); err == nil {
			t.Fatal("runtime validator accepted drifted payload")
		}
		assertStageSchemaRejects(t, schema, payload)
	}

	t.Run("numeric value", func(t *testing.T) {
		var document map[string]any
		if err := json.Unmarshal(baseline, &document); err != nil {
			t.Fatal(err)
		}
		document["evidence_metrics"].([]any)[0].(map[string]any)["value"] = 0.0325
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		assertBothReject(t, payload, outputSchema)
	})
	t.Run("concrete phase", func(t *testing.T) {
		var document map[string]any
		if err := json.Unmarshal(baseline, &document); err != nil {
			t.Fatal(err)
		}
		document["phase"] = "accelerating"
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		assertBothReject(t, payload, outputSchema)
	})
	t.Run("non-null strength", func(t *testing.T) {
		var document map[string]any
		if err := json.Unmarshal(baseline, &document); err != nil {
			t.Fatal(err)
		}
		document["strength"] = 0.5
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		assertBothReject(t, payload, outputSchema)
	})
	t.Run("missing result quality", func(t *testing.T) {
		var document map[string]any
		if err := json.Unmarshal(qualityBaseline, &document); err != nil {
			t.Fatal(err)
		}
		delete(document, "result_quality_status")
		payload, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		assertBothReject(t, payload, qualitySchema)
	})
}

func assertStageSchemaRejects(t *testing.T, schema *jsonschema.Schema, payload []byte) {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var instance any
	if err := decoder.Decode(&instance); err != nil {
		t.Fatalf("decode schema instance: %v", err)
	}
	if err := schema.Validate(instance); err == nil {
		t.Fatalf("documented Schema accepted drifted instance: %s", payload)
	}
}
