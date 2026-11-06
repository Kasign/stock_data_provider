package diagnostics

import (
	"testing"
	"time"
)

func TestObservationRequiresReasonForNonHealthyStatus(t *testing.T) {
	observation := Observation{Status: StatusUnavailable, SafeMessage: "dependency unavailable"}
	if err := observation.Validate(); err == nil {
		t.Fatal("expected reason_code validation error")
	}
}

func TestObservationRejectsControlCharactersAndUnstableReasonCode(t *testing.T) {
	base := Observation{
		Status: StatusDegraded, ReasonCode: "provider.unavailable",
		SafeMessage: "provider unavailable", CheckedAt: time.Now(),
	}
	withControl := base
	withControl.SafeMessage = "provider unavailable\nsecret"
	if err := withControl.Validate(); err == nil {
		t.Fatal("expected control character validation error")
	}

	withUnstableCode := base
	withUnstableCode.ReasonCode = "Provider unavailable"
	if err := withUnstableCode.Validate(); err == nil {
		t.Fatal("expected stable reason code validation error")
	}

	withAttributeControl := base
	withAttributeControl.Attributes = map[string]string{"source": "upstream\tresponse"}
	if err := withAttributeControl.Validate(); err == nil {
		t.Fatal("expected attribute control character validation error")
	}
}

func TestObservationEnforcesAttributeBounds(t *testing.T) {
	observation := Observation{
		Status: StatusHealthy, SafeMessage: "ok", CheckedAt: time.Now(),
		Attributes: map[string]string{"oversized": string(make([]byte, 300))},
	}
	if err := observation.Validate(); err == nil {
		t.Fatal("expected attribute size validation error")
	}
}
