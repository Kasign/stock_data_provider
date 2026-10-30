package model

import "time"

// ProviderProbeCapabilities 返回诊断接口支持的能力标识。
func ProviderProbeCapabilities() []string {
	return []string{"quote", "daily_bars", "stock_basic", "stock_search", "stock_universe", "index_quotes", "index_catalog", "market_breadth", "sector_rankings", "market_capital_flow", "limit_pool", "dragon_tiger", "announcements", "trading_calendar", "security_trading_status", "price_limit_event", "stock_boards", "fundamentals", "valuations", "hot_ranking", "capital_flow", "chip_distribution"}
}

// ProviderProbeCoreCapabilities 返回核心行情和证券基础能力集合。
func ProviderProbeCoreCapabilities() []string {
	return []string{"quote", "daily_bars", "stock_basic", "trading_calendar", "index_quotes", "market_breadth"}
}

// ProviderProbeItem 是一次主动能力探测结果。错误信息经过脱敏和长度限制，不回传凭据或完整查询 URL。
type ProviderProbeItem struct {
	Provider          string        `json:"provider"`
	Capability        string        `json:"capability"`
	Status            string        `json:"status"`
	ElapsedMS         int64         `json:"elapsed_ms"`
	SourceVersion     string        `json:"source_version,omitempty"`
	QualityStatus     QualityStatus `json:"quality_status"`
	QualityReasons    []string      `json:"quality_reasons,omitempty"`
	ErrorCode         string        `json:"error_code,omitempty"`
	ErrorStage        string        `json:"error_stage,omitempty"`
	ErrorMessage      string        `json:"error_message,omitempty"`
	HTTPStatus        int           `json:"http_status,omitempty"`
	SampleSymbol      string        `json:"sample_symbol,omitempty"`
	SampleMarket      string        `json:"sample_market,omitempty"`
	SampleDate        string        `json:"sample_date,omitempty"`
	RowsReturned      int           `json:"rows_returned,omitempty"`
	RowsValidated     int           `json:"rows_validated,omitempty"`
	CoverageTotal     *int          `json:"coverage_total,omitempty"`
	CoverageReturned  *int          `json:"coverage_returned,omitempty"`
	CoverageValidated *int          `json:"coverage_validated,omitempty"`
	CoverageMissing   *int          `json:"coverage_missing,omitempty"`
	CoverageExcluded  *int          `json:"coverage_excluded,omitempty"`
	CoverageRatio     *float64      `json:"coverage_ratio,omitempty"`
	ProviderTime      *time.Time    `json:"provider_time,omitempty"`
	CheckedAt         time.Time     `json:"checked_at"`
}

// ProviderProbeRun 是隔离于业务流量的主动探测批次。
type ProviderProbeRun struct {
	RunID                 string              `json:"run_id"`
	Scope                 string              `json:"scope,omitempty"`
	RequestedCapabilities []string            `json:"requested_capabilities,omitempty"`
	RequestedMarkets      []string            `json:"requested_markets,omitempty"`
	RequestedSymbols      []string            `json:"requested_symbols,omitempty"`
	TimeoutSeconds        int                 `json:"timeout_seconds,omitempty"`
	Status                string              `json:"status"`
	Persisted             bool                `json:"persisted"`
	StartedAt             time.Time           `json:"started_at"`
	FinishedAt            time.Time           `json:"finished_at"`
	Total                 int                 `json:"total"`
	Returned              int                 `json:"returned"`
	Degraded              int                 `json:"degraded"`
	Unknown               int                 `json:"unknown"`
	Failed                int                 `json:"failed"`
	Skipped               int                 `json:"skipped"`
	NotConfigured         int                 `json:"not_configured"`
	NotSupported          int                 `json:"not_supported"`
	Items                 []ProviderProbeItem `json:"items"`
}

// ProviderProbeHistory 是最近的主动探测批次列表。
type ProviderProbeHistory struct {
	Items []ProviderProbeRun `json:"items"`
	Total int                `json:"total"`
}
