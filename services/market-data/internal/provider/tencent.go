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

const tencentVersion = "tencent-finance-api-v1"

type Tencent struct {
	client       *http.Client
	quoteURL     string
	klineURL     string
	noneKlineURL string
	searchURL    string
}

func NewTencent(timeout time.Duration) *Tencent {
	return NewTencentConfigured(timeout, "", "", "")
}

func NewTencentConfigured(timeout time.Duration, quoteURL, klineURL, noneKlineURL string) *Tencent {
	return NewTencentConfiguredWithSearchURL(timeout, quoteURL, klineURL, noneKlineURL, "")
}

// NewTencentConfiguredWithSearchURL configures the Smartbox keyword-search endpoint independently.
func NewTencentConfiguredWithSearchURL(timeout time.Duration, quoteURL, klineURL, noneKlineURL, stockSearchURL string) *Tencent {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if quoteURL == "" {
		quoteURL = "https://web.sqt.gtimg.cn/utf8/q"
	}
	if klineURL == "" {
		klineURL = "https://web.ifzq.gtimg.cn/appstock/app/fqkline/get"
	}
	if noneKlineURL == "" {
		noneKlineURL = "https://web.ifzq.gtimg.cn/appstock/app/kline/kline"
	}
	if stockSearchURL == "" {
		stockSearchURL = defaultTencentStockSearchURL
	}
	return &Tencent{client: &http.Client{Timeout: timeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}, quoteURL: quoteURL, klineURL: klineURL, noneKlineURL: noneKlineURL, searchURL: stockSearchURL}
}
func (p *Tencent) Name() string { return "tencent" }

