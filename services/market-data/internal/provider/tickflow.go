package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const tickflowVersion = "tickflow-rest-v1"

type tickflowIndexDefinition struct {
	Symbol string
	Code   string
	Name   string
}

var tickflowCoreIndexDefinitions = []tickflowIndexDefinition{
	{Symbol: "000001.SH", Code: "000001", Name: "上证指数"},
	{Symbol: "399001.SZ", Code: "399001", Name: "深证成指"},
	{Symbol: "399006.SZ", Code: "399006", Name: "创业板指"},
	{Symbol: "000688.SH", Code: "000688", Name: "科创50"},
	{Symbol: "000016.SH", Code: "000016", Name: "上证50"},
	{Symbol: "000300.SH", Code: "000300", Name: "沪深300"},
}


type TickFlow struct {
	client       *http.Client
	apiURL       string
	freeAPIURL   string
	apiKey       string
	rateMu       sync.Mutex
	lastFreeCall time.Time
}

type tickflowQuotePayload struct {
	Symbol    string                     `json:"symbol"`
	Name      string                     `json:"name"`
	LastPrice *float64                   `json:"last_price"`
	PrevClose *float64                   `json:"prev_close"`
	Open      *float64                   `json:"open"`
	High      *float64                   `json:"high"`
	Low       *float64                   `json:"low"`
	Volume    *float64                   `json:"volume"`
	Amount    *float64                   `json:"amount"`
	Timestamp int64                      `json:"timestamp"`
	Ext       map[string]json.RawMessage `json:"ext"`
}

func NewTickFlowConfigured(timeout time.Duration, apiURL, freeAPIURL, apiKey string) *TickFlow {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if strings.TrimSpace(apiURL) == "" {
		apiURL = "https://api.tickflow.org"
	}
	if strings.TrimSpace(freeAPIURL) == "" {
		freeAPIURL = "https://free-api.tickflow.org"
	}
	return &TickFlow{
		client:     &http.Client{Timeout: timeout},
		apiURL:     strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		freeAPIURL: strings.TrimRight(strings.TrimSpace(freeAPIURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
	}
}

func (p *TickFlow) Name() string { return "tickflow" }

func (p *TickFlow) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	if p.apiKey == "" {
		return nil, errors.New("TickFlow real-time quotes require TICKFLOW_API_KEY")
	}
	symbol, err := tickflowDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := p.getJSON(ctx, p.apiURL, "/v1/quotes", url.Values{"symbols": []string{symbol}}, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow quote request: %w", err)
	}
	data, err := tickflowResponseData(raw)
	if err != nil {
		return nil, err
	}
	var quotes []tickflowQuotePayload
	if err := json.Unmarshal(data, &quotes); err != nil {
		return nil, fmt.Errorf("decode TickFlow quote data: %w", err)
	}
	var q *tickflowQuotePayload
	for i := range quotes {
		if strings.EqualFold(strings.TrimSpace(quotes[i].Symbol), symbol) {
			q = &quotes[i]
			break
		}
	}
	if q == nil {
		return nil, fmt.Errorf("TickFlow quote response did not contain requested symbol %s", symbol)
	}
	if q.LastPrice == nil || !tickflowPositiveFinite(*q.LastPrice) {
		return nil, errors.New("TickFlow quote has missing or invalid last_price")
	}
	if q.PrevClose == nil || !tickflowPositiveFinite(*q.PrevClose) {
		return nil, errors.New("TickFlow quote has missing or invalid prev_close")
	}
	if q.Open == nil || q.High == nil || q.Low == nil ||
		!tickflowPositiveFinite(*q.Open) || !tickflowPositiveFinite(*q.High) || !tickflowPositiveFinite(*q.Low) ||
		*q.Low > *q.High || *q.Open < *q.Low || *q.Open > *q.High || *q.LastPrice < *q.Low || *q.LastPrice > *q.High {
		return nil, errors.New("TickFlow quote violates OHLC invariants")
	}
	if q.Volume == nil || math.IsNaN(*q.Volume) || math.IsInf(*q.Volume, 0) || *q.Volume < 0 || math.Trunc(*q.Volume) != *q.Volume {
		return nil, errors.New("TickFlow quote has invalid volume")
	}
	volume, err := normalize.RawSharesToInt64(*q.Volume)
	if err != nil {
		return nil, fmt.Errorf("normalize TickFlow quote volume in shares: %w", err)
	}
	change := *q.LastPrice - *q.PrevClose
	changePercent := change / *q.PrevClose * 100
	now := time.Now().UTC()
	var quoteTime *time.Time
	quality := model.QualityValid
	var reasons []string
	if value := tickflowProviderQuoteTime(q.Timestamp, now); value != nil {
		quoteTime = value
	} else {
		quality = model.QualityDegraded
		reasons = append(reasons, "provider_quote_timestamp_missing_or_invalid")
	}
	var amount, rawAmount *float64
	amountUnit, rawAmountUnit, amountField := "unknown", "", ""
	if q.Amount != nil && !math.IsNaN(*q.Amount) && !math.IsInf(*q.Amount, 0) && *q.Amount >= 0 {
		value := *q.Amount
		amount, rawAmount = &value, &value
		amountUnit, rawAmountUnit, amountField = "CNY", "CNY", "amount"
	} else {
		quality = model.QualityDegraded
		reasons = append(reasons, "amount_missing_from_provider", "amount_unit_unknown_because_amount_missing")
	}
	rawVolume := *q.Volume
	var turnover *float64
	if raw, ok := q.Ext["turnover_rate"]; ok {
		if value, parseErr := tickflowFloat(raw); parseErr == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			value *= 100
			turnover = &value
		}
	}
	return &model.Quote{
		Symbol: symbol, Name: strings.TrimSpace(q.Name),
		CurrentPrice: q.LastPrice, Change: &change, ChangePercent: &changePercent,
		Open: q.Open, High: q.High, Low: q.Low, PrevClose: q.PrevClose,
		Volume: &volume, Amount: amount, TurnoverRate: turnover,
		QuoteTime: quoteTime, QuoteTimeSemantics: "provider_snapshot_time",
		FetchedAt: now, NormalizedAt: now, Source: p.Name(), SourceVersion: tickflowVersion,
		RawVolume: &rawVolume, RawVolumeUnit: "shares",
		RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: amountField,
		VolumeUnit: "shares", AmountUnit: amountUnit,
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: quality, QualityReasons: reasons,
		NormalizationVer: normalize.Version,
	}, nil
}

