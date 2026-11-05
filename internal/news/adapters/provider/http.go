package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

const maxProviderResponseBytes = 4 << 20

type config struct {
	endpoint string
	apiKey   string
	client   app.HTTPDoer
}

func newConfig(endpoint, apiKey string, client app.HTTPDoer) (config, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return config{}, app.ErrProviderConfig
	}
	if client == nil {
		return config{}, app.ErrProviderConfig
	}
	return config{endpoint: endpoint, apiKey: strings.TrimSpace(apiKey), client: client}, nil
}

func (c config) do(ctx context.Context, method string, target string, headers map[string]string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil { return nil, fmt.Errorf("encode provider request: %w", err) }
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil { return nil, fmt.Errorf("build provider request: %w", err) }
	for key, value := range headers { req.Header.Set(key, value) }
	resp, err := c.client.Do(req)
	if err != nil { return nil, fmt.Errorf("provider transport: %w", err) }
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, maxProviderResponseBytes+1)
	raw, err := io.ReadAll(limited)
	if err != nil { return nil, fmt.Errorf("read provider response: %w", err) }
	if len(raw) > maxProviderResponseBytes { return nil, fmt.Errorf("provider response exceeds size limit") }
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("provider http status %d", resp.StatusCode)
	}
	return raw, nil
}

func candidate(provider, id, title, summary, rawURL, publisher, published string, observed time.Time, dimension string) domain.Candidate {
	item := domain.Candidate{
		ProviderCode: provider, ProviderItemID: strings.TrimSpace(id),
		Title: strings.TrimSpace(title), Summary: strings.TrimSpace(summary),
		RawURL: strings.TrimSpace(rawURL), PublisherName: strings.TrimSpace(publisher),
		PublishedAtRaw: strings.TrimSpace(published), ObservedAt: observed,
		QueryDimension: dimension,
	}
	if item.Title == "" { item.MissingFields = append(item.MissingFields, "title") }
	if item.RawURL == "" { item.MissingFields = append(item.MissingFields, "url") }
	if item.PublishedAtRaw == "" { item.MissingFields = append(item.MissingFields, "published_at") }
	if item.PublisherName == "" { item.MissingFields = append(item.MissingFields, "publisher") }
	if parsed, err := time.Parse(time.RFC3339, item.PublishedAtRaw); err == nil {
		item.PublishedAt = &parsed
	}
	if parsed, err := url.Parse(item.RawURL); err == nil && parsed.Host != "" {
		item.PublisherDomain = strings.ToLower(parsed.Hostname())
	}
	return item
}

func limitOrDefault(limit int) int {
	if limit < 1 { return 10 }
	if limit > 100 { return 100 }
	return limit
}

func queryURL(endpoint, query string, limit int) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil { return "", err }
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("count", strconv.Itoa(limit))
	u.RawQuery = q.Encode()
	return u.String(), nil
}
