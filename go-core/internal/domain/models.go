package domain

import (
	"bytes"
	"encoding/json"
	"time"
)

// DailyBar is a provider-neutral daily OHLCV record. Volume is normalized to shares
// and Amount to CNY unless the provider explicitly marks another currency.
type DailyBar struct {
	Date                    string     `json:"date"`
	Open                    float64    `json:"open"`
	High                    float64    `json:"high"`
	Low                     float64    `json:"low"`
	Close                   float64    `json:"close"`
	Volume                  float64    `json:"volume"`
	Amount                  float64    `json:"amount"`
	PctChg                  float64    `json:"pct_chg"`
	MissingFields           []string   `json:"missing_fields,omitempty"`
	IsPartial               bool       `json:"is_partial,omitempty"`
	Source                  string     `json:"source,omitempty"`
	SourceVersion           string     `json:"source_version,omitempty"`
	RequestID               string     `json:"request_id,omitempty"`
	NormalizationVersion    string     `json:"normalization_version,omitempty"`
	RawVolume               *float64   `json:"raw_volume,omitempty"`
	RawVolumeUnit           string     `json:"raw_volume_unit,omitempty"`
	RawAmount               *float64   `json:"raw_amount,omitempty"`
	RawAmountUnit           string     `json:"raw_amount_unit,omitempty"`
	ProviderTimestamp       *time.Time `json:"provider_timestamp,omitempty"`
	FetchedAt               time.Time  `json:"fetched_at,omitempty"`
	NormalizedAt            time.Time  `json:"normalized_at,omitempty"`
	AdjustmentReferenceDate string     `json:"adjustment_reference_date,omitempty"`
	AdjustmentBasis         string     `json:"adjustment_basis,omitempty"`
	VolumeUnit              string     `json:"volume_unit,omitempty"`
	AmountUnit              string     `json:"amount_unit,omitempty"`
	QualityStatus           string     `json:"quality_status,omitempty"`
	QualityReasons          []string   `json:"quality_reasons,omitempty"`
	FallbackFrom            string     `json:"fallback_from,omitempty"`
	FallbackReason          string     `json:"fallback_reason,omitempty"`
}

type Quote struct {
	Code                 string     `json:"code"`
	Name                 string     `json:"name,omitempty"`
	Price                *float64   `json:"price,omitempty"`
	Change               *float64   `json:"change,omitempty"`
	ChangePct            *float64   `json:"change_pct,omitempty"`
	Open                 *float64   `json:"open,omitempty"`
	High                 *float64   `json:"high,omitempty"`
	Low                  *float64   `json:"low,omitempty"`
	PrevClose            *float64   `json:"prev_close,omitempty"`
	Volume               *float64   `json:"volume,omitempty"`
	Amount               *float64   `json:"amount,omitempty"`
	VolumeRatio          *float64   `json:"volume_ratio,omitempty"`
	TurnoverRate         *float64   `json:"turnover_rate,omitempty"`
	Source               string     `json:"source,omitempty"`
	SourceVersion        string     `json:"source_version,omitempty"`
	RequestID            string     `json:"request_id,omitempty"`
	NormalizationVersion string     `json:"normalization_version,omitempty"`
	RawVolume            *float64   `json:"raw_volume,omitempty"`
	RawVolumeUnit        string     `json:"raw_volume_unit,omitempty"`
	RawAmount            *float64   `json:"raw_amount,omitempty"`
	RawAmountUnit        string     `json:"raw_amount_unit,omitempty"`
	Timestamp            string     `json:"timestamp,omitempty"`
	QuoteTime            string     `json:"quote_time,omitempty"`
	ProviderTimestamp    *time.Time `json:"provider_timestamp,omitempty"`
	FetchedAt            time.Time  `json:"fetched_at,omitempty"`
	NormalizedAt         time.Time  `json:"normalized_at,omitempty"`
	VolumeUnit           string     `json:"volume_unit,omitempty"`
	AmountUnit           string     `json:"amount_unit,omitempty"`
	AdjustmentBasis      string     `json:"adjustment_basis,omitempty"`
	QualityStatus        string     `json:"quality_status,omitempty"`
	QualityReasons       []string   `json:"quality_reasons,omitempty"`
	FallbackFrom         string     `json:"fallback_from,omitempty"`
	FallbackReason       string     `json:"fallback_reason,omitempty"`
}

