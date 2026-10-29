package model

import "time"

type DelistingStatus string

const (
	DelistingStatusUnknown         DelistingStatus = "unknown"
	DelistingStatusNormal          DelistingStatus = "normal"
	DelistingStatusDelistingRisk   DelistingStatus = "delisting_risk"
	DelistingStatusDelistingPeriod DelistingStatus = "delisting_period"
)

type SuspensionStatus string

const (
	SuspensionStatusUnknown       SuspensionStatus = "unknown"
	SuspensionStatusNotTradingDay SuspensionStatus = "not_trading_day"
	SuspensionStatusNoEvent       SuspensionStatus = "no_suspension_event"
	SuspensionStatusSuspended     SuspensionStatus = "suspended"
	SuspensionStatusIntraday      SuspensionStatus = "intraday_suspension"
	SuspensionStatusResumed       SuspensionStatus = "resumed"
)

// SecurityTradingStatusSnapshot records exchange-calendar, effective-name and suspension evidence
// for a specific symbol/date. Unknown fields are deliberately separate from normal status.
type SecurityTradingStatusSnapshot struct {
	Symbol                  string             `json:"symbol"`
	TradeDate               string             `json:"trade_date"`
	SecurityName            string             `json:"security_name,omitempty"`
	NameEffectiveFrom       string             `json:"name_effective_from,omitempty"`
	NameEffectiveTo         string             `json:"name_effective_to,omitempty"`
	RiskWarningStatus       RiskWarningStatus  `json:"risk_warning_status"`
	DelistingStatus          DelistingStatus     `json:"delisting_status"`
	ListingDate             string             `json:"listing_date,omitempty"`
	ListingTradingDayNumber int                `json:"listing_trading_day_number,omitempty"`
	ListingStatusKnown      bool               `json:"listing_status_known"`
	IsTradingDay            bool               `json:"is_trading_day"`
	TradingDayKnown         bool               `json:"trading_day_known"`
	SuspensionStatus        SuspensionStatus   `json:"suspension_status"`
	SuspensionTiming        string             `json:"suspension_timing,omitempty"`
	SuspensionType          string             `json:"suspension_type,omitempty"`
	Source                  string             `json:"source"`
	SourceVersion           string             `json:"source_version"`
	FetchedAt               time.Time          `json:"fetched_at"`
	QualityStatus           QualityStatus      `json:"quality_status"`
	QualityReasons          []string           `json:"quality_reasons,omitempty"`
}
