package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestBuildMatchesQNTCTR005Golden(t *testing.T) {
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "golden", "QNT-CTR-005-result-manifest.golden.json")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read manifest golden: %v", err)
	}
	var golden struct {
		Manifest Manifest `json:"manifest"`
	}
	if err := json.Unmarshal(content, &golden); err != nil {
		t.Fatalf("decode manifest golden: %v", err)
	}

	got, err := Build(Input{
		ResultType:        golden.Manifest.ResultType,
		SchemaVersion:     golden.Manifest.SchemaVersion,
		SystemAsOf:        time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs: []string{"data_snapshot:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		ResultFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !reflect.DeepEqual(got, golden.Manifest) {
		t.Fatalf("Build() manifest mismatch: got=%+v want=%+v", got, golden.Manifest)
	}
	if err := got.Verify(); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
}

func TestFingerprintPayloadUsesCanonicalJSON(t *testing.T) {
	first, err := FingerprintPayload(json.RawMessage(`{"value":1,"label":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	second, err := FingerprintPayload(json.RawMessage(`{ "label" : "x", "value" : 1 }`))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("equivalent JSON payloads produced different fingerprints: %s != %s", first, second)
	}
	if _, err := FingerprintPayload(json.RawMessage(`{"value":1} trailing`)); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected malformed payload rejection, got %v", err)
	}
	if _, err := FingerprintPayload(json.RawMessage(`[]`)); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected non-object payload rejection, got %v", err)
	}
}

func TestBuildSortsSnapshotRefsAndRejectsInvalidInputs(t *testing.T) {
	base := Input{
		ResultType:        "factor",
		SchemaVersion:     "1.0.0",
		SystemAsOf:        time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs: []string{"snapshot-b", "snapshot-a"},
		ResultFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	got, err := Build(base)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.InputSnapshotRefs, []string{"snapshot-a", "snapshot-b"}) {
		t.Fatalf("snapshot refs not sorted: %#v", got.InputSnapshotRefs)
	}

	cases := []struct {
		name  string
		input Input
	}{
		{name: "empty refs", input: func() Input { v := base; v.InputSnapshotRefs = nil; return v }()},
		{name: "duplicate refs", input: func() Input { v := base; v.InputSnapshotRefs = []string{"same", "same"}; return v }()},
		{name: "blank ref", input: func() Input { v := base; v.InputSnapshotRefs = []string{" "}; return v }()},
		{name: "unknown result type", input: func() Input { v := base; v.ResultType = "unknown"; return v }()},
		{name: "invalid schema version", input: func() Input { v := base; v.SchemaVersion = "v1"; return v }()},
		{name: "missing timestamp", input: func() Input { v := base; v.SystemAsOf = time.Time{}; return v }()},
		{name: "invalid result fingerprint", input: func() Input { v := base; v.ResultFingerprint = "BAD"; return v }()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := Build(tc.input); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("expected ErrInvalidManifest, got %v", err)
			}
		})
	}
}

func TestVerifyRejectsManifestMutation(t *testing.T) {
	got, err := Build(Input{
		ResultType:        "stock",
		SchemaVersion:     "1.0.0",
		SystemAsOf:        time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs: []string{"snapshot-a"},
		ResultFingerprint: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatal(err)
	}
	got.InputSnapshotRefs[0] = "snapshot-b"
	if err := got.Verify(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected tampered manifest to fail verification, got %v", err)
	}
}
