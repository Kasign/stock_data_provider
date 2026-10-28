package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/cache"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/repository"
)

type chipDistributionSource interface {
	provider.NamedProvider
	FetchChipDistribution(context.Context, string) (*model.ChipDistribution, error)
}

type DataService struct {
	providers                 []provider.NamedProvider
	quoteProviders            []provider.QuoteProvider
	barProviders              []provider.DailyBarProvider
	stockBasicProviders       []provider.StockBasicProvider
	stockSearchProviders      []provider.StockSearchProvider
	sectorRankingProviders    []provider.SectorRankingProvider
	indexQuoteProviders       []provider.IndexQuoteProvider
	marketBreadthProviders    []provider.MarketBreadthProvider
	limitPoolProviders        []provider.LimitPoolProvider
	dragonTigerProviders      []provider.DragonTigerProvider
	announcementProviders     []provider.AnnouncementProvider
	stockBoardsProviders      []provider.StockBoardsProvider
	tradingCalendarProviders  []provider.TradingCalendarProvider
	fundamentalsProviders     []provider.FundamentalsProvider
	valuationProviders        []provider.ValuationProvider
	indexCatalogProviders     []provider.IndexCatalogProvider
	chipDistributionProviders []provider.ChipDistributionProvider
	capitalFlowProviders      []provider.CapitalFlowProvider
	hotRankingProviders       []provider.HotRankingProvider
	supplementProviders       []provider.SupplementProvider
	maxBars                   int
	retries                   int
	breaker                   *circuitBreaker
	cache                     cache.Cache
	store                     *repository.Store
	quoteTTL                  time.Duration
	barsTTL                   time.Duration
	defaultBars               int
	logger                    *slog.Logger
}

func New(providers []provider.Provider, c cache.Cache, store *repository.Store, quoteTTL, barsTTL time.Duration, defaultBars int, logger *slog.Logger) *DataService {
	return NewConfigured(providers, providers, providers, c, store, quoteTTL, barsTTL, defaultBars, 2000, logger)
}

// NewConfigured retains source compatibility for aggregate Provider adapters.
func NewConfigured(allProviders, quoteProviders, barProviders []provider.Provider, c cache.Cache, store *repository.Store, quoteTTL, barsTTL time.Duration, defaultBars, maxBars int, logger *slog.Logger) *DataService {
	return NewConfiguredWithResilience(allProviders, quoteProviders, barProviders, c, store, quoteTTL, barsTTL, defaultBars, maxBars, ResilienceConfig{Retries: 1, CircuitFailureThreshold: 3, CircuitCooldown: 45 * time.Second}, logger)
}

// NewConfiguredWithResilience is the compatibility constructor. New code should
// use NewWithCapabilities so a provider can implement only the data types it owns.
func NewConfiguredWithResilience(allProviders, quoteProviders, barProviders []provider.Provider, c cache.Cache, store *repository.Store, quoteTTL, barsTTL time.Duration, defaultBars, maxBars int, resilience ResilienceConfig, logger *slog.Logger) *DataService {
	all := make([]provider.NamedProvider, 0, len(allProviders))
	quotes := make([]provider.QuoteProvider, 0, len(quoteProviders))
	bars := make([]provider.DailyBarProvider, 0, len(barProviders))
	for _, p := range allProviders {
		all = append(all, p)
	}
	for _, p := range quoteProviders {
		quotes = append(quotes, p)
	}
	for _, p := range barProviders {
		bars = append(bars, p)
	}
	return NewWithCapabilities(all, quotes, bars, c, store, quoteTTL, barsTTL, defaultBars, maxBars, resilience, logger)
}

// NewWithCapabilities wires independent provider capabilities. This is the
// preferred constructor for future quote-only, historical-only and other
// data-domain adapters.
func NewWithCapabilities(allProviders []provider.NamedProvider, quoteProviders []provider.QuoteProvider, barProviders []provider.DailyBarProvider, c cache.Cache, store *repository.Store, quoteTTL, barsTTL time.Duration, defaultBars, maxBars int, resilience ResilienceConfig, logger *slog.Logger) *DataService {
	resilience = resilience.normalized()
	if c == nil {
		c = cache.NewDisabled()
	}
	if logger == nil {
		logger = slog.Default()
	}
	if defaultBars <= 0 {
		defaultBars = 120
	}
	if maxBars < defaultBars {
		maxBars = defaultBars
	}
	if maxBars > 2000 {
		maxBars = 2000
	}
	return &DataService{
		providers:      append([]provider.NamedProvider(nil), allProviders...),
		quoteProviders: append([]provider.QuoteProvider(nil), quoteProviders...),
		barProviders:   append([]provider.DailyBarProvider(nil), barProviders...),
		maxBars:        maxBars, retries: resilience.Retries,
		breaker: newCircuitBreaker(resilience.CircuitFailureThreshold, resilience.CircuitCooldown),
		cache:   c, store: store, quoteTTL: quoteTTL, barsTTL: barsTTL,
		defaultBars: defaultBars, logger: logger,
	}
}

func (s *DataService) Providers() []provider.NamedProvider {
	return append([]provider.NamedProvider(nil), s.providers...)
}

func (s *DataService) SetStockBasicProviders(providers []provider.StockBasicProvider) {
	s.stockBasicProviders = append([]provider.StockBasicProvider(nil), providers...)
}

func (s *DataService) StockBasicProviders() []provider.StockBasicProvider {
	return append([]provider.StockBasicProvider(nil), s.stockBasicProviders...)
}

func (s *DataService) SetStockSearchProviders(providers []provider.StockSearchProvider) {
	s.stockSearchProviders = append([]provider.StockSearchProvider(nil), providers...)
}

func (s *DataService) StockSearchProviders() []provider.StockSearchProvider {
	return append([]provider.StockSearchProvider(nil), s.stockSearchProviders...)
}

func (s *DataService) SetSectorRankingProviders(providers []provider.SectorRankingProvider) {
	s.sectorRankingProviders = append([]provider.SectorRankingProvider(nil), providers...)
}

func (s *DataService) SectorRankingProviders() []provider.SectorRankingProvider {
	return append([]provider.SectorRankingProvider(nil), s.sectorRankingProviders...)
}

func (s *DataService) SetIndexQuoteProviders(providers []provider.IndexQuoteProvider) {
	s.indexQuoteProviders = append([]provider.IndexQuoteProvider(nil), providers...)
}
func (s *DataService) IndexQuoteProviders() []provider.IndexQuoteProvider {
	return append([]provider.IndexQuoteProvider(nil), s.indexQuoteProviders...)
}
func validateIndexQuoteSnapshot(snapshot *model.IndexQuoteSnapshot) error {
	if snapshot == nil || len(snapshot.Items) == 0 {
		return errors.New("index-quote provider returned an empty snapshot")
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("index-quote source or fetched_at is missing")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("index-quote snapshot quality %q is unsupported", snapshot.QualityStatus)
	}
	seen := make(map[string]bool, len(snapshot.Items))
	hasDegradedItem := false
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Symbol) == "" || strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" {
			return errors.New("index-quote row has missing symbol/code/name")
		}
		if item.Source != snapshot.Source || item.FetchedAt.IsZero() || item.LatestPrice == nil || item.ChangePercent == nil {
			return errors.New("index-quote row failed source, fetched_at or required-value validation")
		}
		if item.QualityStatus != model.QualityValid && item.QualityStatus != model.QualityDegraded {
			return fmt.Errorf("index-quote quality %q is unsupported", item.QualityStatus)
		}
		if *item.LatestPrice <= 0 || math.IsNaN(*item.LatestPrice) || math.IsInf(*item.LatestPrice, 0) {
			return fmt.Errorf("index %s has a non-positive or non-finite latest price", item.Symbol)
		}
		for _, value := range []*float64{item.Change, item.ChangePercent, item.Open, item.High, item.Low, item.PrevClose, item.Amount} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return fmt.Errorf("index %s contains a non-finite numeric value", item.Symbol)
			}
		}
		for _, value := range []*float64{item.Open, item.High, item.Low, item.PrevClose} {
			if value != nil && *value <= 0 {
				return fmt.Errorf("index %s contains a non-positive OHLC/previous-close value", item.Symbol)
			}
		}
		if item.Low != nil && item.High != nil && *item.Low > *item.High {
			return fmt.Errorf("index %s has low greater than high", item.Symbol)
		}
		if item.Amount == nil {
			if item.AmountUnit != "unknown" {
				return fmt.Errorf("index %s has no amount but amount_unit is not unknown", item.Symbol)
			}
		} else if *item.Amount < 0 || item.AmountUnit != "CNY" {
			return fmt.Errorf("index %s has an invalid amount or amount unit", item.Symbol)
		}
		if item.QuoteTime == nil {
			if item.QualityStatus == model.QualityValid {
				return fmt.Errorf("index %s cannot be valid without a provider quote timestamp", item.Symbol)
			}
			hasDegradedItem = true
		} else {
			if item.QuoteTimeSemantics != "provider_snapshot_time" {
				return fmt.Errorf("index %s has unsupported quote-time semantics %q", item.Symbol, item.QuoteTimeSemantics)
			}
			if item.QuoteTime.IsZero() || item.QuoteTime.After(item.FetchedAt.Add(5*time.Minute)) || item.QuoteTime.After(time.Now().Add(5*time.Minute)) {
				return fmt.Errorf("index %s has a zero or future provider quote timestamp", item.Symbol)
			}
		}
		if item.QualityStatus == model.QualityDegraded {
			hasDegradedItem = true
		}
		if seen[item.Symbol] {
			return fmt.Errorf("duplicate index symbol %q", item.Symbol)
		}
		seen[item.Symbol] = true
	}
	if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
		return errors.New("index-quote snapshot cannot be valid when one or more items are degraded")
	}
	return nil
}
func (s *DataService) IndexQuotes(ctx context.Context) (*model.IndexQuoteSnapshot, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const ttl = 15 * time.Second
	const key = "md:v1:index-quotes:core"
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.IndexQuoteSnapshot
		if json.Unmarshal(data, &cached) == nil && validateIndexQuoteSnapshot(&cached) == nil && time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < ttl {
			return &cached, true, nil
		}
	} else if cacheErr != nil {
		s.logger.Warn("index quote cache read failed", "error", cacheErr)
	}
	if len(s.indexQuoteProviders) == 0 {
		return nil, false, errors.New("no enabled index-quote provider")
	}
	var failures []string
	for _, p := range s.indexQuoteProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchIndexQuotes(ctx)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateIndexQuoteSnapshot(snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		payload, marshalErr := json.Marshal(snapshot)
		if marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("index quote cache write failed", "error", cacheErr)
			}
		} else {
			s.logger.Warn("index quote cache encode failed", "error", marshalErr)
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all index-quote providers failed: %s", strings.Join(failures, "; "))
}

func (s *DataService) SetMarketBreadthProviders(providers []provider.MarketBreadthProvider) {
	s.marketBreadthProviders = append([]provider.MarketBreadthProvider(nil), providers...)
}
func (s *DataService) MarketBreadthProviders() []provider.MarketBreadthProvider {
	return append([]provider.MarketBreadthProvider(nil), s.marketBreadthProviders...)
}
func (s *DataService) SetLimitPoolProviders(providers []provider.LimitPoolProvider) {
	s.limitPoolProviders = append([]provider.LimitPoolProvider(nil), providers...)
}
func (s *DataService) LimitPoolProviders() []provider.LimitPoolProvider {
	return append([]provider.LimitPoolProvider(nil), s.limitPoolProviders...)
}
func (s *DataService) SetDragonTigerProviders(providers []provider.DragonTigerProvider) {
	s.dragonTigerProviders = append([]provider.DragonTigerProvider(nil), providers...)
}
func (s *DataService) DragonTigerProviders() []provider.DragonTigerProvider {
	return append([]provider.DragonTigerProvider(nil), s.dragonTigerProviders...)
}
func (s *DataService) SetStockBoardsProviders(providers []provider.StockBoardsProvider) {
	s.stockBoardsProviders = append([]provider.StockBoardsProvider(nil), providers...)
}
func (s *DataService) StockBoardsProviders() []provider.StockBoardsProvider {
	return append([]provider.StockBoardsProvider(nil), s.stockBoardsProviders...)
}
func (s *DataService) SetTradingCalendarProviders(providers []provider.TradingCalendarProvider) {
	s.tradingCalendarProviders = append([]provider.TradingCalendarProvider(nil), providers...)
}
func (s *DataService) TradingCalendarProviders() []provider.TradingCalendarProvider {
	return append([]provider.TradingCalendarProvider(nil), s.tradingCalendarProviders...)
}

