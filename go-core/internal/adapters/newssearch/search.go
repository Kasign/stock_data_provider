package newssearch

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/ports"
)

const (
	defaultBochaEndpoint = "https://api.bocha.cn/v1/web-search"
	defaultTavilyEndpoint = "https://api.tavily.com/search"
	defaultBraveNewsEndpoint = "https://api.search.brave.com/res/v1/news/search"
	defaultCacheTTL      = 10 * time.Minute
	maxResponseBytes     = 4 << 20
)

// 官方监管、交易所和信息披露网站仅用于可信度加权；不把搜索结果限制为这些域名。
var defaultOfficialNewsDomains = []string{
	"gov.cn", "csrc.gov.cn", "pbc.gov.cn",
	"sse.com.cn", "szse.cn", "bse.cn", "cninfo.com.cn",
}

// 可信财经媒体获得次级排序加分，不等于对单篇报道内容作真实性背书。
var defaultFinancialNewsDomains = []string{
	"xinhuanet.com", "people.com.cn", "chinanews.com.cn", "chinadaily.com.cn",
	"stcn.com", "cs.com.cn", "cnstock.com", "cls.cn", "yicai.com",
	"21jingji.com", "jrj.com.cn", "eastmoney.com",
}

// 仅过滤明显的荐股/导流营销词，避免用宽泛的情绪词误删正常财经报道。
var defaultNewsSpamTerms = []string{
	"免费荐股", "荐股群", "股票交流群", "稳赚不赔", "老师带单",
	"开户返佣", "开户链接", "加微信", "扫码入群", "牛股群",
}

// Config controls the Go-native Chinese news-search chain.
// Public SearXNG discovery is intentionally not enabled: configure trusted self-hosted endpoints explicitly.
// StockAnnouncementSource supplies stock-specific disclosure records without inventing publication times.
type StockAnnouncementSource interface {
	FetchStockAnnouncements(context.Context, string, string, string, int, int) (domain.StockAnnouncementSnapshot, error)
}

type Config struct {
	BochaAPIKeys       []string
	BochaEndpoint      string
	TavilyAPIKeys      []string
	TavilyEndpoint     string
	BraveSearchAPIKeys []string
	BraveNewsEndpoint  string
	SearXNGBaseURLs    []string
	TrustedDomains     []string
	BlockedTerms       []string
	MaxAgeDays         int
	StrategyProfile    string
	RequestTimeout     time.Duration
	CacheTTL           time.Duration
	StockAnnouncements StockAnnouncementSource
}

