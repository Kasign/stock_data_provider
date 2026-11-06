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
		SourceGoldenRef      string                   `json:"source_golden_ref"`
		ExpectedBinding      map[string]string        `json:"expected_binding"`
		ExpectedDecision     string                   `json:"expected_decision"`
		ExpectedContent      bool                     `json:"expected_content_returned"`
	}
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SourceGoldenRef == "" || len(fixture.ExpectedBinding) == 0 ||
		fixture.ExpectedDecision != string(AuthorizationDecisionConditional) || fixture.ExpectedContent {
		t.Fatalf("unexpected NEWS-CTR-002 golden metadata: %+v", fixture)
	}
	binding := fixture.AuthorizationBinding
	if fixture.ExpectedBinding["subject_ref"] != "caller_ref" ||
		fixture.ExpectedBinding["resource_ref"] != "resource_ref" ||
		fixture.ExpectedBinding["purpose_code"] != "purpose_code" ||
		fixture.ExpectedBinding["content_level"] != "requested_content_level" ||
		fixture.ExpectedBinding["policy_version"] != "policy_snapshot.snapshot_ref" ||
		binding.AuthorizationRef.SubjectRef != binding.CallerRef ||
		binding.AuthorizationRef.ResourceRef != binding.ResourceRef ||
		binding.AuthorizationRef.PurposeCode != binding.PurposeCode ||
		binding.AuthorizationRef.ContentLevel != binding.RequestedContentLevel ||
		binding.AuthorizationRef.PolicyVersion != binding.PolicySnapshot.SnapshotRef {
		t.Fatalf("NEWS-CTR-002 expected binding mapping does not match DTO: %+v", fixture.ExpectedBinding)
	}
	return binding
}

func TestValidateNewsAuthorizationBindingGolden(t *testing.T) {
	binding := loadNewsAuthorizationGolden(t)
	if err := ValidateNewsAuthorizationBinding(binding); err != nil {
		t.Fatalf("golden binding rejected: %v", err)
	}
}

func TestValidateNewsAuthorizationBindingAcceptsExplicitNullableRefs(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)
	base.ProviderRef = NullableOpaqueRef{Present: true}
	base.EndpointRef = NullableOpaqueRef{Present: true}

	policies := make([]AuthorizationPolicyVersion, 0, 1)
	for _, policy := range base.PolicySnapshot.Policies {
		if policy.PolicyType == "source" {
			policies = append(policies, policy)
		}
	}
	base.PolicySnapshot.Policies = policies
	if err := ValidateNewsAuthorizationBinding(base); err != nil {
		t.Fatalf("explicit null provider_ref/endpoint_ref with source inheritance rejected: %v", err)
	}
}

func TestValidateNewsAuthorizationBindingRequiresNullableFields(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)

	missingProviderField := base
	missingProviderField.ProviderRef.Present = false
	if !errors.Is(ValidateNewsAuthorizationBinding(missingProviderField), ErrAuthorizationBindingInvalid) {
		t.Fatal("provider_ref must be present even when nullable")
	}

	missingEndpointField := base
	missingEndpointField.EndpointRef.Present = false
	if !errors.Is(ValidateNewsAuthorizationBinding(missingEndpointField), ErrAuthorizationBindingInvalid) {
		t.Fatal("endpoint_ref must be present even when nullable")
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
	missingProvider.PolicySnapshot.Policies = make([]AuthorizationPolicyVersion, 0, len(base.PolicySnapshot.Policies)-1)
	for _, policy := range base.PolicySnapshot.Policies {
		if policy.PolicyType != "provider_output" {
			missingProvider.PolicySnapshot.Policies = append(missingProvider.PolicySnapshot.Policies, policy)
		}
	}
	if !errors.Is(ValidateNewsAuthorizationBinding(missingProvider), ErrAuthorizationBindingInvalid) {
		t.Fatal("provider output policy version must be present when provider_ref is set")
	}

	duplicateSource := base
	duplicateSource.PolicySnapshot.Policies = append([]AuthorizationPolicyVersion(nil), base.PolicySnapshot.Policies...)
	duplicateSource.PolicySnapshot.Policies = append(duplicateSource.PolicySnapshot.Policies, AuthorizationPolicyVersion{
		PolicyType: "source", PolicyRef: "sec-edgar", PolicyVersion: "2.2.0",
	})
	if !errors.Is(ValidateNewsAuthorizationBinding(duplicateSource), ErrAuthorizationBindingInvalid) {
		t.Fatal("one snapshot cannot contain multiple effective versions for the same source policy")
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

func TestValidateNewsAuthorizationBindingRequiresConditionsArray(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)
	base.AuthorizationRef.Conditions = nil
	if !errors.Is(ValidateNewsAuthorizationBinding(base), ErrAuthorizationBindingInvalid) {
		t.Fatal("conditions must be an array, not null")
	}
}

func TestValidateNewsAuthorizationBindingRequiresExpiresAtField(t *testing.T) {
	base := loadNewsAuthorizationGolden(t)
	base.AuthorizationRef.ExpiresAt.Present = false
	if !errors.Is(ValidateNewsAuthorizationBinding(base), ErrAuthorizationBindingInvalid) {
		t.Fatal("expires_at must be present even when its value is null")
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
