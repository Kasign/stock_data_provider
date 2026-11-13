package authorization

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type goldenVector struct {
	AuthorizationRef json.RawMessage `json:"authorization_ref"`
	PolicySnapshot   json.RawMessage `json:"policy_snapshot"`
	ExpectedSHA256   string          `json:"expected_sha256"`
}

func TestVerifyFingerprintGOV006Goldens(t *testing.T) {
	files := []string{
		"GOV-CTR-006-data-allowed.golden.json",
		"GOV-CTR-006-news-conditional.golden.json",
		"GOV-CTR-006-quant-allowed.golden.json",
		"GOV-CTR-006-research-allowed.golden.json",
	}
	for _, name := range files {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join("..", "..", "..", "doc", "整体任务规划", "golden", name)
			payload, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var vector goldenVector
			if err := json.Unmarshal(payload, &vector); err != nil {
				t.Fatal(err)
			}
			if err := VerifyFingerprint(vector.AuthorizationRef, vector.PolicySnapshot); err != nil {
				t.Fatalf("VerifyFingerprint() error = %v", err)
			}
			actual, err := Fingerprint(vector.AuthorizationRef, vector.PolicySnapshot)
			if err != nil {
				t.Fatal(err)
			}
			if actual != vector.ExpectedSHA256 {
				t.Fatalf("fingerprint = %s, want %s", actual, vector.ExpectedSHA256)
			}
		})
	}
}

func TestVerifyFingerprintRejectsChangedPolicyVersion(t *testing.T) {
	ref := []byte(`{"schema_version":"1.0.0","policy_version":"snapshot-1","decision_fingerprint":"0000000000000000000000000000000000000000000000000000000000000000"}`)
	snapshot := []byte(`{"schema_version":"1.0.0","snapshot_ref":"snapshot-1","policies":[{"policy_type":"source","policy_ref":"source-1","policy_version":"1.0.1"}]}`)
	if err := VerifyFingerprint(ref, snapshot); err == nil {
		t.Fatal("VerifyFingerprint() accepted a mismatched decision fingerprint")
	}
}

func TestVerifyFingerprintRejectsUnorderedPolicySnapshot(t *testing.T) {
	ref := []byte(`{"schema_version":"1.0.0","policy_version":"snapshot-1","decision_fingerprint":"0000000000000000000000000000000000000000000000000000000000000000"}`)
	snapshot := []byte(`{"schema_version":"1.0.0","snapshot_ref":"snapshot-1","policies":[{"policy_type":"source","policy_ref":"z","policy_version":"1"},{"policy_type":"source","policy_ref":"a","policy_version":"1"}]}`)
	if err := VerifyFingerprint(ref, snapshot); err == nil {
		t.Fatal("VerifyFingerprint() accepted an unordered policy snapshot")
	}
}

func TestVerifyFingerprintRejectsDuplicateJSONMembers(t *testing.T) {
	ref := []byte(`{"schema_version":"1.0.0","policy_version":"snapshot-1","policy_version":"snapshot-1","decision_fingerprint":"0000000000000000000000000000000000000000000000000000000000000000"}`)
	snapshot := []byte(`{"schema_version":"1.0.0","snapshot_ref":"snapshot-1","policies":[{"policy_type":"source","policy_ref":"source-1","policy_version":"1"}]}`)
	if err := VerifyFingerprint(ref, snapshot); err == nil {
		t.Fatal("VerifyFingerprint() accepted duplicate JSON members")
	}
}