func (p *TickFlow) FetchIndexQuotes(ctx context.Context) (*model.IndexQuoteSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	if p.apiKey == "" {
		return nil, errors.New("TickFlow index quotes require TICKFLOW_API_KEY")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	symbols := make([]string, 0, len(tickflowCoreIndexDefinitions))
	definitions := make(map[string]tickflowIndexDefinition, len(tickflowCoreIndexDefinitions))
	for _, definition := range tickflowCoreIndexDefinitions {
		symbols = append(symbols, definition.Symbol)
		definitions[definition.Symbol] = definition
	}
	raw, err := p.getJSON(ctx, p.apiURL, "/v1/quotes", url.Values{"symbols": []string{strings.Join(symbols, ",")}}, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow index quote request: %w", err)
	}
	data, err := tickflowResponseData(raw)
	if err != nil {
		return nil, err
	}
	var quotes []tickflowQuotePayload
	if err := json.Unmarshal(data, &quotes); err != nil {
		return nil, fmt.Errorf("decode TickFlow index quote data: %w", err)
	}

	fetchedAt := time.Now().UTC()
	snapshot := &model.IndexQuoteSnapshot{Source: p.Name(), SourceVersion: tickflowVersion + "-index-quotes-v1", FetchedAt: fetchedAt, QualityStatus: model.QualityValid, Items: make([]model.MarketIndexQuote, 0, len(tickflowCoreIndexDefinitions))}
	markSnapshotDegraded := func(reason string) {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = appendTickflowUniqueReason(snapshot.QualityReasons, reason)
	}
	quotesBySymbol := make(map[string]tickflowQuotePayload, len(quotes))
	for _, quote := range quotes {
		symbol := strings.ToUpper(strings.TrimSpace(quote.Symbol))
		if _, expected := definitions[symbol]; !expected {
			markSnapshotDegraded("provider_returned_unrequested_index_ignored")
			continue
		}
		if _, duplicate := quotesBySymbol[symbol]; duplicate {
			markSnapshotDegraded("duplicate_index_row_dropped")
			continue
		}
		quotesBySymbol[symbol] = quote
	}

	for _, definition := range tickflowCoreIndexDefinitions {
		quote, exists := quotesBySymbol[definition.Symbol]
		if !exists {
			markSnapshotDegraded("provider_core_index_rows_missing")
			continue
		}
		if quote.LastPrice == nil || !tickflowPositiveFinite(*quote.LastPrice) || quote.PrevClose == nil || !tickflowPositiveFinite(*quote.PrevClose) {
			markSnapshotDegraded("provider_index_required_price_field_missing_or_invalid")
			continue
		}
		change := *quote.LastPrice - *quote.PrevClose
		if rawChange, ok := quote.Ext["change_amount"]; ok {
			if value, parseErr := tickflowFloat(rawChange); parseErr == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
				change = value
			}
		}
		changePercent := change / *quote.PrevClose * 100
		if rawChangePercent, ok := quote.Ext["change_pct"]; ok {
			if value, parseErr := tickflowFloat(rawChangePercent); parseErr == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
				changePercent = value * 100
			}
		}

		reasons := []string{}
		var quoteTime *time.Time
		quoteTimeSemantics := ""
		if value := tickflowProviderQuoteTime(quote.Timestamp, fetchedAt); value != nil {
			quoteTime = value
			quoteTimeSemantics = "provider_snapshot_time"
		}
		if quoteTime == nil {
			reasons = append(reasons, "provider_quote_timestamp_missing_or_invalid")
		}
		var amount *float64
		amountUnit := "unknown"
		if quote.Amount != nil && tickflowNonNegativeFinite(*quote.Amount) {
			amount, amountUnit = quote.Amount, "CNY"
		} else {
			reasons = append(reasons, "provider_amount_missing_or_invalid")
		}
		if quote.Open == nil || quote.High == nil || quote.Low == nil {
			reasons = append(reasons, "provider_index_ohlc_missing")
		} else if !tickflowPositiveFinite(*quote.Open) || !tickflowPositiveFinite(*quote.High) ||
			!tickflowPositiveFinite(*quote.Low) || *quote.Low > *quote.High ||
			*quote.Open < *quote.Low || *quote.Open > *quote.High ||
			*quote.LastPrice < *quote.Low || *quote.LastPrice > *quote.High {
			return nil, fmt.Errorf("TickFlow index %s has invalid OHLC values", definition.Symbol)
		}

		item := model.MarketIndexQuote{
			Symbol: definition.Symbol, Code: definition.Code, Name: definition.Name,
			LatestPrice: quote.LastPrice, Change: &change, ChangePercent: &changePercent,
			Open: quote.Open, High: quote.High, Low: quote.Low, PrevClose: quote.PrevClose,
			Amount: amount, AmountUnit: amountUnit, QuoteTime: quoteTime, QuoteTimeSemantics: quoteTimeSemantics,
			Source: p.Name(), SourceVersion: snapshot.SourceVersion, FetchedAt: fetchedAt,
			QualityStatus: model.QualityValid, QualityReasons: reasons,
		}
		if len(reasons) > 0 {
			item.QualityStatus = model.QualityDegraded
			markSnapshotDegraded("one_or_more_index_rows_degraded")
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if len(snapshot.Items) == 0 {
		return nil, errors.New("TickFlow index quote rows failed index-code and price validation")
	}
	return snapshot, nil
}


func (p *TickFlow) FetchMarketBreadth(ctx context.Context) (*model.MarketBreadthSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	if p.apiKey == "" {
		return nil, errors.New("TickFlow market breadth requires TICKFLOW_API_KEY")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 先读取上游证券池目录，不能使用硬编码股票数量推断全市场覆盖。
	universeRaw, err := p.getJSON(ctx, p.apiURL, "/v1/universes/CN_Equity_A", nil, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow A-share universe request: %w", err)
	}
	universeData, err := tickflowResponseData(universeRaw)
	if err != nil {
		return nil, err
	}
	var universe struct {
		ID          string   `json:"id"`
		SymbolCount int      `json:"symbol_count"`
		Symbols     []string `json:"symbols"`
	}
	if err := json.Unmarshal(universeData, &universe); err != nil {
		return nil, fmt.Errorf("decode TickFlow A-share universe: %w", err)
	}
	if universe.ID != "CN_Equity_A" || universe.SymbolCount <= 0 || len(universe.Symbols) != universe.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share universe metadata is incomplete: id=%q symbol_count=%d symbols=%d", universe.ID, universe.SymbolCount, len(universe.Symbols))
	}
	// 防止异常配置/响应造成超大快照聚合；正常 A 股全集远低于该上限。
	const maxUniverseSecurities = 20000
	if universe.SymbolCount > maxUniverseSecurities {
		return nil, fmt.Errorf("TickFlow A-share universe symbol_count=%d exceeds safety limit %d", universe.SymbolCount, maxUniverseSecurities)
	}
	symbols := make([]string, 0, len(universe.Symbols))
	expected := make(map[string]struct{}, len(universe.Symbols))
	for _, rawSymbol := range universe.Symbols {
		symbol, normalizeErr := tickflowDomesticSymbol(rawSymbol)
		if normalizeErr != nil {
			return nil, fmt.Errorf("TickFlow A-share universe contains an unsupported symbol %q: %w", rawSymbol, normalizeErr)
		}
		if _, duplicate := expected[symbol]; duplicate {
			return nil, fmt.Errorf("TickFlow A-share universe contains duplicate symbol %s", symbol)
		}
		expected[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	if len(expected) != universe.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share universe normalized symbol count mismatch: expected=%d actual=%d", universe.SymbolCount, len(expected))
	}

	// 全市场实时行情需授权 Key；免费日 K endpoint 不提供此能力。
	quotesRaw, err := p.getJSON(ctx, p.apiURL, "/v1/quotes", url.Values{"universes": []string{"CN_Equity_A"}}, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow A-share universe quotes request: %w", err)
	}
	quotesData, err := tickflowResponseData(quotesRaw)
	if err != nil {
		return nil, err
	}
	var quotes []tickflowQuotePayload
	if err := json.Unmarshal(quotesData, &quotes); err != nil {
		return nil, fmt.Errorf("decode TickFlow A-share universe quotes: %w", err)
	}
	if len(quotes) != universe.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share quotes coverage mismatch: returned=%d universe=%d", len(quotes), universe.SymbolCount)
	}
	quotesBySymbol := make(map[string]tickflowQuotePayload, len(quotes))
	for _, quote := range quotes {
		symbol, normalizeErr := tickflowDomesticSymbol(quote.Symbol)
		if normalizeErr != nil {
			return nil, fmt.Errorf("TickFlow A-share quote has an unsupported symbol %q: %w", quote.Symbol, normalizeErr)
		}
		if _, ok := expected[symbol]; !ok {
			return nil, fmt.Errorf("TickFlow returned quote for symbol %s outside CN_Equity_A", symbol)
		}
		if _, duplicate := quotesBySymbol[symbol]; duplicate {
			return nil, fmt.Errorf("TickFlow returned duplicate A-share quote for %s", symbol)
		}
		quotesBySymbol[symbol] = quote
	}
	if len(quotesBySymbol) != universe.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share unique quote coverage mismatch: returned=%d universe=%d", len(quotesBySymbol), universe.SymbolCount)
	}
	for _, symbol := range symbols {
		if _, ok := quotesBySymbol[symbol]; !ok {
			return nil, fmt.Errorf("TickFlow A-share quote is missing universe member %s", symbol)
		}
	}

	fetchedAt := time.Now().UTC()
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	snapshot := &model.MarketBreadthSnapshot{
		Market: "cn_a_share", TotalSecurities: universe.SymbolCount, CoveredSecurities: universe.SymbolCount,
		Source: p.Name(), SourceVersion: tickflowVersion + "-market-breadth-v1",
		FetchedAt: fetchedAt, QualityStatus: model.QualityValid,
	}
	degrade := func(reason string) {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = appendTickflowUniqueReason(snapshot.QualityReasons, reason)
	}
	amountTotal := 0.0
	amountCoverage := 0
	reportDate := ""
	limitUpCount, limitDownCount := 0, 0
	limitCountsComplete := true

	for _, symbol := range symbols {
		quote := quotesBySymbol[symbol]
		quoteTime := tickflowProviderQuoteTime(quote.Timestamp, fetchedAt)
		if quoteTime == nil {
			degrade("provider_quote_timestamp_missing_or_invalid")
		} else {
			date := quoteTime.In(location).Format("2006-01-02")
			if reportDate == "" {
				reportDate = date
			} else if reportDate != date {
				return nil, fmt.Errorf("TickFlow A-share quotes span multiple report dates: %s and %s", reportDate, date)
			}
		}

		priceValid := quote.LastPrice != nil && quote.PrevClose != nil &&
			tickflowPositiveFinite(*quote.LastPrice) && tickflowPositiveFinite(*quote.PrevClose)
		if !priceValid {
			snapshot.UnclassifiedCount++
			degrade("provider_required_price_missing_or_invalid")
			limitCountsComplete = false
		} else {
			switch {
			case *quote.LastPrice > *quote.PrevClose:
				snapshot.AdvancingCount++
			case *quote.LastPrice < *quote.PrevClose:
				snapshot.DecliningCount++
			default:
				snapshot.UnchangedCount++
			}
		}

		if quote.Amount != nil && tickflowNonNegativeFinite(*quote.Amount) {
			amountTotal += *quote.Amount
			if math.IsNaN(amountTotal) || math.IsInf(amountTotal, 0) {
				return nil, errors.New("TickFlow A-share total amount overflow")
			}
			amountCoverage++
		} else {
			degrade("provider_amount_missing_or_invalid")
		}

		// TickFlow Quote 扩展字段不保证带涨跌停价，按原 Python 适配器的 A 股规则推导。
		// 主板证券若缺少名称，就无法判断 ST 状态；此时不推测涨跌停比例。
		if !priceValid {
			limitCountsComplete = false
			continue
		}
		name := strings.TrimSpace(quote.Name)
		if name == "" {
			if rawName, exists := quote.Ext["name"]; exists {
				_ = json.Unmarshal(rawName, &name)
				name = strings.TrimSpace(name)
			}
		}
		ratio, ratioKnown := tickflowLimitRatio(symbol, name)
		if !ratioKnown {
			limitCountsComplete = false
			continue
		}
		limitUp, limitUpTolerance, validLimitUp := tickflowLimitPrice(*quote.PrevClose, ratio, true)
		limitDown, limitDownTolerance, validLimitDown := tickflowLimitPrice(*quote.PrevClose, ratio, false)
		if !validLimitUp || !validLimitDown {
			limitCountsComplete = false
			continue
		}
		if math.Abs(*quote.LastPrice-limitUp) <= limitUpTolerance {
			limitUpCount++
		}
		if math.Abs(*quote.LastPrice-limitDown) <= limitDownTolerance {
			limitDownCount++
		}
	}
	if reportDate == "" {
		return nil, errors.New("TickFlow A-share quotes contain no valid provider timestamp for a report date")
	}
	if !limitCountsComplete {
		degrade("provider_limit_counts_unavailable_or_incomplete")
	}
	snapshot.ProviderReportDate = reportDate
	snapshot.AmountCoverageCount = amountCoverage
	if amountCoverage == snapshot.CoveredSecurities {
		snapshot.TotalAmountCNY = &amountTotal
	} else {
		// 金额有缺失时不把部分累加值暴露为全市场成交额。
		snapshot.TotalAmountCNY = nil
	}
	if limitCountsComplete {
		snapshot.LimitUpCount = &limitUpCount
		snapshot.LimitDownCount = &limitDownCount
	}
	return snapshot, nil
}


