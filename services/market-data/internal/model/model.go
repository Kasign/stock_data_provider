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
	Symbol           string          `json:"symbol"`
	Name             string          `json:"name,omitempty"`
	CurrentPrice     *float64        `json:"current_price"`
	Change           *float64        `json:"change"`
	ChangePercent    *float64        `json:"change_percent"`
	Open             *float64        `json:"open"`
	High             *float64        `json:"high"`
	Low              *float64        `json:"low"`
	PrevClose        *float64        `json:"prev_close"`
	Volume           *int64          `json:"volume"`
	Amount           *float64        `json:"amount"`
	VolumeRatio      *float64        `json:"volume_ratio,omitempty"`
	TurnoverRate     *float64        `json:"turnover_rate,omitempty"`
	QuoteTime        *time.Time      `json:"quote_time"`
	FetchedAt        time.Time       `json:"fetched_at"`
	NormalizedAt     time.Time       `json:"normalized_at"`
	Source           string          `json:"source"`
	SourceVersion    string          `json:"source_version,omitempty"`
	RawVolume        *float64        `json:"raw_volume,omitempty"`
	RawVolumeUnit    string          `json:"raw_volume_unit,omitempty"`
	RawAmount        *float64        `json:"raw_amount,omitempty"`
	RawAmountUnit    string          `json:"raw_amount_unit,omitempty"`
	RawAmountField   string          `json:"raw_amount_field,omitempty"`
	VolumeUnit       string          `json:"volume_unit"`
	AmountUnit       string          `json:"amount_unit"`
	AdjustmentBasis  AdjustmentBasis `json:"adjustment_basis"`
	QualityStatus    QualityStatus   `json:"quality_status"`
	QualityReasons   []string        `json:"quality_reasons,omitempty"`
	FallbackFrom     string          `json:"fallback_from,omitempty"`
	FallbackReason   string          `json:"fallback_reason,omitempty"`
	RequestID        string          `json:"request_id,omitempty"`
	NormalizationVer string          `json:"normalization_version"`
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

type SourceStatus struct {
	Name               string `json:"name"`
	Enabled            bool   `json:"enabled"`
	QuoteCapable       bool   `json:"quote_capable"`
	DailyBarCapable    bool   `json:"daily_bar_capable"`
	ChipCapable        bool   `json:"chip_capable,omitempty"`
	CapitalFlowCapable bool   `json:"capital_flow_capable,omitempty"`
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
