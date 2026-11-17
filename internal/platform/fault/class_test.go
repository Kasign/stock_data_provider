package fault

import "testing"

func TestClassValidMatchesPLATCTR001Contract(t *testing.T) {
	valid := []Class{
		ClassValidation,
		ClassUnauthenticated,
		ClassForbidden,
		ClassNotFound,
		ClassConflict,
		ClassUnavailable,
		ClassTimeout,
		ClassInternal,
	}
	for _, class := range valid {
		if !class.Valid() {
			t.Errorf("class %q must be valid", class)
		}
	}
	for _, class := range []Class{"", "configuration", "dependency", "data_quality", "custom"} {
		if class.Valid() {
			t.Errorf("class %q must not be valid", class)
		}
	}
}
