package newssearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/go-core/internal/domain"
)

type fallbackSearcher struct {
	stockCalls  int
	marketCalls int
	stockItems  []domain.NewsItem
	marketItems []domain.NewsItem
	err         error
}

func (f *fallbackSearcher) SearchStockNews(context.Context, string, string, int) ([]domain.NewsItem, error) {
	f.stockCalls++
	return append([]domain.NewsItem(nil), f.stockItems...), f.err
}

func (f *fallbackSearcher) SearchMarketNews(context.Context, string) ([]domain.NewsItem, error) {
	f.marketCalls++
	return append([]domain.NewsItem(nil), f.marketItems...), f.err
}

func freshDateString(daysAgo int) string {
	return time.Now().In(time.FixedZone("CST", 8*60*60)).AddDate(0, 0, -daysAgo).Format("2006-01-02")
}

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(ioDiscard{}, nil)) }

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }

func TestBochaSearchMapsFreshDomesticNewsAndRequestFreshness(t *testing.T) {
	published := freshDateString(1) + "T10:15:00+08:00"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Fatalf("authorization=%q", got)
		}
		var request struct {
			Query     string `json:"query"`
			Freshness string `json:"freshness"`
			Summary   bool   `json:"summary"`
			Count     int    `json:"count"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request.Query != "招商银行 600036.SH 股票 最新消息" || request.Freshness != "oneWeek" || !request.Summary || request.Count != 10 {
			t.Fatalf("unexpected Bocha request: %+v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":200,"data":{"webPages":{"value":[{"name":"招商银行披露业绩公告 600036","url":"https://finance.example.com/news/1?utm_source=test","siteName":"财经媒体","datePublished":%q,"summary":"招商银行业绩更新"}]}}}`, published)
	}))
	defer server.Close()
	fallback := &fallbackSearcher{}
	searcher := New(Config{BochaAPIKeys: []string{"test-key"}, BochaEndpoint: server.URL, MaxAgeDays: 3, StrategyProfile: "short", RequestTimeout: time.Second}, fallback, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 5)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.stockCalls != 0 {
		t.Fatalf("native Bocha success should not use Python bridge, calls=%d", fallback.stockCalls)
	}
	if len(items) != 1 {
		t.Fatalf("len(items)=%d items=%+v", len(items), items)
	}
	if items[0].Title != "招商银行披露业绩公告 600036" || items[0].Source != "财经媒体" ||
		items[0].PublishedAt != freshDateString(1) || items[0].RetrievedAt == "" || items[0].Relevance <= 0 {
		t.Fatalf("news fields were not mapped: %+v", items[0])
	}
}

func TestSearXNGSearchUsesFreshnessFilterDeduplicatesAndMapsPublisher(t *testing.T) {
	date := freshDateString(0)
	var mu sync.Mutex
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		mu.Unlock()
		if r.URL.Path != "/search" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.URL.Query().Get("format") != "json" || r.URL.Query().Get("time_range") != "day" {
			t.Fatalf("unexpected query parameters: %s", r.URL.RawQuery)
		}
		payload := map[string]any{"results": []map[string]any{
			{"title": "招商银行最新公告", "url": "https://finance.example.com/news/1?utm_source=a", "content": "最新公告摘要", "publishedDate": date},
			{"title": "招商银行最新公告", "url": "https://finance.example.com/news/1?utm_source=b", "content": "重复链接", "publishedDate": date},
			{"title": "陈旧新闻", "url": "https://finance.example.com/old", "content": "旧消息", "publishedDate": freshDateString(20)},
			{"title": "没有发布时间", "url": "https://finance.example.com/unknown", "content": "缺失日期"},
			{"title": "市场新闻", "url": "https://market.example.com/market", "content": "宏观市场消息", "publishedDate": date},
		}}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}))
	defer server.Close()
	searcher := New(Config{SearXNGBaseURLs: []string{server.URL}, MaxAgeDays: 1, StrategyProfile: "ultra_short", RequestTimeout: time.Second}, nil, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 fresh unique dated articles; got %d: %+v", len(items), items)
	}
	if items[0].Title != "招商银行最新公告" || items[0].PublishedAt != date || items[0].Source != "finance.example.com" {
		t.Fatalf("unexpected ranking or mapping: %+v", items[0])
	}
	if requestCount != 1 {
		t.Fatalf("native search requests=%d want 1", requestCount)
	}
}

