package storage

import (
	"context"
	"testing"
)

func TestExpireRunDeadlineValidatesRepositoryAndArguments(t *testing.T) {
	var repository *MySQLRunRepository
	if _, _, err := repository.ExpireRunDeadline(context.Background(), "run-1", 1); err == nil {
		t.Fatal("expected nil repository to be rejected")
	}
	repository = &MySQLRunRepository{}
	if _, _, err := repository.ExpireRunDeadline(context.Background(), "run-1", 1); err == nil {
		t.Fatal("expected missing database to be rejected")
	}
}
