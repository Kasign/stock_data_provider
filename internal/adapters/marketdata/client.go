package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL          *url.URL
	httpClient       *http.Client
	apiKey           string
	maxResponseBytes int64
	adjustment       string
	marketLocation   *time.Location
}

func NewClient(cfg Config) (*Client, error) {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 135 * time.Second
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = 16 << 20
	}
	if cfg.MaxResponseBytes < 1024 || cfg.MaxResponseBytes > 128<<20 {
		return nil, errors.New("MaxResponseBytes must be between 1024 and 134217728")
	}
	u, err := url.Parse(strings.TrimSpace(cfg.BaseURL))
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, errors.New("MARKET_DATA_BASE_URL must be an absolute http/https URL")
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return nil, errors.New("MARKET_DATA_BASE_URL must not contain credentials, query, or fragment")
	}
	if len(strings.TrimSpace(cfg.APIKey)) < 32 || isPlaceholderSecret(cfg.APIKey) {
		return nil, errors.New("MARKET_DATA_API_KEY must be a non-placeholder secret with at least 32 characters")
	}
	if cfg.HistoryDays < 1 || cfg.HistoryDays > 365 {
		return nil, errors.New("history days must be between 1 and 365")
	}
	if cfg.MaxConcurrency < 1 || cfg.MaxConcurrency > 32 {
		return nil, errors.New("collector max concurrency must be between 1 and 32")
	}
	if cfg.Adjustment == "" {
		cfg.Adjustment = "forward_adjusted"
	}
	switch cfg.Adjustment {
	case "none", "forward_adjusted", "backward_adjusted":
	default:
		return nil, fmt.Errorf("unsupported market-data adjustment basis %q", cfg.Adjustment)
	}
	location, err := time.LoadLocation(cfg.MarketTimezone)
	if err != nil {
		return nil, fmt.Errorf("load market timezone: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return &Client{
		baseURL: u,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		apiKey:           strings.TrimSpace(cfg.APIKey),
		maxResponseBytes: cfg.MaxResponseBytes,
		adjustment:       cfg.Adjustment,
		marketLocation:   location,
	}, nil
}

func (c *Client) GetQuote(ctx context.Context, symbol string) (QuoteResponse, time.Time, error) {
	canonical, err := canonicalSymbol(symbol)
	if err != nil {
		return QuoteResponse{}, time.Time{}, err
	}
	var result QuoteResponse
	fetchedAt, err := c.getJSON(ctx, "/api/v1/quotes/"+url.PathEscape(canonical), nil, &result)
	if err != nil {
		return QuoteResponse{}, fetchedAt, err
	}
	if strings.TrimSpace(result.StockCode) == "" {
		result.StockCode = canonical
	}
	if result.FetchedAt.IsZero() {
		result.FetchedAt = fetchedAt
	}
	if result.CurrentPrice <= 0 || !isFinite(result.CurrentPrice) {
		return QuoteResponse{}, fetchedAt, errors.New("Go market-data quote has missing, non-finite, or non-positive current_price")
	}
	if !finiteOptional(result.Change) || !finiteOptional(result.ChangePct) ||
		!finiteOptional(result.Open) || !finiteOptional(result.High) ||
		!finiteOptional(result.Low) || !finiteOptional(result.PrevClose) ||
		!finiteOptional(result.Volume) || !finiteOptional(result.Amount) {
		return QuoteResponse{}, fetchedAt, errors.New("Go market-data quote contains a non-finite optional numeric field")
	}
	if result.UpdateTime == "" {
		return result, fetchedAt, errors.New("Go market-data quote has no provider quote_time")
	}
	return result, fetchedAt, nil
}

