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
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const tushareVersion = "tushare-pro-daily-v1"

// Tushare 通过 HTTP JSON 接口读取 A 股日 K。需要用户自己的 Token，默认关闭。
// daily 接口返回原始 OHLC；不能满足请求复权口径时明确失败，不伪装复权数据。
type Tushare struct {
	client   *http.Client
	endpoint string
	token    string
	rateMu   sync.Mutex
	lastCall time.Time
}

func NewTushareConfigured(timeout time.Duration, endpoint, token string) *Tushare {
	if timeout <= 0 { timeout = 12 * time.Second }
	if strings.TrimSpace(endpoint) == "" { endpoint = "https://api.tushare.pro" }
	return &Tushare{client: &http.Client{Timeout: timeout}, endpoint: strings.TrimSpace(endpoint), token: strings.TrimSpace(token)}
}

func (p *Tushare) Name() string { return "tushare" }

func (p *Tushare) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	if p == nil || p.client == nil { return nil, errors.New("Tushare HTTP client is not configured") }
	if p.token == "" { return nil, errors.New("Tushare token is not configured") }
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil { return nil, err }
	if adjustment != model.AdjustmentNone {
		return nil, fmt.Errorf("Tushare daily endpoint only provides unadjusted OHLC; requested adjustment %q is unsupported", adjustment)
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil { return nil, fmt.Errorf("invalid Tushare start date %q", startDate) }
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil { return nil, fmt.Errorf("invalid Tushare end date %q", endDate) }
	if start.After(end) { return nil, errors.New("Tushare start date must not be after end date") }
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil { loc = time.FixedZone("CST", 8*60*60) }
	if endDate > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("Tushare end date must not be after today in Asia/Shanghai")
	}
	if limit <= 0 { limit = 120 }
	if limit > 2000 { return nil, errors.New("Tushare daily-bar limit must not exceed 2000") }
	if ctx == nil { ctx = context.Background() }
	if err := ctx.Err(); err != nil { return nil, err }
	if err := p.waitForRateLimit(ctx); err != nil { return nil, err }

	payload := map[string]any{
		"api_name": "daily",
		"token": p.token,
		"params": map[string]string{"ts_code": symbol, "start_date": start.Format("20060102"), "end_date": end.Format("20060102")},
		"fields": "ts_code,trade_date,open,high,low,close,pct_chg,vol,amount",
	}
	body, err := json.Marshal(payload)
	if err != nil { return nil, fmt.Errorf("encode Tushare daily request: %w", err) }
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil { return nil, fmt.Errorf("create Tushare daily request: %w", err) }
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	resp, err := p.client.Do(req)
	if err != nil { return nil, fmt.Errorf("Tushare daily request: %w", err) }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Tushare HTTP status %d", resp.StatusCode)
	}
	raw, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil { return nil, fmt.Errorf("read Tushare daily response: %w", err) }
	var envelope struct {
		Code *int `json:"code"`
		Msg string `json:"msg"`
		Data *struct {
			Fields []string `json:"fields"`
			Items [][]json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil { return nil, fmt.Errorf("decode Tushare daily response: %w", err) }
	if envelope.Code == nil { return nil, errors.New("Tushare response is missing code") }
	if *envelope.Code != 0 {
		msg := strings.TrimSpace(envelope.Msg)
		if msg == "" { msg = "unspecified provider error" }
		return nil, fmt.Errorf("Tushare provider error code=%d: %s", *envelope.Code, msg)
	}
	if envelope.Data == nil { return nil, errors.New("Tushare response is missing data") }
	indices := make(map[string]int, len(envelope.Data.Fields))
	for i, field := range envelope.Data.Fields { indices[field] = i }
	for _, required := range []string{"ts_code", "trade_date", "open", "high", "low", "close", "vol"} {
		if _, ok := indices[required]; !ok { return nil, fmt.Errorf("Tushare daily response is missing field %q", required) }
	}
	if len(envelope.Data.Items) == 0 { return nil, errors.New("Tushare returned no daily K-line rows") }

	get := func(row []json.RawMessage, field string, rowIndex int) (json.RawMessage, error) {
		index, ok := indices[field]
		if !ok { return nil, nil }
		if index >= len(row) { return nil, fmt.Errorf("Tushare row %d is missing value for field %q", rowIndex, field) }
		return row[index], nil
	}
	fetchedAt := time.Now().UTC()
	out := make([]model.DailyBar, 0, len(envelope.Data.Items))
	seenDates := make(map[string]struct{}, len(envelope.Data.Items))
	for rowIndex, row := range envelope.Data.Items {
		codeRaw, err := get(row, "ts_code", rowIndex)
		if err != nil { return nil, err }
		code, err := tushareString(codeRaw)
		if err != nil || strings.ToUpper(strings.TrimSpace(code)) != symbol {
			return nil, fmt.Errorf("Tushare row %d symbol mismatch: requested %s, returned %q", rowIndex, symbol, code)
		}
		dateRaw, err := get(row, "trade_date", rowIndex)
		if err != nil { return nil, err }
		dateText, err := tushareString(dateRaw)
		if err != nil { return nil, fmt.Errorf("Tushare row %d has an invalid trade date", rowIndex) }
		tradeDate, err := time.Parse("20060102", dateText)
		if err != nil { tradeDate, err = time.Parse("2006-01-02", dateText) }
		if err != nil { return nil, fmt.Errorf("Tushare row %d has invalid trade date %q", rowIndex, dateText) }
		isoDate := tradeDate.Format("2006-01-02")
		if isoDate < start.Format("2006-01-02") || isoDate > end.Format("2006-01-02") {
			return nil, fmt.Errorf("Tushare row %d date %s is outside requested range", rowIndex, isoDate)
		}
		if _, duplicate := seenDates[isoDate]; duplicate { return nil, fmt.Errorf("Tushare returned duplicate trade date %s", isoDate) }
		seenDates[isoDate] = struct{}{}

		readFloat := func(field string, positive bool) (float64, error) {
			rawValue, e := get(row, field, rowIndex)
			if e != nil { return 0, e }
			value, e := tushareRawFloat(rawValue)
			if e != nil || (positive && value <= 0) || (!positive && value < 0) {
				return 0, fmt.Errorf("invalid Tushare %s on %s", field, isoDate)
			}
			return value, nil
		}
		open, err := readFloat("open", true); if err != nil { return nil, err }
		high, err := readFloat("high", true); if err != nil { return nil, err }
		low, err := readFloat("low", true); if err != nil { return nil, err }
		closePrice, err := readFloat("close", true); if err != nil { return nil, err }
		if low > high || open < low || open > high || closePrice < low || closePrice > high {
			return nil, fmt.Errorf("Tushare OHLC invariant failed on %s", isoDate)
		}
		rawVolume, err := readFloat("vol", false); if err != nil { return nil, err }
		volume, err := normalize.HandsToShares(rawVolume)
		if err != nil { return nil, fmt.Errorf("normalize Tushare volume on %s: %w", isoDate, err) }

		var amount, rawAmount *float64
		amountUnit, rawAmountUnit, rawAmountField := "unknown", "", ""
		quality := model.QualityValid
		var reasons []string
		amountRaw, err := get(row, "amount", rowIndex)
		if err != nil { return nil, err }
		if len(bytes.TrimSpace(amountRaw)) == 0 || bytes.Equal(bytes.TrimSpace(amountRaw), []byte("null")) || bytes.Equal(bytes.TrimSpace(amountRaw), []byte("\"\"")) {
			quality = model.QualityDegraded
			reasons = append(reasons, "amount_missing_from_provider", "amount_unit_unknown_because_amount_missing")
		} else {
			value, parseErr := tushareRawFloat(amountRaw)
			if parseErr != nil || value < 0 { return nil, fmt.Errorf("invalid Tushare amount on %s", isoDate) }
			rawCopy := value
			rawAmount = &rawCopy
			rawAmountUnit, rawAmountField = "thousand_CNY", "amount"
			normalizedAmount := value * 1000 // Tushare daily.amount uses thousand CNY.
			if math.IsInf(normalizedAmount, 0) || math.IsNaN(normalizedAmount) { return nil, fmt.Errorf("Tushare amount overflow on %s", isoDate) }
			amount = &normalizedAmount
			amountUnit = "CNY"
		}
		rawVolumeCopy := rawVolume
		pctRaw, err := get(row, "pct_chg", rowIndex)
		if err != nil { return nil, err }
		var pct *float64
		if len(bytes.TrimSpace(pctRaw)) > 0 && !bytes.Equal(bytes.TrimSpace(pctRaw), []byte("null")) {
			value, parseErr := tushareRawFloat(pctRaw)
			if parseErr == nil { pct = &value } else { quality = model.QualityDegraded; reasons = append(reasons, "provider_change_percent_unavailable") }
		}
		out = append(out, model.DailyBar{
			Symbol: symbol, TradeDate: isoDate, Period: "1d",
			Open: open, High: high, Low: low, Close: closePrice,
			Volume: &volume, Amount: amount, ChangePercent: pct,
			AdjustmentBasis: model.AdjustmentNone, AdjustmentMethod: "tushare_daily_raw_ohlc_no_adjustment",
			Source: p.Name(), SourceVersion: tushareVersion, FetchedAt: fetchedAt, NormalizedAt: time.Now().UTC(),
			RawVolume: &rawVolumeCopy, RawVolumeUnit: "hands", RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: rawAmountField,
			VolumeUnit: "shares", AmountUnit: amountUnit, QualityStatus: quality, QualityReasons: reasons, NormalizationVer: normalize.Version,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TradeDate < out[j].TradeDate })
	return normalizeProviderDailyBars(symbol, start.Format("2006-01-02"), end.Format("2006-01-02"), limit, out, 0)
}

func (p *Tushare) waitForRateLimit(ctx context.Context) error {
	p.rateMu.Lock()
	defer p.rateMu.Unlock()
	if !p.lastCall.IsZero() {
		delay := time.Until(p.lastCall.Add(700 * time.Millisecond))
		if delay > 0 {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done(): return ctx.Err()
			case <-timer.C:
			}
		}
	}
	p.lastCall = time.Now()
	return nil
}

func tushareString(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) { return "", errors.New("empty Tushare string") }
	var text string
	if err := json.Unmarshal(raw, &text); err == nil { return strings.TrimSpace(text), nil }
	var number json.Number
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&number); err != nil { return "", err }
	return number.String(), nil
}

func tushareRawFloat(raw json.RawMessage) (float64, error) {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) { return 0, errors.New("empty Tushare number") }
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil {
		if math.IsNaN(number) || math.IsInf(number, 0) { return 0, errors.New("non-finite Tushare number") }
		return number, nil
	}
	text, err := tushareString(raw)
	if err != nil || text == "" { return 0, errors.New("invalid Tushare number") }
	number, err = strconv.ParseFloat(text, 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) { return 0, errors.New("invalid Tushare number") }
	return number, nil
}
