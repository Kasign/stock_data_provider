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
	Symbol                 string        `json:"symbol"`
	Name                   string        `json:"name"`
	IPODate                string        `json:"ipo_date,omitempty"`
	OutDate                string        `json:"out_date,omitempty"`
	SecurityType           int           `json:"security_type"`
	SecurityTypeLabel      string        `json:"security_type_label"`
	ListingStatus          int           `json:"listing_status"`
	ListingStatusLabel     string        `json:"listing_status_label"`
	DynamicPE              *float64      `json:"pe_dynamic,omitempty"`
	PB                     *float64      `json:"pb,omitempty"`
	Industry               string        `json:"industry,omitempty"`
	MarketCapCNY           *float64      `json:"market_cap_cny,omitempty"`
	FloatMarketCapCNY      *float64      `json:"float_market_cap_cny,omitempty"`
	BoardCode              string        `json:"board_code,omitempty"`
	ROEPercent             *float64      `json:"roe_percent,omitempty"`
	NetProfitMarginPercent *float64      `json:"net_profit_margin_percent,omitempty"`
	NetProfitCNY           *float64      `json:"net_profit_cny,omitempty"`
	GrossMarginPercent     *float64      `json:"gross_margin_percent,omitempty"`
	Source                 string        `json:"source"`
	SourceVersion          string        `json:"source_version,omitempty"`
	FetchedAt              time.Time     `json:"fetched_at"`
	QualityStatus          QualityStatus `json:"quality_status"`
	QualityReasons         []string      `json:"quality_reasons,omitempty"`
	NormalizationVer       string        `json:"normalization_version"`
}

