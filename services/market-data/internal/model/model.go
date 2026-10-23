package model

import "time"

type AdjustmentBasis string

const (
	AdjustmentNone     AdjustmentBasis = "none"
	AdjustmentForward  AdjustmentBasis = "forward_adjusted"
	AdjustmentBackward AdjustmentBasis = "backward_adjusted"
	AdjustmentUnknown  AdjustmentBasis = "unknown"
)

type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityStale    QualityStatus = "stale"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

type Quote struct {
	Symbol        string   `json:"symbol"`
	Name          string   `json:"name,omitempty"`
	CurrentPrice  *float64 `json:"current_price"`
	Change        *float64 `json:"change"`
	ChangePercent *float64 `json:"change_percent"`
	Open          *float64 `json:"open"`
	High          *float64 `json:"high"`
	Low           *float64 `json:"low"`
	PrevClose     *float64 `json:"prev_close"`
	Volume        *int64   `json:"volume"`
	Amount        *float64 `json:"amount"`
	VolumeRatio   *float64 `json:"volume_ratio,omitempty"`
	TurnoverRate  *float64 `json:"turnover_rate,omitempty"`
	// QuoteTime 是供应商提供的行情快照/更新时间，不保证等于最近一笔成交时间。
	QuoteTime          *time.Time      `json:"quote_time"`
	QuoteTimeSemantics string          `json:"quote_time_semantics,omitempty"`
	FetchedAt          time.Time       `json:"fetched_at"`
	NormalizedAt       time.Time       `json:"normalized_at"`
	Source             string          `json:"source"`
	SourceVersion      string          `json:"source_version,omitempty"`
	RawVolume          *float64        `json:"raw_volume,omitempty"`
	RawVolumeUnit      string          `json:"raw_volume_unit,omitempty"`
	RawAmount          *float64        `json:"raw_amount,omitempty"`
	RawAmountUnit      string          `json:"raw_amount_unit,omitempty"`
	RawAmountField     string          `json:"raw_amount_field,omitempty"`
	VolumeUnit         string          `json:"volume_unit"`
	AmountUnit         string          `json:"amount_unit"`
	AdjustmentBasis    AdjustmentBasis `json:"adjustment_basis"`
	QualityStatus      QualityStatus   `json:"quality_status"`
	QualityReasons     []string        `json:"quality_reasons,omitempty"`
	FallbackFrom       string          `json:"fallback_from,omitempty"`
	FallbackReason     string          `json:"fallback_reason,omitempty"`
	RequestID          string          `json:"request_id,omitempty"`
	NormalizationVer   string          `json:"normalization_version"`
}

type DailyBar struct {
	Symbol            string          `json:"symbol"`
	TradeDate         string          `json:"trade_date"`
	IsPartial         bool            `json:"is_partial,omitempty"`
	Period            string          `json:"period"`
	Open              float64         `json:"open"`
	High              float64         `json:"high"`
	Low               float64         `json:"low"`
	Close             float64         `json:"close"`
	Volume            *int64          `json:"volume"`
	Amount            *float64        `json:"amount"`
	ChangePercent     *float64        `json:"change_percent"`
	AdjustmentBasis   AdjustmentBasis `json:"adjustment_basis"`
	AdjustmentMethod  string          `json:"adjustment_method,omitempty"`
	AdjustmentRefDate string          `json:"adjustment_reference_date,omitempty"`
	Source            string          `json:"source"`
	SourceVersion     string          `json:"source_version,omitempty"`
	ProviderTime      *time.Time      `json:"provider_timestamp,omitempty"`
	FetchedAt         time.Time       `json:"fetched_at"`
	NormalizedAt      time.Time       `json:"normalized_at"`
	RawVolume         *float64        `json:"raw_volume,omitempty"`
	RawVolumeUnit     string          `json:"raw_volume_unit,omitempty"`
	RawAmount         *float64        `json:"raw_amount,omitempty"`
	RawAmountUnit     string          `json:"raw_amount_unit,omitempty"`
	RawAmountField    string          `json:"raw_amount_field,omitempty"`
	VolumeUnit        string          `json:"volume_unit"`
	AmountUnit        string          `json:"amount_unit"`
	QualityStatus     QualityStatus   `json:"quality_status"`
	QualityReasons    []string        `json:"quality_reasons,omitempty"`
	FallbackFrom      string          `json:"fallback_from,omitempty"`
	FallbackReason    string          `json:"fallback_reason,omitempty"`
	RequestID         string          `json:"request_id,omitempty"`
	NormalizationVer  string          `json:"normalization_version"`
}