type Searcher struct {
	client             *http.Client
	providers          []remoteProvider
	fallback           ports.NewsSearcher
	logger             *slog.Logger
	windowDays         int
	cacheTTL           time.Duration
	trustedDomains     []string
	blockedTerms       []string
	stockAnnouncements StockAnnouncementSource

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type cacheEntry struct {
	items     []domain.NewsItem
	expiresAt time.Time
}

type remoteProvider interface {
	Name() string
	Search(context.Context, string, int, int) ([]searchCandidate, error)
}

type searchCandidate struct {
	Title                     string
	Snippet                   string
	URL                       string
	Source                    string
	PublishedRaw              string
	StockSpecificAnnouncement bool
}

type bochaProvider struct {
	client   *http.Client
	endpoint string
	keys     []string
	mu       sync.Mutex
	nextKey  int
}

type tavilyProvider struct {
	client   *http.Client
	endpoint string
	keys     []string
	mu       sync.Mutex
	nextKey  int
}

type braveNewsProvider struct {
	client   *http.Client
	endpoint string
	keys     []string
	mu       sync.Mutex
	nextKey  int
}

type searxngProvider struct {
	client *http.Client
	bases  []string
	mu     sync.Mutex
	next   int
}

type tavilyResponse struct {
	Results []struct {
		Title         string `json:"title"`
		URL           string `json:"url"`
		Content       string `json:"content"`
		PublishedDate string `json:"published_date"`
	} `json:"results"`
}

type braveNewsResponse struct {
	Results []struct {
		Title         string `json:"title"`
		URL           string `json:"url"`
		Description   string `json:"description"`
		PublishedDate string `json:"published_date"`
		PageAge       string `json:"page_age"`
		// page_fetched must never be treated as the article publication time.
	} `json:"results"`
}

type bochaResponse struct {
	Code json.RawMessage `json:"code"`
	Msg  string          `json:"msg"`
	Data struct {
		WebPages struct {
			Value []struct {
				Name          string          `json:"name"`
				URL           string          `json:"url"`
				SiteName      string          `json:"siteName"`
				DatePublished json.RawMessage `json:"datePublished"`
				Summary       string          `json:"summary"`
				Snippet       string          `json:"snippet"`
			} `json:"value"`
		} `json:"webPages"`
	} `json:"data"`
}

type searxngResponse struct {
	Results []struct {
		Title                  string          `json:"title"`
		URL                    string          `json:"url"`
		Content                string          `json:"content"`
		Description            string          `json:"description"`
		PublishedDate          json.RawMessage `json:"publishedDate"`
		PublishedDateSnakeCase json.RawMessage `json:"published_date"`
		PublishedAt            json.RawMessage `json:"published_at"`
	} `json:"results"`
}

// New builds a domestic-first chain: Bocha, followed by explicitly configured SearXNG instances.
// The existing NewsSearcher remains the compatibility fallback if native results are unavailable or stale.
func New(cfg Config, fallback ports.NewsSearcher, logger *slog.Logger) *Searcher {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = 10 * time.Second
	}
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = defaultCacheTTL
	}
	if strings.TrimSpace(cfg.BochaEndpoint) == "" {
		cfg.BochaEndpoint = defaultBochaEndpoint
	}
	if strings.TrimSpace(cfg.TavilyEndpoint) == "" {
		cfg.TavilyEndpoint = defaultTavilyEndpoint
	}
	if strings.TrimSpace(cfg.BraveNewsEndpoint) == "" {
		cfg.BraveNewsEndpoint = defaultBraveNewsEndpoint
	}
	if cfg.MaxAgeDays <= 0 {
		cfg.MaxAgeDays = 3
	}
	profileDays := map[string]int{"ultra_short": 1, "short": 3, "medium": 7, "long": 30}
	profile := strings.ToLower(strings.TrimSpace(cfg.StrategyProfile))
	if profile == "" {
		profile = "short"
	}
	if _, ok := profileDays[profile]; !ok {
		logger.Warn("unknown NEWS_STRATEGY_PROFILE; using short", "profile", profile)
		profile = "short"
	}
	windowDays := cfg.MaxAgeDays
	if profileDays[profile] < windowDays {
		windowDays = profileDays[profile]
	}
	if windowDays < 1 {
		windowDays = 1
	}

	client := &http.Client{Timeout: cfg.RequestTimeout}
	searcher := &Searcher{
		client: client, fallback: fallback, logger: logger,
		windowDays: windowDays, cacheTTL: cfg.CacheTTL, stockAnnouncements: cfg.StockAnnouncements,
		trustedDomains: normalizeDomains(append(append([]string{}, defaultOfficialNewsDomains...), cfg.TrustedDomains...)),
		blockedTerms:   normalizeTerms(append(append([]string{}, defaultNewsSpamTerms...), cfg.BlockedTerms...)),
		cache:          make(map[string]cacheEntry),
	}
	keys := compactStrings(cfg.BochaAPIKeys)
	if len(keys) > 0 {
		searcher.providers = append(searcher.providers, &bochaProvider{
			client: client, endpoint: strings.TrimSpace(cfg.BochaEndpoint), keys: keys,
		})
	}
	tavilyKeys := compactStrings(cfg.TavilyAPIKeys)
	if len(tavilyKeys) > 0 {
		searcher.providers = append(searcher.providers, &tavilyProvider{
			client: client, endpoint: strings.TrimSpace(cfg.TavilyEndpoint), keys: tavilyKeys,
		})
	}
	braveKeys := compactStrings(cfg.BraveSearchAPIKeys)
	if len(braveKeys) > 0 {
		searcher.providers = append(searcher.providers, &braveNewsProvider{
			client: client, endpoint: strings.TrimSpace(cfg.BraveNewsEndpoint), keys: braveKeys,
		})
	}
	bases := normalizeSearchURLs(cfg.SearXNGBaseURLs)
	if len(bases) > 0 {
		searcher.providers = append(searcher.providers, &searxngProvider{client: client, bases: bases})
	}
	return searcher
}

// NewFromEnv reuses Python-compatible environment names; credentials are never logged.
func NewFromEnv(fallback ports.NewsSearcher, logger *slog.Logger, announcementSources ...StockAnnouncementSource) *Searcher {
	cfg := Config{
		BochaAPIKeys:       splitCSV(os.Getenv("BOCHA_API_KEYS")),
		BochaEndpoint:      envOr("GO_NEWS_BOCHA_ENDPOINT", defaultBochaEndpoint),
		TavilyAPIKeys:      splitCSV(envOr("TAVILY_API_KEYS", os.Getenv("TAVILY_API_KEY"))),
		TavilyEndpoint:     envOr("GO_NEWS_TAVILY_ENDPOINT", defaultTavilyEndpoint),
		BraveSearchAPIKeys: splitCSV(envOr("BRAVE_SEARCH_API_KEYS", envOr("BRAVE_SEARCH_API_KEY", os.Getenv("BRAVE_API_KEY")))),
		BraveNewsEndpoint:  envOr("GO_NEWS_BRAVE_NEWS_ENDPOINT", defaultBraveNewsEndpoint),
		SearXNGBaseURLs:    splitCSV(os.Getenv("SEARXNG_BASE_URLS")),
		TrustedDomains:  splitCSV(os.Getenv("GO_NEWS_TRUSTED_DOMAINS")),
		BlockedTerms:    splitCSV(os.Getenv("GO_NEWS_BLOCKED_TERMS")),
		MaxAgeDays:      positiveIntEnv("NEWS_MAX_AGE_DAYS", 3),
		StrategyProfile: envOr("NEWS_STRATEGY_PROFILE", "short"),
		RequestTimeout:  time.Duration(positiveIntEnv("NEWS_SEARCH_TIMEOUT_SECONDS", 10)) * time.Second,
	}
	if len(announcementSources) > 0 {
		cfg.StockAnnouncements = announcementSources[0]
	}
	return New(cfg, fallback, logger)
}