func validateMarketBreadth(snapshot *model.MarketBreadthSnapshot) error {
	if snapshot == nil {
		return errors.New("market-breadth provider returned an empty snapshot")
	}
	if snapshot.Market != "cn_a_share" {
		return fmt.Errorf("unsupported market-breadth market %q", snapshot.Market)
	}
	if snapshot.TotalSecurities <= 0 || snapshot.CoveredSecurities != snapshot.TotalSecurities {
		return fmt.Errorf("market-breadth coverage mismatch: covered=%d total=%d", snapshot.CoveredSecurities, snapshot.TotalSecurities)
	}
	if (snapshot.LimitUpCount != nil && *snapshot.LimitUpCount < 0) || (snapshot.LimitDownCount != nil && *snapshot.LimitDownCount < 0) {
		return errors.New("market-breadth limit counts cannot be negative")
	}
	counts := snapshot.AdvancingCount + snapshot.DecliningCount + snapshot.UnchangedCount + snapshot.UnclassifiedCount
	if snapshot.AdvancingCount < 0 || snapshot.DecliningCount < 0 || snapshot.UnchangedCount < 0 || snapshot.UnclassifiedCount < 0 || counts != snapshot.CoveredSecurities {
		return errors.New("market-breadth category counts do not reconcile to covered securities")
	}
	if snapshot.AmountCoverageCount < 0 || snapshot.AmountCoverageCount > snapshot.CoveredSecurities {
		return errors.New("market-breadth amount coverage is invalid")
	}
	if snapshot.AmountCoverageCount == snapshot.CoveredSecurities {
		if snapshot.TotalAmountCNY == nil || math.IsNaN(*snapshot.TotalAmountCNY) || math.IsInf(*snapshot.TotalAmountCNY, 0) || *snapshot.TotalAmountCNY < 0 {
			return errors.New("market-breadth amount invalid despite complete amount coverage")
		}
	} else {
		if snapshot.TotalAmountCNY != nil {
			return errors.New("partial market-breadth amount coverage must not expose a total amount")
		}
		// 金额覆盖不完整时，即使证券涨跌分类完整，也不能把整个市场广度快照声明为 valid。
		if snapshot.QualityStatus == model.QualityValid {
			return errors.New("partial market-breadth amount coverage cannot have valid quality")
		}
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("market-breadth source or fetched_at is missing")
	}
	reportDate := strings.TrimSpace(snapshot.ProviderReportDate)
	parsedReportDate, reportDateErr := time.Parse("2006-01-02", reportDate)
	if reportDateErr != nil || parsedReportDate.Format("2006-01-02") != reportDate {
		return errors.New("market-breadth provider_report_date is missing or invalid")
	}
	if reportDate > time.Now().In(marketLocation()).Format("2006-01-02") {
		return fmt.Errorf("market-breadth provider_report_date %s is in the future", reportDate)
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("market-breadth quality status %q is unsupported", snapshot.QualityStatus)
	}
	if snapshot.QualityStatus == model.QualityValid {
		for _, reason := range snapshot.QualityReasons {
			switch reason {
			case "market_amount_quotes_collected_non_atomically", "provider_snapshot_timestamp_not_exposed":
				return fmt.Errorf("market-breadth snapshot cannot be valid while quality reason %q is present", reason)
			}
		}
	}
	return nil
}

func (s *DataService) MarketBreadth(ctx context.Context) (*model.MarketBreadthSnapshot, bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	// 市场广度包含全市场成交额分页聚合，使用 5 分钟缓存控制上游请求量。
	const ttl = 5 * time.Minute
	const key = "md:v1:market-breadth:cn-a-share"
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.MarketBreadthSnapshot
		if json.Unmarshal(data, &cached) == nil && validateMarketBreadth(&cached) == nil && time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < ttl {
			return &cached, true, nil
		}
	} else if cacheErr != nil {
		s.logger.Warn("market breadth cache read failed", "error", cacheErr)
	}
	if len(s.marketBreadthProviders) == 0 {
		return nil, false, errors.New("no enabled market-breadth provider")
	}
	var failures []string
	for _, p := range s.marketBreadthProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchMarketBreadth(ctx)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateMarketBreadth(snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if data, err := json.Marshal(snapshot); err == nil {
			if cacheErr := s.cache.Set(ctx, key, data, ttl); cacheErr != nil {
				s.logger.Warn("market breadth cache write failed", "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all market-breadth providers failed: %s", strings.Join(failures, "; "))
}

func normalizeLimitPoolDate(rawDate string) (string, error) {
	rawDate = strings.TrimSpace(rawDate)
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	if rawDate == "" {
		return today, nil
	}
	parsed, err := time.Parse("2006-01-02", rawDate)
	if err != nil || parsed.Format("2006-01-02") != rawDate {
		return "", fmt.Errorf("invalid limit-pool date %q: expected YYYY-MM-DD", rawDate)
	}
	date := parsed.Format("2006-01-02")
	if date > today {
		return "", fmt.Errorf("limit-pool date %s must not be in the future (China market date %s)", date, today)
	}
	return date, nil
}

func validateLimitPoolSnapshot(poolType, tradeDate string, limit int, snapshot *model.LimitPoolSnapshot) error {
	if snapshot == nil {
		return errors.New("limit-pool provider returned an empty snapshot")
	}
	if snapshot.PoolType != poolType || snapshot.TradeDate != tradeDate {
		return fmt.Errorf("limit-pool identity mismatch: requested %s/%s got %s/%s", poolType, tradeDate, snapshot.PoolType, snapshot.TradeDate)
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("limit-pool source or fetched_at is missing")
	}
	if snapshot.ProviderReportDate != "" {
		parsed, dateErr := time.Parse("2006-01-02", snapshot.ProviderReportDate)
		if dateErr != nil || parsed.Format("2006-01-02") != snapshot.ProviderReportDate {
			return errors.New("limit-pool provider report date is invalid")
		}
		if snapshot.ProviderReportDate > time.Now().In(marketLocation()).Format("2006-01-02") {
			return errors.New("limit-pool provider report date is in the future")
		}
		if snapshot.ProviderReportDate != tradeDate {
			return fmt.Errorf("limit-pool provider report date does not match requested trade date: requested=%s returned=%s", tradeDate, snapshot.ProviderReportDate)
		}
	}
	if snapshot.Total < 0 || len(snapshot.Items) > limit {
		return errors.New("limit-pool total is negative or item count exceeds requested limit")
	}
	if snapshot.HasData {
		if snapshot.Total <= 0 || len(snapshot.Items) == 0 {
			return errors.New("limit-pool claims data but has no total/items")
		}
		if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
			return fmt.Errorf("limit-pool quality status %q is unsupported", snapshot.QualityStatus)
		}
		seen := make(map[string]bool, len(snapshot.Items))
		hasDegradedItem := false
		for _, item := range snapshot.Items {
			if strings.TrimSpace(item.Symbol) == "" || strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" || item.ChangePercent == nil {
				return errors.New("limit-pool item missing required fields")
			}
			if item.Source != snapshot.Source || item.FetchedAt.IsZero() {
				return errors.New("limit-pool item source or fetched_at mismatch")
			}
			if item.QualityStatus != model.QualityValid && item.QualityStatus != model.QualityDegraded {
				return fmt.Errorf("limit-pool item %s has unsupported quality %q", item.Symbol, item.QualityStatus)
			}
			if item.QualityStatus == model.QualityDegraded {
				hasDegradedItem = true
			}
			for _, value := range []*float64{
				item.LastPrice, item.ChangePercent, item.TurnoverRatioPercent,
				item.Turnover, item.SealMoney, item.MaxSealMoney,
			} {
				if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
					return fmt.Errorf("limit-pool item %s contains a non-finite numeric value", item.Symbol)
				}
			}
			if item.LastPrice != nil && *item.LastPrice <= 0 {
				return fmt.Errorf("limit-pool item %s has a non-positive last price", item.Symbol)
			}
			for _, value := range []*float64{item.TurnoverRatioPercent, item.Turnover, item.SealMoney, item.MaxSealMoney} {
				if value != nil && *value < 0 {
					return fmt.Errorf("limit-pool item %s has a negative turnover or seal metric", item.Symbol)
				}
			}
			for _, value := range []*int{item.ConsecutiveBoards, item.BrokenCount, item.OpenTimes} {
				if value != nil && *value < 0 {
					return fmt.Errorf("limit-pool item %s has a negative count field", item.Symbol)
				}
			}
			if seen[item.Symbol] {
				return fmt.Errorf("limit-pool duplicate symbol %q", item.Symbol)
			}
			seen[item.Symbol] = true
		}
		if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
			return errors.New("limit-pool snapshot cannot be valid when one or more items are degraded")
		}
		return nil
	}
	if snapshot.Total != 0 || len(snapshot.Items) != 0 || snapshot.QualityStatus != model.QualityUnknown {
		return errors.New("empty limit pool must remain unknown, not a confirmed zero-event pool")
	}
	return nil
}


func markLimitPoolTruncated(snapshot *model.LimitPoolSnapshot) {
	if snapshot == nil || !snapshot.HasData || snapshot.Total <= len(snapshot.Items) {
		return
	}

	snapshot.QualityStatus = model.QualityDegraded
	const reason = "provider_limit_pool_response_truncated"
	for _, existing := range snapshot.QualityReasons {
		if existing == reason {
			return
		}
	}
	snapshot.QualityReasons = append(snapshot.QualityReasons, reason)
}

func (s *DataService) LimitPool(ctx context.Context, rawType, rawDate string, limit int) (*model.LimitPoolSnapshot, bool, error) {
	poolType := strings.ToLower(strings.TrimSpace(rawType))
	if poolType == "" {
		poolType = "up"
	}
	if poolType != "up" && poolType != "down" && poolType != "broken" {
		return nil, false, fmt.Errorf("unsupported limit-pool type %q; want up, down or broken", rawType)
	}
	date, err := normalizeLimitPoolDate(rawDate)
	if err != nil {
		return nil, false, err
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 100 {
		return nil, false, errors.New("limit-pool limit must be between 1 and 100")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const ttl = time.Minute
	key := fmt.Sprintf("md:v1:limit-pool:%s:%s:%d", poolType, date, limit)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.LimitPoolSnapshot
		if json.Unmarshal(data, &cached) == nil && cached.HasData && validateLimitPoolSnapshot(poolType, date, limit, &cached) == nil && time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < ttl {
			// 兼容升级前缓存：若供应商总数大于实际条数，不能沿用缓存中的 valid。
			markLimitPoolTruncated(&cached)
			return &cached, true, nil
		}
	} else if cacheErr != nil {
		s.logger.Warn("limit pool cache read failed", "type", poolType, "date", date, "error", cacheErr)
	}
	if len(s.limitPoolProviders) == 0 {
		return nil, false, errors.New("no enabled limit-pool provider")
	}
	var failures []string
	var unknownEmpty *model.LimitPoolSnapshot
	for _, p := range s.limitPoolProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchLimitPool(ctx, poolType, date, limit)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateLimitPoolSnapshot(poolType, date, limit, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if !snapshot.HasData {
			// Unknown empty is not proof of zero events; continue to the next source.
			unknownEmpty = snapshot
			failures = append(failures, p.Name()+": empty pool is unknown, not confirmed zero")
			continue
		}
		// total 是供应商报告总量；当前接口没有分页游标，未返回完整条数时显式降级。
		markLimitPoolTruncated(snapshot)
		if data, err := json.Marshal(snapshot); err == nil {
			if cacheErr := s.cache.Set(ctx, key, data, ttl); cacheErr != nil {
				s.logger.Warn("limit pool cache write failed", "type", poolType, "date", date, "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	if unknownEmpty != nil {
		// 所有 Provider 都未能确认空池为真实零事件时，保留 unknown；不缓存，避免下一次请求跳过重新回退。
		const reason = "limit_pool_fallback_exhausted_without_confirmed_data"
		alreadyPresent := false
		for _, existing := range unknownEmpty.QualityReasons {
			if existing == reason {
				alreadyPresent = true
				break
			}
		}
		if !alreadyPresent {
			unknownEmpty.QualityReasons = append(unknownEmpty.QualityReasons, reason)
		}
		return unknownEmpty, false, nil
	}
	return nil, false, fmt.Errorf("all limit-pool providers failed: %s", strings.Join(failures, "; "))
}

func normalizeDragonTigerDate(rawDate string) (string, error) {
	rawDate = strings.TrimSpace(rawDate)
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	if rawDate == "" {
		return today, nil
	}
	parsed, err := time.Parse("2006-01-02", rawDate)
	if err != nil || parsed.Format("2006-01-02") != rawDate {
		return "", fmt.Errorf("invalid dragon-tiger date %q: expected YYYY-MM-DD", rawDate)
	}
	date := parsed.Format("2006-01-02")
	if date > today {
		return "", fmt.Errorf("dragon-tiger date %s must not be in the future (China market date %s)", date, today)
	}
	return date, nil
}

func validateDragonTigerSnapshot(tradeDate string, page, pageSize int, snapshot *model.DragonTigerSnapshot) error {
	if snapshot == nil {
		return errors.New("dragon-tiger provider returned an empty snapshot")
	}
	if snapshot.TradeDate != tradeDate || snapshot.Page != page || snapshot.PageSize != pageSize {
		return fmt.Errorf("dragon-tiger request identity mismatch: wanted %s/%d/%d, got %s/%d/%d", tradeDate, page, pageSize, snapshot.TradeDate, snapshot.Page, snapshot.PageSize)
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("dragon-tiger source or fetched_at is missing")
	}
	if snapshot.Total < 0 || snapshot.Pages < 0 || snapshot.SourceReturned < 0 ||
		snapshot.ExcludedNonAShareCount < 0 || snapshot.Returned != len(snapshot.Items) ||
		snapshot.Returned > snapshot.SourceReturned ||
		snapshot.SourceReturned != snapshot.Returned+snapshot.ExcludedNonAShareCount {
		return errors.New("dragon-tiger totals or source/accepted/excluded row counts are invalid")
	}
	if snapshot.HasData {
		if snapshot.Total <= 0 || snapshot.Pages <= 0 || snapshot.Returned == 0 ||
			snapshot.SourceReturned == 0 || snapshot.Total < snapshot.SourceReturned {
			return errors.New("dragon-tiger totals/items are inconsistent")
		}
		if snapshot.ProviderReportDate != tradeDate {
			return errors.New("dragon-tiger provider report date does not match requested trade date")
		}
		if snapshot.HasMore != (page < snapshot.Pages) {
			return errors.New("dragon-tiger has_more does not match provider page count")
		}
		if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
			return fmt.Errorf("dragon-tiger quality status %q is unsupported", snapshot.QualityStatus)
		}
		seen := make(map[string]struct{}, len(snapshot.Items))
		hasDegradedItem := false
		for _, item := range snapshot.Items {
			if strings.TrimSpace(item.Symbol) == "" || strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" || item.TradeDate != tradeDate {
				return errors.New("dragon-tiger item is missing identity fields or has mismatched trade date")
			}
			if item.Source != snapshot.Source || item.FetchedAt.IsZero() || (item.QualityStatus != model.QualityValid && item.QualityStatus != model.QualityDegraded) {
				return errors.New("dragon-tiger item source, fetched_at, or quality is invalid")
			}
			if item.QualityStatus == model.QualityDegraded {
				hasDegradedItem = true
			}
			key := item.Symbol + "|" + item.Reason + "|" + item.Explanation
			if _, ok := seen[key]; ok {
				return fmt.Errorf("dragon-tiger contains duplicate row %q", key)
			}
			seen[key] = struct{}{}
			for _, value := range []*float64{item.BuyAmountCNY, item.SellAmountCNY, item.DealAmountCNY, item.NetBuyAmountCNY, item.ClosePrice, item.ChangePercent, item.DealAmountRatio} {
				if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
					return errors.New("dragon-tiger item contains a non-finite numeric value")
				}
			}
			for _, value := range []*float64{item.BuyAmountCNY, item.SellAmountCNY, item.DealAmountCNY} {
				if value != nil && *value < 0 {
					return errors.New("dragon-tiger buy/sell/deal amount cannot be negative")
				}
			}
			if item.ClosePrice != nil && *item.ClosePrice <= 0 {
				return errors.New("dragon-tiger close price must be positive when present")
			}
		}
		if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
			return errors.New("dragon-tiger snapshot cannot be valid when one or more items are degraded")
		}
		return nil
	}
	if snapshot.Total == 0 {
		if snapshot.Pages != 0 || snapshot.SourceReturned != 0 || snapshot.Returned != 0 ||
			snapshot.ExcludedNonAShareCount != 0 || len(snapshot.Items) != 0 || snapshot.HasMore ||
			snapshot.QualityStatus != model.QualityUnknown {
			return errors.New("empty dragon-tiger report must remain unknown, not a confirmed zero-event report")
		}
		return nil
	}
	// 供应商页可能全部属于可转债等非 A 股品种。允许返回 unknown 空页，但必须明确记录全部被排除且保留分页元数据。
	if snapshot.Pages <= 0 || snapshot.SourceReturned <= 0 || snapshot.Returned != 0 ||
		snapshot.ExcludedNonAShareCount != snapshot.SourceReturned || len(snapshot.Items) != 0 ||
		snapshot.QualityStatus != model.QualityUnknown || snapshot.HasMore != (page < snapshot.Pages) {
		return errors.New("empty supported-A-share page has inconsistent source counts or must remain unknown")
	}
	return nil
}

