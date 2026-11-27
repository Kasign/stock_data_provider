package storage

import (
	"testing"
	"time"
)

func TestEqualNewsDBTimestampNormalizesToMicroseconds(t *testing.T) {
	stored := time.Date(2026, 11, 27, 11, 30, 27, 144625000, time.UTC)
	inMemory := stored.Add(789 * time.Nanosecond)
	if !equalNewsDBTimestamp(stored, inMemory) {
		t.Fatal("nanoseconds below DATETIME(6) resolution must compare equal to the stored instant")
	}
	if equalNewsDBTimestamp(stored, stored.Add(time.Microsecond)) {
		t.Fatal("instants one microsecond apart must not compare equal")
	}
}

func TestEqualNewsDBTimestampComparesInstantsAcrossLocations(t *testing.T) {
	utc := time.Date(2026, 11, 27, 11, 30, 27, 144625000, time.UTC)
	offset := utc.In(time.FixedZone("UTC+8", 8*60*60))
	if !equalNewsDBTimestamp(offset, utc) {
		t.Fatal("the same instant in different locations must compare equal")
	}
}
