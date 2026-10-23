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

const eastmoneyVersion = "eastmoney-stock-api-v1"

// Bound implicit historical requests so old out-of-window rows do not pollute
// quality warnings when callers only want the most recent N trading bars.
const eastmoneyLookbackDaysPerBar = 4
const eastmoneyLookbackBufferDays = 30

// defaultEastmoneyUT is the public web token commonly used by Eastmoney front-end clients.
// It is not a user credential or private API key; deployments can override it if Eastmoney rotates it.
const defaultEastmoneyUT = "7eea3edcaed734bea9cbfc24409ed989"
const defaultEastmoneyStockBasicURL = "https://datacenter.eastmoney.com/securities/api/data/v1/get"

type Eastmoney struct {
	client        *http.Client
	quoteURL      string
	klineURL      string
	stockBasicURL string
	ut            string
}

func NewEastmoney(timeout time.Duration, quoteURL, klineURL string) *Eastmoney {
	return NewEastmoneyConfigured(timeout, quoteURL, klineURL, "")
}

// NewEastmoneyConfigured allows the public Eastmoney front-end token to be overridden without changing endpoint URLs.
func NewEastmoneyConfigured(timeout time.Duration, quoteURL, klineURL, ut string) *Eastmoney {
	return NewEastmoneyConfiguredWithStockBasic(timeout, quoteURL, klineURL, "", ut)
}

// NewEastmoneyConfiguredWithStockBasic configures the independent F10 security-identity endpoint.
func NewEastmoneyConfiguredWithStockBasic(timeout time.Duration, quoteURL, klineURL, stockBasicURL, ut string) *Eastmoney {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if quoteURL == "" {
		quoteURL = "https://push2.eastmoney.com/api/qt/stock/get"
	}
	if klineURL == "" {
		klineURL = "https://push2his.eastmoney.com/api/qt/stock/kline/get"
	}
	if stockBasicURL == "" {
		stockBasicURL = defaultEastmoneyStockBasicURL
	}
	ut = strings.TrimSpace(ut)
	if ut == "" {
		ut = defaultEastmoneyUT
	}
	return &Eastmoney{
		client:   &http.Client{Timeout: timeout, CheckRedirect: eastmoneyRedirectPolicy},
		quoteURL: quoteURL, klineURL: klineURL, stockBasicURL: stockBasicURL, ut: ut,
	}
}
func (p *Eastmoney) Name() string { return "eastmoney" }