func TestNativeSearchFallsBackToPythonBridgeWhenResultsAreStaleOrUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"code":200,"data":{"webPages":{"value":[{"name":"旧闻","url":"https://example.com/old","datePublished":%q,"snippet":"过期消息"}]}}}`, freshDateString(10))
	}))
	defer server.Close()
	fallback := &fallbackSearcher{stockItems: []domain.NewsItem{{Title: "Python bridge fresh result", URL: "https://python.example.com/fresh", PublishedAt: freshDateString(0)}}}
	searcher := New(Config{BochaAPIKeys: []string{"test-key"}, BochaEndpoint: server.URL, MaxAgeDays: 3, StrategyProfile: "short", RequestTimeout: time.Second}, fallback, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036", "招商银行", 5)
	if err != nil {
		t.Fatal(err)
	}
	if fallback.stockCalls != 1 || len(items) != 1 || items[0].Title != "Python bridge fresh result" {
		t.Fatalf("fallback was not used after stale native results: calls=%d items=%+v", fallback.stockCalls, items)
	}
}

func TestNoNativeConfigUsesCompatibilityFallbackAndCachesResults(t *testing.T) {
	fallback := &fallbackSearcher{marketItems: []domain.NewsItem{{Title: "市场新闻", URL: "https://market.example.com/news"}}}
	searcher := New(Config{}, fallback, testLogger())
	first, err := searcher.SearchMarketNews(context.Background(), "CN")
	if err != nil {
		t.Fatal(err)
	}
	second, err := searcher.SearchMarketNews(context.Background(), "CN")
	if err != nil {
		t.Fatal(err)
	}
	if fallback.marketCalls != 1 || len(first) != 1 || len(second) != 1 {
		t.Fatalf("expected one cached fallback call: calls=%d first=%d second=%d", fallback.marketCalls, len(first), len(second))
	}
}

func TestQueryWindowAndURLNormalization(t *testing.T) {
	window := New(Config{MaxAgeDays: 30, StrategyProfile: "medium"}, nil, testLogger())
	if window.windowDays != 7 {
		t.Fatalf("effective window=%d want 7", window.windowDays)
	}
	rawURL, err := url.Parse("https://WWW.Example.com/news/1/?utm_source=x&keep=1#fragment")
	if err != nil {
		t.Fatal(err)
	}
	got := canonicalArticleURL(rawURL)
	if strings.Contains(got, "utm_source") || strings.Contains(got, "#fragment") || !strings.Contains(got, "keep=1") {
		t.Fatalf("canonical URL was not normalized: %s", got)
	}
}

func TestNewsQualityFiltersPromotionalContentAndBoostsTrustedSources(t *testing.T) {
	searcher := New(Config{
		TrustedDomains: []string{"licensed.example.cn"},
		BlockedTerms:   []string{"课程推广"},
	}, nil, testLogger())
	published := time.Now().UTC().Format(time.RFC3339)
	raw := []searchCandidate{
		{
			Title:        "证监会发布市场监管公告",
			URL:          "https://www.csrc.gov.cn/news/1",
			Source:       "来源标签不可用于可信度判定",
			PublishedRaw: published,
		},
		{
			Title:        "A股市场综合观察",
			URL:          "https://finance.example.cn/news/2",
			Source:       "普通来源",
			PublishedRaw: published,
		},
		{
			Title:        "免费荐股，加微信领取牛股",
			URL:          "https://licensed.example.cn/ad/1",
			Source:       "营销页面",
			PublishedRaw: published,
		},
		{
			Title:        "股票市场学习资料",
			Snippet:      "课程推广，添加老师了解详情",
			URL:          "https://licensed.example.cn/ad/2",
			Source:       "营销页面",
			PublishedRaw: published,
		},
	}
	items := searcher.filterAndRank(raw, "", "")
	if len(items) != 2 {
		t.Fatalf("expected only two non-promotional articles, got %d: %+v", len(items), items)
	}
	if items[0].item.URL != "https://www.csrc.gov.cn/news/1" {
		t.Fatalf("official regulator domain should rank ahead of an untrusted domain: %+v", items)
	}
}

func TestNewsDomainMatchingUsesLabelBoundaries(t *testing.T) {
	if !matchesNewsDomain("exchange.csrc.gov.cn", []string{"csrc.gov.cn"}) {
		t.Fatal("subdomains of a trusted domain should match")
	}
	if matchesNewsDomain("csrc.gov.cn.evil.example", []string{"csrc.gov.cn"}) {
		t.Fatal("a hostname that only contains a trusted suffix must not match")
	}
	if matchesNewsDomain("notcsrc.gov.cn", []string{"csrc.gov.cn"}) {
		t.Fatal("domain matching must require a DNS label boundary")
	}
	if got := normalizeDomains([]string{"https://WWW.Example.com/path", "www.example.com", "bad.example.com/path"}); len(got) != 1 || got[0] != "www.example.com" {
		t.Fatalf("unexpected normalized domains: %#v", got)
	}
}

func TestNewsSearchRejectsFuturePublicationTimestamp(t *testing.T) {
	searcher := New(Config{}, nil, testLogger())
	future := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)
	items := searcher.filterAndRank([]searchCandidate{{
		Title:        "证监会发布新规",
		URL:          "https://www.csrc.gov.cn/news/future",
		PublishedRaw: future,
	}}, "", "")
	if len(items) != 0 {
		t.Fatalf("future-dated article must be rejected, got %+v", items)
	}
}

type announcementFetcherStub struct {
	snapshot domain.StockAnnouncementSnapshot
	err      error
	calls    int
}

func (f *announcementFetcherStub) FetchStockAnnouncements(_ context.Context, code, startDate, endDate string, page, pageSize int) (domain.StockAnnouncementSnapshot, error) {
	f.calls++
	snapshot := f.snapshot
	snapshot.Symbol, snapshot.StartDate, snapshot.EndDate = code, startDate, endDate
	snapshot.Page, snapshot.PageSize, snapshot.Returned = page, pageSize, len(snapshot.Items)
	return snapshot, f.err
}

func makeAnnouncementSnapshot(title, sourceURL, date string) domain.StockAnnouncementSnapshot {
	now := time.Now().UTC()
	return domain.StockAnnouncementSnapshot{
		HasData: true, Total: 1, Pages: 1, Returned: 1, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: "degraded",
		Items: []domain.StockAnnouncement{{ArticleID: "AN-TEST-1", Symbol: "600036.SH", StockName: "招商银行", Title: title, Category: "定期报告",
			AnnouncementDate: date, SourceURL: sourceURL, Source: "eastmoney", SourceVersion: "test-v1", FetchedAt: now, QualityStatus: "degraded"}},
	}
}

func TestStockAnnouncementsMergeWithWebSearchAndDeduplicateByTitle(t *testing.T) {
	date := freshDateString(1)
	announcementURL := "https://data.eastmoney.com/notices/detail/600036/AN-TEST-1.html"
	title := "招商银行第三季度报告"
	announcementSource := &announcementFetcherStub{snapshot: makeAnnouncementSnapshot(title, announcementURL, date)}
	webSearch := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/search" {
			t.Errorf("unexpected search path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{"title": title, "url": "https://finance.example.com/news/quarter-report", "content": "招商银行最新定期报告", "publishedDate": date}}})
	}))
	defer webSearch.Close()
	fallback := &fallbackSearcher{}
	searcher := New(Config{SearXNGBaseURLs: []string{webSearch.URL}, MaxAgeDays: 3, StrategyProfile: "short", RequestTimeout: time.Second, StockAnnouncements: announcementSource}, fallback, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 5)
	if err != nil {
		t.Fatal(err)
	}
	if announcementSource.calls != 1 {
		t.Fatalf("announcement calls=%d, want 1", announcementSource.calls)
	}
	if len(items) != 1 {
		t.Fatalf("same-title announcement and web result should be deduplicated: %+v", items)
	}
	if items[0].URL != announcementURL || items[0].PublishedAt != date || items[0].Source != "eastmoney" {
		t.Fatalf("disclosure should take precedence without invented timestamp: %+v", items[0])
	}
	if fallback.stockCalls != 0 {
		t.Fatalf("native result should not invoke fallback, calls=%d", fallback.stockCalls)
	}
}

func TestStockAnnouncementsWorkWithoutWebSearchProviders(t *testing.T) {
	date := freshDateString(1)
	source := &announcementFetcherStub{snapshot: makeAnnouncementSnapshot("招商银行第三季度报告", "https://data.eastmoney.com/notices/detail/600036/AN-TEST-1.html", date)}
	fallback := &fallbackSearcher{}
	searcher := New(Config{StockAnnouncements: source, RequestTimeout: time.Second}, fallback, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].PublishedAt != date {
		t.Fatalf("announcements should be a standalone native source: %+v", items)
	}
	if fallback.stockCalls != 0 {
		t.Fatalf("announcement-only search should not fallback, calls=%d", fallback.stockCalls)
	}
}

func TestTavilyProviderMapsSourcePublicationTime(t *testing.T) {
	published := freshDateString(1) + "T09:30:00+08:00"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer tavily-test-key" {
			t.Errorf("unexpected Tavily request: method=%s", r.Method)
		}
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["topic"] != "news" || request["query"] != "招商银行 600036.SH 股票 最新消息" || request["include_published_date"] != true {
			t.Errorf("unexpected Tavily payload: %#v", request)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{
			"title": "招商银行披露业绩公告", "url": "https://finance.example.com/tavily/1",
			"content": "业绩更新", "published_date": published,
		}}})
	}))
	defer server.Close()
	searcher := New(Config{TavilyAPIKeys: []string{"tavily-test-key"}, TavilyEndpoint: server.URL}, nil, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].PublishedAt != freshDateString(1) {
		t.Fatalf("Tavily result/date not mapped: %+v", items)
	}
}

func TestBraveNewsProviderUsesChinaLanguageAndSourceDate(t *testing.T) {
	published := freshDateString(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Subscription-Token") != "brave-test-key" || r.URL.Query().Get("country") != "CN" ||
			r.URL.Query().Get("search_lang") != "zh-hans" {
			t.Errorf("unexpected Brave News request: headers=%v query=%s", r.Header, r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"title": "招商银行业绩公告", "url": "https://finance.example.com/brave/1", "description": "新闻摘要", "page_age": published, "page_fetched": time.Now().UTC().Format(time.RFC3339)},
			{"title": "只有抓取时间的结果", "url": "https://finance.example.com/brave/2", "description": "不能把 page_fetched 当发布时间", "page_fetched": time.Now().UTC().Format(time.RFC3339)},
		}})
	}))
	defer server.Close()
	searcher := New(Config{BraveSearchAPIKeys: []string{"brave-test-key"}, BraveNewsEndpoint: server.URL}, nil, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "招商银行业绩公告" {
		t.Fatalf("Brave must reject results with only fetch timestamps: %+v", items)
	}
}

func TestNativeNewsSearchAggregatesConfiguredProvidersAndDeduplicates(t *testing.T) {
	date := freshDateString(1)
	var mu sync.Mutex
	tavilyCalls, braveCalls := 0, 0
	tavilyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		tavilyCalls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"title": "招商银行业绩公告 600036", "url": "https://finance.example.com/tavily/shared", "content": "来自 Tavily", "published_date": date},
			{"title": "招商银行回购公告", "url": "https://finance.example.com/tavily/buyback", "content": "来自 Tavily 的回购内容", "published_date": date},
		}})
	}))
	defer tavilyServer.Close()
	braveServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		braveCalls++
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{
			{"title": "招商银行业绩公告 600036", "url": "https://finance.example.com/brave/shared", "description": "重复标题", "page_age": date},
			{"title": "招商银行分红公告", "url": "https://finance.example.com/brave/dividend", "description": "来自 Brave 的分红内容", "page_age": date},
		}})
	}))
	defer braveServer.Close()
	searcher := New(Config{
		TavilyAPIKeys: []string{"tavily-test-key"}, TavilyEndpoint: tavilyServer.URL,
		BraveSearchAPIKeys: []string{"brave-test-key"}, BraveNewsEndpoint: braveServer.URL,
		MaxAgeDays: 3, StrategyProfile: "short", RequestTimeout: time.Second,
	}, nil, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 10)
	if err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	if tavilyCalls != 1 || braveCalls != 1 {
		t.Fatalf("configured providers must each be called once: tavily=%d brave=%d", tavilyCalls, braveCalls)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3 unique results after aggregation: %+v", items)
	}
}


func TestStockSearchUsesFiveNewsDimensionsAndPreservesLabels(t *testing.T) {
	date := freshDateString(1)
	var mu sync.Mutex
	queries := make(map[string]struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query().Get("q")
		mu.Lock()
		queries[query] = struct{}{}
		mu.Unlock()
		articleURL := "https://news.example.com/search?q=" + url.QueryEscape(query)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"results": []map[string]any{{
			"title": query, "url": articleURL, "content": query, "publishedDate": date,
		}}})
	}))
	defer server.Close()
	searcher := New(Config{
		SearXNGBaseURLs: []string{server.URL}, MaxAgeDays: 3, MaxSearches: 5,
		StrategyProfile: "short", RequestTimeout: time.Second,
	}, nil, testLogger())
	items, err := searcher.SearchStockNews(context.Background(), "600036.SH", "招商银行", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 5 {
		t.Fatalf("expected five distinct query-dimension results, got %d: %+v", len(items), items)
	}
	want := map[string]bool{"general": false, "financial": false, "regulatory": false, "business": false, "ownership": false}
	for _, item := range items {
		if _, ok := want[item.Dimension]; !ok {
			t.Errorf("unexpected news dimension %q: %+v", item.Dimension, item)
			continue
		}
		want[item.Dimension] = true
	}
	for dimension, found := range want {
		if !found {
			t.Errorf("missing query dimension %q", dimension)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(queries) != 5 {
		t.Fatalf("expected 5 query variants, got %d: %#v", len(queries), queries)
	}
}