func (p *TickFlow) FetchSectorRankings(ctx context.Context, rawCategory string, limit int) (*model.SectorRankingSnapshot, error) {
	category := strings.ToLower(strings.TrimSpace(rawCategory))
	if category != "industry" {
		return nil, fmt.Errorf("TickFlow supports only SW1 industry rankings, got category %q", rawCategory)
	}
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	if p.apiKey == "" {
		return nil, errors.New("TickFlow SW1 sector rankings require TICKFLOW_API_KEY")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		return nil, errors.New("TickFlow SW1 sector ranking limit must not exceed 100")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// 从官方证券池目录发现 SW1 行业分类，避免硬编码行业列表。
	universeListRaw, err := p.getJSON(ctx, p.apiURL, "/v1/universes", nil, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow universe-list request: %w", err)
	}
	universeListData, err := tickflowResponseData(universeListRaw)
	if err != nil {
		return nil, err
	}
	var universeSummaries []struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		SymbolCount int    `json:"symbol_count"`
	}
	if err := json.Unmarshal(universeListData, &universeSummaries); err != nil {
		return nil, fmt.Errorf("decode TickFlow universe list: %w", err)
	}
	summaries := make(map[string]struct {
		ID          string
		Name        string
		SymbolCount int
	}, len(universeSummaries))
	sw1IDs := make([]string, 0, 40)
	for _, item := range universeSummaries {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		if _, duplicate := summaries[id]; duplicate {
			return nil, fmt.Errorf("TickFlow universe list contains duplicate ID %s", id)
		}
		summaries[id] = struct {
			ID          string
			Name        string
			SymbolCount int
		}{ID: id, Name: strings.TrimSpace(item.Name), SymbolCount: item.SymbolCount}
		if strings.HasPrefix(id, "CN_Equity_SW1_") && strings.HasPrefix(strings.TrimSpace(item.Name), "SW1") {
			sw1IDs = append(sw1IDs, id)
		}
	}
	if len(sw1IDs) == 0 {
		return nil, errors.New("TickFlow returned no CN_Equity_SW1_* universes")
	}
	sort.Strings(sw1IDs)

	// 读取 A 股全集作为证券代码白名单，避免把海外证券或指数混入行业收益率。
	universeRaw, err := p.getJSON(ctx, p.apiURL, "/v1/universes/CN_Equity_A", nil, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow A-share universe request: %w", err)
	}
	universeData, err := tickflowResponseData(universeRaw)
	if err != nil {
		return nil, err
	}
	var shareUniverse struct {
		ID          string   `json:"id"`
		SymbolCount int      `json:"symbol_count"`
		Symbols     []string `json:"symbols"`
	}
	if err := json.Unmarshal(universeData, &shareUniverse); err != nil {
		return nil, fmt.Errorf("decode TickFlow A-share universe: %w", err)
	}
	if shareUniverse.ID != "CN_Equity_A" || shareUniverse.SymbolCount <= 0 ||
		len(shareUniverse.Symbols) != shareUniverse.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share universe metadata is incomplete: id=%q count=%d symbols=%d",
			shareUniverse.ID, shareUniverse.SymbolCount, len(shareUniverse.Symbols))
	}
	const maxUniverseSecurities = 20000
	if shareUniverse.SymbolCount > maxUniverseSecurities {
		return nil, fmt.Errorf("TickFlow A-share universe count %d exceeds safety limit %d",
			shareUniverse.SymbolCount, maxUniverseSecurities)
	}
	expectedSymbols := make(map[string]struct{}, len(shareUniverse.Symbols))
	for _, rawSymbol := range shareUniverse.Symbols {
		symbol, normalizeErr := tickflowDomesticSymbol(rawSymbol)
		if normalizeErr != nil {
			return nil, fmt.Errorf("TickFlow A-share universe contains unsupported symbol %q: %w", rawSymbol, normalizeErr)
		}
		if _, duplicate := expectedSymbols[symbol]; duplicate {
			return nil, fmt.Errorf("TickFlow A-share universe contains duplicate symbol %s", symbol)
		}
		expectedSymbols[symbol] = struct{}{}
	}
	if len(expectedSymbols) != shareUniverse.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share universe unique count mismatch: expected=%d actual=%d",
			shareUniverse.SymbolCount, len(expectedSymbols))
	}

	batchRaw, err := p.postJSON(ctx, p.apiURL, "/v1/universes/batch", map[string]any{"ids": sw1IDs}, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow SW1 universe-batch request: %w", err)
	}
	batchData, err := tickflowResponseData(batchRaw)
	if err != nil {
		return nil, err
	}
	var details map[string]struct {
		ID          string   `json:"id"`
		SymbolCount int      `json:"symbol_count"`
		Symbols     []string `json:"symbols"`
	}
	if err := json.Unmarshal(batchData, &details); err != nil {
		return nil, fmt.Errorf("decode TickFlow SW1 universe details: %w", err)
	}
	if len(details) != len(sw1IDs) {
		return nil, fmt.Errorf("TickFlow SW1 detail coverage mismatch: returned=%d requested=%d", len(details), len(sw1IDs))
	}

	industrySymbols := make(map[string]map[string]struct{}, len(sw1IDs))
	industryIDs := make(map[string]string, len(sw1IDs))
	symbolIndustry := make(map[string]string, shareUniverse.SymbolCount)
	for _, id := range sw1IDs {
		summary := summaries[id]
		detail, ok := details[id]
		if !ok {
			return nil, fmt.Errorf("TickFlow SW1 universe detail is missing %s", id)
		}
		if detail.ID != "" && detail.ID != id {
			return nil, fmt.Errorf("TickFlow SW1 detail ID mismatch: requested=%s returned=%s", id, detail.ID)
		}
		if detail.SymbolCount <= 0 || len(detail.Symbols) != detail.SymbolCount {
			return nil, fmt.Errorf("TickFlow SW1 universe %s is incomplete: count=%d symbols=%d",
				id, detail.SymbolCount, len(detail.Symbols))
		}
		if summary.SymbolCount > 0 && summary.SymbolCount != detail.SymbolCount {
			return nil, fmt.Errorf("TickFlow SW1 universe %s summary/detail count mismatch: summary=%d detail=%d",
				id, summary.SymbolCount, detail.SymbolCount)
		}
		name := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(summary.Name), "SW1"))
		if name == "" {
			return nil, fmt.Errorf("TickFlow SW1 universe %s has no industry name", id)
		}
		if existingID, exists := industryIDs[name]; exists && existingID != id {
			return nil, fmt.Errorf("TickFlow SW1 universe IDs %s and %s share the same industry name %q", existingID, id, name)
		}
		if _, exists := industrySymbols[name]; !exists {
			industrySymbols[name] = make(map[string]struct{}, len(detail.Symbols))
			industryIDs[name] = id
		}
		seenIndustrySymbols := make(map[string]struct{}, len(detail.Symbols))
		for _, rawSymbol := range detail.Symbols {
			symbol, normalizeErr := tickflowDomesticSymbol(rawSymbol)
			if normalizeErr != nil {
				return nil, fmt.Errorf("TickFlow SW1 universe %s contains unsupported symbol %q: %w", id, rawSymbol, normalizeErr)
			}
			if _, duplicate := seenIndustrySymbols[symbol]; duplicate {
				return nil, fmt.Errorf("TickFlow SW1 universe %s contains duplicate constituent %s", id, symbol)
			}
			seenIndustrySymbols[symbol] = struct{}{}
			if _, allowed := expectedSymbols[symbol]; !allowed {
				return nil, fmt.Errorf("TickFlow SW1 universe %s includes symbol %s outside CN_Equity_A", id, symbol)
			}
			if previousIndustry, exists := symbolIndustry[symbol]; exists && previousIndustry != name {
				return nil, fmt.Errorf("TickFlow A-share %s belongs to multiple SW1 universes (%s, %s)",
					symbol, previousIndustry, name)
			}
			symbolIndustry[symbol] = name
			industrySymbols[name][symbol] = struct{}{}
		}
	}
	if len(industrySymbols) == 0 {
		return nil, errors.New("TickFlow SW1 universes contain no industry constituents")
	}

	quotesRaw, err := p.getJSON(ctx, p.apiURL, "/v1/quotes",
		url.Values{"universes": []string{"CN_Equity_A"}}, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow A-share universe quotes request: %w", err)
	}
	quotesData, err := tickflowResponseData(quotesRaw)
	if err != nil {
		return nil, err
	}
	var quotes []tickflowQuotePayload
	if err := json.Unmarshal(quotesData, &quotes); err != nil {
		return nil, fmt.Errorf("decode TickFlow A-share quotes: %w", err)
	}
	if len(quotes) != shareUniverse.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share quotes coverage mismatch: returned=%d universe=%d",
			len(quotes), shareUniverse.SymbolCount)
	}
	quotesBySymbol := make(map[string]tickflowQuotePayload, len(quotes))
	reportDate := ""
	fetchedAt := time.Now().UTC()
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	timestampIncomplete := false
	for _, quote := range quotes {
		symbol, normalizeErr := tickflowDomesticSymbol(quote.Symbol)
		if normalizeErr != nil {
			return nil, fmt.Errorf("TickFlow A-share quote has unsupported symbol %q: %w", quote.Symbol, normalizeErr)
		}
		if _, allowed := expectedSymbols[symbol]; !allowed {
			return nil, fmt.Errorf("TickFlow returned quote for %s outside CN_Equity_A", symbol)
		}
		if _, duplicate := quotesBySymbol[symbol]; duplicate {
			return nil, fmt.Errorf("TickFlow returned duplicate A-share quote for %s", symbol)
		}
		quotesBySymbol[symbol] = quote
		quoteTime := tickflowProviderQuoteTime(quote.Timestamp, fetchedAt)
		if quoteTime == nil {
			timestampIncomplete = true
			continue
		}
		date := quoteTime.In(location).Format("2006-01-02")
		if reportDate == "" {
			reportDate = date
		} else if date != reportDate {
			return nil, fmt.Errorf("TickFlow A-share quotes span multiple report dates: %s and %s", reportDate, date)
		}
	}
	if len(quotesBySymbol) != shareUniverse.SymbolCount {
		return nil, fmt.Errorf("TickFlow A-share unique quote coverage mismatch: returned=%d universe=%d",
			len(quotesBySymbol), shareUniverse.SymbolCount)
	}
	if reportDate == "" {
		return nil, errors.New("TickFlow A-share quotes contain no valid provider timestamp for an industry report date")
	}
	for symbol := range expectedSymbols {
		if _, ok := quotesBySymbol[symbol]; !ok {
			return nil, fmt.Errorf("TickFlow A-share quote is missing universe member %s", symbol)
		}
	}

	type industryValue struct {
		id              string
		name            string
		changeTotal     float64
		changeCount     int
		advancingCount  int
		decliningCount  int
		partialCoverage bool
	}
	industries := make([]industryValue, 0, len(industrySymbols))
	for name, symbols := range industrySymbols {
		value := industryValue{id: industryIDs[name], name: name}
		for symbol := range symbols {
			quote := quotesBySymbol[symbol]
			last, prev := quote.LastPrice, quote.PrevClose
			if last == nil || prev == nil || !tickflowPositiveFinite(*last) || !tickflowPositiveFinite(*prev) {
				value.partialCoverage = true
				continue
			}
			changePercent, hasChange := 0.0, false
			if raw, exists := quote.Ext["change_pct"]; exists {
				if parsed, parseErr := tickflowFloat(raw); parseErr == nil && !math.IsNaN(parsed) && !math.IsInf(parsed, 0) {
					changePercent, hasChange = parsed*100, true
				}
			}
			if !hasChange {
				changePercent = (*last - *prev) / *prev * 100
			}
			if math.IsNaN(changePercent) || math.IsInf(changePercent, 0) {
				value.partialCoverage = true
				continue
			}
			value.changeTotal += changePercent
			if math.IsNaN(value.changeTotal) || math.IsInf(value.changeTotal, 0) {
				return nil, fmt.Errorf("TickFlow SW1 industry %s change average overflow", name)
			}
			value.changeCount++
			switch {
			case *last > *prev:
				value.advancingCount++
			case *last < *prev:
				value.decliningCount++
			}
		}
		if value.changeCount == 0 {
			continue
		}
		industries = append(industries, value)
	}
	if len(industries) == 0 {
		return nil, errors.New("TickFlow SW1 sector rankings contain no valid constituent quote changes")
	}
	sort.Slice(industries, func(i, j int) bool {
		left := industries[i].changeTotal / float64(industries[i].changeCount)
		right := industries[j].changeTotal / float64(industries[j].changeCount)
		if left == right {
			return industries[i].name < industries[j].name
		}
		return left > right
	})
	snapshot := &model.SectorRankingSnapshot{
		Category: category, Total: len(industrySymbols), Source: p.Name(),
		SourceVersion: tickflowVersion + "-sw1-sector-rankings-v1", FetchedAt: fetchedAt,
		QualityStatus: model.QualityDegraded,
		QualityReasons: []string{
			"provider_snapshot_time_not_exposed_for_aggregated_sector",
			"sector_change_percent_is_average_of_constituent_changes",
			"sector_rankings_collected_non_atomically",
		},
		Items: make([]model.SectorRanking, 0, min(limit, len(industries))),
	}
	if timestampIncomplete {
		snapshot.QualityReasons = appendTickflowUniqueReason(snapshot.QualityReasons, "one_or_more_quote_timestamps_missing_or_invalid")
	}
	for i, item := range industries {
		if i >= limit {
			break
		}
		averageChange := item.changeTotal / float64(item.changeCount)
		reasons := []string{
			"sector_change_percent_is_average_of_constituent_changes",
			"provider_snapshot_time_not_exposed_for_aggregated_sector",
		}
		if timestampIncomplete {
			reasons = appendTickflowUniqueReason(reasons, "one_or_more_quote_timestamps_missing_or_invalid")
		}
		if item.partialCoverage {
			reasons = appendTickflowUniqueReason(reasons, "sector_constituent_quote_coverage_partial")
			snapshot.QualityReasons = appendTickflowUniqueReason(snapshot.QualityReasons, "one_or_more_industry_constituent_quotes_missing_or_invalid")
		}
		advancing, declining := item.advancingCount, item.decliningCount
		snapshot.Items = append(snapshot.Items, model.SectorRanking{
			Rank: i + 1, Code: item.id, Name: item.name, Category: category,
			ChangePercent: &averageChange, AmountUnit: "unknown",
			AdvancingCount: &advancing, DecliningCount: &declining,
			Source: p.Name(), SourceVersion: snapshot.SourceVersion, FetchedAt: fetchedAt,
			ProviderReportDate: reportDate, QualityStatus: model.QualityDegraded, QualityReasons: reasons,
		})
	}
	return snapshot, nil
}