func (c *Client) GetHistory(ctx context.Context, symbol string, days int) (HistoryResult, error) {
	if days < 1 || days > 365 {
		return HistoryResult{}, errors.New("history days must be between 1 and 365")
	}
	canonical, err := canonicalSymbol(symbol)
	if err != nil {
		return HistoryResult{}, err
	}
	query := url.Values{}
	query.Set("limit", strconv.Itoa(days))
	query.Set("adjustment", c.adjustment)
	var payload apiHistory
	fetchedAt, err := c.getJSON(ctx, "/api/v1/bars/"+url.PathEscape(canonical)+"/daily", query, &payload)
	if err != nil {
		return HistoryResult{}, err
	}
	if len(payload.Bars) == 0 {
		return HistoryResult{}, errors.New("Go market-data API returned no daily bars")
	}
	if payload.Symbol == "" {
		payload.Symbol = canonical
	}
	limitations := make([]string, 0, 2)
	if payload.AdjustmentBasis == "" || payload.AdjustmentBasis == "unknown" {
		limitations = append(limitations, "adjustment_basis is unavailable or unknown")
	}
	return HistoryResult{
		Symbol: payload.Symbol, Bars: payload.Bars, FetchedAt: fetchedAt,
		Dataset: historyDataset, Limitations: limitations, Warnings: append([]string(nil), payload.Warnings...),
		QualityStatus: payload.QualityStatus,
	}, nil
}

func (c *Client) getJSON(ctx context.Context, requestPath string, query url.Values, output any) (time.Time, error) {
	if ctx == nil {
		return time.Time{}, errors.New("context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return time.Time{}, err
	}
	joined := *c.baseURL
	pathOnly := strings.TrimLeft(requestPath, "/")
	joined.Path = strings.TrimRight(joined.Path, "/") + "/" + pathOnly
	if query != nil {
		joined.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joined.String(), nil)
	if err != nil {
		return time.Time{}, fmt.Errorf("build Go market-data API request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "stock-analysis-go-marketdata-adapter/1.0")
	req.Header.Set("X-API-Key", c.apiKey)
	resp, err := c.httpClient.Do(req)
	fetchedAt := time.Now().UTC()
	if err != nil {
		return fetchedAt, fmt.Errorf("request Go market-data API %s: %w", joined.Redacted(), err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fetchedAt, &HTTPStatusError{Method: req.Method, Path: requestPath, Status: resp.StatusCode}
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return fetchedAt, fmt.Errorf("read Go market-data API response: %w", err)
	}
	if int64(len(body)) > c.maxResponseBytes {
		return fetchedAt, fmt.Errorf("Go market-data API response exceeds configured limit of %d bytes", c.maxResponseBytes)
	}
	if err := json.Unmarshal(body, output); err != nil {
		return fetchedAt, fmt.Errorf("decode Go market-data API JSON response: %w", err)
	}
	return time.Now().UTC(), nil
}

// canonicalSymbol 将证券代码转换为 Go 行情 API 需要的显式交易所代码；不会猜测美股等海外市场的代码规则。
func canonicalSymbol(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	value = strings.ReplaceAll(value, " ", "")
	if value == "" {
		return "", errors.New("stock symbol is required")
	}
	switch {
	case len(value) == 8 && strings.HasPrefix(value, "SH") && digits(value[2:]):
		return value[2:] + ".SH", nil
	case len(value) == 8 && strings.HasPrefix(value, "SZ") && digits(value[2:]):
		return value[2:] + ".SZ", nil
	case len(value) == 8 && strings.HasPrefix(value, "BJ") && digits(value[2:]):
		return value[2:] + ".BJ", nil
	case len(value) == 9 && digits(value[:6]) && strings.Contains("|SH|SZ|BJ|", "|"+value[7:9]+"|") && value[6] == '.':
		return value, nil
	case len(value) == 6 && digits(value):
		switch value[0] {
		case '5', '6', '9':
			return value + ".SH", nil
		case '4', '8':
			return value + ".BJ", nil
		default:
			return value + ".SZ", nil
		}
	default:
		return "", fmt.Errorf("unsupported mainland stock symbol %q; use six digits or an explicit .SH/.SZ/.BJ suffix", raw)
	}
}

func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseTimestamp(value string, location *time.Location) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
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
	return nil, fmt.Errorf("unsupported timestamp format %q", value)
}

func hasExplicitUTCOffset(value string) bool {
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
			if layout == "2006-01-02" || layout == "20060102" {
				return time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 0, 0, 0, 0, time.UTC), nil
			}
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported daily bar date %q", value)
}

func isFinite(v float64) bool        { return !math.IsNaN(v) && !math.IsInf(v, 0) }
func finiteOptional(v *float64) bool { return v == nil || isFinite(*v) }
