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

type DataService struct {
	providers              []provider.NamedProvider
	quoteProviders         []provider.QuoteProvider
	barProviders           []provider.DailyBarProvider
	stockBasicProviders    []provider.StockBasicProvider
	sectorRankingProviders []provider.SectorRankingProvider
	supplementProviders    []provider.SupplementProvider
	maxBars                int
	retries                int
	breaker                *circuitBreaker
	cache                  cache.Cache
	store                  *repository.Store
	quoteTTL               time.Duration
	barsTTL                time.Duration
	defaultBars            int
	logger                 *slog.Logger
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

func (s *DataService) SetSectorRankingProviders(providers []provider.SectorRankingProvider) {
	s.sectorRankingProviders = append([]provider.SectorRankingProvider(nil), providers...)
}

func (s *DataService) SectorRankingProviders() []provider.SectorRankingProvider {
	return append([]provider.SectorRankingProvider(nil), s.sectorRankingProviders...)
}

func validateSectorRankingSnapshot(category string, limit int, snapshot *model.SectorRankingSnapshot) error {
	if snapshot == nil {
		return errors.New("sector-ranking provider returned an empty response")
	}
	if snapshot.Category != category {
		return fmt.Errorf("sector-ranking category mismatch: requested %s, got %q", category, snapshot.Category)
	}
	if strings.TrimSpace(snapshot.Source) == "" || snapshot.FetchedAt.IsZero() {
		return errors.New("sector-ranking source or fetched_at is missing")
	}
	if snapshot.QualityStatus != model.QualityValid && snapshot.QualityStatus != model.QualityDegraded {
		return fmt.Errorf("sector-ranking quality status %q is unsupported", snapshot.QualityStatus)
	}
	if len(snapshot.Items) == 0 || len(snapshot.Items) > limit {
		return fmt.Errorf("sector-ranking item count %d is outside 1..%d", len(snapshot.Items), limit)
	}
	seen := make(map[string]bool, len(snapshot.Items))
	for _, item := range snapshot.Items {
		if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" || item.Category != category {
			return errors.New("sector-ranking item has missing code/name or mismatched category")
		}
		if item.Source != snapshot.Source {
			return errors.New("sector-ranking item source differs from its snapshot source")
		}
		if item.FetchedAt.IsZero() || (item.QualityStatus != model.QualityValid && item.QualityStatus != model.QualityDegraded) {
			return errors.New("sector-ranking item has invalid retrieval time or quality status")
		}
		if seen[item.Code] {
			return fmt.Errorf("sector-ranking duplicate board code %q", item.Code)
		}
		seen[item.Code] = true
	}
	return nil
}

