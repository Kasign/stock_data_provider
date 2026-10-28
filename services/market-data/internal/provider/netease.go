package provider

import (
	"context"
	"encoding/csv"
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
	"unicode/utf8"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
	"golang.org/x/text/encoding/htmlindex"
)

const neteaseVersion = "netease-history-csv-v1"
const neteaseQuoteVersion = "netease-finance-quote-v1"
const maxNeteaseQuoteBytes = 1 << 20
const neteaseFields = "TCLOSE;HIGH;LOW;TOPEN;LCLOSE;CHG;PCHG;TURNOVER;VOTURNOVER;VATURNOVER"

// Netease reads public quote JSONP and historical CSV endpoints. The endpoints
// are unofficial; historical bars provide raw, unadjusted OHLC only. Provider enablement
// is controlled by providers.json and MARKET_DATA_NETEASE_*_ENABLED settings.

type Netease struct {
	client     *http.Client
	quoteURL   string
	historyURL string
}

func NewNetease(timeout time.Duration, historyURL string) *Netease {
	return NewNeteaseConfigured(timeout, "", historyURL)
}

func NewNeteaseConfigured(timeout time.Duration, quoteURL, historyURL string) *Netease {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if strings.TrimSpace(quoteURL) == "" {
		quoteURL = "https://api.money.126.net/data/feed"
	}
	if strings.TrimSpace(historyURL) == "" {
		historyURL = "https://quotes.money.163.com/service/chddata.html"
	}
	return &Netease{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		quoteURL:   strings.TrimRight(strings.TrimSpace(quoteURL), "/"),
		historyURL: strings.TrimSpace(historyURL),
	}
}

func (p *Netease) Name() string { return "netease" }

