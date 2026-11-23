package app

import (
	"testing"
	"time"
)

func TestBuildAndVerifyDailyBarsCloseSeriesSnapshot(t *testing.T) {
	input := closeSeriesSnapshotTestInput(t)
	got, err := BuildDailyBarsCloseSeriesSnapshot(input, "principal:test-user", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() error = %v", err)
	}
	if err := got.Verify(); err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if got.Manifest.SchemaVersion != "2.0.0" || got.Manifest.SnapshotRef == "" ||
		got.Manifest.ManifestFingerprint == "" || got.PayloadFingerprint == "" {
		t.Fatalf("snapshot fingerprints or schema version missing: %+v", got)
	}
	if got.Manifest.Sources[0].SourceVersion != nil {
		t.Fatalf("missing Provider data version must remain null: %+v", got.Manifest.Sources[0])
	}
	input.Payload.Bars[0].Close = "99.99"
	if got.Payload.Bars[0].Close != "43.49" {
		t.Fatal("snapshot payload aliases the caller's mutable input")
	}
}

func TestDailyBarsCloseSeriesSnapshotRejectsTampering(t *testing.T) {
	input := closeSeriesSnapshotTestInput(t)
	snapshot, err := BuildDailyBarsCloseSeriesSnapshot(input, "principal:test-user", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() error = %v", err)
	}

	payloadTampered := snapshot
	payloadTampered.Payload.Bars = append([]DailyBarsCloseSeriesBar(nil), snapshot.Payload.Bars...)
	payloadTampered.Payload.Bars[0].Close = "43.50"
	if err := payloadTampered.Verify(); err == nil {
		t.Fatal("Verify() accepted a tampered payload")
	}

	manifestTampered := snapshot
	manifestTampered.Manifest.CallerRef = "principal:other-user"
	if err := manifestTampered.Verify(); err == nil {
		t.Fatal("Verify() accepted a tampered manifest")
	}

	asOfTampered := snapshot
	asOfTampered.Manifest.SystemAsOf = "2026-11-20T07:58:00Z"
	if err := asOfTampered.Verify(); err == nil {
		t.Fatal("Verify() accepted bars observed after system_as_of")
	}
}

func TestDailyBarsCloseSeriesSnapshotFingerprintsBindManifestAndPayload(t *testing.T) {
	input := closeSeriesSnapshotTestInput(t)
	first, err := BuildDailyBarsCloseSeriesSnapshot(input, "principal:test-user", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() error = %v", err)
	}

	callerChanged, err := BuildDailyBarsCloseSeriesSnapshot(input, "principal:other-user", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() with changed caller error = %v", err)
	}
	if callerChanged.PayloadFingerprint != first.PayloadFingerprint ||
		callerChanged.Manifest.ManifestFingerprint == first.Manifest.ManifestFingerprint ||
		callerChanged.Manifest.SnapshotRef == first.Manifest.SnapshotRef {
		t.Fatal("SnapshotRef must change when Manifest changes while PayloadFingerprint remains stable")
	}

	payloadChangedInput := input
	payloadChangedInput.Payload = cloneDailyBarsCloseSeriesPayload(input.Payload)
	payloadChangedInput.Payload.Bars[0].Close = "43.50"
	payloadChangedInput.Payload.Bars[0].CloseEvidence.SourceToken = "43.5000"
	payloadChanged, err := BuildDailyBarsCloseSeriesSnapshot(payloadChangedInput, "principal:test-user", "quant_analysis")
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesSnapshot() with changed payload error = %v", err)
	}
	if payloadChanged.Manifest.ManifestFingerprint != first.Manifest.ManifestFingerprint ||
		payloadChanged.PayloadFingerprint == first.PayloadFingerprint ||
		payloadChanged.Manifest.SnapshotRef == first.Manifest.SnapshotRef {
		t.Fatal("SnapshotRef must change when Payload changes while ManifestFingerprint remains stable")
	}
}

func TestBuildDailyBarsCloseSeriesSnapshotRejectsUntrustedCallerAndPurpose(t *testing.T) {
	input := closeSeriesSnapshotTestInput(t)
	if _, err := BuildDailyBarsCloseSeriesSnapshot(input, " ", "quant_analysis"); err == nil {
		t.Fatal("BuildDailyBarsCloseSeriesSnapshot() accepted a blank caller_ref")
	}
	if _, err := BuildDailyBarsCloseSeriesSnapshot(input, "principal:test-user", "unknown_purpose"); err == nil {
		t.Fatal("BuildDailyBarsCloseSeriesSnapshot() accepted an unsupported purpose")
	}
}

func closeSeriesSnapshotTestInput(t *testing.T) DailyBarsCloseSeriesInput {
	t.Helper()
	cutoff := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	observed := cutoff.Add(-time.Minute)
	query := DailyBarsQuery{
		Symbol: "600036.SH", StartDate: "2026-11-17", EndDate: "2026-11-18",
		Limit: 2, AdjustmentBasis: AdjustmentNone, SystemObservedAtCutoff: &cutoff,
	}
	snapshot := DailyBarsV2Snapshot{
		Symbol: "600036.SH", Interval: "1d", AdjustmentBasis: AdjustmentNone,
		QualityStatus: QualityValid, SystemObservedAtCutoff: &cutoff,
		TradingCalendarRef: closeSeriesStringPointer("calendar-ref"),
		CoverageEvidence: DailyBarCoverageEvidence{
			CalendarQualityStatus: QualityValid, ExpectedTradingDays: 2, MinimumRequiredBars: 2,
			CoverageBasis: "exchange_calendar", CoverageSufficient: true,
		},
		Bars: []DailyBar{
			closeSeriesTestBar("2026-11-17", "43.49", "43.4900", 4, cutoff, observed),
			closeSeriesTestBar("2026-11-18", "43.12", "43.120", 3, cutoff, observed),
		},
	}
	input, err := BuildDailyBarsCloseSeriesInput(query, snapshot)
	if err != nil {
		t.Fatalf("BuildDailyBarsCloseSeriesInput() error = %v", err)
	}
	return input
}