func (p *TickFlow) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	symbol, err := tickflowDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	adjust := ""
	switch adjustment {
	case model.AdjustmentNone:
		adjust = "none"
	case model.AdjustmentForward:
		adjust = "forward"
	case model.AdjustmentBackward:
		adjust = "backward"
	default:
		return nil, fmt.Errorf("unsupported TickFlow adjustment %q", adjustment)
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil {
		return nil, fmt.Errorf("invalid TickFlow start date %q", startDate)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil {
		return nil, fmt.Errorf("invalid TickFlow end date %q", endDate)
	}
	if start.After(end) {
		return nil, errors.New("TickFlow start date must not be after end date")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if end.Format("2006-01-02") > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("TickFlow end date must not be after today in Asia/Shanghai")
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 2000 {
		return nil, errors.New("TickFlow daily-bar limit must not exceed 2000")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	baseURL, authenticated := p.freeAPIURL, false
	if p.apiKey != "" {
		baseURL, authenticated = p.apiURL, true
	} else {
		if err := p.waitForFreeRateLimit(ctx); err != nil {
			return nil, err
		}
	}
	startMS := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc).UnixMilli()
	endMS := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), loc).UnixMilli()
	params := url.Values{
		"symbol":     []string{symbol},
		"period":     []string{"1d"},
		"count":      []string{strconv.Itoa(limit)},
		"start_time": []string{strconv.FormatInt(startMS, 10)},
		"end_time":   []string{strconv.FormatInt(endMS, 10)},
		"adjust":     []string{adjust},
	}
	raw, err := p.getJSON(ctx, baseURL, "/v1/klines", params, authenticated)
	if err != nil {
		return nil, fmt.Errorf("TickFlow daily K-line request: %w", err)
	}
	dataRaw, err := tickflowResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp []json.RawMessage `json:"timestamp"`
		Open      []json.RawMessage `json:"open"`
		High      []json.RawMessage `json:"high"`
		Low       []json.RawMessage `json:"low"`
		Close     []json.RawMessage `json:"close"`
		Volume    []json.RawMessage `json:"volume"`
		Amount    []json.RawMessage `json:"amount"`
		PrevClose []json.RawMessage `json:"prev_close"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("decode TickFlow K-line data: %w", err)
	}
	n := len(data.Timestamp)
	if n == 0 {
		return nil, errors.New("TickFlow returned no daily K-line rows")
	}
	for name, length := range map[string]int{"open": len(data.Open), "high": len(data.High), "low": len(data.Low), "close": len(data.Close), "volume": len(data.Volume)} {
		if length != n {
			return nil, fmt.Errorf("TickFlow K-line %s length %d does not match timestamp length %d", name, length, n)
		}
	}
	// 可选数组可以整体缺省，但一旦提供，必须与时间戳一一对应，避免静默错配。
	for name, length := range map[string]int{"amount": len(data.Amount), "prev_close": len(data.PrevClose)} {
		if length > 0 && length != n {
			return nil, fmt.Errorf("TickFlow K-line %s length %d does not match timestamp length %d", name, length, n)
		}
	}
	fetchedAt := time.Now().UTC()
	out := make([]model.DailyBar, 0, n)
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		timestamp, err := tickflowInt64(data.Timestamp[i])
		if err != nil || timestamp <= 0 {
			return nil, fmt.Errorf("TickFlow K-line row %d has invalid timestamp", i)
		}
		tradeDate := time.UnixMilli(timestamp).In(loc).Format("2006-01-02")
		if tradeDate < start.Format("2006-01-02") || tradeDate > end.Format("2006-01-02") {
			return nil, fmt.Errorf("TickFlow K-line row %d date %s is outside requested range", i, tradeDate)
		}
		if _, duplicate := seen[tradeDate]; duplicate {
			return nil, fmt.Errorf("TickFlow returned duplicate trade date %s", tradeDate)
		}
		seen[tradeDate] = struct{}{}
		open, err := tickflowFloat(data.Open[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid open on %s", tradeDate)
		}
		high, err := tickflowFloat(data.High[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid high on %s", tradeDate)
		}
		low, err := tickflowFloat(data.Low[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid low on %s", tradeDate)
		}
		closePrice, err := tickflowFloat(data.Close[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid close on %s", tradeDate)
		}
		if !tickflowPositiveFinite(open) || !tickflowPositiveFinite(high) || !tickflowPositiveFinite(low) || !tickflowPositiveFinite(closePrice) ||
			low > high || open < low || open > high || closePrice < low || closePrice > high {
			return nil, fmt.Errorf("TickFlow OHLC invariant failed on %s", tradeDate)
		}
		rawVolume, err := tickflowFloat(data.Volume[i])
		if err != nil || rawVolume < 0 || math.Trunc(rawVolume) != rawVolume {
			return nil, fmt.Errorf("TickFlow has invalid volume on %s", tradeDate)
		}
		volume, err := normalize.RawSharesToInt64(rawVolume)
		if err != nil {
			return nil, fmt.Errorf("normalize TickFlow volume in shares on %s: %w", tradeDate, err)
		}

		var amount, rawAmount *float64
		amountUnit, rawAmountUnit, rawAmountField := "unknown", "", ""
		quality := model.QualityValid
		var reasons []string
		if i < len(data.Amount) && len(bytesTrim(data.Amount[i])) > 0 && string(bytesTrim(data.Amount[i])) != "null" {
			value, parseErr := tickflowFloat(data.Amount[i])
			if parseErr != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("TickFlow has invalid amount on %s", tradeDate)
			}
			amount, rawAmount = &value, &value
			amountUnit, rawAmountUnit, rawAmountField = "CNY", "CNY", "amount"
		} else {
			quality = model.QualityDegraded
			reasons = append(reasons, "amount_missing_from_provider", "amount_unit_unknown_because_amount_missing")
		}
		var pct *float64
		if len(data.PrevClose) > 0 {
			prev, parseErr := tickflowFloat(data.PrevClose[i])
			if parseErr == nil && tickflowPositiveFinite(prev) {
				value := (closePrice - prev) / prev * 100
				if !math.IsNaN(value) && !math.IsInf(value, 0) {
					pct = &value
				}
			}
			if pct == nil {
				quality = model.QualityDegraded
				reasons = append(reasons, "provider_change_percent_missing_or_invalid")
			}
		}
		rawVolumeCopy := rawVolume
		out = append(out, model.DailyBar{
			Symbol: symbol, TradeDate: tradeDate, Period: "1d",
			Open: open, High: high, Low: low, Close: closePrice, Volume: &volume,
			Amount: amount, ChangePercent: pct, AdjustmentBasis: adjustment,
			AdjustmentMethod: "tickflow_api_" + adjust,
			Source:           p.Name(), SourceVersion: tickflowVersion,
			FetchedAt: fetchedAt, NormalizedAt: time.Now().UTC(),
			RawVolume: &rawVolumeCopy, RawVolumeUnit: "shares",
			RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: rawAmountField,
			VolumeUnit: "shares", AmountUnit: amountUnit, QualityStatus: quality, QualityReasons: reasons,
			NormalizationVer: normalize.Version,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TradeDate < out[j].TradeDate })
	if len(data.PrevClose) == 0 {
		// 仅在排序后用前一个自然时间顺序的收盘价回算，避免上游倒序返回时算反涨跌幅。
		// 这种回算无法替代供应商原始涨跌幅，所有受影响记录明确降级。
		for i := range out {
			if i == 0 {
				out[i].QualityStatus = model.QualityDegraded
				out[i].QualityReasons = appendTickflowUniqueReason(out[i].QualityReasons, "provider_change_percent_unavailable_for_first_bar")
				continue
			}
			previousClose := out[i-1].Close
			if !tickflowPositiveFinite(previousClose) || !tickflowPositiveFinite(out[i].Close) {
				out[i].ChangePercent = nil
				out[i].QualityStatus = model.QualityDegraded
				out[i].QualityReasons = appendTickflowUniqueReason(out[i].QualityReasons, "provider_change_percent_derived_from_previous_chronological_close_unavailable")
				continue
			}
			changePercent := (out[i].Close - previousClose) / previousClose * 100
			if math.IsNaN(changePercent) || math.IsInf(changePercent, 0) {
				out[i].ChangePercent = nil
				out[i].QualityStatus = model.QualityDegraded
				out[i].QualityReasons = appendTickflowUniqueReason(out[i].QualityReasons, "provider_change_percent_derived_from_previous_chronological_close_unavailable")
				continue
			}
			out[i].ChangePercent = &changePercent
			out[i].QualityStatus = model.QualityDegraded
			out[i].QualityReasons = appendTickflowUniqueReason(out[i].QualityReasons, "provider_change_percent_derived_from_previous_chronological_close")
		}
	}
	return normalizeProviderDailyBars(symbol, start.Format("2006-01-02"), end.Format("2006-01-02"), limit, out, 0)
}

func (p *TickFlow) getJSON(ctx context.Context, baseURL, path string, query url.Values, authenticated bool) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	if authenticated && p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	return readBoundedBody(resp.Body, maxProviderPayloadBytes)
}


func (p *TickFlow) postJSON(ctx context.Context, baseURL, path string, payload any, authenticated bool) ([]byte, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("encode TickFlow request body: %w", err)
	}
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	if authenticated && p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	return readBoundedBody(resp.Body, maxProviderPayloadBytes)
}

func (p *TickFlow) waitForFreeRateLimit(ctx context.Context) error {
	p.rateMu.Lock()
	defer p.rateMu.Unlock()
	if !p.lastFreeCall.IsZero() {
		delay := time.Until(p.lastFreeCall.Add(time.Second))
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	p.lastFreeCall = time.Now()
	return nil
}

func tickflowDomesticSymbol(raw string) (string, error) {
	symbol, err := normalize.NormalizeSymbol(raw)
	if err != nil {
		return "", err
	}
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") {
		return "", fmt.Errorf("TickFlow Go adapter only accepts domestic SH/SZ/BJ securities, got %q", raw)
	}
	return symbol, nil
}

func tickflowResponseData(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		Code    *int            `json:"code"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode TickFlow response envelope: %w", err)
	}
	if envelope.Code != nil && *envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = strings.TrimSpace(envelope.Msg)
		}
		if message == "" {
			message = strings.TrimSpace(envelope.Error)
		}
		return nil, fmt.Errorf("TickFlow provider error code=%d: %s", *envelope.Code, message)
	}
	if strings.TrimSpace(envelope.Error) != "" {
		return nil, fmt.Errorf("TickFlow provider error: %s", strings.TrimSpace(envelope.Error))
	}
	if len(bytesTrim(envelope.Data)) == 0 || string(bytesTrim(envelope.Data)) == "null" {
		msg := strings.TrimSpace(envelope.Message)
		if msg == "" {
			msg = strings.TrimSpace(envelope.Msg)
		}
		return nil, fmt.Errorf("TickFlow response has no data: %s", msg)
	}
	return envelope.Data, nil
}