// DragonTiger 获取指定交易日的龙虎榜分页。盘后报表未发布时，空结果保留为 unknown。
func (s *DataService) DragonTiger(ctx context.Context, rawDate string, page, pageSize int) (*model.DragonTigerSnapshot, bool, error) {
	date, err := normalizeDragonTigerDate(rawDate)
	if err != nil {
		return nil, false, err
	}
	if page < 1 || page > 1000 {
		return nil, false, errors.New("dragon-tiger page must be between 1 and 1000")
	}
	if pageSize < 1 || pageSize > 500 {
		return nil, false, errors.New("dragon-tiger page size must be between 1 and 500")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const ttl = 5 * time.Minute
	const emptyTTL = 30 * time.Second
	key := fmt.Sprintf("md:v1:dragon-tiger:%s:%d:%d", date, page, pageSize)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.DragonTigerSnapshot
		if json.Unmarshal(data, &cached) == nil && validateDragonTigerSnapshot(date, page, pageSize, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			expiry := ttl
			cacheEligible := cached.HasData
			if !cached.HasData {
				expiry = emptyTTL
				// 兼容旧缓存：未知空页若未记录回退链已耗尽，多 Provider 场景必须重新尝试后备源。
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "dragon_tiger_fallback_exhausted_without_confirmed_data" {
						fallbackExhausted = true
						break
					}
				}
				cacheEligible = fallbackExhausted || len(s.dragonTigerProviders) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < expiry {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("dragon-tiger cache read failed", "date", date, "page", page, "error", cacheErr)
	}
	if len(s.dragonTigerProviders) == 0 {
		return nil, false, errors.New("no enabled dragon-tiger provider")
	}
	var failures []string
	var unknownEmpty *model.DragonTigerSnapshot
	for _, p := range s.dragonTigerProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		snapshot, fetchErr := p.FetchDragonTiger(ctx, date, page, pageSize)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if err := validateDragonTigerSnapshot(date, page, pageSize, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if !snapshot.HasData {
			// 空报告或该页未保留 A 股都不能直接证明无可用龙虎榜数据；尝试后续 Provider。
			unknownEmpty = snapshot
			failures = append(failures, p.Name()+": no confirmed A-share dragon-tiger rows on this page")
			continue
		}
		if data, marshalErr := json.Marshal(snapshot); marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, data, ttl); cacheErr != nil {
				s.logger.Warn("dragon-tiger cache write failed", "date", date, "page", page, "error", cacheErr)
			}
		}
		return snapshot, false, nil
	}
	if unknownEmpty != nil {
		// 回退链耗尽后才短缓存 unknown，防止旧的首源空缓存挡住其他来源。
		unknownEmpty.QualityReasons = appendReason(unknownEmpty.QualityReasons, "dragon_tiger_fallback_exhausted_without_confirmed_data")
		if data, marshalErr := json.Marshal(unknownEmpty); marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, data, emptyTTL); cacheErr != nil {
				s.logger.Warn("dragon-tiger unknown cache write failed", "date", date, "page", page, "error", cacheErr)
			}
		}
		return unknownEmpty, false, nil
	}
	return nil, false, fmt.Errorf("all dragon-tiger providers failed: %s", strings.Join(failures, "; "))
}

func validateSectorRankingSnapshot(category string, limit int, snapshot *model.SectorRankingSnapshot) error {
	if snapshot == nil {
		return errors.New("sector-ranking provider returned an empty response")
	}
	if snapshot.Category != category {
		return fmt.Errorf("sector-ranking category mismatch: requested %s, got %q", category, snapshot.Category)
	}
	if snapshot.Direction != "" && snapshot.Direction != "up" && snapshot.Direction != "down" {
		return fmt.Errorf("sector-ranking direction %q is unsupported", snapshot.Direction)
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("sector-ranking source or fetched_at is missing")
	}
	if snapshot.QualityStatus == model.QualityValid {
		return errors.New("sector-ranking snapshot cannot be valid without a provider snapshot timestamp")
	}
	if snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("sector-ranking quality status %q is unsupported", snapshot.QualityStatus)
	}
	if len(snapshot.Items) == 0 || len(snapshot.Items) > limit {
		return fmt.Errorf("sector-ranking item count %d is outside 1..%d", len(snapshot.Items), limit)
	}
	if snapshot.Total <= 0 || snapshot.Total < len(snapshot.Items) {
		return errors.New("sector-ranking total is less than returned items or not positive")
	}
	seenCodes := make(map[string]bool, len(snapshot.Items))
	seenRanks := make(map[int]bool, len(snapshot.Items))
	hasDegradedItem := false
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" || item.Category != category || item.ChangePercent == nil {
			return errors.New("sector-ranking item has missing code/name/change-percent or mismatched category")
		}
		if item.Rank < 1 {
			return errors.New("sector-ranking item rank must be positive")
		}
		if item.Source != snapshot.Source {
			return errors.New("sector-ranking item source differs from its snapshot source")
		}
		if item.QualityStatus == model.QualityValid {
			return errors.New("sector-ranking item cannot be valid without a provider snapshot timestamp")
		}
		if item.FetchedAt.IsZero() || item.QualityStatus != model.QualityDegraded {
			return errors.New("sector-ranking item has invalid retrieval time or quality status")
		}
		for _, value := range []*float64{item.LatestPrice, item.Change, item.ChangePercent, item.Amount, item.LeaderChangePercent} {
			if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
				return fmt.Errorf("sector-ranking item %s contains a non-finite numeric value", item.Code)
			}
		}
		if item.LatestPrice != nil && *item.LatestPrice <= 0 {
			return fmt.Errorf("sector-ranking item %s has a non-positive latest price", item.Code)
		}
		if item.Amount == nil {
			if item.AmountUnit != "unknown" {
				return fmt.Errorf("sector-ranking item %s has no amount but amount_unit is not unknown", item.Code)
			}
		} else if *item.Amount < 0 || item.AmountUnit != "CNY" {
			return fmt.Errorf("sector-ranking item %s has an invalid amount or amount unit", item.Code)
		}
		for _, count := range []*int{item.AdvancingCount, item.DecliningCount} {
			if count != nil && *count < 0 {
				return fmt.Errorf("sector-ranking item %s has a negative advancing/declining count", item.Code)
			}
		}
		if seenCodes[item.Code] {
			return fmt.Errorf("sector-ranking duplicate board code %q", item.Code)
		}
		seenCodes[item.Code] = true
		if seenRanks[item.Rank] {
			return fmt.Errorf("sector-ranking duplicate rank %d", item.Rank)
		}
		seenRanks[item.Rank] = true
		if item.QualityStatus == model.QualityDegraded {
			hasDegradedItem = true
		}
	}
	if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
		return errors.New("sector-ranking snapshot cannot be valid when one or more items are degraded")
	}
	return nil
}

func (s *DataService) SectorRankings(ctx context.Context, rawCategory string, limit int) (*model.SectorRankingSnapshot, bool, error) {
	return s.SectorRankingsByDirection(ctx, rawCategory, "up", limit)
}

func (s *DataService) SectorRankingsByDirection(ctx context.Context, rawCategory, rawDirection string, limit int) (*model.SectorRankingSnapshot, bool, error) {
	category := strings.ToLower(strings.TrimSpace(rawCategory))
	if category != "industry" && category != "concept" {
		return nil, false, fmt.Errorf("unsupported sector category %q; want industry or concept", rawCategory)
	}
	direction := strings.ToLower(strings.TrimSpace(rawDirection))
	if direction == "" {
		direction = "up"
	}
	if direction != "up" && direction != "down" {
		return nil, false, fmt.Errorf("unsupported sector ranking direction %q; want up or down", rawDirection)
	}
	if limit == 0 {
		limit = 10
	}
	if limit < 1 || limit > 100 {
		return nil, false, errors.New("sector ranking limit must be between 1 and 100")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const ttl = time.Minute
	key := fmt.Sprintf("md:v2:sector-ranking:%s:%s:%d", category, direction, limit)
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.SectorRankingSnapshot
		if json.Unmarshal(data, &cached) == nil && validateSectorRankingSnapshot(category, limit, &cached) == nil &&
			time.Since(cached.FetchedAt) >= 0 && time.Since(cached.FetchedAt) < ttl {
			return &cached, true, nil
		}
	} else if cacheErr != nil {
		s.logger.Warn("sector-ranking cache read failed", "category", category, "error", cacheErr)
	}
	if len(s.sectorRankingProviders) == 0 {
		return nil, false, errors.New("no enabled sector-ranking provider")
	}
	var failures []string
	for _, p := range s.sectorRankingProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		var snapshot *model.SectorRankingSnapshot
		var fetchErr error
		if directional, ok := p.(provider.DirectionalSectorRankingProvider); ok {
			snapshot, fetchErr = directional.FetchSectorRankingsDirection(ctx, category, direction, limit)
		} else if direction == "up" {
			snapshot, fetchErr = p.FetchSectorRankings(ctx, category, limit)
		} else {
			fetchErr = fmt.Errorf("provider does not support %s sector ranking", direction)
		}
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if snapshot != nil {
			if snapshot.Direction != "" && snapshot.Direction != direction {
				failures = append(failures, p.Name()+": provider returned a different sector ranking direction")
				continue
			}
			snapshot.Direction = direction
		}
		if snapshot != nil && snapshot.QualityStatus == model.QualityValid {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendReason(snapshot.QualityReasons, "provider_snapshot_timestamp_not_exposed")
		}
		if snapshot != nil {
			for index := range snapshot.Items {
				if snapshot.Items[index].QualityStatus == model.QualityValid {
					snapshot.Items[index].QualityStatus = model.QualityDegraded
					snapshot.Items[index].QualityReasons = appendReason(snapshot.Items[index].QualityReasons, "provider_snapshot_timestamp_not_exposed")
				}
			}
		}
		if err := validateSectorRankingSnapshot(category, limit, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		payload, marshalErr := json.Marshal(snapshot)
		if marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("sector-ranking cache write failed", "category", category, "error", cacheErr)
			}
		} else {
			s.logger.Warn("sector-ranking cache encode failed", "category", category, "error", marshalErr)
		}
		return snapshot, false, nil
	}
	return nil, false, fmt.Errorf("all sector-ranking providers failed: %s", strings.Join(failures, "; "))
}
func validateStockBasic(symbol string, item *model.StockBasic) error {
	if item == nil {
		return errors.New("stock-basic provider returned an empty response")
	}
	canonical, err := normalize.NormalizeSymbol(item.Symbol)
	if err != nil || canonical != symbol {
		return fmt.Errorf("stock-basic symbol mismatch: requested %s, got %q", symbol, item.Symbol)
	}
	if strings.TrimSpace(item.Name) == "" {
		return errors.New("stock-basic name is empty")
	}
	if item.SecurityType < 1 || item.SecurityType > 4 {
		return fmt.Errorf("stock-basic security type %d is unsupported", item.SecurityType)
	}
	if item.ListingStatus != 0 && item.ListingStatus != 1 {
		return fmt.Errorf("stock-basic listing status %d is unsupported", item.ListingStatus)
	}
	for field, value := range map[string]string{"ipo_date": item.IPODate, "out_date": item.OutDate} {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, err := time.Parse("2006-01-02", value); err != nil {
			return fmt.Errorf("stock-basic %s %q is invalid", field, value)
		}
	}
	for field, value := range map[string]*float64{"pe_dynamic": item.DynamicPE, "pb": item.PB, "market_cap_cny": item.MarketCapCNY, "float_market_cap_cny": item.FloatMarketCapCNY, "roe_percent": item.ROEPercent, "net_profit_margin_percent": item.NetProfitMarginPercent, "net_profit_cny": item.NetProfitCNY, "gross_margin_percent": item.GrossMarginPercent} {
		if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
			return fmt.Errorf("stock-basic %s is non-finite", field)
		}
	}
	if item.MarketCapCNY != nil && *item.MarketCapCNY < 0 {
		return errors.New("stock-basic market_cap_cny must not be negative")
	}
	if item.FloatMarketCapCNY != nil && *item.FloatMarketCapCNY < 0 {
		return errors.New("stock-basic float_market_cap_cny must not be negative")
	}
	if item.QualityStatus == model.QualityValid {
		return errors.New("stock-basic cannot be valid without a provider snapshot timestamp")
	}
	if item.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("stock-basic quality status %q is unsupported", item.QualityStatus)
	}
	if strings.TrimSpace(item.Source) == "" {
		return errors.New("stock-basic source is missing")
	}
	if item.FetchedAt.IsZero() {
		return errors.New("stock-basic fetched_at is missing")
	}
	return nil
}

