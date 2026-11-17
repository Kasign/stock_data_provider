package storage

import "testing"

func TestCapacityLimitsRequireExplicitPositiveValues(t *testing.T) {
	for _, limits := range []CapacityLimits{
		{},
		{MaxQueuedRuns: 1},
		{MaxWorkers: 1},
		{MaxQueuedRuns: -1, MaxWorkers: 1},
		{MaxQueuedRuns: 1, MaxWorkers: -1},
	} {
		if err := limits.Validate(); err == nil {
			t.Errorf("invalid capacity limits were accepted: %+v", limits)
		}
	}
	if err := (CapacityLimits{MaxQueuedRuns: 1, MaxWorkers: 1}).Validate(); err != nil {
		t.Fatalf("valid capacity limits rejected: %v", err)
	}
}
