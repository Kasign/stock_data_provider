package platform

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func TestAuthorizationFingerprintVerifierAcceptsGOV006NewsGolden(t *testing.T) {
	payload, err := os.ReadFile("../../../../doc/整体任务规划/golden/NEWS-CTR-002-authorization-binding.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Binding domain.NewsAuthorizationBinding `json:"authorization_binding"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	verifier := NewAuthorizationFingerprintVerifier()
	if err := verifier.VerifyNewsAuthorizationFingerprint(context.Background(), fixture.Binding); err != nil {
		t.Fatalf("VerifyNewsAuthorizationFingerprint() error = %v", err)
	}
}

func TestAuthorizationFingerprintVerifierRejectsTamperedFingerprint(t *testing.T) {
	payload, err := os.ReadFile("../../../../doc/整体任务规划/golden/NEWS-CTR-002-authorization-binding.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Binding domain.NewsAuthorizationBinding `json:"authorization_binding"`
	}
	if err := json.Unmarshal(payload, &fixture); err != nil {
		t.Fatal(err)
	}
	fixture.Binding.AuthorizationRef.DecisionFingerprint = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := NewAuthorizationFingerprintVerifier().VerifyNewsAuthorizationFingerprint(context.Background(), fixture.Binding); err == nil {
		t.Fatal("tampered decision fingerprint was accepted")
	}
}