// FetchStockBasic retrieves low-frequency A-share identity and listing metadata from Eastmoney F10.
func (p *Eastmoney) FetchStockBasic(ctx context.Context, rawSymbol string) (*model.StockBasic, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}

	fetchRow := func(reportName string) (map[string]json.RawMessage, error) {
		params := url.Values{
			"reportName":   {reportName},
			"columns":      {"ALL"},
			"quoteColumns": {""},
			"filter":       {fmt.Sprintf(`(SECUCODE="%s")`, symbol)},
			"pageNumber":   {"1"},
			"pageSize":     {"1"},
			"sortTypes":    {""},
			"sortColumns":  {""},
			"source":       {"HSF10"},
			"client":       {"PC"},
			"v":            {strconv.FormatInt(time.Now().UnixMilli(), 10)},
		}
		var payload struct {
			Result *struct {
				Data []map[string]json.RawMessage `json:"data"`
			} `json:"result"`
		}
		u, err := url.Parse(p.stockBasicURL)
		if err != nil {
			return nil, fmt.Errorf("eastmoney stock-basic endpoint URL: %w", err)
		}
		u.RawQuery = params.Encode()
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
		req.Header.Set("Referer", "https://emweb.securities.eastmoney.com/")
		req.Header.Set("Origin", "https://emweb.securities.eastmoney.com")
		req.Header.Set("Accept", "application/json, text/plain, */*")
		resp, err := p.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("eastmoney stock-basic request: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
			return nil, fmt.Errorf("eastmoney stock-basic HTTP status %d", resp.StatusCode)
		}
		body, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
		if err != nil {
			return nil, fmt.Errorf("read Eastmoney stock-basic payload: %w", err)
		}
		if err := decodeSingleJSON(body, &payload); err != nil {
			return nil, fmt.Errorf("decode Eastmoney stock-basic JSON: %w", err)
		}
		if payload.Result == nil || len(payload.Result.Data) == 0 {
			return nil, fmt.Errorf("eastmoney stock-basic report %s returned no row for %s", reportName, symbol)
		}
		row := payload.Result.Data[0]
		field := func(name string) string {
			raw, ok := row[name]
			if !ok || len(raw) == 0 || string(raw) == "null" {
				return ""
			}
			var value string
			if json.Unmarshal(raw, &value) == nil {
				return strings.TrimSpace(value)
			}
			return strings.TrimSpace(strings.Trim(string(raw), `"`))
		}
		providerSymbol := field("SECUCODE")
		if providerSymbol == "" {
			providerSymbol = field("SECURITY_CODE")
			if len(providerSymbol) == 6 {
				if strings.HasPrefix(providerSymbol, "6") {
					providerSymbol += ".SH"
				} else if strings.HasPrefix(providerSymbol, "0") || strings.HasPrefix(providerSymbol, "3") {
					providerSymbol += ".SZ"
				}
			}
		}
		canonical, symbolErr := normalize.NormalizeSymbol(providerSymbol)
		if symbolErr != nil || canonical != symbol {
			return nil, fmt.Errorf("eastmoney stock-basic symbol mismatch in %s: requested %s, returned %q", reportName, symbol, providerSymbol)
		}
		return row, nil
	}

	readField := func(row map[string]json.RawMessage, name string) string {
		raw, ok := row[name]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			return ""
		}
		var value string
		if json.Unmarshal(raw, &value) == nil {
			return strings.TrimSpace(value)
		}
		return strings.TrimSpace(strings.Trim(string(raw), `"`))
	}

	// 发行信息表有上市日期，但没有稳定的 A 股简称字段。
	issueRow, err := fetchRow("RPT_PCF10_ORG_ISSUEINFO")
	if err != nil {
		return nil, err
	}
	name := readField(issueRow, "SECURITY_NAME_ABBR")
	if name == "" {
		// 公司概况表提供 STR_NAMEA/ORG_NAME；独立查询避免把股票代码误当成名称。
		orgRow, orgErr := fetchRow("RPT_F10_BASIC_ORGINFO")
		if orgErr != nil {
			return nil, orgErr
		}
		for _, key := range []string{"STR_NAMEA", "SECURITY_NAME_ABBR", "ORG_NAME"} {
			if name = readField(orgRow, key); name != "" {
				break
			}
		}
	}
	if name == "" {
		return nil, fmt.Errorf("eastmoney stock-basic name is empty for %s", symbol)
	}
	ipoDate, err := parseEastmoneyProfileDate(readField(issueRow, "LISTING_DATE"))
	if err != nil {
		return nil, err
	}
	outDate, err := parseEastmoneyProfileDate(readField(issueRow, "OUT_DATE"))
	if err != nil {
		return nil, err
	}
	status, statusLabel := 1, "listed"
	if outDate != "" {
		status, statusLabel = 0, "delisted"
	}
	return &model.StockBasic{
		Symbol: symbol, Name: name, IPODate: ipoDate, OutDate: outDate,
		SecurityType: 1, SecurityTypeLabel: "stock",
		ListingStatus: status, ListingStatusLabel: statusLabel,
		Source: p.Name(), SourceVersion: "eastmoney-f10-issueinfo-orginfo-v1",
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityValid,
		NormalizationVer: normalize.Version,
	}, nil
}

func parseEastmoneyProfileDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "-" || strings.EqualFold(value, "null") {
		return "", nil
	}
	for _, layout := range []string{
		"2006-01-02", "2006-01-02 15:04:05", "2006-01-02T15:04:05",
		"2006/01/02", "20060102", time.RFC3339,
	} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	return "", fmt.Errorf("eastmoney stock-basic date %q has an unsupported format", value)
}

