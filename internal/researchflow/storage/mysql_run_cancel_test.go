package storage

import (
	"context"
	"errors"
	"testing"
)

func TestBeginRunCancellationValidatesRepositoryAndArguments(t *testing.T) {
	repository := &MySQLRunRepository{}
	if _, err := repository.BeginRunCancellation(context.Background(), "run-1", 1); err == nil {
		t.Fatal("expected missing database to be rejected")
	}
	repository = &MySQLRunRepository{db: nil}
	if _, err := repository.BeginRunCancellation(context.Background(), "", 1); err == nil {
		t.Fatal("expected missing database to be rejected before argument validation")
	}
	_ = errors.Is
}
