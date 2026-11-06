package diagnostics

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func validObservationForContractTest() Observation {
	return Observation{
		Status:      StatusHealthy,
		SafeMessage: "ok",
		CheckedAt:   time.Date(2026, 11, 6, 14, 40, 0, 0, time.UTC),
		Duration:    time.Millisecond,
	}
}

func TestObservationValidateEnforcesPLATCTR004Bounds(t *testing.T) {
	tests := []struct {
		name  string
		apply func(*Observation)
	}{
		{name: "negative duration", apply: func(o *Observation) { o.Duration = -1 }},
		{name: "missing checked at", apply: func(o *Observation) { o.CheckedAt = time.Time{} }},
		{name: "missing safe message", apply: func(o *Observation) { o.SafeMessage = "" }},
		{name: "unknown status", apply: func(o *Observation) { o.Status = Status("unknown") }},
		{name: "missing non healthy reason", apply: func(o *Observation) { o.Status = StatusFailed }},
		{name: "invalid reason code", apply: func(o *Observation) { o.ReasonCode = "Bad.Code" }},
		{name: "reason code byte limit", apply: func(o *Observation) { o.ReasonCode = strings.Repeat("a", 161) }},
		{name: "safe message byte limit", apply: func(o *Observation) { o.SafeMessage = strings.Repeat("a", 513) }},
		{name: "evidence ref byte limit", apply: func(o *Observation) { o.EvidenceRef = strings.Repeat("a", 513) }},
		{name: "attribute count", apply: func(o *Observation) {
			o.Attributes = make(map[string]string, 17)
			for i := 0; i < 17; i++ {
				o.Attributes[strconv.Itoa(i)] = "v"
			}
		}},
		{name: "attribute key byte limit", apply: func(o *Observation) { o.Attributes = map[string]string{strings.Repeat("k", 65): "v"} }},
		{name: "attribute value byte limit", apply: func(o *Observation) { o.Attributes = map[string]string{"key": strings.Repeat("v", 257)} }},
		{name: "attribute total byte limit", apply: func(o *Observation) { o.Attributes = map[string]string{"a": strings.Repeat("v", 2048)} }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			observation := validObservationForContractTest()
			tt.apply(&observation)
			if err := observation.Validate(); err == nil {
				t.Fatal("expected Observation validation failure")
			}
		})
	}
}

func TestRunnerNormalizesCheckedAtToUTC(t *testing.T) {
	localZone := time.FixedZone("test-offset", 8*60*60)
	check := Check{ID: "timezone", Run: func(context.Context) (Observation, error) {
		return Observation{
			Status:      StatusHealthy,
			SafeMessage: "ok",
			CheckedAt:   time.Date(2026, 11, 6, 22, 40, 0, 0, localZone),
		}, nil
	}}

	report, err := (Runner{}).Run(context.Background(), []Check{check})
	if err != nil {
		t.Fatal(err)
	}
	checkedAt := report.Results[0].Observation.CheckedAt
	if checkedAt.Location() != time.UTC {
		t.Fatalf("checked_at location = %v, want UTC", checkedAt.Location())
	}
	encoded, err := json.Marshal(report.Results[0].Observation)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), "Z") {
		t.Fatalf("checked_at must serialize in UTC: %s", encoded)
	}
}

func TestRunnerPreservesInputResultOrder(t *testing.T) {
	checks := []Check{
		{ID: "slow-first", Run: func(context.Context) (Observation, error) {
			time.Sleep(20 * time.Millisecond)
			return Observation{Status: StatusHealthy, SafeMessage: "ok"}, nil
		}},
		{ID: "fast-second", Run: func(context.Context) (Observation, error) {
			return Observation{Status: StatusHealthy, SafeMessage: "ok"}, nil
		}},
	}
	report, err := (Runner{MaxConcurrent: 2}).Run(context.Background(), checks)
	if err != nil {
		t.Fatal(err)
	}
	if report.Results[0].CheckID != "slow-first" || report.Results[1].CheckID != "fast-second" {
		t.Fatalf("result order changed: %+v", report.Results)
	}
}

func TestPLATCTR004GoldenFixturesSatisfyGoValidator(t *testing.T) {
	files := []string{
		"../../../doc/platform/golden/PLAT-CTR-004-observation-healthy.golden.json",
		"../../../doc/platform/golden/PLAT-CTR-004-observation-unavailable.golden.json",
	}
	for _, path := range files {
		t.Run(path, func(t *testing.T) {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			var observation Observation
			if err := json.Unmarshal(data, &observation); err != nil {
				t.Fatal(err)
			}
			if err := observation.Validate(); err != nil {
				t.Fatalf("Golden rejected by Go validator: %v", err)
			}
			encoded, err := json.Marshal(observation)
			if err != nil {
				t.Fatal(err)
			}
			var roundTrip Observation
			if err := json.Unmarshal(encoded, &roundTrip); err != nil {
				t.Fatal(err)
			}
			if err := roundTrip.Validate(); err != nil {
				t.Fatalf("serialized Observation violates contract: %v", err)
			}
		})
	}
}
