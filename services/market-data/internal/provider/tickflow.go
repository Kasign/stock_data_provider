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

const tickflowVersion = "tickflow-rest-v1"

type TickFlow struct {
	client       *http.Client
	apiURL       string
	freeAPIURL   string
	apiKey       string
	rateMu       sync.Mutex
	lastFreeCall time.Time
}

type tickflowQuotePayload struct {
	Symbol    string                     `json:"symbol"`
	Name      string                     `json:"name"`
	LastPrice *float64                   `json:"last_price"`
	PrevClose *float64                   `json:"prev_close"`
	Open      *float64                   `json:"open"`
	High      *float64                   `json:"high"`
	Low       *float64                   `json:"low"`
	Volume    *float64                   `json:"volume"`
	Amount    *float64                   `json:"amount"`
	Timestamp int64                      `json:"timestamp"`
	Ext       map[string]json.RawMessage `json:"ext"`
}

func NewTickFlowConfigured(timeout time.Duration, apiURL, freeAPIURL, apiKey string) *TickFlow {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if strings.TrimSpace(apiURL) == "" {
		apiURL = "https://api.tickflow.org"
	}
	if strings.TrimSpace(freeAPIURL) == "" {
		freeAPIURL = "https://free-api.tickflow.org"
	}
	return &TickFlow{
		client:     &http.Client{Timeout: timeout},
		apiURL:     strings.TrimRight(strings.TrimSpace(apiURL), "/"),
		freeAPIURL: strings.TrimRight(strings.TrimSpace(freeAPIURL), "/"),
		apiKey:     strings.TrimSpace(apiKey),
	}
}

func (p *TickFlow) Name() string { return "tickflow" }

func (p *TickFlow) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	if p.apiKey == "" {
		return nil, errors.New("TickFlow real-time quotes require TICKFLOW_API_KEY")
	}
	symbol, err := tickflowDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := p.getJSON(ctx, p.apiURL, "/v1/quotes", url.Values{"symbols": []string{symbol}}, true)
	if err != nil {
		return nil, fmt.Errorf("TickFlow quote request: %w", err)
	}
	data, err := tickflowResponseData(raw)
	if err != nil {
		return nil, err
	}
	var quotes []tickflowQuotePayload
	if err := json.Unmarshal(data, &quotes); err != nil {
		return nil, fmt.Errorf("decode TickFlow quote data: %w", err)
	}
	var q *tickflowQuotePayload
	for i := range quotes {
		if strings.EqualFold(strings.TrimSpace(quotes[i].Symbol), symbol) {
			q = &quotes[i]
			break
		}
	}
	if q == nil {
		return nil, fmt.Errorf("TickFlow quote response did not contain requested symbol %s", symbol)
	}
	if q.LastPrice == nil || !tickflowPositiveFinite(*q.LastPrice) {
		return nil, errors.New("TickFlow quote has missing or invalid last_price")
	}
	if q.PrevClose == nil || !tickflowPositiveFinite(*q.PrevClose) {
		return nil, errors.New("TickFlow quote has missing or invalid prev_close")
	}
	if q.Open == nil || q.High == nil || q.Low == nil ||
		!tickflowPositiveFinite(*q.Open) || !tickflowPositiveFinite(*q.High) || !tickflowPositiveFinite(*q.Low) ||
		*q.Low > *q.High || *q.Open < *q.Low || *q.Open > *q.High || *q.LastPrice < *q.Low || *q.LastPrice > *q.High {
		return nil, errors.New("TickFlow quote violates OHLC invariants")
	}
	if q.Volume == nil || math.IsNaN(*q.Volume) || math.IsInf(*q.Volume, 0) || *q.Volume < 0 || math.Trunc(*q.Volume) != *q.Volume {
		return nil, errors.New("TickFlow quote has invalid volume")
	}
	volume, err := normalize.HandsToShares(*q.Volume)
	if err != nil {
		return nil, fmt.Errorf("normalize TickFlow quote volume: %w", err)
	}
	change := *q.LastPrice - *q.PrevClose
	changePercent := change / *q.PrevClose * 100
	now := time.Now().UTC()
	var quoteTime *time.Time
	quality := model.QualityValid
	var reasons []string
	if q.Timestamp > 0 {
		value := time.UnixMilli(q.Timestamp).UTC()
		quoteTime = &value
	} else {
		quality = model.QualityDegraded
		reasons = append(reasons, "provider_quote_timestamp_missing")
	}
	var amount, rawAmount *float64
	amountUnit, rawAmountUnit, amountField := "unknown", "", ""
	if q.Amount != nil && !math.IsNaN(*q.Amount) && !math.IsInf(*q.Amount, 0) && *q.Amount >= 0 {
		value := *q.Amount
		amount, rawAmount = &value, &value
		amountUnit, rawAmountUnit, amountField = "CNY", "CNY", "amount"
	} else {
		quality = model.QualityDegraded
		reasons = append(reasons, "amount_missing_from_provider", "amount_unit_unknown_because_amount_missing")
	}
	rawVolume := *q.Volume
	var turnover *float64
	if raw, ok := q.Ext["turnover_rate"]; ok {
		if value, parseErr := tickflowFloat(raw); parseErr == nil && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0) {
			value *= 100
			turnover = &value
		}
	}
	return &model.Quote{
		Symbol: symbol, Name: strings.TrimSpace(q.Name),
		CurrentPrice: q.LastPrice, Change: &change, ChangePercent: &changePercent,
		Open: q.Open, High: q.High, Low: q.Low, PrevClose: q.PrevClose,
		Volume: &volume, Amount: amount, TurnoverRate: turnover,
		QuoteTime: quoteTime, QuoteTimeSemantics: "provider_snapshot_time",
		FetchedAt: now, NormalizedAt: now, Source: p.Name(), SourceVersion: tickflowVersion,
		RawVolume: &rawVolume, RawVolumeUnit: "hands",
		RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: amountField,
		VolumeUnit: "shares", AmountUnit: amountUnit,
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: quality, QualityReasons: reasons,
		NormalizationVer: normalize.Version,
	}, nil
}

