package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const (
	hithinkVersion          = "hithink-finance-rest-v1"
	hithinkMaxPagesPerRange = 40
	hithinkMaxRowsPerCall   = 20000
)

// Hithink 对接同花顺官方金融数据 REST API。接口需要 API Key，默认不开启。
// 认证只通过 X-api-key Header 发送，不将 Key 放入 URL、日志或错误文本。
type Hithink struct {
	client  *http.Client
	baseURL string
	apiKey  string

	rateMu   sync.Mutex
	lastCall time.Time
}

func NewHithinkConfigured(timeout time.Duration, baseURL, apiKey string) *Hithink {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://fuyao.aicubes.cn"
	}
	return &Hithink{
		client:  &http.Client{Timeout: timeout},
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
	}
}

func (p *Hithink) Name() string { return "hithink" }

func (p *Hithink) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺行情接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	symbol, err := hithinkDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := url.Values{}
	query.Set("thscodes", symbol)
	raw, err := p.get(ctx, "/api/a-share/prices/snapshot", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺行情请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp *int64 `json:"timestamp"`
		Item      []struct {
			THSCode       string   `json:"thscode"`
			LastPrice     *float64 `json:"last_price"`
			PriceChange   *float64 `json:"price_change"`
			ChangeRatio   *float64 `json:"price_change_ratio_pct"`
			OpenPrice     *float64 `json:"open_price"`
			HighPrice     *float64 `json:"high_price"`
			LowPrice      *float64 `json:"low_price"`
			PreviousPrice *float64 `json:"prev_price"`
			Volume        *float64 `json:"volume"`
			Turnover      *float64 `json:"turnover"`
		} `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺行情数据失败: %w", err)
	}
	var item *struct {
		THSCode       string   `json:"thscode"`
		LastPrice     *float64 `json:"last_price"`
		PriceChange   *float64 `json:"price_change"`
		ChangeRatio   *float64 `json:"price_change_ratio_pct"`
		OpenPrice     *float64 `json:"open_price"`
		HighPrice     *float64 `json:"high_price"`
		LowPrice      *float64 `json:"low_price"`
		PreviousPrice *float64 `json:"prev_price"`
		Volume        *float64 `json:"volume"`
		Turnover      *float64 `json:"turnover"`
	}
	for i := range data.Item {
		if strings.EqualFold(strings.TrimSpace(data.Item[i].THSCode), symbol) {
			item = &data.Item[i]
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("同花顺行情响应未包含请求证券 %s", symbol)
	}
	if item.LastPrice == nil || !hithinkPositiveFinite(*item.LastPrice) ||
		item.PreviousPrice == nil || !hithinkPositiveFinite(*item.PreviousPrice) ||
		item.OpenPrice == nil || item.HighPrice == nil || item.LowPrice == nil ||
		!hithinkPositiveFinite(*item.OpenPrice) || !hithinkPositiveFinite(*item.HighPrice) || !hithinkPositiveFinite(*item.LowPrice) ||
		*item.LowPrice > *item.HighPrice || *item.OpenPrice < *item.LowPrice || *item.OpenPrice > *item.HighPrice ||
		*item.LastPrice < *item.LowPrice || *item.LastPrice > *item.HighPrice {
		return nil, errors.New("同花顺行情 OHLC/价格字段不满足数据契约")
	}

	change := *item.LastPrice - *item.PreviousPrice
	if item.PriceChange != nil && hithinkFinite(*item.PriceChange) {
		change = *item.PriceChange
	}
	changePercent := change / *item.PreviousPrice * 100
	if item.ChangeRatio != nil && hithinkFinite(*item.ChangeRatio) {
		changePercent = *item.ChangeRatio
	}
	now := time.Now().UTC()
	var volume *int64
	var rawVolume *float64
	var amount, rawAmount *float64
	quality := model.QualityDegraded
	reasons := []string{"provider_quote_timestamp_not_available_for_symbol_query"}
	if item.Volume != nil && hithinkNonNegativeFinite(*item.Volume) && math.Trunc(*item.Volume) == *item.Volume && *item.Volume <= float64(math.MaxInt64) {
		v := int64(*item.Volume)
		volume = &v
		vRaw := *item.Volume
		rawVolume = &vRaw
	} else {
		reasons = append(reasons, "volume_missing_or_invalid")
	}
	if item.Turnover != nil && hithinkNonNegativeFinite(*item.Turnover) {
		v := *item.Turnover
		amount, rawAmount = &v, &v
	} else {
		reasons = append(reasons, "amount_missing_or_invalid")
	}
	qualityReason := []string(nil)
	if len(reasons) > 0 {
		qualityReason = append(qualityReason, reasons...)
	}
	volumeUnit, amountUnit := "shares", "CNY"
	rawAmountUnit, rawAmountField := "CNY", "turnover"
	if amount == nil {
		amountUnit, rawAmountUnit, rawAmountField = "unknown", "", ""
	}
	return &model.Quote{
		Symbol: symbol,
		CurrentPrice: item.LastPrice,
		Change: &change,
		ChangePercent: &changePercent,
		Open: item.OpenPrice,
		High: item.HighPrice,
		Low: item.LowPrice,
		PrevClose: item.PreviousPrice,
		Volume: volume,
		Amount: amount,
		FetchedAt: now,
		NormalizedAt: now,
		Source: p.Name(),
		SourceVersion: hithinkVersion,
		RawVolume: rawVolume,
		RawVolumeUnit: volumeUnit,
		RawAmount: rawAmount,
		RawAmountUnit: rawAmountUnit,
		RawAmountField: rawAmountField,
		VolumeUnit: volumeUnit,
		AmountUnit: amountUnit,
		AdjustmentBasis: model.AdjustmentNone,
		QualityStatus: quality,
		QualityReasons: qualityReason,
		NormalizationVer: normalize.Version,
	}, nil
}

func (p *Hithink) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺历史 K 线接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	symbol, err := hithinkDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil {
		return nil, fmt.Errorf("无效的同花顺日 K 起始日期 %q", startDate)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil {
		return nil, fmt.Errorf("无效的同花顺日 K 结束日期 %q", endDate)
	}
	if start.After(end) {
		return nil, errors.New("同花顺日 K 起始日期不能晚于结束日期")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if end.Format("2006-01-02") > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("同花顺日 K 结束日期不能晚于 Asia/Shanghai 今日")
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 2000 {
		return nil, errors.New("同花顺日 K 条数上限为 2000")
	}
	adjust := ""
	switch adjustment {
	case model.AdjustmentNone:
		adjust = "none"
	case model.AdjustmentForward:
		adjust = "forward"
	case model.AdjustmentBackward:
		adjust = "backward"
	default:
		return nil, fmt.Errorf("不支持的同花顺复权口径 %q", adjustment)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	all := make([]model.DailyBar, 0, min(limit, 2000))
	segmentStart := start
	for !segmentStart.After(end) {
		segmentEnd := segmentStart.AddDate(9, 0, 0)
		if segmentEnd.After(end) {
			segmentEnd = end
		}
		segmentBars, err := p.fetchDailySegment(ctx, symbol, segmentStart, segmentEnd, adjust, loc)
		if err != nil {
			return nil, err
		}
		all = append(all, segmentBars...)
		if len(all) > hithinkMaxRowsPerCall {
			return nil, fmt.Errorf("同花顺日 K 结果超过安全上限 %d 行", hithinkMaxRowsPerCall)
		}
		segmentStart = segmentEnd.AddDate(0, 0, 1)
	}
	if len(all) == 0 {
		return nil, errors.New("同花顺未返回请求区间的日 K")
	}
	sort.Slice(all, func(i, j int) bool { return all[i].TradeDate < all[j].TradeDate })
	for i := 1; i < len(all); i++ {
		if all[i].TradeDate == all[i-1].TradeDate {
			return nil, fmt.Errorf("同花顺返回重复交易日 %s", all[i].TradeDate)
		}
	}
	for i := range all {
		if all[i].ChangePercent == nil && i > 0 && all[i-1].Close > 0 {
			v := (all[i].Close - all[i-1].Close) / all[i-1].Close * 100
			all[i].ChangePercent = &v
		}
		if all[i].ChangePercent == nil {
			all[i].QualityStatus = model.QualityDegraded
			all[i].QualityReasons = append(all[i].QualityReasons, "change_percent_unavailable_for_first_bar")
		}
	}
	return normalizeProviderDailyBars(symbol, start.Format("2006-01-02"), end.Format("2006-01-02"), limit, all, 0)
}

// FetchFundamentals 使用同花顺财务指标端点读取单个报告期的能力指标。
// 官方响应不包含可确认的披露时间，也未为每个指标提供单位说明；因此缺失项不补零，完整性/时间可用性不满足要求时保持 degraded。
func (p *Hithink) FetchFundamentals(ctx context.Context, rawSymbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺财务指标接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	symbol, err := hithinkDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	nowYear := time.Now().Year()
	if year < 2000 || year > nowYear {
		return nil, fmt.Errorf("同花顺财务指标年份必须在 2000..%d", nowYear)
	}
	if quarter < 1 || quarter > 4 {
		return nil, errors.New("同花顺财务指标季度必须为 1..4")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	report := fmt.Sprintf("%d-%d", year, quarter)
	query := url.Values{}
	query.Set("thscode", symbol)
	query.Set("report", report)
	raw, err := p.get(ctx, "/api/a-share/financials/indicators", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺财务指标请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		THSCode string `json:"thscode"`
		Report  string `json:"report"`
		Abilities []struct {
			Ability string `json:"ability"`
			Indicators []struct {
				IndexID string `json:"index_id"`
				Value json.RawMessage `json:"value"`
			} `json:"indicators"`
		} `json:"abilities"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺财务指标失败: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(data.THSCode), symbol) {
		return nil, fmt.Errorf("同花顺财务响应证券代码不匹配: requested=%s returned=%q", symbol, data.THSCode)
	}
	if strings.TrimSpace(data.Report) != report {
		return nil, fmt.Errorf("同花顺财务响应报告期不匹配: requested=%s returned=%q", report, data.Report)
	}

	reportDate := time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	snapshot := &model.FundamentalsSnapshot{
		Symbol: symbol, Year: year, Quarter: quarter,
		Sections: make([]model.FundamentalSection, 0, len(data.Abilities)),
		Source: p.Name(), SourceVersion: hithinkVersion,
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"publication_date_not_exposed_by_provider", "indicator_units_not_exposed_by_provider"},
	}
	for _, ability := range data.Abilities {
		category := hithinkFundamentalCategory(ability.Ability)
		section := model.FundamentalSection{
			Category: category, ReportDate: reportDate,
			Metrics: make([]model.FundamentalMetric, 0, len(ability.Indicators)),
			QualityStatus: model.QualityDegraded,
			QualityReasons: []string{"publication_date_not_exposed_by_provider", "metric_unit_not_exposed_by_provider"},
		}
		invalidValueCount := 0
		for _, item := range ability.Indicators {
			indexID := strings.TrimSpace(item.IndexID)
			if indexID == "" {
				invalidValueCount++
				continue
			}
			value, ok := hithinkParseFundamentalValue(item.Value)
			if !ok {
				invalidValueCount++
				continue
			}
			section.Metrics = append(section.Metrics, model.FundamentalMetric{
				Key: indexID, Name: indexID, Value: value,
				Unit: "provider_defined", SourceField: indexID,
			})
		}
		section.HasData = len(section.Metrics) > 0
		if invalidValueCount > 0 {
			section.QualityReasons = append(section.QualityReasons, "one_or_more_metrics_missing_or_invalid")
		}
		if !section.HasData {
			section.QualityReasons = append(section.QualityReasons, "no_numeric_metrics_available")
		} else {
			snapshot.HasData = true
		}
		snapshot.Sections = append(snapshot.Sections, section)
	}
	if !snapshot.HasData {
		snapshot.QualityReasons = append(snapshot.QualityReasons, "no_numeric_metrics_for_requested_report")
	}
	return snapshot, nil
}

