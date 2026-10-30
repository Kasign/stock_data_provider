package httpapi

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/service"
)

type Server struct {
	apiKey            string
	defaultAdjustment string
	maxBars           int
	requestTimeout    time.Duration
	svc               *service.DataService
	sources           []model.SourceStatus
	logger            *slog.Logger
}
type errorBody struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id,omitempty"`
	} `json:"error"`
}

func New(apiKey, defaultAdjustment string, svc *service.DataService, providers []provider.Provider, logger *slog.Logger) *Server {
	return NewWithConfig(apiKey, defaultAdjustment, svc, providers, providers, providers, 2000, 110*time.Second, logger)
}

// NewWithCapabilities is kept for aggregate Provider callers. New deployments
// should use NewWithCapabilityInterfaces to register independent capabilities.
func NewWithCapabilities(apiKey, defaultAdjustment string, svc *service.DataService, allProviders, quoteProviders, barProviders []provider.Provider, maxBars int, logger *slog.Logger) *Server {
	return NewWithConfig(apiKey, defaultAdjustment, svc, allProviders, quoteProviders, barProviders, maxBars, 110*time.Second, logger)
}

func NewWithConfig(apiKey, defaultAdjustment string, svc *service.DataService, allProviders, quoteProviders, barProviders []provider.Provider, maxBars int, requestTimeout time.Duration, logger *slog.Logger) *Server {
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
	return NewWithCapabilityInterfaces(apiKey, defaultAdjustment, svc, all, quotes, bars, maxBars, requestTimeout, logger)
}

