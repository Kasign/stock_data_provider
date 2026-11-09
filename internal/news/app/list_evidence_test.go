package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

type evidenceSnapshotPageReaderStub struct {
	page EvidenceSnapshotPage
	err error
}

func (s evidenceSnapshotPageReaderStub) Execute(context.Context, ListEvidenceRequest) (EvidenceSnapshotPage, error) {
	return s.page, s.err
}

type evidenceRecordReaderStub struct {
	records map[string]ResearchEvidenceRecord
	errForVersion string
	err error
	calls int
}

func (s *evidenceRecordReaderStub) Execute(_ context.Context, request GetEvidenceRequest) (ResearchEvidenceRecord, error) {
	s.calls++
	if request.VersionID == s.errForVersion {
		return ResearchEvidenceRecord{}, s.err
	}
	return s.records[request.VersionID], nil
}

func validListEvidenceRequest(now time.Time) ListEvidenceRequest {
	return ListEvidenceRequest{
		CallerRef: "service:research", PurposeCode: EvidencePurposeResearchAnalysis,
		RequestedContentLevel: EvidenceContentSummary, SystemAsOf: now.Add(-time.Minute).UTC(),
		Filters: &EvidenceQueryFilters{NewsIDs: []string{"news-1", "news-2"}}, PageSize: 10,
	}
}

func TestListEvidenceAuthorizesEveryReferenceAndPreservesCursor(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request := validListEvidenceRequest(now)
	next := "signed-next-cursor"
	observedAt := request.SystemAsOf.Add(-time.Minute)
	pageReader := evidenceSnapshotPageReaderStub{page: EvidenceSnapshotPage{
		Items: []EvidenceSnapshotItem{
			{NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1", SystemObservedAt: observedAt},
			{NewsID: "news-2", VersionID: "version-2", ObservationID: "observation-2", SystemObservedAt: observedAt},
		},
		NextCursor: &next, SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf,
	}}
	recordReader := &evidenceRecordReaderStub{records: map[string]ResearchEvidenceRecord{
		"version-1": {NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1", SystemObservedAt: observedAt, SystemAsOf: request.SystemAsOf},
		"version-2": {NewsID: "news-2", VersionID: "version-2", ObservationID: "observation-2", SystemObservedAt: observedAt, SystemAsOf: request.SystemAsOf},
	}}
	usecase, err := NewListEvidence(pageReader, recordReader)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.SnapshotRef != "snapshot-1" ||
		result.NextCursor == nil || *result.NextCursor != next || recordReader.calls != 2 {
		t.Fatalf("unexpected evidence page: %+v calls=%d", result, recordReader.calls)
	}
}

func TestListEvidenceDoesNotReturnPartialPageWhenAnItemIsDenied(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request := validListEvidenceRequest(now)
	observedAt := request.SystemAsOf.Add(-time.Minute)
	pageReader := evidenceSnapshotPageReaderStub{page: EvidenceSnapshotPage{
		Items: []EvidenceSnapshotItem{
			{NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1", SystemObservedAt: observedAt},
			{NewsID: "news-2", VersionID: "version-2", ObservationID: "observation-2", SystemObservedAt: observedAt},
		},
		SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf,
	}}
	denied := errors.New("evidence authorization denied")
	recordReader := &evidenceRecordReaderStub{
		records: map[string]ResearchEvidenceRecord{
			"version-1": {NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1", SystemObservedAt: observedAt, SystemAsOf: request.SystemAsOf},
		},
		errForVersion: "version-2", err: denied,
	}
	usecase, err := NewListEvidence(pageReader, recordReader)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if !errors.Is(err, denied) || result.Items != nil || recordReader.calls != 2 {
		t.Fatalf("partial evidence page was returned: result=%+v err=%v calls=%d", result, err, recordReader.calls)
	}
}

func TestListEvidenceRejectsReferenceAndRecordMismatch(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request := validListEvidenceRequest(now)
	observedAt := request.SystemAsOf.Add(-time.Minute)
	pageReader := evidenceSnapshotPageReaderStub{page: EvidenceSnapshotPage{
		Items: []EvidenceSnapshotItem{
			{NewsID: "news-1", VersionID: "version-1", ObservationID: "snapshot-observation", SystemObservedAt: observedAt},
		},
		SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf,
	}}
	recordReader := &evidenceRecordReaderStub{records: map[string]ResearchEvidenceRecord{
		"version-1": {NewsID: "news-1", VersionID: "version-1", ObservationID: "different-observation", SystemObservedAt: observedAt, SystemAsOf: request.SystemAsOf},
	}}
	usecase, err := NewListEvidence(pageReader, recordReader)
	if err != nil {
		t.Fatal(err)
	}
	usecase.now = func() time.Time { return now }
	result, err := usecase.Execute(context.Background(), request)
	if !errors.Is(err, ErrEvidenceSnapshotPageUnavailable) || result.Items != nil {
		t.Fatalf("mismatched reference was not rejected: result=%+v err=%v", result, err)
	}
}
