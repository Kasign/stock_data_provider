package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"example.com/stock-analysis-flow/internal/news/app"
	"example.com/stock-analysis-flow/internal/news/domain"
)

type config struct {
	endpoint string
	apiKey   string
	client   app.HTTPDoer
}

func newConfig(endpoint, apiKey string, client app.HTTPDoer) (config, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") || hasCredentialQuery(parsed.Query()) {
		return config{}, app.ErrProviderConfig
	}
	if client == nil {
		return config{}, app.ErrProviderConfig
	}
	return config{endpoint: endpoint, apiKey: strings.TrimSpace(apiKey), client: client}, nil
}

func hasCredentialQuery(query url.Values) bool {
	for key := range query {
		normalized := strings.NewReplacer("_", "", "-", "").Replace(strings.ToLower(key))
		switch normalized {
		case "apikey", "accesskey", "accesstoken", "token", "authorization", "subscriptiontoken", "clientsecret":
			return true
		}
	}
	return false
}

func (c config) do(ctx context.Context, method, target string, headers map[string]string, payload any) ([]byte, error) {
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return nil, &app.ProviderError{Code: app.ProviderErrorRequestInvalid}
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, &app.ProviderError{Code: app.ProviderErrorRequestInvalid}
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.client.Do(req)
	if err != nil {
		if isProviderTimeout(err) {
			return nil, &app.ProviderError{Code: app.ProviderErrorTimeout, Retryable: true}
		}
		retryable := isRetryableProviderTransport(err)
		return nil, &app.ProviderError{Code: app.ProviderErrorTransportFailed, Retryable: retryable}
	}
	if resp == nil || resp.Body == nil {
		return nil, &app.ProviderError{Code: app.ProviderErrorResponseInvalid}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		if isProviderTimeout(err) {
			return nil, &app.ProviderError{Code: app.ProviderErrorTimeout, Retryable: true}
		}
		return nil, &app.ProviderError{Code: app.ProviderErrorTransportFailed, Retryable: isRetryableProviderTransport(err)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			return nil, &app.ProviderError{Code: app.ProviderErrorUnauthorized}
		case http.StatusRequestTimeout:
			return nil, &app.ProviderError{Code: app.ProviderErrorTimeout, Retryable: true}
		case http.StatusTooManyRequests:
			return nil, &app.ProviderError{Code: app.ProviderErrorRateLimited, Retryable: true}
		default:
			if resp.StatusCode >= 500 {
				return nil, &app.ProviderError{Code: app.ProviderErrorUpstreamUnavailable, Retryable: true}
			}
			return nil, &app.ProviderError{Code: app.ProviderErrorBusinessRejected}
		}
	}
	return raw, nil
}

func candidate(providerCode, itemID, title, summary, rawURL, publisherName, publisherDomain, published string, observed time.Time, dimension string) domain.Candidate {
	item := domain.Candidate{
		ProviderCode:    providerCode,
		ProviderItemID:  strings.TrimSpace(itemID),
		Title:           strings.TrimSpace(title),
		ProviderSummary: strings.TrimSpace(summary),
		RawURL:          strings.TrimSpace(rawURL),
		PublisherName:   strings.TrimSpace(publisherName),
		PublisherDomain: strings.ToLower(strings.TrimSpace(publisherDomain)),
		PublishedAtRaw:  strings.TrimSpace(published),
		ObservedAt:      observed,
		QueryDimension:  dimension,
	}
	if item.Title == "" {
		item.MissingFields = append(item.MissingFields, "title")
	}
	if item.RawURL == "" {
		item.MissingFields = append(item.MissingFields, "url")
	}
	if item.PublishedAtRaw == "" {
		item.MissingFields = append(item.MissingFields, "published_at")
	} else if parsed, err := time.Parse(time.RFC3339, item.PublishedAtRaw); err == nil {
		item.PublishedAt = &parsed
	}
	if item.PublisherName == "" {
		item.MissingFields = append(item.MissingFields, "publisher")
	}
	return item
}

func limitOrDefault(limit int) int {
	if limit < 1 {
		return 10
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func queryURL(endpoint, query string, limit int) (string, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("format", "json")
	q.Set("count", strconv.Itoa(limit))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// isProviderTimeout classifies transport and context deadline failures without treating
// ordinary caller cancellation as a provider timeout.
func isProviderTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr) && networkErr.Timeout()
}

func isRetryableProviderTransport(err error) bool {
	var networkErr net.Error
	return errors.As(err, &networkErr) && networkErr.Temporary()
}
