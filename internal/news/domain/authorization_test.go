package domain

import (
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func loadNewsAuthorizationGolden(t *testing.T) NewsAuthorizationBinding {
	t.Helper()
	file, err := os.Open("../../../doc/整体任务规划/golden/NEWS-CTR-002-authorization-binding.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	var fixture struct {
		AuthorizationBinding NewsAuthorizationBinding `json:"authorization_binding"`
		SourceGoldenRef     string                    `json:"source_golden_ref"`
		ExpectedDecision    string                    `json:"expected_decision"`
		ExpectedContent     bool                      `json:"expected_content_returned"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SourceGoldenRef == "" || fixture.ExpectedDecision != string(AuthorizationDecisionConditional) || fixture.ExpectedContent {
		t.Fatalf("unexpected NEWS-CTR-002 golden metadata: %+v", fixture)
	}
	return fixture.AuthorizationBinding
}

func TestValidateNewsAuthorizationBindingGolden(t *testing.T) {
	binding := loadNewsAuthorizationGolden(t)
	if err := ValidateNewsAuthorizationBinding(binding); err != nil {
		t.Fatalf("golden binding rejected: %v", err)
	}
}

func TestValidateNewsAuthorizationBindingRejectsMismatchedIdentityAndResource(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)

	identityMismatch := base
	identityMismatch.AuthorizationRef.SubjectRef = "service:other"
	if !errors.Is(ValidateNewsAuthorizationBinding(identityMismatch), ErrAuthorizationBindingInvalid) {
		t.Fatal("mismatched trusted caller must be rejected")
	}

	resourceMismatch := base
	resourceMismatch.AuthorizationRef.ResourceRef = "news_evidence_version:news_001:v2"
	if !errors.Is(ValidateNewsAuthorizationBinding(resourceMismatch), ErrAuthorizationBindingInvalid) {
		t.Fatal("authorization for another evidence version must be rejected")
	}
}

func TestValidateNewsAuthorizationBindingRejectsIncompleteOrUnsortedPolicySnapshot(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)

	missingProvider := base
	missingProvider.PolicySnapshot.Policies = append([]AuthorizationPolicyVersion(nil), base.PolicySnapshot.Policies[1:]...)
	if !errors.Is(ValidateNewsAuthorizationBinding(missingProvider), ErrAuthorizationBindingInvalid) {
		t.Fatal("provider output policy version must be present when provider_ref is set")
	}

	unsorted := base
	unsorted.PolicySnapshot.Policies = append([]AuthorizationPolicyVersion(nil), base.PolicySnapshot.Policies...)
	unsorted.PolicySnapshot.Policies[0], unsorted.PolicySnapshot.Policies[1] = unsorted.PolicySnapshot.Policies[1], unsorted.PolicySnapshot.Policies[0]
	if !errors.Is(ValidateNewsAuthorizationBinding(unsorted), ErrAuthorizationBindingInvalid) {
		t.Fatal("policy versions must be strictly sorted")
	}
}

func TestValidateNewsAuthorizationBindingRejectsConditionalAndContentLevelViolations(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)

	conditionalWithoutConditions := base
	conditionalWithoutConditions.AuthorizationRef.Conditions = []string{}
	if !errors.Is(ValidateNewsAuthorizationBinding(conditionalWithoutConditions), ErrAuthorizationBindingInvalid) {
		t.Fatal("conditional authorization requires explicit conditions")
	}

	allowedBelowRequested := base
	allowedBelowRequested.AuthorizationRef.Decision = AuthorizationDecisionAllowed
	allowedBelowRequested.AuthorizationRef.Conditions = []string{}
	allowedBelowRequested.AuthorizationRef.ContentLevel = AuthorizationContentMetadata
	if !errors.Is(ValidateNewsAuthorizationBinding(allowedBelowRequested), ErrAuthorizationBindingInvalid) {
		t.Fatal("allowed authorization cannot provide less than the requested content level")
	}
}

func TestValidateNewsAuthorizationBindingRejectsNonCanonicalFingerprintAndTime(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)

	badFingerprint := base
	badFingerprint.AuthorizationRef.DecisionFingerprint = "ABC"
	if !errors.Is(ValidateNewsAuthorizationBinding(badFingerprint), ErrAuthorizationBindingInvalid) {
		t.Fatal("fingerprint must be lowercase SHA-256 hex")
	}

	nonUTC := base
	nonUTC.AuthorizationRef.DecidedAt = "2026-11-06T05:01:00+00:00"
	if !errors.Is(ValidateNewsAuthorizationBinding(nonUTC), ErrAuthorizationBindingInvalid) {
		t.Fatal("authorization timestamps must use UTC Z form")
	}
}
