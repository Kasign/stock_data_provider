package pythonapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is a typed, bounded HTTP client for daily_stock_analysis' existing API.
type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	bearerToken      string
	cookie           string
	maxResponseBytes int64
	marketLocation   *time.Location
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 20 * time.Second
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = 8 << 20
	}
	if cfg.MaxResponseBytes < 1024 || cfg.MaxResponseBytes > 128<<20 {
		return nil, errors.New("MaxResponseBytes must be between 1024 and 134217728")
	}
	if strings.TrimSpace(cfg.MarketTimezone) == "" {
		cfg.MarketTimezone = "Asia/Shanghai"
	}
	marketLocation, err := time.LoadLocation(cfg.MarketTimezone)
	if err != nil {
		return nil, fmt.Errorf("load market timezone: %w", err)
	}
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("PYTHON_API_BASE_URL must be an absolute http/https URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("PYTHON_API_BASE_URL must not contain credentials, query, or fragment")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return &Client{
		baseURL:          u,
		httpClient:       &http.Client{Timeout: cfg.Timeout},
		bearerToken:      strings.TrimSpace(cfg.BearerToken),
		cookie:           strings.TrimSpace(cfg.Cookie),
		maxResponseBytes: cfg.MaxResponseBytes,
		marketLocation:   marketLocation,
	}, nil
}

func (c *Client) GetQuote(ctx context.Context, symbol string) (QuoteResponse, time.Time, error) {
	if err := validateSymbol(symbol); err != nil {
		return QuoteResponse{}, time.Time{}, err
	}
	var result QuoteResponse
	path := quotePath + url.PathEscape(strings.TrimSpace(symbol)) + "/quote"
	fetchedAt, err := c.getJSON(ctx, path, &result)
	if err != nil {
		return QuoteResponse{}, time.Time{}, err
	}
	if strings.TrimSpace(result.StockCode) == "" {
		result.StockCode = strings.TrimSpace(symbol)
	}
	if !isFinite(result.CurrentPrice) || result.CurrentPrice <= 0 {
		return QuoteResponse{}, fetchedAt, errors.New("python API quote has missing, non-finite, or non-positive current_price")
	}
	if !finiteOptional(result.Change) || !finiteOptional(result.ChangePct) || !finiteOptional(result.Open) ||
		!finiteOptional(result.High) || !finiteOptional(result.Low) || !finiteOptional(result.PrevClose) ||
		!finiteOptional(result.Volume) || !finiteOptional(result.Amount) {
		return QuoteResponse{}, fetchedAt, errors.New("python API quote contains a non-finite optional numeric field")
	}
	return result, fetchedAt, nil
}

func (c *Client) GetHistory(ctx context.Context, symbol string, days int) (HistoryResult, error) {
	if err := validateSymbol(symbol); err != nil {
		return HistoryResult{}, err
	}
	if days < 1 || days > 365 {
		return HistoryResult{}, errors.New("history days must be between 1 and 365")
	}
	path := quotePath + url.PathEscape(strings.TrimSpace(symbol)) + "/history"
	query := url.Values{}
	query.Set("period", "daily")
	query.Set("days", itoa(days))
	var payload apiHistory
	fetchedAt, err := c.getJSON(ctx, path+"?"+query.Encode(), &payload)
	if err != nil {
		return HistoryResult{}, err
	}
	if len(payload.Data) == 0 {
		return HistoryResult{}, errors.New("python API returned no daily bars")
	}
	return HistoryResult{
		Symbol: strings.TrimSpace(payload.StockCode), Name: payload.StockName,
		Bars: payload.Data, FetchedAt: fetchedAt, Dataset: historyDataset,
	}, nil
}

// GetProfile returns the Python project's aggregated profile payload. Blocks are
// retained as JSON because optional blocks can evolve independently upstream.
func (c *Client) GetProfile(ctx context.Context, symbol string, historyDays int) (ProfileResponse, error) {
	if err := validateSymbol(symbol); err != nil {
		return ProfileResponse{}, err
	}
	if historyDays < 1 || historyDays > 365 {
		return ProfileResponse{}, errors.New("history days must be between 1 and 365")
	}
	path := quotePath + url.PathEscape(strings.TrimSpace(symbol)) + "/profile?" + url.Values{"history_days": []string{itoa(historyDays)}}.Encode()
	var result ProfileResponse
	_, err := c.getJSON(ctx, path, &result)
	if err != nil {
		return ProfileResponse{}, err
	}
	return result, nil
}

