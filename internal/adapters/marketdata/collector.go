package marketdata

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"example.com/stock-analysis-flow/internal/analysis/app"
	"example.com/stock-analysis-flow/internal/analysis/domain"
)

// Collector 通过原生 Go 行情 HTTP API 实现 Runner 的 DataCollector 接口。
// 适配器会保留逐条 Provider 元数据，并明确记录缺失的发布时间/可用时间，不会自行伪造。
type Collector struct {
	client         *Client
	historyDays    int
	maxConcurrency int
	indexSymbols   []string
}

func NewCollector(client *Client, cfg Config) (*Collector, error) {
	if client == nil {
		return nil, errors.New("Go market-data API client is required")
	}
	if cfg.HistoryDays < 1 || cfg.HistoryDays > 365 {
		return nil, errors.New("collector history days must be between 1 and 365")
	}
	if cfg.MaxConcurrency < 1 || cfg.MaxConcurrency > 32 {
		return nil, errors.New("collector max concurrency must be between 1 and 32")
	}
	return &Collector{
		client: client, historyDays: cfg.HistoryDays,
		maxConcurrency: cfg.MaxConcurrency, indexSymbols: append([]string(nil), cfg.IndexSymbols...),
	}, nil
}

var _ app.DataCollector = (*Collector)(nil)

