// Package marketdata 将 Data 的公开 Application Port 适配为旧 Analysis Runner 所需的数据采集端口。
package marketdata

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	analysisapp "example.com/stock-analysis-flow/internal/analysis/app"
	"example.com/stock-analysis-flow/internal/analysis/domain"
	dataapp "example.com/stock-analysis-flow/internal/marketdata/app"
)

// AnalysisCollector 只依赖 Data 的公开 Application Port，不访问 Provider、Repository 或 HTTP API。
type AnalysisCollector struct {
	reader        dataapp.MarketDataReader
	historyLimit  int
	adjustment    dataapp.AdjustmentBasis
	maxConcurrent int
	indexSymbols  []string
}

func NewAnalysisCollector(reader dataapp.MarketDataReader, historyLimit int, adjustment dataapp.AdjustmentBasis, indexSymbols []string) (*AnalysisCollector, error) {
	if reader == nil {
		return nil, errors.New("Market Data Application Port is required")
	}
	if historyLimit < 21 || historyLimit > 2000 {
		return nil, errors.New("Market Data history limit must be in 21..2000")
	}
	switch adjustment {
	case dataapp.AdjustmentNone, dataapp.AdjustmentForward, dataapp.AdjustmentBackward:
	default:
		return nil, fmt.Errorf("unsupported Market Data adjustment basis %q", adjustment)
	}
	symbols := append([]string(nil), indexSymbols...)
	if len(symbols) == 0 {
		symbols = []string{"000001.SH"}
	}
	return &AnalysisCollector{
		reader: reader, historyLimit: historyLimit, adjustment: adjustment,
		maxConcurrent: 4, indexSymbols: uniqueIndexSymbols(symbols),
	}, nil
}

var _ analysisapp.DataCollector = (*AnalysisCollector)(nil)

type collectResult struct {
	quote *domain.RawQuote
	bars  []domain.RawBar
	index []domain.RawBar
	notes []domain.SourceNote
	err   error
}