// ProviderNames lists configured native providers without exposing credentials.
func (s *Searcher) ProviderNames() []string {
	names := make([]string, 0, len(s.providers))
	for _, p := range s.providers {
		names = append(names, p.Name())
	}
	if s.stockAnnouncements != nil {
		names = append(names, "GoStockAnnouncements")
	}
	return names
}

func (s *Searcher) Available() bool { return len(s.providers) > 0 || s.stockAnnouncements != nil }

func (s *Searcher) SearchStockNews(ctx context.Context, code, name string, limit int) ([]domain.NewsItem, error) {
	code, name = strings.TrimSpace(code), strings.TrimSpace(name)
	if limit < 1 {
		limit = 5
	}
	if limit > 20 {
		limit = 20
	}
	queryParts := make([]string, 0, 4)
	if name != "" {
		queryParts = append(queryParts, name)
	}
	if code != "" {
		queryParts = append(queryParts, code)
	}
	queryParts = append(queryParts, "股票", "最新消息")
	query := strings.Join(queryParts, " ")
	cacheKey := fmt.Sprintf("stock|%s|%s|%d|%d", strings.ToLower(code), strings.ToLower(name), limit, s.windowDays)
	return s.searchWithFallback(ctx, cacheKey, query, code, name, limit,
		func(ctx context.Context) ([]domain.NewsItem, error) {
			if s.fallback == nil {
				return nil, errors.New("no configured news-search provider")
			}
			return s.fallback.SearchStockNews(ctx, code, name, limit)
		})
}

func (s *Searcher) SearchMarketNews(ctx context.Context, region string) ([]domain.NewsItem, error) {
	region = strings.TrimSpace(region)
	query := "中国 A 股市场 最新新闻"
	if region != "" && !strings.EqualFold(region, "cn") && !strings.EqualFold(region, "china") && region != "中国" {
		query = region + " 股市 最新新闻"
	}
	cacheKey := fmt.Sprintf("market|%s|%d|%d", strings.ToLower(region), 5, s.windowDays)
	return s.searchWithFallback(ctx, cacheKey, query, "", "", 5,
		func(ctx context.Context) ([]domain.NewsItem, error) {
			if s.fallback == nil {
				return nil, errors.New("no configured news-search provider")
			}
			return s.fallback.SearchMarketNews(ctx, region)
		})
}