func (c *Collector) Collect(ctx context.Context, request domain.RunRequest, universe domain.UniverseSnapshot, requirements domain.RequirementSet) (domain.RawSnapshot, error) {
	if ctx == nil {
		return domain.RawSnapshot{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return domain.RawSnapshot{}, err
	}
	if request.AsOf.IsZero() {
		return domain.RawSnapshot{}, errors.New("analysis as_of timestamp is required")
	}

	wantsHistory, wantsQuote, wantsIndex := requestedDatasets(requirements)
	if !wantsHistory && !wantsQuote && !wantsIndex {
		// 当前 MVP 的核心分析始终需要日 K。若调用方未提供数据需求契约，则使用文档约定的默认需求，避免静默返回空结果。
		wantsHistory = true
	}

	type fetchJob struct {
		symbol string
		kind   string
	}
	jobs := make([]fetchJob, 0, len(universe.Securities)+len(c.indexSymbols))
	seen := make(map[string]struct{})
	add := func(symbol, kind string) {
		symbol = strings.TrimSpace(symbol)
		if symbol == "" {
			return
		}
		key := strings.ToUpper(symbol) + "|" + kind
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		jobs = append(jobs, fetchJob{symbol: symbol, kind: kind})
	}
	for _, security := range universe.Securities {
		isIndex := isIndexSecurity(security)
		if isIndex {
			if wantsIndex || wantsHistory {
				add(security.Symbol, "index")
			}
			continue
		}
		if wantsHistory || wantsQuote {
			add(security.Symbol, "stock")
		}
	}
	if wantsIndex {
		for _, symbol := range c.indexSymbols {
			add(symbol, "index")
		}
	}

	snapshot := domain.RawSnapshot{AsOf: request.AsOf}
	if len(jobs) == 0 {
		snapshot.SourceNotes = append(snapshot.SourceNotes, domain.SourceNote{
			Dataset: dataSetName(wantsHistory, wantsQuote, wantsIndex), Source: defaultSourceName,
			FetchedAt: time.Now().UTC(), QualityStatus: "unavailable",
			Diagnostic: "universe contained no securities for the requested datasets",
		})
		if request.AsOfMode == domain.AsOfModeRealtimeSnapshot {
			snapshot.AsOf = time.Now().In(c.client.marketLocation)
		}
		return snapshot, nil
	}

	results := make(chan structResult, len(jobs))
	jobCh := make(chan fetchJob)
	workers := c.maxConcurrency
	if workers > len(jobs) {
		workers = len(jobs)
	}
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobCh {
				if ctx.Err() != nil {
					results <- structResult{err: ctx.Err()}
					continue
				}
				results <- c.fetchOne(ctx, request, job.symbol, job.kind, wantsHistory, wantsQuote, wantsIndex)
			}
		}()
	}
	go func() {
		defer close(jobCh)
		for _, job := range jobs {
			select {
			case jobCh <- job:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	for item := range results {
		if item.err != nil {
			if ctx.Err() != nil {
				return domain.RawSnapshot{}, ctx.Err()
			}
			// 单只股票的数据源错误会保留为诊断说明；剩余数据是否满足各策略要求，由质量门禁决定。
		}
		snapshot.Quotes = append(snapshot.Quotes, item.quotes...)
		snapshot.Bars = append(snapshot.Bars, item.bars...)
		snapshot.IndexBars = append(snapshot.IndexBars, item.index...)
		snapshot.SourceNotes = append(snapshot.SourceNotes, item.notes...)
	}
	sort.Slice(snapshot.Quotes, func(i, j int) bool {
		return snapshot.Quotes[i].Symbol < snapshot.Quotes[j].Symbol
	})
	sort.Slice(snapshot.Bars, func(i, j int) bool {
		if snapshot.Bars[i].Symbol != snapshot.Bars[j].Symbol {
			return snapshot.Bars[i].Symbol < snapshot.Bars[j].Symbol
		}
		return snapshot.Bars[i].TradeDate.Before(snapshot.Bars[j].TradeDate)
	})
	sort.Slice(snapshot.IndexBars, func(i, j int) bool {
		if snapshot.IndexBars[i].Symbol != snapshot.IndexBars[j].Symbol {
			return snapshot.IndexBars[i].Symbol < snapshot.IndexBars[j].Symbol
		}
		return snapshot.IndexBars[i].TradeDate.Before(snapshot.IndexBars[j].TradeDate)
	})
	sort.Slice(snapshot.SourceNotes, func(i, j int) bool {
		if snapshot.SourceNotes[i].Dataset != snapshot.SourceNotes[j].Dataset {
			return snapshot.SourceNotes[i].Dataset < snapshot.SourceNotes[j].Dataset
		}
		return snapshot.SourceNotes[i].Symbol < snapshot.SourceNotes[j].Symbol
	})
	if err := ctx.Err(); err != nil {
		return domain.RawSnapshot{}, err
	}
	if request.AsOfMode == domain.AsOfModeRealtimeSnapshot {
		snapshot.AsOf = time.Now().In(c.client.marketLocation)
	}
	return snapshot, nil
}

func (c *Collector) fetchOne(ctx context.Context, request domain.RunRequest, symbol, kind string, wantsHistory, wantsQuote, wantsIndex bool) (out structResult) {
	fetchedAt := time.Now().UTC()
	if (kind == "stock" && wantsHistory) || (kind == "index" && (wantsIndex || wantsHistory)) {
		result, err := c.client.GetHistory(ctx, symbol, c.historyDays)
		if err != nil {
			out.notes = append(out.notes, domain.SourceNote{
				Dataset: historyDatasetFor(kind), Symbol: symbol, Source: defaultSourceName,
				FetchedAt: time.Now().UTC(), QualityStatus: "unavailable", Diagnostic: safeDiagnostic(err),
				MissingFields: []string{"daily_bars"},
			})
		} else {
			bars, skippedFuture, invalidRows, rowMissing := c.convertBars(symbol, result, request.AsOf)
			if kind == "index" {
				out.index = append(out.index, bars...)
			} else {
				out.bars = append(out.bars, bars...)
			}
			missing := append([]string(nil), rowMissing...)
			sort.Strings(missing)
			status := "partial_metadata"
			adjustmentBasis := "unknown"
			for _, item := range result.Bars {
				if value := strings.TrimSpace(item.AdjustmentBasis); value != "" {
					adjustmentBasis = value
					break
				}
			}
			diagnostic := "Go market-data API exposes provider source and adjustment basis when available, but not exact publication-availability time."
			for _, warning := range result.Warnings {
				if warning = strings.TrimSpace(warning); warning != "" {
					diagnostic += " Provider warning: " + warning + "."
				}
			}
			if result.QualityStatus != "" && result.QualityStatus != "valid" {
				diagnostic += " Provider reported quality_status=" + result.QualityStatus + "."
			}
			if len(bars) == 0 {
				status = "unavailable"
				diagnostic = "no valid daily bars at or before request as_of"
				missing = append(missing, "valid_daily_bars")
			}
			if skippedFuture > 0 {
				diagnostic += fmt.Sprintf(" Filtered %d daily bar(s) after as_of or not yet complete at as_of.", skippedFuture)
			}
			if invalidRows > 0 {
				status = "partial_metadata"
				diagnostic += fmt.Sprintf(" Skipped %d invalid bar row(s).", invalidRows)
			}
			out.notes = append(out.notes, domain.SourceNote{
				Dataset: historyDatasetFor(kind), Symbol: symbol, Source: defaultSourceName,
				FetchedAt: result.FetchedAt, QualityStatus: status,
				Diagnostic: diagnostic, MissingFields: missing, AdjustmentBasis: adjustmentBasis,
			})
		}
	}
	if wantsQuote && kind == "stock" {
		quote, at, err := c.client.GetQuote(ctx, symbol)
		if err != nil {
			out.notes = append(out.notes, domain.SourceNote{
				Dataset: quoteDataset, Symbol: symbol, Source: defaultSourceName,
				FetchedAt: time.Now().UTC(), QualityStatus: "unavailable", Diagnostic: safeDiagnostic(err),
				MissingFields: []string{"current_price"},
			})
		} else {
			quoteTime, parseErr := parseTimestamp(quote.UpdateTime, c.client.marketLocation)
			if parseErr != nil || quoteTime == nil {
				diagnostic := "quote update_time is absent; quote discarded because its point-in-time cannot be verified"
				if parseErr != nil {
					diagnostic = "quote update_time could not be parsed; quote discarded"
				}
				out.notes = append(out.notes, domain.SourceNote{
					Dataset: quoteDataset, Symbol: symbol, Source: defaultSourceName,
					FetchedAt: at, QualityStatus: "unavailable", Diagnostic: diagnostic,
					MissingFields: []string{"valid_update_time"},
				})
			} else if request.AsOfMode == domain.AsOfModeRealtimeSnapshot &&
				quoteTime.After(time.Now().In(c.client.marketLocation)) {
				out.notes = append(out.notes, domain.SourceNote{
					Dataset: quoteDataset, Symbol: symbol, Source: defaultSourceName,
					FetchedAt: at, QualityStatus: "unavailable",
					Diagnostic:    "quote timestamp is later than collector clock; quote discarded",
					MissingFields: []string{"valid_quote_timestamp"},
				})
			} else if request.AsOfMode != domain.AsOfModeRealtimeSnapshot &&
				quoteTime.After(request.AsOf) {
				out.notes = append(out.notes, domain.SourceNote{
					Dataset: quoteDataset, Symbol: symbol, Source: defaultSourceName,
					FetchedAt: at, QualityStatus: "unavailable", Diagnostic: "quote timestamp is later than request as_of; quote discarded",
					MissingFields: []string{"quote_at_or_before_as_of"},
				})
			} else {
				sourceName := strings.TrimSpace(quote.Source)
				if sourceName == "" {
					sourceName = defaultSourceName
				}
				quoteFetchedAt := quote.FetchedAt
				if quoteFetchedAt.IsZero() {
					quoteFetchedAt = at
				}
				out.quotes = append(out.quotes, domain.RawQuote{
					Symbol: symbol, Name: quote.StockName, CurrentPrice: quote.CurrentPrice,
					Change: quote.Change, ChangePercent: quote.ChangePct, Open: quote.Open,
					High: quote.High, Low: quote.Low, PrevClose: quote.PrevClose,
					Volume: quote.Volume, Amount: quote.Amount, QuoteTime: quoteTime,
					Source: sourceName, FetchedAt: quoteFetchedAt,
				})
				missing := []string{"available_at"}
				for field, value := range map[string]*float64{
					"change": quote.Change, "change_percent": quote.ChangePct, "open": quote.Open,
					"high": quote.High, "low": quote.Low, "prev_close": quote.PrevClose,
					"volume": quote.Volume, "amount": quote.Amount,
				} {
					if value == nil {
						missing = append(missing, field)
					}
				}
				sort.Strings(missing)
				status := "partial_metadata"
				diagnostic := "Go market-data API identifies the provider, but does not expose exact publication-availability time."
				if len(quote.QualityReasons) > 0 {
					diagnostic += " Provider quality reasons: " + strings.Join(quote.QualityReasons, ", ") + "."
				}
				if quote.QualityStatus != "" && quote.QualityStatus != "valid" {
					diagnostic += " Provider reported quality_status=" + quote.QualityStatus + "."
				}
				out.notes = append(out.notes, domain.SourceNote{
					Dataset: quoteDataset, Symbol: symbol, Source: sourceName,
					FetchedAt: at, QualityStatus: status, Diagnostic: diagnostic, MissingFields: missing,
				})
			}
		}
	}
	if !wantsHistory && !wantsQuote && !wantsIndex {
		out.notes = append(out.notes, domain.SourceNote{Dataset: "none", Symbol: symbol, Source: defaultSourceName, FetchedAt: fetchedAt, QualityStatus: "unavailable", Diagnostic: "no supported dataset requested"})
	}
	return out
}

// structResult 仅在本适配器内部使用，避免将传输层细节泄漏到应用层接口。
type structResult struct {
	quotes []domain.RawQuote
	bars   []domain.RawBar
	index  []domain.RawBar
	notes  []domain.SourceNote
	err    error
}

func (c *Collector) convertBars(symbol string, result HistoryResult, asOf time.Time) ([]domain.RawBar, int, int, []string) {
	rows := make([]domain.RawBar, 0, len(result.Bars))
	seen := make(map[string]struct{}, len(result.Bars))
	// 只有日期的 K 线会按配置的市场时区解释。调用方的 RunRequest.AsOf 可能使用 UTC；
	// 显式转换市场时区可避免交易日期意外偏移。
	asOfDate := asOf.In(c.client.marketLocation).Format("2006-01-02")
	futureCount, invalidCount := 0, 0
	missingSet := make(map[string]struct{})
	for _, item := range result.Bars {
		tradeDate, err := parseTradeDate(item.Date)
		if err != nil || item.Open == nil || item.High == nil || item.Low == nil || item.Close == nil ||
			!isFinite(*item.Open) || !isFinite(*item.High) || !isFinite(*item.Low) || !isFinite(*item.Close) ||
			*item.Open <= 0 || *item.High <= 0 || *item.Low <= 0 || *item.Close <= 0 ||
			*item.High < *item.Low || *item.High < *item.Open || *item.High < *item.Close ||
			*item.Low > *item.Open || *item.Low > *item.Close ||
			(item.Volume != nil && (!isFinite(*item.Volume) || *item.Volume < 0)) ||
			(item.Amount != nil && (!isFinite(*item.Amount) || *item.Amount < 0)) {
			invalidCount++
			continue
		}
		tradeDateText := tradeDate.Format("2006-01-02")
		if tradeDateText > asOfDate ||
			(tradeDateText == asOfDate &&
				!dailyBarAvailableAtAsOf(asOf, c.client.marketLocation)) {
			futureCount++
			continue
		}
		key := tradeDate.Format("2006-01-02")
		if _, duplicate := seen[key]; duplicate {
			invalidCount++
			continue
		}
		seen[key] = struct{}{}
		missingFields := make([]string, 0, 4)
		missingFields = append(missingFields, "available_at")
		missingSet["available_at"] = struct{}{}
		sourceName := strings.TrimSpace(item.Source)
		if sourceName == "" {
			missingFields = append(missingFields, "provider_source")
			missingSet["provider_source"] = struct{}{}
		}
		adjustmentBasis := strings.TrimSpace(item.AdjustmentBasis)
		if adjustmentBasis == "" || adjustmentBasis == "unknown" {
			adjustmentBasis = "unknown"
			missingFields = append(missingFields, "adjustment_basis")
			missingSet["adjustment_basis"] = struct{}{}
		}
		if item.Volume == nil {
			missingFields = append(missingFields, "volume")
			missingSet["volume"] = struct{}{}
		}
		if item.Amount == nil {
			missingFields = append(missingFields, "amount")
			missingSet["amount"] = struct{}{}
		}
		// 成交量或成交额缺失时继续保留 nil，不能把缺失值改成 0。
		barFetchedAt := item.FetchedAt
		if barFetchedAt.IsZero() {
			barFetchedAt = result.FetchedAt
		}
		if sourceName == "" {
			sourceName = defaultSourceName
		}
		rows = append(rows, domain.RawBar{
			Symbol: symbol, TradeDate: tradeDate, Open: *item.Open, High: *item.High,
			Low: *item.Low, Close: *item.Close, Volume: item.Volume, Amount: item.Amount,
			Source: sourceName, FetchedAt: barFetchedAt,
			AdjustBasis: adjustmentBasis, MissingFields: missingFields,
		})
	}
	missingFields := make([]string, 0, len(missingSet))
	for field := range missingSet {
		missingFields = append(missingFields, field)
	}
	sort.Strings(missingFields)
	return rows, futureCount, invalidCount, missingFields
}

func requestedDatasets(requirements domain.RequirementSet) (history, quote, index bool) {
	all := make([]domain.DataRequirement, 0, len(requirements.Core))
	all = append(all, requirements.Core...)
	for _, items := range requirements.ByStrategy {
		all = append(all, items...)
	}
	for _, items := range requirements.ByFactor {
		all = append(all, items...)
	}
	for _, item := range all {
		switch strings.ToLower(strings.TrimSpace(item.Dataset)) {
		case historyDataset, "daily_bars", "daily.kline", "kline":
			history = true
		case quoteDataset, "realtime_quote":
			quote = true
		case indexDataset, "index_bars", "index_daily":
			index = true
		}
	}
	return history, quote, index
}

func isIndexSecurity(security domain.Security) bool {
	if strings.EqualFold(strings.TrimSpace(security.AssetType), "index") || strings.EqualFold(strings.TrimSpace(security.AssetType), "benchmark") {
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

func historyDatasetFor(kind string) string {
	if kind == "index" {
		return indexDataset
	}
	return historyDataset
}

func dataSetName(history, quote, index bool) string {
	parts := make([]string, 0, 3)
	if history {
		parts = append(parts, historyDataset)
	}
	if quote {
		parts = append(parts, quoteDataset)
	}
	if index {
		parts = append(parts, indexDataset)
	}
	if len(parts) == 0 {
		return "unknown"
	}
	return strings.Join(parts, ",")
}

func dailyBarAvailableAtAsOf(asOf time.Time, marketLocation *time.Location) bool {
	local := asOf.In(marketLocation)

	// A 股日 K 保守地在 15:05 后才允许作为已完成日 K 使用。
	return local.Hour() > 15 ||
		(local.Hour() == 15 && local.Minute() >= 5)
}

func safeDiagnostic(err error) string {
	if err == nil {
		return ""
	}
	// 客户端错误信息不会包含响应正文或认证信息，避免意外泄露敏感内容。
	message := strings.TrimSpace(err.Error())
	if len(message) > 400 {
		message = message[:400]
	}
	return message
}
