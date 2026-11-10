package freshness

import (
	"strings"
	"time"
)

// Status is independent from the data quality status. A fresh result never upgrades
// degraded or unknown data quality.
type Status string

const (
	StatusFresh         Status = "fresh"
	StatusStale         Status = "stale"
	StatusUnknown       Status = "unknown"
	StatusNotApplicable Status = "not_applicable"
)

// Reason codes are stable machine-readable explanations for a freshness decision.
const (
	ReasonWithinSLA               = "within_sla"
	ReasonExceedsSLA              = "exceeds_sla"
	ReasonMissingDataTime         = "source_time_missing"
	ReasonMissingTimeSemantics    = "source_time_semantics_missing"
	ReasonMissingEvaluationTime   = "evaluation_time_missing"
	ReasonMissingCalendarEvidence = "trading_calendar_evidence_missing"
	ReasonNonTradingDay           = "non_trading_day"
	ReasonMarketNotUpdating       = "market_not_updating"
	ReasonSecuritySuspended       = "security_suspended"
	ReasonSecurityStatusUnknown   = "security_trading_status_unknown"
	ReasonSourceTimeInFuture      = "source_time_in_future"
	ReasonInvalidPolicy           = "freshness_policy_invalid"
)

// Policy defines the expected update cadence for a data domain.
// MaxAge is evaluated only during ActivePhases. An empty ActivePhases list means
// every market phase is eligible for age evaluation.
type Policy struct {
	Domain                   string
	MaxAge                   time.Duration
	MaxFutureSkew            time.Duration
	ActivePhases             []string
	RequireTradingCalendar   bool
	IgnoreWhenSecurityPaused bool
}

// Evidence distinguishes source data time from service collection time.
// FetchedAt is returned for diagnostics only and is never used as the data timestamp.
type Evidence struct {
	DataTime     *time.Time
	DataTimeType string
	FetchedAt    *time.Time
}

// Context is supplied by the caller; Evaluate does not read clocks or services.
type Context struct {
	Now                   time.Time
	MarketPhase           string
	CalendarKnown         bool
	IsTradingDay          bool
	SecurityTradingStatus string
}

// Result is a deterministic freshness decision with the evidence used.
type Result struct {
	Status       Status        `json:"freshness_status"`
	Reason       string        `json:"freshness_reason"`
	Domain       string        `json:"freshness_domain,omitempty"`
	DataTime     *time.Time    `json:"data_time,omitempty"`
	DataTimeType string        `json:"data_time_type,omitempty"`
	FetchedAt    *time.Time    `json:"fetched_at,omitempty"`
	CheckedAt    time.Time     `json:"checked_at,omitempty"`
	Age          time.Duration `json:"age,omitempty"`
	MaxAge       time.Duration `json:"max_age,omitempty"`
	MarketPhase  string        `json:"market_phase,omitempty"`
}

// Evaluate computes freshness from explicit evidence and context. It is pure:
// callers must resolve calendar/session/security context before invoking it.
func Evaluate(policy Policy, evidence Evidence, context Context) Result {
	result := Result{
		Status:       StatusUnknown,
		Domain:       strings.TrimSpace(policy.Domain),
		DataTime:     evidence.DataTime,
		DataTimeType: strings.TrimSpace(evidence.DataTimeType),
		FetchedAt:    evidence.FetchedAt,
		CheckedAt:    context.Now,
		MaxAge:       policy.MaxAge,
		MarketPhase:  strings.TrimSpace(context.MarketPhase),
	}

	if policy.MaxAge <= 0 || strings.TrimSpace(policy.Domain) == "" {
		result.Reason = ReasonInvalidPolicy
		return result
	}
	if context.Now.IsZero() {
		result.Reason = ReasonMissingEvaluationTime
		return result
	}
	if evidence.DataTime == nil || evidence.DataTime.IsZero() {
		result.Reason = ReasonMissingDataTime
		return result
	}
	if result.DataTimeType == "" {
		result.Reason = ReasonMissingTimeSemantics
		return result
	}
	if policy.RequireTradingCalendar && !context.CalendarKnown {
		result.Reason = ReasonMissingCalendarEvidence
		return result
	}
	if policy.RequireTradingCalendar && !context.IsTradingDay {
		result.Status, result.Reason = StatusNotApplicable, ReasonNonTradingDay
		return result
	}
	if policy.IgnoreWhenSecurityPaused {
		switch context.SecurityTradingStatus {
		case "suspended", "intraday_suspension":
			result.Status, result.Reason = StatusNotApplicable, ReasonSecuritySuspended
			return result
		case "", "unknown":
			result.Reason = ReasonSecurityStatusUnknown
			return result
		}
	}
	if len(policy.ActivePhases) > 0 && !contains(policy.ActivePhases, result.MarketPhase) {
		result.Status, result.Reason = StatusNotApplicable, ReasonMarketNotUpdating
		return result
	}

	age := context.Now.Sub(*evidence.DataTime)
	if age < -policy.MaxFutureSkew {
		result.Reason = ReasonSourceTimeInFuture
		return result
	}
	if age < 0 {
		age = 0
	}
	result.Age = age
	if age <= policy.MaxAge {
		result.Status, result.Reason = StatusFresh, ReasonWithinSLA
		return result
	}
	result.Status, result.Reason = StatusStale, ReasonExceedsSLA
	return result
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