type Bundle struct {
	Symbol                string            `json:"symbol"`
	Name                  string            `json:"name,omitempty"`
	Quote                 *Quote            `json:"quote,omitempty"`
	DailyBars             []DailyBar        `json:"daily_bars"`
	Sources               map[string]string `json:"sources"`
	Warnings              []string          `json:"warnings,omitempty"`
	SnapshotID            string            `json:"snapshot_id"`
	AnalysisReferenceTime time.Time         `json:"analysis_reference_time"`
	FetchedAt             time.Time         `json:"fetched_at"`
	QualityStatus         QualityStatus     `json:"quality_status"`
}

// StockBoard represents one board membership returned for a security. ChangePercent uses percentage points.
type StockBoard struct {
	Code           string        `json:"code"`
	Name           string        `json:"name"`
	ChangePercent  *float64      `json:"change_percent,omitempty"`
	Source         string        `json:"source"`
	SourceVersion  string        `json:"source_version,omitempty"`
	FetchedAt      time.Time     `json:"fetched_at"`
	QualityStatus  QualityStatus `json:"quality_status"`
	QualityReasons []string      `json:"quality_reasons,omitempty"`
}

type StockBoardSnapshot struct {
	Symbol         string        `json:"symbol"`
	Total          int           `json:"total"`
	HasData        bool          `json:"has_data"`
	Items          []StockBoard  `json:"items"`
	Source         string        `json:"source"`
	SourceVersion  string        `json:"source_version,omitempty"`
	FetchedAt      time.Time     `json:"fetched_at"`
	QualityStatus  QualityStatus `json:"quality_status"`
	QualityReasons []string      `json:"quality_reasons,omitempty"`
}

// StockBasic 是低频证券身份信息。IPODate 为兼容 profile 服务保留，语义对应供应商给出的上市日期。
type StockBasic struct {
	Symbol             string        `json:"symbol"`
	Name               string        `json:"name"`
	IPODate            string        `json:"ipo_date,omitempty"`
	OutDate            string        `json:"out_date,omitempty"`
	SecurityType       int           `json:"security_type"`
	SecurityTypeLabel  string        `json:"security_type_label"`
	ListingStatus      int           `json:"listing_status"`
	ListingStatusLabel string        `json:"listing_status_label"`
	Source             string        `json:"source"`
	SourceVersion      string        `json:"source_version,omitempty"`
	FetchedAt          time.Time     `json:"fetched_at"`
	QualityStatus      QualityStatus `json:"quality_status"`
	NormalizationVer   string        `json:"normalization_version"`
}

// SectorRanking 表示行业/概念板块的即时排名。涨跌幅单位为百分点，成交额单位为人民币元。
type SectorRanking struct {
	Rank                int           `json:"rank"`
	Code                string        `json:"code"`
	Name                string        `json:"name"`
	Category            string        `json:"category"`
	LatestPrice         *float64      `json:"latest_price,omitempty"`
	Change              *float64      `json:"change,omitempty"`
	ChangePercent       *float64      `json:"change_percent,omitempty"`
	Amount              *float64      `json:"amount,omitempty"`
	AmountUnit          string        `json:"amount_unit"`
	AdvancingCount      *int          `json:"advancing_count,omitempty"`
	DecliningCount      *int          `json:"declining_count,omitempty"`
	LeadingStock        string        `json:"leading_stock,omitempty"`
	LeaderChangePercent *float64      `json:"leader_change_percent,omitempty"`
	Source              string        `json:"source"`
	ProviderReportDate  string        `json:"provider_report_date,omitempty"`
	SourceVersion       string        `json:"source_version,omitempty"`
	FetchedAt           time.Time     `json:"fetched_at"`
	QualityStatus       QualityStatus `json:"quality_status"`
	QualityReasons      []string      `json:"quality_reasons,omitempty"`
}