func (s *DataService) SectorRankings(ctx context.Context, rawCategory string, limit int) (*model.SectorRankingSnapshot, bool, error) {
	category := strings.ToLower(strings.TrimSpace(rawCategory))
	if category != "industry" && category != "concept" {
		return nil, false, fmt.Errorf("unsupported sector category %q; want industry or concept", rawCategory)
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
	key := fmt.Sprintf("md:v1:sector-ranking:%s:%d", category, limit)
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
		snapshot, fetchErr := p.FetchSectorRankings(ctx, category, limit)
		if fetchErr != nil {
			failures = append(failures, p.Name()+": "+fetchErr.Error())
			continue
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
	key := "md:v1:stock-basic:" + symbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var item model.StockBasic
		if json.Unmarshal(data, &item) == nil && validateStockBasic(symbol, &item) == nil &&
			item.QualityStatus == model.QualityValid && time.Since(item.FetchedAt) >= 0 && time.Since(item.FetchedAt) < ttl {
			return &item, true, nil
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
			item.QualityStatus = model.QualityValid
		}
		if item.NormalizationVer == "" {
			item.NormalizationVer = normalize.Version
		}
		if err := validateStockBasic(symbol, item); err != nil {
			failures = append(failures, p.Name()+": "+err.Error())
			continue
		}
		if item.QualityStatus != model.QualityValid {
			failures = append(failures, p.Name()+": stock-basic quality status is not valid")
			continue
		}
		payload, marshalErr := json.Marshal(item)
		if marshalErr == nil {
			if cacheErr := s.cache.Set(ctx, key, payload, ttl); cacheErr != nil {
				s.logger.Warn("stock-basic cache write failed", "symbol", symbol, "error", cacheErr)
			}
		} else {
			s.logger.Warn("stock-basic cache encode failed", "symbol", symbol, "error", marshalErr)
		}
		return item, false, nil
	}
	return nil, false, fmt.Errorf("all stock-basic providers failed: %s", strings.Join(failures, "; "))
}

// SetSupplementProviders 注册显式启用的非价格类补充数据源。
// 此类 Provider 不会被加入实时行情或日 K 的兜底链。
func (s *DataService) SetSupplementProviders(providers []provider.SupplementProvider) {
	s.supplementProviders = append([]provider.SupplementProvider(nil), providers...)
}

func (s *DataService) SupplementProviders() []provider.SupplementProvider {
	return append([]provider.SupplementProvider(nil), s.supplementProviders...)
}

func (s *DataService) ChipDistribution(ctx context.Context, rawSymbol string) (*model.ChipDistribution, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	key := "md:v1:chip:" + symbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var item model.ChipDistribution
		if json.Unmarshal(data, &item) == nil && item.Symbol == symbol && item.AverageCost > 0 && time.Since(item.FetchedAt) >= 0 && time.Since(item.FetchedAt) < 5*time.Minute {
			return &item, true, nil
		}
	} else if cacheErr != nil {
		s.logger.Warn("chip cache read failed", "error", cacheErr)
	}
	if len(s.supplementProviders) == 0 {
		return nil, false, errors.New("no enabled chip-distribution provider")
	}
	var failures []string
	for _, p := range s.supplementProviders {
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
		if symbolErr != nil || itemSymbol != symbol || item.AverageCost <= 0 || !isFiniteSupplement(item.AverageCost) {
			failures = append(failures, p.Name()+": chip response failed symbol or average-cost validation")
			continue
		}
		ratioInvalid := false
		for _, ratio := range []*float64{item.ProfitRatio, item.Concentration90, item.Concentration70} {
			if ratio != nil && (!isFiniteSupplement(*ratio) || *ratio < 0 || *ratio > 1) {
				ratioInvalid = true
				break
			}
		}
		if ratioInvalid {
			failures = append(failures, p.Name()+": chip response contains a ratio outside [0,1]")
			continue
		}
		item.Symbol = symbol
		item.Source = p.Name()
		if strings.TrimSpace(item.SourceVersion) == "" {
			item.SourceVersion = "unknown"
		}
		if item.FetchedAt.IsZero() {
			item.FetchedAt = time.Now().UTC()
		}
		if item.QualityStatus == "" {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "provider_quality_status_missing")
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
	return nil, false, fmt.Errorf("all chip-distribution providers failed: %s", strings.Join(failures, " | "))
}

func (s *DataService) CapitalFlow(ctx context.Context, rawSymbol string) (*model.CapitalFlow, bool, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, false, err
	}
	key := "md:v1:capital-flow:" + symbol
	if data, ok, cacheErr := s.cache.Get(ctx, key); cacheErr == nil && ok {
		var item model.CapitalFlow
		if json.Unmarshal(data, &item) == nil && item.Symbol == symbol && time.Since(item.FetchedAt) >= 0 && time.Since(item.FetchedAt) < 5*time.Minute {
			return &item, true, nil
		}
	} else if cacheErr != nil {
		s.logger.Warn("capital-flow cache read failed", "error", cacheErr)
	}
	if len(s.supplementProviders) == 0 {
		return nil, false, errors.New("no enabled capital-flow provider")
	}
	var failures []string
	for _, p := range s.supplementProviders {
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
		for _, value := range []*float64{item.MainNetInflow, item.Inflow5D, item.Inflow10D} {
			if value != nil && !isFiniteSupplement(*value) {
				symbolErr = errors.New("non-finite capital-flow amount")
				break
			}
		}
		if symbolErr != nil {
			failures = append(failures, p.Name()+": capital-flow amount validation failed")
			continue
		}
		item.Symbol = symbol
		item.Source = p.Name()
		if strings.TrimSpace(item.SourceVersion) == "" {
			item.SourceVersion = "unknown"
		}
		if item.FetchedAt.IsZero() {
			item.FetchedAt = time.Now().UTC()
		}
		if item.QualityStatus == "" {
			item.QualityStatus = model.QualityDegraded
			item.QualityReasons = appendReason(item.QualityReasons, "provider_quality_status_missing")
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
			if symbolErr == nil && cachedSymbol == symbol && q.NormalizationVer == normalize.Version && normalize.ValidateQuote(&q) == nil {
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
		if q.FetchedAt.IsZero() {
			q.FetchedAt = time.Now().UTC()
		}
		if q.NormalizedAt.IsZero() {
			q.NormalizedAt = time.Now().UTC()
		}
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
		if _, err := time.Parse("2006-01-02", start); err != nil {
			return nil, false, nil, fmt.Errorf("invalid start date %q", start)
		}
	}
	if end != "" {
		if _, err := time.Parse("2006-01-02", end); err != nil {
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
			for _, bar := range v.Bars {
				cachedSymbol, symbolErr := normalize.NormalizeSymbol(bar.Symbol)
				if symbolErr != nil || cachedSymbol != symbol || bar.AdjustmentBasis != basis || bar.NormalizationVer != normalize.Version {
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
			if bars[j].FetchedAt.IsZero() {
				bars[j].FetchedAt = time.Now().UTC()
			}
			if bars[j].NormalizedAt.IsZero() {
				bars[j].NormalizedAt = time.Now().UTC()
			}
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
			if len(bars) > len(bestPartial) {
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

func quoteCacheEligible(q *model.Quote) bool {
	if q == nil || q.QualityStatus == model.QualityStale || q.QualityStatus == model.QualityInvalid || q.QualityStatus == model.QualityUnknown {
		return false
	}
	if q.QuoteTime == nil {
		return false
	}
	for _, reason := range q.QualityReasons {
		switch reason {
		case "provider_timestamp_missing", "quote_older_than_180_seconds_during_trading_clock", "quote_older_than_24_hours_outside_trading_clock", "provider_timestamp_more_than_60_seconds_in_future":
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
			strings.Contains(lower, "contract"):
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
