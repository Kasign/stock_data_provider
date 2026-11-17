package market

import (
	"testing"
	"time"
)

func TestPhaseContextDoesNotApplyShanghaiSessionToForeignMarkets(t *testing.T) {
	got := PhaseContext("us", time.Date(2026, 10, 20, 10, 0, 0, 0, time.UTC))
	if got["phase"] != "unknown" || got["trading_calendar_status"] != "not_checked" {
		t.Fatalf("foreign market session must remain unknown until implemented: %#v", got)
	}
}

func TestPhaseContextLabelsChinaClockPhase(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Skipf("timezone database unavailable: %v", err)
	}
	got := PhaseContext("cn", time.Date(2026, 10, 20, 10, 0, 0, 0, loc))
	if got["phase"] != "morning_session" {
		t.Fatalf("want morning_session, got %#v", got)
	}
}
