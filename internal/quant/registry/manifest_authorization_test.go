package registry

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gowebpki/jcs"
)

type resultManifestGolden struct {
	Manifest          json.RawMessage `json:"manifest"`
	FingerprintInput  json.RawMessage `json:"fingerprint_input"`
	CanonicalJSON     string          `json:"canonical_json"`
	ExpectedSHA256     string          `json:"expected_sha256"`
}

type authorizationGolden struct {
	AuthorizationRef json.RawMessage `json:"authorization_ref"`
	PolicySnapshot   struct {
		SnapshotRef string `json:"snapshot_ref"`
		Policies    []struct {
			PolicyType    string `json:"policy_type"`
			PolicyRef     string `json:"policy_ref"`
			PolicyVersion string `json:"policy_version"`
		} `json:"policies"`
	} `json:"policy_snapshot"`
	FingerprintInput json.RawMessage `json:"fingerprint_input"`
	CanonicalJSON    string          `json:"canonical_json"`
	ExpectedSHA256   string          `json:"expected_sha256"`
}

func readPlanningFixture(t *testing.T, relativePath string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "..", "doc", "整体任务规划", relativePath)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read planning fixture %s: %v", path, err)
	}
	return content
}

func TestQuantResultManifestAndAuthorizationGolden(t *testing.T) {
	var manifestGolden resultManifestGolden
	if err := json.Unmarshal(readPlanningFixture(t, filepath.Join("golden", "QNT-CTR-005-result-manifest.golden.json")), &manifestGolden); err != nil {
		t.Fatalf("decode manifest golden: %v", err)
	}
	var authorization authorizationGolden
	if err := json.Unmarshal(readPlanningFixture(t, filepath.Join("golden", "GOV-CTR-006-quant-allowed.golden.json")), &authorization); err != nil {
		t.Fatalf("decode authorization golden: %v", err)
	}

	manifestSchema, err := compileJSONSchema(readPlanningFixture(t, filepath.Join("schema", "QNT-CTR-005-result-manifest-1.0.0.schema.json")))
	if err != nil {
		t.Fatalf("compile QNT-CTR-005 schema: %v", err)
	}
	if err := validateJSONSchemaInstance(manifestSchema, manifestGolden.Manifest); err != nil {
		t.Fatalf("validate QNT-CTR-005 manifest: %v", err)
	}
	authorizationSchema, err := compileJSONSchema(readPlanningFixture(t, filepath.Join("schema", "GOV-CTR-006-authorization-ref-1.0.0.schema.json")))
	if err != nil {
		t.Fatalf("compile GOV-CTR-006 schema: %v", err)
	}
	if err := validateJSONSchemaInstance(authorizationSchema, authorization.AuthorizationRef); err != nil {
		t.Fatalf("validate Quant AuthorizationRef: %v", err)
	}

	manifestCanonical, err := jcs.Transform(manifestGolden.FingerprintInput)
	if err != nil {
		t.Fatalf("canonicalize manifest fingerprint input: %v", err)
	}
	if string(manifestCanonical) != manifestGolden.CanonicalJSON {
		t.Fatalf("manifest canonical JSON mismatch: got %s", manifestCanonical)
	}
	manifestHash := sha256.Sum256(manifestCanonical)
	manifestFingerprint := hex.EncodeToString(manifestHash[:])
	if manifestFingerprint != manifestGolden.ExpectedSHA256 {
		t.Fatalf("manifest fingerprint = %s, want %s", manifestFingerprint, manifestGolden.ExpectedSHA256)
	}

	var manifest struct {
		ResultID            string `json:"result_id"`
		ManifestFingerprint string `json:"manifest_fingerprint"`
	}
	if err := json.Unmarshal(manifestGolden.Manifest, &manifest); err != nil {
		t.Fatalf("decode manifest identity: %v", err)
	}
	if manifest.ManifestFingerprint != manifestFingerprint ||
		manifest.ResultID != "quant_result:sha256:"+manifestFingerprint {
		t.Fatalf("manifest identity is not content-addressed: %+v", manifest)
	}

	authCanonical, err := jcs.Transform(authorization.FingerprintInput)
	if err != nil {
		t.Fatalf("canonicalize authorization fingerprint input: %v", err)
	}
	if string(authCanonical) != authorization.CanonicalJSON {
		t.Fatalf("authorization canonical JSON mismatch: got %s", authCanonical)
	}
	authHash := sha256.Sum256(authCanonical)
	authFingerprint := hex.EncodeToString(authHash[:])
	if authFingerprint != authorization.ExpectedSHA256 {
		t.Fatalf("authorization fingerprint = %s, want %s", authFingerprint, authorization.ExpectedSHA256)
	}

	var auth struct {
		DecisionFingerprint string  `json:"decision_fingerprint"`
		PolicyOwner         string  `json:"policy_owner"`
		Decision            string  `json:"decision"`
		PurposeCode         string  `json:"purpose_code"`
		ContentLevel        string  `json:"content_level"`
		SubjectRef          string  `json:"subject_ref"`
		ResourceRef         string  `json:"resource_ref"`
		PolicyVersion       string  `json:"policy_version"`
		ExpiresAt           *string `json:"expires_at"`
	}
	if err := json.Unmarshal(authorization.AuthorizationRef, &auth); err != nil {
		t.Fatalf("decode authorization binding: %v", err)
	}
	if auth.DecisionFingerprint != authFingerprint ||
		auth.PolicyOwner != "quant" || auth.Decision != "allowed" ||
		auth.PurposeCode != "quant_analysis" || auth.ContentLevel != "full" ||
		auth.SubjectRef != "service:researchflow" || auth.ResourceRef != manifest.ResultID ||
		auth.PolicyVersion != authorization.PolicySnapshot.SnapshotRef || auth.ExpiresAt != nil {
		t.Fatalf("Quant AuthorizationRef binding mismatch: %+v", auth)
	}
	if len(authorization.PolicySnapshot.Policies) != 1 {
		t.Fatalf("policy snapshot entries = %d, want 1", len(authorization.PolicySnapshot.Policies))
	}
	policy := authorization.PolicySnapshot.Policies[0]
	if policy.PolicyType != "quant_result_access" || policy.PolicyRef != "quant-result-access" || policy.PolicyVersion != "1.0.0" {
		t.Fatalf("unexpected Quant policy snapshot: %+v", policy)
	}
}

