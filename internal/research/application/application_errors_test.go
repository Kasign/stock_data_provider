package application

import (
	"context"
	"errors"
	"fmt"
	"testing"

	researcherrors "example.com/stock-analysis-flow/internal/research"
	"example.com/stock-analysis-flow/internal/platform/fault"
)

func TestMapRepositoryErrorPreservesStableClassification(t *testing.T) {
	cases := []struct {
		name string
		cause error
		class fault.Class
		code string
	}{
		{name: "not found", cause: fmt.Errorf("adapter: %w", researcherrors.ErrNotFound), class: fault.ClassNotFound, code: "research_record_not_found"},
		{name: "conflict", cause: fmt.Errorf("adapter: %w", researcherrors.ErrConflict), class: fault.ClassConflict, code: "research_record_conflict"},
		{name: "corrupt", cause: fmt.Errorf("adapter: %w", researcherrors.ErrCorruptRecord), class: fault.ClassInternal, code: "research_record_corrupt"},
		{name: "deadline", cause: fmt.Errorf("adapter: %w", context.DeadlineExceeded), class: fault.ClassTimeout, code: "research_request_timeout"},
		{name: "unknown", cause: errors.New("database password must not leak"), class: fault.ClassInternal, code: "research_operation_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := mapRepositoryError(tc.cause).(*fault.Fault)
			if !ok { t.Fatalf("expected *fault.Fault, got %T", mapRepositoryError(tc.cause)) }
			if got.Class != tc.class || got.Code != tc.code {
				t.Fatalf("unexpected mapping: class=%q code=%q", got.Class, got.Code)
			}
			if got.Cause == nil { t.Fatal("original adapter error must remain available as internal Cause") }
			if got.Error() == "" || got.Error() == tc.cause.Error() {
				t.Fatalf("public error text must use a safe summary, got %q", got.Error())
			}
			if tc.name == "unknown" && got.Error() == "database password must not leak" {
				t.Fatal("adapter details leaked through Fault.Error")
			}
		})
	}
}

func TestMapRepositoryErrorCancellationIsNotReportedAsDeadline(t *testing.T) {
	got, ok := mapRepositoryError(context.Canceled).(*fault.Fault)
	if !ok { t.Fatalf("expected *fault.Fault, got %T", mapRepositoryError(context.Canceled)) }
	if got.Code != "research_request_canceled" || got.Class != fault.ClassTimeout {
		t.Fatalf("cancellation mapping changed unexpectedly: class=%q code=%q", got.Class, got.Code)
	}
}