func NewWithCapabilityInterfaces(apiKey, defaultAdjustment string, svc *service.DataService, allProviders []provider.NamedProvider, quoteProviders []provider.QuoteProvider, barProviders []provider.DailyBarProvider, maxBars int, requestTimeout time.Duration, logger *slog.Logger) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if maxBars <= 0 || maxBars > 2000 {
		maxBars = 2000
	}
	quoteEnabled, barsEnabled := map[string]bool{}, map[string]bool{}
	for _, p := range quoteProviders {
		quoteEnabled[p.Name()] = true
	}
	for _, p := range barProviders {
		barsEnabled[p.Name()] = true
	}
	known := map[string]bool{}
	for _, p := range allProviders {
		known[p.Name()] = true
	}
	names := make([]string, 0, len(known))
	for name := range known {
		names = append(names, name)
	}
	sort.Strings(names)
	sources := make([]model.SourceStatus, 0, len(names))
	stockBasicEnabled := map[string]bool{}
	stockSearchEnabled := map[string]bool{}
	sectorRankingEnabled := map[string]bool{}
	indexQuoteEnabled := map[string]bool{}
	marketBreadthEnabled := map[string]bool{}
	limitPoolEnabled := map[string]bool{}
	dragonTigerEnabled := map[string]bool{}
	announcementsEnabled := map[string]bool{}
	stockBoardsEnabled := map[string]bool{}
	tradingCalendarEnabled := map[string]bool{}
	securityTradingStatusEnabled := map[string]bool{}
	fundamentalsEnabled := map[string]bool{}
	valuationEnabled := map[string]bool{}
	indexCatalogEnabled := map[string]bool{}
	historicalIndexConstituentsEnabled := map[string]bool{}
	chipEnabled := map[string]bool{}
	capitalFlowEnabled := map[string]bool{}
	hotRankingEnabled := map[string]bool{}
	if svc != nil {
		var priceLimitEventProviders []provider.PriceLimitEventProvider
		for _, p := range allProviders {
			if eventProvider, ok := p.(provider.PriceLimitEventProvider); ok {
				priceLimitEventProviders = append(priceLimitEventProviders, eventProvider)
			}
		}
		if len(priceLimitEventProviders) > 0 {
			svc.SetPriceLimitEventProviders(priceLimitEventProviders)
		}
		for _, p := range svc.StockBasicProviders() {
			stockBasicEnabled[p.Name()] = true
		}
		for _, p := range svc.StockSearchProviders() {
			stockSearchEnabled[p.Name()] = true
		}
		for _, p := range svc.SectorRankingProviders() {
			sectorRankingEnabled[p.Name()] = true
		}
		for _, p := range svc.IndexQuoteProviders() {
			indexQuoteEnabled[p.Name()] = true
		}
		for _, p := range svc.MarketBreadthProviders() {
			marketBreadthEnabled[p.Name()] = true
		}
		for _, p := range svc.LimitPoolProviders() {
			limitPoolEnabled[p.Name()] = true
		}
		for _, p := range svc.DragonTigerProviders() {
			dragonTigerEnabled[p.Name()] = true
		}
		for _, p := range svc.AnnouncementProviders() {
			announcementsEnabled[p.Name()] = true
		}
		for _, p := range svc.StockBoardsProviders() {
			stockBoardsEnabled[p.Name()] = true
		}
		for _, p := range svc.TradingCalendarProviders() {
			tradingCalendarEnabled[p.Name()] = true
		}
		for _, p := range svc.SecurityTradingStatusProviders() {
			securityTradingStatusEnabled[p.Name()] = true
		}
		for _, p := range svc.FundamentalsProviders() {
			fundamentalsEnabled[p.Name()] = true
		}
		for _, p := range svc.ValuationProviders() {
			valuationEnabled[p.Name()] = true
		}
		for _, p := range svc.IndexCatalogProviders() {
			indexCatalogEnabled[p.Name()] = true
			if _, ok := p.(provider.HistoricalIndexConstituentProvider); ok {
				historicalIndexConstituentsEnabled[p.Name()] = true
			}
		}
		for _, p := range svc.ChipDistributionProviders() {
			chipEnabled[p.Name()] = true
		}
		for _, p := range svc.CapitalFlowProviders() {
			capitalFlowEnabled[p.Name()] = true
		}
		for _, p := range svc.HotRankingProviders() {
			hotRankingEnabled[p.Name()] = true
		}
	}
	for _, name := range names {
		sources = append(sources, model.SourceStatus{
			Name: name, Enabled: true,
			QuoteCapable: quoteEnabled[name], DailyBarCapable: barsEnabled[name],
			StockBasicCapable: stockBasicEnabled[name], StockSearchCapable: stockSearchEnabled[name], SectorRankingCapable: sectorRankingEnabled[name],
			IndexQuoteCapable: indexQuoteEnabled[name], MarketBreadthCapable: marketBreadthEnabled[name], LimitPoolCapable: limitPoolEnabled[name],
			StockBoardsCapable: stockBoardsEnabled[name], TradingCalendarCapable: tradingCalendarEnabled[name],
			SecurityTradingStatusCapable: securityTradingStatusEnabled[name], FundamentalsCapable: fundamentalsEnabled[name], ValuationCapable: valuationEnabled[name], IndexCatalogCapable: indexCatalogEnabled[name], HistoricalIndexConstituentsCapable: historicalIndexConstituentsEnabled[name], ChipCapable: chipEnabled[name], CapitalFlowCapable: capitalFlowEnabled[name], HotRankingCapable: hotRankingEnabled[name], DragonTigerCapable: dragonTigerEnabled[name], AnnouncementsCapable: announcementsEnabled[name],
		})
	}
	if requestTimeout <= 0 {
		requestTimeout = 110 * time.Second
	}
	return &Server{apiKey: apiKey, defaultAdjustment: defaultAdjustment, maxBars: maxBars, requestTimeout: requestTimeout, svc: svc, sources: sources, logger: logger}
}
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", s.live)
	mux.HandleFunc("GET /health/ready", s.ready)
	mux.Handle("GET /api/v1/quotes/{symbol}", s.auth(http.HandlerFunc(s.quote)))
	mux.Handle("GET /api/v1/market/quotes", s.auth(http.HandlerFunc(s.batchQuotes)))
	mux.Handle("GET /api/v1/bars/{symbol}/daily", s.auth(http.HandlerFunc(s.bars)))
	mux.Handle("GET /api/v1/bundles/{symbol}", s.auth(http.HandlerFunc(s.bundle)))
	mux.Handle("GET /api/v1/data-sources", s.auth(http.HandlerFunc(s.dataSources)))
	mux.Handle("GET /api/v1/data-sources/diagnostics", s.auth(http.HandlerFunc(s.dataSourceDiagnostics)))
	mux.Handle("GET /api/v1/market/sectors", s.auth(http.HandlerFunc(s.sectorRankings)))
	mux.Handle("GET /api/v1/market/indices", s.auth(http.HandlerFunc(s.indexQuotes)))
	mux.Handle("GET /api/v1/market/valuations", s.auth(http.HandlerFunc(s.valuations)))
	mux.Handle("GET /api/v1/market/index-catalog", s.auth(http.HandlerFunc(s.indexCatalog)))
	mux.Handle("GET /api/v1/market/indices/{symbol}/constituents", s.auth(http.HandlerFunc(s.indexConstituents)))
	mux.Handle("GET /api/v1/market/indices/{symbol}/daily", s.auth(http.HandlerFunc(s.indexDailyBars)))
	mux.Handle("GET /api/v1/market/breadth", s.auth(http.HandlerFunc(s.marketBreadth)))
	mux.Handle("GET /api/v1/market/hot-stocks", s.auth(http.HandlerFunc(s.hotStocks)))
	mux.Handle("GET /api/v1/market/limit-pools", s.auth(http.HandlerFunc(s.limitPool)))
	mux.Handle("GET /api/v1/market/dragon-tiger", s.auth(http.HandlerFunc(s.dragonTiger)))
	mux.Handle("GET /api/v1/market/trading-calendar", s.auth(http.HandlerFunc(s.tradingCalendar)))
	mux.Handle("GET /api/v1/market/session", s.auth(http.HandlerFunc(s.marketSession)))
	mux.Handle("GET /api/v1/stocks/search", s.auth(http.HandlerFunc(s.searchStocks)))
	mux.Handle("GET /api/v1/stocks/{symbol}/trading-status", s.auth(http.HandlerFunc(s.securityTradingStatus)))
	mux.Handle("GET /api/v1/stocks/{symbol}/price-limit-rule", s.auth(http.HandlerFunc(s.priceLimitRule)))
	mux.Handle("GET /api/v1/stocks/{symbol}/profile", s.auth(http.HandlerFunc(s.stockBasic)))
	mux.Handle("GET /api/v1/stocks/{symbol}/boards", s.auth(http.HandlerFunc(s.stockBoards)))
	mux.Handle("GET /api/v1/stocks/{symbol}/announcements", s.auth(http.HandlerFunc(s.stockAnnouncements)))
	mux.Handle("GET /api/v1/stocks/{symbol}/fundamentals", s.auth(http.HandlerFunc(s.fundamentals)))
	mux.Handle("GET /api/v1/stocks/{symbol}/chip-distribution", s.auth(http.HandlerFunc(s.chipDistribution)))
	mux.Handle("GET /api/v1/stocks/{symbol}/capital-flow", s.auth(http.HandlerFunc(s.capitalFlow)))
	return securityHeaders(s.requestID(s.accessLog(s.timeoutContext(s.requestTimeout, mux))))
}
func (s *Server) live(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, map[string]any{"status": "ok", "service": "market-data", "time": time.Now().UTC()})
}
func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.svc.Ready(ctx); err != nil {
		writeErr(w, 503, "DEPENDENCY_UNAVAILABLE", "market-data dependencies are not ready", requestID(r))
		return
	}
	writeJSON(w, 200, map[string]any{"status": "ready", "service": "market-data"})
}
func (s *Server) quote(w http.ResponseWriter, r *http.Request) {
	q, cacheHit, err := s.svc.Quote(r.Context(), r.PathValue("symbol"))
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if cacheHit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, 200, q)
}
func (s *Server) bars(w http.ResponseWriter, r *http.Request) {
	asOfValues, hasAsOf := r.URL.Query()["as_of"]
	var asOf time.Time
	if hasAsOf {
		if len(asOfValues) != 1 || strings.TrimSpace(asOfValues[0]) == "" {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "as_of must be a single RFC3339 timestamp", requestID(r))
			return
		}
		parsed, parseErr := time.Parse(time.RFC3339Nano, strings.TrimSpace(asOfValues[0]))
		if parseErr != nil || parsed.After(time.Now().UTC()) {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "as_of must be a valid non-future RFC3339 timestamp", requestID(r))
			return
		}
		asOf = parsed
	}
	rawSymbol := r.PathValue("symbol")
	symbol, symbolErr := normalize.NormalizeSymbol(rawSymbol)
	if symbolErr != nil {
		writeErr(w, 400, "VALIDATION_ERROR", symbolErr.Error(), requestID(r))
		return
	}
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		var n int
		if _, err := fmtSscan(v, &n); err != nil || n < 1 || n > s.maxBars {
			writeErr(w, 400, "VALIDATION_ERROR", "limit must be an integer between 1 and configured maxBars", requestID(r))
			return
		}
		limit = n
	}
	adjustment := r.URL.Query().Get("adjustment")
	if adjustment == "" {
		adjustment = s.defaultAdjustment
	}
	basis, err := normalize.Adjustment(adjustment)
	if err != nil {
		writeErr(w, 400, "VALIDATION_ERROR", err.Error(), requestID(r))
		return
	}
	if hasAsOf {
		bars, warnings, err := s.svc.DailyBarsAsOf(r.Context(), symbol, start, end, limit, basis, asOf)
		switch {
		case errors.Is(err, service.ErrDailyBarsAsOfUnavailable):
			writeErr(w, http.StatusServiceUnavailable, "PIT_STORAGE_UNAVAILABLE", "daily-bar point-in-time storage is not enabled", requestID(r))
			return
		case errors.Is(err, service.ErrDailyBarsAsOfNotFound):
			writeErr(w, http.StatusNotFound, "PIT_REVISION_NOT_FOUND", "no coherent daily-bar revisions were recorded by the requested as_of time", requestID(r))
			return
		case err != nil:
			writeDomainErr(w, err, requestID(r))
			return
		}
		bars = service.CalculateMovingAverages(bars)
		quality := model.QualityValid
		if normalize.HasQualityDegradingBarWarnings(warnings) {
			quality = model.QualityDegraded
		}
		for _, bar := range bars {
			if bar.QualityStatus != model.QualityValid {
				quality = model.QualityDegraded
				break
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"symbol": symbol, "period": "1d", "adjustment_basis": basis,
			"volume_unit": "shares", "amount_unit": "CNY", "quality_status": quality,
			"warnings": warnings, "point_in_time": true, "as_of": asOf.UTC(), "bars": bars,
		})
		return
	}
	bars, hit, warnings, err := s.svc.DailyBars(r.Context(), symbol, start, end, limit, basis)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if hit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	bars = service.CalculateMovingAverages(bars)
	quality := model.QualityValid
	if normalize.HasQualityDegradingBarWarnings(warnings) {
		quality = model.QualityDegraded
	}
	for _, b := range bars {
		if b.QualityStatus != model.QualityValid {
			quality = model.QualityDegraded
			break
		}
	}
	writeJSON(w, 200, map[string]any{"symbol": symbol, "period": "1d", "adjustment_basis": basis, "volume_unit": "shares", "amount_unit": "CNY", "quality_status": quality, "warnings": warnings, "bars": bars})
}
func (s *Server) bundle(w http.ResponseWriter, r *http.Request) {
	lookback := 0
	if v := r.URL.Query().Get("lookback_days"); v != "" {
		if _, err := fmtSscan(v, &lookback); err != nil || lookback < 20 || lookback > s.maxBars {
			writeErr(w, 400, "VALIDATION_ERROR", "lookback_days must be between 20 and configured maxBars", requestID(r))
			return
		}
	}
	asOfRaw := strings.TrimSpace(r.URL.Query().Get("as_of"))
	var asOf time.Time
	provided := asOfRaw != ""
	var err error
	if provided {
		asOf, err = time.Parse(time.RFC3339, asOfRaw)
		if err != nil {
			writeErr(w, 400, "VALIDATION_ERROR", "as_of must be RFC3339 with timezone", requestID(r))
			return
		}
	}
	adjustment := r.URL.Query().Get("adjustment")
	if adjustment == "" {
		adjustment = s.defaultAdjustment
	}
	basis, err := normalize.Adjustment(adjustment)
	if err != nil {
		writeErr(w, 400, "VALIDATION_ERROR", err.Error(), requestID(r))
		return
	}
	b, err := s.svc.Bundle(r.Context(), r.PathValue("symbol"), lookback, asOf, provided, basis)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	writeJSON(w, 200, b)
}
func (s *Server) dataSources(w http.ResponseWriter, r *http.Request) {
	sources := append([]model.SourceStatus(nil), s.sources...)
	if s.svc != nil {
		for _, p := range s.svc.SupplementProviders() {
			sources = append(sources, model.SourceStatus{Name: p.Name(), Enabled: true, ChipCapable: true, CapitalFlowCapable: true})
		}
	}
	sort.Slice(sources, func(i, j int) bool { return sources[i].Name < sources[j].Name })
	writeJSON(w, 200, map[string]any{"sources": sources, "normalization_version": normalize.Version})
}