func tickflowFloat(raw json.RawMessage) (float64, error) {
	raw = json.RawMessage(bytesTrim(raw))
	if len(raw) == 0 || string(raw) == "null" {
		return 0, errors.New("empty TickFlow number")
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if strings.TrimSpace(s) == "" {
			return 0, errors.New("empty TickFlow number")
		}
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}
	return strconv.ParseFloat(string(raw), 64)
}

func tickflowInt64(raw json.RawMessage) (int64, error) {
	value, err := tickflowFloat(raw)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value ||
		value < -float64(math.MaxInt64) || value >= float64(math.MaxInt64) {
		return 0, errors.New("invalid TickFlow integer")
	}
	return int64(value), nil
}

func appendTickflowUniqueReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func tickflowNonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func tickflowPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func bytesTrim(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}

// tickflowProviderQuoteTime 仅接受毫秒 Unix 时间戳；拒绝明显不合理的历史值和未来值。
// 不在这里猜测“过期”阈值，避免把休市、节假日或盘后最后一次有效快照误判成错误。
func tickflowProviderQuoteTime(timestamp int64, reference time.Time) *time.Time {
	const earliestPlausibleTimestamp int64 = 946684800000 // 2000-01-01 00:00:00 UTC
	if timestamp < earliestPlausibleTimestamp || timestamp > reference.Add(5*time.Minute).UnixMilli() {
		return nil
	}
	value := time.UnixMilli(timestamp).UTC()
	return &value
}
 
