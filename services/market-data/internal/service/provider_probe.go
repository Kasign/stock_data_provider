package service

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/provider"
)

var (
	probeHTTPStatusPattern = regexp.MustCompile(`(?i)\bHTTP(?:\s+status)?\s*[:=]?\s*(\d{3})\b`)
	probeURLQueryPattern   = regexp.MustCompile(`(?i)(https?://[^\s"'?]+)\?[^\s"' ]*`)
	probeSecretPattern     = regexp.MustCompile(`(?i)(token|api[_-]?key|apikey|cookie|authorization)(["'=:\s]+)([^&\s,;"']+)`)
)

type providerProbeFunc func(context.Context) (model.QualityStatus, string, []string, error)
type providerProbeJob struct {
	provider   string
	capability string
	run        providerProbeFunc
}

type providerProbeEvidenceContextKey struct{}

type providerProbeEvidence struct {
	sampleSymbol  string
	sampleMarket  string
	sampleDate    string
	rowsReturned  int
	rowsValidated int
	coverageRatio *float64
	providerTime  *time.Time
}

func recordProviderProbeCoverage(ctx context.Context, coverage float64) {
	evidence, ok := ctx.Value(providerProbeEvidenceContextKey{}).(*providerProbeEvidence)
	if !ok || evidence == nil || math.IsNaN(coverage) || math.IsInf(coverage, 0) || coverage < 0 || coverage > 1 {
		return
	}
	evidence.coverageRatio = &coverage
}

func recordProviderProbeEvidence(ctx context.Context, symbol, sampleDate string, rowsReturned, rowsValidated int, providerTime *time.Time) {
	evidence, ok := ctx.Value(providerProbeEvidenceContextKey{}).(*providerProbeEvidence)
	if !ok || evidence == nil {
		return
	}
	evidence.sampleSymbol = symbol
	if separator := strings.LastIndex(symbol, "."); separator >= 0 && separator+1 < len(symbol) {
		market := strings.ToUpper(symbol[separator+1:])
		if market == "SH" || market == "SZ" || market == "BJ" {
			evidence.sampleMarket = market
		}
	}
	if sampleDate == "" && providerTime != nil && !providerTime.IsZero() {
		sampleDate = providerTime.In(marketLocation()).Format("2006-01-02")
	}
	evidence.sampleDate = sampleDate
	evidence.rowsReturned = rowsReturned
	evidence.rowsValidated = rowsValidated
	evidence.providerTime = providerTime
}

// ProviderProbeOptions 控制主动诊断的执行范围和单项超时。
type ProviderProbeOptions struct {
	Scope          string
	Capabilities   []string
	Markets        []string
	Symbols        []string
	TimeoutSeconds int
}

// ProbeProviderCapabilities 保留旧调用方式，默认执行全部诊断能力。
func (s *DataService) ProbeProviderCapabilities(ctx context.Context) (*model.ProviderProbeRun, error) {
	return s.ProbeProviderCapabilitiesWithOptions(ctx, ProviderProbeOptions{Scope: "all", TimeoutSeconds: 12})
}