func TestQuantResultManifestSchemaRejectsEmptySnapshotAndInvalidIdentity(t *testing.T) {
	schema, err := compileJSONSchema(readPlanningFixture(t, filepath.Join("schema", "QNT-CTR-005-result-manifest-1.0.0.schema.json")))
	if err != nil {
		t.Fatalf("compile QNT-CTR-005 schema: %v", err)
	}
	invalid := []string{
		`{"result_id":"quant_result:sha256:invalid","manifest_fingerprint":"invalid","result_type":"stock","schema_version":"1.0.0","system_as_of":"2026-11-06T05:02:00Z","input_snapshot_refs":[],"result_fingerprint":"invalid"}`,
		`{"result_id":"quant_result:sha256:29e81fdc80342387b910ab2c50a2732e5010defc7ce3e8030de316b37f4b8299","manifest_fingerprint":"29e81fdc80342387b910ab2c50a2732e5010defc7ce3e8030de316b37f4b8299","result_type":"stock","schema_version":"1.0.0","system_as_of":"not-a-time","input_snapshot_refs":["snapshot-a"],"result_fingerprint":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`,
	}
	for _, instance := range invalid {
		if err := validateJSONSchemaInstance(schema, []byte(instance)); err == nil {
			t.Errorf("invalid manifest unexpectedly passed: %s", instance)
		}
	}
}

func TestQuantAuthorizationBindingRejectsResourceMismatch(t *testing.T) {
	var authorization authorizationGolden
	if err := json.Unmarshal(readPlanningFixture(t, filepath.Join("golden", "GOV-CTR-006-quant-allowed.golden.json")), &authorization); err != nil {
		t.Fatal(err)
	}
	var auth map[string]json.RawMessage
	if err := json.Unmarshal(authorization.AuthorizationRef, &auth); err != nil {
		t.Fatal(err)
	}
	auth["resource_ref"] = json.RawMessage(`"quant_result:sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"`)
	changed, err := json.Marshal(auth)
	if err != nil {
		t.Fatal(err)
	}
	var binding struct {
		ResourceRef string `json:"resource_ref"`
	}
	if err := json.Unmarshal(changed, &binding); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(changed, authorization.AuthorizationRef) || binding.ResourceRef == "" {
		t.Fatal("test mutation did not change the authorization resource binding")
	}
	if binding.ResourceRef == "quant_result:sha256:29e81fdc80342387b910ab2c50a2732e5010defc7ce3e8030de316b37f4b8299" {
		t.Fatal("resource mismatch mutation unexpectedly retained the manifest result_id")
	}
}