func (s *Searcher) searchWithFallback(ctx context.Context, cacheKey, query, code, name string, limit int, fallbackCall func(context.Context) ([]domain.NewsItem, error)) ([]domain.NewsItem, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if cached, ok := s.cacheGet(cacheKey); ok {
		return cached, nil
	}
	if !s.Available() {
		items, err := fallbackCall(ctx)
		if err == nil {
			s.cachePut(cacheKey, items)
		}
		return items, err
	}

	nativeItems, nativeErr := s.searchNative(ctx, query, code, name, limit)
	if len(nativeItems) > 0 {
		s.cachePut(cacheKey, nativeItems)
		return nativeItems, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	fallbackItems, fallbackErr := fallbackCall(ctx)
	if fallbackErr == nil {
		s.cachePut(cacheKey, fallbackItems)
		return fallbackItems, nil
	}
	if nativeErr != nil {
		return nil, fmt.Errorf("native news search failed (%v); Python bridge fallback failed: %w", nativeErr, fallbackErr)
	}
	return nil, fallbackErr
}

func (s *Searcher) searchNative(ctx context.Context, query, code, name string, limit int) ([]domain.NewsItem, error) {
	providerLimit := limit * 2
	if providerLimit < 10 {
		providerLimit = 10
	}
	if providerLimit > 50 {
		providerLimit = 50
	}

	type providerResult struct {
		index      int
		name       string
		candidates []searchCandidate
		err        error
	}
	results := make(chan providerResult, len(s.providers))
	for index, provider := range s.providers {
		index, provider := index, provider
		go func() {
			raw, err := provider.Search(ctx, query, s.windowDays, providerLimit)
			results <- providerResult{index: index, name: provider.Name(), candidates: raw, err: err}
		}()
	}

	var announcementCh chan []searchCandidate
	if strings.TrimSpace(code) != "" && s.stockAnnouncements != nil {
		announcementCh = make(chan []searchCandidate, 1)
		go func() {
			announcementCh <- s.fetchStockAnnouncementCandidates(ctx, code)
		}()
	}
	announcementResolved := announcementCh == nil
	var announcementCandidates []searchCandidate
	collectAnnouncements := func() []searchCandidate {
		if announcementResolved {
			return announcementCandidates
		}
		announcementResolved = true
		timer := time.NewTimer(2200 * time.Millisecond)
		defer timer.Stop()
		select {
		case announcementCandidates = <-announcementCh:
		case <-ctx.Done():
		case <-timer.C:
			s.logger.Warn("stock announcement search exceeded latency budget")
		}
		return announcementCandidates
	}

	byProvider := make([][]searchCandidate, len(s.providers))
	failures := make([]string, 0, len(s.providers))
	for received := 0; received < len(s.providers); received++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case result := <-results:
			if result.err != nil {
				s.logger.Warn("native news provider request failed", "provider", result.name, "error", result.err)
				failures = append(failures, result.name+": "+result.err.Error())
				continue
			}
			byProvider[result.index] = result.candidates
		}
	}

	raw := make([]searchCandidate, 0, providerLimit*len(s.providers))
	raw = append(raw, collectAnnouncements()...)
	for _, candidates := range byProvider {
		raw = append(raw, candidates...)
	}
	items := s.filterAndRank(raw, code, name)
	if len(items) > 0 {
		return trimRanked(items, limit), nil
	}
	if len(failures) > 0 {
		return nil, errors.New(strings.Join(failures, "; "))
	}
	return []domain.NewsItem{}, nil
}

func (s *Searcher) fetchStockAnnouncementCandidates(ctx context.Context, code string) []searchCandidate {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -(s.windowDays - 1)).Format("2006-01-02")

	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	snapshot, err := s.stockAnnouncements.FetchStockAnnouncements(requestCtx, code, startDate, endDate, 1, 100)
	if err != nil {
		if ctx.Err() == nil {
			s.logger.Warn("Go stock-announcement source unavailable", "error", err)
		}
		return nil
	}
	if !sameNewsStockCode(code, snapshot.Symbol) || snapshot.StartDate != startDate || snapshot.EndDate != endDate ||
		snapshot.Page != 1 || snapshot.PageSize != 100 || snapshot.Returned != len(snapshot.Items) {
		s.logger.Warn("Go stock-announcement source returned a mismatched snapshot")
		return nil
	}
	if !snapshot.HasData {
		return nil
	}
	candidates := make([]searchCandidate, 0, len(snapshot.Items))
	for _, item := range snapshot.Items {
		status := strings.ToLower(strings.TrimSpace(item.QualityStatus))
		if status != "valid" && status != "degraded" {
			continue
		}
		if !sameNewsStockCode(code, item.Symbol) || item.Source != snapshot.Source ||
			strings.TrimSpace(item.Title) == "" || strings.TrimSpace(item.SourceURL) == "" {
			continue
		}
		published := strings.TrimSpace(item.AnnouncementDate)
		if item.PublishedAt != nil && !item.PublishedAt.IsZero() {
			published = item.PublishedAt.Format(time.RFC3339Nano)
		}
		snippet := strings.TrimSpace(strings.Join([]string{item.StockName, item.Category}, " "))
		candidates = append(candidates, searchCandidate{
			Title: item.Title, Snippet: snippet, URL: item.SourceURL, Source: item.Source,
			PublishedRaw: published, StockSpecificAnnouncement: true,
		})
	}
	return candidates
}

func sameNewsStockCode(left, right string) bool {
	canonical := func(value string) string {
		value = strings.ToUpper(strings.TrimSpace(value))
		if index := strings.IndexByte(value, '.'); index >= 0 {
			value = value[:index]
		}
		for _, prefix := range []string{"SH", "SZ", "BJ"} {
			if strings.HasPrefix(value, prefix) && len(value) == 8 {
				value = value[2:]
				break
			}
		}
		return value
	}
	return canonical(left) != "" && canonical(left) == canonical(right)
}

type rankedNews struct {
	item   domain.NewsItem
	score  int
	date   time.Time
	direct bool
}

