package application

import (
	"testing"

	"example.com/stock-analysis-flow/internal/notification/domain"
)

func TestVerifiedUnknownDeliveryEvidenceBindsSnapshotIdentity(t *testing.T) {
	delivery, channel, attempts, evidence, _ := unknownReconciliationFixture(t)
	verified := verifiedUnknownDeliveryEvidence{
		evidence: evidence, deliveryVersion: delivery.Version,
		attemptID: attempts[0].AttemptID, channel: cloneChannelSnapshot(channel),
	}
	if !verified.matches(delivery, channel, attempts) {
		t.Fatal("evidence from the exact registry lookup snapshot should match")
	}

	changedDelivery := delivery
	changedDelivery.Version++
	if verified.matches(changedDelivery, channel, attempts) {
		t.Fatal("evidence must not be reusable against a different delivery version")
	}
	changedChannel := cloneChannelSnapshot(channel)
	changedChannel.Enabled = !changedChannel.Enabled
	if verified.matches(delivery, changedChannel, attempts) {
		t.Fatal("evidence must not be reusable against a different channel snapshot")
	}
	changedAttempts := append([]domain.DeliveryAttempt(nil), attempts...)
	changedAttempts[0].AttemptID += "-other"
	if verified.matches(delivery, channel, changedAttempts) {
		t.Fatal("evidence must not be reusable against a different attempt")
	}
}
