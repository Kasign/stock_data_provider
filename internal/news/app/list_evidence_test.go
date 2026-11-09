package app

import (
	"context"
	"errors"
	"testing"
	"time"
)

type evidenceSnapshotPageReaderStub struct {
	page      EvidenceSnapshotPage
	err       error
	afterRead func()
}

func (s evidenceSnapshotPageReaderStub) Execute(context.Context, ListEvidenceRequest) (EvidenceSnapshotPage, error) {
	if s.afterRead != nil {
		s.afterRead()
	}
	return s.page, s.err
}

type evidenceRecordReaderStub struct {
	records       map[string]ResearchEvidenceRecord
	errForVersion string
	err           error
	calls         int
	afterRead     func()
}

func (s *evidenceRecordReaderStub) Execute(_ context.Context, request GetEvidenceRequest) (ResearchEvidenceRecord, error) {
	s.calls++
	if s.afterRead != nil { s.afterRead() }
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


func TestListEvidenceRejectsMalformedPageMetadataBeforeResolvingRecords(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request := validListEvidenceRequest(now)
	observedAt := request.SystemAsOf.Add(-time.Minute)
	validItem := EvidenceSnapshotItem{
		NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1",
		SystemObservedAt: observedAt,
	}
	badCursor := "bad\ncursor"
	cases := []struct {
		name string
		page EvidenceSnapshotPage
	}{
		{
			name: "invalid next cursor",
			page: EvidenceSnapshotPage{Items: []EvidenceSnapshotItem{validItem}, NextCursor: &badCursor, SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf},
		},
		{
			name: "short page with next cursor",
			page: EvidenceSnapshotPage{Items: []EvidenceSnapshotItem{validItem}, NextCursor: func() *string { value := "signed-cursor"; return &value }(), SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf},
		},
		{
			name: "duplicate evidence reference",
			page: EvidenceSnapshotPage{Items: []EvidenceSnapshotItem{validItem, validItem}, SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf},
		},
		{
			name: "non canonical identifier",
			page: EvidenceSnapshotPage{Items: []EvidenceSnapshotItem{{NewsID: " news-1 ", VersionID: "version-1", ObservationID: "observation-1", SystemObservedAt: observedAt}}, SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf},
		},
		{
			name: "unstable item ordering",
			page: EvidenceSnapshotPage{Items: []EvidenceSnapshotItem{
				{NewsID: "news-2", VersionID: "version-2", ObservationID: "observation-2", SystemObservedAt: observedAt},
				validItem,
			}, SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			recordReader := &evidenceRecordReaderStub{}
			usecase, err := NewListEvidence(evidenceSnapshotPageReaderStub{page: test.page}, recordReader)
			if err != nil {
				t.Fatal(err)
			}
			usecase.now = func() time.Time { return now }
			if _, err := usecase.Execute(context.Background(), request); !errors.Is(err, ErrEvidenceSnapshotPageUnavailable) {
				t.Fatalf("Execute() error = %v, want snapshot page unavailable", err)
			}
			if recordReader.calls != 0 {
				t.Fatalf("malformed page reached record reader %d times", recordReader.calls)
			}
		})
	}
}

func TestListEvidenceStopsResolvingItemsWhenContextIsCanceled(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	request := validListEvidenceRequest(now)
	observedAt := request.SystemAsOf.Add(-time.Minute)
	page := EvidenceSnapshotPage{Items: []EvidenceSnapshotItem{
		{NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1", SystemObservedAt: observedAt},
		{NewsID: "news-2", VersionID: "version-2", ObservationID: "observation-2", SystemObservedAt: observedAt},
	}, SnapshotRef: "snapshot-1", SystemAsOf: request.SystemAsOf}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader := &evidenceRecordReaderStub{
		records: map[string]ResearchEvidenceRecord{
			"version-1": {NewsID: "news-1", VersionID: "version-1", ObservationID: "observation-1", SystemObservedAt: observedAt, SystemAsOf: request.SystemAsOf},
			"version-2": {NewsID: "news-2", VersionID: "version-2", ObservationID: "observation-2", SystemObservedAt: observedAt, SystemAsOf: request.SystemAsOf},
		},
		afterRead: cancel,
	}
	usecase, _ := NewListEvidence(evidenceSnapshotPageReaderStub{page:page}, reader)
	usecase.now = func() time.Time { return now }
	if _, err := usecase.Execute(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("Execute() error = %v, want context.Canceled", err)
	}
	if reader.calls != 1 {
		t.Fatalf("canceled page resolved %d records, want 1", reader.calls)
	}
}
