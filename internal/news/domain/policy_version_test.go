package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
	"time"
)

func TestValidateNewsPolicyVersionRejectsInvalidMetadataFieldPaths(t *testing.T) {
	for _, fields := range [][]string{
		{"payload.*"},
		{"payload.publishTime", "payload.publishTime"},
		{"payload[0].publishTime"},
	} {
		payload, err := json.Marshal(SourcePolicy{
			Status:                AuthorizationApproved,
			EvidenceRef:           "policy-evidence",
			AllowedMetadataFields: fields,
		})
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(payload)
		version := NewsPolicyVersion{
			PolicyType: NewsPolicyTypeSource, PolicyRef: "source-policy",
			PolicyVersion: "source-policy-v1", Payload: payload,
			PayloadHash: hex.EncodeToString(hash[:]), CreatedAt: time.Now().UTC(),
		}
		if err := ValidateNewsPolicyVersion(version); err == nil {
			t.Fatalf("invalid metadata field policy accepted: %v", fields)
		}
	}
}

func TestValidateNewsPolicyVersionKeepsLegacyFieldPolicyDefaultDenied(t *testing.T) {
	payload, err := json.Marshal(SourcePolicy{
		Status:      AuthorizationApproved,
		EvidenceRef: "legacy-policy-evidence",
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256(payload)
	version := NewsPolicyVersion{
		PolicyType: NewsPolicyTypeSource, PolicyRef: "legacy-source-policy",
		PolicyVersion: "legacy-source-policy-v1", Payload: payload,
		PayloadHash: hex.EncodeToString(hash[:]), CreatedAt: time.Now().UTC(),
	}
	if err := ValidateNewsPolicyVersion(version); err != nil {
		t.Fatalf("legacy policy without field list should remain readable: %v", err)
	}
	if len(mustDecodeSourcePolicy(t, payload).AllowedMetadataFields) != 0 {
		t.Fatal("legacy policy unexpectedly grants metadata field permission")
	}
}

func mustDecodeSourcePolicy(t *testing.T, payload []byte) SourcePolicy {
	t.Helper()
	var policy SourcePolicy
	if err := decodeNewsPolicyPayload(payload, &policy); err != nil {
		t.Fatal(err)
	}
	return policy
}