// ProbeProviderCapabilitiesWithOptions 主动探测已启用 Provider 的能力。
// 每个探测直接调用对应 Provider，不经过业务请求的熔断统计；单项超时和并发均受限。
func (s *DataService) ProbeProviderCapabilitiesWithOptions(ctx context.Context, options ProviderProbeOptions) (*model.ProviderProbeRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if options.Scope == "" {
		options.Scope = "all"
	}
	if options.Scope != "all" && options.Scope != "core" {
		return nil, errors.New("provider-probe scope must be all or core")
	}
	if options.TimeoutSeconds == 0 {
		options.TimeoutSeconds = 12
	}
	if options.TimeoutSeconds < 1 || options.TimeoutSeconds > 30 {
		return nil, errors.New("provider-probe timeout_seconds must be between 1 and 30")
	}
	allowed := make(map[string]bool, len(model.ProviderProbeCapabilities()))
	for _, capability := range model.ProviderProbeCapabilities() {
		allowed[capability] = true
	}
	selected := make(map[string]bool)
	if len(options.Capabilities) > 0 {
		for _, capability := range options.Capabilities {
			if !allowed[capability] {
				return nil, fmt.Errorf("provider-probe capability %q is not supported", capability)
			}
			if selected[capability] {
				return nil, fmt.Errorf("provider-probe capability %q is duplicated", capability)
			}
			if options.Scope == "core" && !isCoreProbeCapability(capability) {
				return nil, fmt.Errorf("provider-probe capability %q is outside core scope", capability)
			}
			selected[capability] = true
		}
	} else if options.Scope == "core" {
		for _, capability := range model.ProviderProbeCoreCapabilities() {
			selected[capability] = true
		}
	} else {
		for capability := range allowed {
			selected[capability] = true
		}
	}
	if len(selected) == 0 {
		return nil, errors.New("provider-probe capabilities must not be empty")
	}
	if len(options.Markets) > 1 || len(options.Symbols) > 1 {
		return nil, errors.New("provider-probe currently accepts at most one market and one symbol per request")
	}
	marketSamples := map[string]string{
		"SH_MAIN": "600519.SH",
		"SZ_MAIN": "000001.SZ",
		"CHINEXT": "300750.SZ",
		"STAR":    "688981.SH",
		"BSE":     "920001.BJ",
	}
	selectedMarket := ""
	if len(options.Markets) == 1 {
		selectedMarket = strings.ToUpper(strings.TrimSpace(options.Markets[0]))
		if _, ok := marketSamples[selectedMarket]; !ok {
			return nil, fmt.Errorf("provider-probe market %q is not supported", options.Markets[0])
		}
	}
	probeSymbol := "600036.SH"
	if selectedMarket != "" {
		probeSymbol = marketSamples[selectedMarket]
	}
	if len(options.Symbols) == 1 {
		probeSymbol = strings.ToUpper(strings.TrimSpace(options.Symbols[0]))
		if !regexp.MustCompile("^\\d{6}\\.(SH|SZ|BJ)$").MatchString(probeSymbol) {
			return nil, fmt.Errorf("provider-probe symbol %q must use six digits and SH/SZ/BJ suffix", options.Symbols[0])
		}
	}
	if selectedMarket != "" && !probeSymbolMatchesBoard(probeSymbol, selectedMarket) {
		return nil, fmt.Errorf("provider-probe symbol %q does not belong to market %q", probeSymbol, selectedMarket)
	}
	if selectedMarket != "" {
		options.Markets = []string{selectedMarket}
	}
	if len(options.Symbols) == 1 {
		options.Symbols = []string{probeSymbol}
	}
	if len(options.Markets) > 0 || len(options.Symbols) > 0 {
		if len(options.Capabilities) == 0 {
			return nil, errors.New("provider-probe markets or symbols require an explicit stock-scoped capabilities list")
		}
		for capability := range selected {
			if !isStockScopedProbeCapability(capability) {
				return nil, fmt.Errorf("provider-probe capability %q cannot be filtered by market or symbol", capability)
			}
		}
	}
	startedAt := time.Now().UTC()
	requested := make([]string, 0, len(selected))
	for capability := range selected {
		requested = append(requested, capability)
	}
	sort.Strings(requested)
	run := &model.ProviderProbeRun{RunID: newFetchID(), Scope: options.Scope, RequestedCapabilities: requested, RequestedMarkets: append([]string(nil), options.Markets...), RequestedSymbols: append([]string(nil), options.Symbols...), TimeoutSeconds: options.TimeoutSeconds, StartedAt: startedAt, Items: make([]model.ProviderProbeItem, 0)}
	jobs := make([]providerProbeJob, 0)
	add := func(name, capability string, fn providerProbeFunc) {
		jobs = append(jobs, providerProbeJob{provider: name, capability: capability, run: fn})
	}
	quoteSymbol := "600519.SH"
	barsSymbol := "600036.SH"
	searchSymbol := "600519"
	if len(options.Markets) > 0 || len(options.Symbols) > 0 {
		quoteSymbol = probeSymbol
		barsSymbol = probeSymbol
		searchSymbol = probeSymbol[:6]
	}
	for _, p := range s.quoteProviders {
		p := p
		add(p.Name(), "quote", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			q, err := p.FetchQuote(probeCtx, quoteSymbol)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if q == nil {
				return model.QualityUnknown, "", nil, errors.New("empty quote response")
			}
			recordProviderProbeEvidence(probeCtx, quoteSymbol, "", 1, 0, q.QuoteTime)
			if q.Symbol != quoteSymbol {
				return model.QualityInvalid, q.SourceVersion, nil, fmt.Errorf("quote contract invalid: symbol mismatch, got %q, want %q", q.Symbol, quoteSymbol)
			}
			if q.Source == "" {
				return model.QualityInvalid, q.SourceVersion, nil, errors.New("quote source missing")
			}
			if err := normalize.ValidateQuote(q); err != nil {
				return model.QualityInvalid, q.SourceVersion, nil, err
			}
			if q.SourceVersion == "" {
				return model.QualityInvalid, "", nil, errors.New("quote source version missing")
			}
			recordProviderProbeEvidence(probeCtx, quoteSymbol, "", 1, 1, q.QuoteTime)
			return q.QualityStatus, q.SourceVersion, append([]string(nil), q.QualityReasons...), nil
		})
	}
	for _, p := range s.barProviders {
		p := p
		add(p.Name(), "daily_bars", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			bars, err := p.FetchDailyBars(probeCtx, barsSymbol, "", "", 5, model.AdjustmentNone)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			validated, warnings := normalize.ValidateAndSortBars(barsSymbol, bars, "", "", 5)
			latestDate := ""
			var providerTime *time.Time
			if len(validated) > 0 {
				latest := validated[len(validated)-1]
				latestDate = latest.TradeDate
				providerTime = latest.ProviderTime
			}
			recordProviderProbeEvidence(probeCtx, barsSymbol, latestDate, len(bars), len(validated), providerTime)
			if len(validated) == 0 {
				return model.QualityInvalid, "", warnings, errors.New("daily-bars probe returned no valid bars")
			}
			quality := model.QualityValid
			version := ""
			reasons := append([]string(nil), warnings...)
			for _, bar := range validated {
				if version == "" {
					version = bar.SourceVersion
				}
				if bar.QualityStatus != model.QualityValid {
					quality = model.QualityDegraded
				}
				for _, reason := range bar.QualityReasons {
					reasons = appendReason(reasons, reason)
				}
			}
			if len(warnings) > 0 {
				quality = model.QualityDegraded
			}
			if version == "" {
				return model.QualityInvalid, "", reasons, errors.New("daily-bars source version missing")
			}
			return quality, version, reasons, nil
		})
	}
	for _, p := range s.stockBasicProviders {
		p := p
		add(p.Name(), "stock_basic", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			item, err := p.FetchStockBasic(probeCtx, barsSymbol)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateStockBasic(barsSymbol, item); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return item.QualityStatus, item.SourceVersion, append([]string(nil), item.QualityReasons...), nil
		})
	}
	for _, p := range s.stockSearchProviders {
		p := p
		add(p.Name(), "stock_search", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.SearchStocks(probeCtx, searchSymbol, 1)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateStockSearchSnapshot(searchSymbol, 1, snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.indexQuoteProviders {
		p := p
		add(p.Name(), "index_quotes", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.FetchIndexQuotes(probeCtx)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateIndexQuoteSnapshot(snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			if snapshot != nil {
				symbol := ""
				if len(snapshot.Items) > 0 {
					symbol = snapshot.Items[0].Symbol
				}
				recordProviderProbeEvidence(probeCtx, symbol, "", len(snapshot.Items), len(snapshot.Items), nil)
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.marketBreadthProviders {
		p := p
		add(p.Name(), "market_breadth", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.FetchMarketBreadth(probeCtx)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateMarketBreadth(snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			if snapshot != nil {
				recordProviderProbeEvidence(probeCtx, "", snapshot.ProviderReportDate, snapshot.CoveredSecurities, snapshot.CoveredSecurities, snapshot.NewestQuoteTime)
				if snapshot.TotalSecurities > 0 {
					recordProviderProbeCoverage(probeCtx, float64(snapshot.CoveredSecurities)/float64(snapshot.TotalSecurities))
				}
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.sectorRankingProviders {
		p := p
		add(p.Name(), "sector_rankings", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			snapshot, err := p.FetchSectorRankings(probeCtx, "industry", 5)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateSectorRankingSnapshot("industry", 5, snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			if snapshot != nil {
				recordProviderProbeEvidence(probeCtx, "", snapshot.ProviderReportDate, len(snapshot.Items), len(snapshot.Items), nil)
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, append([]string(nil), snapshot.QualityReasons...), nil
		})
	}
	for _, p := range s.marketCapitalFlowProviders {
		p := p
		add(p.Name(), "market_capital_flow", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			loc := marketLocation()
			end := time.Now().In(loc)
			start := end.AddDate(0, 0, -14)
			snapshot, err := p.FetchMarketCapitalFlow(probeCtx, start.Format("2006-01-02"), end.Format("2006-01-02"))
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateMarketCapitalFlowSnapshot(start.Format("2006-01-02"), end.Format("2006-01-02"), snapshot); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			return snapshot.QualityStatus, snapshot.SourceVersion, []string{"market_capital_flow_history_probe"}, nil
		})
	}

	// 以下检查覆盖非行情类的重要业务能力。探测只校验返回契约，不把空数据自动视为成功。
	loc := marketLocation()
	nowLocal := time.Now().In(loc)
	probeDate := nowLocal.Format("2006-01-02")
	calendarStart := nowLocal.AddDate(0, 0, -14).Format("2006-01-02")
	calendarEnd := nowLocal.AddDate(0, 0, 14).Format("2006-01-02")
	for _, p := range s.indexCatalogProviders {
		p := p
		add(p.Name(), "index_catalog", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchIndexCatalog(probeCtx, "industry")
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.Tag != "industry" || v.Source == "" || v.FetchedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("index-catalog response missing requested tag/source/fetched_at")
			}
			if v.Total < len(v.Items) || v.ProviderReturned < len(v.Items) {
				return model.QualityInvalid, v.SourceVersion, nil, errors.New("index-catalog response has inconsistent counts")
			}
			sample := ""
			if len(v.Items) > 0 {
				sample = v.Items[0].Symbol
			}
			recordProviderProbeEvidence(probeCtx, sample, "", len(v.Items), len(v.Items), v.ProviderTime)
			seen := make(map[string]bool, len(v.Items))
			for _, item := range v.Items {
				if item.Symbol == "" || item.Name == "" || seen[item.Symbol] {
					return model.QualityInvalid, v.SourceVersion, nil, errors.New("index-catalog row has missing or duplicate identity")
				}
				seen[item.Symbol] = true
			}
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.securityTradingStatusProviders {
		p := p
		add(p.Name(), "security_trading_status", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchSecurityTradingStatus(probeCtx, barsSymbol, probeDate)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.Symbol != barsSymbol || v.TradeDate != probeDate || v.Source == "" || v.FetchedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("security-trading-status response missing requested symbol/date/source/fetched_at")
			}
			if !v.TradingDayKnown || !v.ListingStatusKnown || v.SuspensionStatus == model.SuspensionStatusUnknown || v.RiskWarningStatus == model.RiskWarningUnknown || v.DelistingStatus == model.DelistingStatusUnknown {
				return model.QualityDegraded, v.SourceVersion, appendReason(v.QualityReasons, "security_status_contains_unknown_fields"), nil
			}
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.priceLimitEventProviders {
		p := p
		add(p.Name(), "price_limit_event", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchPriceLimitEventEvidence(probeCtx, barsSymbol, probeDate)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.Symbol != barsSymbol || v.Source == "" || v.SourceVersion == "" || v.ObservedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("price-limit-event evidence missing symbol/source/version/observed_at")
			}
			if !v.CoverageConfirmed || v.Status == model.PriceLimitExceptionUnknown {
				return model.QualityDegraded, v.SourceVersion, appendReason(v.QualityReasons, "special_event_absence_not_confirmed"), nil
			}
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.stockBoardsProviders {
		p := p
		add(p.Name(), "stock_boards", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchStockBoards(probeCtx, barsSymbol)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.Symbol != barsSymbol || v.Source == "" || v.FetchedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("stock-boards response missing symbol/source/fetched_at or has wrong symbol")
			}
			if v.Total < 0 || v.ProviderReturned < len(v.Items) || (v.HasData && len(v.Items) == 0) {
				return model.QualityInvalid, v.SourceVersion, nil, errors.New("stock-boards response has inconsistent counts")
			}
			for _, item := range v.Items {
				if strings.TrimSpace(item.Code) == "" || strings.TrimSpace(item.Name) == "" || item.Source != v.Source {
					return model.QualityInvalid, v.SourceVersion, nil, errors.New("stock-boards row missing identity or source")
				}
			}
			recordProviderProbeEvidence(probeCtx, barsSymbol, probeDate, len(v.Items), len(v.Items), nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.limitPoolProviders {
		p := p
		add(p.Name(), "limit_pool", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchLimitPool(probeCtx, "up", probeDate, 20)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.TradeDate != probeDate || v.Source == "" || v.FetchedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("limit-pool response missing requested trade date/source/fetched_at")
			}
			if v.Total < 0 || v.ProviderReturned < len(v.Items) || (v.HasData && len(v.Items) == 0) {
				return model.QualityInvalid, v.SourceVersion, nil, errors.New("limit-pool response has inconsistent counts")
			}
			for _, item := range v.Items {
				if item.Symbol == "" || item.Code == "" || item.Name == "" {
					return model.QualityInvalid, v.SourceVersion, nil, errors.New("limit-pool row missing security identity")
				}
			}
			sample := ""
			if len(v.Items) > 0 {
				sample = v.Items[0].Symbol
			}
			recordProviderProbeEvidence(probeCtx, sample, v.TradeDate, len(v.Items), len(v.Items), nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.dragonTigerProviders {
		p := p
		add(p.Name(), "dragon_tiger", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchDragonTiger(probeCtx, probeDate, 1, 20)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.TradeDate != probeDate || v.Source == "" || v.FetchedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("dragon-tiger response missing requested trade date/source/fetched_at")
			}
			if v.Total < 0 || v.Returned != len(v.Items) || v.SourceReturned < v.Returned {
				return model.QualityInvalid, v.SourceVersion, nil, errors.New("dragon-tiger response has inconsistent pagination counts")
			}
			for _, item := range v.Items {
				if item.Symbol == "" || item.Code == "" || item.TradeDate != probeDate {
					return model.QualityInvalid, v.SourceVersion, nil, errors.New("dragon-tiger row has invalid security identity or trade date")
				}
			}
			sample := ""
			if len(v.Items) > 0 {
				sample = v.Items[0].Symbol
			}
			recordProviderProbeEvidence(probeCtx, sample, v.TradeDate, len(v.Items), len(v.Items), nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.announcementProviders {
		p := p
		add(p.Name(), "announcements", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchStockAnnouncements(probeCtx, barsSymbol, calendarStart, probeDate, 1, 20)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateStockAnnouncementSnapshot(barsSymbol, calendarStart, probeDate, 1, 20, v); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			recordProviderProbeEvidence(probeCtx, barsSymbol, probeDate, len(v.Items), len(v.Items), nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.tradingCalendarProviders {
		p := p
		add(p.Name(), "trading_calendar", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchTradingCalendar(probeCtx, calendarStart, calendarEnd)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateTradingCalendarSnapshot(v, calendarStart, calendarEnd); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			sampleDate := calendarEnd
			if len(v.Items) > 0 && v.Items[0].Date != "" {
				sampleDate = v.Items[0].Date
			}
			recordProviderProbeEvidence(probeCtx, "", sampleDate, len(v.Items), len(v.Items), nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.fundamentalsProviders {
		p := p
		add(p.Name(), "fundamentals", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchFundamentals(probeCtx, barsSymbol, nowLocal.Year(), 2)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateFundamentalsSnapshot(barsSymbol, nowLocal.Year(), 2, v); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			recordProviderProbeEvidence(probeCtx, barsSymbol, "", len(v.Sections), len(v.Sections), nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.valuationProviders {
		p := p
		add(p.Name(), "valuations", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchValuations(probeCtx, []string{barsSymbol})
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateValuationSnapshot([]string{barsSymbol}, v); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			recordProviderProbeEvidence(probeCtx, barsSymbol, v.ProviderReportDate, len(v.Items), len(v.Items), v.ProviderTimestamp)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.hotRankingProviders {
		p := p
		add(p.Name(), "hot_ranking", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchHotStocks(probeCtx, "popular", 20)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if err := validateHotRankingSnapshot("popular", v); err != nil {
				return model.QualityInvalid, "", nil, err
			}
			sample := ""
			if len(v.Items) > 0 {
				sample = v.Items[0].Symbol
			}
			recordProviderProbeEvidence(probeCtx, sample, "", len(v.Items), len(v.Items), v.ProviderTime)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.capitalFlowProviders {
		p := p
		add(p.Name(), "capital_flow", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchCapitalFlow(probeCtx, barsSymbol)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.Symbol != barsSymbol || v.Source == "" || v.FetchedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("capital-flow response missing requested symbol/source/fetched_at")
			}
			for _, value := range []*float64{v.MainNetInflow, v.Inflow5D, v.Inflow10D} {
				if value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)) {
					return model.QualityInvalid, v.SourceVersion, nil, errors.New("capital-flow response contains non-finite amount")
				}
			}
			if v.Observations < 0 {
				return model.QualityInvalid, v.SourceVersion, nil, errors.New("capital-flow response has negative observation count")
			}
			recordProviderProbeEvidence(probeCtx, barsSymbol, v.ProviderDate, v.Observations, v.Observations, nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	for _, p := range s.chipDistributionProviders {
		p := p
		add(p.Name(), "chip_distribution", func(probeCtx context.Context) (model.QualityStatus, string, []string, error) {
			v, err := p.FetchChipDistribution(probeCtx, barsSymbol)
			if err != nil {
				return model.QualityUnknown, "", nil, err
			}
			if v == nil || v.Symbol != barsSymbol || v.Source == "" || v.FetchedAt.IsZero() {
				return model.QualityInvalid, "", nil, errors.New("chip-distribution response missing requested symbol/source/fetched_at")
			}
			if math.IsNaN(v.AverageCost) || math.IsInf(v.AverageCost, 0) || v.AverageCost < 0 {
				return model.QualityInvalid, v.SourceVersion, nil, errors.New("chip-distribution response has invalid average cost")
			}
			if v.Cost90Low != nil && v.Cost90High != nil && *v.Cost90Low > *v.Cost90High {
				return model.QualityInvalid, v.SourceVersion, nil, errors.New("chip-distribution 90-percent cost range is inverted")
			}
			recordProviderProbeEvidence(probeCtx, barsSymbol, v.ProviderDate, 1, 1, nil)
			return v.QualityStatus, v.SourceVersion, append([]string(nil), v.QualityReasons...), nil
		})
	}
	// 固定核心能力清单：未注册 Provider 的能力也必须显式出现在诊断报告中。
	requiredCapabilities := make([]string, 0, len(selected))
	for capability := range selected {
		requiredCapabilities = append(requiredCapabilities, capability)
	}
	sort.Strings(requiredCapabilities)
	filteredJobs := make([]providerProbeJob, 0, len(jobs))
	for _, job := range jobs {
		if selected[job.capability] {
			filteredJobs = append(filteredJobs, job)
		}
	}
	jobs = filteredJobs
	configured := make(map[string]bool, len(requiredCapabilities))
	for _, job := range jobs {
		configured[job.capability] = true
	}
	if selected["stock_universe"] {
		for _, p := range s.providers {
			if universeProvider, ok := p.(provider.StockUniverseProvider); ok {
				// 全市场证券目录可能返回数千条记录，默认轻量探测不下载完整目录；明确记录为 skipped。
				configured["stock_universe"] = true
				add(universeProvider.Name(), "stock_universe", func(context.Context) (model.QualityStatus, string, []string, error) {
					return model.QualityUnknown, "", []string{"full_market_universe_excluded_from_lightweight_probe"}, errors.New("probe skipped: full-market universe scan is intentionally excluded from lightweight diagnostics")
				})
			}
		}
	}
	for _, capability := range requiredCapabilities {
		if configured[capability] {
			continue
		}
		jobs = append(jobs, providerProbeJob{provider: "system", capability: capability, run: func(context.Context) (model.QualityStatus, string, []string, error) {
			return model.QualityUnknown, "", nil, errors.New("capability has no registered provider")
		}})
	}
	run.Total = len(jobs)
	results := make([]model.ProviderProbeItem, len(jobs))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, job := range jobs {
		wg.Add(1)
		go func(index int, job providerProbeJob) {
			defer wg.Done()
			item := model.ProviderProbeItem{Provider: job.provider, Capability: job.capability, Status: "failed", QualityStatus: model.QualityUnknown}
			checkedAt := time.Now().UTC()
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				setProbeFailure(&item, ctx.Err())
				item.ElapsedMS = time.Since(checkedAt).Milliseconds()
				item.CheckedAt = time.Now().UTC()
				results[index] = item
				return
			}
			// 等待并发槽位的时间不计入单项 Provider 超时，避免排队任务被误报为超时。
			probeCtx, cancel := context.WithTimeout(ctx, time.Duration(options.TimeoutSeconds)*time.Second)
			evidence := &providerProbeEvidence{}
			probeCtx = context.WithValue(probeCtx, providerProbeEvidenceContextKey{}, evidence)
			quality, version, reasons, err := job.run(probeCtx)
			cancel()
			<-sem
			item.SampleSymbol = evidence.sampleSymbol
			item.SampleMarket = evidence.sampleMarket
			item.SampleDate = evidence.sampleDate
			item.RowsReturned = evidence.rowsReturned
			item.RowsValidated = evidence.rowsValidated
			item.CoverageRatio = evidence.coverageRatio
			item.ProviderTime = evidence.providerTime
			item.ElapsedMS = time.Since(checkedAt).Milliseconds()
			item.CheckedAt = time.Now().UTC()
			item.SourceVersion = version
			item.QualityReasons = reasons
			if err != nil {
				setProbeFailure(&item, err)
				if strings.Contains(err.Error(), "probe skipped:") {
					item.Status = "skipped"
					item.ErrorCode = "probe_skipped_by_policy"
					item.ErrorStage = "configuration"
				} else if job.provider == "system" && strings.Contains(err.Error(), "no registered provider") {
					item.Status = "not_configured"
					item.ErrorCode = "provider_not_configured"
					item.ErrorStage = "configuration"
				}
			} else {
				item.QualityStatus = quality
				switch quality {
				case model.QualityValid:
					item.Status = "returned"
				case model.QualityUnknown:
					item.Status = "returned_unknown"
				default:
					item.Status = "returned_degraded"
				}
			}
			results[index] = item
		}(i, job)
	}
	wg.Wait()
	sort.Slice(results, func(i, j int) bool {
		if results[i].Provider != results[j].Provider {
			return results[i].Provider < results[j].Provider
		}
		return results[i].Capability < results[j].Capability
	})
	run.Items = results
	for _, item := range results {
		switch item.Status {
		case "returned":
			run.Returned++
		case "returned_degraded":
			run.Degraded++
		case "returned_unknown":
			run.Unknown++
		case "not_configured":
			run.NotConfigured++
		case "skipped":
			run.Skipped++
		case "not_supported":
			run.NotSupported++
		default:
			run.Failed++
		}
	}
	run.Status = "success"
	if run.Degraded > 0 || run.Unknown > 0 || run.NotConfigured > 0 || run.Skipped > 0 || run.NotSupported > 0 {
		run.Status = "degraded"
	}
	if run.Failed > 0 {
		run.Status = "failed"
	}
	if run.NotConfigured == run.Total && run.Total > 0 {
		run.Status = "not_configured"
	}
	run.FinishedAt = time.Now().UTC()
	if len(jobs) == 0 {
		run.Status = "no_capabilities_configured"
	}
	if s.store != nil {
		run.Persisted = true
		if err := s.store.SaveProviderProbeRun(ctx, run); err != nil {
			run.Persisted = false
			return run, fmt.Errorf("persist provider-probe run: %w", err)
		}
	}
	return run, nil
}

func isStockScopedProbeCapability(capability string) bool {
	switch capability {
	case "quote", "daily_bars", "stock_basic", "stock_search", "security_trading_status", "price_limit_event", "stock_boards", "announcements", "fundamentals", "valuations", "capital_flow", "chip_distribution":
		return true
	default:
		return false
	}
}

func probeSymbolMatchesBoard(symbol, market string) bool {
	if len(symbol) != 9 {
		return false
	}
	code, exchange := symbol[:6], symbol[7:]
	switch market {
	case "SH_MAIN":
		return exchange == "SH" && (strings.HasPrefix(code, "600") || strings.HasPrefix(code, "601") || strings.HasPrefix(code, "603") || strings.HasPrefix(code, "605"))
	case "SZ_MAIN":
		return exchange == "SZ" && (strings.HasPrefix(code, "000") || strings.HasPrefix(code, "001") || strings.HasPrefix(code, "002") || strings.HasPrefix(code, "003"))
	case "CHINEXT":
		return exchange == "SZ" && (strings.HasPrefix(code, "300") || strings.HasPrefix(code, "301"))
	case "STAR":
		return exchange == "SH" && (strings.HasPrefix(code, "688") || strings.HasPrefix(code, "689"))
	case "BSE":
		return exchange == "BJ"
	default:
		return false
	}
}

func isCoreProbeCapability(capability string) bool {
	for _, candidate := range model.ProviderProbeCoreCapabilities() {
		if candidate == capability {
			return true
		}
	}
	return false
}

func classifyProbeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	message := strings.ToLower(err.Error())
	switch {
	case strings.Contains(message, "429"), strings.Contains(message, "too many requests"), strings.Contains(message, "rate limit"):
		return "rate_limited"
	case strings.Contains(message, "401"), strings.Contains(message, "unauthorized"), strings.Contains(message, "invalid token"):
		return "authentication_failed"
	case strings.Contains(message, "403"), strings.Contains(message, "forbidden"), strings.Contains(message, "permission denied"):
		return "permission_denied"
	}
	if match := probeHTTPStatusPattern.FindStringSubmatch(err.Error()); len(match) == 2 {
		return "http_" + match[1]
	}
	if strings.Contains(message, "contract") || strings.Contains(message, "invalid") || strings.Contains(message, "missing") || strings.Contains(message, "unexpected") {
		return "contract_invalid"
	}
	if strings.Contains(message, "no such host") || strings.Contains(message, "dial tcp") ||
		strings.Contains(message, "connection refused") || strings.Contains(message, "connection reset") ||
		strings.Contains(message, "eof") || strings.Contains(message, "network is unreachable") {
		return "network_error"
	}
	return "provider_error"
}

func probeHTTPStatus(err error) int {
	if err == nil {
		return 0
	}
	match := probeHTTPStatusPattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return 0
	}
	status, _ := strconv.Atoi(match[1])
	return status
}

func sanitizeProbeError(err error) string {
	if err == nil {
		return ""
	}
	message := probeURLQueryPattern.ReplaceAllString(err.Error(), `$1?[query redacted]`)
	message = probeSecretPattern.ReplaceAllString(message, `$1$2[redacted]`)
	message = strings.TrimSpace(message)
	if len(message) > 400 {
		message = message[:400] + "…"
	}
	return message
}

func probeErrorStage(code string) string {
	switch {
	case code == "timeout":
		return "timeout"
	case code == "cancelled":
		return "cancellation"
	case code == "rate_limited":
		return "rate_limit"
	case code == "authentication_failed":
		return "provider_auth"
	case code == "permission_denied":
		return "provider_permission"
	case code == "contract_invalid":
		return "contract"
	case code == "network_error":
		return "connect"
	case strings.HasPrefix(code, "http_"):
		return "http"
	default:
		return "provider"
	}
}

func setProbeFailure(item *model.ProviderProbeItem, err error) {
	if item == nil || err == nil {
		return
	}
	item.ErrorCode = classifyProbeError(err)
	item.ErrorStage = probeErrorStage(item.ErrorCode)
	item.ErrorMessage = sanitizeProbeError(err)
	item.HTTPStatus = probeHTTPStatus(err)
}

// ProviderProbeHistory returns persisted active-probe runs.
func (s *DataService) ProviderProbeHistory(ctx context.Context, limit int) (*model.ProviderProbeHistory, error) {
	if s.store == nil {
		return nil, errors.New("provider-probe history storage is not enabled")
	}
	if limit == 0 {
		limit = 20
	}
	if limit < 1 || limit > 100 {
		return nil, errors.New("provider-probe history limit must be between 1 and 100")
	}
	items, err := s.store.ListProviderProbeRuns(ctx, limit)
	if err != nil {
		return nil, err
	}
	return &model.ProviderProbeHistory{Items: items, Total: len(items)}, nil
}
