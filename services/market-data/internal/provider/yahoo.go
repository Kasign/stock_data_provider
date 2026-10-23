package provider

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

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const yahooFinanceVersion = "yahoo-finance-chart-v1"

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
				Timestamp []int64 `json:"timestamp"`
				Indicators struct {
					Quote []struct {
						Open   []*float64 `json:"open"`
						High   []*float64 `json:"high"`
						Low    []*float64 `json:"low"`
						Close  []*float64 `json:"close"`
						Volume []*int64   `json:"volume"`
					} `json:"quote"`
					AdjClose []struct {
						Values []*float64 `json:"adjclose"`
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
	var adjclose []*float64
	if len(result.Indicators.AdjClose) > 0 {
		adjclose = result.Indicators.AdjClose[0].Values
	}

	for index, timestamp := range result.Timestamp {
		if index >= len(quote.Open) || index >= len(quote.High) || index >= len(quote.Low) ||
			index >= len(quote.Close) || index >= len(quote.Volume) {
			rejectedRows++
			continue
		}
		if quote.Open[index] == nil || quote.High[index] == nil || quote.Low[index] == nil ||
			quote.Close[index] == nil || quote.Volume[index] == nil {
			// Null OHLCV points are not trading bars; record omissions for quality diagnostics.
			rejectedRows++
			continue
		}
		openV, highV, lowV, closeV := *quote.Open[index], *quote.High[index], *quote.Low[index], *quote.Close[index]
		volume := *quote.Volume[index]
		if !finiteNumber(openV) || !finiteNumber(highV) || !finiteNumber(lowV) || !finiteNumber(closeV) ||
			openV <= 0 || highV <= 0 || lowV <= 0 || closeV <= 0 || volume < 0 {
			rejectedRows++
			continue
		}
		date := time.Unix(timestamp, 0).In(loc).Format("2006-01-02")
		if date < startISO || date > endISO {
			rejectedRows++
			continue
		}
		factor := 1.0
		if adjustment != model.AdjustmentNone {
			if index >= len(adjclose) || adjclose[index] == nil || !finiteNumber(*adjclose[index]) || *adjclose[index] <= 0 {
				return nil, fmt.Errorf("Yahoo Finance adjusted close missing or invalid at %s", date)
			}
			factor = *adjclose[index] / closeV
			if !finiteNumber(factor) || factor <= 0 {
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
			QualityReasons: []string{"provider_amount_unavailable"},
			NormalizationVer: normalize.Version,
		}
		if adjustment == model.AdjustmentNone {
			bar.AdjustmentRefDate = ""
		}
		bars = append(bars, bar)
	}
	return normalizeProviderDailyBars(symbol, startISO, endISO, limit, bars, rejectedRows)
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
