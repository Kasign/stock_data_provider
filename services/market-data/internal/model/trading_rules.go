package model

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