// FetchQuote uses NetEase's JSONP quote endpoint and validates the provider echo.
func (p *Netease) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	code, err := neteaseCode(symbol)
	if err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(p.quoteURL + "/" + code + ",money.api")
	if err != nil {
		return nil, fmt.Errorf("parse Netease quote endpoint: %w", err)
	}
	query := endpoint.Query()
	query.Set("callback", "_ntes_quote_callback")
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Netease quote request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MarketDataProvider/1.0)")
	req.Header.Set("Accept", "application/javascript,application/json,*/*")
	req.Header.Set("Referer", "https://money.163.com/")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Netease quote request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Netease quote HTTP status %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxNeteaseQuoteBytes)
	if err != nil {
		return nil, fmt.Errorf("read Netease quote response: %w", err)
	}
	jsonBody, err := decodeNeteaseJSONP(body)
	if err != nil {
		return nil, fmt.Errorf("decode Netease quote JSONP: %w", err)
	}
	var payload map[string]neteaseQuoteRow
	if err := json.Unmarshal(jsonBody, &payload); err != nil {
		return nil, fmt.Errorf("parse Netease quote JSON: %w", err)
	}
	row, ok := payload[code]
	if !ok {
		return nil, fmt.Errorf("Netease quote response does not contain requested code %s", code)
	}
	if reported := strings.TrimSpace(row.Code); reported != "" && reported != code {
		return nil, fmt.Errorf("Netease quote code mismatch: requested %s got %q", code, reported)
	}
	if reported := strings.TrimSpace(row.Symbol); reported != "" && reported != symbol[:6] {
		return nil, fmt.Errorf("Netease quote symbol mismatch: requested %s got %q", symbol, reported)
	}
	price, err := parseNeteaseNumber(string(row.Price))
	if err != nil || price <= 0 {
		return nil, errors.New("Netease quote current price is missing or invalid")
	}
	openV, err := parseNeteaseNumber(string(row.Open))
	if err != nil || openV <= 0 {
		return nil, errors.New("Netease quote open price is missing or invalid")
	}
	highV, err := parseNeteaseNumber(string(row.High))
	if err != nil || highV <= 0 {
		return nil, errors.New("Netease quote high price is missing or invalid")
	}
	lowV, err := parseNeteaseNumber(string(row.Low))
	if err != nil || lowV <= 0 {
		return nil, errors.New("Netease quote low price is missing or invalid")
	}
	prevClose, err := parseNeteaseNumber(string(row.YestClose))
	if err != nil || prevClose <= 0 {
		return nil, errors.New("Netease quote previous close is missing or invalid")
	}
	change, err := parseNeteaseNumber(string(row.Updown))
	if err != nil {
		return nil, errors.New("Netease quote change is missing or invalid")
	}
	percentRatio, err := parseNeteaseNumber(string(row.Percent))
	if err != nil {
		return nil, errors.New("Netease quote percent is missing or invalid")
	}
	changePercent := percentRatio * 100
	if !finiteNumber(changePercent) || math.Abs(changePercent) > 1000 {
		return nil, errors.New("Netease quote percent is outside a plausible numeric range")
	}
	if !finiteNumber(change) {
		return nil, errors.New("Netease quote change is not a finite number")
	}
	canonicalChange := price - prevClose
	canonicalChangePercent := canonicalChange / prevClose * 100
	if !finiteNumber(canonicalChange) || !finiteNumber(canonicalChangePercent) {
		return nil, errors.New("Netease quote canonical change is not a finite number")
	}
	if math.Abs(change-canonicalChange) > 0.05 {
		return nil, errors.New("Netease quote change is inconsistent with latest and previous close")
	}
	if math.Abs(changePercent-canonicalChangePercent) > 0.15 {
		return nil, errors.New("Netease quote percent is inconsistent with latest and previous close")
	}
	// 使用最新价和昨收价计算规范值，源端涨跌字段仅用于交叉校验。
	change = canonicalChange
	changePercent = canonicalChangePercent
	rawVolume, volumeErr := parseNeteaseNumber(string(row.Volume))
	var volume *int64
	if volumeErr == nil {
		converted, convertErr := normalize.RawSharesToInt64(rawVolume)
		if convertErr == nil {
			volume = &converted
		}
	}
	rawAmount, amountErr := parseNeteaseNumber(string(row.Turnover))
	var amount *float64
	if amountErr == nil && rawAmount >= 0 && finiteNumber(rawAmount) {
		amount = &rawAmount
	}
	now := time.Now().UTC()
	quoteTime := validProviderSnapshotTime(parseNeteaseQuoteTime(firstNonEmptyString(row.Time, row.Update)), now)
	q := &model.Quote{
		Symbol: symbol, Name: strings.TrimSpace(row.Name),
		CurrentPrice: &price, Change: &change, ChangePercent: &changePercent,
		Open: &openV, High: &highV, Low: &lowV, PrevClose: &prevClose,
		Volume: volume, Amount: amount, QuoteTime: quoteTime,
		QuoteTimeSemantics: "provider_snapshot_time",
		FetchedAt:          now, NormalizedAt: now, Source: p.Name(), SourceVersion: neteaseQuoteVersion,
		RawVolume: float64PointerIfValid(rawVolume, volumeErr == nil), RawVolumeUnit: optionalVolumeUnit(volume, "shares"),
		RawAmount: float64PointerIfValid(rawAmount, amountErr == nil), RawAmountUnit: optionalUnit(amount, "CNY"),
		RawAmountField: "turnover", VolumeUnit: optionalVolumeUnit(volume, "shares"),
		AmountUnit: optionalUnit(amount, "CNY"), AdjustmentBasis: model.AdjustmentNone,
		QualityStatus: model.QualityValid, NormalizationVer: normalize.Version,
	}
	if strings.TrimSpace(row.Symbol) == "" && strings.TrimSpace(row.Code) == "" {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "provider_symbol_missing_assumed_from_request")
	}
	if volume == nil {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "volume_missing_or_invalid")
	}
	if amount == nil {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "amount_missing_or_invalid")
	}
	if quoteTime == nil {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "provider_timestamp_missing")
	}
	if q.Name == "" {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "provider_name_missing")
	}
	if err := normalize.ValidateQuote(q); err != nil {
		return nil, fmt.Errorf("Netease quote data contract: %w", err)
	}
	return q, nil
}

type neteaseQuoteRow struct {
	Code      string          `json:"code"`
	Symbol    string          `json:"symbol"`
	Name      string          `json:"name"`
	Price     json.RawMessage `json:"price"`
	Percent   json.RawMessage `json:"percent"`
	Updown    json.RawMessage `json:"updown"`
	Open      json.RawMessage `json:"open"`
	High      json.RawMessage `json:"high"`
	Low       json.RawMessage `json:"low"`
	YestClose json.RawMessage `json:"yestclose"`
	Volume    json.RawMessage `json:"volume"`
	Turnover  json.RawMessage `json:"turnover"`
	Time      string          `json:"time"`
	Update    string          `json:"update"`
}