func (p *Tencent) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tencent stock quote only supports common A-share equities, got %q", rawSymbol)
	}
	apiCode, err := tencentCode(symbol)
	if err != nil {
		return nil, err
	}
	body, err := p.getText(ctx, p.quoteURL+"="+url.QueryEscape(apiCode))
	if err != nil {
		return nil, err
	}
	start := strings.Index(body, "\"")
	end := strings.LastIndex(body, "\"")
	if start < 0 || end <= start {
		return nil, errors.New("tencent response has no quoted payload")
	}
	fields := strings.Split(body[start+1:end], "~")
	if len(fields) < 35 {
		return nil, fmt.Errorf("tencent response has only %d fields", len(fields))
	}
	reportedSymbol := strings.TrimSpace(fields[2])
	if reportedSymbol != "" {
		canonical, canonicalErr := normalize.NormalizeSymbol(reportedSymbol)
		if canonicalErr != nil || canonical != symbol {
			return nil, fmt.Errorf("tencent quote symbol mismatch: requested %s got %q", symbol, reportedSymbol)
		}
	}
	val := func(i int) (float64, error) {
		if i >= len(fields) || strings.TrimSpace(fields[i]) == "" || strings.TrimSpace(fields[i]) == "-" {
			return 0, fmt.Errorf("Tencent field %d is missing", i)
		}
		return strconv.ParseFloat(strings.TrimSpace(fields[i]), 64)
	}
	price, e := val(3)
	if e != nil {
		return nil, e
	}
	prev, e := val(4)
	if e != nil {
		return nil, e
	}
	open, e := val(5)
	if e != nil {
		return nil, e
	}
	rawVol, e := val(6)
	if e != nil {
		return nil, e
	}
	change, e := val(31)
	if e != nil {
		return nil, e
	}
	pct, e := val(32)
	if e != nil {
		return nil, e
	}
	high, e := val(33)
	if e != nil {
		return nil, e
	}
	low, e := val(34)
	if e != nil {
		return nil, e
	}
	volume, rawVolumeUnit, volumeVerified, conversionErr := tencentQuoteVolume(fields, rawVol, price)
	if conversionErr != nil {
		return nil, fmt.Errorf("normalize Tencent quote volume: %w", conversionErr)
	}
	rawAmount, amount, amountField, rawAmountUnit := tencentQuoteAmount(fields, low, high, volume)
	fetchedAt := time.Now().UTC()
	var qtime *time.Time
	if len(fields) > 30 {
		qtime = parseTencentTime(fields[30])
	}
	qtime = validProviderSnapshotTime(qtime, fetchedAt)
	var turnoverRate *float64
	if parsed, parseErr := parsePositiveField(fields, 38); parseErr == nil {
		// Tencent field 38 is interpreted as turnover rate by the existing
		// cross-check logic. Keep it in percentage points and never infer volume_ratio.
		turnoverRate = &parsed
	}
	rv := rawVol
	q := &model.Quote{Symbol: symbol, Name: fields[1], CurrentPrice: &price, Change: &change, ChangePercent: &pct, Open: &open, High: &high, Low: &low, PrevClose: &prev, Volume: &volume, Amount: amount, TurnoverRate: turnoverRate, QuoteTime: qtime, QuoteTimeSemantics: "provider_snapshot_time", FetchedAt: fetchedAt, NormalizedAt: fetchedAt, Source: p.Name(), SourceVersion: tencentVersion, RawVolume: &rv, RawVolumeUnit: rawVolumeUnit, RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: amountField, VolumeUnit: "shares", AmountUnit: optionalUnit(amount, "CNY"), AdjustmentBasis: model.AdjustmentNone, QualityStatus: model.QualityValid, NormalizationVer: normalize.Version}
	if reportedSymbol == "" {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "provider_symbol_missing_assumed_from_request")
	}
	if amount == nil {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "amount_missing_or_unit_unconfirmed")
	} else if amountField == "f37" {
		// This legacy turnover field is lower precision in common Tencent layouts.
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "lower_precision_turnover_field_used")
	}
	if !volumeVerified {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "volume_unit_unverified")
	} else if strings.HasSuffix(rawVolumeUnit, "_inferred") {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "volume_unit_inferred_from_turnover_metrics")
	}
	if qtime == nil {
		q.QualityStatus = model.QualityDegraded
		q.QualityReasons = append(q.QualityReasons, "provider_timestamp_missing")
	}
	q.QualityReasons = append(q.QualityReasons, "volume_ratio_unsupported_by_tencent_quote_contract")
	if err := normalize.ValidateQuote(q); err != nil {
		return nil, fmt.Errorf("Tencent quote data contract: %w", err)
	}
	if !volumeVerified {
		return nil, errors.New("Tencent quote volume unit cannot be confirmed; refusing to assume 100-share lots")
	}
	// ValidateQuote 先校验源端涨跌字段；规范输出随后统一按最新价和昨收价计算。
	canonicalChange := *q.CurrentPrice - *q.PrevClose
	canonicalChangePercent := canonicalChange / *q.PrevClose * 100
	q.Change = &canonicalChange
	q.ChangePercent = &canonicalChangePercent
	return q, nil
}

