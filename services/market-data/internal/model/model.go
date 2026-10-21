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
	Name            string `json:"name"`
	Enabled         bool   `json:"enabled"`
	QuoteCapable    bool   `json:"quote_capable"`
	DailyBarCapable bool   `json:"daily_bar_capable"`
}
