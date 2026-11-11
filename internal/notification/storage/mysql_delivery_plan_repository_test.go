package storage

import (
	"encoding/json"
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestValidateDeliveryPlanColumns(t *testing.T) {
	plan := domain.DeliveryPlan{
		DeliveryID: "delivery-1", SourceModule: "quant", EventID: "event-1",
		SubscriptionID: "subscription-1", SubscriptionVersion: 2,
		ChannelRef: "channel-1", ChannelConfigVersion: 3,
		TemplateID: "template-1", TemplateVersion: 4, RecipientRef: "recipient-1",
		PayloadFingerprint: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		AuthorizationPolicyVersions: []domain.AuthorizationPolicyVersion{
			{PolicyType: "source", PolicyRef: "source-1", PolicyVersion: "v1"},
		},
		AuthorizationRefs:      []string{"authorization-1"},
		PlanFingerprint:        "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		DeliveryIdempotencyKey: "idempotency-1",
	}
	policies, _ := json.Marshal(plan.AuthorizationPolicyVersions)
	refs, _ := json.Marshal(plan.AuthorizationRefs)
	columns := deliveryPlanColumns{
		deliveryID: plan.DeliveryID, sourceModule: plan.SourceModule, eventID: plan.EventID,
		subscriptionID: plan.SubscriptionID, subscriptionVersion: plan.SubscriptionVersion,
		channelRef: plan.ChannelRef, channelConfigVersion: plan.ChannelConfigVersion,
		templateID: plan.TemplateID, templateVersion: plan.TemplateVersion, recipientRef: plan.RecipientRef,
		payloadFingerprint: plan.PayloadFingerprint, policyVersionsJSON: policies,
		authorizationRefsJSON: refs, planFingerprint: plan.PlanFingerprint,
		deliveryIdempotencyKey: plan.DeliveryIdempotencyKey,
	}
	if err := validateDeliveryPlanColumns(plan, columns); err != nil {
		t.Fatalf("matching persisted columns rejected: %v", err)
	}

	// AuthorizationRefs 使用 omitempty，空切片在 plan_json 中会解码为 nil；
	// MariaDB 独立 JSON 列仍可能保存为 []，两者语义相同，重放必须接受。
	emptyRefsPlan := plan
	emptyRefsPlan.AuthorizationRefs = nil
	emptyRefsColumns := columns
	emptyRefsColumns.authorizationRefsJSON = []byte("[]")
	if err := validateDeliveryPlanColumns(emptyRefsPlan, emptyRefsColumns); err != nil {
		t.Fatalf("nil and empty authorization refs should be equivalent: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*deliveryPlanColumns)
	}{
		{name: "delivery id", mutate: func(c *deliveryPlanColumns) { c.deliveryID = "other" }},
		{name: "identity", mutate: func(c *deliveryPlanColumns) { c.eventID = "other" }},
		{name: "version", mutate: func(c *deliveryPlanColumns) { c.subscriptionVersion++ }},
		{name: "payload fingerprint", mutate: func(c *deliveryPlanColumns) { c.payloadFingerprint = "other" }},
		{name: "policy versions", mutate: func(c *deliveryPlanColumns) { c.policyVersionsJSON = []byte("[]") }},
		{name: "authorization refs", mutate: func(c *deliveryPlanColumns) { c.authorizationRefsJSON = []byte("[]") }},
		{name: "plan fingerprint", mutate: func(c *deliveryPlanColumns) { c.planFingerprint = "other" }},
		{name: "idempotency key", mutate: func(c *deliveryPlanColumns) { c.deliveryIdempotencyKey = "other" }},
		{name: "invalid policy json", mutate: func(c *deliveryPlanColumns) { c.policyVersionsJSON = []byte("{") }},
		{name: "invalid refs json", mutate: func(c *deliveryPlanColumns) { c.authorizationRefsJSON = []byte("{") }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			copy := columns
			copy.policyVersionsJSON = append([]byte(nil), columns.policyVersionsJSON...)
			copy.authorizationRefsJSON = append([]byte(nil), columns.authorizationRefsJSON...)
			test.mutate(&copy)
			err := validateDeliveryPlanColumns(plan, copy)
			if !errors.Is(err, errDeliveryPlanSnapshotColumnsMismatch) {
				t.Fatalf("expected snapshot/columns mismatch, got %v", err)
			}
		})
	}
}
