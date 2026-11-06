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

func TestObservationEnforcesAttributeBounds(t *testing.T) {
	observation := Observation{
		Status: StatusHealthy, SafeMessage: "ok", CheckedAt: time.Now(),
		Attributes: map[string]string{"oversized": string(make([]byte, 300))},
	}
	if err := observation.Validate(); err == nil {
		t.Fatal("expected attribute size validation error")
	}
}