func (p *Eastmoney) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	secid, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	q := url.Values{}
	q.Set("secid", secid)
	q.Set("fields", "f43,f44,f45,f46,f47,f48,f50,f57,f58,f60,f86,f169,f170,f168")
	q.Set("fltt", "2") // request formatted prices in yuan and percentages in percentage points
	q.Set("invt", "2")
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	var payload struct {
		RC   *int                       `json:"rc"`
		Data map[string]json.RawMessage `json:"data"`
	}
	if err := p.getJSON(ctx, p.quoteURL, q, &payload); err != nil {
		return nil, err
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney quote provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil {
		return nil, errors.New("eastmoney returned an empty quote")
	}
	getString := func(key string) string {
		var s string
		if json.Unmarshal(payload.Data[key], &s) == nil {
			return s
		}
		var n json.Number
		dec := json.NewDecoder(strings.NewReader(string(payload.Data[key])))
		dec.UseNumber()
		if dec.Decode(&n) == nil {
			return n.String()
		}
		return ""
	}
	reportedSymbol := strings.TrimSpace(getString("f57"))
	if reportedSymbol != "" {
		canonical, canonicalErr := normalize.NormalizeSymbol(reportedSymbol)
		if canonicalErr != nil || canonical != symbol {
			return nil, fmt.Errorf("Eastmoney quote symbol mismatch: requested %s got %q", symbol, reportedSymbol)
		}
	}
	getNumber := func(key string) (*float64, error) {
		s := getString(key)
		if s == "" || s == "-" || s == "null" {
			return nil, nil
		}
		v, e := strconv.ParseFloat(s, 64)
		if e != nil || math.IsNaN(v) || math.IsInf(v, 0) {
			return nil, fmt.Errorf("invalid Eastmoney numeric field %s", key)
		}
		return &v, nil
	}
	price, err := getNumber("f43")
	if err != nil {
		return nil, err
	}
	high, err := getNumber("f44")
	if err != nil {
		return nil, err
	}
	low, err := getNumber("f45")
	if err != nil {
		return nil, err
	}
	open, err := getNumber("f46")
	if err != nil {
		return nil, err
	}
	prev, err := getNumber("f60")
	if err != nil {
		return nil, err
	}
	change, err := getNumber("f169")
	if err != nil {
		return nil, err
	}
	pct, err := getNumber("f170")
	if err != nil {
		return nil, err
	}
	rawVol, err := getNumber("f47")
	if err != nil {
		return nil, err
	}
	var volume *int64
	var volumePtr *float64
	if rawVol != nil {
		shares, conversionErr := normalize.HandsToShares(*rawVol)
		if conversionErr != nil {
			return nil, fmt.Errorf("normalize Eastmoney quote volume: %w", conversionErr)
		}
		volume = &shares
		volumePtr = rawVol
	}
	rawAmount, err := getNumber("f48")
	if err != nil {
		return nil, err
	}
	amount := rawAmount // Eastmoney quote amount is CNY
	volumeRatio, err := getNumber("f50")
	if err != nil {
		return nil, err
	}
	turnoverRate, err := getNumber("f168")
	if err != nil {
		return nil, err
	}
	name := getString("f58")
	providerTime := parseEastmoneyTime(getString("f86"))
	quote := &model.Quote{Symbol: symbol, Name: name, CurrentPrice: price, Change: change, ChangePercent: pct, Open: open, High: high, Low: low, PrevClose: prev, Volume: volume, Amount: amount, VolumeRatio: volumeRatio, TurnoverRate: turnoverRate, QuoteTime: providerTime, QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: time.Now().UTC(), NormalizedAt: time.Now().UTC(), Source: p.Name(), SourceVersion: eastmoneyVersion, RawVolume: volumePtr, RawVolumeUnit: "hands", RawAmount: rawAmount, RawAmountUnit: optionalUnit(rawAmount, "CNY"), RawAmountField: optionalUnitName(rawAmount, "f48"), VolumeUnit: "shares", AmountUnit: optionalUnit(amount, "CNY"), AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
	if reportedSymbol == "" {
		quote.QualityStatus = model.QualityDegraded
		quote.QualityReasons = append(quote.QualityReasons, "provider_symbol_missing_assumed_from_request")
	}
	if providerTime == nil {
		quote.QualityStatus = model.QualityDegraded
		quote.QualityReasons = append(quote.QualityReasons, "provider_timestamp_missing")
	}
	if name == "" {
		quote.QualityReasons = append(quote.QualityReasons, "name_missing")
		quote.QualityStatus = model.QualityDegraded
	}
	if err := normalize.ValidateQuote(quote); err != nil {
		return nil, fmt.Errorf("Eastmoney quote data contract: %w", err)
	}
	return quote, nil
}

func (p *Eastmoney) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	secid, err := eastmoneySecID(symbol)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	beg, end, err := eastmoneyDateRange(startDate, endDate, limit, time.Now())
	if err != nil {
		return nil, err
	}
	fqt := "1"
	basis := adjustment
	switch adjustment {
	case model.AdjustmentForward:
		fqt = "1"
	case model.AdjustmentBackward:
		fqt = "2"
	case model.AdjustmentNone:
		fqt = "0"
	default:
		return nil, fmt.Errorf("unsupported adjustment basis %q", adjustment)
	}
	q := url.Values{}
	q.Set("secid", secid)
	q.Set("fields1", "f1,f2,f3,f4,f5,f6")
	q.Set("fields2", "f51,f52,f53,f54,f55,f56,f57,f58,f59,f60,f61")
	q.Set("klt", "101")
	q.Set("fqt", fqt)
	q.Set("beg", beg)
	q.Set("end", end)
	q.Set("lmt", strconv.Itoa(limit))
	q.Set("ut", p.ut)
	q.Set("_", strconv.FormatInt(time.Now().UnixMilli(), 10))
	var payload struct {
		RC   *int `json:"rc"`
		Data *struct {
			Code   string   `json:"code"`
			Name   string   `json:"name"`
			KLines []string `json:"klines"`
		} `json:"data"`
	}
	if err := p.getJSON(ctx, p.klineURL, q, &payload); err != nil {
		return nil, err
	}
	if payload.RC != nil && *payload.RC != 0 {
		return nil, fmt.Errorf("Eastmoney daily-bars provider error rc=%d", *payload.RC)
	}
	if payload.Data == nil {
		return nil, errors.New("eastmoney returned empty daily bars")
	}
	if reported := strings.TrimSpace(payload.Data.Code); reported != "" {
		reportedSymbol, normalizeErr := normalize.NormalizeSymbol(reported)
		if normalizeErr != nil || reportedSymbol != symbol {
			return nil, fmt.Errorf("Eastmoney daily-bars symbol mismatch: requested %s got %q", symbol, reported)
		}
	}
	out := make([]model.DailyBar, 0, len(payload.Data.KLines))
	rejectedRows := 0
	for _, line := range payload.Data.KLines {
		parts := strings.Split(line, ",")
		if len(parts) < 7 {
			rejectedRows++
			continue
		}
		open, e1 := strconv.ParseFloat(parts[1], 64)
		closeV, e2 := strconv.ParseFloat(parts[2], 64)
		high, e3 := strconv.ParseFloat(parts[3], 64)
		low, e4 := strconv.ParseFloat(parts[4], 64)
		rawVol, e5 := strconv.ParseFloat(parts[5], 64)
		amount, e6 := strconv.ParseFloat(parts[6], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || e6 != nil ||
			!finiteNumber(open) || !finiteNumber(closeV) || !finiteNumber(high) || !finiteNumber(low) || !finiteNumber(amount) || amount < 0 {
			rejectedRows++
			continue
		}
		shares, conversionErr := normalize.HandsToShares(rawVol)
		if conversionErr != nil {
			rejectedRows++
			continue
		}
		amountValue := amount
		rawVolume := rawVol
		rawAmount := amount
		bar := model.DailyBar{Symbol: symbol, TradeDate: parts[0], Period: "1d", Open: open, High: high, Low: low, Close: closeV, Volume: &shares, Amount: &amountValue, AdjustmentBasis: basis, Source: p.Name(), SourceVersion: eastmoneyVersion, FetchedAt: time.Now().UTC(), NormalizedAt: time.Now().UTC(), RawVolume: &rawVolume, RawVolumeUnit: "hands", RawAmount: &rawAmount, RawAmountUnit: "CNY", RawAmountField: "kline_field_6", VolumeUnit: "shares", AmountUnit: "CNY", QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
		if len(parts) > 8 {
			if pct, e := strconv.ParseFloat(parts[8], 64); e == nil {
				bar.ChangePercent = &pct
			}
		}
		out = append(out, bar)
	}
	// 使用实际发给上游的查询区间（包括自动计算的回溯起始日）过滤结果。
	startISO := beg[:4] + "-" + beg[4:6] + "-" + beg[6:]
	endISO := end[:4] + "-" + end[4:6] + "-" + end[6:]
	return normalizeProviderDailyBars(symbol, startISO, endISO, limit, out, rejectedRows)
}

// eastmoneyDateRange preserves explicit caller ranges. When no start date is
// specified, it asks for a bounded lookback large enough to normally cover the
// requested trading-bar limit, while avoiding an unnecessary all-history scan.
// Four calendar days per requested bar plus a 30-day buffer accommodates
// weekends, exchange holidays, and short suspensions without relying on lmt alone.
func eastmoneyDateRange(startDate, endDate string, limit int, now time.Time) (string, string, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}

	end := now.In(loc)
	if endDate != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02", endDate, loc)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid end date: %w", parseErr)
		}
		end = parsed
	}

	start := end.AddDate(0, 0, -(limit*eastmoneyLookbackDaysPerBar + eastmoneyLookbackBufferDays))
	if startDate != "" {
		parsed, parseErr := time.ParseInLocation("2006-01-02", startDate, loc)
		if parseErr != nil {
			return "", "", fmt.Errorf("invalid start date: %w", parseErr)
		}
		start = parsed
	}
	if start.After(end) {
		return "", "", errors.New("start date must not be after end date")
	}
	return start.Format("20060102"), end.Format("20060102"), nil
}