type ChipDistribution struct {
	ProfitRatio     *float64       `json:"profit_ratio,omitempty"`
	Concentration90 *float64       `json:"concentration_90,omitempty"`
	Concentration70 *float64       `json:"concentration_70,omitempty"`
	CostAverage     *float64       `json:"cost_average,omitempty"`
	CostMedian      *float64       `json:"cost_median,omitempty"`
	Raw             map[string]any `json:"raw,omitempty"`
}

type StockBundle struct {
	Code                  string            `json:"code"`
	Name                  string            `json:"name,omitempty"`
	Quote                 *Quote            `json:"quote,omitempty"`
	DailyBars             []DailyBar        `json:"daily_bars"`
	Chip                  *ChipDistribution `json:"chip,omitempty"`
	CapitalFlow           map[string]any    `json:"capital_flow,omitempty"`
	Fundamentals          map[string]any    `json:"fundamentals,omitempty"`
	Boards                []map[string]any  `json:"boards,omitempty"`
	Sources               map[string]string `json:"sources,omitempty"`
	Warnings              []string          `json:"warnings,omitempty"`
	SnapshotID            string            `json:"snapshot_id,omitempty"`
	AnalysisReferenceTime time.Time         `json:"analysis_reference_time,omitempty"`
	FetchedAt             time.Time         `json:"fetched_at"`
	QualityStatus         string            `json:"quality_status,omitempty"`
}

type MarketIndex struct {
	Code      string  `json:"code"`
	Name      string  `json:"name"`
	Current   float64 `json:"current"`
	Change    float64 `json:"change"`
	ChangePct float64 `json:"change_pct"`
	Open      float64 `json:"open,omitempty"`
	High      float64 `json:"high,omitempty"`
	Low       float64 `json:"low,omitempty"`
	PrevClose float64 `json:"prev_close,omitempty"`
	Volume    float64 `json:"volume,omitempty"`
	Amount    float64 `json:"amount,omitempty"`
	Amplitude float64 `json:"amplitude,omitempty"`
}

type MarketOverview struct {
	Region         string           `json:"region"`
	Date           string           `json:"date"`
	Indices        []MarketIndex    `json:"indices"`
	UpCount        int              `json:"up_count"`
	DownCount      int              `json:"down_count"`
	FlatCount      int              `json:"flat_count"`
	LimitUpCount   int              `json:"limit_up_count"`
	LimitDownCount int              `json:"limit_down_count"`
	TotalAmount    float64          `json:"total_amount"`
	TopSectors     []map[string]any `json:"top_sectors,omitempty"`
	BottomSectors  []map[string]any `json:"bottom_sectors,omitempty"`
	TopConcepts    []map[string]any `json:"top_concepts,omitempty"`
	BottomConcepts []map[string]any `json:"bottom_concepts,omitempty"`
	Warnings       []string         `json:"warnings,omitempty"`
}

// StockAnnouncement preserves a stock-specific disclosure record.
// AnnouncementDate is the source disclosure date; PublishedAt exists only when the source
// provides an exact publication timestamp. FetchedAt is ingestion time and is never a substitute.
type StockAnnouncement struct {
	ArticleID        string     `json:"article_id"`
	Symbol           string     `json:"symbol"`
	StockName        string     `json:"stock_name,omitempty"`
	Title            string     `json:"title"`
	Category         string     `json:"category,omitempty"`
	AnnouncementDate string     `json:"announcement_date"`
	PublishedAt      *time.Time `json:"published_at,omitempty"`
	SourceURL        string     `json:"source_url"`
	Source           string     `json:"source"`
	SourceVersion    string     `json:"source_version,omitempty"`
	FetchedAt        time.Time  `json:"fetched_at"`
	QualityStatus    string     `json:"quality_status"`
	QualityReasons   []string   `json:"quality_reasons,omitempty"`
}

// StockAnnouncementSnapshot is one validated page from the stock-announcement endpoint.
type StockAnnouncementSnapshot struct {
	Symbol         string              `json:"symbol"`
	StartDate      string              `json:"start_date"`
	EndDate        string              `json:"end_date"`
	Page           int                 `json:"page"`
	PageSize       int                 `json:"page_size"`
	Total          int                 `json:"total"`
	Pages          int                 `json:"pages"`
	Returned       int                 `json:"returned"`
	HasData        bool                `json:"has_data"`
	HasMore        bool                `json:"has_more"`
	Items          []StockAnnouncement `json:"items"`
	Source         string              `json:"source"`
	SourceVersion  string              `json:"source_version,omitempty"`
	FetchedAt      time.Time           `json:"fetched_at"`
	QualityStatus  string              `json:"quality_status"`
	QualityReasons []string            `json:"quality_reasons,omitempty"`
}

