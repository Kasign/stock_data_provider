package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"golang.org/x/text/encoding/htmlindex"
)

const sinaVersion = "sina-finance-quote-v1"
const sinaKlineVersion = "sina-finance-kline-v1"
const maxSinaResponseBytes = 1 << 20
const maxSinaKlineBars = 1023

// 新浪 Provider 同时实现实时行情和历史日 K。两个端点字段契约不同，必须分别归一化。
type Sina struct {
	client   *http.Client
	quoteURL string
	klineURL string
}

func NewSina(timeout time.Duration, quoteURL string) *Sina {
	return NewSinaConfigured(timeout, quoteURL, "")
}

func NewSinaConfigured(timeout time.Duration, quoteURL, klineURL string) *Sina {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if strings.TrimSpace(quoteURL) == "" {
		quoteURL = "https://hq.sinajs.cn/list="
	}
	if strings.TrimSpace(klineURL) == "" {
		klineURL = "https://money.finance.sina.com.cn/quotes_service/api/json_v2.php/CN_MarketData.getKLineData"
	}
	return &Sina{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		quoteURL: strings.TrimSpace(quoteURL),
		klineURL: strings.TrimSpace(klineURL),
	}
}

func (p *Sina) Name() string { return "sina" }

// FetchDailyBars 读取新浪历史日 K。该端点没有可验证的复权参数和成交额字段，
// 因此只接受 none 口径，成交额缺失必须标记 degraded，不推算金额。
func (p *Sina) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	code, err := sinaCode(symbol)
	if err != nil {
		return nil, err
	}
	if adjustment != model.AdjustmentNone {
		return nil, fmt.Errorf("Sina historical K-line endpoint only supports verified adjustment basis %q", model.AdjustmentNone)
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	start, end, err := sinaDailyDateRange(startDate, endDate, limit, time.Now())
	if err != nil {
		return nil, err
	}
	startISO, endISO := start.Format("2006-01-02"), end.Format("2006-01-02")
	dataLength := sinaKlineDataLength(start, end, limit)

	endpoint, err := url.Parse(p.klineURL)
	if err != nil {
		return nil, fmt.Errorf("parse Sina K-line endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("symbol", code)
	query.Set("scale", "240")
	query.Set("ma", "no")
	query.Set("datalen", strconv.Itoa(dataLength))
	endpoint.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Sina K-line request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MarketDataProvider/1.0)")
	req.Header.Set("Referer", "https://finance.sina.com.cn/")
	req.Header.Set("Accept", "application/json,*/*")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Sina K-line request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Sina K-line HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSinaResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Sina K-line response: %w", err)
	}
	if len(body) > maxSinaResponseBytes {
		return nil, errors.New("Sina K-line response exceeds 1 MiB")
	}
	var rows []sinaKlineRow
	if err := decodeSingleJSON(body, &rows); err != nil {
		return nil, fmt.Errorf("decode Sina K-line JSON: %w", err)
	}
	if len(rows) == 0 {
		return nil, errors.New("Sina returned an empty K-line series")
	}

	type datedRow struct {
		row  sinaKlineRow
		date string
	}
	dated := make([]datedRow, 0, len(rows))
	oldestDate := ""
	rejectedRows := 0
	for _, row := range rows {
		day := strings.TrimSpace(row.Day)
		if len(day) < len("2006-01-02") {
			rejectedRows++
			continue
		}
		day = day[:len("2006-01-02")]
		if _, err := time.Parse("2006-01-02", day); err != nil {
			rejectedRows++
			continue
		}
		if oldestDate == "" || day < oldestDate {
			oldestDate = day
		}
		dated = append(dated, datedRow{row: row, date: day})
	}
	// 新浪最多返回 1023 条。如果返回触顶而最老记录仍晚于请求开始日期，
	// 明确报错而不是把不完整的历史区间伪装成成功响应。
	if dataLength == maxSinaKlineBars && len(rows) >= dataLength && startDate != "" && oldestDate != "" && oldestDate > startISO {
		return nil, fmt.Errorf("Sina K-line lookback limit reached: requested start %s but oldest returned row is %s", startISO, oldestDate)
	}

	now := time.Now().UTC()
	bars := make([]model.DailyBar, 0, len(dated))
	for _, item := range dated {
		openV, errOpen := parseSinaKlineNumber(item.row.Open)
		highV, errHigh := parseSinaKlineNumber(item.row.High)
		lowV, errLow := parseSinaKlineNumber(item.row.Low)
		closeV, errClose := parseSinaKlineNumber(item.row.Close)
		rawVolume, errVolume := parseSinaKlineNumber(item.row.Volume)
		if errOpen != nil || errHigh != nil || errLow != nil || errClose != nil || errVolume != nil {
			rejectedRows++
			continue
		}
		volume, err := normalize.RawSharesToInt64(rawVolume)
		if err != nil {
			rejectedRows++
			continue
		}
		rawVolumeCopy := rawVolume
		reasons := []string{"provider_amount_unavailable", "provider_symbol_not_echoed_by_kline_response"}
		if item.date == time.Now().In(sinaChinaLocation()).Format("2006-01-02") {
			reasons = append(reasons, "sina_current_day_daily_bar_may_be_partial")
		}
		bars = append(bars, model.DailyBar{
			Symbol: symbol, TradeDate: item.date, Period: "1d",
			Open: openV, High: highV, Low: lowV, Close: closeV,
			Volume: &volume, Amount: nil,
			AdjustmentBasis: model.AdjustmentNone, AdjustmentMethod: "sina_raw_ohlc_no_adjustment",
			Source: p.Name(), SourceVersion: sinaKlineVersion,
			FetchedAt: now, NormalizedAt: now, RawVolume: &rawVolumeCopy,
			RawVolumeUnit: "shares", RawAmountUnit: "unknown",
			VolumeUnit: "shares", AmountUnit: "unknown",
			QualityStatus: model.QualityDegraded, QualityReasons: reasons,
			NormalizationVer: normalize.Version,
		})
	}
	return normalizeProviderDailyBars(symbol, startISO, endISO, limit, bars, rejectedRows)
}