// dataSourceDiagnostics 返回真实业务请求形成的运行诊断，不执行额外探测请求。
func (s *Server) dataSourceDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.svc == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}, "scope": []string{"quote", "daily_bars"}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"items":         s.svc.ProviderDiagnostics(),
		"scope":         []string{"quote", "daily_bars"},
		"observed_from": "in_process_circuit_breaker",
	})
}

func (s *Server) indexQuotes(w http.ResponseWriter, r *http.Request) {
	snapshot, cacheHit, err := s.svc.IndexQuotes(r.Context())
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if cacheHit {
		w.Header().Set("X-Market-Data-Cache", "HIT")
	} else {
		w.Header().Set("X-Market-Data-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, snapshot)
}
func (s *Server) sectorRankings(w http.ResponseWriter, r *http.Request) {
	category := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("type")))
	if category == "" {
		category = "industry"
	}
	if category != "industry" && category != "concept" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "type must be industry or concept", requestID(r))
		return
	}
	direction := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("direction")))
	if direction == "" {
		direction = "up"
	}
	if direction != "up" && direction != "down" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "direction must be up or down", requestID(r))
		return
	}
	limit := 10
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 100 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be an integer between 1 and 100", requestID(r))
			return
		}
		limit = parsed
	}
	snapshot, cacheHit, err := s.svc.SectorRankingsByDirection(r.Context(), category, direction, limit)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if cacheHit {
		w.Header().Set("X-Market-Data-Cache", "HIT")
	} else {
		w.Header().Set("X-Market-Data-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) marketBreadth(w http.ResponseWriter, r *http.Request) {
	snapshot, cacheHit, err := s.svc.MarketBreadth(r.Context())
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if cacheHit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) limitPool(w http.ResponseWriter, r *http.Request) {
	poolType := strings.TrimSpace(r.URL.Query().Get("type"))
	if poolType == "" {
		poolType = "up"
	}
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date != "" {
		if _, err := time.Parse("2006-01-02", date); err != nil {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must use YYYY-MM-DD", requestID(r))
			return
		}
	}
	limit := 100
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be between 1 and 100", requestID(r))
			return
		}
		limit = n
	}
	if poolType != "up" && poolType != "down" && poolType != "broken" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "type must be up, down or broken", requestID(r))
		return
	}
	snapshot, cacheHit, err := s.svc.LimitPool(r.Context(), poolType, date, limit)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if cacheHit {
		w.Header().Set("X-Cache", "HIT")
	} else {
		w.Header().Set("X-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) dragonTiger(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date != "" {
		if parsed, err := time.Parse("2006-01-02", date); err != nil || parsed.Format("2006-01-02") != date {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "date must use YYYY-MM-DD", requestID(r))
			return
		}
	}
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "page must be between 1 and 1000", requestID(r))
			return
		}
		page = n
	}
	pageSize := 100
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "page_size must be between 1 and 500", requestID(r))
			return
		}
		pageSize = n
	}
	item, hit, err := s.svc.DragonTiger(r.Context(), date, page, pageSize)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, hit)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) searchStocks(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "q is required", requestID(r))
		return
	}
	if len([]rune(query)) > 64 {
		writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "q must not exceed 64 characters", requestID(r))
		return
	}
	limit := 10
	if rawLimit := strings.TrimSpace(r.URL.Query().Get("limit")); rawLimit != "" {
		parsed, err := strconv.Atoi(rawLimit)
		if err != nil || parsed < 1 || parsed > 50 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "limit must be between 1 and 50", requestID(r))
			return
		}
		limit = parsed
	}
	snapshot, err := s.svc.SearchStocks(r.Context(), query, limit)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if snapshot.QualityStatus == model.QualityStale {
		// 只有服务明确返回 stale 时才暴露缓存状态；正常响应不代表命中了服务层缓存。
		w.Header().Set("X-Market-Data-Cache", "STALE")
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) stockBasic(w http.ResponseWriter, r *http.Request) {
	item, hit, err := s.svc.StockBasic(r.Context(), r.PathValue("symbol"))
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	if hit {
		w.Header().Set("X-Market-Data-Cache", "HIT")
	} else {
		w.Header().Set("X-Market-Data-Cache", "MISS")
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) stockAnnouncements(w http.ResponseWriter, r *http.Request) {
	startDate := strings.TrimSpace(r.URL.Query().Get("start_date"))
	endDate := strings.TrimSpace(r.URL.Query().Get("end_date"))
	page := 1
	if raw := r.URL.Query().Get("page"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 1000 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "page must be between 1 and 1000", requestID(r))
			return
		}
		page = n
	}
	pageSize := 20
	if raw := r.URL.Query().Get("page_size"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 100 {
			writeErr(w, http.StatusBadRequest, "VALIDATION_ERROR", "page_size must be between 1 and 100", requestID(r))
			return
		}
		pageSize = n
	}
	snapshot, cacheHit, err := s.svc.StockAnnouncements(r.Context(), r.PathValue("symbol"), startDate, endDate, page, pageSize)
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, cacheHit)
	writeJSON(w, http.StatusOK, snapshot)
}

