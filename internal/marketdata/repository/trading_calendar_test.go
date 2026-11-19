package repository

import (
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/model"
)

func validRepositoryTradingCalendar() *model.TradingCalendarSnapshot {
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: "2026-10-22", EndDate: "2026-10-24",
		TotalDays: 3, TradingDays: 2,
		Items: []model.TradingCalendarDay{
			{Date: "2026-10-22", IsTradingDay: true},
			{Date: "2026-10-23", IsTradingDay: true},
			{Date: "2026-10-24", IsTradingDay: false},
		},
		Source: "calendar-test", SourceVersion: "adapter-v1",
		FetchedAt:      time.Date(2026, 10, 25, 1, 0, 0, 0, time.UTC),
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"timestamp_missing", "calendar_inferred", "timestamp_missing"},
	}
}

func TestTradingCalendarSnapshotFingerprintIsDeterministicAndContentAddressed(t *testing.T) {
	snapshot := validRepositoryTradingCalendar()
	_, firstJSON, firstFingerprint, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.FetchedAt = snapshot.FetchedAt.Add(24 * time.Hour)
	_, secondJSON, secondFingerprint, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if firstFingerprint != secondFingerprint || string(firstJSON) != string(secondJSON) {
		t.Fatal("fetch observation time must not change calendar content identity")
	}
	snapshot.Items[1].IsTradingDay = false
	snapshot.TradingDays = 1
	_, _, changedFingerprint, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if changedFingerprint == firstFingerprint {
		t.Fatal("a changed trading-day fact must produce a different calendar reference")
	}
}

func TestTradingCalendarSnapshotFingerprintBindsSourceAndQuality(t *testing.T) {
	snapshot := validRepositoryTradingCalendar()
	_, _, first, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	snapshot.SourceVersion = "adapter-v2"
	_, _, adapterChanged, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if adapterChanged == first {
		t.Fatal("adapter version change must produce a different calendar reference")
	}
	snapshot.SourceVersion = "adapter-v1"
	snapshot.QualityReasons = append(snapshot.QualityReasons, "source_conflict")
	_, _, qualityChanged, err := buildTradingCalendarSnapshotPayload(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if qualityChanged == first {
		t.Fatal("quality evidence change must produce a different calendar reference")
	}
}

func TestTradingCalendarSnapshotRejectsInvalidDateCoverage(t *testing.T) {
	snapshot := validRepositoryTradingCalendar()
	snapshot.Items = snapshot.Items[:2]
	if _, _, _, err := buildTradingCalendarSnapshotPayload(snapshot); err == nil {
		t.Fatal("incomplete calendar coverage must be rejected")
	}
}

func TestTradingCalendarSnapshotRefRejectsMalformedReference(t *testing.T) {
	for _, ref := range []string{"", "data_calendar:sha256:xyz", "data_calendar:sha256:" + strings.Repeat("A", 64)} {
		if _, err := parseTradingCalendarSnapshotRef(ref); err == nil {
			t.Fatalf("expected malformed reference to be rejected: %q", ref)
		}
	}
}
