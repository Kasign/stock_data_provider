package fault

import (
	"fmt"
	"testing"
)

func TestFaultUnwrapReturnsCause(t *testing.T) {
	cause := fmt.Errorf("cause marker")
	value := &Fault{Cause: cause}
	if got := value.Unwrap(); got != cause {
		t.Fatalf("Unwrap() = %v, want original cause %v", got, cause)
	}
	if got := (*Fault)(nil).Unwrap(); got != nil {
		t.Fatalf("nil Fault Unwrap() = %v, want nil", got)
	}
}