func (s *Server) stockBoards(w http.ResponseWriter, r *http.Request) {
	item, hit, err := s.svc.StockBoards(r.Context(), r.PathValue("symbol"))
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, hit)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) chipDistribution(w http.ResponseWriter, r *http.Request) {
	item, hit, err := s.svc.ChipDistribution(r.Context(), r.PathValue("symbol"))
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, hit)
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) capitalFlow(w http.ResponseWriter, r *http.Request) {
	item, hit, err := s.svc.CapitalFlow(r.Context(), r.PathValue("symbol"))
	if err != nil {
		writeDomainErr(w, err, requestID(r))
		return
	}
	setCacheHeader(w, hit)
	writeJSON(w, http.StatusOK, item)
}

func setCacheHeader(w http.ResponseWriter, hit bool) {
	if hit {
		w.Header().Set("X-Cache", "HIT")
		return
	}
	w.Header().Set("X-Cache", "MISS")
}

// accessLog 只记录请求元数据和响应体字节数，不记录正文、查询参数或密钥。
func (s *Server) accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(recorder, r)
		s.logger.Info("http request completed", "service", "market-data", "request_id", requestID(r),
			"method", r.Method, "path", boundedLogPath(r.URL.Path), "status_code", recorder.status,
			"duration_ms", time.Since(started).Milliseconds(), "response_bytes", recorder.bytes)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusRecorder) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusRecorder) Write(data []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(data)
	w.bytes += n
	return n, err
}
func boundedLogPath(path string) string {
	if len(path) > 160 {
		return path[:160]
	}
	return path
}
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.Header.Get("X-API-Key"))
		if token == "" {
			a := strings.TrimSpace(r.Header.Get("Authorization"))
			if strings.HasPrefix(a, "Bearer ") {
				token = strings.TrimSpace(strings.TrimPrefix(a, "Bearer "))
			}
		}
		if token == "" || subtle.ConstantTimeCompare([]byte(token), []byte(s.apiKey)) != 1 {
			writeErr(w, 401, "AUTH_REQUIRED", "valid API key required", requestID(r))
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) timeoutContext(timeout time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (s *Server) requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if !validRequestID(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id)))
	})
}

