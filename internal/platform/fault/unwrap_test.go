package fault

import (
	"errors"
	"fmt"
	"testing"
)

func TestFaultUnwrapPreservesCause(t *testing.T) {
	cause := errors.New("cause marker")
	value := New(ClassUnavailable, "storage.mariadb.unavailable", "database unavailable", cause)
	wrapped := fmt.Errorf("operation failed: %w", value)
	if !errors.Is(wrapped, cause) {
		t.Fatal("Fault must preserve errors.Is behavior through Cause")
	}
	var got *Fault
	if !errors.As(wrapped, &got) || got != value {
		t.Fatalf("Fault must preserve errors.As behavior: got=%p want=%p", got, value)
	}
}
