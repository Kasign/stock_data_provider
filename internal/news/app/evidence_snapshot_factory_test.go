package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"example.com/stock-analysis-flow/internal/news/domain"
)

type evidencePITRepositoryStub struct {
	records []EvidencePITRecord
	err     error
	calls   int
}

func (s *evidencePITRepositoryStub) GetEvidenceVersionAt(context.Context, GetEvidenceRequest) (EvidencePITRecord, error) {
	return EvidencePITRecord{}, ErrEvidencePITNotFound
}

func (s *evidencePITRepositoryStub) ListEvidenceVersionsAt(context.Context, ListEvidenceRequest) ([]EvidencePITRecord, error) {
	s.calls++
	return s.records, s.err
}

func evidencePITRecords(asOf time.Time) []EvidencePITRecord {
	older := asOf.Add(-2 * time.Minute)
	newer := asOf.Add(-time.Minute)
	return []EvidencePITRecord{
		{
			Observation: domain.Observation{ID: "observation-2", NewsID: "news-2", ObservedAt: newer},
			Version: domain.NewsItemVersion{ID: "version-2", NewsID: "news-2", ObservationID: "observation-2", ObservedAt: newer},
		},
		{
			Observation: domain.Observation{ID: "observation-1", NewsID: "news-1", ObservedAt: older},
			Version: domain.NewsItemVersion{ID: "version-1", NewsID: "news-1", ObservationID: "observation-1", ObservedAt: older},
		},
	}
}

func TestCreateEvidenceSnapshotForQueryFreezesOnlyPITReferences(t *testing.T) {
	now := time.Now().UTC()
	request := validEvidenceListRequest(now)
	request.Filters = &EvidenceQueryFilters{NewsIDs: []string{"news-1", "news-2"}}
	request.SystemAsOf = now.Add(-time.Second)

	query := &evidencePITRepositoryStub{records: evidencePITRecords(request.SystemAsOf)}
	store := &evidenceSnapshotRepositoryStub{}
	service, err := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-1" })
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return now }

	snapshot, err := service.Execute(context.Background(), request)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if query.calls != 1 || store.calls != 1 {
		t.Fatalf("query calls=%d snapshot writes=%d", query.calls, store.calls)
	}
	if snapshot.SnapshotRef != "snapshot-1" || len(snapshot.Items) != 2 {
		t.Fatalf("unexpected snapshot: %+v", snapshot)
	}
	if snapshot.Items[0].NewsID != "news-2" || snapshot.Items[1].NewsID != "news-1" {
		t.Fatalf("PIT ordering was not preserved: %+v", snapshot.Items)
	}
	if store.created.Items[0].VersionID != "version-2" {
		t.Fatalf("snapshot store did not receive immutable references: %+v", store.created.Items)
	}
}

func TestCreateEvidenceSnapshotForQueryRejectsOversizedAndInvalidPITResults(t *testing.T) {
	now := time.Now().UTC()
	request := validEvidenceListRequest(now)
	request.Filters = &EvidenceQueryFilters{}
	request.SystemAsOf = now.Add(-time.Second)

	t.Run("oversized", func(t *testing.T) {
		records := make([]EvidencePITRecord, MaximumEvidenceSnapshotSize+1)
		query := &evidencePITRepositoryStub{records: records}
		store := &evidenceSnapshotRepositoryStub{}
		service, _ := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-1" })
		service.now = func() time.Time { return now }
		if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrEvidenceSnapshotTooLarge) {
			t.Fatalf("Execute() error = %v", err)
		}
		if store.calls != 0 {
			t.Fatalf("oversized PIT result reached snapshot storage: %d", store.calls)
		}
	})

	t.Run("mismatched observation", func(t *testing.T) {
		record := evidencePITRecords(request.SystemAsOf)[0]
		record.Version.ObservationID = "other-observation"
		query := &evidencePITRepositoryStub{records: []EvidencePITRecord{record}}
		store := &evidenceSnapshotRepositoryStub{}
		service, _ := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-1" })
		service.now = func() time.Time { return now }
		if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrInvalidEvidenceSnapshot) {
			t.Fatalf("Execute() error = %v", err)
		}
		if store.calls != 0 {
			t.Fatalf("invalid PIT result reached snapshot storage: %d", store.calls)
		}
	})
}

func TestCreateEvidenceSnapshotForQueryDoesNotRequeryOnCursorRequest(t *testing.T) {
	now := time.Now().UTC()
	request := validEvidenceListRequest(now)
	cursor := "signed-cursor"
	request.Cursor = &cursor
	query := &evidencePITRepositoryStub{}
	store := &evidenceSnapshotRepositoryStub{}
	service, _ := NewCreateEvidenceSnapshotForQuery(query, store, func() string { return "snapshot-1" })
	service.now = func() time.Time { return now }

	if _, err := service.Execute(context.Background(), request); !errors.Is(err, ErrInvalidEvidenceQuery) {
		t.Fatalf("Execute() error = %v", err)
	}
	if query.calls != 0 || store.calls != 0 {
		t.Fatalf("cursor request re-ran PIT or wrote snapshot: query=%d store=%d", query.calls, store.calls)
	}
}