type sinaKlineRow struct {
	Day    string          `json:"day"`
	Open   json.RawMessage `json:"open"`
	High   json.RawMessage `json:"high"`
	Low    json.RawMessage `json:"low"`
	Close  json.RawMessage `json:"close"`
	Volume json.RawMessage `json:"volume"`
}

func parseSinaKlineNumber(raw json.RawMessage) (float64, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return 0, errors.New("Sina K-line numeric field is missing")
	}
	if strings.HasPrefix(value, "\"") {
		var decoded string
		if err := json.Unmarshal(raw, &decoded); err != nil {
			return 0, err
		}
		value = strings.TrimSpace(decoded)
	}
	if value == "" || value == "-" {
		return 0, errors.New("Sina K-line numeric field is empty")
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || !finiteNumber(number) {
		return 0, errors.New("Sina K-line numeric field is not finite")
	}
	return number, nil
}

func sinaChinaLocation() *time.Location {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

func sinaDailyDateRange(startDate, endDate string, limit int, now time.Time) (time.Time, time.Time, error) {
	loc := sinaChinaLocation()
	localNow := now.In(loc)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	if strings.TrimSpace(endDate) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid Sina K-line end date: %w", err)
		}
		end = parsed
	}
	if limit <= 0 {
		limit = 400
	}
	startDays := limit*4 + 30
	maxWindowDays := maxSinaKlineBars*7/5 - 30
	if startDays > maxWindowDays {
		startDays = maxWindowDays
	}
	start := end.AddDate(0, 0, -startDays)
	if strings.TrimSpace(startDate) != "" {
		parsed, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid Sina K-line start date: %w", err)
		}
		start = parsed
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, errors.New("Sina K-line start date must not be after end date")
	}
	return start, end, nil
}

func sinaKlineDataLength(start, end time.Time, limit int) int {
	calendarDays := int(math.Ceil(end.Sub(start).Hours()/24)) + 1
	estimate := calendarDays*7/5 + 20
	if estimate < limit {
		estimate = limit
	}
	if estimate < 32 {
		estimate = 32
	}
	if estimate > maxSinaKlineBars {
		estimate = maxSinaKlineBars
	}
	return estimate
}