func (p *Eastmoney) getJSON(ctx context.Context, base string, params url.Values, out any) error {
	u, err := url.Parse(base)
	if err != nil {
		return err
	}
	u.RawQuery = params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36")
	req.Header.Set("Referer", "https://quote.eastmoney.com/")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("eastmoney request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("eastmoney HTTP status %d", resp.StatusCode)
	}
	body, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return fmt.Errorf("read Eastmoney payload: %w", err)
	}
	if err := decodeSingleJSON(body, out); err != nil {
		return fmt.Errorf("decode Eastmoney JSON: %w", err)
	}
	return nil
}

func eastmoneySecID(symbol string) (string, error) {
	p := strings.Split(symbol, ".")
	code := p[0]
	exchange := ""
	if len(p) == 2 {
		exchange = p[1]
	}
	if exchange == "" {
		symbol2, err := normalize.NormalizeSymbol(code)
		if err != nil {
			return "", err
		}
		exchange = strings.Split(symbol2, ".")[1]
	}
	switch exchange {
	case "SH":
		return "1." + code, nil
	case "SZ":
		return "0." + code, nil
	case "BJ":
		return "0." + code, nil
	default:
		return "", fmt.Errorf("unsupported A-share exchange %q", exchange)
	}
}

func parseEastmoneyTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	// 常见响应将 f86 编码为上海本地时间；遇到数字型 Unix 时间戳时再按秒/毫秒处理。
	for _, layout := range []string{"20060102150405", "200601021504", "2006-01-02 15:04:05", "2006-01-02 15:04"} {
		if t, parseErr := time.ParseInLocation(layout, s, loc); parseErr == nil {
			t = t.UTC()
			return &t
		}
	}
	if epoch, parseErr := strconv.ParseInt(s, 10, 64); parseErr == nil {
		var t time.Time
		switch len(s) {
		case 10:
			t = time.Unix(epoch, 0).UTC()
		case 13:
			t = time.UnixMilli(epoch).UTC()
		default:
			return nil
		}
		// 避免把任意十位数字或明显错误的时间值当成有效报价时间。
		if t.Year() < 2000 || t.After(time.Now().Add(24*time.Hour)) {
			return nil
		}
		return &t
	}
	return nil
}

func finiteNumber(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) }

// eastmoneyRedirectPolicy 只跟随 HTTPS 下的东方财富自有域名重定向，兼容 push2 -> push2delay 的行情调度。
func eastmoneyRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) >= 3 {
		return errors.New("Eastmoney redirect limit exceeded")
	}
	if req.URL.Scheme == "https" && isEastmoneyHostname(req.URL.Hostname()) {
		return nil
	}
	// 对外部域名和 HTTPS 降级到 HTTP 的跳转，不发送第二次请求。
	return http.ErrUseLastResponse
}

func isEastmoneyHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(host)), ".")
	return host == "eastmoney.com" || strings.HasSuffix(host, ".eastmoney.com")
}