func (p *Tencent) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := normalize.NormalizeSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tencent daily bars only support common A-share equities, got %q", rawSymbol)
	}
	apiCode, err := tencentCode(symbol)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 400
	}
	if limit > 2000 {
		limit = 2000
	}
	start, end := startDate, endDate
	if start != "" {
		if _, e := time.Parse("2006-01-02", start); e != nil {
			return nil, fmt.Errorf("invalid start date: %w", e)
		}
	}
	if end != "" {
		if _, e := time.Parse("2006-01-02", end); e != nil {
			return nil, fmt.Errorf("invalid end date: %w", e)
		}
	}
	// 日期边界和区间错误必须在发起上游请求前拒绝，避免浪费调用额度并防止未来日期进入日 K 链路。
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	if start != "" && start > today {
		return nil, errors.New("Tencent start date must not be after today in Asia/Shanghai")
	}
	if end != "" && end > today {
		return nil, errors.New("Tencent end date must not be after today in Asia/Shanghai")
	}
	if start != "" && end != "" && start > end {
		return nil, errors.New("start date must not be after end date")
	}
	adjustCode, key := "", "day"
	endpoint := p.noneKlineURL
	switch adjustment {
	case model.AdjustmentForward:
		adjustCode = "qfq"
		key = "qfqday"
		endpoint = p.klineURL
	case model.AdjustmentBackward:
		adjustCode = "hfq"
		key = "hfqday"
		endpoint = p.klineURL
	case model.AdjustmentNone:
	default:
		return nil, fmt.Errorf("unsupported adjustment basis %q", adjustment)
	}
	param := strings.Join([]string{apiCode, "day", start, end, strconv.Itoa(limit)}, ",")
	if adjustCode != "" {
		param += "," + adjustCode
	}
	body, err := p.getJSONBytes(ctx, endpoint+"?param="+url.QueryEscape(param))
	if err != nil {
		return nil, err
	}
	var root struct {
		Data map[string]map[string]json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("decode Tencent K-line JSON: %w", err)
	}
	obj := root.Data[apiCode]
	if len(obj) == 0 {
		return nil, errors.New("tencent returned no K-line object")
	}
	rawRows := obj[key]
	if len(rawRows) == 0 && adjustment == model.AdjustmentNone {
		rawRows = obj["day"]
	}
	var rows [][]json.RawMessage
	if err := json.Unmarshal(rawRows, &rows); err != nil {
		return nil, fmt.Errorf("decode Tencent %s rows: %w", key, err)
	}
	out := make([]model.DailyBar, 0, len(rows))
	rejectedRows := 0
	for _, row := range rows {
		if len(row) < 6 {
			rejectedRows++
			continue
		}
		// Only the first six columns are mandatory and stable across Tencent
		// K-line variants. Some responses have extra fields that can be objects
		// (for example corporate-action metadata); do not discard a valid candle
		// merely because an optional trailing field is not scalar text.
		vals := make([]string, 6)
		valid := true
		for i := 0; i < 6; i++ {
			var value string
			if json.Unmarshal(row[i], &value) == nil {
				vals[i] = value
				continue
			}
			var number json.Number
			decoder := json.NewDecoder(strings.NewReader(string(row[i])))
			decoder.UseNumber()
			if decoder.Decode(&number) == nil {
				vals[i] = number.String()
			} else {
				valid = false
				break
			}
		}
		if !valid {
			rejectedRows++
			continue
		}
		open, e1 := strconv.ParseFloat(vals[1], 64)
		closeV, e2 := strconv.ParseFloat(vals[2], 64)
		high, e3 := strconv.ParseFloat(vals[3], 64)
		low, e4 := strconv.ParseFloat(vals[4], 64)
		rawVol, e5 := strconv.ParseFloat(vals[5], 64)
		if e1 != nil || e2 != nil || e3 != nil || e4 != nil || e5 != nil || !finiteNumber(open) || !finiteNumber(closeV) || !finiteNumber(high) || !finiteNumber(low) {
			rejectedRows++
			continue
		}
		shares, conversionErr := tencentDailyVolumeToShares(rawVol, symbol)
		if conversionErr != nil {
			rejectedRows++
			continue
		}
		rv := rawVol
		rawVolumeUnit := "hands"
		code := strings.Split(symbol, ".")[0]
		if strings.HasPrefix(code, "688") || strings.HasPrefix(code, "689") {
			rawVolumeUnit = "shares"
		}
		// Tencent's public front-end K-line payload has six observed fields:
		// date, open, close, high, low and volume. There is no official public
		// field contract for trailing values, so never infer turnover from row[6].
		// Missing amount must remain explicit rather than being fabricated.
		var rawAmount *float64
		var amount *float64
		quality := model.QualityDegraded
		reasons := []string{"tencent_daily_kline_has_no_verified_amount_field"}
		amountUnit := "unknown"
		b := model.DailyBar{Symbol: symbol, TradeDate: vals[0], Period: "1d", Open: open, High: high, Low: low, Close: closeV, Volume: &shares, Amount: amount, AdjustmentBasis: adjustment, Source: p.Name(), SourceVersion: tencentVersion, FetchedAt: time.Now().UTC(), NormalizedAt: time.Now().UTC(), RawVolume: &rv, RawVolumeUnit: rawVolumeUnit, RawAmount: rawAmount, RawAmountUnit: optionalUnit(rawAmount, "CNY"), RawAmountField: optionalUnitName(rawAmount, "kline_field_6"), VolumeUnit: "shares", AmountUnit: amountUnit, QualityStatus: quality, QualityReasons: reasons, NormalizationVer: normalize.Version}
		out = append(out, b)
	}

	return normalizeProviderDailyBars(symbol, start, end, limit, out, rejectedRows)
}

