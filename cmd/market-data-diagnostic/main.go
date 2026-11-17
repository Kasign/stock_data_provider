package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"example.com/stock-analysis-flow/internal/marketdata/config"
	"example.com/stock-analysis-flow/internal/marketdata/model"
	"example.com/stock-analysis-flow/internal/marketdata/provider"
	"example.com/stock-analysis-flow/internal/platform/httpclient"
)

const (
	sampleSymbol = "600519.SH"
	sampleDate   = "2026-11-09"
	maxCapture   = int64(4 << 20)
)

type capture struct {
	Method           string            `json:"method"`
	RequestURL       string            `json:"request_url"`
	RequestHeaders   map[string]string `json:"request_headers,omitempty"`
	StatusCode       int               `json:"status_code,omitempty"`
	ResponseHeaders  map[string]string `json:"response_headers,omitempty"`
	BodyComplete     bool              `json:"body_complete"`
	BodyBytes        int               `json:"body_bytes"`
	RawPayloadBase64 string            `json:"raw_payload_base64,omitempty"`
	RawPayloadText   string            `json:"raw_payload_text,omitempty"`
	Error            string            `json:"error,omitempty"`
	DurationMS       int64             `json:"duration_ms"`
}

type captureDoer struct {
	next    provider.HTTPDoer
	limit   int64
	secrets []string
	mu      sync.Mutex
	items   []capture
}

func (d *captureDoer) Do(req *http.Request) (*http.Response, error) {
	started := time.Now()
	item := capture{
		Method: req.Method, RequestURL: sanitizeURL(req.URL),
		RequestHeaders: selectedHeaders(req.Header, []string{"Accept", "Content-Type", "User-Agent", "Referer"}),
	}
	resp, err := d.next.Do(req)
	item.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		item.Error = d.sanitizeError(err.Error())
		d.append(item)
		return nil, err
	}
	if resp == nil {
		item.Error = "HTTP executor returned a nil response"
		d.append(item)
		return nil, errors.New(item.Error)
	}
	item.StatusCode = resp.StatusCode
	item.ResponseHeaders = selectedHeaders(resp.Header, []string{"Content-Type", "Content-Length", "Date", "ETag", "Last-Modified", "Server"})
	if resp.Body == nil {
		item.BodyComplete = true
		d.append(item)
		return resp, nil
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, d.limit+1))
	_ = resp.Body.Close()
	if int64(len(body)) > d.limit {
		body = body[:d.limit]
		item.BodyComplete = false
		item.Error = "capture body exceeded configured limit"
	} else {
		item.BodyComplete = readErr == nil
	}
	if readErr != nil {
		item.Error = d.sanitizeError(readErr.Error())
	}
	item.BodyBytes = len(body)
	item.RawPayloadBase64 = base64.StdEncoding.EncodeToString(body)
	if utf8.Valid(body) {
		item.RawPayloadText = string(body)
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	resp.ContentLength = int64(len(body))
	d.append(item)
	return resp, nil
}

func (d *captureDoer) append(item capture) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.items = append(d.items, item)
}

func (d *captureDoer) snapshot() []capture {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := append([]capture(nil), d.items...)
	return out
}

func (d *captureDoer) sanitizeError(value string) string {
	for _, secret := range d.secrets {
		if secret != "" {
			value = strings.ReplaceAll(value, secret, "[REDACTED]")
		}
	}
	return value
}

