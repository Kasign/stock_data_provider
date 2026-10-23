package provider

import (
	"context"
	"encoding/csv"
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
const neteaseFields = "TCLOSE;HIGH;LOW;TOPEN;LCLOSE;CHG;PCHG;TURNOVER;VOTURNOVER;VATURNOVER"

// Netease reads public historical CSV. The endpoint is unofficial and provides
// raw, unadjusted OHLC only; the adapter remains disabled by default.
type Netease struct {
	client     *http.Client
	historyURL string
}

func NewNetease(timeout time.Duration, historyURL string) *Netease {
	if timeout <= 0 {
		timeout = 12 * time.Second
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
		historyURL: strings.TrimSpace(historyURL),
	}
}

func (p *Netease) Name() string { return "netease" }

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
			Volume: &shares, Amount: amount, AdjustmentBasis: model.AdjustmentNone,
			AdjustmentMethod: "netease_raw_ohlc_no_adjustment",
			Source:           p.Name(), SourceVersion: neteaseVersion,
			FetchedAt: now, NormalizedAt: now, RawVolume: &rawVolumeCopy,
			RawVolumeUnit: volumeUnit, RawAmount: &rawAmountCopy, RawAmountUnit: amountUnit,
			RawAmountField: "VATURNOVER", VolumeUnit: "shares", AmountUnit: amountUnit,
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
func neteaseValidateTradingUnits(rawVolume, rawAmount, low, high float64) (int64, string, *float64, string, model.QualityStatus, []string) {
	rawAmountCopy := rawAmount
	if rawVolume <= 0 || rawAmount <= 0 || low <= 0 || high < low {
		return int64(math.Round(math.Max(0, rawVolume))), "unknown", nil, "unknown", model.QualityDegraded,
			[]string{"netease_amount_volume_price_consistency_unavailable", "netease_volume_unit_unverified"}
	}
	sharesAverage := rawAmount / rawVolume
	handsAverage := rawAmount / (rawVolume * 100)
	sharesMatch := sharesAverage >= low*0.95 && sharesAverage <= high*1.05
	handsMatch := handsAverage >= low*0.95 && handsAverage <= high*1.05
	if sharesMatch && !handsMatch {
		shares, err := normalize.RawSharesToInt64(rawVolume)
		if err == nil {
			return shares, "shares", &rawAmountCopy, "CNY", model.QualityValid, nil
		}
	}
	if handsMatch && !sharesMatch {
		if rawVolume >= float64(math.MaxInt64)/100 {
			return 0, "unknown", nil, "unknown", model.QualityDegraded, []string{"netease_volume_overflow", "netease_volume_unit_unverified"}
		}
		shares, err := normalize.RawSharesToInt64(rawVolume * 100)
		if err == nil {
			return shares, "hands_inferred", &rawAmountCopy, "CNY", model.QualityDegraded,
				[]string{"netease_volume_unit_inferred_from_amount_price"}
		}
	}
	shares, err := normalize.RawSharesToInt64(rawVolume)
	if err != nil {
		return 0, "unknown", nil, "unknown", model.QualityDegraded, []string{"netease_volume_unit_unverified"}
	}
	return shares, "unknown", nil, "unknown", model.QualityDegraded,
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