// GetDataOverview queries the read-only provider capability/quality diagnostic.
// It does not trigger upstream market-data requests.
func (c *Client) GetDataOverview(ctx context.Context) (json.RawMessage, error) {
	var result json.RawMessage
	_, err := c.getJSON(ctx, "/api/v1/data/overview", &result)
	if err != nil {
		return nil, err
	}
	if len(result) == 0 || !json.Valid(result) {
		return nil, errors.New("python API data overview returned invalid JSON")
	}
	return result, nil
}

func (c *Client) getJSON(ctx context.Context, requestPath string, output any) (time.Time, error) {
	if ctx == nil {
		return time.Time{}, errors.New("context must not be nil")
	}
	joined := *c.baseURL
	pathOnly, rawQuery, hasQuery := strings.Cut(requestPath, "?")
	joined.Path = strings.TrimRight(joined.Path, "/") + "/" + strings.TrimLeft(pathOnly, "/")
	if hasQuery {
		joined.RawQuery = rawQuery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joined.String(), nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("build python API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "stock-analysis-go-adapter/1.0")
	if c.bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.bearerToken)
	}
	if c.cookie != "" {
		req.Header.Set("Cookie", c.cookie)
	}
	resp, err := c.httpClient.Do(req)
	fetchedAt := time.Now().UTC()
	if err != nil {
		return fetchedAt, fmt.Errorf("request python API %s: %w", joined.Redacted(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Drain only a small amount without exposing the response body in logs.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fetchedAt, &HTTPStatusError{Method: req.Method, Path: requestPath, Status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return fetchedAt, fmt.Errorf("read python API response: %w", err)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return fetchedAt, fmt.Errorf("python API response exceeds configured limit of %d bytes", c.maxResponseBytes)
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fetchedAt, fmt.Errorf("decode python API JSON response: %w", err)
	}
	return time.Now().UTC(), nil
}

func parseTimestamp(value string, location *time.Location) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	// The upstream quote API often returns a naive local-market timestamp.
	// Parse explicit offsets first; otherwise interpret as the configured market
	// timezone (A-share default: Asia/Shanghai), never as host-local/UTC by accident.
	normalized := value
	if !strings.Contains(normalized, "T") && strings.Contains(normalized, " ") {
		normalized = strings.Replace(normalized, " ", "T", 1)
	}
	if strings.HasSuffix(strings.ToUpper(normalized), "Z") || hasExplicitUTCOffset(normalized) {
		if parsed, err := time.Parse(time.RFC3339Nano, normalized); err == nil {
			return &parsed, nil
		}
	}
	if location != nil {
		for _, layout := range []string{"2006-01-02T15:04:05.999999999", "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02"} {
			if parsed, err := time.ParseInLocation(layout, normalized, location); err == nil {
				return &parsed, nil
			}
		}
	}
	if parsed, err := time.Parse(time.RFC3339Nano, normalized); err == nil {
		return &parsed, nil
	}
	return nil, fmt.Errorf("unsupported timestamp format %q", value)
}

func hasExplicitUTCOffset(value string) bool {
	if len(value) < 6 {
		return false
	}
	// Only inspect the time portion so date hyphens do not count as an offset.
	separator := strings.IndexAny(value, "T ")
	if separator < 0 || separator+1 >= len(value) {
		return false
	}
	clock := value[separator+1:]
	return strings.Contains(clock, "+") || strings.Contains(clock, "-")
}

func parseTradeDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{"2006-01-02", "20060102", time.RFC3339Nano, "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			// The upstream API defines daily bar dates, not instants. Normalize
			// date-only values to UTC midnight to avoid host-local timezone drift.
			if layout == "2006-01-02" || layout == "20060102" {
				return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), nil
			}
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported daily bar date %q", value)
}

func validateSymbol(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("stock symbol is required")
	}
	if len(value) > 32 {
		return errors.New("stock symbol exceeds 32 characters")
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-' {
			continue
		}
		return errors.New("stock symbol contains unsupported characters")
	}
	return nil
}

func isFinite(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

func finiteOptional(v *float64) bool { return v == nil || isFinite(*v) }
