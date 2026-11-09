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


func TestRecordValidatedSnapshotIsolatesMutableSlices(t *testing.T) {
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	snapshot, err := record.ValidatedSnapshot()
	if err != nil {
		t.Fatalf("ValidatedSnapshot() error = %v", err)
	}

	originalPayload := string(snapshot.Payload)
	originalRef := snapshot.Manifest.InputSnapshotRefs[0]
	record.Payload[0] = '['
	record.Manifest.InputSnapshotRefs[0] = "snapshot:mutated"
	if string(snapshot.Payload) != originalPayload {
		t.Fatalf("snapshot payload changed after source mutation: %s", snapshot.Payload)
	}
	if snapshot.Manifest.InputSnapshotRefs[0] != originalRef {
		t.Fatalf("snapshot reference changed after source mutation: %q", snapshot.Manifest.InputSnapshotRefs[0])
	}

	snapshot.Payload[0] = '['
	snapshot.Manifest.InputSnapshotRefs[0] = "snapshot:snapshot-mutated"
	if record.Payload[0] != '[' {
		t.Fatal("expected source payload to remain independently mutable")
	}
	if record.Manifest.InputSnapshotRefs[0] != "snapshot:mutated" {
		t.Fatalf("source reference changed after snapshot mutation: %q", record.Manifest.InputSnapshotRefs[0])
	}
}

func TestRecordValidatedSnapshotRejectsInvalidRecord(t *testing.T) {
	record := validRecord(t, `{"metric_code":"momentum_20d","value":0.12}`)
	record.Payload = json.RawMessage(`{"metric_code":"momentum_20d","value":0.13}`)
	if _, err := record.ValidatedSnapshot(); !errors.Is(err, manifest.ErrInvalidPayload) {
		t.Fatalf("ValidatedSnapshot() error = %v, want ErrInvalidPayload", err)
	}
}
