package provider

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) Do(r *http.Request) (*http.Response, error) {
	return f(r)
}

func response(body string) *http.Response {
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
}

func TestBochaPreservesUnknownPublicationTimeAndPublisherIdentity(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodPost {
			t.Fatalf("method=%s", req.Method)
		}
		if req.Header.Get("Authorization") != "Bearer test-secret" {
			t.Fatal("missing auth header")
		}
		return response(`{"code":200,"data":{"webPages":{"value":[{"name":"标题","url":"https://article.example/a","siteName":"媒体","summary":"摘要"}]}}}`), nil
	})
	p, err := NewBocha("https://api.example.test/v1/web-search", "test-secret", client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("items=%d", len(items))
	}
	got := items[0]
	if got.ProviderCode != BochaCode || got.PublisherName != "媒体" || got.PublishedAtRaw != "" || got.PublishedAt != nil {
		t.Fatalf("candidate=%+v", got)
	}
	if got.PublisherDomain != "" {
		t.Fatalf("publisher domain must not be inferred from result URL: %q", got.PublisherDomain)
	}
}

func TestBraveDoesNotTreatPageAgeAsPublicationTime(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("X-Subscription-Token") != "secret" {
			t.Fatal("missing brave token")
		}
		return response(`{"results":[{"title":"标题","url":"https://news.example/a","description":"摘要","page_age":"2026-11-05T01:00:00Z"}]}`), nil
	})
	p, err := NewBrave("https://api.example.test/news", "secret", client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if items[0].PublishedAtRaw != "" {
		t.Fatalf("page_age must not be publication time: %q", items[0].PublishedAtRaw)
	}
}

func TestTavilyUsesBearerHeaderAndPreservesPublishedDate(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Header.Get("Authorization") != "Bearer tavily-secret" {
			t.Fatal("missing Tavily bearer token")
		}
		if strings.Contains(req.URL.RawQuery, "api_key") {
			t.Fatal("credential must not be placed in URL")
		}
		return response(`{"results":[{"title":"标题","url":"https://publisher.example/a","content":"摘要","published_date":"2026-11-04"}]}`), nil
	})
	p, err := NewTavily("https://api.example.test/search", "tavily-secret", client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].PublishedAtRaw != "2026-11-04" || items[0].PublishedAt != nil {
		t.Fatalf("items=%+v", items)
	}
}

func TestSearXNGPreservesProviderAndParsesExplicitTimestamp(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/search" || req.URL.Query().Get("q") != "市场消息" || req.URL.Query().Get("format") != "json" || req.URL.Query().Get("language") != "zh-CN" {
			t.Fatalf("request URL=%s", req.URL.String())
		}
		return response(`{"results":[{"title":"标题","url":"https://publisher.example/a","content":"摘要","publishedDate":"2026-11-04T08:30:00+08:00"}]}`), nil
	})
	p, err := NewSearXNG("https://searx.example/search?language=zh-CN", client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.Search(context.Background(), domain.SearchRequest{Query: "市场消息", Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ProviderCode != SearXNGCode || items[0].PublishedAt == nil {
		t.Fatalf("items=%+v", items)
	}
}

func TestProviderRejectsCredentialURLAndInvalidEndpoint(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response("{}"), nil
	})
	if _, err := NewBocha("https://user:pass@example.com", "x", client); err == nil {
		t.Fatal("expected credential URL rejection")
	}
	if _, err := NewSearXNG("file:///tmp", client); err == nil {
		t.Fatal("expected scheme rejection")
	}
}

func TestProviderHTTPStatusClassifiesRetryCandidate(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusTooManyRequests, Body: io.NopCloser(strings.NewReader("quota")), Header: make(http.Header)}, nil
	})
	p, err := NewTavily("https://api.example.test/search", "secret", client)
	if err != nil {
		t.Fatal(err)
	}
	_, err = p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 5})
	providerErr, ok := err.(*app.ProviderError)
	if !ok || !providerErr.Retryable || providerErr.Code != app.ProviderErrorRateLimited {
		t.Fatalf("error=%#v", err)
	}
}

func TestTavilyRejectsMissingResultsField(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(`{}`), nil
	})
	p, err := NewTavily("https://api.example.test/search", "secret", client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 5})
	if err == nil || items != nil {
		t.Fatalf("missing results field must be rejected: items=%+v err=%v", items, err)
	}
}

func TestBochaRejectsMissingResultEnvelope(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(`{"code":200,"data":{}}`), nil
	})
	p, err := NewBocha("https://api.example.test/v1/web-search", "secret", client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 5})
	if err == nil || items != nil {
		t.Fatalf("missing result envelope must be rejected: items=%+v err=%v", items, err)
	}
}

func TestSearXNGRejectsMissingResultsField(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(`{}`), nil
	})
	p, err := NewSearXNG("https://searx.example", client)
	if err != nil {
		t.Fatal(err)
	}
	items, err := p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 5})
	if err == nil || items != nil {
		t.Fatalf("missing results field must be rejected: items=%+v err=%v", items, err)
	}
}

func TestProviderRejectsCredentialQueryParameters(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(`{}`), nil
	})
	if _, err := NewTavily("https://api.example.test/search?api_key=secret", "secret", client); err == nil {
		t.Fatal("credential query parameters must be rejected")
	}
}