func decodeNeteaseJSONP(body []byte) ([]byte, error) {
	text := strings.TrimSpace(string(body))
	open := strings.IndexByte(text, '(')
	close := strings.LastIndexByte(text, ')')
	if open <= 0 || close <= open {
		return nil, errors.New("JSONP callback wrapper is missing")
	}
	callback := strings.TrimSpace(text[:open])
	for _, r := range callback {
		if !(r == '_' || r == '.' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9') {
			return nil, errors.New("JSONP callback name contains invalid characters")
		}
	}
	trailing := strings.TrimSpace(text[close+1:])
	if trailing != "" && trailing != ";" {
		return nil, errors.New("unexpected content after JSONP callback")
	}
	return []byte(strings.TrimSpace(text[open+1 : close])), nil
}

func parseNeteaseQuoteTime(raw string) *time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	for _, layout := range []string{"2006/01/02 15:04:05", "2006-01-02 15:04:05", time.RFC3339} {
		var parsed time.Time
		var parseErr error
		if layout == time.RFC3339 {
			parsed, parseErr = time.Parse(layout, raw)
		} else {
			parsed, parseErr = time.ParseInLocation(layout, raw, loc)
		}
		if parseErr == nil {
			return &parsed
		}
	}
	return nil
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func float64PointerIfValid(value float64, ok bool) *float64 {
	if !ok || !finiteNumber(value) {
		return nil
	}
	return &value
}

