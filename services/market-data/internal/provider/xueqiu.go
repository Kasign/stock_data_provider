package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const xueqiuVersion = "xueqiu-hot-ranking-v1"

type Xueqiu struct {
	client  *http.Client
	baseURL string
	cookie  string
}

// NewXueqiuConfigured configures an optional domestic ranking fallback.
// The cookie is kept only in memory and is never emitted to logs or errors.
func NewXueqiuConfigured(timeout time.Duration, baseURL, cookie string) *Xueqiu {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://stock.xueqiu.com"
	}
	return &Xueqiu{client: &http.Client{Timeout: timeout}, baseURL: baseURL, cookie: strings.TrimSpace(cookie)}
}

func (p *Xueqiu) Name() string { return "xueqiu" }

func xueqiuString(row map[string]json.RawMessage, field string) string {
	raw := bytes.TrimSpace(row[field])
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(string(raw))
}

func xueqiuNumber(row map[string]json.RawMessage, fields ...string) (*float64, error) {
	for _, field := range fields {
		raw := bytes.TrimSpace(row[field])
		if len(raw) == 0 || bytes.Equal(raw, []byte("null")) || bytes.Equal(raw, []byte{'"', '"'}) {
			continue
		}
		var value float64
		if err := json.Unmarshal(raw, &value); err != nil {
			var text string
			if stringErr := json.Unmarshal(raw, &text); stringErr != nil {
				return nil, fmt.Errorf("雪球字段 %s 不是有效数值", field)
			}
			var parseErr error
			value, parseErr = strconv.ParseFloat(strings.TrimSpace(text), 64)
			if parseErr != nil {
				return nil, fmt.Errorf("雪球字段 %s 不是有效数值", field)
			}
		}
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("雪球字段 %s 不是有限数值", field)
		}
		return &value, nil
	}
	return nil, nil
}

func xueqiuSymbol(row map[string]json.RawMessage) (string, error) {
	raw := strings.ToUpper(strings.TrimSpace(xueqiuString(row, "symbol")))
	if raw == "" {
		raw = strings.ToUpper(strings.TrimSpace(xueqiuString(row, "code")))
	}
	if raw == "" {
		return "", errors.New("雪球榜单行缺少证券代码")
	}
	exchange := strings.ToUpper(strings.TrimSpace(xueqiuString(row, "exchange")))
	switch {
	case strings.Contains(raw, "."):
	case len(raw) == 8 && (strings.HasPrefix(raw, "SH") || strings.HasPrefix(raw, "SZ") || strings.HasPrefix(raw, "BJ")):
		raw = raw[2:] + "." + raw[:2]
	case len(raw) == 6:
		if exchange == "" {
			switch raw[0] {
			case '6', '9':
				exchange = "SH"
			case '0', '1', '2', '3':
				exchange = "SZ"
			case '4', '8':
				exchange = "BJ"
			}
		}
		if exchange == "" {
			return "", fmt.Errorf("雪球证券代码 %q 缺少交易所信息", raw)
		}
		raw += "." + exchange
	default:
		return "", fmt.Errorf("雪球证券代码格式无效: %q", raw)
	}
	symbol, err := normalize.NormalizeSymbol(raw)
	if err != nil {
		return "", err
	}
	if !isTushareAShareSymbol(symbol) {
		return "", fmt.Errorf("雪球榜单返回了非 A 股证券 %q", symbol)
	}
	return symbol, nil
}

func xueqiuProviderTime(raw json.RawMessage, now time.Time) *time.Time {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	var numeric int64
	if json.Unmarshal(raw, &numeric) == nil && numeric > 0 {
		var candidate time.Time
		if numeric > 100000000000 {
			candidate = time.UnixMilli(numeric).UTC()
		} else {
			candidate = time.Unix(numeric, 0).UTC()
		}
		if !candidate.After(now.Add(5*time.Minute)) && !candidate.Before(now.AddDate(-1, 0, 0)) {
			return &candidate
		}
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05"} {
			var parsed time.Time
			var err error
			if layout == time.RFC3339Nano {
				parsed, err = time.Parse(layout, text)
			} else {
				parsed, err = time.ParseInLocation(layout, text, time.FixedZone("CST", 8*60*60))
			}
			if err == nil && !parsed.After(now.Add(5*time.Minute)) && !parsed.Before(now.AddDate(-1, 0, 0)) {
				parsed = parsed.UTC()
				return &parsed
			}
		}
	}
	return nil
}