// Tencent quote payloads expose turnover at field 35's price/volume/amount
// tuple and, on older layouts, field 37 in 10,000 CNY. Field 57 is not a
// documented turnover source in the reference adapter and must not be used.
// Every candidate is checked against price range and normalized volume before
// being accepted; uncertain values remain missing rather than being rescaled.
func tencentQuoteVolume(fields []string, rawVolume, price float64) (int64, string, bool, error) {
	if math.IsNaN(rawVolume) || math.IsInf(rawVolume, 0) || rawVolume < 0 {
		return 0, "unknown", false, errors.New("raw Tencent quote volume must be finite and non-negative")
	}
	if rawVolume >= float64(math.MaxInt64)/100 {
		return 0, "unknown", false, errors.New("raw Tencent quote volume overflows normalized shares")
	}
	// Tencent's public field 6 does not have a reliable unit declaration across
	// observed quote payloads. Infer shares vs lots only when turnover rate and
	// circulating market value make one interpretation substantially more likely.
	turnoverRate, rateErr := parsePositiveField(fields, 38)
	circMVEight, capErr := parsePositiveField(fields, 44)
	if rateErr == nil && capErr == nil && price > 0 && turnoverRate <= 100 {
		expectedShares := (circMVEight * 100_000_000 / price) * turnoverRate / 100
		if expectedShares > 0 && !math.IsInf(expectedShares, 0) && !math.IsNaN(expectedShares) {
			sharesCandidate := rawVolume
			lotsCandidate := rawVolume * 100
			sharesError := math.Abs(sharesCandidate-expectedShares) / expectedShares
			lotsError := math.Abs(lotsCandidate-expectedShares) / expectedShares
			if math.Min(sharesError, lotsError) <= 0.5 {
				if sharesError <= lotsError {
					shares, err := normalize.RawSharesToInt64(sharesCandidate)
					return shares, "shares_inferred", true, err
				}
				shares, err := normalize.HandsToShares(rawVolume)
				return shares, "hands_inferred", true, err
			}
		}
	}
	// Unit inference must be supported by both turnover rate and circulating
	// market value. If those fields are absent or contradictory, leave the
	// normalized value unavailable so FetchQuote can reject this provider result
	// and let the service try the next configured source.
	return 0, "unknown", false, nil
}

func parsePositiveField(fields []string, index int) (float64, error) {
	if index < 0 || index >= len(fields) {
		return 0, errors.New("field missing")
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(fields[index]), 64)
	if err != nil || !finiteNumber(v) || v <= 0 {
		return 0, errors.New("field is not a positive finite number")
	}
	return v, nil
}

