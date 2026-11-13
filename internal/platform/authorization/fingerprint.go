// Package authorization contains shared, domain-neutral authorization primitives.
package authorization

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gowebpki/jcs"
)

var (
	ErrInvalidAuthorizationFingerprint = errors.New("authorization: invalid fingerprint input")
	ErrAuthorizationFingerprintMismatch = errors.New("authorization: fingerprint mismatch")
)

type policyVersion struct {
	PolicyType    string `json:"policy_type"`
	PolicyRef     string `json:"policy_ref"`
	PolicyVersion string `json:"policy_version"`
}

type policySnapshot struct {
	SnapshotRef   string          `json:"snapshot_ref"`
	Policies      []policyVersion `json:"policies"`
	SchemaVersion string          `json:"schema_version"`
}

// VerifyFingerprint verifies the GOV-CTR-006 decision fingerprint.
// authorizationRef and snapshot are the complete owner-issued JSON objects.
// The verifier reconstructs evaluated_policy_versions itself; caller-supplied
// fingerprint_input or canonical_json values are never trusted.
func VerifyFingerprint(authorizationRef, snapshot []byte) error {
	expected, err := fingerprintFromObjects(authorizationRef, snapshot)
	if err != nil {
		return err
	}
	var ref struct {
		Fingerprint string `json:"decision_fingerprint"`
	}
	if err := json.Unmarshal(authorizationRef, &ref); err != nil ||
		len(ref.Fingerprint) != sha256.Size*2 {
		return ErrInvalidAuthorizationFingerprint
	}
	decoded, err := hex.DecodeString(ref.Fingerprint)
	if err != nil || len(decoded) != sha256.Size {
		return ErrInvalidAuthorizationFingerprint
	}
	if subtle.ConstantTimeCompare(decoded, expected) != 1 {
		return ErrAuthorizationFingerprintMismatch
	}
	return nil
}

// Fingerprint computes the GOV-CTR-006 fingerprint from owner-issued objects.
// It is exposed for Owner implementations and conformance tests.
func Fingerprint(authorizationRef, snapshot []byte) (string, error) {
	sum, err := fingerprintFromObjects(authorizationRef, snapshot)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(sum), nil
}

func fingerprintFromObjects(authorizationRef, snapshot []byte) ([]byte, error) {
	// Run JCS over each original object first. Besides validating I-JSON input,
	// this rejects duplicate object members before encoding/json could collapse them.
	if _, err := jcs.Transform(authorizationRef); err != nil {
		return nil, ErrInvalidAuthorizationFingerprint
	}
	if _, err := jcs.Transform(snapshot); err != nil {
		return nil, ErrInvalidAuthorizationFingerprint
	}

	var ref map[string]json.RawMessage
	if err := json.Unmarshal(authorizationRef, &ref); err != nil || len(ref) == 0 {
		return nil, ErrInvalidAuthorizationFingerprint
	}
	var policy policySnapshot
	if err := json.Unmarshal(snapshot, &policy); err != nil ||
		policy.SnapshotRef == "" || policy.SchemaVersion != "1.0.0" || len(policy.Policies) == 0 {
		return nil, ErrInvalidAuthorizationFingerprint
	}
	var snapshotRef string
	if err := json.Unmarshal(ref["policy_version"], &snapshotRef); err != nil ||
		snapshotRef == "" || snapshotRef != policy.SnapshotRef {
		return nil, ErrInvalidAuthorizationFingerprint
	}
	var schemaVersion string
	if err := json.Unmarshal(ref["schema_version"], &schemaVersion); err != nil || schemaVersion != "1.0.0" {
		return nil, ErrInvalidAuthorizationFingerprint
	}

	policies := append([]policyVersion(nil), policy.Policies...)
	for i, value := range policies {
		if value.PolicyType == "" || value.PolicyRef == "" || value.PolicyVersion == "" {
			return nil, ErrInvalidAuthorizationFingerprint
		}
		if i > 0 && comparePolicy(policies[i-1], value) >= 0 {
			return nil, ErrInvalidAuthorizationFingerprint
		}
	}

	delete(ref, "authorization_ref")
	delete(ref, "decision_fingerprint")
	encodedPolicies, err := json.Marshal(policies)
	if err != nil {
		return nil, fmt.Errorf("%w: encode policy versions: %v", ErrInvalidAuthorizationFingerprint, err)
	}
	ref["evaluated_policy_versions"] = encodedPolicies
	input, err := json.Marshal(ref)
	if err != nil {
		return nil, fmt.Errorf("%w: encode fingerprint input: %v", ErrInvalidAuthorizationFingerprint, err)
	}
	canonical, err := jcs.Transform(input)
	if err != nil {
		return nil, fmt.Errorf("%w: JCS canonicalization: %v", ErrInvalidAuthorizationFingerprint, err)
	}
	sum := sha256.Sum256(canonical)
	return sum[:], nil
}

func comparePolicy(a, b policyVersion) int {
	for _, pair := range [][2]string{{a.PolicyType, b.PolicyType}, {a.PolicyRef, b.PolicyRef}, {a.PolicyVersion, b.PolicyVersion}} {
		if pair[0] < pair[1] {
			return -1
		}
		if pair[0] > pair[1] {
			return 1
		}
	}
	return 0
}
