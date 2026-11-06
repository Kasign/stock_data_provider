package domain

import (
	"strings"
	"testing"
)

func validDeliveryPlan() DeliveryPlan {
	return DeliveryPlan{
		SourceModule: "quant", EventID: "event-1",
		SubscriptionID: "sub-1", SubscriptionVersion: 2,
		ChannelRef: "channel-1", ChannelConfigVersion: 3,
		TemplateID: "template-1", TemplateVersion: 4,
		RecipientRef: "recipient-1",
		PayloadFingerprint: strings.Repeat("a", 64),
		AuthorizationPolicyVersions: []string{"policy-b:v2", "policy-a:v1"},
		AuthorizationRefs: []string{"authz-ref-1"},
		DeliveryID: "delivery-1", DeliveryIdempotencyKey: "delivery-key-1",
	}
}

func TestDeliveryPlanFingerprintSortsPolicyVersionSet(t *testing.T) {
	plan := validDeliveryPlan()
	first, err := plan.CalculateFingerprint()
	if err != nil {
		t.Fatal(err)
	}
	plan.AuthorizationPolicyVersions = []string{"policy-a:v1", "policy-b:v2"}
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
	plan.AuthorizationPolicyVersions = []string{"policy-a:v2", "policy-b:v2"}
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
