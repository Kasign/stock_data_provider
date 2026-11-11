package storage

import (
	"context"
	"testing"
)

func TestLoadDeadlineFinalizationSnapshotValidatesRepositoryAndReference(t *testing.T) {
	var repository *MySQLRunRepository
	if _, err := repository.LoadDeadlineFinalizationSnapshot(context.Background(), "run-1"); err == nil {
		t.Fatal("expected nil repository to be rejected")
	}
	repository = &MySQLRunRepository{}
	if _, err := repository.LoadDeadlineFinalizationSnapshot(context.Background(), "run-1"); err == nil {
		t.Fatal("expected missing database to be rejected")
	}
	if _, err := repository.LoadDeadlineFinalizationSnapshot(context.Background(), " "); err == nil {
		t.Fatal("expected empty Run reference to be rejected")
	}
}