func hithinkParseFundamentalValue(raw json.RawMessage) (float64, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, false
	}
	var textValue string
	if json.Unmarshal(raw, &textValue) == nil {
		textValue = strings.TrimSpace(textValue)
		if textValue == "" {
			return 0, false
		}
		value, err := strconv.ParseFloat(textValue, 64)
		return value, err == nil && hithinkFinite(value)
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	return value, err == nil && hithinkFinite(value)
}

func hithinkFundamentalCategory(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "growth":
		return "growth"
	case "profitability":
		return "profitability"
	case "solvency":
		return "solvency"
	case "operation":
		return "operating"
	case "cash-flow":
		return "cash_flow"
	default:
		value := strings.ToLower(strings.TrimSpace(raw))
		value = strings.ReplaceAll(value, "-", "_")
		if value == "" {
			return "other"
		}
		return value
	}
}

func (p *Hithink) fetchDailySegment(ctx context.Context, symbol string, start, end time.Time, adjust string, loc *time.Location) ([]model.DailyBar, error) {
	startMS := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc).UnixMilli()
	endMS := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), loc).UnixMilli()
	offset := 0
	out := make([]model.DailyBar, 0, 1000)
	seen := make(map[string]struct{})
	for page := 0; page < hithinkMaxPagesPerRange; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		query := url.Values{}
		query.Set("thscode", symbol)
		query.Set("interval", "1d")
		query.Set("start", strconv.FormatInt(startMS, 10))
		query.Set("end", strconv.FormatInt(endMS, 10))
		query.Set("adjust", adjust)
		query.Set("offset", strconv.Itoa(offset))
		raw, err := p.get(ctx, "/api/a-share/prices/historical", query)
		if err != nil {
			return nil, fmt.Errorf("同花顺历史 K 线区间 %s..%s 请求失败: %w", start.Format("2006-01-02"), end.Format("2006-01-02"), err)
		}
		dataRaw, err := hithinkResponseData(raw)
		if err != nil {
			return nil, err
		}
		var data struct {
			Timestamp *int64 `json:"timestamp"`
			Item []struct {
				DateMS      *int64   `json:"date_ms"`
				OpenPrice   *float64 `json:"open_price"`
				HighPrice   *float64 `json:"high_price"`
				LowPrice    *float64 `json:"low_price"`
				ClosePrice  *float64 `json:"close_price"`
				Volume      *float64 `json:"volume"`
				Turnover    *float64 `json:"turnover"`
			} `json:"item"`
		}
		if err := json.Unmarshal(dataRaw, &data); err != nil {
			return nil, fmt.Errorf("解析同花顺历史 K 线失败: %w", err)
		}
		if len(data.Item) == 0 {
			return out, nil
		}
		fetchedAt := time.Now().UTC()
		for rowIndex, item := range data.Item {
			if item.DateMS == nil || item.OpenPrice == nil || item.HighPrice == nil || item.LowPrice == nil || item.ClosePrice == nil {
				return nil, fmt.Errorf("同花顺日 K 第 %d 行缺少日期或 OHLC", rowIndex)
			}
			tradeDate := time.UnixMilli(*item.DateMS).In(loc).Format("2006-01-02")
			if tradeDate < start.Format("2006-01-02") || tradeDate > end.Format("2006-01-02") {
				return nil, fmt.Errorf("同花顺日 K 日期 %s 超出请求区间", tradeDate)
			}
			if _, duplicate := seen[tradeDate]; duplicate {
				return nil, fmt.Errorf("同花顺分页返回重复交易日 %s", tradeDate)
			}
			seen[tradeDate] = struct{}{}
			open, high, low, closePrice := *item.OpenPrice, *item.HighPrice, *item.LowPrice, *item.ClosePrice
			if !hithinkPositiveFinite(open) || !hithinkPositiveFinite(high) || !hithinkPositiveFinite(low) || !hithinkPositiveFinite(closePrice) ||
				low > high || open < low || open > high || closePrice < low || closePrice > high {
				return nil, fmt.Errorf("同花顺日 K %s 的 OHLC 不满足数据约束", tradeDate)
			}
			var volume *int64
			var rawVolume *float64
			quality := model.QualityValid
			var reasons []string
			if item.Volume == nil || !hithinkNonNegativeFinite(*item.Volume) || math.Trunc(*item.Volume) != *item.Volume || *item.Volume > float64(math.MaxInt64) {
				quality = model.QualityDegraded
				reasons = append(reasons, "volume_missing_or_invalid")
			} else {
				v := int64(*item.Volume)
				volume = &v
				raw := *item.Volume
				rawVolume = &raw
			}
			var amount *float64
			var rawAmount *float64
			amountUnit, rawAmountUnit, rawAmountField := "unknown", "", ""
			if item.Turnover == nil || !hithinkNonNegativeFinite(*item.Turnover) {
				quality = model.QualityDegraded
				reasons = append(reasons, "amount_missing_or_invalid")
			} else {
				v := *item.Turnover
				amount, rawAmount = &v, &v
				amountUnit, rawAmountUnit, rawAmountField = "CNY", "CNY", "turnover"
			}
			out = append(out, model.DailyBar{
				Symbol: symbol, TradeDate: tradeDate, Period: "1d",
				Open: open, High: high, Low: low, Close: closePrice,
				Volume: volume, Amount: amount,
				AdjustmentBasis: model.AdjustmentBasis(hithinkAdjustmentBasis(adjust)),
				AdjustmentMethod: "hithink_api_" + adjust,
				Source: p.Name(), SourceVersion: hithinkVersion,
				FetchedAt: fetchedAt, NormalizedAt: time.Now().UTC(),
				RawVolume: rawVolume, RawVolumeUnit: "shares",
				RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: rawAmountField,
				VolumeUnit: "shares", AmountUnit: amountUnit,
				QualityStatus: quality, QualityReasons: reasons, NormalizationVer: normalize.Version,
			})
			if len(out) > hithinkMaxRowsPerCall {
				return nil, fmt.Errorf("同花顺分页结果超过安全上限 %d 行", hithinkMaxRowsPerCall)
			}
		}
		offset += len(data.Item)
	}
	return nil, fmt.Errorf("同花顺在 %d 页内未结束分页，拒绝返回可能不完整的 K 线", hithinkMaxPagesPerRange)
}

