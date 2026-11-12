package domain

import (
	"strings"
	"testing"
	"time"
)

func TestFanoutWorkItemIDIsStableAndScoped(t *testing.T) {
	first := FanoutWorkItemID("quant", "event-1")
	if first != FanoutWorkItemID("quant", "event-1") || first == FanoutWorkItemID("research", "event-1") ||
		!strings.HasPrefix(first, "ntf-fw-") {
		t.Fatalf("unexpected fanout identity: %s", first)
	}
}

func TestFanoutWorkItemValidatesLeaseAndRetryState(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	base := FanoutWorkItem{
		WorkItemID: FanoutWorkItemID("quant", "event-1"), SourceModule: "quant", EventID: "event-1",
		EventType: "quant.risk_decision.changed", PayloadFingerprint: strings.Repeat("a", 64),
		Status: FanoutPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := base.Validate(); err != nil {
		t.Fatal(err)
	}
	processing := base
	processing.Status = FanoutProcessing
	processing.HasSnapshot = true
	processing.AttemptCount = 1
	processing.LeaseWorkerID = "worker-1"
	processing.LeaseExpiresAt = now.Add(time.Minute)
	processing.FencingToken = 1
	if err := processing.Validate(); err != nil {
		t.Fatal(err)
	}
	invalid := processing
	invalid.LeaseWorkerID = ""
	if err := invalid.Validate(); err == nil {
		t.Fatal("processing state must carry a lease owner")
	}
	retry := base
	retry.Status = FanoutRetryWait
	retry.HasSnapshot = true
	retry.AttemptCount = 1
	retry.FencingToken = 1
	retry.NextAttemptAt = now.Add(time.Second)
	if err := retry.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestFanoutWorkItemRequiresSnapshotAfterInitialPendingState(t *testing.T) {
	now := time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)
	item := FanoutWorkItem{
		WorkItemID: FanoutWorkItemID("quant", "event-2"), SourceModule: "quant", EventID: "event-2",
		EventType: "quant.risk_decision.changed", PayloadFingerprint: strings.Repeat("b", 64),
		Status: FanoutPending, CreatedAt: now, UpdatedAt: now,
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("new pending item may defer its first snapshot: %v", err)
	}
	item.AttemptCount = 1
	if err := item.Validate(); err == nil {
		t.Fatal("a previously claimed item must not lose its fixed snapshot")
	}
}