func (s *Searcher) filterAndRank(raw []searchCandidate, code, name string) []rankedNews {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	earliest := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -(s.windowDays - 1))

	seenURLs := make(map[string]struct{}, len(raw))
	seenTitles := make(map[string]struct{}, len(raw))
	items := make([]rankedNews, 0, len(raw))
	for _, candidate := range raw {
		title := cleanText(candidate.Title)
		snippet := cleanText(candidate.Snippet)
		articleURL := strings.TrimSpace(candidate.URL)
		parsedURL, err := url.Parse(articleURL)
		if err != nil || parsedURL.Host == "" || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || title == "" {
			continue
		}
		published, ok := parsePublishedTime(candidate.PublishedRaw)
		if !ok {
			continue
		}
		published = published.In(loc)
		// 未来发布时间通常代表上游字段错位、时区误判或搜索索引脏数据。
		if published.Before(earliest) || published.After(now) {
			continue
		}
		if isSpamNews(title, snippet, s.blockedTerms) {
			continue
		}
		canonical := canonicalArticleURL(parsedURL)
		titleKey := normalizeTitle(title)
		if _, exists := seenURLs[canonical]; exists {
			continue
		}
		if _, exists := seenTitles[titleKey]; exists {
			continue
		}
		seenURLs[canonical] = struct{}{}
		seenTitles[titleKey] = struct{}{}
		source := strings.TrimSpace(candidate.Source)
		if source == "" {
			source = strings.TrimPrefix(strings.ToLower(parsedURL.Hostname()), "www.")
		}
		score, direct := scoreRelevance(title, snippet, code, name)
		if candidate.StockSpecificAnnouncement {
			score += 45
			direct = true
		}
		score += newsSourceScore(parsedURL.Hostname(), s.trustedDomains)
		items = append(items, rankedNews{
			item: domain.NewsItem{
				Title: title, Summary: snippet, URL: articleURL, Source: source,
				PublishedAt: published.Format("2006-01-02"), RetrievedAt: time.Now().UTC().Format(time.RFC3339Nano),
				Relevance: float64(score),
			},
			score: score, date: published, direct: direct,
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].direct != items[j].direct {
			return items[i].direct
		}
		if items[i].score != items[j].score {
			return items[i].score > items[j].score
		}
		return items[i].date.After(items[j].date)
	})
	return items
}

func hasDirectArticle(items []rankedNews) bool {
	for _, item := range items {
		if item.direct {
			return true
		}
	}
	return false
}

func rankTotal(items []rankedNews) int {
	total := 0
	for _, item := range items {
		total += item.score
	}
	return total
}

func trimRanked(items []rankedNews, limit int) []domain.NewsItem {
	if len(items) > limit {
		items = items[:limit]
	}
	result := make([]domain.NewsItem, len(items))
	for i, item := range items {
		result[i] = item.item
	}
	return result
}

func scoreRelevance(title, snippet, code, name string) (int, bool) {
	titleLower, snippetLower := strings.ToLower(title), strings.ToLower(snippet)
	code = strings.ToLower(strings.TrimSpace(code))
	code = strings.TrimSuffix(strings.TrimSuffix(code, ".sh"), ".sz")
	name = strings.ToLower(strings.TrimSpace(name))
	score, direct := 0, false
	if code != "" && len(code) >= 4 && strings.Contains(titleLower, code) {
		score += 70
		direct = true
	}
	if name != "" && strings.Contains(titleLower, name) {
		score += 65
		direct = true
	}
	if code != "" && len(code) >= 4 && strings.Contains(snippetLower, code) {
		score += 25
	}
	if name != "" && strings.Contains(snippetLower, name) {
		score += 20
	}
	for _, term := range []string{"公告", "业绩", "财报", "回购", "增持", "减持", "分红", "中标", "合同", "诉讼", "处罚", "预告", "重大"} {
		if strings.Contains(titleLower, term) {
			score += 4
		}
	}
	// 市场复盘使用独立主题词打分；标题权重大于摘要，避免大量无关的近期网页挤占结果。
	for _, term := range []string{"降准", "降息", "央行", "证监会", "财政部", "国务院", "国资委", "上证指数", "深证成指", "创业板", "a股", "交易所", "成交额", "ipo", "发行上市", "重大资产重组", "产业政策", "货币政策", "财政政策"} {
		if strings.Contains(titleLower, term) {
			score += 5
		} else if strings.Contains(snippetLower, term) {
			score += 2
		}
	}
	return score, direct
}

func (s *Searcher) cacheGet(key string) ([]domain.NewsItem, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.cache[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.cache, key)
		return nil, false
	}
	return append([]domain.NewsItem(nil), entry.items...), true
}

func (s *Searcher) cachePut(key string, items []domain.NewsItem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.cache) >= 512 {
		for cacheKey, entry := range s.cache {
			if time.Now().After(entry.expiresAt) || len(s.cache) >= 512 {
				delete(s.cache, cacheKey)
			}
		}
	}
	s.cache[key] = cacheEntry{items: append([]domain.NewsItem(nil), items...), expiresAt: time.Now().Add(s.cacheTTL)}
}

