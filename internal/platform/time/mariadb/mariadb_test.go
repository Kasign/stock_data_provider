package mariadb

import "testing"

func TestNewSourceRejectsNilDatabase(t *testing.T) {
	if _, err := NewSource(nil); err == nil {
		t.Fatal("nil database was accepted")
	}
}

func TestClockQueryUsesUTCWithMicroseconds(t *testing.T) {
	if ClockQuery != "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')" {
		t.Fatalf("unexpected MariaDB clock query: %q", ClockQuery)
	}
}