func TestProviderHTTPStatusMapsStableErrorCodes(t *testing.T) {
	tests := []struct {
		status    int
		code      string
		retryable bool
	}{
		{status: http.StatusUnauthorized, code: app.ProviderErrorUnauthorized},
		{status: http.StatusForbidden, code: app.ProviderErrorUnauthorized},
		{status: http.StatusRequestTimeout, code: app.ProviderErrorTimeout, retryable: true},
		{status: http.StatusTooManyRequests, code: app.ProviderErrorRateLimited, retryable: true},
		{status: http.StatusBadRequest, code: app.ProviderErrorBusinessRejected},
		{status: http.StatusServiceUnavailable, code: app.ProviderErrorUpstreamUnavailable, retryable: true},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: test.status, Body: io.NopCloser(strings.NewReader("upstream body")), Header: make(http.Header)}, nil
			})
			p, err := NewTavily("https://api.example.test/search", "secret", client)
			if err != nil {
				t.Fatal(err)
			}
			_, err = p.Search(context.Background(), domain.SearchRequest{Query: "测试", Limit: 5})
			providerErr, ok := err.(*app.ProviderError)
			if !ok || providerErr.Code != test.code || providerErr.Retryable != test.retryable {
				t.Fatalf("error=%#v, want code=%q retryable=%v", err, test.code, test.retryable)
			}
		})
	}
}

func TestSearchProvidersSatisfyCommonCandidateContract(t *testing.T) {
	tests := []struct {
		name    string
		new     func(app.HTTPDoer) (app.Provider, error)
		payload string
	}{
		{
			name: "bocha",
			new: func(client app.HTTPDoer) (app.Provider, error) {
				return NewBocha("https://api.example.test/search", "secret", client)
			},
			payload: `{"code":200,"data":{"webPages":{"value":[{"id":"b-1","name":"标题","url":"https://publisher.example/b","siteName":"媒体"}]}}}`,
		},
		{
			name: "tavily",
			new: func(client app.HTTPDoer) (app.Provider, error) {
				return NewTavily("https://api.example.test/search", "secret", client)
			},
			payload: `{"results":[{"title":"标题","url":"https://publisher.example/t","content":"摘要"}]}`,
		},
		{
			name: "brave",
			new: func(client app.HTTPDoer) (app.Provider, error) {
				return NewBrave("https://api.example.test/search", "secret", client)
			},
			payload: `{"results":[{"title":"标题","url":"https://publisher.example/br","description":"摘要"}]}`,
		},
		{
			name: "searxng",
			new: func(client app.HTTPDoer) (app.Provider, error) {
				return NewSearXNG("https://searx.example/search", client)
			},
			payload: `{"results":[{"title":"标题","url":"https://publisher.example/s","content":"摘要"}]}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return response(test.payload), nil
			})
			provider, err := test.new(client)
			if err != nil {
				t.Fatal(err)
			}
			code := provider.Code()
			if code == "" || strings.TrimSpace(code) != code {
				t.Fatalf("Provider Code is not canonical: %q", code)
			}
			items, err := provider.Search(context.Background(), domain.SearchRequest{
				Query: "市场消息", QueryDimension: "market", Limit: 10,
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 1 {
				t.Fatalf("candidate count=%d, want 1", len(items))
			}
			if items[0].ProviderCode != code {
				t.Fatalf("candidate ProviderCode=%q, want %q", items[0].ProviderCode, code)
			}
			if items[0].QueryDimension != "market" || items[0].ObservedAt.IsZero() {
				t.Fatalf("candidate context was not preserved: %+v", items[0])
			}
		})
	}
}

func TestBochaSearchThroughApplicationContract(t *testing.T) {
	client := roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(`{"code":200,"data":{"webPages":{"value":[{"id":"bocha-1","name":"标题","url":"https://publisher.example/a","siteName":"媒体","summary":"摘要"}]}}}`), nil
	})
	provider, err := NewBocha("https://api.example.test/search", "secret", client)
	if err != nil {
		t.Fatal(err)
	}
	result := app.NewSearchCandidates(app.StaticProviderSet{Providers: []app.Provider{provider}}).
		Execute(context.Background(), domain.SearchRequest{
			Query: "市场消息", QueryDimension: " market ", Limit: 10,
		})
	if result.Status != domain.SearchSuccess {
		t.Fatalf("search status=%s failure=%s", result.Status, result.FailureCode)
	}
	if len(result.Outcomes) != 1 || result.Outcomes[0].ProviderCode != BochaCode ||
		result.Outcomes[0].Status != domain.ProviderSuccess || result.Outcomes[0].CandidateCount != 1 ||
		result.Outcomes[0].QueryDimension != "market" {
		t.Fatalf("provider outcome=%+v", result.Outcomes)
	}
	if len(result.Candidates) != 1 {
		t.Fatalf("candidate count=%d", len(result.Candidates))
	}
	candidate := result.Candidates[0]
	if candidate.ProviderCode != BochaCode || candidate.QueryDimension != "market" ||
		candidate.Title != "标题" || candidate.ObservedAt.IsZero() {
		t.Fatalf("normalized candidate=%+v", candidate)
	}
}
