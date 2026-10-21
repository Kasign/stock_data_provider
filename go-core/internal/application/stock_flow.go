package application

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/contextpack"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/indicators"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/marketdata"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/ports"
)

type StockFlow struct {
	marketData  ports.MarketDataProvider
	search      ports.NewsSearcher
	analyzer    ports.Analyzer
	persistence ports.Persistence
	notifier    ports.Notifier
	now         func() time.Time
}

func NewStockFlow(m ports.MarketDataProvider, s ports.NewsSearcher, a ports.Analyzer, p ports.Persistence, n ports.Notifier) *StockFlow {
	return &StockFlow{marketData: m, search: s, analyzer: a, persistence: p, notifier: n, now: time.Now}
}

func (f *StockFlow) Run(ctx context.Context, request domain.AnalysisRequest) (response domain.StockWorkflowResponse, err error) {
	started := f.now().UTC()
	code := strings.ToUpper(strings.TrimSpace(request.StockCode))
	if code == "" {
		return response, &InputError{Message: "stock_code is required"}
	}
	if len(code) > 32 {
		return response, &InputError{Message: "stock_code must be at most 32 characters"}
	}
	if f.marketData == nil {
		return response, errors.New("market data provider is not configured")
	}
	if f.analyzer == nil {
		return response, errors.New("stock analyzer is not configured")
	}
	queryID := strings.TrimSpace(request.QueryID)
	if queryID == "" {
		queryID = newQueryID("stock", code, started)
	}
	asOf := request.AsOf
	asOfExplicit := !asOf.IsZero()
	if asOfExplicit {
		delta := started.Sub(asOf)
		if delta < 0 || delta > 90*time.Second {
			return response, &InputError{Message: "this live stock-analysis workflow requires an explicit as_of to be at or before execution and within 90 seconds; the current Python bridge cannot provide historically aligned quote, fundamentals, chip and news snapshots"}
		}
	}
	lookback := request.LookbackDays
	if lookback == 0 {
		lookback = 60
	}
	if lookback < 20 {
		lookback = 20
	}
	if lookback > 400 {
		lookback = 400
	}
	reportType := strings.TrimSpace(request.ReportType)
	if reportType == "" {
		reportType = "detailed"
	}
	trace := domain.WorkflowTrace{QueryID: queryID, StartedAt: started}
	warnings := make([]string, 0)

	asOfArgument := ""
	if asOfExplicit {
		asOfArgument = asOf.Format(time.RFC3339Nano)
	}
	bundle, stageErr := runStage(ctx, &trace, f.now, "fetch_stock_bundle", func() (domain.StockBundle, error) {
		return f.marketData.FetchStockBundle(ctx, code, lookback, asOfArgument)
	})
	if stageErr != nil {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, fmt.Errorf("fetch stock bundle: %w", stageErr)
	}
	if bundle.Code == "" {
		bundle.Code = code
	} else if strings.ToUpper(strings.TrimSpace(bundle.Code)) != code {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, errors.New("market data provider returned a stock code different from the requested target")
	}
	if bundle.Name == "" {
		bundle.Name = code
	}
	// For a live request the market-data service decides the snapshot time after
	// fetching its quote and bars. Do not turn the workflow start time into a
	// strict point-in-time cutoff, which can reject a quote fetched milliseconds later.
	if !asOfExplicit {
		switch {
		case !bundle.AnalysisReferenceTime.IsZero():
			asOf = bundle.AnalysisReferenceTime.UTC()
		case !bundle.FetchedAt.IsZero():
			asOf = bundle.FetchedAt.UTC()
		default:
			asOf = f.now().UTC()
		}
	}
	var normalizationWarnings []string
	if bundle.Quote != nil {
		if bundle.Quote.Code == "" {
			bundle.Quote.Code = code
		}
		if bundle.Quote.Name == "" {
			bundle.Quote.Name = bundle.Name
		}
		if bundle.Quote.Price != nil && (!isFinite(*bundle.Quote.Price) || *bundle.Quote.Price <= 0) {
			bundle.Quote.Price = nil
			normalizationWarnings = append(normalizationWarnings, "discarded invalid realtime quote price; any historical close is exposed only as a labelled last-completed-close reference")
		}
	}

	normalizedBars, normalizationStats := marketdata.NormalizeDailyBars(bundle.DailyBars)
	cutoffDate := marketdata.MarketDate(code, asOf)
	filteredBars, futureBarCount := marketdata.FilterBarsAfterDate(normalizedBars, cutoffDate)
	bundle.DailyBars = filteredBars
	if marketdata.MarkLatestCNIntradayBar(bundle.DailyBars, code, asOf, f.now().UTC()) {
		normalizationWarnings = append(normalizationWarnings, "latest daily bar is an intraday partial bar; it is not a completed closing candle")
	}
	if futureBarCount > 0 {
		normalizationWarnings = append(normalizationWarnings, fmt.Sprintf("dropped %d daily-bar rows later than requested analysis cutoff %s", futureBarCount, cutoffDate))
	}
	if normalizationStats.DroppedInvalid > 0 {
		normalizationWarnings = append(normalizationWarnings, fmt.Sprintf("dropped %d invalid daily-bar rows", normalizationStats.DroppedInvalid))
	}
	if normalizationStats.DuplicateDates > 0 {
		normalizationWarnings = append(normalizationWarnings, fmt.Sprintf("collapsed %d duplicate daily-bar dates using the last valid row", normalizationStats.DuplicateDates))
	}
	if asOfExplicit {
		if markQuoteAfterAnalysisCutoff(bundle.Quote, asOf) {
			normalizationWarnings = append(normalizationWarnings, "realtime quote timestamp is later than requested analysis as_of; quote values were excluded")
		}
	} else if markQuoteAfterLiveSnapshotReference(bundle.Quote, asOf) {
		// In live mode the provider and local clock may differ by a small amount.
		// Keep the quote as a separately-labelled degraded observation rather than
		// applying historical point-in-time rejection to an ordinary live request.
		normalizationWarnings = append(normalizationWarnings, "realtime quote timestamp is slightly later than the bundle reference time; quote is retained as degraded live data and must not be used for strict point-in-time claims")
	}
	bundle.Warnings = append(bundle.Warnings, normalizationWarnings...)
	if bundle.Quote == nil && len(bundle.DailyBars) == 0 {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, errors.New("no quote or daily bars available; analysis was not generated")
	}

	// Persist only completed bars. An intraday partial candle is useful as a
	// clearly marked analysis input, but writing it to stock_daily would make it
	// look like a completed trading-day record and can poison resume/cache checks.
	persistableBars := make([]domain.DailyBar, 0, len(bundle.DailyBars))
	partialBars := 0
	unitUnverifiedBars := 0
	for _, bar := range bundle.DailyBars {
		if bar.IsPartial {
			partialBars++
			continue
		}
		// The migration bridge may return legacy-provider bars whose volume,
		// amount and adjustment contracts have not been verified. Persisting the
		// numeric payload would make unknown units look canonical in stock_daily.
		if bar.VolumeUnit != "shares" || bar.AmountUnit != "CNY" || (bar.AdjustmentBasis != "none" && bar.AdjustmentBasis != "forward_adjusted" && bar.AdjustmentBasis != "backward_adjusted") || (bar.QualityStatus != "valid" && bar.QualityStatus != "degraded") || containsField(bar.MissingFields, "volume") || containsField(bar.MissingFields, "amount") || containsField(bar.MissingFields, "pct_chg") {
			unitUnverifiedBars++
			continue
		}
		persistableBars = append(persistableBars, bar)
	}
	if partialBars > 0 {
		warnings = append(warnings, fmt.Sprintf("excluded %d intraday partial daily bar(s) from completed-history persistence", partialBars))
	}
	if unitUnverifiedBars > 0 {
		warnings = append(warnings, fmt.Sprintf("excluded %d daily bar(s) with unverified units, adjustment basis, quality, or required fields from legacy persistence", unitUnverifiedBars))
	}
	// The stock_daily table remains owned by the Python persistence adapter
	// during migration. Data acquisition itself must not write database state.
	if f.persistence != nil && len(persistableBars) > 0 {
		source := "unknown"
		if bundle.Sources != nil && strings.TrimSpace(bundle.Sources["daily_bars"]) != "" {
			source = bundle.Sources["daily_bars"]
		}
		_, saveBarsErr := runStage(ctx, &trace, f.now, "persist_daily_bars", func() (int, error) {
			return f.persistence.SaveDailyBars(ctx, code, persistableBars, source)
		})
		if saveBarsErr != nil {
			warnings = append(warnings, "daily history persistence failed: "+saveBarsErr.Error())
		}
	} else {
		message := "no verified completed daily bars or persistence adapter configured"
		trace.Stages = append(trace.Stages, domain.StageResult{Name: "persist_daily_bars", Status: "skipped", Message: message})
	}

	indicatorBars := completedBarsForIndicators(bundle.DailyBars)
	completedBarsCount := 0
	for _, bar := range bundle.DailyBars {
		if !bar.IsPartial {
			completedBarsCount++
		}
	}
	if len(indicatorBars) < completedBarsCount {
		warnings = append(warnings, fmt.Sprintf("excluded %d completed daily bar(s) with unknown/stale quality, missing provenance, or mismatched price basis from technical indicators", completedBarsCount-len(indicatorBars)))
	}
	technical, stageErr := runStage(ctx, &trace, f.now, "calculate_technical_indicators", func() (domain.TechnicalSnapshot, error) {
		return indicators.Calculate(indicatorBars)
	})
	if stageErr != nil {
		warnings = append(warnings, "technical indicators unavailable: "+stageErr.Error())
		technical = domain.TechnicalSnapshot{}
	}

	var news []domain.NewsItem
	newsSearchPerformed := false
	searchNews := request.SearchNews == nil || *request.SearchNews
	if searchNews && f.search != nil {
		news, stageErr = runStage(ctx, &trace, f.now, "search_stock_news", func() ([]domain.NewsItem, error) { return f.search.SearchStockNews(ctx, code, bundle.Name, 8) })
		newsSearchPerformed = stageErr == nil
		if stageErr != nil {
			warnings = append(warnings, "news search failed: "+stageErr.Error())
		} else {
			var filtered []domain.NewsItem
			var newsWarnings []string
			filtered, newsWarnings = filterNewsByReference(news, asOf, asOfExplicit)
			news = filtered
			warnings = append(warnings, newsWarnings...)
		}
	} else {
		trace.Stages = append(trace.Stages, domain.StageResult{Name: "search_stock_news", Status: "skipped", Message: "disabled by request or search adapter unavailable"})
	}
	analysisContext := contextpack.BuildStockContext(bundle, technical, news, asOf.Format(time.RFC3339))
	analysisContext.AsOf = asOf
	analysisContext.NewsSearchPerformed = newsSearchPerformed
	if newsSearchPerformed {
		count := len(news)
		analysisContext.NewsResultCount = &count
	}
	warnings = append(warnings, analysisContext.Warnings...)
	result, stageErr := runStage(ctx, &trace, f.now, "llm_stock_analysis", func() (domain.AnalysisResult, error) {
		return f.analyzer.AnalyzeStock(ctx, analysisContext, news, reportType, queryID, request.ReportLanguage)
	})
	if stageErr != nil {
		trace.FinishedAt = f.now().UTC()
		response.Trace = trace
		return response, fmt.Errorf("analyze stock: %w", stageErr)
	}
	result.Code = code
	if result.Name == "" {
		result.Name = bundle.Name
	}
	result.QueryID = queryID
	if result.ContextSnapshot == nil {
		result.ContextSnapshot = make(map[string]any)
	}
	result.ContextSnapshot["data_quality"] = analysisContext.DataQuality
	result.ContextSnapshot["technical"] = analysisContext.Technical
	result.ContextSnapshot["sources"] = bundle.Sources
	result.ContextSnapshot["as_of"] = asOf.Format(time.RFC3339Nano)
	result.ContextSnapshot["as_of_explicit"] = asOfExplicit
	if bundle.SnapshotID != "" {
		result.ContextSnapshot["data_snapshot_id"] = bundle.SnapshotID
	}
	if !bundle.FetchedAt.IsZero() {
		result.ContextSnapshot["data_snapshot_fetched_at"] = bundle.FetchedAt.UTC().Format(time.RFC3339Nano)
	}
	if !result.Success {
		if result.ErrorMessage == "" {
			result.ErrorMessage = "analyzer returned unsuccessful result"
		}
		warnings = append(warnings, result.ErrorMessage)
	}

	if f.persistence != nil {
		id, saveErr := runStage(ctx, &trace, f.now, "persist_stock_analysis", func() (int64, error) {
			return f.persistence.SaveStockAnalysis(ctx, result, analysisContext, reportType, queryID)
		})
		if saveErr != nil {
			warnings = append(warnings, "persistence failed: "+saveErr.Error())
		} else if id > 0 {
			if result.ContextSnapshot == nil {
				result.ContextSnapshot = map[string]any{}
			}
			result.ContextSnapshot["saved_history_id"] = id
		}
	} else {
		trace.Stages = append(trace.Stages, domain.StageResult{Name: "persist_stock_analysis", Status: "skipped", Message: "no persistence adapter configured"})
	}
	if request.Notify && result.Success && f.notifier != nil {
		_, notifyErr := runStage(ctx, &trace, f.now, "notify_stock_analysis", func() (struct{}, error) { return struct{}{}, f.notifier.NotifyStockAnalysis(ctx, result, reportType) })
		if notifyErr != nil {
			warnings = append(warnings, "notification failed: "+notifyErr.Error())
		}
	} else {
		message := "notification disabled or adapter unavailable"
		if request.Notify && !result.Success {
			message = "skipped because analysis result is unsuccessful"
		}
		trace.Stages = append(trace.Stages, domain.StageResult{Name: "notify_stock_analysis", Status: "skipped", Message: message})
	}
	trace.FinishedAt = f.now().UTC()
	response = domain.StockWorkflowResponse{Result: result, Context: analysisContext, Trace: trace, Warnings: dedupe(warnings)}
	return response, nil
}

