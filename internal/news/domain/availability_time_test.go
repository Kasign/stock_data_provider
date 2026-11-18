package domain

import (
	"testing"
	"time"
)

func TestParseAvailabilityTimeSecondPrecision(t *testing.T) {
	got := ParseAvailabilityTime("2026-11-16T10:20:30+08:00")
	want := time.Date(2026, 11, 16, 2, 20, 30, 0, time.UTC)
	if got.Status != AvailabilityTimeParsed || got.Precision != AvailabilityTimePrecisionSecond ||
		got.CandidateAvailableAt == nil || !got.CandidateAvailableAt.Equal(want) ||
		got.CandidateAvailableAt.Nanosecond() != 0 || got.SourceTimezoneOrOffset != "+08:00" {
		t.Fatalf("unexpected parsed candidate: %+v", got)
	}
}

func TestParseAvailabilityTimeMinutePrecisionUsesEndOfMinute(t *testing.T) {
	got := ParseAvailabilityTime("2026-11-16T10:20+08:00")
	want := time.Date(2026, 11, 16, 2, 20, 59, 0, time.UTC)
	if got.Status != AvailabilityTimeParsed || got.Precision != AvailabilityTimePrecisionMinute ||
		got.CandidateAvailableAt == nil || !got.CandidateAvailableAt.Equal(want) {
		t.Fatalf("unexpected minute candidate: %+v", got)
	}
}

func TestParseAvailabilityTimeRejectsLowPrecisionValues(t *testing.T) {
	tests := []struct {
		raw       string
		precision AvailabilityTimePrecision
		reason    string
	}{
		{"2026", AvailabilityTimePrecisionYear, "news.available_at.year_only"},
		{"2026-11", AvailabilityTimePrecisionMonth, "news.available_at.month_only"},
		{"2026-11-16", AvailabilityTimePrecisionDate, "news.available_at.date_only"},
	}
	for _, test := range tests {
		t.Run(test.raw, func(t *testing.T) {
			got := ParseAvailabilityTime(test.raw)
			if got.Status != AvailabilityTimeUnsupportedPrecision || got.Precision != test.precision ||
				got.ReasonCode != test.reason || got.CandidateAvailableAt != nil {
				t.Fatalf("unexpected result: %+v", got)
			}
		})
	}
}

func TestParseAvailabilityTimeRejectsAmbiguousAndSubsecondValues(t *testing.T) {
	withoutZone := ParseAvailabilityTime("2026-11-16T10:20:30")
	if withoutZone.Status != AvailabilityTimeAmbiguous ||
		withoutZone.ReasonCode != "news.available_at.timezone_missing" ||
		withoutZone.CandidateAvailableAt != nil {
		t.Fatalf("timezone-less value must remain ambiguous: %+v", withoutZone)
	}

	subsecond := ParseAvailabilityTime("2026-11-16T10:20:30.123Z")
	if subsecond.Status != AvailabilityTimeUnsupportedPrecision ||
		subsecond.Precision != AvailabilityTimePrecisionSubsecond ||
		subsecond.CandidateAvailableAt != nil {
		t.Fatalf("subsecond value must not be silently rounded: %+v", subsecond)
	}
}

func TestParseAvailabilityTimePreservesRawValueAndRejectsInvalidTimestamp(t *testing.T) {
	raw := " 2026-11-16T10:20:99Z "
	got := ParseAvailabilityTime(raw)
	if got.RawValue != raw || got.Status != AvailabilityTimeUnparseable ||
		got.ReasonCode != "news.available_at.invalid_second_timestamp" ||
		got.CandidateAvailableAt != nil {
		t.Fatalf("unexpected invalid timestamp result: %+v", got)
	}
}

func TestParseAvailabilityTimeParsesExplicitUTCMinute(t *testing.T) {
	got := ParseAvailabilityTime("2026-11-16T10:20Z")
	want := time.Date(2026, 11, 16, 10, 20, 59, 0, time.UTC)
	if got.Status != AvailabilityTimeParsed || got.CandidateAvailableAt == nil ||
		!got.CandidateAvailableAt.Equal(want) || got.CandidateAvailableAt.Location() != time.UTC {
		t.Fatalf("explicit UTC minute should parse to its conservative boundary: %+v", got)
	}
}
