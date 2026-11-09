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
type ProviderProbeFieldIssue struct {
	Stage         string `json:"stage"`
	Field         string `json:"field"`
	ProviderField string `json:"provider_field,omitempty"`
	Code          string `json:"code"`
	Expected      string `json:"expected"`
	ActualType    string `json:"actual_type,omitempty"`
	ActualPreview string `json:"actual_preview,omitempty"`
	Severity      string `json:"severity"`
	Message       string `json:"message"`
	RowIndex      *int   `json:"row_index,omitempty"`
	Symbol        string `json:"symbol,omitempty"`
	TradeDate     string `json:"trade_date,omitempty"`
	Count         int    `json:"count,omitempty"`
}

// ProviderProbeCoverageDimension describes a field-specific coverage denominator.
type ProviderProbeCoverageDimension struct {
	Dimension  string  `json:"dimension"`
	Population string  `json:"population"`
	Source     string  `json:"source"`
	Total      int     `json:"total"`
	Returned   int     `json:"returned"`
	Validated  int     `json:"validated"`
	Missing    int     `json:"missing"`
	Excluded   int     `json:"excluded"`
	Ratio      float64 `json:"ratio"`
}

// ProviderProbeItem 是一次主动能力探测结果。错误信息经过脱敏和长度限制，不回传凭据或完整查询 URL。
type ProviderProbeItem struct {
	Provider               string                           `json:"provider"`
	Capability             string                           `json:"capability"`
	Status                 string                           `json:"status"`
	ElapsedMS              int64                            `json:"elapsed_ms"`
	SourceVersion          string                           `json:"source_version,omitempty"`
	AdjustmentBasis        AdjustmentBasis                  `json:"adjustment_basis,omitempty"`
	QualityStatus          QualityStatus                    `json:"quality_status"`
	QualityReasons         []string                         `json:"quality_reasons,omitempty"`
	ValidationTotalCount   int                              `json:"validation_total_count,omitempty"`
	ValidationErrorCount   int                              `json:"validation_error_count,omitempty"`
	ValidationWarningCount int                              `json:"validation_warning_count,omitempty"`
	ValidationInfoCount    int                              `json:"validation_info_count,omitempty"`
	ValidationTruncated    bool                             `json:"validation_truncated,omitempty"`
	ValidationIssues       []ProviderProbeFieldIssue        `json:"validation_issues,omitempty"`
	ErrorCode              string                           `json:"error_code,omitempty"`
	ErrorStage             string                           `json:"error_stage,omitempty"`
	ErrorMessage           string                           `json:"error_message,omitempty"`
	HTTPStatus             int                              `json:"http_status,omitempty"`
	SampleSymbol           string                           `json:"sample_symbol,omitempty"`
	SampleMarket           string                           `json:"sample_market,omitempty"`
	SampleDate             string                           `json:"sample_date,omitempty"`
	RowsReturned           int                              `json:"rows_returned,omitempty"`
	RowsValidated          int                              `json:"rows_validated,omitempty"`
	CoverageTotal          *int                             `json:"coverage_total,omitempty"`
	CoveragePopulation     string                           `json:"coverage_population,omitempty"`
	CoverageSource         string                           `json:"coverage_source,omitempty"`
	CoverageReturned       *int                             `json:"coverage_returned,omitempty"`
	CoverageValidated      *int                             `json:"coverage_validated,omitempty"`
	CoverageMissing        *int                             `json:"coverage_missing,omitempty"`
	CoverageExcluded       *int                             `json:"coverage_excluded,omitempty"`
	CoverageRatio          *float64                         `json:"coverage_ratio,omitempty"`
	CoverageDimensions     []ProviderProbeCoverageDimension `json:"coverage_dimensions,omitempty"`
	ProviderTime           *time.Time                       `json:"provider_time,omitempty"`
	ProviderTimeType       string                           `json:"provider_time_type,omitempty"`
	FetchedAt              *time.Time                       `json:"fetched_at,omitempty"`
	CheckedAt              time.Time                        `json:"checked_at"`
}

// ProviderProbeFieldComparison 是跨 Provider 对账的单字段证据。
// 不可比较字段不输出数值，避免把缺失值误展示为 0。
type ProviderProbeFieldComparison struct {
	Field      string   `json:"field"`
	Status     string   `json:"status"`
	Left       *float64 `json:"left,omitempty"`
	Right      *float64 `json:"right,omitempty"`
	Difference *float64 `json:"difference,omitempty"`
	Tolerance  *float64 `json:"tolerance,omitempty"`
	Reason     string   `json:"reason,omitempty"`
}