func (p *Hithink) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := p.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	endpoint := p.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	req.Header.Set("X-api-key", p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = readBoundedBody(resp.Body, 4096)
		return nil, fmt.Errorf("HTTP 状态码 %d", resp.StatusCode)
	}
	return readBoundedBody(resp.Body, maxProviderPayloadBytes)
}

func (p *Hithink) waitForRateLimit(ctx context.Context) error {
	p.rateMu.Lock()
	defer p.rateMu.Unlock()
	if !p.lastCall.IsZero() {
		delay := time.Until(p.lastCall.Add(150 * time.Millisecond))
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
			}
		}
	}
	p.lastCall = time.Now()
	return nil
}

func hithinkResponseData(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		Code      *int            `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("解析同花顺响应信封失败: %w", err)
	}
	if envelope.Code == nil {
		return nil, errors.New("同花顺响应缺少 code")
	}
	if *envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = "unspecified provider error"
		}
		if envelope.RequestID != "" {
			return nil, fmt.Errorf("同花顺业务错误 code=%d request_id=%s: %s", *envelope.Code, envelope.RequestID, message)
		}
		return nil, fmt.Errorf("同花顺业务错误 code=%d: %s", *envelope.Code, message)
	}
	if len(strings.TrimSpace(string(envelope.Data))) == 0 || strings.TrimSpace(string(envelope.Data)) == "null" {
		return nil, errors.New("同花顺成功响应缺少 data")
	}
	return envelope.Data, nil
}

func hithinkDomesticSymbol(raw string) (string, error) {
	symbol, err := normalize.NormalizeSymbol(raw)
	if err != nil {
		return "", err
	}
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") {
		return "", fmt.Errorf("同花顺 A 股适配器只支持 SH/SZ/BJ 证券，收到 %q", raw)
	}
	return symbol, nil
}

func hithinkPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hithinkNonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hithinkFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hithinkAdjustmentBasis(adjust string) string {
	switch adjust {
	case "forward":
		return string(model.AdjustmentForward)
	case "backward":
		return string(model.AdjustmentBackward)
	default:
		return string(model.AdjustmentNone)
	}
}