func (p *bochaProvider) Name() string { return "Bocha" }

func (p *bochaProvider) Search(ctx context.Context, query string, days, limit int) ([]searchCandidate, error) {
	p.mu.Lock()
	start := p.nextKey
	p.nextKey = (p.nextKey + 1) % len(p.keys)
	p.mu.Unlock()
	var failures []string
	for i := 0; i < len(p.keys); i++ {
		key := p.keys[(start+i)%len(p.keys)]
		freshness := "oneWeek"
		switch {
		case days <= 1:
			freshness = "oneDay"
		case days <= 7:
			freshness = "oneWeek"
		case days <= 30:
			freshness = "oneMonth"
		default:
			freshness = "oneYear"
		}
		payload := map[string]any{"query": query, "freshness": freshness, "summary": true, "count": limit}
		body, _ := json.Marshal(payload)
		headers := http.Header{
			"Authorization": []string{"Bearer " + key},
			"Content-Type":  []string{"application/json"},
			"Accept":        []string{"application/json"},
		}
		respBody, status, err := requestJSON(ctx, p.client, http.MethodPost, p.endpoint, headers, body)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if status != http.StatusOK {
			failures = append(failures, fmt.Sprintf("HTTP %d", status))
			continue
		}
		var response bochaResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			failures = append(failures, "invalid JSON response")
			continue
		}
		if rawMessage(response.Code) != "200" {
			message := strings.TrimSpace(response.Msg)
			if message == "" {
				message = "provider returned non-success code " + rawMessage(response.Code)
			}
			failures = append(failures, message)
			continue
		}
		items := response.Data.WebPages.Value
		result := make([]searchCandidate, 0, len(items))
		if len(items) > limit {
			items = items[:limit]
		}
		for _, item := range items {
			snippet := item.Summary
			if strings.TrimSpace(snippet) == "" {
				snippet = item.Snippet
			}
			result = append(result, searchCandidate{
				Title: item.Name, Snippet: truncateRunes(snippet, 500), URL: item.URL,
				Source: item.SiteName, PublishedRaw: rawMessage(item.DatePublished),
			})
		}
		return result, nil
	}
	return nil, fmt.Errorf("all configured Bocha keys failed: %s", strings.Join(failures, "; "))
}


func (p *tavilyProvider) Name() string { return "Tavily" }

func (p *tavilyProvider) Search(ctx context.Context, query string, days, limit int) ([]searchCandidate, error) {
	p.mu.Lock()
	start := p.nextKey
	p.nextKey = (p.nextKey + 1) % len(p.keys)
	p.mu.Unlock()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	startDate := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	endDate := now.Format("2006-01-02")
	var failures []string
	for i := 0; i < len(p.keys); i++ {
		key := p.keys[(start+i)%len(p.keys)]
		payload := map[string]any{
			"query": query, "topic": "news", "search_depth": "basic", "max_results": limit,
			"include_published_date": true, "filter_by_published_date": true,
			"start_date": startDate, "end_date": endDate,
			"include_answer": false, "include_raw_content": false, "include_images": false,
		}
		body, _ := json.Marshal(payload)
		headers := http.Header{
			"Authorization": []string{"Bearer " + key},
			"Content-Type":  []string{"application/json"},
			"Accept":        []string{"application/json"},
		}
		respBody, status, err := requestJSON(ctx, p.client, http.MethodPost, p.endpoint, headers, body)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if status != http.StatusOK {
			failures = append(failures, fmt.Sprintf("HTTP %d", status))
			continue
		}
		var response tavilyResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			failures = append(failures, "invalid JSON response")
			continue
		}
		result := make([]searchCandidate, 0, len(response.Results))
		for _, item := range response.Results {
			result = append(result, searchCandidate{
				Title: item.Title, Snippet: truncateRunes(item.Content, 500),
				URL: item.URL, PublishedRaw: item.PublishedDate,
			})
		}
		return result, nil
	}
	return nil, fmt.Errorf("all configured Tavily keys failed: %s", strings.Join(failures, "; "))
}

func (p *braveNewsProvider) Name() string { return "BraveNews" }