type NewsItem struct {
	Title       string  `json:"title,omitempty"`
	Summary     string  `json:"summary,omitempty"`
	URL         string  `json:"url,omitempty"`
	Source      string  `json:"source,omitempty"`
	PublishedAt string  `json:"published_at,omitempty"`
	RetrievedAt string  `json:"retrieved_at,omitempty"`
	Relevance   float64 `json:"relevance,omitempty"`
	Dimension   string  `json:"dimension,omitempty"`
}

type TechnicalSnapshot struct {
	MA5         *float64 `json:"ma5,omitempty"`
	MA10        *float64 `json:"ma10,omitempty"`
	MA20        *float64 `json:"ma20,omitempty"`
	MA60        *float64 `json:"ma60,omitempty"`
	RSI14       *float64 `json:"rsi14,omitempty"`
	MACDDIF     *float64 `json:"macd_dif,omitempty"`
	MACDDEA     *float64 `json:"macd_dea,omitempty"`
	MACDHist    *float64 `json:"macd_hist,omitempty"`
	MAAlignment string   `json:"ma_alignment,omitempty"`
	TrendStatus string   `json:"trend_status,omitempty"`
	BiasMA5Pct  *float64 `json:"bias_ma5_pct,omitempty"`
	BiasMA10Pct *float64 `json:"bias_ma10_pct,omitempty"`
	BiasMA20Pct *float64 `json:"bias_ma20_pct,omitempty"`
}

type DataQuality struct {
	Level    string            `json:"level"`
	Score    int               `json:"score"`
	Missing  []string          `json:"missing,omitempty"`
	Warnings []string          `json:"warnings,omitempty"`
	Sources  map[string]string `json:"sources,omitempty"`
}

type AnalysisContext struct {
	Code                string            `json:"code"`
	Name                string            `json:"name"`
	Market              string            `json:"market"`
	AsOf                time.Time         `json:"as_of"`
	Quote               *Quote            `json:"quote,omitempty"`
	DailyBars           []DailyBar        `json:"daily_bars"`
	Technical           TechnicalSnapshot `json:"technical"`
	Chip                *ChipDistribution `json:"chip,omitempty"`
	CapitalFlow         map[string]any    `json:"capital_flow_context,omitempty"`
	Fundamentals        map[string]any    `json:"fundamental_context,omitempty"`
	Boards              []map[string]any  `json:"boards,omitempty"`
	News                []NewsItem        `json:"news,omitempty"`
	NewsSearchPerformed bool              `json:"news_search_performed"`
	NewsResultCount     *int              `json:"news_result_count,omitempty"`
	DataQuality         DataQuality       `json:"data_quality"`
	Phase               map[string]any    `json:"phase,omitempty"`
	Warnings            []string          `json:"warnings,omitempty"`
}

type AnalysisRequest struct {
	StockCode      string    `json:"stock_code"`
	QueryID        string    `json:"query_id,omitempty"`
	ReportType     string    `json:"report_type,omitempty"`
	AsOf           time.Time `json:"as_of,omitempty"`
	LookbackDays   int       `json:"lookback_days,omitempty"`
	Notify         bool      `json:"notify,omitempty"`
	SearchNews     *bool     `json:"search_news,omitempty"`
	ReportLanguage string    `json:"report_language,omitempty"`
}

type MarketReviewRequest struct {
	Region         string    `json:"region,omitempty"`
	QueryID        string    `json:"query_id,omitempty"`
	AsOf           time.Time `json:"as_of,omitempty"`
	Notify         bool      `json:"notify,omitempty"`
	ReportLanguage string    `json:"report_language,omitempty"`
}

type StockBatchRequest struct {
	StockCodes     []string  `json:"stock_codes"`
	QueryID        string    `json:"query_id,omitempty"`
	ReportType     string    `json:"report_type,omitempty"`
	AsOf           time.Time `json:"as_of,omitempty"`
	LookbackDays   int       `json:"lookback_days,omitempty"`
	Notify         bool      `json:"notify,omitempty"`
	SearchNews     *bool     `json:"search_news,omitempty"`
	ReportLanguage string    `json:"report_language,omitempty"`
}

