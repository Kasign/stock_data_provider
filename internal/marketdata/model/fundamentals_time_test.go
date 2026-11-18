package model

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestFundamentalsSnapshotNormalizeInstantsUTC(t *testing.T) {
	zone := time.FixedZone("UTC+8", 8*60*60)
	fetchedAt := time.Date(2026, 8, 29, 10, 0, 0, 123456789, zone)
	availableAt := fetchedAt.Add(time.Minute)
	observedAt := fetchedAt.Add(2 * time.Minute)
	snapshot := FundamentalsSnapshot{
		FetchedAt:        fetchedAt,
		SystemObservedAt: &observedAt,
		Sections:         []FundamentalSection{{AvailableAt: &availableAt}},
	}

	snapshot.NormalizeInstantsUTC()

	for name, value := range map[string]time.Time{
		"fetched_at":         snapshot.FetchedAt,
		"available_at":       *snapshot.Sections[0].AvailableAt,
		"system_observed_at": *snapshot.SystemObservedAt,
	} {
		if value.Location() != time.UTC || value.Format("Z07:00") != "Z" {
			t.Errorf("%s was not normalized to UTC: %s (%s)", name, value.Format(time.RFC3339Nano), value.Location())
		}
	}

	payload, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "+08:00") || !strings.Contains(string(payload), "Z") {
		t.Fatalf("serialized financial timestamps must use UTC Z: %s", payload)
	}
}

func TestFundamentalsSnapshotNormalizeInstantsUTCIsNilSafe(t *testing.T) {
	var snapshot *FundamentalsSnapshot
	snapshot.NormalizeInstantsUTC()
}
