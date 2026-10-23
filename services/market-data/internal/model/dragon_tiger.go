package model

import "time"

// DragonTigerItem 表示东方财富龙虎榜的一条盘后记录。金额为人民币元，比例为百分点；缺失金额保留为 nil。
type DragonTigerItem struct {
	Symbol           string        `json:"symbol"`
	Code             string        `json:"code"`
	Name             string        `json:"name"`
	TradeDate        string        `json:"trade_date"`
	ChangePercent    *float64      `json:"change_percent,omitempty"`
	ClosePrice       *float64      `json:"close_price_cny,omitempty"`
	NetBuyAmountCNY  *float64      `json:"net_buy_amount_cny,omitempty"`
	BuyAmountCNY     *float64      `json:"buy_amount_cny,omitempty"`
	SellAmountCNY    *float64      `json:"sell_amount_cny,omitempty"`
	DealAmountCNY    *float64      `json:"deal_amount_cny,omitempty"`
	DealAmountRatio  *float64      `json:"deal_amount_ratio,omitempty"`
	Reason           string        `json:"reason,omitempty"`
	Explanation      string        `json:"explanation,omitempty"`
	SecurityTypeCode string        `json:"security_type_code,omitempty"`
	Source           string        `json:"source"`
	SourceVersion    string        `json:"source_version"`
	FetchedAt        time.Time     `json:"fetched_at"`
	QualityStatus    QualityStatus `json:"quality_status"`
	QualityReasons   []string      `json:"quality_reasons,omitempty"`
}

// DragonTigerSnapshot 表示指定交易日的龙虎榜分页。Total/Pages 是供应商计数，Items 仅对应请求页。
type DragonTigerSnapshot struct {
	TradeDate          string            `json:"trade_date"`
	ProviderReportDate string            `json:"provider_report_date,omitempty"`
	Page               int               `json:"page"`
	PageSize           int               `json:"page_size"`
	Total              int               `json:"total"`
	Pages              int               `json:"pages"`
	Returned           int               `json:"returned"`
	HasData            bool              `json:"has_data"`
	HasMore            bool              `json:"has_more"`
	Items              []DragonTigerItem `json:"items"`
	Source             string            `json:"source"`
	SourceVersion      string            `json:"source_version"`
	FetchedAt          time.Time         `json:"fetched_at"`
	QualityStatus      QualityStatus     `json:"quality_status"`
	QualityReasons     []string           `json:"quality_reasons,omitempty"`
}
