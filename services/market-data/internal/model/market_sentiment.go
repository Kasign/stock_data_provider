package model

import "time"

// MarketSentimentSnapshot 是单个交易日的市场情绪观测。所有计数均保留覆盖范围和来源质量，不把缺失计数当作零。
type MarketSentimentSnapshot struct {
	Market string `json:"market"`
	TradeDate string `json:"trade_date"`
	ProviderReportDate string `json:"provider_report_date,omitempty"`
	TotalSecurities int `json:"total_securities"`
	CoveredSecurities int `json:"covered_securities"`
	AdvancingCount int `json:"advancing_count"`
	DecliningCount int `json:"declining_count"`
	UnchangedCount int `json:"unchanged_count"`
	UnclassifiedCount int `json:"unclassified_count"`
	LimitUpCount *int `json:"limit_up_count"`
	LimitDownCount *int `json:"limit_down_count"`
	BrokenBoardCount *int `json:"broken_board_count"`
	LimitUpCountIsLowerBound bool `json:"limit_up_count_is_lower_bound,omitempty"`
	LimitDownCountIsLowerBound bool `json:"limit_down_count_is_lower_bound,omitempty"`
	BrokenBoardCountIsLowerBound bool `json:"broken_board_count_is_lower_bound,omitempty"`
	PoolSources map[string]string `json:"pool_sources,omitempty"`
	PoolSourceVersions map[string]string `json:"pool_source_versions,omitempty"`
	Source string `json:"source"`
	SourceVersion string `json:"source_version,omitempty"`
	FetchedAt time.Time `json:"fetched_at"`
	QualityStatus QualityStatus `json:"quality_status"`
	QualityReasons []string `json:"quality_reasons,omitempty"`
}

// MarketSentimentHistory 是按交易日返回的历史情绪快照。
type MarketSentimentHistory struct {
	Market string `json:"market"`
	StartDate string `json:"start_date"`
	EndDate string `json:"end_date"`
	Items []MarketSentimentSnapshot `json:"items"`
	Total int `json:"total"`
}
