package result

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/factor"
	"example.com/stock-analysis-flow/internal/quant/manifest"
)

const factorTestAsOf = "2026-11-23T00:00:00Z"

func validFactorMetric(t *testing.T, code, value string, sampleCount int) factor.DecimalQuantMetric {
	t.Helper()
	asOf, err := time.Parse(time.RFC3339Nano, factorTestAsOf)
	if err != nil {
		t.Fatal(err)
	}
	fixture := factor.DecimalResult{Code: code, Status: factor.StatusAvailable, SampleCount: sampleCount}
	if value != "" {
		fixture.Value = value
	}
	metric, err := factor.NewDecimalQuantMetric(fixture, asOf, factor.QualityValid, "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	return metric
}

func factorManifestInput(t *testing.T, metric factor.DecimalQuantMetric) manifest.Input {
	t.Helper()
	asOf, err := time.Parse(time.RFC3339Nano, factorTestAsOf)
	if err != nil {
		t.Fatal(err)
	}
	return manifest.Input{
		ResultType: "factor", Scope: "stock", SecurityID: "600000.SH",
		TradeDate: "2026-11-23", Period: metric.Period,
		QualityStatus: "valid", ResultStatus: "completed",
		SchemaVersion: EnvelopeSchemaVersion, PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion: metric.CalculationVersion, ParameterFingerprint: "sha256:factor-parameters",
		SystemAsOf: asOf, InputSnapshotRefs: []string{"data_snapshot:sha256:" + strings.Repeat("a", 64)},
	}
}

func TestFactorPayloadIsRegisteredInEnvelopeUnion(t *testing.T) {
	metric := validFactorMetric(t, "ma20_distance", "0.904761904762", 20)
	payload, err := NewFactorPayload(metric)
	if err != nil {
		t.Fatal(err)
	}
	if payload.resultType() != "factor" || payload.payloadSchemaVersion() != "1.0.0" || !supportedPayloadValue(payload) {
		t.Fatalf("factor payload not registered in Envelope union: type=%s version=%s",
			payload.resultType(), payload.payloadSchemaVersion())
	}

	if _, err := DecodePayload("factor", "9.9.9", json.RawMessage(`{}`)); !errors.Is(err, ErrUnsupportedPayload) {
		t.Fatalf("unregistered factor version error = %v, want ErrUnsupportedPayload", err)
	}
	if _, err := DecodePayload("trend", "1.0.0", json.RawMessage(`{}`)); err == nil {
		t.Fatal("trend decoder accepted a factor-shaped payload")
	}
}

func TestFactorRecordRoundTripsThroughBuildRecordAndDecode(t *testing.T) {
	metric := validFactorMetric(t, "ma20_distance", "0.904761904762", 20)
	payload, err := NewFactorPayload(metric)
	if err != nil {
		t.Fatal(err)
	}
	record, err := BuildRecord(factorManifestInput(t, metric), payload)
	if err != nil {
		t.Fatalf("BuildRecord() error = %v", err)
	}
	if err := record.Validate(); err != nil {
		t.Fatalf("Record.Validate() error = %v", err)
	}
	decoded, err := DecodePayload(record.Manifest.ResultType, record.Manifest.PayloadSchemaVersion, record.Payload)
	if err != nil {
		t.Fatalf("DecodePayload() error = %v", err)
	}
	factorPayload, ok := decoded.(FactorPayload)
	if !ok {
		t.Fatalf("decoded payload type = %T, want FactorPayload", decoded)
	}
	got := factorPayload.Metric()
	if got.MetricCode != "ma20_distance" || got.Value == nil || *got.Value != "0.904761904762" || got.SampleCount != 20 {
		t.Fatalf("round-tripped metric = %+v", got)
	}
}

func TestFactorEnvelopeProjectsManifestAndRoundTrips(t *testing.T) {
	metric := validFactorMetric(t, "momentum_5d", "0.052631578947", 6)
	payload, err := NewFactorPayload(metric)
	if err != nil {
		t.Fatal(err)
	}
	source, err := manifest.BuildWithPayload(factorManifestInput(t, metric), mustMarshalPayload(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildEnvelope(source, "authorization-ref:current", payload)
	if err != nil {
		t.Fatalf("BuildEnvelope() error = %v", err)
	}
	if envelope.SchemaVersion != EnvelopeSchemaVersion || envelope.ResultType != "factor" ||
		envelope.ResultID != source.ResultID || envelope.AuthorizationRef != "authorization-ref:current" {
		t.Fatalf("Envelope did not project Manifest: %+v", envelope)
	}
	encoded, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Envelope
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("unmarshal Envelope: %v", err)
	}
	if err := decoded.ValidateAgainstManifest(source); err != nil {
		t.Fatalf("round-tripped Envelope violates Manifest: %v", err)
	}
	if _, ok := decoded.Payload.(FactorPayload); !ok {
		t.Fatalf("round-tripped payload type = %T, want FactorPayload", decoded.Payload)
	}
}

func TestFactorEnvelopeRejectsManifestContextDrift(t *testing.T) {
	metric := validFactorMetric(t, "momentum_20d", "0.2", 21)
	payload, err := NewFactorPayload(metric)
	if err != nil {
		t.Fatal(err)
	}
	source, err := manifest.BuildWithPayload(factorManifestInput(t, metric), mustMarshalPayload(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildEnvelope(source, "authorization-ref:current", payload)
	if err != nil {
		t.Fatal(err)
	}
	mutations := []struct {
		name   string
		mutate func(*Envelope)
	}{
		{"period", func(e *Envelope) { e.Period = "5d" }},
		{"system as of", func(e *Envelope) { e.SystemAsOf = "2026-11-22T00:00:00Z" }},
		{"algorithm version", func(e *Envelope) { e.AlgorithmVersion = "9.9.9" }},
		{"quality status", func(e *Envelope) { e.QualityStatus = "degraded" }},
		{"result status", func(e *Envelope) { e.ResultStatus = "insufficient_data" }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			changed := envelope
			tc.mutate(&changed)
			if err := changed.Validate(); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("Validate() = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}

func TestFactorPayloadRejectsInvalidMetric(t *testing.T) {
	metric := validFactorMetric(t, "ma20_distance", "0.904761904762", 20)
	mutations := []struct {
		name   string
		mutate func(*factor.DecimalQuantMetric)
	}{
		{"non decimal value", func(m *factor.DecimalQuantMetric) { v := "NaN"; m.Value = &v }},
		{"null value on available", func(m *factor.DecimalQuantMetric) { m.Value = nil }},
		{"below minimum samples", func(m *factor.DecimalQuantMetric) { m.SampleCount = 19 }},
		{"period mismatch", func(m *factor.DecimalQuantMetric) { m.Period = "5d" }},
		{"non semver", func(m *factor.DecimalQuantMetric) { m.CalculationVersion = "v1" }},
		{"invalid quality", func(m *factor.DecimalQuantMetric) { m.QualityStatus = factor.QualityInvalid }},
		{"non canonical time", func(m *factor.DecimalQuantMetric) { m.SystemAsOf = "2026-11-23T08:00:00+08:00" }},
		{"unknown metric code", func(m *factor.DecimalQuantMetric) { m.MetricCode = "made_up" }},
		{"forged bound", func(m *factor.DecimalQuantMetric) { b := "1"; m.LowerBound = &b }},
	}
	for _, tc := range mutations {
		t.Run(tc.name, func(t *testing.T) {
			changed := metric
			tc.mutate(&changed)
			if _, err := NewFactorPayload(changed); !errors.Is(err, ErrInvalidEnvelope) {
				t.Fatalf("NewFactorPayload() error = %v, want ErrInvalidEnvelope", err)
			}
		})
	}
}

func TestDecodeFactorPayloadRejectsNullSampleCountAndUnknownFields(t *testing.T) {
	metric := validFactorMetric(t, "ma20_distance", "0.904761904762", 20)
	raw := mustMarshalPayload(t, mustFactorPayload(t, metric))
	nullSample := strings.Replace(string(raw), `"sample_count":20`, `"sample_count":null`, 1)
	if _, err := DecodeFactorPayload(json.RawMessage(nullSample)); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("null sample_count error = %v, want ErrInvalidEnvelope", err)
	}
	withUnknown := strings.Replace(string(raw), `"metric_code"`, `"unknown":1,"metric_code"`, 1)
	if _, err := DecodeFactorPayload(json.RawMessage(withUnknown)); !errors.Is(err, ErrInvalidEnvelope) {
		t.Fatalf("unknown field error = %v, want ErrInvalidEnvelope", err)
	}
}

func TestFactorPayloadCopiesCallerOwnedValue(t *testing.T) {
	metric := validFactorMetric(t, "ma20_distance", "0.904761904762", 20)
	payload, err := NewFactorPayload(metric)
	if err != nil {
		t.Fatal(err)
	}
	*metric.Value = "mutated"
	if got := payload.Metric(); got.Value == nil || *got.Value != "0.904761904762" {
		t.Fatalf("FactorPayload retained caller-owned value memory: %+v", got)
	}
	got := payload.Metric()
	*got.Value = "mutated-again"
	if again := payload.Metric(); again.Value == nil || *again.Value != "0.904761904762" {
		t.Fatalf("FactorPayload.Metric() leaked internal value memory: %+v", again)
	}
}

func TestFactorEnvelopeMatchesGolden(t *testing.T) {
	metric := validFactorMetric(t, "ma20_distance", "0.904761904762", 20)
	payload := mustFactorPayload(t, metric)
	source, err := manifest.BuildWithPayload(factorManifestInput(t, metric), mustMarshalPayload(t, payload))
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := BuildEnvelope(source, "authorization-ref:current", payload)
	if err != nil {
		t.Fatal(err)
	}
	assertEnvelopeGolden(t, "quant-result-envelope-factor-1.0.0.golden.json", envelope)
}

func mustFactorPayload(t *testing.T, metric factor.DecimalQuantMetric) FactorPayload {
	t.Helper()
	payload, err := NewFactorPayload(metric)
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func mustMarshalPayload(t *testing.T, payload FactorPayload) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