func (p *Sina) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	code, err := sinaCode(symbol)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.quoteURL+code, nil)
	if err != nil {
		return nil, fmt.Errorf("build Sina quote request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MarketDataProvider/1.0)")
	req.Header.Set("Referer", "https://finance.sina.com.cn/")
	req.Header.Set("Accept", "text/plain,*/*")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Sina quote request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Sina quote HTTP status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxSinaResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read Sina quote response: %w", err)
	}
	if len(body) > maxSinaResponseBytes {
		return nil, errors.New("Sina quote response exceeds 1 MiB")
	}
	body = decodeSinaResponseBody(body, resp.Header.Get("Content-Type"))

	text := strings.TrimSpace(string(body))
	start := strings.Index(text, "=\"")
	end := strings.LastIndex(text, "\";")
	if start < 0 || end <= start+2 {
		return nil, errors.New("Sina response does not contain a quoted quote payload")
	}
	fields := strings.Split(text[start+2:end], ",")
	if len(fields) < 32 {
		return nil, fmt.Errorf("Sina quote response has only %d fields; expected at least 32", len(fields))
	}

	name := strings.TrimSpace(fields[0])
	// 先按 HTTP Content-Type 解码响应字符集；若上游未声明或声明了未知字符集，则保留原字节，并通过 UTF-8 校验将无效名称降级处理。
	if !utf8.ValidString(name) {
		name = ""
	}
	parseRequired := func(index int, field string) (float64, error) {
		raw := strings.TrimSpace(fields[index])
		if raw == "" || raw == "-" {
			return 0, fmt.Errorf("Sina quote field %s is missing", field)
		}
		value, parseErr := strconv.ParseFloat(raw, 64)
		if parseErr != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return 0, fmt.Errorf("Sina quote field %s is not a finite number", field)
		}
		return value, nil
	}
	open, err := parseRequired(1, "open")
	if err != nil {
		return nil, err
	}
	prevClose, err := parseRequired(2, "prev_close")
	if err != nil {
		return nil, err
	}
	price, err := parseRequired(3, "current_price")
	if err != nil {
		return nil, err
	}
	high, err := parseRequired(4, "high")
	if err != nil {
		return nil, err
	}
	low, err := parseRequired(5, "low")
	if err != nil {
		return nil, err
	}
	if open <= 0 || prevClose <= 0 || price <= 0 || high <= 0 || low <= 0 {
		return nil, errors.New("Sina quote OHLC and previous close must be positive")
	}
	if low > high || open < low || open > high || price < low || price > high {
		return nil, errors.New("Sina quote OHLC invariant failed")
	}
	rawVolume, err := parseRequired(8, "volume")
	if err != nil || rawVolume < 0 {
		return nil, errors.New("Sina quote volume must be a non-negative number of shares")
	}
	volume, err := normalize.RawSharesToInt64(rawVolume)
	if err != nil {
		return nil, fmt.Errorf("normalize Sina quote volume: %w", err)
	}

	var amount *float64
	rawAmountText := strings.TrimSpace(fields[9])
	if rawAmountText != "" && rawAmountText != "-" {
		v, parseErr := strconv.ParseFloat(rawAmountText, 64)
		if parseErr == nil && !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 {
			amount = &v
		}
	}
	change := price - prevClose
	changePct := change / prevClose * 100
	fetchedAt := time.Now().UTC()
	var quoteTime *time.Time
	dateText, timeText := strings.TrimSpace(fields[30]), strings.TrimSpace(fields[31])
	if dateText != "" && timeText != "" {
		if parsed, parseErr := time.ParseInLocation("2006-01-02 15:04:05", dateText+" "+timeText, time.FixedZone("CST", 8*60*60)); parseErr == nil {
			utc := parsed.UTC()
			quoteTime = &utc
		}
	}
	quality := model.QualityValid
	reasons := make([]string, 0, 3)
	if name == "" {
		quality = model.QualityDegraded
		reasons = append(reasons, "provider_name_missing_or_invalid_encoding")
	}
	if quoteTime == nil {
		quality = model.QualityDegraded
		reasons = append(reasons, "provider_timestamp_missing_or_invalid")
	}
	amountUnit := "CNY"
	if amount == nil {
		quality = model.QualityDegraded
		amountUnit = "unknown"
		reasons = append(reasons, "amount_missing_or_invalid")
	}

	rawVolumeCopy := rawVolume
	quote := &model.Quote{
		Symbol: symbol, Name: name,
		CurrentPrice: &price, Change: &change, ChangePercent: &changePct,
		Open: &open, High: &high, Low: &low, PrevClose: &prevClose,
		Volume: &volume, Amount: amount, QuoteTime: quoteTime, QuoteTimeSemantics: "provider_snapshot_time",
		FetchedAt: fetchedAt, NormalizedAt: fetchedAt,
		Source: p.Name(), SourceVersion: sinaVersion,
		RawVolume: &rawVolumeCopy, RawVolumeUnit: "shares",
		RawAmount: amount, RawAmountUnit: amountUnit, RawAmountField: optionalUnitName(amount, "field_9"),
		VolumeUnit: "shares", AmountUnit: amountUnit,
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: quality,
		QualityReasons: reasons, NormalizationVer: normalize.Version,
	}
	if err := normalize.ValidateQuote(quote); err != nil {
		return nil, fmt.Errorf("Sina quote data contract: %w", err)
	}
	return quote, nil
}

func decodeSinaResponseBody(body []byte, contentType string) []byte {
	if strings.TrimSpace(contentType) == "" {
		return body
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return body
	}
	charset := strings.TrimSpace(params["charset"])
	if charset == "" {
		return body
	}
	encoding, err := htmlindex.Get(charset)
	if err != nil {
		return body
	}
	decoded, err := encoding.NewDecoder().Bytes(body)
	if err != nil {
		return body
	}
	return decoded
}

func sinaCode(symbol string) (string, error) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || len(parts[0]) != 6 {
		return "", fmt.Errorf("Sina supports six-digit mainland A-share symbols only, got %q", symbol)
	}
	switch parts[1] {
	case "SH":
		return "sh" + parts[0], nil
	case "SZ":
		return "sz" + parts[0], nil
	default:
		return "", fmt.Errorf("Sina does not support exchange suffix %q", parts[1])
	}
}