type requestIDKey struct{}

func requestID(r *http.Request) string { v, _ := r.Context().Value(requestIDKey{}).(string); return v }
func newRequestID() string             { return time.Now().UTC().Format("20060102T150405.000000000Z") }
func validRequestID(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == ':') {
			return false
		}
	}
	return true
}
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	body, err := json.Marshal(v)
	if err != nil {
		status = http.StatusInternalServerError
		body = []byte(`{"error":{"code":"INTERNAL_ERROR","message":"response serialization failed"}}`)
	} else {
		body = append(body, '\n')
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}
func writeErr(w http.ResponseWriter, status int, code, message, id string) {
	var b errorBody
	b.Error.Code = code
	b.Error.Message = message
	b.Error.RequestID = id
	writeJSON(w, status, b)
}
func writeDomainErr(w http.ResponseWriter, err error, id string) {
	msg := err.Error()
	status := 502
	code := "PROVIDER_BAD_RESPONSE"
	if strings.Contains(msg, "invalid A-share symbol") || strings.Contains(msg, "B-share code") || strings.Contains(msg, "exchange suffix") || strings.Contains(msg, "invalid start date") || strings.Contains(msg, "invalid end date") || strings.Contains(msg, "must not be after") || strings.Contains(msg, "must not be in the future") || strings.Contains(msg, "unsupported adjustment") || strings.Contains(msg, "cannot infer exchange") {
		status = 400
		code = "VALIDATION_ERROR"
	} else if strings.Contains(msg, "no enabled") && strings.Contains(msg, "provider") {
		status = 503
		code = "PROVIDER_UNAVAILABLE"
	} else if errors.Is(err, context.DeadlineExceeded) {
		status = 504
		code = "PROVIDER_TIMEOUT"
	} else if errors.Is(err, context.Canceled) {
		status = 408
		code = "REQUEST_CANCELED"
	}
	writeErr(w, status, code, msg, id)
}

// fmtSscan is separated to keep input parsing constrained to a single integer.
func fmtSscan(value string, out *int) (int, error) {
	n, err := strconv.Atoi(value)
	if err == nil {
		*out = n
	}
	return n, err
}
