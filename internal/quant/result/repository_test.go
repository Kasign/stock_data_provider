package result

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/manifest"
)

func validRecord(t *testing.T, payload string) Record {
	t.Helper()
	raw := json.RawMessage(payload)
	m, err := manifest.BuildWithPayload(manifest.Input{
		ResultType: "factor", SchemaVersion: "1.0.0",
		SystemAsOf: time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		InputSnapshotRefs: []string{"snapshot:001"},
	}, raw)
	if err != nil {
		t.Fatalf("BuildWithPayload() error = %v", err)
	}
	return Record{Manifest: m, Payload: raw}
}

func TestRecordValidateBindsPayloadToManifest(t *testing.T) {
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	if err := record.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRecordValidateRejectsPayloadTampering(t *testing.T) {
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	record.Payload = json.RawMessage(`{"metric_code":"momentum_20d","value":0.13}`)
	if err := record.Validate(); !errors.Is(err, manifest.ErrInvalidPayload) {
		t.Fatalf("Validate() error = %v, want ErrInvalidPayload", err)
	}
}

func TestRecordValidateRejectsInvalidManifest(t *testing.T) {
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	record.Manifest.ResultID = "forged"
	if err := record.Validate(); !errors.Is(err, manifest.ErrInvalidManifest) {
		t.Fatalf("Validate() error = %v, want ErrInvalidManifest", err)
	}
}
