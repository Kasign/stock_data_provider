package model

import "time"

// StockUniverseSnapshot 是通过完整证券目录分页构建的 A 股证券池，不包含行情字段。
type StockUniverseSnapshot struct {
	Items []StockSearchItem `json:"items"`
	Total int `json:"total"`
	TotalIsLowerBound bool `json:"total_is_lower_bound"`
	Source string `json:"source"`
	SourceVersion string `json:"source_version"`
	FetchedAt time.Time `json:"fetched_at"`
	QualityStatus QualityStatus `json:"quality_status"`
	QualityReasons []string `json:"quality_reasons,omitempty"`
}

// MarketStockOverviewItem 是一只 A 股的行情与均线概览。
type MarketStockOverviewItem struct {
	Symbol string `json:"symbol"`
	Code string `json:"code"`
	Name string `json:"name"`
	Exchange string `json:"exchange"`
	Quote *Quote `json:"quote,omitempty"`
	MA5 *float64 `json:"ma5"`
	MA20 *float64 `json:"ma20"`
	MAAdjustmentBasis AdjustmentBasis `json:"ma_adjustment_basis"`
	MAProvider string `json:"ma_provider,omitempty"`
	ErrorCode string `json:"error_code,omitempty"`
	QualityStatus QualityStatus `json:"quality_status"`
	QualityReasons []string `json:"quality_reasons,omitempty"`
}

// MarketStocksSnapshot 是分页市场概览。Total 描述证券目录规模；QuoteSucceeded/Failed 只针对当前页。
type MarketStocksSnapshot struct {
	Page int `json:"page"`
	PageSize int `json:"page_size"`
	Total int `json:"total"`
	TotalIsLowerBound bool `json:"total_is_lower_bound"`
	Returned int `json:"returned"`
	QuoteSucceeded int `json:"quote_succeeded"`
	QuoteFailed int `json:"quote_failed"`
	CoverageRatio float64 `json:"coverage_ratio"`
	IncludeMA bool `json:"include_ma"`
	MAAdjustmentBasis AdjustmentBasis `json:"ma_adjustment_basis"`
	UniverseSource string `json:"universe_source"`
	UniverseSourceVersion string `json:"universe_source_version"`
	UniverseFetchedAt time.Time `json:"universe_fetched_at"`
	FetchedAt time.Time `json:"fetched_at"`
	Items []MarketStockOverviewItem `json:"items"`
	QualityStatus QualityStatus `json:"quality_status"`
	QualityReasons []string `json:"quality_reasons,omitempty"`
}