func runStage[T any](ctx context.Context, trace *domain.WorkflowTrace, now func() time.Time, name string, fn func() (T, error)) (T, error) {
	start := now().UTC()
	var zero T
	if err := ctx.Err(); err != nil {
		trace.Stages = append(trace.Stages, domain.StageResult{Name: name, Status: "cancelled", Message: err.Error()})
		return zero, err
	}
	value, err := fn()
	elapsed := now().UTC().Sub(start).Milliseconds()
	if elapsed < 0 {
		elapsed = 0
	}
	stage := domain.StageResult{Name: name, DurationMS: elapsed, Status: "succeeded"}
	if err != nil {
		stage.Status = "failed"
		stage.Message = err.Error()
	}
	trace.Stages = append(trace.Stages, stage)
	return value, err
}
func newQueryID(prefix, subject string, now time.Time) string {
	return fmt.Sprintf("go-%s-%s-%d", prefix, strings.NewReplacer("/", "-", ".", "-").Replace(subject), now.UnixNano())
}
func isFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func dedupe(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// completedBarsForIndicators prevents an unfinished intraday daily candle from
// shifting moving averages, RSI, MACD, or trend classification. The partial bar
// remains available in the analysis context as a separately labeled overlay.
func quoteObservationTime(quote *domain.Quote) (time.Time, bool) {
	if quote == nil {
		return time.Time{}, false
	}
	text := strings.TrimSpace(quote.QuoteTime)
	if text == "" {
		text = strings.TrimSpace(quote.Timestamp)
	}
	if text != "" {
		parsed, err := time.Parse(time.RFC3339Nano, text)
		if err == nil {
			return parsed.UTC(), true
		}
	}
	if quote.ProviderTimestamp != nil && !quote.ProviderTimestamp.IsZero() {
		return quote.ProviderTimestamp.UTC(), true
	}
	return time.Time{}, false
}

// markQuoteAfterAnalysisCutoff strictly rejects quotes after an explicitly
// requested historical/reference cutoff. It must not be used for ordinary live
// requests, where the snapshot reference is captured after provider calls.
func markQuoteAfterAnalysisCutoff(quote *domain.Quote, cutoff time.Time) bool {
	if quote == nil || cutoff.IsZero() {
		return false
	}
	quoteTime, ok := quoteObservationTime(quote)
	if !ok || !quoteTime.After(cutoff) {
		return false
	}
	quote.QualityStatus = "unknown"
	if !containsField(quote.QualityReasons, "quote_timestamp_after_analysis_cutoff") {
		quote.QualityReasons = append(quote.QualityReasons, "quote_timestamp_after_analysis_cutoff")
	}
	return true
}

// markQuoteAfterLiveSnapshotReference is a soft diagnostic for small clock skew
// in live mode; it does not make the quote unusable. Historical requests use the
// strict function above instead.
func markQuoteAfterLiveSnapshotReference(quote *domain.Quote, reference time.Time) bool {
	if quote == nil || reference.IsZero() {
		return false
	}
	quoteTime, ok := quoteObservationTime(quote)
	if !ok || !quoteTime.After(reference) {
		return false
	}
	if quote.QualityStatus == "valid" {
		quote.QualityStatus = "degraded"
	}
	if !containsField(quote.QualityReasons, "provider_timestamp_after_live_snapshot_reference") {
		quote.QualityReasons = append(quote.QualityReasons, "provider_timestamp_after_live_snapshot_reference")
	}
	return true
}

func containsField(fields []string, target string) bool {
	for _, field := range fields {
		if field == target {
			return true
		}
	}
	return false
}

func completedBarsForIndicators(bars []domain.DailyBar) []domain.DailyBar {
	// Only a contiguous suffix from one source/normalization version/adjustment
	// reference is eligible. A stale or unknown-quality series may be shown as
	// historical context, but must not produce apparently current MA/RSI values.
	end := len(bars) - 1
	for end >= 0 && bars[end].IsPartial {
		end--
	}
	if end < 0 || !indicatorBarUsable(bars[end]) {
		return nil
	}
	anchor := bars[end]
	start := end
	for start >= 0 {
		bar := bars[start]
		if bar.IsPartial || !indicatorBarUsable(bar) ||
			bar.AdjustmentBasis != anchor.AdjustmentBasis ||
			bar.Source != anchor.Source ||
			bar.NormalizationVersion != anchor.NormalizationVersion ||
			bar.AdjustmentReferenceDate != anchor.AdjustmentReferenceDate {
			break
		}
		start--
	}
	return append([]domain.DailyBar(nil), bars[start+1:end+1]...)
}

func indicatorBarUsable(bar domain.DailyBar) bool {
	if bar.IsPartial || !knownAdjustmentBasis(bar.AdjustmentBasis) {
		return false
	}
	if strings.TrimSpace(bar.Source) == "" || strings.EqualFold(strings.TrimSpace(bar.Source), "unknown") {
		return false
	}
	if strings.TrimSpace(bar.NormalizationVersion) == "" {
		return false
	}
	if bar.QualityStatus != "valid" && bar.QualityStatus != "degraded" {
		return false
	}
	if containsField(bar.MissingFields, "open") || containsField(bar.MissingFields, "high") || containsField(bar.MissingFields, "low") || containsField(bar.MissingFields, "close") {
		return false
	}
	if !isFinite(bar.Close) || bar.Close <= 0 {
		return false
	}
	return true
}

func knownAdjustmentBasis(basis string) bool {
	switch basis {
	case "none", "forward_adjusted", "backward_adjusted":
		return true
	default:
		return false
	}
}