type AnalysisResult struct {
	Code            string         `json:"code"`
	Name            string         `json:"name,omitempty"`
	SentimentScore  int            `json:"sentiment_score,omitempty"`
	TrendPrediction string         `json:"trend_prediction,omitempty"`
	OperationAdvice string         `json:"operation_advice,omitempty"`
	DecisionType    string         `json:"decision_type,omitempty"`
	ConfidenceLevel string         `json:"confidence_level,omitempty"`
	ReportLanguage  string         `json:"report_language,omitempty"`
	Action          string         `json:"action,omitempty"`
	Dashboard       map[string]any `json:"dashboard,omitempty"`
	AnalysisSummary string         `json:"analysis_summary,omitempty"`
	NewsSummary     string         `json:"news_summary,omitempty"`
	RiskWarning     string         `json:"risk_warning,omitempty"`
	DataSources     string         `json:"data_sources,omitempty"`
	RawResponse     string         `json:"raw_response,omitempty"`
	Success         bool           `json:"success"`
	ErrorMessage    string         `json:"error_message,omitempty"`
	CurrentPrice    *float64       `json:"current_price,omitempty"`
	ChangePct       *float64       `json:"change_pct,omitempty"`
	ModelUsed       string         `json:"model_used,omitempty"`
	QueryID         string         `json:"query_id,omitempty"`
	ContextSnapshot map[string]any `json:"context_snapshot,omitempty"`
	// Extra preserves fields emitted by the existing Python AnalysisResult that
	// the Go core does not yet interpret. This is essential during migration:
	// persistence and notification must not discard legacy report fields.
	Extra map[string]json.RawMessage `json:"-"`
}

// UnmarshalJSON retains unrecognized Python result fields so round-tripping a
// result through Go does not truncate the legacy report contract.
func (r *AnalysisResult) UnmarshalJSON(data []byte) error {
	type alias AnalysisResult
	var decoded alias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*r = AnalysisResult(decoded)

	var incoming map[string]json.RawMessage
	if err := json.Unmarshal(data, &incoming); err != nil {
		return err
	}
	knownBytes, err := json.Marshal(alias(*r))
	if err != nil {
		return err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(knownBytes, &known); err != nil {
		return err
	}
	for key := range incoming {
		if _, exists := known[key]; exists {
			delete(incoming, key)
			continue
		}
	}
	if len(incoming) > 0 {
		r.Extra = incoming
	}
	return nil
}

// MarshalJSON merges preserved fields back into the serialized result while
// ensuring explicitly modeled Go fields take precedence.
func (r AnalysisResult) MarshalJSON() ([]byte, error) {
	type alias AnalysisResult
	knownBytes, err := json.Marshal(alias(r))
	if err != nil {
		return nil, err
	}
	var known map[string]json.RawMessage
	if err := json.Unmarshal(knownBytes, &known); err != nil {
		return nil, err
	}
	for key, value := range r.Extra {
		if _, exists := known[key]; !exists {
			known[key] = bytes.Clone(value)
		}
	}
	return json.Marshal(known)
}

type MarketReviewResult struct {
	Region         string         `json:"region"`
	QueryID        string         `json:"query_id"`
	Report         string         `json:"report"`
	Overview       MarketOverview `json:"overview"`
	News           []NewsItem     `json:"news,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
	SavedHistoryID int64          `json:"saved_history_id,omitempty"`
}

type StageResult struct {
	Name       string `json:"name"`
	Status     string `json:"status"`
	DurationMS int64  `json:"duration_ms"`
	Message    string `json:"message,omitempty"`
}

type WorkflowTrace struct {
	QueryID    string        `json:"query_id"`
	StartedAt  time.Time     `json:"started_at"`
	FinishedAt time.Time     `json:"finished_at"`
	Stages     []StageResult `json:"stages"`
}

type StockWorkflowResponse struct {
	Result   AnalysisResult  `json:"result"`
	Context  AnalysisContext `json:"context"`
	Trace    WorkflowTrace   `json:"trace"`
	Warnings []string        `json:"warnings,omitempty"`
}

type MarketWorkflowResponse struct {
	Result MarketReviewResult `json:"result"`
	Trace  WorkflowTrace      `json:"trace"`
}

type StockBatchFailure struct {
	StockCode string `json:"stock_code"`
	Message   string `json:"message"`
}

type StockBatchWorkflowResponse struct {
	QueryID   string                  `json:"query_id,omitempty"`
	Total     int                     `json:"total"`
	Succeeded int                     `json:"succeeded"`
	Failed    int                     `json:"failed"`
	Results   []StockWorkflowResponse `json:"results"`
	Failures  []StockBatchFailure     `json:"failures,omitempty"`
}