func xueqiuRows(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, fmt.Errorf("雪球榜单 data 不是对象: %w", err)
	}
	for _, key := range []string{"items", "list"} {
		if len(obj[key]) == 0 || bytes.Equal(bytes.TrimSpace(obj[key]), []byte("null")) {
			continue
		}
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(obj[key], &rows); err != nil {
			return nil, fmt.Errorf("解析雪球榜单 %s 失败: %w", key, err)
		}
		return rows, nil
	}
	return []map[string]json.RawMessage{}, nil
}

// FetchHotStocks adapts Xueqiu's public hot-stock list as a fallback.
// "surge" uses the documented market-wide percentage-mover ordering as a proxy, not Eastmoney hot-up.
func (p *Xueqiu) FetchHotStocks(ctx context.Context, rankingType string, limit int) (*model.HotRankingSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("雪球 HTTP client 未配置")
	}
	rankingType = strings.ToLower(strings.TrimSpace(rankingType))
	if rankingType != "popular" && rankingType != "surge" && rankingType != "followers" {
		return nil, fmt.Errorf("雪球不支持榜单类型 %q", rankingType)
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 100 {
		return nil, errors.New("雪球榜单单次请求最多 100 条")
	}
	requestLimit := min(limit, 50)
	path := "/v5/stock/hot_stock/list.json"
	query := url.Values{}
	query.Set("size", strconv.Itoa(requestLimit))
	query.Set("type", "10")
	if rankingType == "followers" {
		query.Set("type", "12")
	}
	if rankingType == "surge" {
		path = "/v5/stock/screener/quote/list.json"
		query.Set("market", "CN")
		query.Set("order", "desc")
		query.Set("order_by", "percent")
		query.Set("page", "1")
		query.Set("type", "sh_sz")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("创建雪球榜单请求失败: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 Chrome/124.0 Safari/537.36")
	req.Header.Set("Referer", "https://xueqiu.com/")
	req.Header.Set("Origin", "https://xueqiu.com")
	if p.cookie != "" {
		req.Header.Set("Cookie", p.cookie)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("雪球榜单请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = readBoundedBody(resp.Body, 4096)
		return nil, fmt.Errorf("雪球榜单返回 HTTP 状态码 %d（如需鉴权，请配置 MARKET_DATA_XUEQIU_COOKIE）", resp.StatusCode)
	}
	raw, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("读取雪球榜单响应失败: %w", err)
	}
	var envelope map[string]json.RawMessage
	if err := decodeSingleJSON(raw, &envelope); err != nil {
		return nil, fmt.Errorf("解析雪球榜单响应失败: %w", err)
	}
	if rawCode := bytes.TrimSpace(envelope["error_code"]); len(rawCode) > 0 {
		code, parseErr := strconv.Atoi(strings.TrimSpace(strings.Trim(string(rawCode), "\"")))
		if parseErr == nil && code != 0 {
			message := xueqiuString(envelope, "error_description")
			if message == "" {
				message = xueqiuString(envelope, "message")
			}
			if len(message) > 200 {
				message = message[:200]
			}
			return nil, fmt.Errorf("雪球榜单接口错误 code=%d: %s", code, message)
		}
	}
	dataRaw := bytes.TrimSpace(envelope["data"])
	if len(dataRaw) == 0 || bytes.Equal(dataRaw, []byte("null")) {
		return nil, errors.New("雪球榜单响应缺少 data")
	}
	rows, err := xueqiuRows(dataRaw)
	if err != nil {
		return nil, err
	}
	var dataObject map[string]json.RawMessage
	_ = json.Unmarshal(dataRaw, &dataObject)
	now := time.Now().UTC()
	providerTime := xueqiuProviderTime(dataObject["timestamp"], now)
	snapshot := &model.HotRankingSnapshot{
		RankingType: rankingType, Items: make([]model.HotRankingItem, 0, min(requestLimit, len(rows))),
		Source: p.Name(), SourceVersion: xueqiuVersion, FetchedAt: now,
		QualityStatus:  model.QualityDegraded,
		QualityReasons: []string{"provider_hot_ranking_is_delayed_not_a_real_time_trading_signal"},
	}
	if providerTime != nil {
		snapshot.ProviderTime = providerTime
	} else {
		snapshot.QualityReasons = appendTushareReason(snapshot.QualityReasons, "provider_snapshot_timestamp_missing_or_invalid")
	}
	if rankingType == "surge" {
		snapshot.QualityReasons = appendTushareReason(snapshot.QualityReasons, "xueqiu_surge_uses_percentage_mover_proxy_not_eastmoney_hot_up_equivalent")
	}
	if len(rows) > requestLimit {
		snapshot.QualityReasons = appendTushareReason(snapshot.QualityReasons, "provider_returned_more_rows_than_requested")
	}
	seen := make(map[string]struct{}, requestLimit)
	for index, row := range rows {
		if len(snapshot.Items) >= requestLimit {
			break
		}
		symbol, symbolErr := xueqiuSymbol(row)
		if symbolErr != nil {
			return nil, fmt.Errorf("雪球榜单第 %d 行证券代码无效: %w", index, symbolErr)
		}
		if _, duplicate := seen[symbol]; duplicate {
			return nil, fmt.Errorf("雪球榜单返回重复证券 %s", symbol)
		}
		seen[symbol] = struct{}{}
		name := strings.TrimSpace(xueqiuString(row, "name"))
		if name == "" {
			name = strings.TrimSpace(xueqiuString(row, "stock_name"))
		}
		latestPrice, err := xueqiuNumber(row, "current", "current_price")
		if err != nil {
			return nil, err
		}
		if latestPrice != nil && *latestPrice <= 0 {
			latestPrice = nil
		}
		change, err := xueqiuNumber(row, "change")
		if err != nil {
			return nil, err
		}
		changePercent, err := xueqiuNumber(row, "percent", "change_percent")
		if err != nil {
			return nil, err
		}
		heat, err := xueqiuNumber(row, "value", "heat")
		if err != nil {
			return nil, err
		}
		var rankChange *int
		if rawRankChange := strings.TrimSpace(xueqiuString(row, "rank_change")); rawRankChange != "" {
			value, parseErr := strconv.Atoi(rawRankChange)
			if parseErr == nil && value >= -1000000 && value <= 1000000 {
				rankChange = &value
			}
		}
		itemReasons := []string{"provider_hot_ranking_is_delayed_not_a_real_time_trading_signal"}
		if providerTime == nil {
			itemReasons = appendTushareReason(itemReasons, "provider_snapshot_timestamp_missing_or_invalid")
		}
		if rankingType == "surge" {
			itemReasons = appendTushareReason(itemReasons, "xueqiu_surge_uses_percentage_mover_proxy_not_eastmoney_hot_up_equivalent")
		}
		snapshot.Items = append(snapshot.Items, model.HotRankingItem{
			Rank: len(snapshot.Items) + 1, Symbol: symbol, Code: strings.Split(symbol, ".")[0],
			Name: name, LatestPrice: latestPrice, Change: change, ChangePercent: changePercent,
			RankChange: rankChange, Heat: heat, Source: p.Name(), SourceVersion: xueqiuVersion,
			FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: itemReasons,
		})
	}
	snapshot.Total = len(snapshot.Items)
	if snapshot.Total == 0 {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = appendTushareReason(snapshot.QualityReasons, "empty_xueqiu_ranking_cannot_confirm_zero_results_or_unavailable_source")
	}
	return snapshot, nil
}
