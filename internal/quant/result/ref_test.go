package result

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"

	"example.com/stock-analysis-flow/internal/quant/manifest"
)

func testManifest(t *testing.T) manifest.Manifest {
	t.Helper()
	payload := []byte(`{"metric_code":"momentum_20d","value":0.2}`)
	m, err := manifest.BuildWithPayload(manifest.Input{
		ResultType:           "factor",
		Scope:                "stock",
		SecurityID:           "opaque-security-ref",
		TradeDate:            "2026-11-06",
		Period:               "20d",
		QualityStatus:        "valid",
		ResultStatus:         "completed",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "momentum.v1",
		ParameterFingerprint: "sha256:parameter-fingerprint",
		SystemAsOf:           time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{"snapshot-b", "snapshot-a"},
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func testManifestWithoutSnapshots(t *testing.T) manifest.Manifest {
	t.Helper()
	payload := []byte(`{"metric_code":"pure_metric","value":1}`)
	m, err := manifest.BuildWithPayload(manifest.Input{
		ResultType:           "indicator",
		Scope:                "stock",
		SecurityID:           "opaque-security-ref",
		TradeDate:            "2026-11-06",
		Period:               "20d",
		QualityStatus:        "valid",
		ResultStatus:         "completed",
		SchemaVersion:        "1.0.0",
		PayloadSchemaVersion: "1.0.0",
		AlgorithmVersion:     "pure.v1",
		ParameterFingerprint: "sha256:pure-parameters",
		SystemAsOf:           time.Date(2026, 11, 9, 0, 0, 0, 0, time.UTC),
		InputSnapshotRefs:    []string{},
	}, payload)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func validRefInput() RefInput {
	return RefInput{
		SchemaVersion:    "1.0.0",
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

func TestBuildRefPreservesExplicitEmptySnapshotRefs(t *testing.T) {
	got, err := BuildRef(validRefInput(), testManifestWithoutSnapshots(t))
	if err != nil {
		t.Fatal(err)
	}
	if got.InputSnapshotRefs == nil || len(got.InputSnapshotRefs) != 0 {
		t.Fatalf("empty snapshot set must remain a non-nil empty slice: %#v", got.InputSnapshotRefs)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte("\"input_snapshot_refs\":[]")) {
		t.Fatalf("empty snapshot set must serialize as an empty array: %s", encoded)
	}
}

func TestBuildRefKeepsCompletedAndQualityIndependent(t *testing.T) {
	input := validRefInput()
	degraded := testManifest(t)
	degraded.QualityStatus = "degraded"
	degraded.ManifestFingerprint, _ = degraded.calculateFingerprint()
	degraded.ResultID = "quant_result:sha256:" + degraded.ManifestFingerprint
	got, err := BuildRef(input, degraded)
	if err != nil {
		t.Fatalf("degraded completed result is representable; policy gate is separate: %v", err)
	}
	if got.ResultStatus != StatusCompleted || got.QualityStatus != QualityDegraded {
		t.Fatalf("result and quality states were conflated: %+v", got)
	}
	unknown := testManifest(t)
	unknown.QualityStatus = "unknown"
	unknown.ManifestFingerprint, _ = unknown.calculateFingerprint()
	unknown.ResultID = "quant_result:sha256:" + unknown.ManifestFingerprint
	got, err = BuildRef(input, unknown)
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
		{"missing authorization ref", func(v *RefInput) { v.AuthorizationRef = "" }},
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
	market := testManifest(t)
	market.Scope = "market"
	market.SecurityID = ""
	market.ManifestFingerprint, _ = market.calculateFingerprint()
	market.ResultID = "quant_result:sha256:" + market.ManifestFingerprint
	got, err := BuildRef(input, market)
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

func TestBuildRefOutputMatchesQNTCTR004Schema(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "..", "doc", "quant", "schema", "QNT-CTR-004-quant-result-ref-1.0.0.schema.json")
	schemaDocument, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read QuantResultRef schema: %v", err)
	}

	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	const schemaURL = "https://quant.invalid/quant-result-ref.schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(schemaDocument)); err != nil {
		t.Fatalf("add QuantResultRef schema: %v", err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatalf("compile QuantResultRef schema: %v", err)
	}

	ref, err := BuildRef(validRefInput(), testManifest(t))
	if err != nil {
		t.Fatalf("BuildRef() error = %v", err)
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal QuantResultRef: %v", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var instance any
	if err := decoder.Decode(&instance); err != nil {
		t.Fatalf("decode QuantResultRef: %v", err)
	}
	if err := schema.Validate(instance); err != nil {
		t.Fatalf("BuildRef output does not satisfy QNT-CTR-004 schema: %v", err)
	}

	emptyRef, err := BuildRef(validRefInput(), testManifestWithoutSnapshots(t))
	if err != nil {
		t.Fatalf("BuildRef() rejected an explicitly empty snapshot set: %v", err)
	}
	emptyEncoded, err := json.Marshal(emptyRef)
	if err != nil {
		t.Fatal(err)
	}
	emptyDecoder := json.NewDecoder(bytes.NewReader(emptyEncoded))
	emptyDecoder.UseNumber()
	var emptyInstance any
	if err := emptyDecoder.Decode(&emptyInstance); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(emptyInstance); err != nil {
		t.Fatalf("QNT-CTR-004 schema rejected an explicitly empty snapshot array: %v", err)
	}

	invalid := make(map[string]any)
	for key, value := range instance.(map[string]any) {
		invalid[key] = value
	}
	delete(invalid, "authorization_ref")
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("schema accepted QuantResultRef without required authorization_ref")
	}
	invalid = make(map[string]any)
	for key, value := range instance.(map[string]any) {
		invalid[key] = value
	}
	invalid["unexpected"] = true

	if err := schema.Validate(invalid); err == nil {
		t.Fatal("schema accepted an unknown QuantResultRef field")
	}

	for _, field := range []string{"period", "algorithm_version", "parameter_fingerprint", "authorization_ref"} {
		invalid = make(map[string]any)
		for key, value := range instance.(map[string]any) {
			invalid[key] = value
		}
		invalid[field] = "   "
		if err := schema.Validate(invalid); err == nil {
			t.Fatalf("schema accepted whitespace-only %s", field)
		}
	}

	nonStock := make(map[string]any)
	for key, value := range instance.(map[string]any) {
		nonStock[key] = value
	}
	nonStock["scope"] = "market"
	delete(nonStock, "security_id")
	if err := schema.Validate(nonStock); err != nil {
		t.Fatalf("schema rejected market ref without security_id: %v", err)
	}

}

func TestBuildRefRejectsInvalidUTF8Metadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RefInput)
	}{
		{"authorization_ref", func(v *RefInput) { v.AuthorizationRef = string([]byte{0xff}) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validRefInput()
			tc.mutate(&input)
			if _, err := BuildRef(input, testManifest(t)); !errors.Is(err, ErrInvalidResultRef) {
				t.Fatalf("BuildRef() error = %v, want ErrInvalidResultRef", err)
			}
		})
	}
}

func TestBuildRefRejectsBlankOrNonCanonicalMetadata(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*RefInput)
	}{
		{"authorization ref whitespace", func(v *RefInput) { v.AuthorizationRef = "\t" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			input := validRefInput()
			tc.mutate(&input)
			if _, err := BuildRef(input, testManifest(t)); !errors.Is(err, ErrInvalidResultRef) {
				t.Fatalf("BuildRef() error = %v, want ErrInvalidResultRef", err)
			}
		})
	}
}

