package market

import (
	"time"
)

// Phase describes clock/session windows only. It intentionally does not claim an
// exchange holiday calendar; the caller must retain the latest known daily bar date
// and mark the calendar status as unknown until an authoritative calendar is wired.
func PhaseContext(marketCode string, asOf time.Time) map[string]any {
	if asOf.IsZero() {
		asOf = time.Now()
	}
	if marketCode != "cn" {
		return map[string]any{
			"market":                  marketCode,
			"phase":                   "unknown",
			"reference_time":          asOf.Format(time.RFC3339),
			"trading_calendar_status": "not_checked",
			"warning":                 "Market-specific time zone, session hours, holidays and special sessions are not implemented in this migration slice.",
		}
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err == nil {
		asOf = asOf.In(loc)
	}
	weekday := asOf.Weekday()
	nonTradingWeekend := weekday == time.Saturday || weekday == time.Sunday
	clock := asOf.Hour()*60 + asOf.Minute()
	phase := "after_hours"
	switch {
	case nonTradingWeekend:
		phase = "weekend"
	case clock < 9*60+15:
		phase = "pre_market"
	case clock < 9*60+30:
		phase = "opening_auction"
	case clock < 11*60+30:
		phase = "morning_session"
	case clock < 13*60:
		phase = "lunch_break"
	case clock < 15*60:
		phase = "afternoon_session"
	default:
		phase = "after_hours"
	}
	return map[string]any{
		"market":                  marketCode,
		"phase":                   phase,
		"reference_time":          asOf.Format(time.RFC3339),
		"timezone":                "Asia/Shanghai",
		"weekend":                 nonTradingWeekend,
		"is_open_by_clock":        phase == "morning_session" || phase == "afternoon_session",
		"trading_calendar_status": "not_checked",
		"warning":                 "Session is inferred from clock time only; exchange holidays and special sessions are not checked.",
	}
}
