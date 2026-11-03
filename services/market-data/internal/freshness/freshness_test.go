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


// TestEvaluateSpecialTradingScenarios keeps quote freshness behavior explicit at
// exchange-session boundaries and for security-level trading events.
func TestEvaluateSpecialTradingScenarios(t *testing.T) {
	now := time.Date(2026, 11, 3, 10, 0, 0, 0, time.FixedZone("CST", 8*3600))
	policy := Policy{
		Domain: "quote", MaxAge: 90 * time.Second, MaxFutureSkew: 2 * time.Minute,
		ActivePhases: []string{"opening_auction", "continuous_morning", "continuous_afternoon", "closing_auction"},
		RequireTradingCalendar: true, IgnoreWhenSecurityPaused: true,
	}
	tests := []struct {
		name       string
		offset     time.Duration
		context    Context
		want       Status
		wantReason string
	}{
		{
			name: "opening auction uses quote SLA", offset: -30 * time.Second,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "opening_auction", SecurityTradingStatus: "normal"},
			want: StatusFresh, wantReason: ReasonWithinSLA,
		},
		{
			name: "auction pause does not age quote", offset: -10 * time.Minute,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "auction_pause", SecurityTradingStatus: "normal"},
			want: StatusNotApplicable, wantReason: ReasonMarketNotUpdating,
		},
		{
			name: "lunch break does not age quote", offset: -10 * time.Minute,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "lunch_break", SecurityTradingStatus: "normal"},
			want: StatusNotApplicable, wantReason: ReasonMarketNotUpdating,
		},
		{
			name: "non trading day", offset: -24 * time.Hour,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: false,
				MarketPhase: "non_trading_day", SecurityTradingStatus: "normal"},
			want: StatusNotApplicable, wantReason: ReasonNonTradingDay,
		},
		{
			name: "full day suspension", offset: -24 * time.Hour,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "continuous_morning", SecurityTradingStatus: "suspended"},
			want: StatusNotApplicable, wantReason: ReasonSecuritySuspended,
		},
		{
			name: "intraday suspension", offset: -10 * time.Minute,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "continuous_morning", SecurityTradingStatus: "intraday_suspension"},
			want: StatusNotApplicable, wantReason: ReasonSecuritySuspended,
		},
		{
			name: "resumed security follows ordinary quote SLA", offset: -30 * time.Second,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "continuous_morning", SecurityTradingStatus: "normal"},
			want: StatusFresh, wantReason: ReasonWithinSLA,
		},
		{
			name: "new listing follows ordinary quote SLA without fabricated grace period", offset: -30 * time.Second,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "continuous_morning", SecurityTradingStatus: "normal"},
			want: StatusFresh, wantReason: ReasonWithinSLA,
		},
		{
			name: "source time within allowed future skew", offset: time.Minute,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "continuous_morning", SecurityTradingStatus: "normal"},
			want: StatusFresh, wantReason: ReasonWithinSLA,
		},
		{
			name: "source time beyond allowed future skew", offset: 3 * time.Minute,
			context: Context{Now: now, CalendarKnown: true, IsTradingDay: true,
				MarketPhase: "continuous_morning", SecurityTradingStatus: "normal"},
			want: StatusUnknown, wantReason: ReasonSourceTimeInFuture,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dataTime := now.Add(tc.offset)
			got := Evaluate(policy, Evidence{DataTime: &dataTime, DataTimeType: "provider_snapshot_time"}, tc.context)
			if got.Status != tc.want || got.Reason != tc.wantReason {
				t.Fatalf("Evaluate() = %s/%s, want %s/%s", got.Status, got.Reason, tc.want, tc.wantReason)
			}
		})
	}
}
