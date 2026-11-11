package manifest

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
		ResultType:           golden.Manifest.ResultType,
		SchemaVersion:        golden.Manifest.SchemaVersion,
		PayloadSchemaVersion: golden.Manifest.PayloadSchemaVersion,
		AlgorithmVersion:     golden.Manifest.AlgorithmVersion,
		ParameterFingerprint: golden.Manifest.ParameterFingerprint,
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"data_snapshot:sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		ResultFingerprint:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
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

func TestBuildAllowsExplicitEmptySnapshotRefs(t *testing.T) {
	got, err := Build(Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "factor.v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{},
		ResultFingerprint:    strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatalf("Build() rejected an explicitly empty snapshot set: %v", err)
	}
	if got.InputSnapshotRefs == nil || len(got.InputSnapshotRefs) != 0 {
		t.Fatalf("empty snapshot set must remain a non-nil empty slice: %#v", got.InputSnapshotRefs)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "\"input_snapshot_refs\":[]") {
		t.Fatalf("empty snapshot set must serialize as an empty array: %s", encoded)
	}
	if err := got.Verify(); err != nil {
		t.Fatalf("Verify() rejected an explicitly empty snapshot set: %v", err)
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
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-b", "snapshot-a"},
		ResultFingerprint:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
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
		{name: "missing refs array", input: func() Input { v := base; v.InputSnapshotRefs = nil; return v }()},
		{name: "duplicate refs", input: func() Input { v := base; v.InputSnapshotRefs = []string{"same", "same"}; return v }()},
		{name: "blank ref", input: func() Input { v := base; v.InputSnapshotRefs = []string{" "}; return v }()},
		{name: "unknown result type", input: func() Input { v := base; v.ResultType = "unknown"; return v }()},
		{name: "invalid schema version", input: func() Input { v := base; v.SchemaVersion = "v1"; return v }()},
		{name: "missing payload schema version", input: func() Input { v := base; v.PayloadSchemaVersion = ""; return v }()},
		{name: "missing algorithm version", input: func() Input { v := base; v.AlgorithmVersion = ""; return v }()},
		{name: "missing parameter fingerprint", input: func() Input { v := base; v.ParameterFingerprint = ""; return v }()},
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

func TestBuildRejectsInvalidUTF8SnapshotReference(t *testing.T) {
	input := Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{string([]byte{0xff})},
		ResultFingerprint:    strings.Repeat("b", 64),
	}
	if _, err := Build(input); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Build() error = %v, want ErrInvalidManifest", err)
	}
}

func TestVerifyRejectsMissingSnapshotArray(t *testing.T) {
	got, err := Build(Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "factor.v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{},
		ResultFingerprint:    strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	got.InputSnapshotRefs = nil
	if err := got.Verify(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Verify() accepted a missing snapshot array: %v", err)
	}
}

func TestVerifyRejectsInvalidUTF8SnapshotReference(t *testing.T) {
	got, err := Build(Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-a"},
		ResultFingerprint:    strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	got.InputSnapshotRefs[0] = string([]byte{0xff})
	if err := got.Verify(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("Verify() error = %v, want ErrInvalidManifest", err)
	}
}

func TestVerifyRejectsManifestMutation(t *testing.T) {
	got, err := Build(Input{
		ResultType:           "stock",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-a"},
		ResultFingerprint:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	})
	if err != nil {
		t.Fatal(err)
	}
	got.InputSnapshotRefs[0] = "snapshot-b"
	if err := got.Verify(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected tampered manifest to fail verification, got %v", err)
	}
}

func TestBuildWithPayloadBindsManifestToCanonicalPayload(t *testing.T) {
	payload := json.RawMessage(`{"metric_code":"momentum_20d","value":0.125}`)
	input := Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-a"},
	}

	got, err := BuildWithPayload(input, payload)
	if err != nil {
		t.Fatalf("BuildWithPayload() error = %v", err)
	}
	if err := got.VerifyPayload(json.RawMessage(`{ "value": 0.125, "metric_code": "momentum_20d" }`)); err != nil {
		t.Fatalf("VerifyPayload() rejected equivalent canonical payload: %v", err)
	}
	if err := got.VerifyPayload(json.RawMessage(`{"metric_code":"momentum_20d","value":0.126}`)); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected changed payload to be rejected, got %v", err)
	}
}