func (p *braveNewsProvider) Search(ctx context.Context, query string, days, limit int) ([]searchCandidate, error) {
	p.mu.Lock()
	start := p.nextKey
	p.nextKey = (p.nextKey + 1) % len(p.keys)
	p.mu.Unlock()
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	startDate := now.AddDate(0, 0, -(days - 1)).Format("2006-01-02")
	endDate := now.Format("2006-01-02")
	freshness := fmt.Sprintf("%sto%s", startDate, endDate)
	switch {
	case days <= 1:
		freshness = "pd"
	case days <= 7:
		freshness = "pw"
	case days <= 31:
		freshness = "pm"
	case days <= 365:
		freshness = "py"
	}
	var failures []string
	for i := 0; i < len(p.keys); i++ {
		key := p.keys[(start+i)%len(p.keys)]
		endpoint, err := url.Parse(p.endpoint)
		if err != nil || endpoint.Host == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
			return nil, errors.New("invalid Brave News endpoint URL")
		}
		params := endpoint.Query()
		params.Set("q", query)
		params.Set("count", strconv.Itoa(limit))
		params.Set("freshness", freshness)
		params.Set("country", "CN")
		params.Set("search_lang", "zh-hans")
		params.Set("ui_lang", "zh-CN")
		params.Set("spellcheck", "false")
		params.Set("safesearch", "strict")
		endpoint.RawQuery = params.Encode()
		headers := http.Header{
			"X-Subscription-Token": []string{key},
			"Accept":               []string{"application/json"},
			"User-Agent":           []string{"stock-analysis-go/1.0"},
		}
		respBody, status, err := requestJSON(ctx, p.client, http.MethodGet, endpoint.String(), headers, nil)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if status != http.StatusOK {
			failures = append(failures, fmt.Sprintf("HTTP %d", status))
			continue
		}
		var response braveNewsResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			failures = append(failures, "invalid JSON response")
			continue
		}
		result := make([]searchCandidate, 0, len(response.Results))
		for _, item := range response.Results {
			published := strings.TrimSpace(item.PublishedDate)
			if published == "" {
				published = strings.TrimSpace(item.PageAge)
			}
			result = append(result, searchCandidate{
				Title: item.Title, Snippet: truncateRunes(item.Description, 500),
				URL: item.URL, PublishedRaw: published,
			})
		}
		return result, nil
	}
	return nil, fmt.Errorf("all configured Brave News keys failed: %s", strings.Join(failures, "; "))
}

func (p *searxngProvider) Name() string { return "SearXNG" }

func (p *searxngProvider) Search(ctx context.Context, query string, days, limit int) ([]searchCandidate, error) {
	p.mu.Lock()
	start := p.next
	p.next = (p.next + 1) % len(p.bases)
	p.mu.Unlock()
	timeRange := "week"
	switch {
	case days <= 1:
		timeRange = "day"
	case days <= 7:
		timeRange = "week"
	case days <= 30:
		timeRange = "month"
	default:
		timeRange = "year"
	}
	var failures []string
	for i := 0; i < len(p.bases); i++ {
		base := strings.TrimRight(p.bases[(start+i)%len(p.bases)], "/")
		endpoint := base
		if !strings.HasSuffix(strings.ToLower(endpoint), "/search") {
			endpoint += "/search"
		}
		parsed, err := url.Parse(endpoint)
		if err != nil {
			failures = append(failures, "invalid endpoint URL")
			continue
		}
		params := parsed.Query()
		params.Set("q", query)
		params.Set("format", "json")
		params.Set("time_range", timeRange)
		parsed.RawQuery = params.Encode()
		headers := http.Header{"Accept": []string{"application/json"}, "User-Agent": []string{"stock-analysis-go/1.0"}}
		respBody, status, err := requestJSON(ctx, p.client, http.MethodGet, parsed.String(), headers, nil)
		if err != nil {
			failures = append(failures, err.Error())
			continue
		}
		if status != http.StatusOK {
			failures = append(failures, fmt.Sprintf("HTTP %d", status))
			continue
		}
		var response searxngResponse
		if err := json.Unmarshal(respBody, &response); err != nil {
			failures = append(failures, "invalid JSON response")
			continue
		}
		items := response.Results
		if len(items) > limit {
			items = items[:limit]
		}
		result := make([]searchCandidate, 0, len(items))
		for _, item := range items {
			published := rawMessage(item.PublishedDate)
			if published == "" {
				published = rawMessage(item.PublishedDateSnakeCase)
			}
			if published == "" {
				published = rawMessage(item.PublishedAt)
			}
			snippet := item.Content
			if strings.TrimSpace(snippet) == "" {
				snippet = item.Description
			}
			result = append(result, searchCandidate{
				Title: item.Title, Snippet: truncateRunes(snippet, 500),
				URL: item.URL, PublishedRaw: published,
			})
		}
		return result, nil
	}
	return nil, fmt.Errorf("all configured SearXNG instances failed: %s", strings.Join(failures, "; "))
}

