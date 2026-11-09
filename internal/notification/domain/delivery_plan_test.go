package domain

import (
	"errors"
	"strings"
	"testing"
)

func validDeliveryPlan() DeliveryPlan {
	return DeliveryPlan{
		SourceModule: "quant", EventID: "event-1",
		SubscriptionID: "sub-1", SubscriptionVersion: 2,
		ChannelRef: "channel-1", ChannelConfigVersion: 3,
		TemplateID: "template-1", TemplateVersion: 4,
		RecipientRef:       "recipient-1",
		PayloadFingerprint: strings.Repeat("a", 64),
		AuthorizationPolicyVersions: []AuthorizationPolicyVersion{
			{PolicyType: "source", PolicyRef: "source-1", PolicyVersion: "v2"},
			{PolicyType: "provider_output", PolicyRef: "provider-1", PolicyVersion: "v1"},
		},
		AuthorizationRefs: []string{"authz-ref-1"},
		DeliveryID:        "delivery-1", DeliveryIdempotencyKey: "delivery-key-1",
	}
}

func TestDeliveryPlanFingerprintSortsPolicyVersionSet(t *testing.T) {
	plan := validDeliveryPlan()
	first, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	plan.AuthorizationPolicyVersions = []AuthorizationPolicyVersion{
		{PolicyType: "provider_output", PolicyRef: "provider-1", PolicyVersion: "v1"},
		{PolicyType: "source", PolicyRef: "source-1", PolicyVersion: "v2"},
	}
	second, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("policy version set ordering changed plan fingerprint")
	}
	plan.AuthorizationRefs = []string{"different-audit-ref"}
	third, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first != third {
		t.Fatal("audit-only AuthorizationRef changed plan fingerprint")
	}
	plan.AuthorizationPolicyVersions = []AuthorizationPolicyVersion{
		{PolicyType: "provider_output", PolicyRef: "provider-1", PolicyVersion: "v2"},
		{PolicyType: "source", PolicyRef: "source-1", PolicyVersion: "v2"},
	}
	fourth, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if first == fourth {
		t.Fatal("authorization policy version change did not change plan fingerprint")
	}
}

func TestDeliveryPlanUniqueKeyAndFingerprint(t *testing.T) {
	plan := validDeliveryPlan()
	plan.PlanFingerprint, _ = plan.CalculateFingerprint()
	if err := plan.VerifyFingerprint(); err != nil {
		t.Fatal(err)
	}
	other := plan
	other.PlanFingerprint = strings.Repeat("0", 64)
	if !plan.SameUniqueKey(other) {
		t.Fatal("same delivery tuple should share unique key")
	}
	other.RecipientRef = "recipient-2"
	if plan.SameUniqueKey(other) {
		t.Fatal("different recipient must not share delivery unique key")
	}
}

func TestDeliveryPlanAuthorizationPolicyFingerprintIncludesFullTuple(t *testing.T) {
	plan := validDeliveryPlan()
	base, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}

	changedType := plan
	changedType.AuthorizationPolicyVersions = append([]AuthorizationPolicyVersion(nil), plan.AuthorizationPolicyVersions...)
	changedType.AuthorizationPolicyVersions[0].PolicyType = "endpoint"
	typeFingerprint, err := changedType.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if base == typeFingerprint {
		t.Fatal("policy_type change must change plan fingerprint")
	}

	changedRef := plan
	changedRef.AuthorizationPolicyVersions = append([]AuthorizationPolicyVersion(nil), plan.AuthorizationPolicyVersions...)
	changedRef.AuthorizationPolicyVersions[0].PolicyRef = "source-2"
	refFingerprint, err := changedRef.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	if base == refFingerprint {
		t.Fatal("policy_ref change must change plan fingerprint")
	}
}

func TestDeliveryPlanRejectsIncompleteOrDuplicateAuthorizationPolicyTuple(t *testing.T) {
	plan := validDeliveryPlan()
	plan.AuthorizationPolicyVersions = []AuthorizationPolicyVersion{
		{PolicyType: "source", PolicyRef: "source-1", PolicyVersion: ""},
	}
	if err := plan.Validate(); !errors.Is(err, ErrInvalidDeliveryPlan) {
		t.Fatalf("incomplete policy tuple must be rejected, got %v", err)
	}

	policy := AuthorizationPolicyVersion{PolicyType: "source", PolicyRef: "source-1", PolicyVersion: "v1"}
	plan.AuthorizationPolicyVersions = []AuthorizationPolicyVersion{policy, policy}
	if err := plan.Validate(); !errors.Is(err, ErrInvalidDeliveryPlan) {
		t.Fatalf("duplicate policy tuple must be rejected, got %v", err)
	}
}