type report struct {
	Provider   string           `json:"provider"`
	Symbol     string           `json:"symbol"`
	TradeDate  string           `json:"trade_date"`
	Period     string           `json:"period"`
	Adjust     string           `json:"adjustment"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt time.Time        `json:"finished_at"`
	Error      string           `json:"error,omitempty"`
	Requests   []capture        `json:"requests"`
	DailyBars  []model.DailyBar `json:"daily_bars,omitempty"`
}

func main() {
	name := flag.String("provider", "", "eastmoney, tencent, sina, or netease")
	flag.Parse()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := run(ctx, strings.ToLower(strings.TrimSpace(*name))); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, name string) error {
	if name != "eastmoney" && name != "tencent" && name != "sina" && name != "netease" {
		return errors.New("必须通过 -provider 指定 eastmoney、tencent、sina 或 netease；每次只采集一个 Provider")
	}
	cfg, err := config.LoadForInProcess()
	if err != nil {
		return fmt.Errorf("加载 Market Data 配置失败: %w", err)
	}
	endpoint := map[string]string{
		"eastmoney": cfg.EastmoneyKlineURL,
		"tencent":   cfg.TencentNoneKlineURL,
		"sina":      cfg.SinaKlineURL,
		"netease":   cfg.NeteaseHistoryURL,
	}[name]
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed == nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("%s 日 K endpoint 必须是合法 HTTPS URL", name)
	}
	port := parsed.Port()
	if port != "" && port != "443" {
		return fmt.Errorf("%s 日 K endpoint 使用非标准 HTTPS 端口 %q，诊断命令拒绝放宽端口策略", name, port)
	}
	host := strings.ToLower(parsed.Hostname())
	origin := strings.ToLower(parsed.Scheme + "://" + parsed.Host)
	dependency := "marketdata." + name + ".dailybars"
	platformClient, err := httpclient.New(httpclient.Config{
		Timeout: cfg.ProviderTimeout, DialTimeout: 5 * time.Second,
		ResponseHeaderTimeout: 8 * time.Second, MaxResponseHeaderBytes: 1 << 20,
		MaxResponseBytes: maxCapture, MaxRetries: 0, MaxConcurrent: 1,
	})
	if err != nil {
		return fmt.Errorf("创建受控 HTTP Client 失败: %w", err)
	}
	defer platformClient.CloseIdleConnections()
	bound, err := platformClient.Bind(httpclient.DestinationPolicy{
		Dependency: dependency, AllowedHosts: []string{host}, AllowedPorts: []uint16{443},
		AllowHTTP: false, AllowRedirects: false, MaxRedirects: 0, Idempotent: false,
	})
	if err != nil {
		return fmt.Errorf("绑定 %s 出站策略失败: %w", name, err)
	}
	captured := &captureDoer{
		next: bound, limit: maxCapture,
		secrets: []string{cfg.EastmoneyUT, cfg.TushareToken, cfg.HithinkAPIKey, cfg.TickFlowAPIKey, cfg.XueqiuCookie, cfg.MXAPIKey},
	}
	dailyProvider, err := buildProvider(cfg, name, host, origin, captured)
	if err != nil {
		return err
	}
	result := report{
		Provider: name, Symbol: sampleSymbol, TradeDate: sampleDate,
		Period: "daily", Adjust: string(model.AdjustmentNone), StartedAt: time.Now().UTC(),
	}
	bars, fetchErr := dailyProvider.FetchDailyBars(ctx, sampleSymbol, sampleDate, sampleDate, 10, model.AdjustmentNone)
	result.FinishedAt = time.Now().UTC()
	result.Requests = captured.snapshot()
	result.DailyBars = bars
	if fetchErr != nil {
		result.Error = captured.sanitizeError(fetchErr.Error())
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return fmt.Errorf("输出诊断报告失败: %w", err)
	}
	if fetchErr != nil {
		return errors.New(result.Error)
	}
	return nil
}

func buildProvider(cfg config.Config, name, host, origin string, doer provider.HTTPDoer) (provider.DailyBarProvider, error) {
	switch name {
	case "eastmoney":
		p := provider.NewEastmoneyConfiguredWithAnnouncements(
			cfg.ProviderTimeout, cfg.EastmoneyQuoteURL, cfg.EastmoneyKlineURL,
			cfg.EastmoneyStockBasicURL, cfg.EastmoneySectorRankingURL, cfg.EastmoneyIndexURL,
			cfg.EastmoneyMarketStocksURL, cfg.EastmoneyLimitPoolBaseURL, cfg.EastmoneyStockBoardsURL,
			cfg.EastmoneyDragonTigerURL, cfg.EastmoneyAnnouncementsURL, cfg.EastmoneyUT,
		).WithHotRankingURLs(cfg.EastmoneyHotRankingsURL, cfg.EastmoneyHotSurgeURL).
			WithCapitalFlowURL(cfg.EastmoneyCapitalFlowURL).
			WithHTTPDoers(map[string]provider.HTTPDoer{origin: doer})
		return p, nil
	case "tencent":
		return provider.NewTencentConfiguredWithHTTPDoers(
			cfg.ProviderTimeout, cfg.TencentQuoteURL, cfg.TencentKlineURL, cfg.TencentNoneKlineURL,
			cfg.TencentStockSearchURL, nil, nil, doer, nil,
		), nil
	case "sina":
		return provider.NewSinaConfiguredWithHTTPDoers(cfg.ProviderTimeout, cfg.SinaQuoteURL, cfg.SinaKlineURL, nil, doer), nil
	case "netease":
		return provider.NewNeteaseConfiguredWithHTTPDoers(cfg.ProviderTimeout, cfg.NeteaseQuoteURL, cfg.NeteaseHistoryURL, nil, doer), nil
	default:
		return nil, fmt.Errorf("不支持的 Provider %q", name)
	}
}

func sanitizeURL(value *url.URL) string {
	if value == nil {
		return ""
	}
	copyURL := *value
	query := copyURL.Query()
	for key := range query {
		if sensitiveQueryKey(key) {
			query.Set(key, "[REDACTED]")
		}
	}
	copyURL.RawQuery = query.Encode()
	copyURL.User = nil
	return copyURL.String()
}

func sensitiveQueryKey(key string) bool {
	key = strings.ToLower(key)
	for _, part := range []string{"token", "key", "secret", "auth", "cookie", "password", "signature", "sign", "ut"} {
		if strings.Contains(key, part) {
			return true
		}
	}
	return false
}

func selectedHeaders(header http.Header, allowed []string) map[string]string {
	out := make(map[string]string)
	for _, name := range allowed {
		if value := header.Get(name); value != "" {
			out[name] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
