package result

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/manifest"
	"example.com/stock-analysis-flow/internal/quant/metric"
	"example.com/stock-analysis-flow/internal/quant/risk"
	"example.com/stock-analysis-flow/internal/quant/trend"
)

func validTrendPayload(t *testing.T) TrendPayload {
	t.Helper()
	value := trend.Result{
		TrendCode:        "price_trend",
		Direction:        trend.DirectionUnknown,
		Phase:            trend.PhaseEmerging,
		Strength:         nil,
		Period:           "20d",
		EvidenceMetrics:  []metric.Metric{},
		ConflictRefs:     []string{},
		QualityStatus:    trend.QualityValid,
		SystemAsOf:       "2026-11-09T00:59:00Z",
		AlgorithmVersion: "trend-algorithm.v1",
	}
	payload, err := NewTrendPayload(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func validRiskPayload(t *testing.T) RiskPayload {
	t.Helper()
	value := risk.Decision{
		DecisionID:     "risk-decision-001",
		Status:         risk.DecisionReject,
		RuleSetVersion: "risk-rules.v7",
		EvaluatedAt:    "2026-11-09T01:00:00Z",
		SystemAsOf:     "2026-11-09T00:59:00Z",
		Reasons: []risk.Reason{{
			Code:         "HARD_GATE",
			Severity:     risk.SeverityBlock,
			Status:       risk.ReasonTriggered,
			SafeMessage:  "硬门禁已触发",
			EvidenceRefs: []string{},
		}},
		InputRefs: []string{},
	}
	payload, err := NewRiskPayload(value)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestBuildEnvelopeProjectsManifestAndRoundTripsTrendPayload(t *testing.T) {
	payload := validTrendPayload(t)
	// result_type 属于 Manifest 身份，必须在构造时确定。
	payloadJSON, err := payload.marshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	manifestValue, err := manifest.BuildWithPayload(manifest.Input{
		ResultType:           "trend",
		Scope:                "stock",
		SecurityID:           "600000.SH",
		Period:               "20d",
		QualityStatus:        "valid",
		ResultStatus:         "completed",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "trend-algorithm.v1",
		ParameterFingerprint: "sha256:envelope-test-parameters",
		SystemAsOf:           envelopeTestTime(t),
		InputSnapshotRefs:    []string{"snapshot:envelope-test"},
	}, payloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildEnvelope(manifestValue, "authorization-ref:current", payload)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.ResultID != manifestValue.ResultID ||
		envelope.ResultType != "trend" ||
		envelope.PayloadSchemaVersion != manifestValue.PayloadSchemaVersion ||
		envelope.AuthorizationRef != "authorization-ref:current" {
		t.Fatalf("Envelope did not project Manifest metadata: %+v", envelope)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Envelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal Envelope: %v", err)
	}
	if err := decoded.ValidateAgainstManifest(manifestValue); err != nil {
		t.Fatalf("round-tripped Envelope did not validate: %v", err)
	}
}

func TestBuildEnvelopeKeepsRiskRejectIndependentFromCompletedResult(t *testing.T) {
	payload := validRiskPayload(t)
	payloadJSON, err := payload.marshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	manifestValue, err := manifest.BuildWithPayload(manifest.Input{
		ResultType:           "risk",
		Scope:                "stock",
		SecurityID:           "600000.SH",
		Period:               "20d",
		QualityStatus:        "valid",
		ResultStatus:         "completed",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "risk-evaluator.v1",
		ParameterFingerprint: "sha256:risk-parameters",
		SystemAsOf:           envelopeTestTime(t),
		InputSnapshotRefs:    []string{},
	}, payloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildEnvelope(manifestValue, "authorization-ref:current", payload)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.ResultStatus != "completed" || envelope.Payload.(RiskPayload).Decision().Status != risk.DecisionReject {
		t.Fatalf("risk domain decision was conflated with result lifecycle: %+v", envelope)
	}
}

func TestEnvelopeRejectsManifestFieldDrift(t *testing.T) {
	payload := validTrendPayload(t)
	payloadJSON, err := payload.marshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	manifestValue, err := manifest.BuildWithPayload(manifest.Input{
		ResultType: "trend", Scope: "stock", SecurityID: "600000.SH", Period: "20d",
		QualityStatus: "valid", ResultStatus: "completed", SchemaVersion: "1.0.0",
		PayloadSchemaVersion: "1.0.0", AlgorithmVersion: "trend-algorithm.v1",
		ParameterFingerprint: "sha256:envelope-test-parameters",
		SystemAsOf: envelopeTestTime(t),
		InputSnapshotRefs: []string{"snapshot:envelope-test"},
	}, payloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildEnvelope(manifestValue, "authorization-ref:current", payload)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{"period", func(e *Envelope) { e.Period = "60d" }},
		{"algorithm version", func(e *Envelope) { e.AlgorithmVersion = "other.v1" }},
		{"quality status", func(e *Envelope) { e.QualityStatus = "degraded" }},
		{"input snapshots", func(e *Envelope) { e.InputSnapshotRefs = []string{"snapshot:other"} }},
		{"result id", func(e *Envelope) { e.ResultID = "quant_result:sha256:" + strings.Repeat("f", 64) }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			changed := envelope
			tc.mutate(&changed)
			if err := changed.ValidateAgainstManifest(manifestValue); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("ValidateAgainstManifest() = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}

func TestDecodePayloadRejectsUnregisteredTypeAndVersion(t *testing.T) {
	valid := json.RawMessage(`{"trend_code":"x"}`)
	if _, err := DecodePayload("stock", "1.0.0", valid); !errors.Is(err, ErrUnsupportedPayload) {
		t.Fatalf("unregistered result_type error = %v", err)
	}
	if _, err := DecodePayload("trend", "9.9.9", valid); !errors.Is(err, ErrUnsupportedPayload) {
		t.Fatalf("unregistered payload version error = %v", err)
	}
	unknownField := json.RawMessage(`{"trend_code":"x","unknown":"value"}`)
	if _, err := DecodePayload("trend", "1.0.0", unknownField); err == nil {
		t.Fatal("strict Trend payload decoder accepted an unknown field")
	}
}

func TestEnvelopeUnmarshalRejectsMissingSnapshotArray(t *testing.T) {
	payload := validTrendPayload(t)
	payloadJSON, err := payload.marshalPayload()
	if err != nil {
		t.Fatal(err)
	}
	manifestValue, err := manifest.BuildWithPayload(manifest.Input{
		ResultType: "trend", Scope: "stock", SecurityID: "600000.SH", Period: "20d",
		QualityStatus: "valid", ResultStatus: "completed", SchemaVersion: "1.0.0",
		PayloadSchemaVersion: "1.0.0", AlgorithmVersion: "trend-algorithm.v1",
		ParameterFingerprint: "sha256:envelope-test-parameters",
		SystemAsOf: envelopeTestTime(t),
		InputSnapshotRefs: []string{"snapshot:envelope-test"},
	}, payloadJSON)
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildEnvelope(manifestValue, "authorization-ref:current", payload)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "input_snapshot_refs")
	missing, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Envelope
	if err := json.Unmarshal(missing, &decoded); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("UnmarshalJSON() = %v, want ErrInvalidEnvelope", err)
	}
}

func TestPayloadConstructorsCopyCallerOwnedMemory(t *testing.T) {
	value := trend.Result{
		TrendCode: "price_trend", Direction: trend.DirectionUnknown, Phase: trend.PhaseUnknown,
		Period: "20d", EvidenceMetrics: []metric.Metric{}, ConflictRefs: []string{"conflict:one"},
		QualityStatus: trend.QualityValid, SystemAsOf: "2026-11-09T00:59:00Z",
		AlgorithmVersion: "trend-algorithm.v1",
	}
	payload, err := NewTrendPayload(value)
	if err != nil {
		t.Fatal(err)
	}
	value.ConflictRefs[0] = "conflict:mutated"
	if payload.Result().ConflictRefs[0] != "conflict:one" {
		t.Fatal("TrendPayload retained caller-owned slice memory")
	}
}

func envelopeTestTime(t *testing.T) time.Time {
	t.Helper()
	value, err := time.Parse(time.RFC3339Nano, "2026-11-09T00:59:00Z")
	if err != nil {
		t.Fatal(err)
	}
	return value
}
