package result

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/quant/manifest"
)

func testManifest(t *testing.T) manifest.Manifest {
	t.Helper()
	payload := []byte(`{"metric_code":"momentum_20d","value":0.2}`)
	m, err := manifest.BuildWithPayload(manifest.Input{
		ResultType: "factor", SchemaVersion: "1.0.0",
		SystemAsOf:        time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		InputSnapshotRefs: []string{"snapshot-b", "snapshot-a"},
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func validRefInput() RefInput {
	return RefInput{
		SchemaVersion: "1.0.0", Scope: ScopeStock, SecurityID: "opaque-security-ref",
		TradeDate: "2026-11-06", Period: "20d", AlgorithmVersion: "momentum.v1",
		ParameterFingerprint: "sha256:parameter-fingerprint",
		QualityStatus:        QualityValid, ResultStatus: StatusCompleted,
		AuthorizationRef: "opaque-authorization-ref",
		CreatedAt:        time.Date(2026, 11, 9, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60)),
	}
}

func TestBuildRefProjectsVerifiedManifestFields(t *testing.T) {
	source := testManifest(t)
	got, err := BuildRef(validRefInput(), source)
	if err != nil {
		t.Fatal(err)
	}
	if got.ResultID != source.ResultID || got.ResultType != source.ResultType ||
		got.SystemAsOf != source.SystemAsOf || !reflect.DeepEqual(got.InputSnapshotRefs, source.InputSnapshotRefs) {
		t.Fatalf("manifest-bound fields were not projected: %+v", got)
	}
	if got.CreatedAt != "2026-11-09T00:00:00Z" {
		t.Fatalf("created_at was not normalized to UTC: %s", got.CreatedAt)
	}
	got.InputSnapshotRefs[0] = "mutated"
	if source.InputSnapshotRefs[0] == "mutated" {
		t.Fatal("result ref must not alias manifest snapshot slice")
	}
}

func TestBuildRefKeepsCompletedAndQualityIndependent(t *testing.T) {
	input := validRefInput()
	input.QualityStatus = QualityDegraded
	got, err := BuildRef(input, testManifest(t))
	if err != nil {
		t.Fatalf("degraded completed result is representable; policy gate is separate: %v", err)
	}
	if got.ResultStatus != StatusCompleted || got.QualityStatus != QualityDegraded {
		t.Fatalf("result and quality states were conflated: %+v", got)
	}
	input.QualityStatus = QualityUnknown
	got, err = BuildRef(input, testManifest(t))
	if err != nil || got.ResultStatus != StatusCompleted || got.QualityStatus != QualityUnknown {
		t.Fatalf("result and quality states must remain independent, got=%+v err=%v", got, err)
	}
}

func TestBuildRefRejectsInvalidMetadataAndManifest(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RefInput)
	}{
		{"invalid DTO schema version", func(v *RefInput) { v.SchemaVersion = "v1" }},
		{"unknown scope", func(v *RefInput) { v.Scope = "unknown" }},
		{"stock without security", func(v *RefInput) { v.SecurityID = "" }},
		{"missing period", func(v *RefInput) { v.Period = "" }},
		{"missing algorithm version", func(v *RefInput) { v.AlgorithmVersion = "" }},
		{"missing parameter fingerprint", func(v *RefInput) { v.ParameterFingerprint = "" }},
		{"unknown quality", func(v *RefInput) { v.QualityStatus = "unknown-value" }},
		{"unknown result status", func(v *RefInput) { v.ResultStatus = "unknown-value" }},
		{"missing authorization ref", func(v *RefInput) { v.AuthorizationRef = "" }},
		{"invalid trade date", func(v *RefInput) { v.TradeDate = "2026-02-30" }},
		{"missing created at", func(v *RefInput) { v.CreatedAt = time.Time{} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validRefInput()
			tc.mutate(&input)
			if _, err := BuildRef(input, testManifest(t)); !errors.Is(err, ErrInvalidResultRef) {
				t.Fatalf("expected ErrInvalidResultRef, got %v", err)
			}
		})
	}

	bad := testManifest(t)
	bad.ResultID = "forged"
	if _, err := BuildRef(validRefInput(), bad); !errors.Is(err, ErrInvalidResultRef) {
		t.Fatalf("tampered manifest should be rejected, got %v", err)
	}
}

func TestBuildRefAllowsNonStockScopeWithoutSecurityID(t *testing.T) {
	input := validRefInput()
	input.Scope = ScopeMarket
	input.SecurityID = ""
	got, err := BuildRef(input, testManifest(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.SecurityID != "" || got.Scope != ScopeMarket {
		t.Fatalf("unexpected market ref: %+v", got)
	}
}

func TestBuildRefMatchesQNTCTR004Golden(t *testing.T) {
	got, err := BuildRef(validRefInput(), testManifest(t))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	goldenBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "doc", "quant", "golden", "result", "quant-result-ref-1.0.0.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want Ref
	if err := json.Unmarshal(goldenBytes, &want); err != nil {
		t.Fatal(err)
	}
	var decoded Ref
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, want) {
		t.Fatalf("QuantResultRef Golden mismatch: got=%s want=%s", encoded, goldenBytes)
	}
}
