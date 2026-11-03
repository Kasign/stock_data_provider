package model

import "time"

// AdjustmentFactor 是某证券某交易日的源端累计复权因子观测。
// SystemObservedAt 由本系统写入历史版本时生成，不由 Provider 提供。
type AdjustmentFactor struct {
	Symbol           string        `json:"symbol"`
	TradeDate        string        `json:"trade_date"`
	Factor           float64       `json:"factor"`
	Source           string        `json:"source"`
	SourceVersion    string        `json:"source_version,omitempty"`
	ProviderTime     *time.Time    `json:"provider_time,omitempty"`
	AvailableAt      *time.Time    `json:"available_at,omitempty"`
	FetchedAt        time.Time     `json:"fetched_at"`
	SystemObservedAt *time.Time    `json:"system_observed_at,omitempty"`
	QualityStatus    QualityStatus `json:"quality_status"`
	QualityReasons   []string      `json:"quality_reasons,omitempty"`
}

// CorporateAction 是公司行动事件。金额单位为人民币元/股，比例为每股变动数量。
// 未由 Provider 明确返回的字段保持 nil，不得用零代替未知。
type CorporateAction struct {
	Symbol                         string        `json:"symbol"`
	ProviderEventID                string        `json:"provider_event_id"`
	EventType                      string        `json:"event_type"`
	EventStatus                    string        `json:"event_status,omitempty"`
	AnnouncementDate               string        `json:"announcement_date,omitempty"`
	ImplementationAnnouncementDate string        `json:"implementation_announcement_date,omitempty"`
	AvailableAt                    *time.Time    `json:"available_at,omitempty"`
	RecordDate                     string        `json:"record_date,omitempty"`
	ExDate                         string        `json:"ex_date"`
	PayDate                        string        `json:"pay_date,omitempty"`
	CashDividendCNY                *float64      `json:"cash_dividend_after_tax_cny_per_share,omitempty"`
	CashDividendBeforeTaxCNY       *float64      `json:"cash_dividend_before_tax_cny_per_share,omitempty"`
	BonusShareRatio                *float64      `json:"bonus_share_ratio,omitempty"`
	TransferShareRatio             *float64      `json:"transfer_share_ratio,omitempty"`
	RightsIssueRatio               *float64      `json:"rights_issue_ratio,omitempty"`
	RightsIssuePriceCNY            *float64      `json:"rights_issue_price_cny,omitempty"`
	Source                         string        `json:"source"`
	SourceVersion                  string        `json:"source_version,omitempty"`
	FetchedAt                      time.Time     `json:"fetched_at"`
	SystemObservedAt               *time.Time    `json:"system_observed_at,omitempty"`
	QualityStatus                  QualityStatus `json:"quality_status"`
	QualityReasons                 []string      `json:"quality_reasons,omitempty"`
}