func (p *TickFlow) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("TickFlow HTTP client is not configured")
	}
	symbol, err := tickflowDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("unsupported TickFlow adjustment %q", adjustment)
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil {
		return nil, fmt.Errorf("invalid TickFlow start date %q", startDate)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil {
		return nil, fmt.Errorf("invalid TickFlow end date %q", endDate)
	}
	if start.After(end) {
		return nil, errors.New("TickFlow start date must not be after end date")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if end.Format("2006-01-02") > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("TickFlow end date must not be after today in Asia/Shanghai")
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 2000 {
		return nil, errors.New("TickFlow daily-bar limit must not exceed 2000")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	baseURL, authenticated := p.freeAPIURL, false
	if p.apiKey != "" {
		baseURL, authenticated = p.apiURL, true
	} else {
		if err := p.waitForFreeRateLimit(ctx); err != nil {
			return nil, err
		}
	}
	startMS := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc).UnixMilli()
	endMS := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), loc).UnixMilli()
	params := url.Values{
		"symbol":     []string{symbol},
		"period":     []string{"1d"},
		"count":      []string{strconv.Itoa(limit)},
		"start_time": []string{strconv.FormatInt(startMS, 10)},
		"end_time":   []string{strconv.FormatInt(endMS, 10)},
		"adjust":     []string{adjust},
	}
	raw, err := p.getJSON(ctx, baseURL, "/v1/klines", params, authenticated)
	if err != nil {
		return nil, fmt.Errorf("TickFlow daily K-line request: %w", err)
	}
	dataRaw, err := tickflowResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp []json.RawMessage `json:"timestamp"`
		Open      []json.RawMessage `json:"open"`
		High      []json.RawMessage `json:"high"`
		Low       []json.RawMessage `json:"low"`
		Close     []json.RawMessage `json:"close"`
		Volume    []json.RawMessage `json:"volume"`
		Amount    []json.RawMessage `json:"amount"`
		PrevClose []json.RawMessage `json:"prev_close"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("decode TickFlow K-line data: %w", err)
	}
	n := len(data.Timestamp)
	if n == 0 {
		return nil, errors.New("TickFlow returned no daily K-line rows")
	}
	for name, length := range map[string]int{"open": len(data.Open), "high": len(data.High), "low": len(data.Low), "close": len(data.Close), "volume": len(data.Volume)} {
		if length != n {
			return nil, fmt.Errorf("TickFlow K-line %s length %d does not match timestamp length %d", name, length, n)
		}
	}
	fetchedAt := time.Now().UTC()
	out := make([]model.DailyBar, 0, n)
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		timestamp, err := tickflowInt64(data.Timestamp[i])
		if err != nil || timestamp <= 0 {
			return nil, fmt.Errorf("TickFlow K-line row %d has invalid timestamp", i)
		}
		tradeDate := time.UnixMilli(timestamp).In(loc).Format("2006-01-02")
		if tradeDate < start.Format("2006-01-02") || tradeDate > end.Format("2006-01-02") {
			return nil, fmt.Errorf("TickFlow K-line row %d date %s is outside requested range", i, tradeDate)
		}
		if _, duplicate := seen[tradeDate]; duplicate {
			return nil, fmt.Errorf("TickFlow returned duplicate trade date %s", tradeDate)
		}
		seen[tradeDate] = struct{}{}
		open, err := tickflowFloat(data.Open[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid open on %s", tradeDate)
		}
		high, err := tickflowFloat(data.High[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid high on %s", tradeDate)
		}
		low, err := tickflowFloat(data.Low[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid low on %s", tradeDate)
		}
		closePrice, err := tickflowFloat(data.Close[i])
		if err != nil {
			return nil, fmt.Errorf("TickFlow invalid close on %s", tradeDate)
		}
		if !tickflowPositiveFinite(open) || !tickflowPositiveFinite(high) || !tickflowPositiveFinite(low) || !tickflowPositiveFinite(closePrice) ||
			low > high || open < low || open > high || closePrice < low || closePrice > high {
			return nil, fmt.Errorf("TickFlow OHLC invariant failed on %s", tradeDate)
		}
		rawVolume, err := tickflowFloat(data.Volume[i])
		if err != nil || rawVolume < 0 || math.Trunc(rawVolume) != rawVolume {
			return nil, fmt.Errorf("TickFlow has invalid volume on %s", tradeDate)
		}
		volume, err := normalize.HandsToShares(rawVolume)
		if err != nil {
			return nil, fmt.Errorf("normalize TickFlow volume on %s: %w", tradeDate, err)
		}

		var amount, rawAmount *float64
		amountUnit, rawAmountUnit, rawAmountField := "unknown", "", ""
		quality := model.QualityValid
		var reasons []string
		if i < len(data.Amount) && len(bytesTrim(data.Amount[i])) > 0 && string(bytesTrim(data.Amount[i])) != "null" {
			value, parseErr := tickflowFloat(data.Amount[i])
			if parseErr != nil || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) {
				return nil, fmt.Errorf("TickFlow has invalid amount on %s", tradeDate)
			}
			amount, rawAmount = &value, &value
			amountUnit, rawAmountUnit, rawAmountField = "CNY", "CNY", "amount"
		} else {
			quality = model.QualityDegraded
			reasons = append(reasons, "amount_missing_from_provider", "amount_unit_unknown_because_amount_missing")
		}
		var pct *float64
		if i < len(data.PrevClose) {
			prev, parseErr := tickflowFloat(data.PrevClose[i])
			if parseErr == nil && prev > 0 {
				value := (closePrice - prev) / prev * 100
				pct = &value
			}
		} else if i > 0 {
			prev, parseErr := tickflowFloat(data.Close[i-1])
			if parseErr == nil && prev > 0 {
				value := (closePrice - prev) / prev * 100
				pct = &value
			}
		}
		if pct == nil {
			quality = model.QualityDegraded
			reasons = append(reasons, "provider_change_percent_unavailable_for_first_bar")
		}
		rawVolumeCopy := rawVolume
		out = append(out, model.DailyBar{
			Symbol: symbol, TradeDate: tradeDate, Period: "1d",
			Open: open, High: high, Low: low, Close: closePrice, Volume: &volume,
			Amount: amount, ChangePercent: pct, AdjustmentBasis: adjustment,
			AdjustmentMethod: "tickflow_api_" + adjust,
			Source:           p.Name(), SourceVersion: tickflowVersion,
			FetchedAt: fetchedAt, NormalizedAt: time.Now().UTC(),
			RawVolume: &rawVolumeCopy, RawVolumeUnit: "hands",
			RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: rawAmountField,
			VolumeUnit: "shares", AmountUnit: amountUnit, QualityStatus: quality, QualityReasons: reasons,
			NormalizationVer: normalize.Version,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TradeDate < out[j].TradeDate })
	return normalizeProviderDailyBars(symbol, start.Format("2006-01-02"), end.Format("2006-01-02"), limit, out, 0)
}

func (p *TickFlow) getJSON(ctx context.Context, baseURL, path string, query url.Values, authenticated bool) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	endpoint := strings.TrimRight(strings.TrimSpace(baseURL), "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	if authenticated && p.apiKey != "" {
		req.Header.Set("x-api-key", p.apiKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP status %d", resp.StatusCode)
	}
	return readBoundedBody(resp.Body, maxProviderPayloadBytes)
}

func (p *TickFlow) waitForFreeRateLimit(ctx context.Context) error {
	p.rateMu.Lock()
	defer p.rateMu.Unlock()
	if !p.lastFreeCall.IsZero() {
		delay := time.Until(p.lastFreeCall.Add(time.Second))
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
	p.lastFreeCall = time.Now()
	return nil
}

func tickflowDomesticSymbol(raw string) (string, error) {
	symbol, err := normalize.NormalizeSymbol(raw)
	if err != nil {
		return "", err
	}
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") {
		return "", fmt.Errorf("TickFlow Go adapter only accepts domestic SH/SZ/BJ securities, got %q", raw)
	}
	return symbol, nil
}

func tickflowResponseData(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		Code    *int            `json:"code"`
		Message string          `json:"message"`
		Msg     string          `json:"msg"`
		Error   string          `json:"error"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("decode TickFlow response envelope: %w", err)
	}
	if envelope.Code != nil && *envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = strings.TrimSpace(envelope.Msg)
		}
		if message == "" {
			message = strings.TrimSpace(envelope.Error)
		}
		return nil, fmt.Errorf("TickFlow provider error code=%d: %s", *envelope.Code, message)
	}
	if strings.TrimSpace(envelope.Error) != "" {
		return nil, fmt.Errorf("TickFlow provider error: %s", strings.TrimSpace(envelope.Error))
	}
	if len(bytesTrim(envelope.Data)) == 0 || string(bytesTrim(envelope.Data)) == "null" {
		msg := strings.TrimSpace(envelope.Message)
		if msg == "" {
			msg = strings.TrimSpace(envelope.Msg)
		}
		return nil, fmt.Errorf("TickFlow response has no data: %s", msg)
	}
	return envelope.Data, nil
}

func tickflowFloat(raw json.RawMessage) (float64, error) {
	raw = json.RawMessage(bytesTrim(raw))
	if len(raw) == 0 || string(raw) == "null" {
		return 0, errors.New("empty TickFlow number")
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if strings.TrimSpace(s) == "" {
			return 0, errors.New("empty TickFlow number")
		}
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}
	return strconv.ParseFloat(string(raw), 64)
}

func tickflowInt64(raw json.RawMessage) (int64, error) {
	value, err := tickflowFloat(raw)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value ||
		value < -float64(math.MaxInt64) || value >= float64(math.MaxInt64) {
		return 0, errors.New("invalid TickFlow integer")
	}
	return int64(value), nil
}

func tickflowPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func bytesTrim(raw []byte) []byte {
	return []byte(strings.TrimSpace(string(raw)))
}
