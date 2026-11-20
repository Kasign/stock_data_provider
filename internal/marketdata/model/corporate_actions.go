package model

import "time"

// AdjustmentFactorBasis 区分来源提供的累计因子口径。
type AdjustmentFactorBasis string

const (
	AdjustmentFactorShared   AdjustmentFactorBasis = "shared"
	AdjustmentFactorForward  AdjustmentFactorBasis = "forward"
	AdjustmentFactorBackward AdjustmentFactorBasis = "backward"
)

// NormalizeAdjustmentFactorBasis 将未显式标注的单列因子按 shared 口径处理。
func NormalizeAdjustmentFactorBasis(value AdjustmentFactorBasis) AdjustmentFactorBasis {
	if value == "" {
		return AdjustmentFactorShared
	}
	return value
}

// AdjustmentFactor 是某证券某交易日、某复权口径的价格仿射变换观测。
// 价格变换为 adjusted_price = Factor * raw_price + PriceOffsetCNY。
// PriceOffsetCNY=nil 表示来源只提供标量乘法因子，偏移量按 0 处理。
// SystemObservedAt 由本系统写入历史版本时生成，不由 Provider 提供。
type AdjustmentFactor struct {
	Symbol           string                `json:"symbol"`
	TradeDate        string                `json:"trade_date"`
	Factor           float64               `json:"factor"`
	PriceOffsetCNY   *float64              `json:"price_offset_cny,omitempty"`
	Basis            AdjustmentFactorBasis `json:"basis"`
	Source           string                `json:"source"`
	SourceVersion    string                `json:"source_version,omitempty"`
	ProviderTime     *time.Time            `json:"provider_time,omitempty"`
	AvailableAt      *time.Time            `json:"available_at,omitempty"`
	FetchedAt        time.Time             `json:"fetched_at"`
	SystemObservedAt *time.Time            `json:"system_observed_at,omitempty"`
	QualityStatus    QualityStatus         `json:"quality_status"`
	QualityReasons   []string              `json:"quality_reasons,omitempty"`
}

// CorporateActionHolderScope 表示公司行动实际适用的股东范围。
type CorporateActionHolderScope string

const (
	CorporateActionHolderScopeUnknown                 CorporateActionHolderScope = "unknown"
	CorporateActionHolderScopeAllShareholders         CorporateActionHolderScope = "all_shareholders"
	CorporateActionHolderScopeTradableShareholders    CorporateActionHolderScope = "tradable_shareholders"
	CorporateActionHolderScopeNonTradableShareholders CorporateActionHolderScope = "non_tradable_shareholders"
)

// CorporateActionAdjustmentPolicy 区分价格连续性调整与持有人总回报调整。
type CorporateActionAdjustmentPolicy string

const (
	CorporateActionAdjustmentPolicyExchangeReference         CorporateActionAdjustmentPolicy = "exchange_reference"
	CorporateActionAdjustmentPolicyTradableHolderTotalReturn CorporateActionAdjustmentPolicy = "tradable_holder_total_return"
	CorporateActionAdjustmentPolicyProviderCompatibility     CorporateActionAdjustmentPolicy = "provider_compatibility"
)

// CorporateAction 是公司行动事件。金额单位为人民币元/股，比例为每股变动数量。
// 股东对价与公司分红分开保存；未知字段保持 nil，不得用零代替未知。
type CorporateAction struct {
	Symbol                         string                      `json:"symbol"`
	ProviderEventID                string                      `json:"provider_event_id"`
	EventType                      string                      `json:"event_type"`
	EventStatus                    string                      `json:"event_status,omitempty"`
	AnnouncementDate               string                      `json:"announcement_date,omitempty"`
	ImplementationAnnouncementDate string                      `json:"implementation_announcement_date,omitempty"`
	AvailableAt                    *time.Time                  `json:"available_at,omitempty"`
	RecordDate                     string                      `json:"record_date,omitempty"`
	ExDate                         string                      `json:"ex_date"`
	PayDate                        string                      `json:"pay_date,omitempty"`
	CashDividendCNY                *float64                    `json:"cash_dividend_after_tax_cny_per_share,omitempty"`
	CashDividendBeforeTaxCNY       *float64                    `json:"cash_dividend_before_tax_cny_per_share,omitempty"`
	BonusShareRatio                *float64                    `json:"bonus_share_ratio,omitempty"`
	TransferShareRatio             *float64                    `json:"transfer_share_ratio,omitempty"`
	RightsIssueRatio               *float64                    `json:"rights_issue_ratio,omitempty"`
	RightsIssuePriceCNY            *float64                    `json:"rights_issue_price_cny,omitempty"`
	CashConsiderationCNY           *float64                    `json:"cash_consideration_cny_per_share,omitempty"`
	ShareConsiderationRatio        *float64                    `json:"share_consideration_ratio,omitempty"`
	HolderScope                    CorporateActionHolderScope `json:"holder_scope,omitempty"`
	ReferencePriceIncluded         *bool                       `json:"reference_price_included,omitempty"`
	Source                         string                      `json:"source"`
	SourceVersion                  string                      `json:"source_version,omitempty"`
	FetchedAt                      time.Time                   `json:"fetched_at"`
	SystemObservedAt               *time.Time                  `json:"system_observed_at,omitempty"`
	QualityStatus                  QualityStatus               `json:"quality_status"`
	QualityReasons                 []string                    `json:"quality_reasons,omitempty"`
}