func (p *Netease) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	code, err := neteaseCode(symbol)
	if err != nil {
		return nil, err
	}
	if adjustment != model.AdjustmentNone {
		return nil, fmt.Errorf("Netease historical CSV only supports verified adjustment basis %q", model.AdjustmentNone)
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	start, end, err := neteaseDateRange(startDate, endDate, limit, time.Now())
	if err != nil {
		return nil, err
	}
	startISO, endISO := start.Format("2006-01-02"), end.Format("2006-01-02")

	u, err := url.Parse(p.historyURL)
	if err != nil {
		return nil, fmt.Errorf("parse Netease history endpoint: %w", err)
	}
	query := u.Query()
	query.Set("code", code)
	query.Set("start", start.Format("20060102"))
	query.Set("end", end.Format("20060102"))
	query.Set("fields", neteaseFields)
	u.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build Netease history request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MarketDataProvider/1.0)")
	req.Header.Set("Accept", "text/csv,text/plain,application/octet-stream,*/*")
	req.Header.Set("Referer", "https://money.163.com/")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Netease history request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Netease history HTTP status %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("read Netease history response: %w", err)
	}
	decoded := decodeSinaResponseBody(body, resp.Header.Get("Content-Type"))
	if !utf8.Valid(decoded) {
		// Some responses omit charset even though the CSV is encoded in GBK.
		gbk, lookupErr := htmlindex.Get("gbk")
		if lookupErr == nil {
			if converted, convertErr := gbk.NewDecoder().Bytes(body); convertErr == nil {
				decoded = converted
			}
		}
	}
	if !utf8.Valid(decoded) {
		return nil, errors.New("Netease history CSV is not valid UTF-8 or GBK after charset decoding")
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(decoded), "\uFEFF")))
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read Netease CSV header: %w", err)
	}
	columns := neteaseCSVColumns(header)
	for _, required := range []string{"date", "code", "open", "high", "low", "close", "volume", "amount"} {
		if _, ok := columns[required]; !ok {
			return nil, fmt.Errorf("Netease CSV missing required column %q; header=%q", required, header)
		}
	}

	rows := make([]model.DailyBar, 0, 256)
	rejectedRows := 0
	now := time.Now().UTC()
	for {
		record, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			rejectedRows++
			continue
		}
		if len(record) == 0 || (len(record) == 1 && strings.TrimSpace(record[0]) == "") {
			continue
		}
		get := func(name string) string {
			index := columns[name]
			if index < 0 || index >= len(record) {
				return ""
			}
			return strings.TrimSpace(record[index])
		}
		date, dateErr := time.Parse("2006-01-02", get("date"))
		rowCode := neteaseCleanCode(get("code"))
		openV, openErr := parseNeteaseNumber(get("open"))
		highV, highErr := parseNeteaseNumber(get("high"))
		lowV, lowErr := parseNeteaseNumber(get("low"))
		closeV, closeErr := parseNeteaseNumber(get("close"))
		rawVolume, volumeErr := parseNeteaseNumber(get("volume"))
		rawAmount, amountErr := parseNeteaseNumber(get("amount"))
		if dateErr != nil || rowCode == "" || rowCode != symbol[:6] ||
			openErr != nil || highErr != nil || lowErr != nil || closeErr != nil ||
			volumeErr != nil || amountErr != nil || !finiteNumber(openV) || !finiteNumber(highV) ||
			!finiteNumber(lowV) || !finiteNumber(closeV) || !finiteNumber(rawVolume) ||
			!finiteNumber(rawAmount) || openV <= 0 || highV <= 0 || lowV <= 0 || closeV <= 0 ||
			rawVolume < 0 || rawAmount < 0 {
			rejectedRows++
			continue
		}
		tradeDate := date.Format("2006-01-02")
		if tradeDate < startISO || tradeDate > endISO {
			rejectedRows++
			continue
		}
		shares, volumeUnit, amount, amountUnit, quality, reasons := neteaseValidateTradingUnits(rawVolume, rawAmount, lowV, highV)
		rawVolumeCopy, rawAmountCopy := rawVolume, rawAmount
		bar := model.DailyBar{
			Symbol: symbol, TradeDate: tradeDate, Period: "1d",
			Open: openV, High: highV, Low: lowV, Close: closeV,
			Volume: shares, Amount: amount, AdjustmentBasis: model.AdjustmentNone,
			AdjustmentMethod: "netease_raw_ohlc_no_adjustment",
			Source:           p.Name(), SourceVersion: neteaseVersion,
			FetchedAt: now, NormalizedAt: now, RawVolume: &rawVolumeCopy,
			RawVolumeUnit: volumeUnit, RawAmount: &rawAmountCopy, RawAmountUnit: amountUnit,
			RawAmountField: "VATURNOVER", VolumeUnit: optionalVolumeUnit(shares, "shares"), AmountUnit: amountUnit,
			QualityStatus: quality, QualityReasons: reasons,
			NormalizationVer: normalize.Version,
		}
		if index, ok := columns["change_percent"]; ok && index >= 0 && index < len(record) {
			if changePercent, parseErr := parseNeteaseNumber(record[index]); parseErr == nil && finiteNumber(changePercent) {
				bar.ChangePercent = &changePercent
			}
		}
		rows = append(rows, bar)
	}
	if len(rows) == 0 {
		return nil, errors.New("Netease returned no valid daily bars in the requested date range")
	}
	return normalizeProviderDailyBars(symbol, startISO, endISO, limit, rows, rejectedRows)
}

func neteaseCSVColumns(header []string) map[string]int {
	columns := make(map[string]int, len(header))
	for index, value := range header {
		key := strings.TrimSpace(strings.TrimPrefix(value, "\uFEFF"))
		switch key {
		case "日期", "date", "Date":
			columns["date"] = index
		case "股票代码", "代码", "code", "Code":
			columns["code"] = index
		case "开盘价", "开盘", "open", "Open":
			columns["open"] = index
		case "最高价", "最高", "high", "High":
			columns["high"] = index
		case "最低价", "最低", "low", "Low":
			columns["low"] = index
		case "收盘价", "收盘", "close", "Close":
			columns["close"] = index
		case "成交量", "成交股数", "volume", "Volume":
			columns["volume"] = index
		case "成交金额", "成交额", "amount", "Amount":
			columns["amount"] = index
		case "涨跌幅", "change_percent", "ChangePercent":
			columns["change_percent"] = index
		}
	}
	return columns
}

func neteaseCleanCode(raw string) string {
	value := strings.TrimSpace(strings.Trim(raw, "'\""))
	if len(value) > 6 {
		value = value[len(value)-6:]
	}
	return value
}

