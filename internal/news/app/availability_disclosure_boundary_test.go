package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

// 巨潮披露日资格记录：字段是披露日编码，必须显式声明边界语义引用与整秒偏移。
func testDisclosureDateQualification() AvailabilitySourceQualification {
	q := testAvailabilitySourceQualification()
	q.QualificationRef = "availability-qualification-cninfo-disclosure-v1"
	q.FieldPath = "announcements.announcementTime"
	q.ParserVersion = "cninfo-disclosure-date-conservative-v1"
	q.DeclaredTimezoneOrOffset = "+08:00"
	q.AllowedPrecisions = []domain.AvailabilityTimePrecision{domain.AvailabilityTimePrecisionDisclosureBoundary}
	q.OfficialSemanticsRef = "https://www.cninfo.com.cn/ (CSRC 指定法定披露渠道；字段为披露日)"
	q.DateBoundarySemanticsRef = "DEC-PLAT-002-073:disclosure-date-end-of-day-v1"
	return q
}

func TestQualifyAvailabilityEvidenceUsesDisclosureBoundaryForQualifiedField(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	catalog := &availabilityQualificationCatalogStub{record: testDisclosureDateQualification()}
	usecase, err := NewQualifyAvailabilityEvidence(catalog)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }

	// 真实巨潮样本：1794931200000 = 北京 2026-11-18 00:00:00（披露日 2026-11-18）
	result, err := usecase.Execute(context.Background(), QualifyAvailabilityEvidenceRequest{
		Binding:      testAvailabilityEvidenceSourceBinding(),
		FieldPath:    "announcements.announcementTime",
		RawValue:     "1794931200000",
		EvidenceRef:  "https://static.cninfo.com.cn/finalpage/2026-11-18/sample.PDF",
		EvidenceHash: strings.Repeat("c", 64),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 11, 18, 15, 59, 59, 0, time.UTC)
	if result.Candidate.Status != domain.AvailabilityTimeParsed ||
		result.Candidate.Precision != domain.AvailabilityTimePrecisionDisclosureBoundary ||
		result.Candidate.CandidateAvailableAt == nil ||
		!result.Candidate.CandidateAvailableAt.Equal(want) {
		t.Fatalf("candidate=%+v want boundary %s", result.Candidate, want)
	}
	if result.ParserVersion != "cninfo-disclosure-date-conservative-v1" {
		t.Fatalf("parser version must carry the qualified provenance: %+v", result)
	}
}

func TestAvailabilityQualificationRequiresBoundaryRefForBoundaryPrecision(t *testing.T) {
	q := testAvailabilitySourceQualification() // 无 DateBoundarySemanticsRef
	q.AllowedPrecisions = []domain.AvailabilityTimePrecision{domain.AvailabilityTimePrecisionDisclosureBoundary}
	if err := q.Validate(); err == nil {
		t.Fatal("disclosure_boundary precision requires a non-empty date boundary semantics ref")
	}
}

func TestAvailabilityQualificationRejectsBoundaryRefWithoutBoundaryPrecision(t *testing.T) {
	q := testAvailabilitySourceQualification() // AllowedPrecisions = second
	q.DateBoundarySemanticsRef = "DEC-PLAT-002-073:disclosure-date-end-of-day-v1"
	if err := q.Validate(); err == nil {
		t.Fatal("date boundary semantics ref requires disclosure_boundary precision")
	}
}

func TestQualifyAvailabilityEvidenceRejectsBoundaryFieldWithoutBoundaryRef(t *testing.T) {
	now := time.Date(2026, 11, 27, 3, 0, 0, 0, time.UTC)
	// 资格记录没有 boundary ref，但调用方给了毫秒时间戳 → 通用解析器应拒绝
	catalog := &availabilityQualificationCatalogStub{record: testAvailabilitySourceQualification()}
	usecase, err := NewQualifyAvailabilityEvidence(catalog)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	_, err = usecase.Execute(context.Background(), QualifyAvailabilityEvidenceRequest{
		Binding:      testAvailabilityEvidenceSourceBinding(),
		FieldPath:    "data.available_time",
		RawValue:     "1794931200000",
		EvidenceRef:  "https://source.example/evidence/1",
		EvidenceHash: strings.Repeat("b", 64),
	})
	if !errors.Is(err, ErrAvailabilitySourceQualificationDenied) {
		t.Fatalf("millisecond value must be rejected without boundary semantics: %v", err)
	}
}

func TestDisclosureBoundaryEvidenceIsPITEligibleInDecision(t *testing.T) {
	candidate := time.Date(2026, 11, 18, 15, 59, 59, 0, time.UTC)
	evidence := reviewAvailabilityEvidence("evidence-boundary", "version-1", domain.AvailabilityTimeParsed, &candidate)
	evidence.Precision = domain.AvailabilityTimePrecisionDisclosureBoundary
	evidence.SourceTimezoneOrOffset = "+08:00"
	if err := eligibleAvailabilityEvidence(evidence); err != nil {
		t.Fatalf("disclosure boundary evidence must be PIT-eligible: %v", err)
	}
}
