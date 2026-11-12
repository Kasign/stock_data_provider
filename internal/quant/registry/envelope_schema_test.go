package registry

import (
	"encoding/json"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/result"
	"example.com/stock-analysis-flow/internal/quant/risk"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

func TestQuantResultEnvelopeSchemaMatchesRegisteredPayloads(t *testing.T) {
	schema := compileQuantSchemaFile(t, "QNT-CTR-004-quant-result-envelope-1.0.0.schema.json")
	asOf, err := time.Parse(time.RFC3339Nano, "2026-11-09T00:59:00Z")
	if err != nil {
		t.Fatal(err)
	}
	trendPayload, err := result.NewTrendPayload(trend.Result{
		TrendCode: "price_trend", Direction: trend.DirectionUnknown, Phase: trend.PhaseEmerging,
		Period: "20d", EvidenceMetrics: []metric.Metric{}, ConflictRefs: []string{},
		QualityStatus: trend.QualityValid, SystemAsOf: "2026-11-09T00:59:00Z",
		AlgorithmVersion: "trend-algorithm.v1",
	})
	if err != nil {
		t.Fatal(err)
	}
	riskPayload, err := result.NewRiskPayload(risk.Decision{
		DecisionID: "risk-decision-001", Status: risk.DecisionReject, RuleSetVersion: "risk-rules.v7",
		EvaluatedAt: "2026-11-09T01:00:00Z", SystemAsOf: "2026-11-09T00:59:00Z",
		Reasons: []risk.Reason{{Code: "HARD_GATE", Severity: risk.SeverityBlock, Status: risk.ReasonTriggered,
			SafeMessage: "硬门禁已触发", EvidenceRefs: []string{}}},
		InputRefs: []string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		resultType string
		algorithmVersion string
		parameterFingerprint string
		snapshots []string
		payload result.Payload
	}{
		{"trend", "trend", "trend-algorithm.v1", "sha256:envelope-test-parameters", []string{"snapshot:envelope-test"}, trendPayload},
		{"risk", "risk", "risk-evaluator.v1", "sha256:risk-parameters", []string{}, riskPayload},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payloadJSON, err := json.Marshal(tc.payload)
			if err != nil {
				t.Fatal(err)
			}
			manifestValue, err := manifest.BuildWithPayload(manifest.Input{
				ResultType: tc.resultType, Scope: "stock", SecurityID: "600000.SH", Period: "20d",
				QualityStatus: "valid", ResultStatus: "completed", SchemaVersion: "1.0.0",
				PayloadSchemaVersion: "1.0.0", AlgorithmVersion: tc.algorithmVersion,
				ParameterFingerprint: tc.parameterFingerprint, SystemAsOf: asOf,
				InputSnapshotRefs: tc.snapshots,
			}, payloadJSON)
			if err != nil {
				t.Fatal(err)
			}
			envelope, err := result.BuildEnvelope(manifestValue, "authorization-ref:current", tc.payload)
			if err != nil {
				t.Fatal(err)
			}
			encoded, err := json.Marshal(envelope)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateJSONSchemaInstance(schema, encoded); err != nil {
				t.Fatalf("Envelope violates QNT-CTR-004 Schema: %v; JSON: %s", err, encoded)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(encoded, &fields); err != nil {
				t.Fatal(err)
			}
			fields["result_type"] = json.RawMessage(`"factor"`)
			unsupported, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			if err := validateJSONSchemaInstance(schema, unsupported); err == nil {
				t.Fatal("Envelope Schema accepted unregistered result_type")
			}
		})
	}
}