// StockSearchItem 是证券目录搜索结果。它只描述证券身份，不包含实时行情价格。
type StockSearchItem struct {
	Symbol   string `json:"symbol"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Exchange string `json:"exchange"`
}

// StockSearchSnapshot 是基于真实 Provider 证券目录进行的名称/代码查询结果。
// FetchedAt 表示本服务获取目录的时间，不表示实时行情时间。
type StockSearchSnapshot struct {
	Query          string            `json:"query"`
	Total          int               `json:"total"`
	Items          []StockSearchItem `json:"items"`
	Source         string            `json:"source"`
	SourceVersion  string            `json:"source_version,omitempty"`
	FetchedAt      time.Time         `json:"fetched_at"`
	QualityStatus  QualityStatus     `json:"quality_status"`
	QualityReasons []string          `json:"quality_reasons,omitempty"`
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
	LimitUpCount        *int          `json:"limit_up_count"`
	LimitDownCount      *int          `json:"limit_down_count"`
	TotalAmountCNY      *float64      `json:"total_amount_cny"`
	AmountCoverageCount int           `json:"amount_coverage_count"`
	Source              string        `json:"source"`
	SourceVersion       string        `json:"source_version,omitempty"`
	ProviderReportDate  string        `json:"provider_report_date,omitempty"`
	FetchedAt           time.Time     `json:"fetched_at"`
	QualityStatus       QualityStatus `json:"quality_status"`
	QualityReasons      []string      `json:"quality_reasons,omitempty"`
}

type LimitPoolItem struct {
	Symbol               string        `json:"symbol"`
	Code                 string        `json:"code"`
	Name                 string        `json:"name"`
	LastPrice            *float64      `json:"last_price,omitempty"`
	ChangePercent        *float64      `json:"change_percent"`
	FirstTouchTime       string        `json:"first_touch_time,omitempty"`
	LastTouchTime        string        `json:"last_touch_time,omitempty"`
	ConsecutiveBoards    *int          `json:"consecutive_boards,omitempty"`
	BrokenCount          *int          `json:"broken_count,omitempty"`
	OpenTimes            *int          `json:"open_times,omitempty"`
	TurnoverRatioPercent *float64      `json:"turnover_ratio_percent,omitempty"`
	Turnover             *float64      `json:"turnover,omitempty"`
	SealMoney            *float64      `json:"seal_money,omitempty"`
	MaxSealMoney         *float64      `json:"max_seal_money,omitempty"`
	ProviderAmountUnit   string        `json:"provider_amount_unit,omitempty"`
	Industry             string        `json:"industry,omitempty"`
	Source               string        `json:"source"`
	SourceVersion        string        `json:"source_version,omitempty"`
	FetchedAt            time.Time     `json:"fetched_at"`
	QualityStatus        QualityStatus `json:"quality_status"`
	QualityReasons       []string      `json:"quality_reasons,omitempty"`
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

type FundamentalMetric struct {
	Key         string  `json:"key"`
	Name        string  `json:"name"`
	Value       float64 `json:"value"`
	Unit        string  `json:"unit"`
	SourceField string  `json:"source_field"`
}

type FundamentalSection struct {
	Category        string              `json:"category"`
	ReportDate      string              `json:"report_date,omitempty"`
	PublicationDate string              `json:"publication_date,omitempty"`
	HasData         bool                `json:"has_data"`
	Metrics         []FundamentalMetric `json:"metrics"`
	QualityStatus   QualityStatus       `json:"quality_status"`
	QualityReasons  []string            `json:"quality_reasons,omitempty"`
}

type FundamentalsSnapshot struct {
	Symbol         string               `json:"symbol"`
	Year           int                  `json:"year"`
	Quarter        int                  `json:"quarter"`
	HasData        bool                 `json:"has_data"`
	Sections       []FundamentalSection `json:"sections"`
	Source         string               `json:"source"`
	SourceVersion  string               `json:"source_version,omitempty"`
	FetchedAt      time.Time            `json:"fetched_at"`
	QualityStatus  QualityStatus        `json:"quality_status"`
	QualityReasons []string             `json:"quality_reasons,omitempty"`
}

// ValuationItem 为单只 A 股的最新估值快照。指标允许为 null 或负数，不将缺失值补零。
type ValuationItem struct {
	Symbol         string        `json:"symbol"`
	Code           string        `json:"code"`
	Name           string        `json:"name,omitempty"`
	PETTM          *float64      `json:"pe_ttm"`
	PEMRQ          *float64      `json:"pe_mrq"`
	PEStatic       *float64      `json:"pe_static"`
	PBMRQ          *float64      `json:"pb_mrq"`
	PSTTM          *float64      `json:"ps_ttm"`
	PCFTTM         *float64      `json:"pcf_ttm"`
	Source         string        `json:"source"`
	SourceVersion  string        `json:"source_version,omitempty"`
	FetchedAt      time.Time     `json:"fetched_at"`
	QualityStatus  QualityStatus `json:"quality_status"`
	QualityReasons []string      `json:"quality_reasons,omitempty"`
}

// ValuationSnapshot 是一次批量估值查询。ProviderTimestamp 是供应商返回的五项估值中最新有效时间，
// 不代表每个指标都在该时刻更新。MissingSymbols 记录未从供应商响应中返回的请求代码。
type ValuationSnapshot struct {
	RequestedSymbols []string        `json:"requested_symbols"`
	Total            int             `json:"total"`
	Items            []ValuationItem `json:"items"`
	MissingSymbols   []string        `json:"missing_symbols,omitempty"`
	// ProviderReportDate 是供应商给出的估值数据日期；日期不等于精确快照时间。
	ProviderReportDate string        `json:"provider_report_date,omitempty"`
	ProviderTimestamp  *time.Time    `json:"provider_timestamp,omitempty"`
	Source             string        `json:"source"`
	SourceVersion      string        `json:"source_version,omitempty"`
	FetchedAt          time.Time     `json:"fetched_at"`
	QualityStatus      QualityStatus `json:"quality_status"`
	QualityReasons     []string      `json:"quality_reasons,omitempty"`
}

// IndexCatalogItem 是一个同花顺指数或板块的目录项。
type IndexCatalogItem struct {
	Symbol string `json:"symbol"`
	Name   string `json:"name"`
	Tag    string `json:"tag"`
}

// IndexCatalogSnapshot 是某个目录类别的完整快照。上游没有分页，因此目录请求会限制最大记录数。
type IndexCatalogSnapshot struct {
	Tag            string             `json:"tag"`
	Items          []IndexCatalogItem `json:"items"`
	Total          int                `json:"total"`
	ProviderTime   *time.Time         `json:"provider_time,omitempty"`
	Source         string             `json:"source"`
	SourceVersion  string             `json:"source_version,omitempty"`
	FetchedAt      time.Time          `json:"fetched_at"`
	QualityStatus  QualityStatus      `json:"quality_status"`
	QualityReasons []string           `json:"quality_reasons,omitempty"`
}

// IndexConstituent 是指数/板块当前包含的一只 A 股，不代表历史成分变更记录。
type IndexConstituent struct {
	Symbol        string   `json:"symbol"`
	Code          string   `json:"code"`
	Name          string   `json:"name"`
	WeightPercent *float64 `json:"weight_percent,omitempty"`
}

// IndexConstituentSnapshot 为指定指数或板块的当前成分股快照。
type IndexConstituentSnapshot struct {
	IndexSymbol        string             `json:"index_symbol"`
	ProviderReportDate string             `json:"provider_report_date,omitempty"`
	Items              []IndexConstituent `json:"items"`
	Total              int                `json:"total"`
	// ProviderReturned 表示所选来源在成分规范化/名称补全前返回的候选行数。
	ProviderReturned int `json:"provider_returned"`
	// ExcludedCount 记录来源已返回、但因重复或缺少可核验名称而未进入 Items 的行数。
	ExcludedCount int `json:"excluded_count,omitempty"`
	ProviderTime       *time.Time         `json:"provider_time,omitempty"`
	Source             string             `json:"source"`
	SourceVersion      string             `json:"source_version,omitempty"`
	FetchedAt          time.Time          `json:"fetched_at"`
	QualityStatus      QualityStatus      `json:"quality_status"`
	QualityReasons     []string           `json:"quality_reasons,omitempty"`
}

// IndexDailyBar 是指数/板块原始日线，不带复权字段，也不会参与个股 DailyBar 的调整逻辑。
type IndexDailyBar struct {
	IndexSymbol    string        `json:"index_symbol"`
	TradeDate      string        `json:"trade_date"`
	Open           float64       `json:"open"`
	High           float64       `json:"high"`
	Low            float64       `json:"low"`
	Close          float64       `json:"close"`
	ChangePercent  *float64      `json:"change_percent,omitempty"`
	Volume         *int64        `json:"volume,omitempty"`
	Amount         *float64      `json:"amount,omitempty"`
	VolumeUnit     string        `json:"volume_unit"`
	AmountUnit     string        `json:"amount_unit"`
	Source         string        `json:"source"`
	SourceVersion  string        `json:"source_version,omitempty"`
	ProviderTime   *time.Time    `json:"provider_time,omitempty"`
	FetchedAt      time.Time     `json:"fetched_at"`
	QualityStatus  QualityStatus `json:"quality_status"`
	QualityReasons []string      `json:"quality_reasons,omitempty"`
}

// IndexDailyBarsSnapshot 是单个指数/板块在指定日期范围的无复权日线快照。
type IndexDailyBarsSnapshot struct {
	IndexSymbol    string          `json:"index_symbol"`
	StartDate      string          `json:"start_date"`
	EndDate        string          `json:"end_date"`
	Items          []IndexDailyBar `json:"items"`
	Total          int             `json:"total"`
	ProviderTime   *time.Time      `json:"provider_time,omitempty"`
	Source         string          `json:"source"`
	SourceVersion  string          `json:"source_version,omitempty"`
	FetchedAt      time.Time       `json:"fetched_at"`
	QualityStatus  QualityStatus   `json:"quality_status"`
	QualityReasons []string        `json:"quality_reasons,omitempty"`
}

// HotRankingItem 是东方财富个股人气榜或飙升榜中的一只股票。
type HotRankingItem struct {
	Rank           int           `json:"rank"`
	Symbol         string        `json:"symbol"`
	Code           string        `json:"code"`
	Name           string        `json:"name,omitempty"`
	LatestPrice    *float64      `json:"latest_price,omitempty"`
	Change         *float64      `json:"change,omitempty"`
	ChangePercent  *float64      `json:"change_percent,omitempty"`
	RankChange     *int          `json:"rank_change,omitempty"`
	Heat           *float64      `json:"heat,omitempty"`
	RankTrend      string        `json:"rank_trend,omitempty"`
	Source         string        `json:"source"`
	SourceVersion  string        `json:"source_version,omitempty"`
	FetchedAt      time.Time     `json:"fetched_at"`
	QualityStatus  QualityStatus `json:"quality_status"`
	QualityReasons []string      `json:"quality_reasons,omitempty"`
}

// HotRankingSnapshot 表示人气榜、飙升榜或关注榜的有界排名快照。ProviderTime 为可选的数据准备时间，不等同于精确行情时点；延迟榜单保持 degraded。
type HotRankingSnapshot struct {
	RankingType    string           `json:"type"`
	Total          int              `json:"total"`
	Items          []HotRankingItem `json:"items"`
	ProviderTime   *time.Time       `json:"provider_time,omitempty"`
	Source         string           `json:"source"`
	SourceVersion  string           `json:"source_version,omitempty"`
	FetchedAt      time.Time        `json:"fetched_at"`
	QualityStatus  QualityStatus    `json:"quality_status"`
	QualityReasons []string         `json:"quality_reasons,omitempty"`
}

type SourceStatus struct {
	Name                   string `json:"name"`
	Enabled                bool   `json:"enabled"`
	QuoteCapable           bool   `json:"quote_capable"`
	DailyBarCapable        bool   `json:"daily_bar_capable"`
	StockBasicCapable      bool   `json:"stock_basic_capable,omitempty"`
	StockSearchCapable     bool   `json:"stock_search_capable,omitempty"`
	SectorRankingCapable   bool   `json:"sector_ranking_capable,omitempty"`
	IndexQuoteCapable      bool   `json:"index_quote_capable,omitempty"`
	MarketBreadthCapable   bool   `json:"market_breadth_capable,omitempty"`
	LimitPoolCapable       bool   `json:"limit_pool_capable,omitempty"`
	StockBoardsCapable     bool   `json:"stock_boards_capable,omitempty"`
	DragonTigerCapable     bool   `json:"dragon_tiger_capable,omitempty"`
	AnnouncementsCapable   bool   `json:"announcements_capable,omitempty"`
	TradingCalendarCapable bool   `json:"trading_calendar_capable,omitempty"`
	FundamentalsCapable    bool   `json:"fundamentals_capable,omitempty"`
	ValuationCapable       bool   `json:"valuation_capable,omitempty"`
	IndexCatalogCapable    bool   `json:"index_catalog_capable,omitempty"`
	ChipCapable            bool   `json:"chip_capable,omitempty"`
	HotRankingCapable      bool   `json:"hot_ranking_capable,omitempty"`
	CapitalFlowCapable     bool   `json:"capital_flow_capable,omitempty"`
}

// ChipDistribution 表示妙想补充接口返回的筹码快照。上游用百分比表示的比例字段会归一化为 [0,1] 小数。
type ChipDistribution struct {
	Symbol          string        `json:"symbol"`
	Date            string        `json:"date,omitempty"`
	AverageCost     float64       `json:"average_cost"`
	Cost90Low       *float64      `json:"cost_90_low,omitempty"`
	Cost90High      *float64      `json:"cost_90_high,omitempty"`
	Cost70Low       *float64      `json:"cost_70_low,omitempty"`
	Cost70High      *float64      `json:"cost_70_high,omitempty"`
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
