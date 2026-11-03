package freshness

import (
	"testing"
	"time"
)

func TestEvaluateDurationSLA(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	policy := Policy{Domain: "quote", MaxAge: 90 * time.Second, ActivePhases: []string{"continuous_morning"}}
	tests := []struct {
		name   string
		offset time.Duration
		want   Status
		reason string
	}{
		{name: "fresh", offset: -60 * time.Second, want: StatusFresh, reason: ReasonWithinSLA},
		{name: "stale", offset: -91 * time.Second, want: StatusStale, reason: ReasonExceedsSLA},
		{name: "future source time", offset: time.Minute, want: StatusUnknown, reason: ReasonSourceTimeInFuture},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataTime := now.Add(tt.offset)
			got := Evaluate(policy, Evidence{DataTime: &dataTime, DataTimeType: "provider_time"}, Context{
				Now: now, MarketPhase: "continuous_morning",
			})
			if got.Status != tt.want || got.Reason != tt.reason {
				t.Fatalf("Evaluate() = %s/%s, want %s/%s", got.Status, got.Reason, tt.want, tt.reason)
			}
		})
	}
}

func TestEvaluateDoesNotUseFetchedAtAsDataTime(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	fetchedAt := now
	got := Evaluate(Policy{Domain: "quote", MaxAge: time.Minute},
		Evidence{FetchedAt: &fetchedAt}, Context{Now: now})
	if got.Status != StatusUnknown || got.Reason != ReasonMissingDataTime {
		t.Fatalf("unexpected result: %+v", got)
	}
}

func TestEvaluateCalendarAndMarketBoundaries(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	dataTime := now.Add(-10 * time.Minute)
	policy := Policy{
		Domain: "quote", MaxAge: time.Minute, RequireTradingCalendar: true,
		ActivePhases: []string{"continuous_morning"},
	}
	tests := []struct {
		name    string
		context Context
		status  Status
		reason  string
	}{
		{name: "calendar unknown", context: Context{Now: now}, status: StatusUnknown, reason: ReasonMissingCalendarEvidence},
		{name: "holiday", context: Context{Now: now, CalendarKnown: true}, status: StatusNotApplicable, reason: ReasonNonTradingDay},
		{name: "lunch break", context: Context{Now: now, CalendarKnown: true, IsTradingDay: true, MarketPhase: "lunch_break"}, status: StatusNotApplicable, reason: ReasonMarketNotUpdating},
		{name: "active session", context: Context{Now: now, CalendarKnown: true, IsTradingDay: true, MarketPhase: "continuous_morning"}, status: StatusStale, reason: ReasonExceedsSLA},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Evaluate(policy, Evidence{DataTime: &dataTime, DataTimeType: "provider_time"}, tt.context)
			if got.Status != tt.status || got.Reason != tt.reason {
				t.Fatalf("Evaluate() = %s/%s, want %s/%s", got.Status, got.Reason, tt.status, tt.reason)
			}
		})
	}
}

func TestEvaluateSecuritySuspensionAndUnknown(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	dataTime := now.Add(-10 * time.Minute)
	policy := Policy{Domain: "quote", MaxAge: time.Minute, IgnoreWhenSecurityPaused: true}
	tests := []struct {
		status string
		want   Status
		reason string
	}{
		{status: "suspended", want: StatusNotApplicable, reason: ReasonSecuritySuspended},
		{status: "intraday_suspension", want: StatusNotApplicable, reason: ReasonSecuritySuspended},
		{status: "unknown", want: StatusUnknown, reason: ReasonSecurityStatusUnknown},
	}
	for _, tt := range tests {
		got := Evaluate(policy, Evidence{DataTime: &dataTime, DataTimeType: "provider_time"}, Context{
			Now: now, SecurityTradingStatus: tt.status,
		})
		if got.Status != tt.want || got.Reason != tt.reason {
			t.Errorf("status %q: got %s/%s, want %s/%s", tt.status, got.Status, got.Reason, tt.want, tt.reason)
		}
	}
}

func TestEvaluateRequiresExplicitTimeSemantics(t *testing.T) {
	now := time.Now()
	got := Evaluate(Policy{Domain: "quote", MaxAge: time.Minute},
		Evidence{DataTime: &now}, Context{Now: now})
	if got.Status != StatusUnknown || got.Reason != ReasonMissingTimeSemantics {
		t.Fatalf("unexpected result: %+v", got)
	}
}