// tickflowLimitRatio follows the domestic A-share limit rules used by the original Python adapter.
// Beijing-market rules take precedence, followed by STAR/ChiNext, ST, then the main board.
// Without a main-board name, ST status is unknown, so no limit ratio is inferred.
func tickflowLimitRatio(symbol, name string) (float64, bool) {
	code := strings.Split(strings.ToUpper(strings.TrimSpace(symbol)), ".")[0]
	if len(code) != 6 {
		return 0, false
	}
	for _, digit := range code {
		if digit < '0' || digit > '9' {
			return 0, false
		}
	}
	if strings.HasPrefix(code, "900") {
		return 0, false
	}
	for _, prefix := range []string{"92", "43", "81", "82", "83", "87", "88"} {
		if strings.HasPrefix(code, prefix) {
			return 0.30, true
		}
	}
	if strings.HasPrefix(code, "688") || strings.HasPrefix(code, "300") {
		return 0.20, true
	}
	name = strings.ToUpper(strings.TrimSpace(name))
	if name == "" {
		return 0, false
	}
	if strings.Contains(name, "ST") {
		return 0.05, true
	}
	return 0.10, true
}

// tickflowLimitPrice reproduces the source adapter's cent-rounding and boundary tolerance.
func tickflowLimitPrice(prevClose, ratio float64, upper bool) (float64, float64, bool) {
	if !tickflowPositiveFinite(prevClose) || !tickflowPositiveFinite(ratio) || ratio >= 1 {
		return 0, 0, false
	}
	rawPrice := prevClose * (1 - ratio)
	if upper {
		rawPrice = prevClose * (1 + ratio)
	}
	if !tickflowPositiveFinite(rawPrice) || math.IsInf(rawPrice*100, 0) {
		return 0, 0, false
	}
	roundedPrice := math.Floor(rawPrice*100+0.5) / 100
	tolerance := math.Round(math.Abs(rawPrice-roundedPrice)*1e10) / 1e10
	if !tickflowPositiveFinite(roundedPrice) || math.IsNaN(tolerance) || math.IsInf(tolerance, 0) {
		return 0, 0, false
	}
	return roundedPrice, tolerance, true
}