func requestJSON(ctx context.Context, client *http.Client, method, endpoint string, headers http.Header, body []byte) ([]byte, int, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		var requestBody io.Reader
		if body != nil {
			requestBody = bytes.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, endpoint, requestBody)
		if err != nil {
			return nil, 0, err
		}
		for key, values := range headers {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				return nil, 0, ctx.Err()
			}
			continue
		}
		payload, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("temporary HTTP %d", resp.StatusCode)
			if attempt == 0 {
				select {
				case <-ctx.Done():
					return nil, 0, ctx.Err()
				case <-time.After(150 * time.Millisecond):
				}
				continue
			}
		}
		return payload, resp.StatusCode, nil
	}
	return nil, 0, lastErr
}

func parsePublishedTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "null" || raw == "-" {
		return time.Time{}, false
	}
	if unquoted, err := strconv.Unquote(raw); err == nil {
		raw = strings.TrimSpace(unquoted)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	for _, layout := range []string{
		time.RFC3339Nano, time.RFC3339, time.RFC1123Z, time.RFC1123,
		"2006-01-02T15:04:05.000Z07:00", "2006-01-02T15:04:05",
		"2006-01-02 15:04:05.000", "2006-01-02 15:04:05", "2006-01-02 15:04",
		"2006-01-02", "2006/01/02 15:04:05", "2006/01/02",
	} {
		var parsed time.Time
		if layout == time.RFC3339Nano || layout == time.RFC3339 || layout == time.RFC1123Z || layout == time.RFC1123 || strings.HasSuffix(layout, "Z07:00") {
			parsed, err = time.Parse(layout, raw)
		} else {
			parsed, err = time.ParseInLocation(layout, raw, loc)
		}
		if err == nil {
			return parsed, true
		}
	}
	if numeric, err := strconv.ParseInt(raw, 10, 64); err == nil {
		if numeric > 1_000_000_000_000 {
			return time.UnixMilli(numeric), true
		}
		if numeric > 1_000_000_000 {
			return time.Unix(numeric, 0), true
		}
	}
	return time.Time{}, false
}

func rawMessage(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(string(raw))
}

func isSpamNews(title, snippet string, blockedTerms []string) bool {
	content := strings.ToLower(cleanText(title + " " + snippet))
	for _, term := range blockedTerms {
		term = strings.ToLower(strings.TrimSpace(term))
		if term != "" && strings.Contains(content, term) {
			return true
		}
	}
	return false
}

func newsSourceScore(host string, trustedDomains []string) int {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	if matchesNewsDomain(host, trustedDomains) {
		return 30
	}
	if matchesNewsDomain(host, defaultFinancialNewsDomains) {
		return 12
	}
	return 0
}

func matchesNewsDomain(host string, domains []string) bool {
	host = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(host), "."))
	for _, domain := range domains {
		domain = strings.ToLower(strings.Trim(strings.TrimSpace(domain), "."))
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func normalizeDomains(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.ToLower(strings.TrimSpace(raw))
		if strings.Contains(value, "://") {
			parsed, err := url.Parse(value)
			if err != nil {
				continue
			}
			value = parsed.Hostname()
		}
		value = strings.Trim(strings.TrimSpace(value), ".")
		if value == "" || strings.ContainsAny(value, "/:@ \t\r\n") {
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

func normalizeTerms(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, raw := range values {
		value := strings.ToLower(cleanText(raw))
		if value == "" {
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

func canonicalArticleURL(parsed *url.URL) string {
	copyURL := *parsed
	copyURL.Fragment = ""
	copyURL.Scheme = strings.ToLower(copyURL.Scheme)
	copyURL.Host = strings.ToLower(copyURL.Host)
	if strings.HasPrefix(copyURL.Host, "www.") {
		copyURL.Host = strings.TrimPrefix(copyURL.Host, "www.")
	}
	query := copyURL.Query()
	for key := range query {
		if strings.HasPrefix(strings.ToLower(key), "utm_") || strings.EqualFold(key, "spm") || strings.EqualFold(key, "from") {
			query.Del(key)
		}
	}
	copyURL.RawQuery = query.Encode()
	if copyURL.Path != "/" {
		copyURL.Path = strings.TrimRight(copyURL.Path, "/")
	}
	return copyURL.String()
}

func normalizeTitle(title string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(title))), " ")
}

func cleanText(raw string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(strings.ReplaceAll(raw, "\u00a0", " ")), " "))
}

func truncateRunes(value string, max int) string {
	runes := []rune(value)
	if len(runes) <= max {
		return value
	}
	return string(runes[:max])
}

func compactStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for _, item := range values {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		result = append(result, item)
	}
	return result
}

func normalizeSearchURLs(values []string) []string {
	result := make([]string, 0, len(values))
	for _, raw := range compactStrings(values) {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
			continue
		}
		result = append(result, strings.TrimRight(raw, "/"))
	}
	return result
}

func splitCSV(raw string) []string { return compactStrings(strings.Split(raw, ",")) }

func positiveIntEnv(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