type SectorRankingSnapshot struct {
	Category       string          `json:"category"`
	Total          int             `json:"total"`
	Items          []SectorRanking `json:"items"`
	Source         string          `json:"source"`
	SourceVersion  string          `json:"source_version,omitempty"`
	FetchedAt      time.Time       `json:"fetched_at"`
	QualityStatus  QualityStatus   `json:"quality_status"`
	QualityReasons []string        `json:"quality_reasons,omitempty"`
}

// MarketIndexQuote is an index snapshot, not an equity quote. Index symbols use an independent allow-listed namespace.
type MarketIndexQuote struct {
	Symbol             string        `json:"symbol"`
	Code               string        `json:"code"`
	Name               string        `json:"name"`
	LatestPrice        *float64      `json:"latest_price,omitempty"`
	Change             *float64      `json:"change,omitempty"`
	ChangePercent      *float64      `json:"change_percent,omitempty"`
	Open               *float64      `json:"open,omitempty"`
	High               *float64      `json:"high,omitempty"`
	Low                *float64      `json:"low,omitempty"`
	PrevClose          *float64      `json:"prev_close,omitempty"`
	Amount             *float64      `json:"amount,omitempty"`
	AmountUnit         string        `json:"amount_unit"`
	QuoteTime          *time.Time    `json:"quote_time,omitempty"`
	QuoteTimeSemantics string        `json:"quote_time_semantics,omitempty"`
	Source             string        `json:"source"`
	SourceVersion      string        `json:"source_version,omitempty"`
	FetchedAt          time.Time     `json:"fetched_at"`
	QualityStatus      QualityStatus `json:"quality_status"`
	QualityReasons     []string      `json:"quality_reasons,omitempty"`
}

type IndexQuoteSnapshot struct {
	Items          []MarketIndexQuote `json:"items"`
	Source         string             `json:"source"`
	SourceVersion  string             `json:"source_version,omitempty"`
	FetchedAt      time.Time          `json:"fetched_at"`
	QualityStatus  QualityStatus      `json:"quality_status"`
	QualityReasons []string           `json:"quality_reasons,omitempty"`
}

type MarketBreadthSnapshot struct {
	Market              string        `json:"market"`
	TotalSecurities     int           `json:"total_securities"`
	CoveredSecurities   int           `json:"covered_securities"`
	AdvancingCount      int           `json:"advancing_count"`
	DecliningCount      int           `json:"declining_count"`
	UnchangedCount      int           `json:"unchanged_count"`
	UnclassifiedCount   int           `json:"unclassified_count"`
	TotalAmountCNY      *float64      `json:"total_amount_cny"`
	AmountCoverageCount int           `json:"amount_coverage_count"`
	Source              string        `json:"source"`
	SourceVersion       string        `json:"source_version,omitempty"`
	FetchedAt           time.Time     `json:"fetched_at"`
	QualityStatus       QualityStatus `json:"quality_status"`
	QualityReasons      []string      `json:"quality_reasons,omitempty"`
}