// StockBasic loads security identity and listing data from independent stock-basic providers.
func (s *DataService) StockBasic(ctx context.Context, rawSymbol string) (*model.StockBasic, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const ttl = 24 * time.Hour
	const degradedTTL = 5 * time.Minute
	key := "md:v1:stock-basic:" + symbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var item model.StockBasic
		if json.Unmarshal(data, &item) == nil && validateStockBasic(symbol, &item) == nil && time.Since(item.FetchedAt) >= 0 {
			maxAge := ttl
			if item.QualityStatus == model.QualityDegraded {
				maxAge = degradedTTL
			}
			if time.Since(item.FetchedAt) < maxAge {
				return &item, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("stock-basic cache read failed", "symbol", symbol, "error", cacheErr)
	}
	if len(s.stockBasicProviders) == 0 {
		return nil, false, errors.New("no enabled stock-basic provider")
	}
	var failures []string
	for _, p := range s.stockBasicProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		item, fetchErr := p.FetchStockBasic(ctx, symbol)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if item == nil {
			failures = append(failures, p.Name()+": empty stock-basic response")
			continue
		}
		returnedSymbol, symbolErr := normalize.NormalizeSymbol(item.Symbol)
		if symbolErr != nil || returnedSymbol != symbol {
			failures = append(failures, p.Name()+": stock-basic symbol mismatch")
			continue
		}
		if item.Source == "" {
			item.Source = p.Name()
		}
		if item.FetchedAt.IsZero() {
			item.FetchedAt = time.Now().UTC()
		}
		if item.SecurityTypeLabel == "" {
			item.SecurityTypeLabel = map[int]string{1: "stock", 2: "index", 3: "other", 4: "convertible_bond"}[item.SecurityType]
		}
		if item.ListingStatusLabel == "" {
			item.ListingStatusLabel = map[int]string{0: "delisted", 1: "listed"}[item.ListingStatus]
		}
		if item.QualityStatus == "" || item.QualityStatus == model.QualityUnknown {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "provider_quality_status_missing_or_unknown")
		}
		if item.QualityStatus == model.QualityValid {
			// 基础资料合并当前估值/财务扩展字段，但 Provider 未暴露可核验快照时间。
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "provider_snapshot_timestamp_not_exposed")
		}
		if item.NormalizationVer == "" {
			item.NormalizationVer = normalize.Version
		}
		if err := validateStockBasic(symbol, item); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		cacheTTL := ttl
		if item.QualityStatus == model.QualityDegraded {
			cacheTTL = degradedTTL
		}
		payload, marshalErr := json.Marshal(item)
		if marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, cacheTTL); cacheErr != nil {
				s.logger.Warn("stock-basic cache write failed", "symbol", symbol, "error", cacheErr)
			}
		} else {
			s.logger.Warn("stock-basic cache encode failed", "symbol", symbol, "error", marshalErr)
		}
		return item, false, nil
	}
	return nil, false, fmt.Errorf("all stock-basic providers failed: %s", strings.Join(failures, "; "))
}

func (s *DataService) SearchStocks(ctx context.Context, rawQuery string, limit int) (*model.StockSearchSnapshot, error) {
	query := strings.TrimSpace(rawQuery)
	if query == "" {
		return nil, errors.New("stock-search query is required")
	}
	if len([]rune(query)) > 64 {
		return nil, errors.New("stock-search query must not exceed 64 characters")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		return nil, errors.New("stock-search limit must not exceed 50")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(s.stockSearchProviders) == 0 {
		return nil, errors.New("no enabled stock-search provider")
	}
	failures := make([]string, 0, len(s.stockSearchProviders))
	var unknownEmpty *model.StockSearchSnapshot
	var staleMatches *model.StockSearchSnapshot
	for _, p := range s.stockSearchProviders {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		snapshot, fetchErr := p.SearchStocks(ctx, query, limit)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if snapshot == nil {
			failures = append(failures, p.Name()+": empty stock-search response")
			continue
		}
		if snapshot.Source == "" {
			snapshot.Source = p.Name()
		}
		if snapshot.Query == "" {
			snapshot.Query = query
		}
		if snapshot.FetchedAt.IsZero() {
			snapshot.FetchedAt = time.Now().UTC()
		}
		if snapshot.QualityStatus == "" {
			snapshot.QualityStatus = model.QualityUnknown
		}
		if err := validateStockSearchSnapshot(query, limit, snapshot); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if snapshot.QualityStatus == model.QualityUnknown {
			if len(snapshot.Items) == 0 {
				// 未知目录中的空匹配不能证明证券不存在，继续尝试其他搜索源。
				unknownEmpty = snapshot
				failures = append(failures, p.Name()+": empty match from unconfirmed stock directory")
			} else {
				// 有匹配条目但目录质量未知时，保留覆盖较完整的候选并继续尝试后备源。
				if unknownMatches == nil || len(snapshot.Items) > len(unknownMatches.Items) {
					unknownMatches = snapshot
				}
				failures = append(failures, p.Name()+": matches retained as unconfirmed directory fallback")
			}
			continue
		}
		if len(snapshot.Items) == 0 && snapshot.QualityStatus == model.QualityStale {
			// 过期目录中的空匹配不能证明证券不存在，继续尝试其他搜索源。
			failures = append(failures, p.Name()+": empty match from stale stock directory")
			continue
		}
		if snapshot.QualityStatus == model.QualityStale {
			// 旧目录有匹配项时保留为最终后备，但先尝试后续来源获取更新结果。
			staleMatches = snapshot
			failures = append(failures, p.Name()+": stale stock-search matches retained as fallback")
			continue
		}
		// 完整且非空的查询结果，或质量状态有效/降级的确定空结果，均可直接返回。
		return snapshot, nil
	}
	if staleMatches != nil {
		staleMatches.QualityReasons = appendReason(staleMatches.QualityReasons, "stock_search_fallback_exhausted_using_stale_directory")
		return staleMatches, nil
	}
	if unknownMatches != nil {
		unknownMatches.QualityStatus = model.QualityUnknown
		unknownMatches.QualityReasons = appendReason(unknownMatches.QualityReasons, "stock_search_fallback_exhausted_with_unconfirmed_quality")
		return unknownMatches, nil
	}
	if unknownEmpty != nil {
		unknownEmpty.QualityStatus = model.QualityUnknown
		unknownEmpty.QualityReasons = appendReason(unknownEmpty.QualityReasons, "stock_search_fallback_exhausted_without_confirmed_match")
		return unknownEmpty, nil
	}
	return nil, fmt.Errorf("all stock-search providers failed: %s", strings.Join(failures, "; "))
}

func validateStockSearchSnapshot(query string, limit int, snapshot *model.StockSearchSnapshot) error {
	if snapshot == nil {
		return errors.New("stock-search provider returned an empty snapshot")
	}
	if strings.TrimSpace(snapshot.Query) != query {
		return errors.New("stock-search query mismatch")
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("stock-search source or fetched_at is missing")
	}
	if snapshot.Total < 0 || snapshot.Total < len(snapshot.Items) || len(snapshot.Items) > limit {
		return errors.New("stock-search total or item count is invalid")
	}
	switch snapshot.QualityStatus {
	case model.QualityValid, model.QualityDegraded, model.QualityStale, model.QualityUnknown:
	default:
		return fmt.Errorf("stock-search quality status %q is unsupported", snapshot.QualityStatus)
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Name) == "" || strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Exchange) == "" {
			return errors.New("stock-search item has missing code, name or exchange")
		}
		symbol, err := normalize.NormalizeSymbol(item.Symbol)
		if err != nil || symbol != item.Symbol {
			return errors.New("stock-search item has an invalid normalized symbol")
		}
		parts := strings.Split(symbol, ".")
		if len(parts) != 2 || item.Code != parts[0] || item.Exchange != parts[1] {
			return errors.New("stock-search item symbol, code and exchange disagree")
		}
		if _, ok := seen[symbol]; ok {
			return fmt.Errorf("stock-search provider returned duplicate symbol %q", symbol)
		}
		seen[symbol] = struct{}{}
	}
	return nil
}

func validateStockBoardsSnapshot(symbol string, snapshot *model.StockBoardSnapshot) error {
	if snapshot == nil {
		return errors.New("stock-boards provider returned an empty snapshot")
	}
	returnedSymbol, err := normalize.NormalizeSymbol(snapshot.Symbol)
	if err != nil || returnedSymbol != symbol {
		return fmt.Errorf("stock-boards symbol mismatch: requested %s, returned %q", symbol, snapshot.Symbol)
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("stock-boards source or fetched_at is missing")
	}
	if snapshot.Total < 0 || snapshot.Total > 1000 || len(snapshot.Items) > snapshot.Total {
		return errors.New("stock-boards total/items count is invalid")
	}
	if !snapshot.HasData {
		if snapshot.Total != 0 || len(snapshot.Items) != 0 || snapshot.QualityStatus != model.QualityUnknown {
			return errors.New("empty stock-board response must remain unknown, not a verified zero-membership result")
		}
		return nil
	}
	if snapshot.Total == 0 || len(snapshot.Items) == 0 || snapshot.Total != len(snapshot.Items) {
		return errors.New("stock-boards coverage is incomplete")
	}
	if snapshot.QualityStatus == model.QualityValid {
		return errors.New("stock-boards cannot be valid without a provider snapshot timestamp")
	}
	if snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("stock-boards quality status %q is unsupported", snapshot.QualityStatus)
	}
	seen := make(map[string]struct{}, len(snapshot.Items))
	hasDegradedItem := false
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" {
			return errors.New("stock-board item is missing code or name")
		}
		if item.Source != snapshot.Source || item.FetchedAt.IsZero() {
			return errors.New("stock-board item source or fetched_at mismatch")
		}
		if item.QualityStatus == model.QualityValid {
			return errors.New("stock-board item cannot be valid without a provider snapshot timestamp")
		}
		if item.QualityStatus != model.QualityDegraded {
			return fmt.Errorf("stock-board quality status %q is unsupported", item.QualityStatus)
		}
		if item.QualityStatus == model.QualityDegraded {
			hasDegradedItem = true
		}
		if _, exists := seen[item.Code]; exists {
			return fmt.Errorf("duplicate stock-board code %q", item.Code)
		}
		seen[item.Code] = struct{}{}
		if item.ChangePercent != nil && (math.IsNaN(*item.ChangePercent) || math.IsInf(*item.ChangePercent, 0)) {
			return fmt.Errorf("stock-board %s has invalid change percent", item.Code)
		}
	}
	if snapshot.QualityStatus == model.QualityValid && hasDegradedItem {
		return errors.New("stock-boards snapshot cannot be valid when one or more items are degraded")
	}
	return nil
}