// ProviderProbeCheckResult 是从一次 Provider 响应中提取的独立诊断维度。
type ProviderProbeCheckResult struct {
	Check                  string                         `json:"check"`
	Provider               string                         `json:"provider,omitempty"`
	Capability             string                         `json:"capability,omitempty"`
	Status                 string                         `json:"status"`
	ErrorCode              string                         `json:"error_code,omitempty"`
	ErrorMessage           string                         `json:"error_message,omitempty"`
	ProviderTime           *time.Time                     `json:"provider_time,omitempty"`
	ProviderTimeType       string                         `json:"provider_time_type,omitempty"`
	FetchedAt              *time.Time                     `json:"fetched_at,omitempty"`
	CalendarKnown          *bool                          `json:"calendar_known,omitempty"`
	IsTradingDay           *bool                          `json:"is_trading_day,omitempty"`
	SecurityTradingStatus  string                         `json:"security_trading_status,omitempty"`
	AgeSeconds             *int64                         `json:"age_seconds,omitempty"`
	FreshnessStatus        string                         `json:"freshness_status,omitempty"`
	FreshnessReason        string                         `json:"freshness_reason,omitempty"`
	FreshnessDomain        string                         `json:"freshness_domain,omitempty"`
	FreshnessMaxAgeSeconds *int64                         `json:"freshness_max_age_seconds,omitempty"`
	MarketPhase            string                         `json:"market_phase,omitempty"`
	ComparedSymbol         string                         `json:"compared_symbol,omitempty"`
	LeftProvider           string                         `json:"left_provider,omitempty"`
	RightProvider          string                         `json:"right_provider,omitempty"`
	LeftSourceVersion      string                         `json:"left_source_version,omitempty"`
	RightSourceVersion     string                         `json:"right_source_version,omitempty"`
	AdjustmentBasis        string                         `json:"adjustment_basis,omitempty"`
	LeftAdjustmentMethod   string                         `json:"left_adjustment_method,omitempty"`
	RightAdjustmentMethod  string                         `json:"right_adjustment_method,omitempty"`
	LeftAdjustmentRefDate  string                         `json:"left_adjustment_reference_date,omitempty"`
	RightAdjustmentRefDate string                         `json:"right_adjustment_reference_date,omitempty"`
	LeftQualityStatus      QualityStatus                  `json:"left_quality_status,omitempty"`
	RightQualityStatus     QualityStatus                  `json:"right_quality_status,omitempty"`
	ComparedDates          []string                       `json:"compared_dates,omitempty"`
	FieldCount             int                            `json:"field_count,omitempty"`
	MismatchCount          int                            `json:"mismatch_count,omitempty"`
	MatchedFieldCount      int                            `json:"matched_field_count,omitempty"`
	NotComparableCount     int                            `json:"not_comparable_count,omitempty"`
	FieldComparisons       []ProviderProbeFieldComparison `json:"field_comparisons,omitempty"`
	CheckedAt              time.Time                      `json:"checked_at"`
}

// ProviderProbeRun 是隔离于业务流量的主动探测批次。
type ProviderProbeRun struct {
	RunID                        string                     `json:"run_id"`
	Scope                        string                     `json:"scope,omitempty"`
	RequestedCapabilities        []string                   `json:"requested_capabilities,omitempty"`
	RequestedChecks              []string                   `json:"requested_checks,omitempty"`
	CheckResults                 []ProviderProbeCheckResult `json:"check_results,omitempty"`
	RequestedMarkets             []string                   `json:"requested_markets,omitempty"`
	RequestedSymbols             []string                   `json:"requested_symbols,omitempty"`
	RequestedDailyBarAdjustments []AdjustmentBasis          `json:"requested_daily_bar_adjustments,omitempty"`
	RequestedDailyBarStartDate   string                     `json:"requested_daily_bar_start_date,omitempty"`
	RequestedDailyBarEndDate     string                     `json:"requested_daily_bar_end_date,omitempty"`
	TimeoutSeconds               int                        `json:"timeout_seconds,omitempty"`
	Status                       string                     `json:"status"`
	Persisted                    bool                       `json:"persisted"`
	StartedAt                    time.Time                  `json:"started_at"`
	FinishedAt                   time.Time                  `json:"finished_at"`
	Total                        int                        `json:"total"`
	Returned                     int                        `json:"returned"`
	Degraded                     int                        `json:"degraded"`
	Unknown                      int                        `json:"unknown"`
	Failed                       int                        `json:"failed"`
	Skipped                      int                        `json:"skipped"`
	NotConfigured                int                        `json:"not_configured"`
	NotSupported                 int                        `json:"not_supported"`
	Items                        []ProviderProbeItem        `json:"items"`
}

// ProviderProbeHistory 是最近的主动探测批次列表。
type ProviderProbeHistory struct {
	Items []ProviderProbeRun `json:"items"`
	Total int                `json:"total"`
}


// ProviderProbeOptions 描述一次 Provider 能力探测请求。
// 字段来源均为调用方显式请求；空切片/空字符串按 DataService 现有默认规则解释。
type ProviderProbeOptions struct {
	Scope               string            // 探测范围；合法值由 Provider 探测服务校验。
	Capabilities        []string          // 指定能力名称；为空时使用服务端默认能力集合。
	Checks              []string          // 指定检查项；为空时使用服务端默认检查集合。
	Markets             []string          // 限定市场；为空时不额外限制市场。
	Symbols             []string          // 指定样本证券代码；为空时使用服务端样本。
	DailyBarAdjustments []AdjustmentBasis // 日线探测使用的复权口径。
	DailyBarStartDate   string            // 日线探测窗口起始交易日，包含边界，YYYY-MM-DD。
	DailyBarEndDate     string            // 日线探测窗口结束交易日，包含边界，YYYY-MM-DD。
	TimeoutSeconds      int               // 单次探测超时秒数；有效范围由服务端校验和限制。
}
