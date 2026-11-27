package app

import (
	"testing"
	"time"
)

func limitPoolTestSources() []DailyBarsCloseSeriesSource {
	version := "eastmoney-limit-pool-v1"
	return []DailyBarsCloseSeriesSource{{SourceRef: "eastmoney", SourceVersion: &version}}
}

func limitPoolTestPayload(t *testing.T) LimitPoolFactsPayload {
	t.Helper()
	up := limitPoolSnapshot("up", "2026-11-27", 1, "600519.SH")
	down := limitPoolSnapshot("down", "2026-11-27", 0)
	payload, err := BuildLimitPoolFactsPayload(up, down, "600036.SH")
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func TestBuildLimitPoolFactsSnapshotIsStableAndVerifiable(t *testing.T) {
	payload := limitPoolTestPayload(t)
	systemAsOf := payload.SystemObservedAt.Add(time.Minute)
	snapshot, err := BuildLimitPoolFactsSnapshot(
		payload, limitPoolTestSources(), "service:researchflow", "risk_review", systemAsOf,
	)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Manifest.TradeDate != "2026-11-27" || snapshot.Manifest.Symbol != "600036.SH" {
		t.Fatalf("manifest identity = %+v", snapshot.Manifest)
	}
	if err := snapshot.Verify(); err != nil {
		t.Fatalf("valid snapshot failed verification: %v", err)
	}
	again, err := BuildLimitPoolFactsSnapshot(
		payload, limitPoolTestSources(), "service:researchflow", "risk_review", systemAsOf,
	)
	if err != nil {
		t.Fatal(err)
	}
	if again.Manifest.SnapshotRef != snapshot.Manifest.SnapshotRef {
		t.Fatal("snapshot_ref must be deterministic for identical inputs")
	}
}

func TestBuildLimitPoolFactsSnapshotChangesWithTradeDate(t *testing.T) {
	payload := limitPoolTestPayload(t)
	base, err := BuildLimitPoolFactsSnapshot(
		payload, limitPoolTestSources(), "service:researchflow", "risk_review", payload.SystemObservedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	other := payload
	other.TradeDate = "2026-11-26"
	baseFingerprint, err := fingerprintLimitPoolFactsPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	otherFingerprint, err := fingerprintLimitPoolFactsPayload(other)
	if err != nil {
		t.Fatal(err)
	}
	if baseFingerprint == otherFingerprint {
		t.Fatal("payload fingerprint must include trade_date")
	}
	changed, err := BuildLimitPoolFactsSnapshot(
		other, limitPoolTestSources(), "service:researchflow", "risk_review", other.SystemObservedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Manifest.SnapshotRef == base.Manifest.SnapshotRef {
		t.Fatal("changing trade_date must change snapshot_ref")
	}
}

func TestLimitPoolFactsSnapshotRejectsTampering(t *testing.T) {
	payload := limitPoolTestPayload(t)
	snapshot, err := BuildLimitPoolFactsSnapshot(
		payload, limitPoolTestSources(), "service:researchflow", "risk_review", payload.SystemObservedAt.Add(time.Minute),
	)
	if err != nil {
		t.Fatal(err)
	}

	tamperedStatus := snapshot
	tamperedStatus.Payload.LimitStatus = LimitPoolStatusUnknown
	if err := tamperedStatus.Verify(); err == nil {
		t.Fatal("tampered payload status was accepted")
	}

	tamperedDate := snapshot
	tamperedDate.Manifest.TradeDate = "2026-11-26"
	if err := tamperedDate.Verify(); err == nil {
		t.Fatal("tampered manifest trade_date was accepted")
	}

	tamperedSource := snapshot
	tamperedSource.Manifest.Sources = []DailyBarsCloseSeriesSource{{SourceRef: "other"}}
	if err := tamperedSource.Verify(); err == nil {
		t.Fatal("tampered manifest sources were accepted")
	}
}

func TestBuildLimitPoolFactsSnapshotRejectsInvalidEnvelope(t *testing.T) {
	payload := limitPoolTestPayload(t)

	cases := []struct {
		name       string
		sources    []DailyBarsCloseSeriesSource
		caller     string
		purpose    string
		systemAsOf time.Time
	}{
		{"no sources", nil, "service:researchflow", "risk_review", payload.SystemObservedAt.Add(time.Minute)},
		{"duplicate source", append(limitPoolTestSources(), limitPoolTestSources()...), "service:researchflow", "risk_review", payload.SystemObservedAt.Add(time.Minute)},
		{"blank caller", limitPoolTestSources(), "", "risk_review", payload.SystemObservedAt.Add(time.Minute)},
		{"unsupported purpose", limitPoolTestSources(), "service:researchflow", "marketing", payload.SystemObservedAt.Add(time.Minute)},
		{"system_as_of before observation", limitPoolTestSources(), "service:researchflow", "risk_review", payload.SystemObservedAt.Add(-time.Second)},
		{"non-UTC system_as_of", limitPoolTestSources(), "service:researchflow", "risk_review", payload.SystemObservedAt.In(time.FixedZone("CST", 8*3600))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := BuildLimitPoolFactsSnapshot(payload, tc.sources, tc.caller, tc.purpose, tc.systemAsOf); err == nil {
				t.Fatal("invalid envelope was accepted")
			}
		})
	}
}
