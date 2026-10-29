package model

import "time"

// RiskWarningStatus must come from a source-backed security status effective on TradeDate.
// Unknown is not equivalent to Normal.
type RiskWarningStatus string

const (
	RiskWarningUnknown RiskWarningStatus = "unknown"
	RiskWarningNormal  RiskWarningStatus = "normal"
	RiskWarningApplied RiskWarningStatus = "risk_warning"
)

// PriceLimitException is an explicitly verified exchange/listing event.
// None means the source confirmed no special no-limit event; Unknown must not be guessed as None.
type PriceLimitException string

const (
	PriceLimitExceptionUnknown         PriceLimitException = "unknown"
	PriceLimitExceptionNone            PriceLimitException = "none"
	PriceLimitExceptionRelistingFirst  PriceLimitException = "relisting_first_day"
	PriceLimitExceptionDelistingFirst  PriceLimitException = "delisting_first_day"
	PriceLimitExceptionExchangeSpecial PriceLimitException = "exchange_specified"
)

type PriceLimitStatus string

const (
	PriceLimitStatusUnknown PriceLimitStatus = "unknown"
	PriceLimitStatusLimited PriceLimitStatus = "limited"
	PriceLimitStatusNoLimit PriceLimitStatus = "no_limit"
)

// PriceLimitRuleSnapshot is a point-in-time interpretation of an exchange rule.
// It is not evidence that the supplied risk-warning/listing/exception inputs are true; callers
// must source those event states independently before using a result to gate analysis.
type PriceLimitRuleSnapshot struct {
	Symbol                  string              `json:"symbol"`
	Market                  string              `json:"market"`
	Board                   string              `json:"board"`
	TradeDate               string              `json:"trade_date"`
	RiskWarningStatus       RiskWarningStatus   `json:"risk_warning_status"`
	ListingTradingDayNumber int                 `json:"listing_trading_day_number,omitempty"`
	ListingStatusKnown      bool                `json:"listing_status_known"`
	SpecialExceptionStatus  PriceLimitException `json:"special_exception_status"`
	SpecialExceptionEvidence *PriceLimitEventEvidence `json:"special_exception_evidence,omitempty"`
	Status                  PriceLimitStatus    `json:"status"`
	LimitPercent            *float64            `json:"limit_percent,omitempty"`
	NoLimitReason           string              `json:"no_limit_reason,omitempty"`
	RuleVersion             string              `json:"rule_version"`
	RuleEffectiveFrom       string              `json:"rule_effective_from,omitempty"`
	RuleEffectiveTo         string              `json:"rule_effective_to,omitempty"`
	RuleSource              string              `json:"rule_source"`
	RuleSourceURL           string              `json:"rule_source_url"`
	QualityStatus           QualityStatus       `json:"quality_status"`
	QualityReasons          []string            `json:"quality_reasons,omitempty"`
}

 
// PriceLimitEventEvidence records the source-backed evidence used to resolve a special
// no-limit event. A missing event is only confirmed when the source guarantees coverage.
type PriceLimitEventEvidence struct {
	Symbol            string              `json:"symbol"`
	Status            PriceLimitException `json:"status"`
	EffectiveFrom     string              `json:"effective_from"`
	EffectiveTo       string              `json:"effective_to,omitempty"`
	Source            string              `json:"source"`
	SourceURL         string              `json:"source_url"`
	AnnouncementID    string              `json:"announcement_id,omitempty"`
	ObservedAt        time.Time           `json:"observed_at"`
	CoverageConfirmed bool                `json:"coverage_confirmed"`
}
