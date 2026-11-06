package fault

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFaultJSONDoesNotExposeInternalFields(t *testing.T) {
	f := New(ClassUnavailable, "data.provider.unavailable", "provider unavailable", errors.New("secret token in upstream error"))
	f.Dependency = "provider.internal-host"
	f.Retryable = true

	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("internal fault fields must not be serialized: %s", data)
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