func tencentQuoteAmount(fields []string, low, high float64, volume int64) (*float64, *float64, string, string) {
	// The reference provider parser uses field 35's price/volume/amount tuple as
	// the high-precision CNY amount. Field 37 is a lower-precision amount in
	// 10,000 CNY. Field 57 is not the amount field and must never be treated as one.
	if len(fields) > 35 {
		parts := strings.Split(fields[35], "/")
		if len(parts) >= 3 {
			text := strings.TrimSpace(parts[2])
			if value, err := strconv.ParseFloat(text, 64); err == nil && finiteNumber(value) && value >= 0 && finitePositiveRangeAmount(value, low, high, volume) {
				raw := value
				amount := value
				return &raw, &amount, "f35.amount", "CNY"
			}
		}
	}
	if len(fields) > 37 {
		text := strings.TrimSpace(fields[37])
		if text != "" && text != "-" && text != "null" {
			if value, err := strconv.ParseFloat(text, 64); err == nil && finiteNumber(value) && value >= 0 {
				cny := value * 10_000
				if finiteNumber(cny) && finitePositiveRangeAmount(cny, low, high, volume) {
					raw := value
					return &raw, &cny, "f37", "ten_thousand_CNY"
				}
			}
		}
	}
	return nil, nil, "", ""
}

func finitePositiveRangeAmount(amount, low, high float64, volume int64) bool {
	if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || low <= 0 || high < low {
		return false
	}
	if volume == 0 {
		return amount == 0
	}
	if amount <= 0 {
		return false
	}
	average := amount / float64(volume)
	return average >= low*0.5 && average <= high*2
}

func optionalUnitName(value *float64, unit string) string {
	if value == nil {
		return ""
	}
	return unit
}

func optionalUnit(value *float64, unit string) string {
	if value == nil {
		return ""
	}
	return unit
}

func (p *Tencent) getText(ctx context.Context, u string) (string, error) {
	b, err := p.getBytes(ctx, u)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
func (p *Tencent) getJSONBytes(ctx context.Context, u string) ([]byte, error) {
	return p.getBytes(ctx, u)
}
func (p *Tencent) getBytes(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; MarketDataService/1.0)")
	req.Header.Set("Referer", "https://gu.qq.com/")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("tencent request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("tencent HTTP status %d", resp.StatusCode)
	}
	return readBoundedBody(resp.Body, maxProviderPayloadBytes)
}
func tencentDailyVolumeToShares(raw float64, canonicalSymbol string) (int64, error) {
	code := strings.Split(canonicalSymbol, ".")[0]
	// The reference Python adapter explicitly documents that Tencent daily
	// K-lines for STAR Market (688/689) are already in shares. Other supported
	// A-share daily K-lines are reported in hands.
	if strings.HasPrefix(code, "688") || strings.HasPrefix(code, "689") {
		return normalize.RawSharesToInt64(raw)
	}
	return normalize.HandsToShares(raw)
}

func tencentCode(symbol string) (string, error) {
	parts := strings.Split(symbol, ".")
	code := parts[0]
	exchange := ""
	if len(parts) > 1 {
		exchange = parts[1]
	}
	if exchange == "BJ" {
		return "", errors.New("Tencent fallback does not support Beijing Stock Exchange symbols")
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
		return "sh" + code, nil
	case "SZ":
		return "sz" + code, nil
	default:
		return "", fmt.Errorf("Tencent unsupported exchange %s", exchange)
	}
}
func tencentChinaLocation(loadLocation func(string) (*time.Location, error)) *time.Location {
	loc, err := loadLocation("Asia/Shanghai")
	if err != nil || loc == nil {
		// 精简容器可能没有 zoneinfo 数据；腾讯返回的是中国本地行情时间，回退到固定 UTC+8。
		return time.FixedZone("CST", 8*60*60)
	}
	return loc
}

func parseTencentTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	loc := tencentChinaLocation(time.LoadLocation)
	for _, layout := range []string{"20060102150405", "200601021504", "2006-01-02 15:04:05"} {
		if t, e := time.ParseInLocation(layout, s, loc); e == nil {
			t = t.UTC()
			return &t
		}
	}
	return nil
}
