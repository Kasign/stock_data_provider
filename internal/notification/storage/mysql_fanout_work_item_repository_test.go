package storage

import (
	"errors"
	"testing"

	"example.com/stock-analysis-flow/internal/notification/application"
)

type fanoutRowsAffectedErrorResult struct{}

func (fanoutRowsAffectedErrorResult) LastInsertId() (int64, error) { return 0, nil }

func (fanoutRowsAffectedErrorResult) RowsAffected() (int64, error) {
	return 0, errors.New("rows affected unavailable")
}

func TestRequireFanoutLeaseUpdateMarksRowsAffectedErrorAsUnknown(t *testing.T) {
	err := requireFanoutLeaseUpdate(fanoutRowsAffectedErrorResult{})
	if !errors.Is(err, application.ErrPersistenceOutcomeUnknown) {
		t.Fatalf("expected persistence outcome unknown, got %v", err)
	}
}
