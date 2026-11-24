package application

import (
	"context"
	"errors"
	"strings"
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
	Fundamentals(context.Context, string, int, int) (*model.FundamentalsSnapshot, bool, error)
	FundamentalsAsOf(context.Context, string, int, int, time.Time) (*model.FundamentalsSnapshot, error)
	Valuations(context.Context, []string) (*model.ValuationSnapshot, bool, error)
	ValuationsAsOf(context.Context, []string, time.Time) (*model.ValuationSnapshot, error)
}

// Reader adapts the existing DataService use cases to the consumer-facing Application Port.
type Reader struct {
	backend serviceBackend
}

var _ app.MarketDataReader = (*Reader)(nil)
var _ app.DailyBarsV2Reader = (*Reader)(nil)

// NewReader creates an Application Port adapter over the existing DataService.
func NewReader(svc *service.DataService) (*Reader, error) {
	if svc == nil {
		return nil, errReaderUnavailable
	}
	return &Reader{backend: svc}, nil
}

func (r *Reader) GetFundamentals(ctx context.Context, query app.FundamentalsQuery) (app.FundamentalsSnapshot, error) {
	if r == nil || r.backend == nil {
		return app.FundamentalsSnapshot{}, errReaderUnavailable
	}
	if ctx == nil {
		return app.FundamentalsSnapshot{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return app.FundamentalsSnapshot{}, err
	}

	var (
		snapshot *model.FundamentalsSnapshot
		err      error
	)
	if query.SystemObservedAtCutoff != nil {
		snapshot, err = r.backend.FundamentalsAsOf(ctx, query.Symbol, query.Year, query.Quarter, *query.SystemObservedAtCutoff)
	} else {
		snapshot, _, err = r.backend.Fundamentals(ctx, query.Symbol, query.Year, query.Quarter)
	}
	if err != nil {
		return app.FundamentalsSnapshot{}, err
	}
	if snapshot == nil {
		return app.FundamentalsSnapshot{}, errors.New("DataService returned an empty fundamentals snapshot without error")
	}
	return mapFundamentals(*snapshot), nil
}

func (r *Reader) GetValuations(ctx context.Context, query app.ValuationsQuery) (app.ValuationsSnapshot, error) {
	if r == nil || r.backend == nil {
		return app.ValuationsSnapshot{}, errReaderUnavailable
	}
	if ctx == nil {
		return app.ValuationsSnapshot{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return app.ValuationsSnapshot{}, err
	}

	var (
		snapshot *model.ValuationSnapshot
		err      error
	)
	if query.SystemObservedAtCutoff != nil {
		snapshot, err = r.backend.ValuationsAsOf(ctx, query.Symbols, *query.SystemObservedAtCutoff)
	} else {
		snapshot, _, err = r.backend.Valuations(ctx, query.Symbols)
	}
	if err != nil {
		return app.ValuationsSnapshot{}, err
	}
	if snapshot == nil {
		return app.ValuationsSnapshot{}, errors.New("DataService returned an empty valuations snapshot without error")
	}
	return mapValuations(*snapshot), nil
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

type dailyBarsCoverageBackend interface {
	DailyBarsWithCoverageEvidence(context.Context, string, string, string, int, model.AdjustmentBasis) ([]model.DailyBar, bool, []string, service.DailyBarCoverageEvidence, error)
	DailyBarsAsOfWithCoverageEvidence(context.Context, string, string, string, int, model.AdjustmentBasis, time.Time) ([]model.DailyBar, []string, service.DailyBarCoverageEvidence, error)
}

type dailyBarsProviderFilteredBackend interface {
	DailyBarsAsOfWithCoverageEvidenceFromSources(context.Context, string, string, string, int, model.AdjustmentBasis, time.Time, []string) ([]model.DailyBar, []string, service.DailyBarCoverageEvidence, error)
}

// GetDailyBarsV2 returns a versioned snapshot whose CalendarRef is captured by
// the same coverage evaluation that determined the returned series quality.
func (r *Reader) GetDailyBarsV2(ctx context.Context, query app.DailyBarsQuery) (app.DailyBarsV2Snapshot, error) {
	if r == nil || r.backend == nil {
		return app.DailyBarsV2Snapshot{}, errReaderUnavailable
	}
	if ctx == nil {
		return app.DailyBarsV2Snapshot{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return app.DailyBarsV2Snapshot{}, err
	}
	if len(query.AllowedProviderCodes) > 0 {
		if query.SystemObservedAtCutoff == nil || !validDailyBarsProviderCodeSet(query.AllowedProviderCodes) {
			return app.DailyBarsV2Snapshot{}, errors.New("provider-constrained daily-bars reads require a PIT cutoff and canonical provider codes")
		}
	}
	backend, ok := r.backend.(dailyBarsCoverageBackend)
	if !ok {
		return app.DailyBarsV2Snapshot{}, errors.New("market-data backend does not expose daily-bar coverage evidence")
	}
	basis := model.AdjustmentBasis(query.AdjustmentBasis)
	var (
		bars     []model.DailyBar
		warnings []string
		evidence service.DailyBarCoverageEvidence
		err      error
	)
	if query.SystemObservedAtCutoff != nil && len(query.AllowedProviderCodes) > 0 {
		filteredBackend, supported := r.backend.(dailyBarsProviderFilteredBackend)
		if !supported {
			return app.DailyBarsV2Snapshot{}, errors.New("market-data backend does not support provider-constrained PIT daily bars")
		}
		bars, warnings, evidence, err = filteredBackend.DailyBarsAsOfWithCoverageEvidenceFromSources(
			ctx, query.Symbol, query.StartDate, query.EndDate, query.Limit, basis,
			*query.SystemObservedAtCutoff, append([]string(nil), query.AllowedProviderCodes...),
		)
	} else if query.SystemObservedAtCutoff != nil {
		bars, warnings, evidence, err = backend.DailyBarsAsOfWithCoverageEvidence(
			ctx, query.Symbol, query.StartDate, query.EndDate, query.Limit, basis, *query.SystemObservedAtCutoff,
		)
	} else {
		bars, _, warnings, evidence, err = backend.DailyBarsWithCoverageEvidence(
			ctx, query.Symbol, query.StartDate, query.EndDate, query.Limit, basis,
		)
	}
	if err != nil {
		return app.DailyBarsV2Snapshot{}, err
	}
	if len(query.AllowedProviderCodes) > 0 {
		for _, bar := range bars {
			if !containsDailyBarsProvider(query.AllowedProviderCodes, bar.Source) {
				return app.DailyBarsV2Snapshot{}, fmt.Errorf("Data backend returned source %q outside the PIT provider allowlist", bar.Source)
			}
		}
	}
	calendarRangeExplicit := strings.TrimSpace(query.StartDate) != "" && strings.TrimSpace(query.EndDate) != ""
	calendarQualityUnreliable := evidence.CalendarQualityStatus != model.QualityValid ||
		containsReaderString(evidence.Warnings, "daily_bar_coverage_uses_degraded_trading_calendar")
	calendarReferenceMissing := evidence.CoverageBasis != "exchange_calendar" || strings.TrimSpace(evidence.CalendarRef) == "" || !calendarRangeExplicit || calendarQualityUnreliable
	calendarDegradationReason := ""
	switch {
	case !calendarRangeExplicit:
		calendarDegradationReason = "daily_bar_coverage_explicit_range_required_for_calendar_ref"
		evidence.CalendarRef = ""
		evidence.CoverageBasis = "unbounded_range_estimate"
		evidence.CalendarQualityStatus = model.QualityDegraded
	case calendarQualityUnreliable:
		calendarDegradationReason = "daily_bar_coverage_calendar_quality_unreliable"
		evidence.CalendarRef = ""
		evidence.CoverageBasis = "degraded_exchange_calendar"
		evidence.CalendarQualityStatus = model.QualityDegraded
	case calendarReferenceMissing:
		calendarDegradationReason = "daily_bar_coverage_calendar_reference_unavailable"
		evidence.CalendarQualityStatus = model.QualityDegraded
	}
	if calendarDegradationReason != "" {
		evidence.Warnings = dedupeReaderStrings(append(evidence.Warnings, calendarDegradationReason))
	}
	warnings = dedupeReaderStrings(append(append([]string(nil), warnings...), evidence.Warnings...))
	result := app.DailyBarsV2Snapshot{
		Symbol: query.Symbol, Interval: "1d", AdjustmentBasis: query.AdjustmentBasis,
		QualityStatus: aggregateQuality(bars, warnings), Warnings: cloneStrings(warnings),
		SystemObservedAtCutoff: clonePointer(query.SystemObservedAtCutoff),
		CoverageEvidence: app.DailyBarCoverageEvidence{
			CalendarQualityStatus: app.QualityStatus(evidence.CalendarQualityStatus),
			ExpectedTradingDays:   evidence.ExpectedTradingDays, MinimumRequiredBars: evidence.MinimumRequiredBars,
			CoverageBasis: evidence.CoverageBasis, CoverageSufficient: evidence.CoverageSufficient,
			Warnings: cloneStrings(evidence.Warnings),
		},
		Bars: make([]app.DailyBar, 0, len(bars)),
	}
	if strings.TrimSpace(evidence.CalendarRef) != "" {
		calendarRef := evidence.CalendarRef
		result.TradingCalendarRef = &calendarRef
	}
	if len(bars) > 0 {
		if bars[0].Symbol != "" {
			result.Symbol = bars[0].Symbol
		}
	}
	for _, bar := range bars {
		mapped := mapDailyBar(bar)
		if calendarReferenceMissing {
			mapped.QualityStatus = app.QualityDegraded
			mapped.QualityReasons = dedupeReaderStrings(append(mapped.QualityReasons, calendarDegradationReason))
		}
		result.Bars = append(result.Bars, mapped)
	}
	if calendarReferenceMissing && len(result.Bars) > 0 {
		result.QualityStatus = app.QualityDegraded
	}
	return result, nil
}

func containsReaderString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func dedupeReaderStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mapFundamentals(source model.FundamentalsSnapshot) app.FundamentalsSnapshot {
	result := app.FundamentalsSnapshot{
		Symbol: source.Symbol, Year: source.Year, Quarter: source.Quarter, HasData: source.HasData,
		Sections: make([]app.FundamentalSection, 0, len(source.Sections)),
		Source:   source.Source, SourceVersion: source.SourceVersion, FetchedAt: source.FetchedAt,
		SystemObservedAt: clonePointer(source.SystemObservedAt),
		QualityStatus:    app.QualityStatus(source.QualityStatus), QualityReasons: cloneStrings(source.QualityReasons),
	}
	for _, section := range source.Sections {
		mapped := app.FundamentalSection{
			Category: section.Category, ReportDate: section.ReportDate, PublicationDate: section.PublicationDate,
			AvailableAt: clonePointer(section.AvailableAt), HasData: section.HasData,
			Metrics:       make([]app.FundamentalMetric, 0, len(section.Metrics)),
			QualityStatus: app.QualityStatus(section.QualityStatus), QualityReasons: cloneStrings(section.QualityReasons),
		}
		for _, metric := range section.Metrics {
			mapped.Metrics = append(mapped.Metrics, app.FundamentalMetric{
				Key: metric.Key, Name: metric.Name, Value: metric.Value, Unit: metric.Unit, SourceField: metric.SourceField,
			})
		}
		result.Sections = append(result.Sections, mapped)
	}
	return result
}

func mapValuations(source model.ValuationSnapshot) app.ValuationsSnapshot {
	result := app.ValuationsSnapshot{
		RequestedSymbols: cloneStrings(source.RequestedSymbols), Total: source.Total,
		Items: make([]app.ValuationItem, 0, len(source.Items)), MissingSymbols: cloneStrings(source.MissingSymbols),
		ProviderReportDate: source.ProviderReportDate, ProviderTimestamp: clonePointer(source.ProviderTimestamp),
		Source: source.Source, SourceVersion: source.SourceVersion, FetchedAt: source.FetchedAt,
		SystemObservedAt: clonePointer(source.SystemObservedAt),
		QualityStatus:    app.QualityStatus(source.QualityStatus), QualityReasons: cloneStrings(source.QualityReasons),
	}
	for _, item := range source.Items {
		result.Items = append(result.Items, app.ValuationItem{
			Symbol: item.Symbol, Code: item.Code, Name: item.Name,
			PETTM: clonePointer(item.PETTM), PEMRQ: clonePointer(item.PEMRQ), PEStatic: clonePointer(item.PEStatic),
			PBMRQ: clonePointer(item.PBMRQ), PSTTM: clonePointer(item.PSTTM), PCFTTM: clonePointer(item.PCFTTM),
			TurnoverRatePercent:      clonePointer(item.TurnoverRatePercent),
			FloatTurnoverRatePercent: clonePointer(item.FloatTurnoverRatePercent),
			VolumeRatio:              clonePointer(item.VolumeRatio), DividendYieldPercent: clonePointer(item.DividendYieldPercent),
			DividendYieldTTMPercent: clonePointer(item.DividendYieldTTMPercent),
			Source:                  item.Source, SourceVersion: item.SourceVersion, FetchedAt: item.FetchedAt,
			QualityStatus: app.QualityStatus(item.QualityStatus), QualityReasons: cloneStrings(item.QualityReasons),
		})
	}
	return result
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
		Symbol:                                source.Symbol,
		TradeDate:                             source.TradeDate,
		IsPartial:                             source.IsPartial,
		Period:                                source.Period,
		Open:                                  source.Open,
		High:                                  source.High,
		Low:                                   source.Low,
		Close:                                 source.Close,
		MA5:                                   clonePointer(source.MA5),
		MA20:                                  clonePointer(source.MA20),
		MA120:                                 clonePointer(source.MA120),
		MA250:                                 clonePointer(source.MA250),
		Volume:                                clonePointer(source.Volume),
		Amount:                                clonePointer(source.Amount),
		ChangePercent:                         clonePointer(source.ChangePercent),
		AdjustmentBasis:                       app.AdjustmentBasis(source.AdjustmentBasis),
		AdjustmentMethod:                      source.AdjustmentMethod,
		AdjustmentReferenceDate:               source.AdjustmentRefDate,
		AdjustmentFactorSource:                source.AdjustmentFactorSource,
		AdjustmentFactorSourceVersion:         source.AdjustmentFactorSourceVersion,
		AdjustmentFactorRevisionHash:          source.AdjustmentFactorRevisionHash,
		AdjustmentFactorSetHash:               source.AdjustmentFactorSetHash,
		AdjustmentFactorReferenceRevisionHash: source.AdjustmentFactorReferenceRevisionHash,
		AdjustmentAlgorithmVersion:            source.AdjustmentAlgorithmVersion,
		AdjustmentFactorSystemObservedAt:      clonePointer(source.AdjustmentFactorSystemObservedAt),
		Source:                                source.Source,
		SourceVersion:                         source.SourceVersion,
		ProviderTime:                          clonePointer(source.ProviderTime),
		FetchedAt:                             source.FetchedAt,
		NormalizedAt:                          source.NormalizedAt,
		SystemObservedAt:                      clonePointer(source.SystemObservedAt),
		RawVolume:                             clonePointer(source.RawVolume),
		RawVolumeUnit:                         source.RawVolumeUnit,
		RawAmount:                             clonePointer(source.RawAmount),
		RawAmountUnit:                         source.RawAmountUnit,
		RawAmountField:                        source.RawAmountField,
		VolumeUnit:                            source.VolumeUnit,
		AmountUnit:                            source.AmountUnit,
		QualityStatus:                         app.QualityStatus(source.QualityStatus),
		QualityReasons:                        cloneStrings(source.QualityReasons),
		FallbackFrom:                          source.FallbackFrom,
		FallbackReason:                        source.FallbackReason,
		RequestID:                             source.RequestID,
		NormalizationVersion:                  source.NormalizationVer,
		ExactValues:                           mapDailyBarExactValues(source.ExactValues),
	}
}

func mapDailyBarExactValues(source *model.DailyBarExactValues) *app.DailyBarExactValues {
	if source == nil {
		return nil
	}
	return &app.DailyBarExactValues{
		Open: source.Open, High: source.High, Low: source.Low, Close: source.Close,
		RawVolume: source.RawVolume, RawAmount: clonePointer(source.RawAmount), AmountCNY: clonePointer(source.AmountCNY),
		ChangePercent: clonePointer(source.ChangePercent), ChangePercentSourceField: source.ChangePercentSourceField,
		ChangePercentEvidence: mapDailyBarDecimalEvidence(source.ChangePercentEvidence),
		OpenEvidence:          mapDailyBarDecimalEvidence(source.OpenEvidence),
		HighEvidence:          mapDailyBarDecimalEvidence(source.HighEvidence),
		LowEvidence:           mapDailyBarDecimalEvidence(source.LowEvidence),
		CloseEvidence:         mapDailyBarDecimalEvidence(source.CloseEvidence),
		RawVolumeEvidence:     mapDailyBarDecimalEvidence(source.RawVolumeEvidence),
		RawAmountEvidence:     mapDailyBarDecimalEvidence(source.RawAmountEvidence),
		AmountCNYEvidence:     mapDailyBarDecimalEvidence(source.AmountCNYEvidence),
	}
}

func mapDailyBarDecimalEvidence(source *model.DailyBarDecimalEvidence) *app.DailyBarDecimalEvidence {
	if source == nil {
		return nil
	}
	return &app.DailyBarDecimalEvidence{
		EvidenceKind: string(source.EvidenceKind), SourceToken: source.SourceToken,
		SourceFractionDigits: clonePointer(source.SourceFractionDigits),
		ProtocolScale:        clonePointer(source.ProtocolScale),
		UnitConversionFactor: source.UnitConversionFactor,
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

func validDailyBarsProviderCodeSet(values []string) bool {
	if len(values) == 0 {
		return false
	}
	for index, value := range values {
		if value == "" || value != strings.TrimSpace(value) {
			return false
		}
		for _, char := range value {
			if !(char >= 'a' && char <= 'z') && !(char >= '0' && char <= '9') && char != '-' && char != '_' {
				return false
			}
		}
		if index > 0 && values[index-1] >= value {
			return false
		}
	}
	return true
}

func containsDailyBarsProvider(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