func (c *AnalysisCollector) Collect(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, requirements domain.RequirementSet) (domain.RawSnapshot, error) {
	if c == nil || c.reader == nil {
		return domain.RawSnapshot{}, errors.New("Market Data Application Port is unavailable")
	}
	if ctx == nil {
		return domain.RawSnapshot{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return domain.RawSnapshot{}, err
	}
	if request.AsOf.IsZero() {
		return domain.RawSnapshot{}, errors.New("analysis as_of timestamp is required")
	}

	wantHistory, wantQuote, wantIndex := requestedDatasets(requirements)
	if !wantHistory && !wantQuote && !wantIndex {
		wantHistory = true
	}

	type job struct {
		security domain.Security
		kind     string
	}
	jobs := make([]job, 0, len(universe.Securities)*2+len(c.indexSymbols))
	seenIndexSymbols := make(map[string]struct{})
	for _, security := range universe.Securities {
		isIndex := isIndexSecurity(security)
		if isIndex {
			if wantHistory || wantIndex {
				jobs = append(jobs, job{security: security, kind: "index"})
				seenIndexSymbols[strings.ToUpper(security.Symbol)] = struct{}{}
			}
			continue
		}
		if wantHistory {
			jobs = append(jobs, job{security: security, kind: "history"})
		}
		if wantQuote {
			jobs = append(jobs, job{security: security, kind: "quote"})
		}
	}
	if wantIndex {
		for _, symbol := range c.indexSymbols {
			if _, exists := seenIndexSymbols[strings.ToUpper(symbol)]; exists {
				continue
			}
			jobs = append(jobs, job{security: domain.Security{Symbol: symbol, AssetType: "index"}, kind: "index"})
		}
	}

	out := domain.RawSnapshot{AsOf: request.AsOf}
	if len(jobs) == 0 {
		out.SourceNotes = append(out.SourceNotes, domain.SourceNote{
			Dataset:       datasetName(wantHistory, wantQuote, wantIndex),
			QualityStatus: "unavailable",
			Diagnostic:    "analysis universe contains no securities for the requested Data capabilities",
		})
		return out, nil
	}

	results := make(chan collectResult, len(jobs))
	jobQueue := make(chan job)
	workers := c.maxConcurrent
	if workers > len(jobs) {
		workers = len(jobs)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range jobQueue {
				if err := ctx.Err(); err != nil {
					results <- collectResult{err: err}
					continue
				}
				results <- c.collectOne(ctx, request.AsOf, item.security, item.kind)
			}
		}()
	}
	go func() {
		defer close(jobQueue)
		for _, item := range jobs {
			select {
			case jobQueue <- item:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		if result.err != nil {
			if ctx.Err() != nil {
				return domain.RawSnapshot{}, ctx.Err()
			}
			continue
		}
		if result.quote != nil {
			out.Quotes = append(out.Quotes, *result.quote)
		}
		out.Bars = append(out.Bars, result.bars...)
		out.IndexBars = append(out.IndexBars, result.index...)
		out.SourceNotes = append(out.SourceNotes, result.notes...)
	}
	if err := ctx.Err(); err != nil {
		return domain.RawSnapshot{}, err
	}
	sort.Slice(out.Quotes, func(i, j int) bool {
		return out.Quotes[i].Symbol < out.Quotes[j].Symbol
	})
	sort.Slice(out.Bars, func(i, j int) bool {
		if out.Bars[i].Symbol != out.Bars[j].Symbol {
			return out.Bars[i].Symbol < out.Bars[j].Symbol
		}
		return out.Bars[i].TradeDate.Before(out.Bars[j].TradeDate)
	})
	sort.Slice(out.IndexBars, func(i, j int) bool {
		if out.IndexBars[i].Symbol != out.IndexBars[j].Symbol {
			return out.IndexBars[i].Symbol < out.IndexBars[j].Symbol
		}
		return out.IndexBars[i].TradeDate.Before(out.IndexBars[j].TradeDate)
	})
	sort.Slice(out.SourceNotes, func(i, j int) bool {
		if out.SourceNotes[i].Dataset != out.SourceNotes[j].Dataset {
			return out.SourceNotes[i].Dataset < out.SourceNotes[j].Dataset
		}
		return out.SourceNotes[i].Symbol < out.SourceNotes[j].Symbol
	})
	return out, nil
}

func (c *AnalysisCollector) collectOne(ctx context.Context, asOf time.Time, security domain.Security, kind string) collectResult {
	switch kind {
	case "quote":
		quote, err := c.reader.GetQuote(ctx, security.Symbol)
		if err != nil {
			return collectResult{notes: []domain.SourceNote{{
				Dataset: "realtime_quote", Symbol: security.Symbol, QualityStatus: "unavailable",
				Diagnostic:    "Data Application Port quote read failed",
				MissingFields: []string{"current_price"},
			}}}
		}
		if quote.CurrentPrice == nil {
			return collectResult{notes: []domain.SourceNote{{
				Dataset: "realtime_quote", Symbol: security.Symbol, Source: quote.Source,
				FetchedAt: quote.FetchedAt, QualityStatus: analysisQualityStatus(quote.QualityStatus),
				Diagnostic:    strings.Join(quote.QualityReasons, "; "),
				MissingFields: []string{"current_price"},
			}}}
		}
		if quote.QuoteTime == nil {
			return collectResult{notes: []domain.SourceNote{{
				Dataset: "realtime_quote", Symbol: security.Symbol, Source: quote.Source,
				FetchedAt: quote.FetchedAt, QualityStatus: "unavailable",
				Diagnostic:    "Data quote_time is missing; the quote cannot be aligned to analysis as_of",
				MissingFields: []string{"quote_time"},
			}}}
		}
		if quote.QuoteTime.After(asOf) {
			return collectResult{notes: []domain.SourceNote{{
				Dataset: "realtime_quote", Symbol: security.Symbol, Source: quote.Source,
				FetchedAt: quote.FetchedAt, QualityStatus: "unavailable",
				Diagnostic:    "Data quote_time is later than the requested analysis as_of",
				MissingFields: []string{"quote_at_or_before_as_of"},
			}}}
		}
		return collectResult{quote: &domain.RawQuote{
			Symbol: quote.Symbol, Name: quote.Name, CurrentPrice: *quote.CurrentPrice,
			Change: cloneFloat(quote.Change), ChangePercent: cloneFloat(quote.ChangePercent),
			Open: cloneFloat(quote.Open), High: cloneFloat(quote.High), Low: cloneFloat(quote.Low),
			PrevClose: cloneFloat(quote.PrevClose), Volume: int64ToFloat(quote.Volume),
			Amount: cloneFloat(quote.Amount), QuoteTime: cloneTime(quote.QuoteTime),
			Source: quote.Source, FetchedAt: quote.FetchedAt,
		}, notes: []domain.SourceNote{{
			Dataset: "realtime_quote", Symbol: quote.Symbol, Source: quote.Source,
			FetchedAt: quote.FetchedAt, QualityStatus: analysisQualityStatus(quote.QualityStatus),
			Diagnostic: strings.Join(quote.QualityReasons, "; "),
		}}}
	case "history", "index":
		snapshot, err := c.reader.GetDailyBars(ctx, dataapp.DailyBarsQuery{
			Symbol: security.Symbol, EndDate: asOf.Format("2006-01-02"),
			Limit: c.historyLimit, AdjustmentBasis: c.adjustment,
		})
		dataset := "daily_bars"
		if kind == "index" {
			dataset = "index_bars"
		}
		if err != nil {
			return collectResult{notes: []domain.SourceNote{{
				Dataset: dataset, Symbol: security.Symbol, QualityStatus: "unavailable",
				Diagnostic:    "Data Application Port daily-bars read failed",
				MissingFields: []string{"daily_bars"},
			}}}
		}
		rows := make([]domain.RawBar, 0, len(snapshot.Bars))
		for _, bar := range snapshot.Bars {
			tradeDate, parseErr := time.Parse("2006-01-02", bar.TradeDate)
			if parseErr != nil || tradeDate.After(asOf) {
				continue
			}
			rows = append(rows, domain.RawBar{
				Symbol: bar.Symbol, TradeDate: tradeDate,
				Open: bar.Open, High: bar.High, Low: bar.Low, Close: bar.Close,
				Volume: int64ToFloat(bar.Volume), Amount: cloneFloat(bar.Amount),
				Source: bar.Source, FetchedAt: bar.FetchedAt,
				AdjustBasis: string(bar.AdjustmentBasis),
			})
		}
		note := domain.SourceNote{
			Dataset: dataset, Symbol: security.Symbol, Source: firstSource(snapshot.Bars),
			QualityStatus:   analysisQualityStatus(snapshot.QualityStatus),
			Diagnostic:      strings.Join(dailyBarDiagnostics(snapshot), "; "),
			AdjustmentBasis: string(snapshot.AdjustmentBasis),
		}
		if len(rows) == 0 {
			note.QualityStatus = "unavailable"
			note.MissingFields = append(note.MissingFields, "valid_daily_bars")
		}
		if kind == "index" {
			return collectResult{index: rows, notes: []domain.SourceNote{note}}
		}
		return collectResult{bars: rows, notes: []domain.SourceNote{note}}
	default:
		return collectResult{err: fmt.Errorf("unsupported Data collection job %q", kind)}
	}
}

func requestedDatasets(requirements domain.RequirementSet) (history, quote, index bool) {
	all := append([]domain.DataRequirement(nil), requirements.Core...)
	for _, items := range requirements.ByStrategy {
		all = append(all, items...)
	}
	for _, items := range requirements.ByFactor {
		all = append(all, items...)
	}
	for _, item := range all {
		switch strings.ToLower(strings.TrimSpace(item.Dataset)) {
		case "daily_bars", "daily.kline", "kline":
			history = true
		case "realtime_quote":
			quote = true
		case "index_bars", "index_daily":
			index = true
		}
	}
	return
}

func isIndexSecurity(security domain.Security) bool {
	if strings.EqualFold(strings.TrimSpace(security.AssetType), "index") ||
		strings.EqualFold(strings.TrimSpace(security.AssetType), "benchmark") {
		return true
	}
	for key, value := range security.Attributes {
		if strings.EqualFold(key, "is_index") && strings.EqualFold(strings.TrimSpace(value), "true") {
			return true
		}
		if strings.EqualFold(key, "instrument_type") && strings.EqualFold(strings.TrimSpace(value), "index") {
			return true
		}
	}
	return false
}

func datasetName(history, quote, index bool) string {
	parts := make([]string, 0, 3)
	if history {
		parts = append(parts, "daily_bars")
	}
	if quote {
		parts = append(parts, "realtime_quote")
	}
	if index {
		parts = append(parts, "index_bars")
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ",")
}

func analysisQualityStatus(status dataapp.QualityStatus) string {
	switch status {
	case dataapp.QualityValid:
		return "valid"
	case dataapp.QualityInvalid:
		return "unavailable"
	default:
		return "partial_metadata"
	}
}

func firstSource(bars []dataapp.DailyBar) string {
	for _, bar := range bars {
		if strings.TrimSpace(bar.Source) != "" {
			return bar.Source
		}
	}
	return ""
}

func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func int64ToFloat(value *int64) *float64 {
	if value == nil {
		return nil
	}
	converted := float64(*value)
	return &converted
}

func dailyBarDiagnostics(snapshot dataapp.DailyBarsSnapshot) []string {
	items := append([]string(nil), snapshot.Warnings...)
	for _, bar := range snapshot.Bars {
		items = append(items, bar.QualityReasons...)
	}
	return items
}

func uniqueIndexSymbols(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		symbol := strings.TrimSpace(value)
		if symbol == "" {
			continue
		}
		key := strings.ToUpper(symbol)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, symbol)
	}
	if len(result) == 0 {
		return []string{"000001.SH"}
	}
	return result
}
