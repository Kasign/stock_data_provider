package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type evidenceSnapshotPageRepositoryStub struct {
	snapshot EvidenceSnapshot
	creates  int
	gets     int
	getErr   error
}

func (s *evidenceSnapshotPageRepositoryStub) CreateEvidenceSnapshot(_ context.Context, snapshot EvidenceSnapshot) error {
	s.creates++
	s.snapshot = snapshot
	return nil
}

func (s *evidenceSnapshotPageRepositoryStub) GetEvidenceSnapshot(_ context.Context, snapshotRef, callerRef string, now time.Time) (EvidenceSnapshot, error) {
	s.gets++
	if s.getErr != nil {
		return EvidenceSnapshot{}, s.getErr
	}
	if snapshotRef != s.snapshot.SnapshotRef || callerRef != s.snapshot.CallerRef {
		return EvidenceSnapshot{}, ErrEvidenceSnapshotUnavailable
	}
	if err := s.snapshot.ValidateAt(now); err != nil || !now.Before(s.snapshot.ExpiresAt) {
		return EvidenceSnapshot{}, ErrEvidenceSnapshotUnavailable
	}
	return s.snapshot, nil
}

func (s *evidenceSnapshotPageRepositoryStub) DeleteExpiredEvidenceSnapshots(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestListEvidenceSnapshotPageCreatesOnceAndContinuesWithoutPIT(t *testing.T) {
	base := time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC)
	request := validEvidenceListRequest(base)
	request.Filters = &EvidenceQueryFilters{}
	request.PageSize = 2
	request.SystemAsOf = base.Add(-time.Minute)

	records := make([]EvidencePITRecord, 5)
	for i := range records {
		id := string(rune('1' + i))
		observedAt := request.SystemAsOf.Add(-time.Duration(i) * time.Minute)
		newsID, versionID, observationID := "news-"+id, "version-"+id, "observation-"+id
		records[i] = EvidencePITRecord{
			Observation: domain.Observation{ID: observationID, NewsID: newsID, ObservedAt: observedAt},
			Version: domain.NewsItemVersion{ID: versionID, NewsID: newsID, ObservationID: observationID, ObservedAt: observedAt},
		}
	}
	query := &evidencePITRepositoryStub{records: records}
	store := &evidenceSnapshotPageRepositoryStub{}
	factory, err := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-page-1" })
	if err != nil {
		t.Fatal(err)
	}
	codec := newTestEvidenceCursorCodec(t, "active", map[string]string{"active": strings.Repeat("k", 32)})
	service, err := NewListEvidenceSnapshotPage(factory, codec)
	if err != nil {
		t.Fatal(err)
	}
	now := base
	service.now = func() time.Time { return now }
	factory.now = func() time.Time { return base }

	first, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("first page: %v", err)
	}
	if len(first.Items) != 2 || first.NextCursor == nil || first.SnapshotRef != "snapshot-page-1" {
		t.Fatalf("unexpected first page: %+v", first)
	}
	if query.calls != 1 || store.creates != 1 || store.gets != 0 {
		t.Fatalf("first page calls: PIT=%d create=%d get=%d", query.calls, store.creates, store.gets)
	}

	now = base.Add(time.Minute)
	secondRequest := request
	secondRequest.Cursor = first.NextCursor
	second, err := service.Execute(context.Background(), secondRequest)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Items) != 2 || second.NextCursor == nil || second.Items[0].NewsID == first.Items[1].NewsID {
		t.Fatalf("unexpected second page: %+v", second)
	}

	now = base.Add(2 * time.Minute)
	thirdRequest := request
	thirdRequest.Cursor = second.NextCursor
	third, err := service.Execute(context.Background(), thirdRequest)
	if err != nil {
		t.Fatalf("third page: %v", err)
	}
	if len(third.Items) != 1 || third.NextCursor != nil {
		t.Fatalf("unexpected final page: %+v", third)
	}
	if query.calls != 1 || store.creates != 1 || store.gets != 2 {
		t.Fatalf("continuation reran PIT or rewrote snapshot: PIT=%d create=%d get=%d", query.calls, store.creates, store.gets)
	}
}

func TestListEvidenceSnapshotPageRejectsCursorAndSnapshotFailures(t *testing.T) {
	now := time.Now().UTC()
	request := validEvidenceListRequest(now)
	request.Filters = &EvidenceQueryFilters{}
	query := &evidencePITRepositoryStub{}
	store := &evidenceSnapshotPageRepositoryStub{getErr: ErrEvidenceSnapshotUnavailable}
	factory, _ := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-1" })
	codec := newTestEvidenceCursorCodec(t, "active", map[string]string{"active": strings.Repeat("k", 32)})
	service, _ := NewListEvidenceSnapshotPage(factory, store, codec)
	service.now = func() time.Time { return now }

	bad := "not-a-signed-cursor"
	request.Cursor = &bad
	if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrEvidenceCursorInvalid) {
		t.Fatalf("invalid cursor error = %v", err)
	}
	if query.calls != 0 || store.gets != 0 {
		t.Fatalf("invalid cursor reached dependencies: PIT=%d get=%d", query.calls, store.gets)
	}

	position := ResearchEvidenceRecord{
		NewsID: "news-1", VersionID: "version-1",
		SystemObservedAt: request.SystemAsOf.Add(-time.Second),
		SystemAsOf: request.SystemAsOf,
	}
	token, err := codec.EncodeNextCursor(ListEvidenceRequest{
		CallerRef: request.CallerRef, PurposeCode: request.PurposeCode,
		RequestedContentLevel: request.RequestedContentLevel, SystemAsOf: request.SystemAsOf,
		Filters: request.Filters, PageSize: request.PageSize,
	}, "snapshot-1", position, now)
	if err != nil {
		t.Fatal(err)
	}
	request.Cursor = &token
	if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrEvidenceSnapshotUnavailable) {
		t.Fatalf("missing snapshot error = %v", err)
	}
	if query.calls != 0 || store.gets != 1 {
		t.Fatalf("cursor continuation unexpectedly reran PIT: PIT=%d get=%d", query.calls, store.gets)
	}
}