func parseNeteaseNumber(raw string) (float64, error) {
	value := strings.TrimSpace(strings.Trim(raw, "'\""))
	value = strings.ReplaceAll(value, ",", "")
	if value == "" || value == "-" || strings.EqualFold(value, "null") {
		return 0, errors.New("Netease numeric value is empty")
	}
	number, err := strconv.ParseFloat(value, 64)
	if err != nil || !finiteNumber(number) {
		return 0, errors.New("Netease numeric value is not finite")
	}
	return number, nil
}

// neteaseValidateTradingUnits uses the reported amount and daily OHLC range to
// test whether volume is in shares or hands. Ambiguous values stay unknown.
func neteaseValidateTradingUnits(rawVolume, rawAmount, low, high float64) (*int64, string, *float64, string, model.QualityStatus, []string) {
	rawAmountCopy := rawAmount
	if rawVolume <= 0 || rawAmount <= 0 || low <= 0 || high < low {
		// 单位尚不能判断时，不向标准 volume 字段填入未经转换的原始数字。
		return nil, "unknown", nil, "unknown", model.QualityDegraded,
			[]string{"netease_amount_volume_price_consistency_unavailable", "netease_volume_unit_unverified"}
	}
	sharesAverage := rawAmount / rawVolume
	handsAverage := rawAmount / (rawVolume * 100)
	sharesMatch := sharesAverage >= low*0.95 && sharesAverage <= high*1.05
	handsMatch := handsAverage >= low*0.95 && handsAverage <= high*1.05
	if sharesMatch && !handsMatch {
		shares, err := normalize.RawSharesToInt64(rawVolume)
		if err == nil {
			return &shares, "shares", &rawAmountCopy, "CNY", model.QualityValid, nil
		}
	}
	if handsMatch && !sharesMatch {
		if rawVolume >= float64(math.MaxInt64)/100 {
			return nil, "unknown", nil, "unknown", model.QualityDegraded, []string{"netease_volume_overflow", "netease_volume_unit_unverified"}
		}
		shares, err := normalize.RawSharesToInt64(rawVolume * 100)
		if err == nil {
			return &shares, "hands_inferred", &rawAmountCopy, "CNY", model.QualityDegraded,
				[]string{"netease_volume_unit_inferred_from_amount_price"}
		}
	}
	if _, err := normalize.RawSharesToInt64(rawVolume); err != nil {
		return nil, "unknown", nil, "unknown", model.QualityDegraded, []string{"netease_volume_unit_unverified"}
	}
	// 即使原始数字可转成整数，若股/手两种解释均不符合成交金额与价格区间，
	// 也不能把它冒充为标准化股数；调用方应让统一校验器剔除此行。
	return nil, "unknown", nil, "unknown", model.QualityDegraded,
		[]string{"netease_amount_volume_price_consistency_failed", "netease_volume_unit_unverified"}
}

func neteaseCode(symbol string) (string, error) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || len(parts[0]) != 6 {
		return "", fmt.Errorf("unsupported Netease symbol %q", symbol)
	}
	switch parts[1] {
	case "SH":
		return "0" + parts[0], nil
	case "SZ":
		return "1" + parts[0], nil
	default:
		return "", fmt.Errorf("Netease supports Shanghai and Shenzhen A-share symbols only, got %q", symbol)
	}
}

func neteaseDateRange(startDate, endDate string, limit int, now time.Time) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	localNow := now.In(loc)
	end := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, loc).AddDate(0, 0, -1)
	if strings.TrimSpace(endDate) != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02", strings.TrimSpace(endDate), loc)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid Netease end date: %w", parseErr)
		}
		end = parsed
	}
	if end.Format("2006-01-02") > localNow.Format("2006-01-02") {
		return time.Time{}, time.Time{}, errors.New("Netease end date must not be after today in Asia/Shanghai")
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	start := end.AddDate(0, 0, -(limit*4 + 30))
	if strings.TrimSpace(startDate) != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02", strings.TrimSpace(startDate), loc)
		if parseErr != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid Netease start date: %w", parseErr)
		}
		start = parsed
	}
	if start.After(end) {
		return time.Time{}, time.Time{}, errors.New("Netease start date must not be after end date")
	}
	return start, end, nil
}
