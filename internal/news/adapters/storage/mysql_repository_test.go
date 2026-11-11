package storage

import (
	"strings"
	"testing"
)

func TestNewMySQLRepositoryRequiresBorrowedDatabase(t *testing.T) {
	if _, err := NewMySQLRepository(nil); err == nil {
		t.Fatal("NewMySQLRepository(nil) error = nil, want validation error")
	}
}

func TestIdentityAdvisoryLockNameIsBoundedAndDoesNotExposeSourceID(t *testing.T) {
	sourceID := "sensitive-source-id"
	name := identityAdvisoryLockName(sourceID)
	if len(name) > 64 {
		t.Fatalf("advisory lock name length = %d, exceeds MariaDB limit", len(name))
	}
	if strings.Contains(name, sourceID) {
		t.Fatal("advisory lock name exposes source ID")
	}
	if name != identityAdvisoryLockName(sourceID) {
		t.Fatal("advisory lock name is not deterministic")
	}
	if name == identityAdvisoryLockName("another-source") {
		t.Fatal("different source IDs share the same advisory lock name")
	}
}

func TestNullableStringUsesSQLNullForEmptyReferences(t *testing.T) {
	if nullableString("") != nil {
		t.Fatal("empty optional reference must map to SQL NULL")
	}
	if got := nullableString("version-1"); got != "version-1" {
		t.Fatalf("nullableString() = %#v", got)
	}
}
