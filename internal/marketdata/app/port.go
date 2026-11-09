// Package app defines the Market Data application boundary consumed by other modules.
// It intentionally exposes neither Provider, Repository, Cache nor HTTP transport types.
package app

import (
	"context"
	"time"
)

// AdjustmentBasis describes the price adjustment basis of a returned quote/bar.
type AdjustmentBasis string

const (
	AdjustmentNone     AdjustmentBasis = "none"
	AdjustmentForward  AdjustmentBasis = "forward_adjusted"
	AdjustmentBackward AdjustmentBasis = "backward_adjusted"
	AdjustmentUnknown  AdjustmentBasis = "unknown"
)

// QualityStatus is the Data-owned quality state of a value or snapshot.
type QualityStatus string

const (
	QualityValid    QualityStatus = "valid"
	QualityDegraded QualityStatus = "degraded"
	QualityStale    QualityStatus = "stale"
	QualityInvalid  QualityStatus = "invalid"
	QualityUnknown  QualityStatus = "unknown"
)

// QuoteSnapshot is an application DTO, not the Data HTTP JSON response.
// Nullable numeric values remain pointers; Volume remains int64 to preserve share precision.
type QuoteSnapshot struct {
	Symbol               string
	Name                 string
	CurrentPrice         *float64
	Change               *float64
	ChangePercent        *float64
	Open                 *float64
	High                 *float64
	Low                  *float64
	PrevClose            *float64
	Volume               *int64
	Amount               *float64
	VolumeRatio          *float64
	TurnoverRate         *float64
	QuoteTime            *time.Time
	QuoteTimeSemantics   string
	FetchedAt            time.Time
	NormalizedAt         time.Time
	Source               string
	SourceVersion        string
	RawVolume            *float64
	RawVolumeUnit        string
	RawAmount            *float64
	RawAmountUnit        string
	RawAmountField       string
	VolumeUnit           string
	AmountUnit           string
	AdjustmentBasis      AdjustmentBasis
	QualityStatus        QualityStatus
	QualityReasons       []string
	FallbackFrom         string
	FallbackReason       string
	RequestID            string
	NormalizationVersion string
}

// DailyBar is an application DTO carrying the provenance and point-in-time
// metadata required by downstream analysis. Unknown numeric fields stay nil.
type DailyBar struct {
	Symbol                           string
	TradeDate                        string
	IsPartial                        bool
	Period                           string
	Open                             float64
	High                             float64
	Low                              float64
	Close                            float64
	MA5                              *float64
	MA20                             *float64
	MA120                            *float64
	MA250                            *float64
	Volume                           *int64
	Amount                           *float64
	ChangePercent                    *float64
	AdjustmentBasis                  AdjustmentBasis
	AdjustmentMethod                 string
	AdjustmentReferenceDate          string
	AdjustmentFactorSource           string
	AdjustmentFactorSourceVersion    string
	AdjustmentFactorSystemObservedAt *time.Time
	Source                           string
	SourceVersion                    string
	ProviderTime                     *time.Time
	FetchedAt                        time.Time
	NormalizedAt                     time.Time
	SystemObservedAt                 *time.Time
	RawVolume                        *float64
	RawVolumeUnit                    string
	RawAmount                        *float64
	RawAmountUnit                    string
	RawAmountField                   string
	VolumeUnit                       string
	AmountUnit                       string
	QualityStatus                    QualityStatus
	QualityReasons                   []string
	FallbackFrom                     string
	FallbackReason                   string
	RequestID                        string
	NormalizationVersion             string
}

// DailyBarsQuery is the explicit read contract for current or historical bars.
// SystemObservedAtCutoff is the system-observation/revision cutoff, not a trade
// date and not the ProviderTime of a quote/bar. Nil requests the current view.
type DailyBarsQuery struct {
	Symbol                 string
	StartDate              string
	EndDate                string
	Limit                  int
	AdjustmentBasis        AdjustmentBasis
	SystemObservedAtCutoff *time.Time
}

// DailyBarsSnapshot preserves per-bar metadata and Data's aggregate quality assessment.
// Aggregate QualityStatus is derived from returned bar quality and Data warnings.
type DailyBarsSnapshot struct {
	Symbol                 string
	AdjustmentBasis        AdjustmentBasis
	VolumeUnit             string
	AmountUnit             string
	QualityStatus          QualityStatus
	Warnings               []string
	SystemObservedAtCutoff *time.Time
	Bars                   []DailyBar
}

// QuoteReader is the narrow quote use case. It does not expose Provider selection.
type QuoteReader interface {
	GetQuote(ctx context.Context, symbol string) (QuoteSnapshot, error)
}

// DailyBarsReader is the narrow historical-data use case. A non-nil
// SystemObservedAtCutoff requires revision-backed point-in-time reconstruction.
type DailyBarsReader interface {
	GetDailyBars(ctx context.Context, query DailyBarsQuery) (DailyBarsSnapshot, error)
}

// MarketDataReader is the current consumer-facing port used by Analysis.
// Future consumers should depend on the narrowest interface they need.
type MarketDataReader interface {
	QuoteReader
	DailyBarsReader
}
