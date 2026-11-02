package provider

import (
	"bytes"
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

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/validation"
)

const yahooFinanceVersion = "yahoo-finance-chart-v1"
const yahooFinanceQuoteVersion = "yahoo-finance-chart-quote-v1"

// YahooFinance 使用 Yahoo Finance 公开 Chart JSON endpoint，仅提供历史日 K。
// 该 endpoint 并非有 SLA 的官方开发者 API；配置默认关闭，成交额缺失必须降级。
type YahooFinance struct {
	client   *http.Client
	chartURL string
}

func NewYahooFinance(timeout time.Duration, chartURL string) *YahooFinance {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if strings.TrimSpace(chartURL) == "" {
		chartURL = "https://query1.finance.yahoo.com/v8/finance/chart"
	}
	return &YahooFinance{
		client: &http.Client{
			Timeout:       timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
		},
		chartURL: strings.TrimRight(strings.TrimSpace(chartURL), "/"),
	}
}

func (p *YahooFinance) Name() string { return "yahoo" }

// FetchQuote uses Yahoo's public chart endpoint as an optional intraday snapshot.
// It is not an official SLA-backed API; because turnover amount is unavailable,
// returned quotes are always degraded and this provider is disabled by default.
func (p *YahooFinance) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	ticker, err := yahooFinanceTicker(symbol)
	if err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(strings.TrimSpace(p.chartURL), "/") + "/" + url.PathEscape(ticker)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Yahoo Finance quote endpoint: %w", err)
	}
	query := u.Query()
	query.Set("range", "1d")
	query.Set("interval", "1m")
	query.Set("events", "div,splits")
	query.Set("includeAdjustedClose", "true")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Yahoo Finance quote request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://finance.yahoo.com/")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Yahoo Finance quote request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Yahoo Finance quote HTTP status %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("read Yahoo Finance quote payload: %w", err)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol               string  `json:"symbol"`
					LongName             string  `json:"longName"`
					ShortName            string  `json:"shortName"`
					RegularMarketTime    int64   `json:"regularMarketTime"`
					RegularMarketPrice   float64 `json:"regularMarketPrice"`
					RegularMarketDayHigh float64 `json:"regularMarketDayHigh"`
					RegularMarketDayLow  float64 `json:"regularMarketDayLow"`
					RegularMarketVolume  *int64  `json:"regularMarketVolume"`
					ChartPreviousClose   float64 `json:"chartPreviousClose"`
				} `json:"meta"`
				Timestamp  []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open []*float64 `json:"open"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}
	if err := decodeSingleJSON(body, &payload); err != nil {
		return nil, fmt.Errorf("decode Yahoo Finance quote JSON: %w", err)
	}
	if payload.Chart.Error != nil {
		return nil, fmt.Errorf("Yahoo Finance quote provider error %s: %s", payload.Chart.Error.Code, payload.Chart.Error.Description)
	}
	if len(payload.Chart.Result) != 1 {
		return nil, errors.New("Yahoo Finance returned no single quote result")
	}
	result := payload.Chart.Result[0]
	if reported := strings.TrimSpace(result.Meta.Symbol); reported != "" && !strings.EqualFold(reported, ticker) {
		return nil, fmt.Errorf("Yahoo Finance quote ticker mismatch: requested %s got %q", ticker, reported)
	}
	meta := result.Meta
	if meta.RegularMarketTime <= 0 || !finiteNumber(meta.RegularMarketPrice) || meta.RegularMarketPrice <= 0 ||
		!finiteNumber(meta.RegularMarketDayHigh) || meta.RegularMarketDayHigh <= 0 ||
		!finiteNumber(meta.RegularMarketDayLow) || meta.RegularMarketDayLow <= 0 ||
		!finiteNumber(meta.ChartPreviousClose) || meta.ChartPreviousClose <= 0 {
		return nil, errors.New("Yahoo Finance quote is missing required finite price metadata")
	}
	if meta.RegularMarketVolume == nil || *meta.RegularMarketVolume < 0 {
		return nil, errors.New("Yahoo Finance quote has missing or invalid regular-market volume")
	}
	if meta.RegularMarketDayLow > meta.RegularMarketDayHigh {
		return nil, errors.New("Yahoo Finance quote day low is greater than day high")
	}

	loc := yahooChinaLocation()
	quoteTime := time.Unix(meta.RegularMarketTime, 0).UTC()
	quoteDay := quoteTime.In(loc).Format("2006-01-02")
	openIndex := -1
	var openPrice float64
	if len(result.Indicators.Quote) > 0 {
		openValues := result.Indicators.Quote[0].Open
		for i, timestamp := range result.Timestamp {
			if i >= len(openValues) || openValues[i] == nil {
				continue
			}
			if time.Unix(timestamp, 0).In(loc).Format("2006-01-02") != quoteDay {
				continue
			}
			value := *openValues[i]
			if !finiteNumber(value) || value <= 0 {
				continue
			}
			if openIndex < 0 || timestamp < result.Timestamp[openIndex] {
				openIndex = i
				openPrice = value
			}
		}
	}
	if openIndex < 0 {
		return nil, fmt.Errorf("Yahoo Finance quote has no valid intraday open for provider quote date %s", quoteDay)
	}
	change := meta.RegularMarketPrice - meta.ChartPreviousClose
	changePercent := change / meta.ChartPreviousClose * 100
	if !finiteNumber(changePercent) || math.Abs(changePercent) > 1000 {
		return nil, errors.New("Yahoo Finance quote percent change is outside a plausible numeric range")
	}

	var volume *int64
	var rawVolume *float64
	if meta.RegularMarketVolume != nil && *meta.RegularMarketVolume >= 0 {
		v := *meta.RegularMarketVolume
		volume = &v
		raw := float64(v)
		rawVolume = &raw
	}
	name := strings.TrimSpace(meta.LongName)
	if name == "" {
		name = strings.TrimSpace(meta.ShortName)
	}
	now := time.Now().UTC()
	q := &model.Quote{
		Symbol: symbol, Name: name,
		CurrentPrice: &meta.RegularMarketPrice, Change: &change, ChangePercent: &changePercent,
		Open: &openPrice, High: &meta.RegularMarketDayHigh, Low: &meta.RegularMarketDayLow,
		PrevClose: &meta.ChartPreviousClose, Volume: volume, Amount: nil,
		QuoteTime: &quoteTime, QuoteTimeSemantics: "provider_snapshot_time",
		FetchedAt: now, NormalizedAt: now, Source: p.Name(), SourceVersion: yahooFinanceQuoteVersion,
		RawVolume: rawVolume, RawVolumeUnit: optionalVolumeUnit(volume, "shares"),
		RawAmountUnit: "unknown",
		VolumeUnit:    optionalVolumeUnit(volume, "shares"), AmountUnit: "unknown",
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_amount_unavailable"}, NormalizationVer: normalize.Version,
	}
	if name == "" {
		q.QualityReasons = append(q.QualityReasons, "provider_name_missing")
	}
	report, err := normalize.ValidateQuoteWithReport(q)
	validation.AddContextReport(ctx, report)
	if err != nil {
		return nil, fmt.Errorf("Yahoo Finance quote data contract: %w", err)
	}
	return q, nil
}

func (p *YahooFinance) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	yahooTicker, err := yahooFinanceTicker(symbol)
	if err != nil {
		return nil, err
	}
	switch adjustment {
	case model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward:
	default:
		return nil, fmt.Errorf("Yahoo Finance does not support adjustment basis %q", adjustment)
	}

	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	start, end, err := yahooDateRange(startDate, endDate, limit, time.Now())
	if err != nil {
		return nil, err
	}

	endExclusive := end.AddDate(0, 0, 1)
	endpoint := p.chartURL + "/" + url.PathEscape(yahooTicker)
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse Yahoo Finance chart endpoint: %w", err)
	}
	query := u.Query()
	query.Set("period1", strconv.FormatInt(start.Unix(), 10))
	query.Set("period2", strconv.FormatInt(endExclusive.Unix(), 10))
	query.Set("interval", "1d")
	query.Set("events", "div,splits")
	query.Set("includeAdjustedClose", "true")
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Yahoo Finance request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Referer", "https://finance.yahoo.com/")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Yahoo Finance chart request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Yahoo Finance chart HTTP status %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("read Yahoo Finance chart payload: %w", err)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Meta struct {
					Symbol string `json:"symbol"`
				} `json:"meta"`
				Timestamp  []json.RawMessage `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []json.RawMessage `json:"open"`
						High   []json.RawMessage `json:"high"`
						Low    []json.RawMessage `json:"low"`
						Close  []json.RawMessage `json:"close"`
						Volume []json.RawMessage `json:"volume"`
					} `json:"quote"`
					AdjClose []struct {
						Values []json.RawMessage `json:"adjclose"`
					} `json:"adjclose"`
				} `json:"indicators"`
			} `json:"result"`
			Error *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"chart"`
	}
	if err := decodeSingleJSON(body, &payload); err != nil {
		addProviderParseIssue(ctx, -1, "chart_response", "chart.result[0].indicators.quote[0]", "decode_error", "valid Yahoo Finance chart JSON with numeric OHLCV arrays", providerJSONValueType(body), "", "Yahoo Finance chart response could not be decoded against the expected field contract")
		return nil, fmt.Errorf("decode Yahoo Finance chart JSON: %w", err)
	}
	if payload.Chart.Error != nil {
		return nil, fmt.Errorf("Yahoo Finance chart provider error %s: %s", payload.Chart.Error.Code, payload.Chart.Error.Description)
	}
	if len(payload.Chart.Result) != 1 {
		return nil, errors.New("Yahoo Finance returned no single chart result")
	}
	result := payload.Chart.Result[0]
	if reported := strings.TrimSpace(result.Meta.Symbol); reported != "" && !strings.EqualFold(reported, yahooTicker) {
		return nil, fmt.Errorf("Yahoo Finance ticker mismatch: requested %s got %q", yahooTicker, reported)
	}
	if len(result.Timestamp) == 0 || len(result.Indicators.Quote) == 0 {
		return nil, errors.New("Yahoo Finance returned empty daily bars")
	}
	quote := result.Indicators.Quote[0]
	if adjustment != model.AdjustmentNone && len(result.Indicators.AdjClose) == 0 {
		return nil, errors.New("Yahoo Finance adjusted-close series is required for an adjusted request")
	}

	loc := yahooChinaLocation()
	startISO, endISO := start.Format("2006-01-02"), end.Format("2006-01-02")
	type rawYahooBar struct {
		date                   string
		open, high, low, close float64
		volume                 int64
		factor                 float64
	}
	rawBars := make([]rawYahooBar, 0, len(result.Timestamp))
	rejectedRows := 0
	var adjclose []json.RawMessage
	if len(result.Indicators.AdjClose) > 0 {
		adjclose = result.Indicators.AdjClose[0].Values
	}

	for index, rawTimestamp := range result.Timestamp {
		timestamp, ok := parseYahooInt64Field(ctx, index, "timestamp", "timestamp", rawTimestamp, "Unix timestamp in seconds")
		if !ok || timestamp <= 0 {
			if ok {
				addProviderParseIssue(ctx, index, "trade_date", "timestamp", "invalid_timestamp", "positive Unix timestamp in seconds", "number", providerJSONPreview(rawTimestamp), "Yahoo Finance 日 K 时间戳必须为正整数")
			}
			rejectedRows++
			continue
		}
		date := time.Unix(timestamp, 0).In(loc).Format("2006-01-02")
		if date < startISO || date > endISO {
			rejectedRows++
			continue
		}
		openV, openOK := parseYahooFloatField(ctx, index, "open", "indicators.quote[0].open", rawYahooArrayValue(quote.Open, index), "finite positive price")
		highV, highOK := parseYahooFloatField(ctx, index, "high", "indicators.quote[0].high", rawYahooArrayValue(quote.High, index), "finite positive price")
		lowV, lowOK := parseYahooFloatField(ctx, index, "low", "indicators.quote[0].low", rawYahooArrayValue(quote.Low, index), "finite positive price")
		closeV, closeOK := parseYahooFloatField(ctx, index, "close", "indicators.quote[0].close", rawYahooArrayValue(quote.Close, index), "finite positive price")
		volume, volumeOK := parseYahooInt64Field(ctx, index, "volume", "indicators.quote[0].volume", rawYahooArrayValue(quote.Volume, index), "non-negative integer shares")
		if !openOK || !highOK || !lowOK || !closeOK || !volumeOK {
			rejectedRows++
			continue
		}
		if volume < 0 {
			addProviderParseIssue(ctx, index, "volume", "indicators.quote[0].volume", "out_of_range", "non-negative integer shares", "number", fmt.Sprintf("%d", volume), "Yahoo Finance 成交量不能为负数")
			rejectedRows++
			continue
		}
		if openV <= 0 || highV <= 0 || lowV <= 0 || closeV <= 0 {
			for field, value := range map[string]float64{"open": openV, "high": highV, "low": lowV, "close": closeV} {
				if value <= 0 {
					addProviderParseIssue(ctx, index, field, "indicators.quote[0]."+field, "out_of_range", "positive price", "number", fmt.Sprintf("%g", value), "Yahoo Finance 日 K 价格必须大于零")
				}
			}
			rejectedRows++
			continue
		}
		factor := 1.0
		if adjustment != model.AdjustmentNone {
			adjRaw := rawYahooArrayValue(adjclose, index)
			adjValue, adjOK := parseYahooFloatField(ctx, index, "adjusted_close", "indicators.adjclose[0].adjclose", adjRaw, "finite positive adjusted close")
			if !adjOK || adjValue <= 0 {
				if adjOK {
					addProviderParseIssue(ctx, index, "adjusted_close", "indicators.adjclose[0].adjclose", "out_of_range", "positive adjusted close", "number", fmt.Sprintf("%g", adjValue), "Yahoo Finance 复权收盘价必须大于零")
				}
				return nil, fmt.Errorf("Yahoo Finance adjusted close missing or invalid at %s", date)
			}
			factor = adjValue / closeV
			if !finiteNumber(factor) || factor <= 0 {
				addProviderParseIssue(ctx, index, "adjustment_factor", "adjclose/close", "invalid_adjustment_factor", "finite positive adjustment factor", "number", fmt.Sprintf("%g", factor), "Yahoo Finance 复权因子无效")
				return nil, fmt.Errorf("Yahoo Finance adjustment factor invalid at %s", date)
			}
		}
		rawBars = append(rawBars, rawYahooBar{
			date: date, open: openV, high: highV, low: lowV, close: closeV,
			volume: volume, factor: factor,
		})
	}
	if len(rawBars) == 0 {
		return nil, errors.New("Yahoo Finance returned no valid daily bars in requested date range")
	}

	// Yahoo's adjclose ratio is referenced to the latest available series date.
	// Rebase it to this request's end/start date to prevent future corporate actions
	// from silently changing the reference level for an explicitly historical query.
	referenceDate := ""
	referenceFactor := 1.0
	adjustmentMethod := "yahoo_raw_ohlc"
	switch adjustment {
	case model.AdjustmentForward:
		referenceDate = rawBars[len(rawBars)-1].date
		referenceFactor = rawBars[len(rawBars)-1].factor
		adjustmentMethod = "yahoo_adjclose_ratio_rebased_to_end"
	case model.AdjustmentBackward:
		referenceDate = rawBars[0].date
		referenceFactor = rawBars[0].factor
		adjustmentMethod = "yahoo_adjclose_ratio_rebased_to_start"
	}
	if referenceFactor == 0 || math.IsNaN(referenceFactor) || math.IsInf(referenceFactor, 0) {
		return nil, errors.New("Yahoo Finance reference adjustment factor is invalid")
	}

	now := time.Now().UTC()
	bars := make([]model.DailyBar, 0, len(rawBars))
	for _, raw := range rawBars {
		factor := 1.0
		if adjustment != model.AdjustmentNone {
			factor = raw.factor / referenceFactor
		}
		if !finiteNumber(factor) || factor <= 0 {
			rejectedRows++
			continue
		}
		volume := raw.volume
		rawVolume := float64(raw.volume)
		bar := model.DailyBar{
			Symbol: symbol, TradeDate: raw.date, Period: "1d",
			Open: raw.open * factor, High: raw.high * factor,
			Low: raw.low * factor, Close: raw.close * factor,
			Volume: &volume, Amount: nil,
			AdjustmentBasis: adjustment, AdjustmentMethod: adjustmentMethod,
			AdjustmentRefDate: referenceDate, Source: p.Name(), SourceVersion: yahooFinanceVersion,
			FetchedAt: now, NormalizedAt: now, RawVolume: &rawVolume,
			RawVolumeUnit: "shares", RawAmountUnit: "unknown", VolumeUnit: "shares",
			AmountUnit: "unknown", QualityStatus: model.QualityDegraded,
			QualityReasons:   []string{"provider_amount_unavailable"},
			NormalizationVer: normalize.Version,
		}
		if adjustment == model.AdjustmentNone {
			bar.AdjustmentRefDate = ""
		}
		bars = append(bars, bar)
	}
	return normalizeProviderDailyBarsWithReport(ctx, symbol, startISO, endISO, limit, bars, rejectedRows)
}


func rawYahooArrayValue(values []json.RawMessage, index int) json.RawMessage {
	if index < 0 || index >= len(values) {
		return nil
	}
	return values[index]
}

func parseYahooFloatField(ctx context.Context, rowIndex int, field, providerField string, raw json.RawMessage, expected string) (float64, bool) {
	valueType := providerJSONValueType(raw)
	if valueType == "missing" || valueType == "null" {
		code := "missing_field"
		if valueType == "null" {
			code = "null_value"
		}
		addProviderParseIssue(ctx, rowIndex, field, providerField, code, expected, valueType, "", "Yahoo Finance 日 K 字段缺失或为空")
		return 0, false
	}
	if valueType != "number" {
		addProviderParseIssue(ctx, rowIndex, field, providerField, "type_mismatch", expected, valueType, providerJSONPreview(raw), "Yahoo Finance 日 K 字段类型不符合数值契约")
		return 0, false
	}
	var value float64
	if err := json.Unmarshal(raw, &value); err != nil || !finiteNumber(value) {
		addProviderParseIssue(ctx, rowIndex, field, providerField, "invalid_number", expected, valueType, providerJSONPreview(raw), "Yahoo Finance 日 K 字段不是有限数值")
		return 0, false
	}
	return value, true
}

func parseYahooInt64Field(ctx context.Context, rowIndex int, field, providerField string, raw json.RawMessage, expected string) (int64, bool) {
	valueType := providerJSONValueType(raw)
	if valueType == "missing" || valueType == "null" {
		code := "missing_field"
		if valueType == "null" {
			code = "null_value"
		}
		addProviderParseIssue(ctx, rowIndex, field, providerField, code, expected, valueType, "", "Yahoo Finance 日 K 字段缺失或为空")
		return 0, false
	}
	if valueType != "number" {
		addProviderParseIssue(ctx, rowIndex, field, providerField, "type_mismatch", expected, valueType, providerJSONPreview(raw), "Yahoo Finance 日 K 字段类型不符合整数契约")
		return 0, false
	}
	value, err := strconv.ParseInt(string(bytes.TrimSpace(raw)), 10, 64)
	if err != nil {
		addProviderParseIssue(ctx, rowIndex, field, providerField, "invalid_integer", expected, valueType, providerJSONPreview(raw), "Yahoo Finance 日 K 字段不是有效 64 位整数")
		return 0, false
	}
	return value, true
}


func yahooFinanceTicker(symbol string) (string, error) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("unsupported Yahoo Finance symbol %q", symbol)
	}
	switch parts[1] {
	case "SH":
		return parts[0] + ".SS", nil
	case "SZ":
		return parts[0] + ".SZ", nil
	default:
		return "", fmt.Errorf("Yahoo Finance does not support A-share exchange %q", parts[1])
	}
}

func yahooChinaLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

func yahooDateRange(startDate, endDate string, limit int, now time.Time) (time.Time, time.Time, error) {
	loc := yahooChinaLocation()
	localNow := now.In(loc)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc)
	if strings.TrimSpace(endDate) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid Yahoo Finance end date: %w", err)
		}
		end = parsed
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	start := end.AddDate(0, 0, -(limit*4 + 30))
	if strings.TrimSpace(startDate) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid Yahoo Finance start date: %w", err)
		}
		start = parsed
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, errors.New("Yahoo Finance start date must not be after end date")
	}
	return start, end, nil
}
