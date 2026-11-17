package time

import "testing"

func TestNewMariaDBSourceRejectsNilDatabase(t *testing.T) {
	if _, err := NewMariaDBSource(nil); err == nil {
		t.Fatal("nil database was accepted")
	}
}

func TestMariaDBClockQueryUsesUTCWithMicroseconds(t *testing.T) {
	if MariaDBClockQuery != "SELECT DATE_FORMAT(UTC_TIMESTAMP(6), '%Y-%m-%dT%H:%i:%s.%f000Z')" {
		t.Fatalf("unexpected MariaDB clock query: %q", MariaDBClockQuery)
	}
}