func TestBuildWithPayloadRejectsCallerSuppliedFingerprintMismatch(t *testing.T) {
	input := Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-a"},
		ResultFingerprint:    "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
	}
	if _, err := BuildWithPayload(input, json.RawMessage(`{"value":1}`)); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("expected mismatched caller fingerprint to be rejected, got %v", err)
	}
}

func TestVerifyPayloadRejectsInvalidManifestBeforePayload(t *testing.T) {
	got, err := BuildWithPayload(Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-a"},
	}, json.RawMessage(`{"value":1}`))
	if err != nil {
		t.Fatal(err)
	}
	got.ResultID = "quant_result:sha256:" + strings.Repeat("0", 64)
	if err := got.VerifyPayload(json.RawMessage(`{"value":1}`)); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected invalid manifest to fail before payload verification, got %v", err)
	}
}

func TestVerifyRejectsNonCanonicalUTCSystemAsOf(t *testing.T) {
	got, err := Build(Input{
		ResultType:           "factor",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "quant-test-v1",
		ParameterFingerprint: strings.Repeat("c", 64),
		SystemAsOf:           time.Date(2026, 11, 6, 5, 2, 0, 120000000, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-a"},
		ResultFingerprint:    strings.Repeat("b", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	got.SystemAsOf = "2026-11-06T05:02:00.120000000Z"
	fingerprint, err := got.calculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	got.ManifestFingerprint = fingerprint
	got.ResultID = resultIDPrefix + fingerprint
	if err := got.Verify(); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("expected non-canonical UTC timestamp to be rejected, got %v", err)
	}
}

func TestFingerprintPayloadRejectsInvalidUTF8(t *testing.T) {
	payload := append([]byte(`{"value":"`), 0xff)
	payload = append(payload, []byte(`"}`)...)
	if _, err := FingerprintPayload(payload); !errors.Is(err, ErrInvalidPayload) {
		t.Fatalf("FingerprintPayload error = %v, want ErrInvalidPayload", err)
	}
}

func TestFingerprintPayloadRejectsDuplicateObjectKeys(t *testing.T) {
	cases := []struct {
		name    string
		payload json.RawMessage
	}{
		{
			name:    "duplicate top-level key",
			payload: json.RawMessage(`{"value":1,"value":2}`),
		},
		{
			name:    "duplicate nested key after escape decoding",
			payload: json.RawMessage(`{"nested":{"a":1,"\u0061":2}}`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := FingerprintPayload(tc.payload); !errors.Is(err, ErrInvalidPayload) {
				t.Fatalf("FingerprintPayload error = %v, want ErrInvalidPayload", err)
			}
		})
	}
}

// FuzzFingerprintPayloadNoPanic 验证任意字节输入都不会导致 payload 指纹计算 panic；
// 被接受的输入必须产生稳定的小写 SHA-256 十六进制指纹。
func FuzzFingerprintPayloadNoPanic(f *testing.F) {
	seeds := [][]byte{
		[]byte(`{"value":1}`),
		[]byte(`{"nested":{"name":"量化"}}`),
		[]byte(`{"value":1,"value":2}`),
		[]byte(`[]`),
		[]byte(`null`),
		[]byte{},
		{0xff, 0xfe, 0xfd},
	}
	for _, seed := range seeds {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, payload []byte) {
		first, err := FingerprintPayload(json.RawMessage(payload))
		if err != nil {
			return
		}
		if !fingerprintRE.MatchString(first) {
			t.Fatalf("accepted payload produced invalid fingerprint %q", first)
		}
		second, err := FingerprintPayload(json.RawMessage(payload))
		if err != nil {
			t.Fatalf("same payload became invalid on second fingerprint: %v", err)
		}
		if first != second {
			t.Fatalf("fingerprint is not deterministic: %q != %q", first, second)
		}
	})
}
