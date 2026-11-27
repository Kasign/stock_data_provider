package storage

import (
	"database/sql"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

func TestSamePersistedAvailabilityEvidenceIgnoresWriteTime(t *testing.T) {
	observedAt := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	candidateAt := time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC)
	ref := "response-snapshot:cninfo/cninfo-his-announcement@" + "d"
	hash := "1793376000000"
	evidence := domain.AvailabilityEvidence{
		ID: "availability-evidence-1", NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1",
		FieldPath: "announcements.announcementTime", RawValue: "1793376000000",
		SourceTimezoneOrOffset: "+08:00", Precision: domain.AvailabilityTimePrecisionDisclosureBoundary,
		ParseStatus: domain.AvailabilityTimeParsed, CandidateAvailableAt: &candidateAt,
		ParserVersion: "cninfo-disclosure-date-conservative-v1", EvidenceRef: &ref, EvidenceHash: &hash,
		ObservedAt: observedAt, RecordedAt: observedAt.Add(time.Second), RetentionUntil: observedAt.Add(24 * time.Hour),
	}
	if !samePersistedAvailabilityEvidence(
		"news-1", "version-1", "observation-1", "announcements.announcementTime", "1793376000000",
		"+08:00", string(domain.AvailabilityTimePrecisionDisclosureBoundary),
		string(domain.AvailabilityTimeParsed), "", "cninfo-disclosure-date-conservative-v1",
		sql.NullTime{Valid: true, Time: candidateAt},
		sql.NullString{Valid: true, String: ref}, sql.NullString{Valid: true, String: hash},
		sql.NullTime{Valid: true, Time: observedAt}, evidence,
	) {
		t.Fatal("identical semantic payload with a different write time must be idempotent")
	}
}

func TestSamePersistedAvailabilityEvidenceRejectsChangedCandidate(t *testing.T) {
	observedAt := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	candidateAt := time.Date(2026, 10, 31, 15, 59, 59, 0, time.UTC)
	changed := candidateAt.Add(time.Second)
	evidence := domain.AvailabilityEvidence{
		ID: "availability-evidence-1", NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1",
		FieldPath: "announcements.announcementTime", RawValue: "1793376000000",
		SourceTimezoneOrOffset: "+08:00", Precision: domain.AvailabilityTimePrecisionDisclosureBoundary,
		ParseStatus: domain.AvailabilityTimeParsed, CandidateAvailableAt: &changed,
		ParserVersion: "cninfo-disclosure-date-conservative-v1",
		ObservedAt:    observedAt, RecordedAt: observedAt.Add(time.Second), RetentionUntil: observedAt.Add(24 * time.Hour),
	}
	if samePersistedAvailabilityEvidence(
		"news-1", "version-1", "observation-1", "announcements.announcementTime", "1793376000000",
		"+08:00", string(domain.AvailabilityTimePrecisionDisclosureBoundary),
		string(domain.AvailabilityTimeParsed), "", "cninfo-disclosure-date-conservative-v1",
		sql.NullTime{Valid: true, Time: candidateAt},
		sql.NullString{}, sql.NullString{},
		sql.NullTime{Valid: true, Time: observedAt}, evidence,
	) {
		t.Fatal("changed candidate time must not be treated as the same Evidence")
	}
}
