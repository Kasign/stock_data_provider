package fault

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestFaultJSONDoesNotExposeCause(t *testing.T) {
	f := New(ClassUnavailable, "data.provider.unavailable", "provider unavailable", errors.New("secret token in upstream error"))
	data, err := json.Marshal(f)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) == "" || contains(string(data), "secret token") {
		t.Fatalf("cause leaked in JSON: %s", data)
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