func TestQuantResultRefSchemaMatchesGoCanonicalTextAndIdentityRules(t *testing.T) {
	schemaPath := filepath.Join("..", "..", "..", "doc", "quant", "schema", "QNT-CTR-004-quant-result-ref-1.0.0.schema.json")
	schemaDocument, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read QuantResultRef schema: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.Draft = jsonschema.Draft2020
	compiler.AssertFormat = true
	const schemaURL = "https://quant.invalid/quant-result-ref-canonical.schema.json"
	if err := compiler.AddResource(schemaURL, bytes.NewReader(schemaDocument)); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := BuildRef(validRefInput(), testManifest(t))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(ref)
	if err != nil {
		t.Fatal(err)
	}
	var base map[string]any
	if err := json.Unmarshal(encoded, &base); err != nil {
		t.Fatal(err)
	}
	clone := func() map[string]any {
		copy := make(map[string]any, len(base))
		for key, value := range base {
			copy[key] = value
		}
		copy["input_snapshot_refs"] = append([]any(nil), base["input_snapshot_refs"].([]any)...)
		return copy
	}
	invalid := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"result_id is not a content address", func(v map[string]any) { v["result_id"] = "result-1" }},
		{"system_as_of has redundant fractional zeros", func(v map[string]any) { v["system_as_of"] = "2026-11-09T00:00:00.000Z" }},
		{"created_at has redundant fractional zeros", func(v map[string]any) { v["created_at"] = "2026-11-09T00:00:00.000Z" }},
		{"system_as_of uses a non-UTC offset", func(v map[string]any) { v["system_as_of"] = "2026-11-09T08:00:00+08:00" }},
		{"created_at uses a non-UTC offset", func(v map[string]any) { v["created_at"] = "2026-11-09T08:00:00+08:00" }},
		{"security_id leading whitespace", func(v map[string]any) { v["security_id"] = " security-1" }},
		{"security_id trailing whitespace", func(v map[string]any) { v["security_id"] = "security-1 " }},
		{"period leading whitespace", func(v map[string]any) { v["period"] = " 20d" }},
		{"period trailing whitespace", func(v map[string]any) { v["period"] = "20d " }},
		{"algorithm_version leading whitespace", func(v map[string]any) { v["algorithm_version"] = " momentum.v1" }},
		{"algorithm_version trailing whitespace", func(v map[string]any) { v["algorithm_version"] = "momentum.v1 " }},
		{"parameter_fingerprint leading whitespace", func(v map[string]any) { v["parameter_fingerprint"] = " sha256:x" }},
		{"parameter_fingerprint trailing whitespace", func(v map[string]any) { v["parameter_fingerprint"] = "sha256:x " }},
		{"authorization_ref leading whitespace", func(v map[string]any) { v["authorization_ref"] = " authorization-1" }},
		{"authorization_ref trailing whitespace", func(v map[string]any) { v["authorization_ref"] = "authorization-1 " }},
		{"snapshot ref leading whitespace", func(v map[string]any) { v["input_snapshot_refs"] = []any{" snapshot-1"} }},
		{"snapshot ref trailing whitespace", func(v map[string]any) { v["input_snapshot_refs"] = []any{"snapshot-1 "} }},
		{"snapshot ref whitespace only", func(v map[string]any) { v["input_snapshot_refs"] = []any{"   "} }},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			instance := clone()
			tc.mutate(instance)
			if err := schema.Validate(instance); err == nil {
				t.Fatal("Schema unexpectedly accepted non-canonical QuantResultRef")
			}
		})
	}
	if err := schema.Validate(base); err != nil {
		t.Fatalf("Schema rejected canonical BuildRef output: %v", err)
	}
	market := clone()
	market["scope"] = "market"
	delete(market, "security_id")
	if err := schema.Validate(market); err != nil {
		t.Fatalf("Schema rejected market Ref without security_id: %v", err)
	}
}
