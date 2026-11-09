package application

import (
	"context"
	"errors"
	"time"

	"example.com/stock-analysis-flow/internal/marketdata/app"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/normalize"
	"example.com/stock-analysis-flow/internal/marketdata/service"
)

var errReaderUnavailable = errors.New("market-data application reader is not configured")

type serviceBackend interface {
	Quote(context.Context, string) (*model.Quote, bool, error)
	DailyBars(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, bool, []string, error)
	DailyBarsAsOf(context.Context, string, string, string, int, model.AdjustmentBasis, time.Time) ([]model.DailyBar, []string, error)
}

// Reader adapts the existing DataService use cases to the consumer-facing Application Port.
type Reader struct {
	backend serviceBackend
}

var _ app.MarketDataReader = (*Reader)(nil)

// NewReader creates an Application Port adapter over the existing DataService.
func NewReader(svc *service.DataService) (*Reader, error) {
	if svc == nil {
		return nil, errReaderUnavailable
	}
	return &Reader{backend: svc}, nil
}

func (r *Reader) GetQuote(ctx context.Context, symbol string) (app.QuoteSnapshot, error) {
	if r == nil || r.backend == nil {
		return app.QuoteSnapshot{}, errReaderUnavailable
	}
	if ctx == nil {
		return app.QuoteSnapshot{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return app.QuoteSnapshot{}, err
	}
	quote, _, err := r.backend.Quote(ctx, symbol)
	if err != nil {
		return app.QuoteSnapshot{}, err
	}
	if quote == nil {
		return app.QuoteSnapshot{}, errors.New("DataService returned an empty quote without error")
	}
	return mapQuote(*quote), nil
}

func (r *Reader) GetDailyBars(ctx context.Context, query app.DailyBarsQuery) (app.DailyBarsSnapshot, error) {
	if r == nil || r.backend == nil {
		return app.DailyBarsSnapshot{}, errReaderUnavailable
	}
	if ctx == nil {
		return app.DailyBarsSnapshot{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return app.DailyBarsSnapshot{}, err
	}

	basis := model.AdjustmentBasis(query.AdjustmentBasis)
	var (
		bars     []model.DailyBar
		warnings []string
		err      error
	)
	if query.SystemObservedAtCutoff != nil {
		bars, warnings, err = r.backend.DailyBarsAsOf(
			ctx, query.Symbol, query.StartDate, query.EndDate, query.Limit, basis, *query.SystemObservedAtCutoff,
		)
	} else {
		bars, _, warnings, err = r.backend.DailyBars(
			ctx, query.Symbol, query.StartDate, query.EndDate, query.Limit, basis,
		)
	}
	if err != nil {
		return app.DailyBarsSnapshot{}, err
	}

	result := app.DailyBarsSnapshot{
		Symbol:                 query.Symbol,
		AdjustmentBasis:        query.AdjustmentBasis,
		QualityStatus:          aggregateQuality(bars, warnings),
		Warnings:               cloneStrings(warnings),
		SystemObservedAtCutoff: clonePointer(query.SystemObservedAtCutoff),
		Bars:                   make([]app.DailyBar, 0, len(bars)),
	}
	if len(bars) > 0 {
		if bars[0].Symbol != "" {
			result.Symbol = bars[0].Symbol
		}
	}
	for _, bar := range bars {
		result.Bars = append(result.Bars, mapDailyBar(bar))
	}
	return result, nil
}

func mapQuote(source model.Quote) app.QuoteSnapshot {
	return app.QuoteSnapshot{
		Symbol:               source.Symbol,
		Name:                 source.Name,
		CurrentPrice:         clonePointer(source.CurrentPrice),
		Change:               clonePointer(source.Change),
		ChangePercent:        clonePointer(source.ChangePercent),
		Open:                 clonePointer(source.Open),
		High:                 clonePointer(source.High),
		Low:                  clonePointer(source.Low),
		PrevClose:            clonePointer(source.PrevClose),
		Volume:               clonePointer(source.Volume),
		Amount:               clonePointer(source.Amount),
		VolumeRatio:          clonePointer(source.VolumeRatio),
		TurnoverRate:         clonePointer(source.TurnoverRate),
		QuoteTime:            clonePointer(source.QuoteTime),
		QuoteTimeSemantics:   source.QuoteTimeSemantics,
		FetchedAt:            source.FetchedAt,
		NormalizedAt:         source.NormalizedAt,
		Source:               source.Source,
		SourceVersion:        source.SourceVersion,
		RawVolume:            clonePointer(source.RawVolume),
		RawVolumeUnit:        source.RawVolumeUnit,
		RawAmount:            clonePointer(source.RawAmount),
		RawAmountUnit:        source.RawAmountUnit,
		RawAmountField:       source.RawAmountField,
		VolumeUnit:           source.VolumeUnit,
		AmountUnit:           source.AmountUnit,
		AdjustmentBasis:      app.AdjustmentBasis(source.AdjustmentBasis),
		QualityStatus:        app.QualityStatus(source.QualityStatus),
		QualityReasons:       cloneStrings(source.QualityReasons),
		FallbackFrom:         source.FallbackFrom,
		FallbackReason:       source.FallbackReason,
		RequestID:            source.RequestID,
		NormalizationVersion: source.NormalizationVer,
	}
}

func mapDailyBar(source model.DailyBar) app.DailyBar {
	return app.DailyBar{
		Symbol:                           source.Symbol,
		TradeDate:                        source.TradeDate,
		IsPartial:                        source.IsPartial,
		Period:                           source.Period,
		Open:                             source.Open,
		High:                             source.High,
		Low:                              source.Low,
		Close:                            source.Close,
		MA5:                              clonePointer(source.MA5),
		MA20:                             clonePointer(source.MA20),
		MA120:                            clonePointer(source.MA120),
		MA250:                            clonePointer(source.MA250),
		Volume:                           clonePointer(source.Volume),
		Amount:                           clonePointer(source.Amount),
		ChangePercent:                    clonePointer(source.ChangePercent),
		AdjustmentBasis:                  app.AdjustmentBasis(source.AdjustmentBasis),
		AdjustmentMethod:                 source.AdjustmentMethod,
		AdjustmentReferenceDate:          source.AdjustmentRefDate,
		AdjustmentFactorSource:           source.AdjustmentFactorSource,
		AdjustmentFactorSourceVersion:    source.AdjustmentFactorSourceVersion,
		AdjustmentFactorSystemObservedAt: clonePointer(source.AdjustmentFactorSystemObservedAt),
		Source:                           source.Source,
		SourceVersion:                    source.SourceVersion,
		ProviderTime:                     clonePointer(source.ProviderTime),
		FetchedAt:                        source.FetchedAt,
		NormalizedAt:                     source.NormalizedAt,
		SystemObservedAt:                 clonePointer(source.SystemObservedAt),
		RawVolume:                        clonePointer(source.RawVolume),
		RawVolumeUnit:                    source.RawVolumeUnit,
		RawAmount:                        clonePointer(source.RawAmount),
		RawAmountUnit:                    source.RawAmountUnit,
		RawAmountField:                   source.RawAmountField,
		VolumeUnit:                       source.VolumeUnit,
		AmountUnit:                       source.AmountUnit,
		QualityStatus:                    app.QualityStatus(source.QualityStatus),
		QualityReasons:                   cloneStrings(source.QualityReasons),
		FallbackFrom:                     source.FallbackFrom,
		FallbackReason:                   source.FallbackReason,
		RequestID:                        source.RequestID,
		NormalizationVersion:             source.NormalizationVer,
	}
}

func aggregateQuality(bars []model.DailyBar, warnings []string) app.QualityStatus {
	if len(bars) == 0 {
		return app.QualityUnknown
	}
	if normalize.HasQualityDegradingBarWarnings(warnings) {
		return app.QualityDegraded
	}
	for _, bar := range bars {
		if bar.QualityStatus != model.QualityValid {
			return app.QualityDegraded
		}
	}
	return app.QualityValid
}

func clonePointer[T any](source *T) *T {
	if source == nil {
		return nil
	}
	value := *source
	return &value
}

func cloneStrings(source []string) []string {
	if source == nil {
		return nil
	}
	return append([]string(nil), source...)
}