// StockBoards obtains board membership without mixing it into quote or daily-bar fallback.
func (s *DataService) StockBoards(ctx context.Context, rawSymbol string) (*model.StockBoardSnapshot, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	const positiveTTL = 12 * time.Hour
	const emptyTTL = 5 * time.Minute
	key := "md:v1:stock-boards:" + symbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var cached model.StockBoardSnapshot
		if json.Unmarshal(data, &cached) == nil && validateStockBoardsSnapshot(symbol, &cached) == nil && time.Since(cached.FetchedAt) >= 0 {
			ttl := positiveTTL
			cacheEligible := cached.HasData
			if !cached.HasData {
				ttl = emptyTTL
				// 兼容旧版本缓存：未知空结果若未记录回退链耗尽，不得阻止新版本尝试其他 Provider。
				fallbackExhausted := false
				for _, reason := range cached.QualityReasons {
					if reason == "stock_boards_fallback_exhausted_without_confirmed_membership" {
						fallbackExhausted = true
						break
					}
				}
				cacheEligible = fallbackExhausted || len(s.stockBoardsProviders) <= 1
			}
			if cacheEligible && time.Since(cached.FetchedAt) < ttl {
				return &cached, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("stock-boards cache read failed", "symbol", symbol, "error", cacheErr)
	}
	if len(s.stockBoardsProviders) == 0 {
		return nil, false, errors.New("no enabled stock-boards provider")
	}
	var failures []string
	var unknownEmpty *model.StockBoardSnapshot
	for _, p := range s.stockBoardsProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		item, fetchErr := p.FetchStockBoards(ctx, symbol)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if item == nil {
			failures = append(failures, p.Name()+": empty stock-boards response")
			continue
		}
		if item.Source == "" {
			item.Source = p.Name()
		}
		if item.FetchedAt.IsZero() {
			item.FetchedAt = time.Now().UTC()
		}
		for index := range item.Items {
			if item.Items[index].Source == "" {
				item.Items[index].Source = p.Name()
			}
			if item.Items[index].FetchedAt.IsZero() {
				item.Items[index].FetchedAt = item.FetchedAt
			}
			if item.Items[index].SourceVersion == "" {
				item.Items[index].SourceVersion = item.SourceVersion
			}
			if item.Items[index].QualityStatus == "" || item.Items[index].QualityStatus == model.QualityUnknown {
				item.Items[index].QualityStatus = model.QualityDegraded
				item.Items[index].QualityReasons = appendReason(item.Items[index].QualityReasons, "provider_quality_not_reported")
			}
			if item.Items[index].QualityStatus == model.QualityValid {
				item.Items[index].QualityStatus = model.QualityDegraded
				item.Items[index].QualityReasons = appendReason(item.Items[index].QualityReasons, "provider_snapshot_timestamp_not_exposed")
			}
		}
		if item.QualityStatus == "" {
			if item.HasData {
				item.QualityStatus = model.QualityDegraded
				item.QualityReasons = appendReason(item.QualityReasons, "provider_quality_not_reported")
			} else {
				item.QualityStatus = model.QualityUnknown
			}
		}
		if item.QualityStatus == model.QualityValid {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "provider_snapshot_timestamp_not_exposed")
		}
		if err := validateStockBoardsSnapshot(symbol, item); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if !item.HasData {
			// 空成员列表只说明当前源无法确认归属，不代表没有所属板块；尝试后续 Provider。
			unknownEmpty = item
			failures = append(failures, p.Name()+": empty board membership is unknown, not confirmed zero")
			continue
		}
		// 已确认有返回条目的快照可以缓存较长时间。
		payload, marshalErr := json.Marshal(item)
		if marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, positiveTTL); cacheErr != nil {
				s.logger.Warn("stock-boards cache write failed", "symbol", symbol, "error", cacheErr)
			}
		} else {
			s.logger.Warn("stock-boards cache encode failed", "symbol", symbol, "error", marshalErr)
		}
		return item, false, nil
	}
	if unknownEmpty != nil {
		// 只有回退链耗尽后才短缓存 unknown，且记录本次已尝试全部启用源。
		unknownEmpty.QualityReasons = appendReason(unknownEmpty.QualityReasons, "stock_boards_fallback_exhausted_without_confirmed_membership")
		payload, marshalErr := json.Marshal(unknownEmpty)
		if marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, emptyTTL); cacheErr != nil {
				s.logger.Warn("stock-boards unknown cache write failed", "symbol", symbol, "error", cacheErr)
			}
		}
		return unknownEmpty, false, nil
	}
	return nil, false, fmt.Errorf("all stock-boards providers failed: %s", strings.Join(failures, "; "))
}

// SetChipDistributionProviders 注册独立的原生筹码分布 Provider。
func (s *DataService) SetChipDistributionProviders(providers []provider.ChipDistributionProvider) {
	s.chipDistributionProviders = append([]provider.ChipDistributionProvider(nil), providers...)
}

func (s *DataService) ChipDistributionProviders() []provider.ChipDistributionProvider {
	return append([]provider.ChipDistributionProvider(nil), s.chipDistributionProviders...)
}

// SetSupplementProviders 注册显式启用的非价格类补充数据源。
// 此类 Provider 不会被加入实时行情或日 K 的兜底链。
func (s *DataService) SetSupplementProviders(providers []provider.SupplementProvider) {
	s.supplementProviders = append([]provider.SupplementProvider(nil), providers...)
}

func (s *DataService) SupplementProviders() []provider.SupplementProvider {
	return append([]provider.SupplementProvider(nil), s.supplementProviders...)
}

// SetCapitalFlowProviders 注册独立的资金流数据源，避免资金流能力依赖筹码分布接口。
func (s *DataService) SetCapitalFlowProviders(providers []provider.CapitalFlowProvider) {
	s.capitalFlowProviders = append([]provider.CapitalFlowProvider(nil), providers...)
}

// CapitalFlowProviders 合并独立资金流 Provider 与旧版 SupplementProvider，按名称去重。
func (s *DataService) CapitalFlowProviders() []provider.CapitalFlowProvider {
	out := make([]provider.CapitalFlowProvider, 0, len(s.capitalFlowProviders)+len(s.supplementProviders))
	seen := make(map[string]struct{}, len(s.capitalFlowProviders)+len(s.supplementProviders))
	appendUnique := func(p provider.CapitalFlowProvider) {
		if p == nil {
			return
		}
		name := strings.ToLower(strings.TrimSpace(p.Name()))
		if _, exists := seen[name]; exists {
			return
		}
		seen[name] = struct{}{}
		out = append(out, p)
	}
	for _, p := range s.capitalFlowProviders {
		appendUnique(p)
	}
	for _, p := range s.supplementProviders {
		appendUnique(p)
	}
	return out
}
func validateChipDistributionSnapshot(symbol string, item *model.ChipDistribution) error {
	if item == nil {
		return errors.New("chip-distribution response is empty")
	}
	canonical, err := normalize.NormalizeSymbol(item.Symbol)
	if err != nil || canonical != symbol || item.Symbol != symbol {
		return errors.New("chip-distribution symbol mismatch")
	}
	if item.AverageCost <= 0 || !isFiniteSupplement(item.AverageCost) {
		return errors.New("chip-distribution average cost is invalid")
	}
	for _, value := range []*float64{item.Cost90Low, item.Cost90High, item.Cost70Low, item.Cost70High} {
		if value != nil && (*value <= 0 || !isFiniteSupplement(*value)) {
			return errors.New("chip-distribution cost range contains an invalid value")
		}
	}
	if item.Cost90Low != nil && item.Cost90High != nil && *item.Cost90Low > *item.Cost90High {
		return errors.New("chip-distribution 90% cost range is reversed")
	}
	if item.Cost70Low != nil && item.Cost70High != nil && *item.Cost70Low > *item.Cost70High {
		return errors.New("chip-distribution 70% cost range is reversed")
	}
	for _, ratio := range []*float64{item.ProfitRatio, item.Concentration90, item.Concentration70} {
		if ratio != nil && (!isFiniteSupplement(*ratio) || *ratio < 0 || *ratio > 1) {
			return errors.New("chip-distribution ratio is outside [0,1]")
		}
	}
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	for _, date := range []string{item.Date, item.ProviderDate} {
		if strings.TrimSpace(date) == "" {
			continue
		}
		parsed, dateErr := time.Parse("2006-01-02", date)
		if dateErr != nil || parsed.Format("2006-01-02") != date {
			return errors.New("chip-distribution provider date is invalid")
		}
		if date > today {
			return errors.New("chip-distribution provider date is in the future")
		}
	}
	if strings.TrimSpace(item.Source) == "" || item.FetchedAt.IsZero() {
		return errors.New("chip-distribution source or fetched_at is missing")
	}
	if item.QualityStatus == model.QualityValid {
		return errors.New("chip-distribution cannot be valid without verified publication availability time")
	}
	if item.QualityStatus != model.QualityDegraded && item.QualityStatus != model.QualityUnknown {
		return fmt.Errorf("chip-distribution quality %q is unsupported", item.QualityStatus)
	}
	return nil
}

func validateCapitalFlowSnapshot(symbol string, item *model.CapitalFlow) error {
	if item == nil {
		return errors.New("capital-flow response is empty")
	}
	canonical, err := normalize.NormalizeSymbol(item.Symbol)
	if err != nil || canonical != symbol || item.Symbol != symbol {
		return errors.New("capital-flow symbol mismatch")
	}
	if strings.TrimSpace(item.ProviderDate) != "" {
		parsed, dateErr := time.Parse("2006-01-02", item.ProviderDate)
		today := time.Now().In(marketLocation()).Format("2006-01-02")
		if dateErr != nil || parsed.Format("2006-01-02") != item.ProviderDate || item.ProviderDate > today {
			return errors.New("capital-flow provider date is invalid or in the future")
		}
	}
	if strings.TrimSpace(item.Source) == "" || item.FetchedAt.IsZero() {
		return errors.New("capital-flow source or fetched_at is missing")
	}
	if item.Observations < 0 {
		return errors.New("capital-flow observation count cannot be negative")
	}
	if item.MainNetInflow != nil && item.Observations < 1 {
		return errors.New("capital-flow main net inflow requires at least one observation")
	}
	if item.Inflow5D != nil && item.Observations < 5 {
		return errors.New("capital-flow five-day sum requires at least five observations")
	}
	if item.Inflow10D != nil && item.Observations < 10 {
		return errors.New("capital-flow ten-day sum requires at least ten observations")
	}
	hasUsableMetric := false
	for _, value := range []*float64{item.MainNetInflow, item.Inflow5D, item.Inflow10D} {
		if value != nil {
			if !isFiniteSupplement(*value) {
				return errors.New("capital-flow amount is non-finite")
			}
			hasUsableMetric = true
		}
	}
	if !hasUsableMetric {
		return errors.New("capital-flow snapshot contains no usable amount metrics")
	}
	if item.QualityStatus == model.QualityValid {
		return errors.New("capital-flow cannot be valid without verified publication availability time")
	}
	if item.QualityStatus != model.QualityDegraded && item.QualityStatus != model.QualityUnknown {
		return fmt.Errorf("capital-flow quality %q is unsupported", item.QualityStatus)
	}
	return nil
}

func (s *DataService) ChipDistribution(ctx context.Context, rawSymbol string) (*model.ChipDistribution, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	chipProviders := make([]chipDistributionSource, 0, len(s.chipDistributionProviders)+len(s.supplementProviders))
	seenChipProviders := make(map[string]struct{}, len(s.chipDistributionProviders)+len(s.supplementProviders))
	for _, p := range s.chipDistributionProviders {
		if _, exists := seenChipProviders[p.Name()]; exists {
			continue
		}
		seenChipProviders[p.Name()] = struct{}{}
		chipProviders = append(chipProviders, p)
	}
	for _, p := range s.supplementProviders {
		if _, exists := seenChipProviders[p.Name()]; exists {
			continue
		}
		seenChipProviders[p.Name()] = struct{}{}
		chipProviders = append(chipProviders, p)
	}
	if len(chipProviders) == 0 {
		return nil, false, errors.New("no enabled chip-distribution provider")
	}
	key := "md:v1:chip:" + symbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var item model.ChipDistribution
		if json.Unmarshal(data, &item) == nil && validateChipDistributionSnapshot(symbol, &item) == nil && time.Since(item.FetchedAt) >= 0 && time.Since(item.FetchedAt) < 5*time.Minute {
			cacheEligible := item.QualityStatus != model.QualityUnknown
			if item.QualityStatus == model.QualityUnknown {
				fallbackExhausted := false
				for _, reason := range item.QualityReasons {
					if reason == "chip_distribution_fallback_exhausted_without_usable_quality" {
						fallbackExhausted = true
						break
					}
				}
				cacheEligible = fallbackExhausted || len(chipProviders) <= 1
			}
			if cacheEligible {
				return &item, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("chip cache read failed", "error", cacheErr)
	}
	var failures []string
	var unknownCandidate *model.ChipDistribution
	for _, p := range chipProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		item, fetchErr := p.FetchChipDistribution(ctx, symbol)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if item == nil {
			failures = append(failures, p.Name()+": empty chip-distribution response")
			continue
		}
		itemSymbol, symbolErr := normalize.NormalizeSymbol(item.Symbol)
		if symbolErr != nil || itemSymbol != symbol {
			failures = append(failures, p.Name()+": chip response symbol mismatch")
			continue
		}
		item.Symbol = symbol
		item.Source = p.Name()
		if strings.TrimSpace(item.SourceVersion) == "" {
			item.SourceVersion = "unknown"
		}
		// fetched_at 表示服务收到回包的时刻，不信任上游同名字段。
		item.FetchedAt = time.Now().UTC()
		if item.QualityStatus == "" {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "provider_quality_status_missing")
		}
		if item.QualityStatus == model.QualityValid {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "publication_availability_timestamp_unverified")
		}
		if err := validateChipDistributionSnapshot(symbol, item); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if item.QualityStatus == model.QualityUnknown {
			// Metrics may be present while their source quality remains unknown; try another source first.
			if unknownCandidate == nil {
				unknownCandidate = item
			}
			failures = append(failures, p.Name()+": chip-distribution quality is unknown")
			continue
		}
		if encoded, encodeErr := json.Marshal(item); encodeErr == nil {
			if cacheErr := s.cache.Set(ctx, key, encoded, 5*time.Minute); cacheErr != nil {
				s.logger.Warn("chip cache write failed", "error", cacheErr)
			}
		}
		return item, false, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if unknownCandidate != nil {
		unknownCandidate.QualityStatus = model.QualityUnknown
		unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, "chip_distribution_fallback_exhausted_without_usable_quality")
		if encoded, encodeErr := json.Marshal(unknownCandidate); encodeErr == nil {
			if cacheErr := s.cache.Set(ctx, key, encoded, 5*time.Minute); cacheErr != nil {
				s.logger.Warn("chip unknown cache write failed", "symbol", symbol, "error", cacheErr)
			}
		}
		return unknownCandidate, false, nil
	}
	return nil, false, fmt.Errorf("all chip-distribution providers failed: %s", strings.Join(failures, " | "))
}

