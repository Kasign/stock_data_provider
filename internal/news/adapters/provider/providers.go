package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

const (
	BochaCode   = "bocha"
	TavilyCode  = "tavily"
	BraveCode   = "brave_news"
	SearXNGCode = "searxng"
)

type Bocha struct{ cfg config }

func NewBocha(endpoint, apiKey string, client app.HTTPDoer) (*Bocha, error) {
	cfg, err := newConfig(endpoint, apiKey, client)
	if err != nil {
		return nil, err
	}
	if cfg.apiKey == "" {
		return nil, app.ErrProviderConfig
	}
	return &Bocha{cfg: cfg}, nil
}

func (p *Bocha) Code() string { return BochaCode }

func (p *Bocha) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Candidate, error) {
	raw, err := p.cfg.do(ctx, http.MethodPost, p.cfg.endpoint,
		map[string]string{"Authorization": "Bearer " + p.cfg.apiKey, "Content-Type": "application/json"},
		map[string]any{"query": req.Query, "summary": true, "count": limitOrDefault(req.Limit)})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Code json.RawMessage `json:"code"`
		Data struct {
			WebPages struct {
				Value []struct {
					ID            string          `json:"id"`
					Name          string          `json:"name"`
					URL           string          `json:"url"`
					SiteName      string          `json:"siteName"`
					SiteDomain    string          `json:"siteDomain"`
					DatePublished json.RawMessage `json:"datePublished"`
					Summary       string          `json:"summary"`
					Snippet       string          `json:"snippet"`
				} `json:"value"`
			} `json:"webPages"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &app.ProviderError{Code: "provider_response_invalid"}
	}
	code := strings.Trim(strings.TrimSpace(string(payload.Code)), "\"")
	if code != "200" {
		return nil, &app.ProviderError{Code: "provider_business_rejected"}
	}

	now := time.Now().UTC()
	out := make([]domain.Candidate, 0, len(payload.Data.WebPages.Value))
	for _, item := range payload.Data.WebPages.Value {
		summary := item.Summary
		if summary == "" {
			summary = item.Snippet
		}
		out = append(out, candidate(BochaCode, item.ID, item.Name, summary, item.URL, item.SiteName, item.SiteDomain,
			rawJSONText(item.DatePublished), now, req.QueryDimension))
	}
	return out, nil
}

type Tavily struct{ cfg config }

func NewTavily(endpoint, apiKey string, client app.HTTPDoer) (*Tavily, error) {
	cfg, err := newConfig(endpoint, apiKey, client)
	if err != nil {
		return nil, err
	}
	if cfg.apiKey == "" {
		return nil, app.ErrProviderConfig
	}
	return &Tavily{cfg: cfg}, nil
}

func (p *Tavily) Code() string { return TavilyCode }

func (p *Tavily) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Candidate, error) {
	raw, err := p.cfg.do(ctx, http.MethodPost, p.cfg.endpoint,
		map[string]string{"Authorization": "Bearer " + p.cfg.apiKey, "Content-Type": "application/json", "Accept": "application/json"},
		map[string]any{"query": req.Query, "topic": "news", "search_depth": "basic", "max_results": limitOrDefault(req.Limit), "include_published_date": true, "include_answer": false, "include_raw_content": false, "include_images": false})
	if err != nil {
		return nil, err
	}

	var payload struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Content       string `json:"content"`
			PublishedDate string `json:"published_date"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &app.ProviderError{Code: "provider_response_invalid"}
	}

	now := time.Now().UTC()
	out := make([]domain.Candidate, 0, len(payload.Results))
	for _, item := range payload.Results {
		out = append(out, candidate(TavilyCode, "", item.Title, item.Content, item.URL, "", "", item.PublishedDate, now, req.QueryDimension))
	}
	return out, nil
}

type Brave struct{ cfg config }

func NewBrave(endpoint, apiKey string, client app.HTTPDoer) (*Brave, error) {
	cfg, err := newConfig(endpoint, apiKey, client)
	if err != nil {
		return nil, err
	}
	if cfg.apiKey == "" {
		return nil, app.ErrProviderConfig
	}
	return &Brave{cfg: cfg}, nil
}

func (p *Brave) Code() string { return BraveCode }

func (p *Brave) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Candidate, error) {
	u, err := url.Parse(p.cfg.endpoint)
	if err != nil {
		return nil, app.ErrProviderConfig
	}
	q := u.Query()
	q.Set("q", req.Query)
	q.Set("count", strconv.Itoa(limitOrDefault(req.Limit)))
	q.Set("country", "CN")
	q.Set("search_lang", "zh-hans")
	q.Set("ui_lang", "zh-CN")
	q.Set("spellcheck", "false")
	q.Set("safesearch", "strict")
	u.RawQuery = q.Encode()

	raw, err := p.cfg.do(ctx, http.MethodGet, u.String(),
		map[string]string{"X-Subscription-Token": p.cfg.apiKey, "Accept": "application/json", "User-Agent": "stock-analysis-go/1.0"}, nil)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Results []struct {
			Title         string `json:"title"`
			URL           string `json:"url"`
			Description   string `json:"description"`
			PublishedDate string `json:"published_date"`
			PageAge       string `json:"page_age"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &app.ProviderError{Code: "provider_response_invalid"}
	}

	now := time.Now().UTC()
	out := make([]domain.Candidate, 0, len(payload.Results))
	for _, item := range payload.Results {
		// Brave page_age 是页面年龄/更新时间信息，不是新闻发布时间。
		out = append(out, candidate(BraveCode, "", item.Title, item.Description, item.URL, "", "", item.PublishedDate, now, req.QueryDimension))
	}
	return out, nil
}

type SearXNG struct{ cfg config }

func NewSearXNG(baseURL string, client app.HTTPDoer) (*SearXNG, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if !strings.HasSuffix(strings.ToLower(baseURL), "/search") {
		baseURL += "/search"
	}
	cfg, err := newConfig(baseURL, "", client)
	if err != nil {
		return nil, err
	}
	return &SearXNG{cfg: cfg}, nil
}

func (p *SearXNG) Code() string { return SearXNGCode }

func (p *SearXNG) Search(ctx context.Context, req domain.SearchRequest) ([]domain.Candidate, error) {
	target, err := queryURL(p.cfg.endpoint, req.Query, limitOrDefault(req.Limit))
	if err != nil {
		return nil, app.ErrProviderConfig
	}
	raw, err := p.cfg.do(ctx, http.MethodGet, target, map[string]string{"Accept": "application/json", "User-Agent": "stock-analysis-go/1.0"}, nil)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Results []struct {
			Title         string          `json:"title"`
			URL           string          `json:"url"`
			Content       string          `json:"content"`
			Description   string          `json:"description"`
			PublishedDate json.RawMessage `json:"publishedDate"`
			PublishedSnake json.RawMessage `json:"published_date"`
			PublishedAt   json.RawMessage `json:"published_at"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, &app.ProviderError{Code: "provider_response_invalid"}
	}

	now := time.Now().UTC()
	out := make([]domain.Candidate, 0, len(payload.Results))
	for _, item := range payload.Results {
		summary := item.Content
		if summary == "" {
			summary = item.Description
		}
		published := rawJSONText(item.PublishedDate)
		if published == "" {
			published = rawJSONText(item.PublishedSnake)
		}
		if published == "" {
			published = rawJSONText(item.PublishedAt)
		}
		out = append(out, candidate(SearXNGCode, "", item.Title, summary, item.URL, "", "", published, now, req.QueryDimension))
	}
	return out, nil
}

func rawJSONText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return strings.TrimSpace(string(raw))
}
