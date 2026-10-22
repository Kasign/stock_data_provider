package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const sinaVersion = "sina-finance-quote-v1"
const maxSinaResponseBytes = 1 << 20

// 新浪 Provider 仅提供实时行情。与东方财富不同，新浪响应第 8 个字段的成交量单位是“股”，不是“手”；因此不能将它注册为日 K Provider。
type Sina struct {
	client   *http.Client
	quoteURL string
}

func NewSina(timeout time.Duration, quoteURL string) *Sina {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	if strings.TrimSpace(quoteURL) == "" {
		quoteURL = "https://hq.sinajs.cn/list="
	}
	return &Sina{
		client: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		quoteURL: strings.TrimSpace(quoteURL),
	}
}

func (p *Sina) Name() string { return "sina" }

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
	// 新浪旧接口有时返回 GBK 编码，即使 HTTP 客户端按 UTF-8 处理也可能出现乱码。若名称编码无效，就不将该名称暴露给调用方；价格和成交量字段为 ASCII，仍可独立校验。
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
		Volume: &volume, Amount: amount, QuoteTime: quoteTime,
		FetchedAt: fetchedAt, NormalizedAt: fetchedAt,
		Source: p.Name(), SourceVersion: sinaVersion,
		RawVolume: &rawVolumeCopy, RawVolumeUnit: "shares",
		RawAmount: amount, RawAmountUnit: amountUnit, RawAmountField: "field_9",
		VolumeUnit: "shares", AmountUnit: amountUnit,
		AdjustmentBasis: model.AdjustmentNone, QualityStatus: quality,
		QualityReasons: reasons, NormalizationVer: normalize.Version,
	}
	return quote, nil
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