func (s *DataService) CapitalFlow(ctx context.Context, rawSymbol string) (*model.CapitalFlow, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	providers := s.CapitalFlowProviders()
	key := "md:v1:capital-flow:" + symbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var item model.CapitalFlow
		if json.Unmarshal(data, &item) == nil && validateCapitalFlowSnapshot(symbol, &item) == nil && time.Since(item.FetchedAt) >= 0 && time.Since(item.FetchedAt) < 5*time.Minute {
			cacheEligible := item.QualityStatus != model.QualityUnknown
			if item.QualityStatus == model.QualityUnknown {
				fallbackExhausted := false
				for _, reason := range item.QualityReasons {
					if reason == "capital_flow_fallback_exhausted_without_usable_quality" {
						fallbackExhausted = true
						break
					}
				}
				// 多 Provider 场景不让旧 unknown 缓存挡住后备源；只有明确记录回退耗尽才允许命中。
				cacheEligible = fallbackExhausted || len(providers) <= 1
			}
			if cacheEligible {
				return &item, true, nil
			}
		}
	} else if cacheErr != nil {
		s.logger.Warn("capital-flow cache read failed", "error", cacheErr)
	}
	if len(providers) == 0 {
		return nil, false, errors.New("no enabled capital-flow provider")
	}
	var failures []string
	var unknownCandidate *model.CapitalFlow
	for _, p := range providers {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		item, fetchErr := p.FetchCapitalFlow(ctx, symbol)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
		}
		if item == nil {
			failures = append(failures, p.Name()+": empty capital-flow response")
			continue
		}
		itemSymbol, symbolErr := normalize.NormalizeSymbol(item.Symbol)
		if symbolErr != nil || itemSymbol != symbol {
			failures = append(failures, p.Name()+": capital-flow response symbol mismatch")
			continue
		}
		item.Symbol = symbol
		item.Source = p.Name()
		if strings.TrimSpace(item.SourceVersion) == "" {
			item.SourceVersion = "unknown"
		}
		// fetched_at 表示服务收到回包的时刻，不信任上游同名字段。
		item.FetchedAt = time.Now().UTC()
		if item.QualityStatus == "" {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "provider_quality_status_missing")
		}
		if item.QualityStatus == model.QualityValid {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "publication_availability_timestamp_unverified")
		}
		if err := validateCapitalFlowSnapshot(symbol, item); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if item.QualityStatus == model.QualityUnknown {
			// unknown 即使包含数值也不能抢先返回；优先给后续来源机会。
			if unknownCandidate == nil {
				unknownCandidate = item
			}
			failures = append(failures, p.Name()+": capital-flow quality is unknown")
			continue
		}
		if encoded, encodeErr := json.Marshal(item); encodeErr == nil {
			if cacheErr := s.cache.Set(ctx, key, encoded, 5*time.Minute); cacheErr != nil {
				s.logger.Warn("capital-flow cache write failed", "error", cacheErr)
			}
		}
		return item, false, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if unknownCandidate != nil {
		unknownCandidate.QualityStatus = model.QualityUnknown
		unknownCandidate.QualityReasons = appendReason(unknownCandidate.QualityReasons, "capital_flow_fallback_exhausted_without_usable_quality")
		if encoded, encodeErr := json.Marshal(unknownCandidate); encodeErr == nil {
			if cacheErr := s.cache.Set(ctx, key, encoded, 5*time.Minute); cacheErr != nil {
				s.logger.Warn("capital-flow unknown cache write failed", "symbol", symbol, "error", cacheErr)
			}
		}
		return unknownCandidate, false, nil
	}
	return nil, false, fmt.Errorf("all capital-flow providers failed: %s", strings.Join(failures, " | "))
}
func appendReason(reasons []string, value string) []string {
	for _, reason := range reasons {
		if reason == value {
			return reasons
		}
	}
	return append(reasons, value)
}

func isFiniteSupplement(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func (s *DataService) QuoteProviders() []provider.QuoteProvider {
	return append([]provider.QuoteProvider(nil), s.quoteProviders...)
}
func (s *DataService) BarProviders() []provider.DailyBarProvider {
	return append([]provider.DailyBarProvider(nil), s.barProviders...)
}
func (s *DataService) Ready(ctx context.Context) error {
	if err := s.cache.Ping(ctx); err != nil {
		return fmt.Errorf("Redis readiness: %w", err)
	}
	if err := s.store.Ping(ctx); err != nil {
		return fmt.Errorf("MariaDB readiness: %w", err)
	}
	return nil
}

func (s *DataService) Quote(ctx context.Context, rawSymbol string) (*model.Quote, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	key := "md:v4:quote:" + symbol
	if data, ok, e := s.cache.Get(ctx, key); e == nil && ok {
		var q model.Quote
		if json.Unmarshal(data, &q) == nil {
			cachedSymbol, symbolErr := normalize.NormalizeSymbol(q.Symbol)
			if symbolErr == nil && cachedSymbol == symbol {
				q.Symbol = cachedSymbol
			}
			cacheNow := time.Now()
			if symbolErr == nil && cachedSymbol == symbol && q.NormalizationVer == normalize.Version &&
				serviceTimestampIsSane(q.FetchedAt, cacheNow) && serviceTimestampIsSane(q.NormalizedAt, cacheNow) &&
				normalize.ValidateQuote(&q) == nil {
				applyQuoteFreshness(&q, time.Now())
				if quoteCacheEligible(&q) {
					return &q, true, nil
				}
				s.logger.Warn("stale or untrustworthy quote cache payload ignored; refreshing provider", "symbol", symbol, "quality_status", q.QualityStatus, "quality_reasons", q.QualityReasons)
			} else {
				s.logger.Warn("invalid quote cache payload ignored", "symbol", symbol)
			}
		} else {
			s.logger.Warn("unparseable quote cache payload ignored", "symbol", symbol)
		}
	} else if e != nil {
		s.logger.Warn("quote cache read failed", "error", e)
	}
	var errs []string
	timeoutFailures := 0
	var qualityFallback *model.Quote
	fetchID := newFetchID()
	if len(s.quoteProviders) == 0 {
		return nil, false, errors.New("no enabled quote provider")
	}
	for i, p := range s.quoteProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}
		breakerKey := p.Name() + ":quote"
		if !s.breaker.Allow(breakerKey) {
			errs = append(errs, p.Name()+": quote circuit breaker open")
			continue
		}
		q, e := retryQuote(ctx, s.retries, func(callCtx context.Context) (*model.Quote, error) { return p.FetchQuote(callCtx, symbol) })
		if ctx.Err() != nil {
			s.breaker.Abandon(breakerKey)
			return nil, false, ctx.Err()
		}
		if e != nil {
			s.breaker.Failure(breakerKey)
			if errors.Is(e, context.DeadlineExceeded) {
				timeoutFailures++
			}
			safeError := s.logProviderFailure(fetchID, "quote", symbol, p.Name(), e)
			errs = append(errs, p.Name()+": "+safeError)
			continue
		}
		if q == nil {
			s.breaker.Failure(breakerKey)
			errs = append(errs, p.Name()+": empty quote without error")
			continue
		}
		// fetched_at 是服务接收到 Provider 回包的时间，不能信任上游同名字段。
		receivedAt := time.Now().UTC()
		if strings.TrimSpace(q.Symbol) != "" {
			providerSymbol, symbolErr := normalize.NormalizeSymbol(q.Symbol)
			if symbolErr != nil || providerSymbol != symbol {
				s.breaker.Failure(breakerKey)
				errs = append(errs, p.Name()+": provider returned a different or invalid symbol")
				continue
			}
		} else {
			q.QualityStatus = model.QualityDegraded
			appendReasonOnce(&q.QualityReasons, "provider_symbol_missing_assumed_from_request")
		}
		q.Symbol = symbol
		q.Source = p.Name()
		q.RequestID = fetchID
		q.FetchedAt = receivedAt
		if q.NormalizationVer == "" {
			q.NormalizationVer = normalize.Version
		}
		if i > 0 {
			q.QualityStatus = model.QualityDegraded
			appendReasonOnce(&q.QualityReasons, "provider_fallback_used")
			q.FallbackFrom = s.quoteProviders[0].Name()
			q.FallbackReason = strings.Join(errs, "; ")
		}
		if q.Amount == nil {
			q.QualityStatus = model.QualityDegraded
			appendReasonOnce(&q.QualityReasons, "amount_missing_from_provider")
		}
		if e = normalize.ValidateQuote(q); e != nil {
			s.breaker.Failure(breakerKey)
			errs = append(errs, p.Name()+" failed contract validation: "+e.Error())
			continue
		}
		applyQuoteFreshness(q, time.Now())
		q.NormalizedAt = time.Now().UTC()
		if !quoteCacheEligible(q) {
			s.breaker.Failure(breakerKey)
			errs = append(errs, p.Name()+": quote not suitable as a fresh live snapshot: "+strings.Join(q.QualityReasons, ","))
			copy := *q
			copy.QualityReasons = append([]string(nil), q.QualityReasons...)
			qualityFallback = &copy
			continue
		}
		s.breaker.Success(breakerKey)
		if s.store != nil {
			if e := s.store.SaveQuote(ctx, q); e != nil {
				s.logger.Error("persist quote failed", "symbol", symbol, "source", q.Source, "error", e)
				appendReasonOnce(&q.QualityReasons, "persistence_write_failed")
				if q.QualityStatus == model.QualityValid {
					q.QualityStatus = model.QualityDegraded
				}
			}
		}
		if encoded, e := json.Marshal(q); e == nil {
			if e = s.cache.Set(ctx, key, encoded, s.quoteTTL); e != nil {
				s.logger.Warn("quote cache write failed", "error", e)
			}
		}
		return q, false, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, false, err
	}
	if qualityFallback != nil {
		appendReasonOnce(&qualityFallback.QualityReasons, "no_provider_returned_a_fresh_timestamped_quote")
		return qualityFallback, false, nil
	}
	if timeoutFailures == len(s.quoteProviders) && timeoutFailures > 0 {
		return nil, false, fmt.Errorf("all quote providers timed out: %w; details: %s", context.DeadlineExceeded, strings.Join(errs, " | "))
	}
	return nil, false, fmt.Errorf("all quote providers failed: %s", strings.Join(errs, " | "))
}

