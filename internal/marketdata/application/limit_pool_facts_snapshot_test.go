package application

import (
	"testing"
	"time"
)

func TestLimitPoolTradeDateClosedRequiresPostCloseSystemAsOf(t *testing.T) {
	// 15:00 Asia/Shanghai == 07:00 UTC.
	if limitPoolTradeDateClosed("2026-11-27", time.Date(2026, 11, 27, 6, 59, 59, 0, time.UTC)) {
		t.Fatal("system_as_of before the close must not be accepted")
	}
	if !limitPoolTradeDateClosed("2026-11-27", time.Date(2026, 11, 27, 7, 0, 0, 0, time.UTC)) {
		t.Fatal("system_as_of at the close must be accepted")
	}
	if !limitPoolTradeDateClosed("2026-11-27", time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("system_as_of after the close must be accepted")
	}
	if limitPoolTradeDateClosed("not-a-date", time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)) {
		t.Fatal("invalid trade_date must not be treated as closed")
	}
}

func TestNewLimitPoolFactsSnapshotCreatorRequiresDependencies(t *testing.T) {
	if _, err := NewLimitPoolFactsSnapshotCreator(nil, nil, nil); err == nil {
		t.Fatal("nil dependencies must be rejected")
	}
}
