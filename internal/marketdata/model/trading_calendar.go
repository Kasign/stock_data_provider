package model

import "time"

// TradingCalendarDay represents one explicit calendar date. Non-trading days
// remain in the response so missing rows cannot be misread as holidays.
type TradingCalendarDay struct {
	Date         string `json:"date"`
	IsTradingDay bool   `json:"is_trading_day"`
}

type TradingCalendarSnapshot struct {
	Market         string               `json:"market"`
	StartDate      string               `json:"start_date"`
	EndDate        string               `json:"end_date"`
	TotalDays      int                  `json:"total_days"`
	TradingDays    int                  `json:"trading_days"`
	Items          []TradingCalendarDay `json:"items"`
	Source         string               `json:"source"`
	SourceVersion  string               `json:"source_version,omitempty"`
	FetchedAt      time.Time            `json:"fetched_at"`
	QualityStatus  QualityStatus        `json:"quality_status"`
	QualityReasons []string             `json:"quality_reasons,omitempty"`
}