func (s *DataService) DailyBars(ctx context.Context, rawSymbol, start, end string, limit int, basis model.AdjustmentBasis) ([]model.DailyBar, bool, []string, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, nil, err
	}
	if limit <= 0 {
		limit = s.defaultBars
	}
	if limit > s.maxBars {
		limit = s.maxBars
	}
	if start != "" {
		parsed, err := time.Parse("2006-01-02", start)
		if err != nil || parsed.Format("2006-01-02") != start {
			return nil, false, nil, fmt.Errorf("invalid start date %q", start)
		}
	}
	if end != "" {
		parsed, err := time.Parse("2006-01-02", end)
		if err != nil || parsed.Format("2006-01-02") != end {
			return nil, false, nil, fmt.Errorf("invalid end date %q", end)
		}
	} else {
		// Never ask a Provider for an unbounded future window and then trust its
		// response blindly. Some quote endpoints use a distant sentinel end date.
		// Canonicalizing the implicit end date also prevents cache entries from
		// silently crossing the exchange-local date boundary.
		end = time.Now().In(marketLocation()).Format("2006-01-02")
	}
	today := time.Now().In(marketLocation()).Format("2006-01-02")
	if end > today {
		return nil, false, nil, errors.New("end date must not be after today in Asia/Shanghai")
	}
	if start != "" && start > end {
		return nil, false, nil, errors.New("start date must not be after end date")
	}
	if basis != model.AdjustmentNone && basis != model.AdjustmentForward && basis != model.AdjustmentBackward {
		return nil, false, nil, fmt.Errorf("unsupported adjustment basis %q", basis)
	}
	key := fmt.Sprintf("md:v4:bars:%s:%s:%s:%s:%d", symbol, start, end, basis, limit)
	if data, ok, e := s.cache.Get(ctx, key); e == nil && ok {
		var v struct {
			Bars     []model.DailyBar `json:"bars"`
			Warnings []string         `json:"warnings"`
		}
		if json.Unmarshal(data, &v) == nil {
			candidates := make([]model.DailyBar, 0, len(v.Bars))
			contractMismatch := false
			cacheNow := time.Now()
			for _, bar := range v.Bars {
				cachedSymbol, symbolErr := normalize.NormalizeSymbol(bar.Symbol)
				if symbolErr != nil || cachedSymbol != symbol || bar.AdjustmentBasis != basis || bar.NormalizationVer != normalize.Version ||
					!serviceTimestampIsSane(bar.FetchedAt, cacheNow) || !serviceTimestampIsSane(bar.NormalizedAt, cacheNow) {
					contractMismatch = true
					break
				}
				candidates = append(candidates, bar)
			}
			if !contractMismatch {
				validated, warnings := normalize.ValidateAndSortBars(symbol, candidates, start, end, limit)
				if len(validated) > 0 {
					if dailyBarsCacheNeedsPostCloseRefresh(validated, time.Now()) {
						s.logger.Info("daily-bar cache has today's partial candle after close; refreshing provider", "symbol", symbol, "trade_date", validated[len(validated)-1].TradeDate)
					} else if coverageSufficient(start, end, limit, len(validated)) {
						mergedWarnings := append(append([]string(nil), v.Warnings...), warnings...)
						return validated, true, dedupeStrings(mergedWarnings), nil
					} else {
						s.logger.Warn("daily-bar cache coverage is insufficient; refreshing providers", "symbol", symbol, "requested", limit, "returned", len(validated))
					}
				}
			}
			s.logger.Warn("invalid daily-bar cache payload ignored", "symbol", symbol, "adjustment", basis, "contract_mismatch", contractMismatch)
		} else {
			s.logger.Warn("unparseable daily-bar cache payload ignored", "symbol", symbol)
		}
	} else if e != nil {
		s.logger.Warn("daily bars cache read failed", "error", e)
	}
	var errs []string
	timeoutFailures := 0
	var bestPartial []model.DailyBar
	var bestPartialWarnings []string
	fetchID := newFetchID()
	if len(s.barProviders) == 0 {
		return nil, false, nil, errors.New("no enabled daily-bar provider")
	}
	for i, p := range s.barProviders {
		if err := ctx.Err(); err != nil {
			return nil, false, nil, err
		}
		breakerKey := p.Name() + ":daily_bars"
		if !s.breaker.Allow(breakerKey) {
			errs = append(errs, p.Name()+": daily-bars circuit breaker open")
			continue
		}
		bars, e := retryBars(ctx, s.retries, func(callCtx context.Context) ([]model.DailyBar, error) {
			return p.FetchDailyBars(callCtx, symbol, start, end, limit, basis)
		})
		if ctx.Err() != nil {
			s.breaker.Abandon(breakerKey)
			return nil, false, nil, ctx.Err()
		}
		if e != nil {
			s.breaker.Failure(breakerKey)
			if errors.Is(e, context.DeadlineExceeded) {
				timeoutFailures++
			}
			safeError := s.logProviderFailure(fetchID, "daily_bars", symbol, p.Name(), e)
			errs = append(errs, p.Name()+": "+safeError)
			continue
		}
		receivedAt := time.Now().UTC()
		if len(bars) == 0 {
			s.breaker.Failure(breakerKey)
			errs = append(errs, p.Name()+": empty daily-bar response without error")
			continue
		}
		contractError := ""
		for _, bar := range bars {
			if strings.TrimSpace(bar.Symbol) != "" {
				providerSymbol, symbolErr := normalize.NormalizeSymbol(bar.Symbol)
				if symbolErr != nil || providerSymbol != symbol {
					contractError = "provider returned a different or invalid symbol in daily bars"
					break
				}
			}
			if bar.AdjustmentBasis != basis {
				contractError = "provider returned K-lines with an adjustment basis different from the requested basis"
				break
			}
		}
		if contractError != "" {
			s.breaker.Failure(breakerKey)
			errs = append(errs, p.Name()+": "+contractError)
			continue // Never salvage only part of a provider response with a mixed contract.
		}
		accepted := make([]model.DailyBar, 0, len(bars))
		for j := range bars {
			bars[j].Symbol = symbol
			bars[j].Source = p.Name()
			bars[j].RequestID = fetchID
			if isCurrentIncompleteSessionBar(bars[j].TradeDate, time.Now()) {
				bars[j].IsPartial = true
				bars[j].QualityStatus = model.QualityDegraded
				appendReasonOnce(&bars[j].QualityReasons, "current_daily_bar_may_be_incomplete")
			}
			bars[j].FetchedAt = receivedAt
			if bars[j].NormalizationVer == "" {
				bars[j].NormalizationVer = normalize.Version
			}
			if bars[j].Amount == nil {
				bars[j].QualityStatus = model.QualityDegraded
				appendReasonOnce(&bars[j].QualityReasons, "amount_missing_from_provider")
			}
			if i > 0 {
				bars[j].QualityStatus = model.QualityDegraded
				bars[j].FallbackFrom = s.barProviders[0].Name()
				bars[j].FallbackReason = strings.Join(errs, "; ")
				appendReasonOnce(&bars[j].QualityReasons, "provider_fallback_used")
			}
			accepted = append(accepted, bars[j])
		}
		bars = accepted
		if len(bars) == 0 {
			s.breaker.Failure(breakerKey)
			errs = append(errs, p.Name()+": no bars matched the requested symbol and adjustment basis")
			continue
		}
		bars, warnings := normalize.ValidateAndSortBars(symbol, bars, start, end, limit)
		if len(bars) == 0 {
			s.breaker.Failure(breakerKey)
			errs = append(errs, p.Name()+" response failed bar validation")
			continue
		}
		normalizedAt := time.Now().UTC()
		for index := range bars {
			bars[index].NormalizedAt = normalizedAt
		}
		// Surface material source/field limitations at the response level too;
		// callers should not need to inspect every candle to discover fallback.
		if i > 0 {
			warnings = append(warnings, "daily_bar_provider_fallback_used")
		}
		for _, bar := range bars {
			if bar.Amount == nil {
				warnings = append(warnings, "daily_bar_amount_missing_from_selected_provider")
				break
			}
		}
		warnings = dedupeStrings(warnings)
		if !coverageSufficient(start, end, limit, len(bars)) {
			s.breaker.Failure(breakerKey)
			warning := fmt.Sprintf("%s returned partial daily-bar coverage: requested %d bars, validated %d; tried fallback providers before using this degraded series", p.Name(), limit, len(bars))
			warnings = append(warnings, warning)
			for j := range bars {
				if bars[j].QualityStatus == model.QualityValid {
					bars[j].QualityStatus = model.QualityDegraded
				}
				appendReasonOnce(&bars[j].QualityReasons, "daily_bar_coverage_below_threshold")
			}
			if preferPartialBars(bars, bestPartial) {
				bestPartial = append([]model.DailyBar(nil), bars...)
				bestPartialWarnings = append([]string(nil), warnings...)
			}
			errs = append(errs, warning)
			continue
		}
		s.breaker.Success(breakerKey)
		if s.store != nil {
			if e := s.store.UpsertBars(ctx, bars); e != nil {
				s.logger.Error("persist daily bars failed", "symbol", symbol, "source", p.Name(), "error", e)
				warnings = append(warnings, "persistence_write_failed")
				for j := range bars {
					appendReasonOnce(&bars[j].QualityReasons, "persistence_write_failed")
					if bars[j].QualityStatus == model.QualityValid {
						bars[j].QualityStatus = model.QualityDegraded
					}
				}
			}
		}
		if encoded, e := json.Marshal(struct {
			Bars     []model.DailyBar `json:"bars"`
			Warnings []string         `json:"warnings"`
		}{bars, warnings}); e == nil {
			if e = s.cache.Set(ctx, key, encoded, s.barsTTL); e != nil {
				s.logger.Warn("daily bars cache write failed", "error", e)
			}
		}
		return bars, false, warnings, nil
	}
	if s.store != nil {
		cached, e := s.store.ListBars(ctx, symbol, start, end, basis, limit)
		if e == nil && len(cached) > 0 {
			coherent, coherentReason := coherentStoredSeries(cached, basis)
			if len(coherent) == 0 {
				errs = append(errs, "MariaDB fallback rejected: "+coherentReason)
				s.logger.Warn("MariaDB fallback rejected because stored bars are not a coherent series", "symbol", symbol, "adjustment", basis, "reason", coherentReason)
			} else {
				filtered := make([]model.DailyBar, 0, len(coherent))
				contractMismatch := false
				for _, bar := range coherent {
					cachedSymbol, symbolErr := normalize.NormalizeSymbol(bar.Symbol)
					if symbolErr != nil || cachedSymbol != symbol || bar.AdjustmentBasis != basis || bar.NormalizationVer != normalize.Version {
						contractMismatch = true
						break
					}
					filtered = append(filtered, bar)
				}
				if contractMismatch {
					s.logger.Error("MariaDB fallback rejected due to stored symbol/adjustment/normalization contract mismatch", "symbol", symbol, "adjustment", basis, "normalization_version", normalize.Version)
					errs = append(errs, "MariaDB fallback rejected due to symbol/adjustment/normalization contract mismatch")
				} else {
					cached, _ = normalize.ValidateAndSortBars(symbol, filtered, start, end, limit)
				}
				if !contractMismatch && len(cached) > 0 {
					for i := range cached {
						cached[i].QualityStatus = model.QualityStale
						appendReasonOnce(&cached[i].QualityReasons, "provider_unavailable_served_from_mariadb")
					}
					// Use the database snapshot only when its coherent series covers
					// at least as recent a trade date and at least as much history as
					// the best provider partial. Never merge their price series.
					if len(bestPartial) == 0 || preferStoredSeries(cached, bestPartial) {
						return cached, false, []string{"provider_unavailable_served_from_mariadb", "mariadb_bars_use_one_coherent_source_and_adjustment_reference"}, nil
					}
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, false, nil, err
	}
	if len(bestPartial) > 0 {
		return bestPartial, false, dedupeStrings(bestPartialWarnings), nil
	}
	if timeoutFailures == len(s.barProviders) && timeoutFailures > 0 {
		return nil, false, nil, fmt.Errorf("all daily-bar providers timed out: %w; details: %s", context.DeadlineExceeded, strings.Join(errs, " | "))
	}
	if len(errs) == 0 {
		errs = append(errs, "no enabled provider")
	}
	return nil, false, nil, errors.New("all daily-bar providers failed: " + strings.Join(errs, " | "))
}

func (s *DataService) Bundle(ctx context.Context, rawSymbol string, lookback int, asOf time.Time, asOfProvided bool, basis model.AdjustmentBasis) (*model.Bundle, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if asOfProvided && asOf.After(time.Now()) {
		return nil, errors.New("as_of must not be in the future")
	}
	if lookback <= 0 {
		lookback = s.defaultBars
	}
	if lookback > s.maxBars {
		lookback = s.maxBars
	}
	end := ""
	excludeAsOfDate := false
	quoteAllowed := !asOfProvided
	if asOfProvided {
		localAsOf := asOf.In(marketLocation())
		end = localAsOf.Format("2006-01-02")
		// Do not subtract a calendar day here: the preceding date may be a
		// weekend or holiday. Fetch through the requested date, then remove that
		// date's full-session candle when the cutoff is before market close.
		excludeAsOfDate = localAsOf.Hour() < 15 || (localAsOf.Hour() == 15 && localAsOf.Minute() < 5)
		age := time.Since(asOf)
		quoteAllowed = age >= 0 && age <= 2*time.Minute
	}
	requestLookback := lookback
	if excludeAsOfDate && requestLookback < s.maxBars {
		requestLookback++ // keep the requested count after dropping the cutoff date
	}
	bars, _, warnings, err := s.DailyBars(ctx, symbol, "", end, requestLookback, basis)
	if err != nil {
		return nil, err
	}
	if excludeAsOfDate {
		cutoffDate := asOf.In(marketLocation()).Format("2006-01-02")
		filtered := make([]model.DailyBar, 0, len(bars))
		for _, bar := range bars {
			if bar.TradeDate == cutoffDate {
				continue
			}
			filtered = append(filtered, bar)
		}
		bars = filtered
		warnings = append(warnings, "as_of_date_daily_bar_excluded_before_market_close")
	}
	bundle := &model.Bundle{Symbol: symbol, DailyBars: bars, Sources: map[string]string{}, Warnings: append([]string{}, warnings...), QualityStatus: model.QualityValid}
	if asOfProvided && basis != model.AdjustmentNone && time.Since(asOf) > 5*time.Minute {
		bundle.Warnings = append(bundle.Warnings, "historical_adjusted_bars_may_use_current_corporate_action_factors; point_in_time_adjustment_not_verified")
		bundle.QualityStatus = model.QualityDegraded
	}
	if normalize.HasQualityDegradingBarWarnings(warnings) {
		bundle.QualityStatus = model.QualityDegraded
	}
	if len(bars) > 0 {
		sourceSet := map[string]struct{}{}
		for _, b := range bars {
			if b.Source != "" {
				sourceSet[b.Source] = struct{}{}
			}
			if b.QualityStatus != model.QualityValid {
				bundle.QualityStatus = model.QualityDegraded
			}
		}
		sources := make([]string, 0, len(sourceSet))
		for source := range sourceSet {
			sources = append(sources, source)
		}
		sort.Strings(sources)
		bundle.Sources["daily_bars"] = strings.Join(sources, ",")
		if len(sources) > 1 {
			bundle.Warnings = append(bundle.Warnings, "daily_bars_mixed_sources; inspect per-bar source metadata before source-sensitive analysis")
			bundle.QualityStatus = model.QualityDegraded
		}
	}
	if quoteAllowed {
		q, _, e := s.Quote(ctx, symbol)
		if e != nil {
			bundle.Warnings = append(bundle.Warnings, "realtime_quote_unavailable")
			bundle.QualityStatus = model.QualityDegraded
		} else if asOfProvided && (q.QuoteTime == nil || q.QuoteTime.After(asOf)) {
			// A near-live as_of request may fetch a quote newer than the exact
			// cutoff. Missing timestamps cannot prove point-in-time availability.
			bundle.Warnings = append(bundle.Warnings, "realtime_quote_excluded_after_or_unknown_as_of_timestamp")
			bundle.QualityStatus = model.QualityDegraded
		} else {
			bundle.Quote = q
			bundle.Name = q.Name
			bundle.Sources["realtime_quote"] = q.Source
			if len(bars) > 0 && bars[len(bars)-1].AdjustmentBasis != q.AdjustmentBasis {
				bundle.Warnings = append(bundle.Warnings, "quote_and_daily_bars_adjustment_basis_mismatch")
				bundle.Warnings = append(bundle.Warnings, "do_not_compare_realtime_unadjusted_price_directly_with_adjusted_history")
				bundle.QualityStatus = model.QualityDegraded
			}
			if q.QualityStatus != model.QualityValid {
				bundle.QualityStatus = model.QualityDegraded
			}
		}
	} else {
		bundle.Warnings = append(bundle.Warnings, "realtime_quote_skipped_as_of_not_live; latest quote must not leak into historical analysis")
		bundle.QualityStatus = model.QualityDegraded
	}
	if len(bars) == 0 {
		bundle.QualityStatus = model.QualityDegraded
		bundle.Warnings = append(bundle.Warnings, "daily_bars_missing")
	}
	bundle.SnapshotID = newFetchID()
	bundle.FetchedAt = time.Now().UTC()
	if asOfProvided {
		bundle.AnalysisReferenceTime = asOf.UTC()
	} else {
		bundle.AnalysisReferenceTime = bundle.FetchedAt
	}
	return bundle, nil
}

// coverageSufficient keeps obviously truncated responses as fallback candidates.
// Explicit date ranges use weekdays as a conservative estimate because this
// service does not yet own an authoritative exchange calendar; the threshold is
// deliberately accounts for exchange holidays while treating visibly truncated
// ranges as fallback candidates. Exhausted fallback candidates are returned only
// as degraded data, never silently treated as complete.
// preferPartialBars keeps the largest validated partial series and, on ties,
// chooses the series with the newest trade date. Equal coverage and recency
// preserve provider priority by retaining the existing candidate.
func preferPartialBars(candidate, current []model.DailyBar) bool {
	if len(candidate) == 0 {
		return false
	}
	if len(current) == 0 || len(candidate) > len(current) {
		return true
	}
	if len(candidate) < len(current) {
		return false
	}
	return candidate[len(candidate)-1].TradeDate > current[len(current)-1].TradeDate
}

func coverageSufficient(start, end string, requested, returned int) bool {
	if returned <= 0 {
		return false
	}
	if strings.TrimSpace(start) == "" {
		if requested < 20 {
			return true
		}
		minimum := (requested*4 + 4) / 5 // require at least 80% for lookback requests
		if minimum < 20 {
			minimum = 20
		}
		return returned >= minimum
	}
	startDate, startErr := time.Parse("2006-01-02", start)
	endDate, endErr := time.Parse("2006-01-02", end)
	if startErr != nil || endErr != nil || startDate.After(endDate) {
		return false
	}
	expected := estimatedWeekdays(startDate, endDate)
	if requested > 0 && expected > requested {
		expected = requested
	}
	if expected == 0 {
		return returned > 0
	}
	// Weekdays overstate actual sessions because exchange holidays are not
	// represented here. Requiring 65% of weekdays is a pragmatic conservative
	// floor; a full exchange calendar should replace this estimate later.
	minimum := (expected*65 + 99) / 100
	if minimum < 1 {
		minimum = 1
	}
	return returned >= minimum
}

func estimatedWeekdays(start, end time.Time) int {
	// Requests are capped at 2,000 bars. Clip older ranges to 30 years because
	// dates farther in the past cannot reasonably increase the required coverage
	// threshold beyond that cap. This also avoids work proportional to an
	// attacker-controlled year-0001 start date.
	if start.Before(end.AddDate(-30, 0, 0)) {
		start = end.AddDate(-30, 0, 0)
	}
	count := 0
	for day := start; !day.After(end); day = day.AddDate(0, 0, 1) {
		if day.Weekday() != time.Saturday && day.Weekday() != time.Sunday {
			count++
		}
	}
	return count
}

func dailyBarsCacheNeedsPostCloseRefresh(bars []model.DailyBar, now time.Time) bool {
	if len(bars) == 0 {
		return false
	}
	loc := marketLocation()
	local := now.In(loc)
	if local.Weekday() == time.Saturday || local.Weekday() == time.Sunday || local.Hour()*60+local.Minute() < 15*60+5 {
		return false
	}
	today := local.Format("2006-01-02")
	for _, bar := range bars {
		if bar.TradeDate == today && bar.IsPartial {
			return true
		}
	}
	return false
}

// coherentStoredSeries refuses to splice providers together. Adjusted series
// additionally require a known common reference date; current Providers do not
// expose that value, so adjusted MariaDB fallbacks are deliberately unavailable
// until a source can verify the adjustment reference.
func preferStoredSeries(stored, providerPartial []model.DailyBar) bool {
	if len(stored) == 0 {
		return false
	}
	if len(providerPartial) == 0 {
		return true
	}
	storedNewest := stored[len(stored)-1].TradeDate
	partialNewest := providerPartial[len(providerPartial)-1].TradeDate
	if storedNewest != partialNewest {
		return storedNewest > partialNewest
	}
	return len(stored) >= len(providerPartial)
}

func coherentStoredSeries(bars []model.DailyBar, basis model.AdjustmentBasis) ([]model.DailyBar, string) {
	type group struct {
		bars   []model.DailyBar
		newest string
	}
	groups := map[string]*group{}
	for _, bar := range bars {
		source := strings.TrimSpace(bar.Source)
		if source == "" {
			continue
		}
		refDate := strings.TrimSpace(bar.AdjustmentRefDate)
		if basis != model.AdjustmentNone && refDate == "" {
			continue
		}
		key := source + "|" + refDate + "|" + bar.NormalizationVer
		g := groups[key]
		if g == nil {
			g = &group{}
			groups[key] = g
		}
		g.bars = append(g.bars, bar)
		if bar.TradeDate > g.newest {
			g.newest = bar.TradeDate
		}
	}
	var best *group
	for _, g := range groups {
		if best == nil || len(g.bars) > len(best.bars) || (len(g.bars) == len(best.bars) && g.newest > best.newest) {
			best = g
		}
	}
	if best == nil {
		if basis != model.AdjustmentNone {
			return nil, "adjusted bars have no verified adjustment reference date; refusing to mix unknown historical adjustment snapshots"
		}
		return nil, "stored bars do not contain a source-coherent normalized series"
	}
	return best.bars, ""
}

func newFetchID() string { return time.Now().UTC().Format("20060102T150405.000000000Z") }

// serviceTimestampIsSane rejects missing/future service timestamps in persisted payloads.
func serviceTimestampIsSane(value, now time.Time) bool {
	return !value.IsZero() && !value.After(now.Add(5*time.Minute))
}

func quoteCacheEligible(q *model.Quote) bool {
	if q == nil || q.QualityStatus == model.QualityStale || q.QualityStatus == model.QualityInvalid || q.QualityStatus == model.QualityUnknown {
		return false
	}
	if q.QuoteTime == nil {
		return false
	}
	for _, reason := range q.QualityReasons {
		switch reason {
		case "provider_timestamp_missing", "quote_older_than_180_seconds_during_trading_clock", "quote_older_than_24_hours_outside_trading_clock", "provider_timestamp_more_than_60_seconds_in_future", "volume_unit_assumed_without_turnover_crosscheck", "volume_unit_unverified":
			return false
		}
	}
	return true
}

func applyQuoteFreshness(q *model.Quote, now time.Time) {
	if q == nil {
		return
	}
	if q.QuoteTime == nil {
		if q.QualityStatus == model.QualityValid || q.QualityStatus == "" {
			q.QualityStatus = model.QualityDegraded
		}
		appendReasonOnce(&q.QualityReasons, "provider_timestamp_missing")
		return
	}
	age := now.Sub(*q.QuoteTime)
	// Clock skew is suspicious regardless of whether the market is open.
	if age < -60*time.Second {
		if q.QualityStatus != model.QualityStale {
			q.QualityStatus = model.QualityDegraded
		}
		appendReasonOnce(&q.QualityReasons, "provider_timestamp_more_than_60_seconds_in_future")
		return
	}
	if isTradingClock(now) {
		if age > 180*time.Second {
			q.QualityStatus = model.QualityStale
			appendReasonOnce(&q.QualityReasons, "quote_older_than_180_seconds_during_trading_clock")
		}
		return
	}
	// Off-session quotes may legitimately stop updating. A full-day-old quote is
	// nevertheless not a current snapshot and must not retain a valid status.
	if age > 24*time.Hour {
		q.QualityStatus = model.QualityStale
		appendReasonOnce(&q.QualityReasons, "quote_older_than_24_hours_outside_trading_clock")
	}
}

// providerFailureURLQuery redacts query parameters from URLs embedded in HTTP client errors.
// Provider URLs often include public tokens or request-specific fields; logs only need the endpoint and failure type.
var providerFailureURLQuery = regexp.MustCompile(`(?i)(https?://[^\s"?]+)\?[^\s"']*`)

func providerFailureSummary(err error) (kind, message string) {
	if err == nil {
		return "unknown", "provider returned an unspecified error"
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		kind = "timeout"
	case errors.Is(err, context.Canceled):
		kind = "canceled"
	default:
		lower := strings.ToLower(err.Error())
		switch {
		case strings.Contains(lower, "network is unreachable"),
			strings.Contains(lower, "no route to host"),
			strings.Contains(lower, "connection refused"),
			strings.Contains(lower, "dial tcp"):
			kind = "network_connect"
		case strings.Contains(lower, "eof"):
			kind = "connection_closed"
		case strings.Contains(lower, "http status"):
			kind = "http_status"
		case strings.Contains(lower, "empty"),
			strings.Contains(lower, "no parsable"),
			strings.Contains(lower, "empty daily-bar response"):
			kind = "empty_or_unparseable_response"
		case strings.Contains(lower, "validation"),
			strings.Contains(lower, "symbol mismatch"),
			strings.Contains(lower, "contract"),
			strings.Contains(lower, "inconsistent"),
			strings.Contains(lower, "ohlc invariant"),
			strings.Contains(lower, "volume unit cannot be confirmed"):
			kind = "data_contract"
		default:
			kind = "provider_error"
		}
	}
	message = providerFailureURLQuery.ReplaceAllString(err.Error(), `$1?[query redacted]`)
	const maxMessageLength = 500
	if len(message) > maxMessageLength {
		message = message[:maxMessageLength] + "…"
	}
	return kind, message
}

func (s *DataService) logProviderFailure(fetchID, capability, symbol, providerName string, err error) string {
	kind, message := providerFailureSummary(err)
	s.logger.Warn("market data provider failed",
		"fetch_id", fetchID,
		"capability", capability,
		"symbol", symbol,
		"provider", providerName,
		"error_kind", kind,
		"error", message,
	)
	return message
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func appendReasonOnce(reasons *[]string, reason string) {
	for _, existing := range *reasons {
		if existing == reason {
			return
		}
	}
	*reasons = append(*reasons, reason)
}

func isTradingClock(t time.Time) bool {
	t = t.In(marketLocation())
	if t.Weekday() == time.Saturday || t.Weekday() == time.Sunday {
		return false
	}
	m := t.Hour()*60 + t.Minute()
	return (m >= 9*60+30 && m <= 11*60+30) || (m >= 13*60 && m <= 15*60)
}

func isCurrentIncompleteSessionBar(tradeDate string, now time.Time) bool {
	loc := marketLocation()
	local := now.In(loc)
	if tradeDate != local.Format("2006-01-02") || local.Weekday() == time.Saturday || local.Weekday() == time.Sunday {
		return false
	}
	// A daily candle remains incomplete throughout the lunch break and the
	// post-close publication window. Testing only active trading hours would
	// incorrectly mark the morning-only candle as complete at 11:45.
	minutes := local.Hour()*60 + local.Minute()
	return minutes < 15*60+5
}

func marketLocation() *time.Location {
	l, e := time.LoadLocation("Asia/Shanghai")
	if e != nil {
		return time.FixedZone("CST", 8*3600)
	}
	return l
}
func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