type LimitPoolItem struct {
	Symbol            string        `json:"symbol"`
	Code              string        `json:"code"`
	Name              string        `json:"name"`
	ChangePercent     *float64      `json:"change_percent"`
	FirstTouchTime    string        `json:"first_touch_time,omitempty"`
	LastTouchTime     string        `json:"last_touch_time,omitempty"`
	ConsecutiveBoards *int          `json:"consecutive_boards,omitempty"`
	BrokenCount       *int          `json:"broken_count,omitempty"`
	Industry          string        `json:"industry,omitempty"`
	Source            string        `json:"source"`
	SourceVersion     string        `json:"source_version,omitempty"`
	FetchedAt         time.Time     `json:"fetched_at"`
	QualityStatus     QualityStatus `json:"quality_status"`
	QualityReasons    []string      `json:"quality_reasons,omitempty"`
}

type LimitPoolSnapshot struct {
	PoolType           string          `json:"type"`
	TradeDate          string          `json:"trade_date"`
	ProviderReportDate string          `json:"provider_report_date,omitempty"`
	Total              int             `json:"total"`
	HasData            bool            `json:"has_data"`
	Items              []LimitPoolItem `json:"items"`
	Source             string          `json:"source"`
	SourceVersion      string          `json:"source_version,omitempty"`
	FetchedAt          time.Time       `json:"fetched_at"`
	QualityStatus      QualityStatus   `json:"quality_status"`
	QualityReasons     []string        `json:"quality_reasons,omitempty"`
}

type SourceStatus struct {
	Name                   string `json:"name"`
	Enabled                bool   `json:"enabled"`
	QuoteCapable           bool   `json:"quote_capable"`
	DailyBarCapable        bool   `json:"daily_bar_capable"`
	StockBasicCapable      bool   `json:"stock_basic_capable,omitempty"`
	SectorRankingCapable   bool   `json:"sector_ranking_capable,omitempty"`
	IndexQuoteCapable      bool   `json:"index_quote_capable,omitempty"`
	MarketBreadthCapable   bool   `json:"market_breadth_capable,omitempty"`
	LimitPoolCapable       bool   `json:"limit_pool_capable,omitempty"`
	StockBoardsCapable     bool   `json:"stock_boards_capable,omitempty"`
	TradingCalendarCapable bool   `json:"trading_calendar_capable,omitempty"`
	ChipCapable            bool   `json:"chip_capable,omitempty"`
	CapitalFlowCapable     bool   `json:"capital_flow_capable,omitempty"`
}

// ChipDistribution 表示妙想补充接口返回的筹码快照。上游用百分比表示的比例字段会归一化为 [0,1] 小数。
type ChipDistribution struct {
	Symbol          string        `json:"symbol"`
	Date            string        `json:"date,omitempty"`
	AverageCost     float64       `json:"average_cost"`
	SourceVersion   string        `json:"source_version,omitempty"`
	ProfitRatio     *float64      `json:"profit_ratio,omitempty"`
	Concentration90 *float64      `json:"concentration_90,omitempty"`
	Concentration70 *float64      `json:"concentration_70,omitempty"`
	Source          string        `json:"source"`
	ProviderDate    string        `json:"provider_date,omitempty"`
	FetchedAt       time.Time     `json:"fetched_at"`
	QualityStatus   QualityStatus `json:"quality_status"`
	QualityReasons  []string      `json:"quality_reasons,omitempty"`
}

// CapitalFlow 表示以人民币元计量的妙想个股资金流摘要。只有所需观测值全部可用时，才会填充 5/10 日合计字段。
type CapitalFlow struct {
	Symbol         string        `json:"symbol"`
	SourceVersion  string        `json:"source_version,omitempty"`
	MainNetInflow  *float64      `json:"main_net_inflow_yuan,omitempty"`
	Inflow5D       *float64      `json:"inflow_5d_yuan,omitempty"`
	Inflow10D      *float64      `json:"inflow_10d_yuan,omitempty"`
	Observations   int           `json:"observations"`
	Source         string        `json:"source"`
	FetchedAt      time.Time     `json:"fetched_at"`
	QualityStatus  QualityStatus `json:"quality_status"`
	QualityReasons []string      `json:"quality_reasons,omitempty"`
}
