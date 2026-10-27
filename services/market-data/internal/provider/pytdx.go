package provider

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/injoyai/ios"
	"github.com/injoyai/tdx"
	"github.com/injoyai/tdx/protocol"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const (
	pytdxVersion        = "pytdx-injoyai-v0.0.86"
	pytdxPageSize       = 800
	pytdxMaxHistoryPage = 40
)

// Pytdx 使用通达信协议的 Go 客户端获取国内 A 股行情和日 K。
// 使用原生 Go 客户端，不在生产请求路径中启动 Python。
type Pytdx struct {
	timeout   time.Duration
	rateMu    sync.Mutex
	lastCall  time.Time
	hosts     []string
	hostIndex int
}

func NewPytdx(timeout time.Duration, configuredHosts ...string) *Pytdx {
	if timeout <= 0 {
		timeout = 12 * time.Second
	}
	hosts := make([]string, 0, len(configuredHosts))
	for _, host := range configuredHosts {
		if host = strings.TrimSpace(host); host != "" {
			hosts = append(hosts, host)
		}
	}
	if len(hosts) == 0 {
		hosts = append(hosts, tdx.Hosts...)
	}
	return &Pytdx{timeout: timeout, hosts: hosts}
}

func (p *Pytdx) Name() string { return "pytdx" }

func (p *Pytdx) dial(ctx context.Context) (*tdx.Client, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(p.hosts) == 0 {
		return nil, errors.New("Pytdx has no configured domestic quote servers")
	}
	dialTimeout := p.timeout
	if dialTimeout > 1500*time.Millisecond {
		dialTimeout = 1500 * time.Millisecond
	}
	// Each dial attempt checks only a small, rotating subset. A dead public server
	// cannot stall a request while the library walks its entire built-in host list.
	dial := func(dialCtx context.Context) (ios.ReadWriteCloser, string, error) {
		p.rateMu.Lock()
		start := p.hostIndex % len(p.hosts)
		attempts := min(6, len(p.hosts))
		p.hostIndex = (start + attempts) % len(p.hosts)
		p.rateMu.Unlock()

		var lastErr error
		for i := 0; i < attempts; i++ {
			if err := dialCtx.Err(); err != nil {
				return nil, "", err
			}
			host := strings.TrimSpace(p.hosts[(start+i)%len(p.hosts)])
			if host == "" {
				continue
			}
			address := host
			if _, _, err := net.SplitHostPort(address); err != nil {
				address = net.JoinHostPort(strings.Trim(address, "[]"), "7709")
			}
			dialer := net.Dialer{Timeout: dialTimeout}
			conn, err := dialer.DialContext(dialCtx, "tcp", address)
			if err == nil {
				return conn, address, nil
			}
			lastErr = err
		}
		if lastErr == nil {
			lastErr = errors.New("no valid host in configured Pytdx server list")
		}
		return nil, "", fmt.Errorf("unable to connect to a configured TDX server: %w", lastErr)
	}
	client, err := tdx.DialWith(dial, tdx.WithRedial(), tdx.WithLevel(tdx.LevelError))
	if err != nil {
		return nil, fmt.Errorf("Pytdx connect to domestic quote server: %w", err)
	}
	client.SetTimeout(p.timeout)
	return client, nil
}

func (p *Pytdx) waitForRateLimit(ctx context.Context) error {
	p.rateMu.Lock()
	defer p.rateMu.Unlock()
	if !p.lastCall.IsZero() {
		delay := time.Until(p.lastCall.Add(150 * time.Millisecond))
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
	p.lastCall = time.Now()
	return nil
}

func (p *Pytdx) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	symbol, err := pytdxDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := p.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	client, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	wireSymbol := pytdxWireSymbol(symbol)
	response, err := client.GetQuote(wireSymbol)
	if err != nil {
		return nil, fmt.Errorf("Pytdx quote request: %w", err)
	}
	var found *protocol.Quote
	parts := strings.Split(symbol, ".")
	code, exchange := parts[0], strings.ToLower(parts[1])
	for _, item := range response {
		if item != nil && strings.TrimSpace(item.Code) == code &&
			strings.EqualFold(item.Exchange.String(), exchange) && item.Kline != nil {
			found = item
			break
		}
	}
	if found == nil || found.Kline == nil {
		return nil, fmt.Errorf("Pytdx quote response did not contain requested symbol %s", symbol)
	}
	bar := found.Kline
	current := bar.Close.Float64()
	prevClose := bar.Last.Float64()
	open := bar.Open.Float64()
	high := bar.High.Float64()
	low := bar.Low.Float64()
	if !pytdxPositiveFinite(current) || !pytdxPositiveFinite(prevClose) ||
		!pytdxPositiveFinite(open) || !pytdxPositiveFinite(high) || !pytdxPositiveFinite(low) ||
		low > high || open < low || open > high || current < low || current > high {
		return nil, errors.New("Pytdx quote violates positive-price/OHLC invariants")
	}
	rawVolume := float64(bar.Volume)
	if !pytdxNonNegativeFinite(rawVolume) || math.Trunc(rawVolume) != rawVolume {
		return nil, errors.New("Pytdx quote contains an invalid volume in hands")
	}
	volume, err := normalize.HandsToShares(rawVolume)
	if err != nil {
		return nil, fmt.Errorf("normalize Pytdx quote volume: %w", err)
	}
	change := current - prevClose
	changePercent := change / prevClose * 100
	now := time.Now().UTC()
	amountValue := bar.Amount.Float64()
	var amount, rawAmount *float64
	amountUnit, rawAmountUnit, rawAmountField := "unknown", "", ""
	quality := model.QualityDegraded
	reasons := []string{
		"provider_quote_timestamp_not_exposed",
		"provider_name_not_exposed_by_quote_contract",
	}
	if pytdxNonNegativeFinite(amountValue) && amountValue > 0 {
		amount, rawAmount = &amountValue, &amountValue
		amountUnit, rawAmountUnit, rawAmountField = "CNY", "CNY", "amount"
	} else {
		reasons = append(reasons, "amount_missing_or_zero_in_quote")
	}
	rawVolumeCopy := rawVolume
	return &model.Quote{
		Symbol:           symbol,
		CurrentPrice:     &current,
		Change:           &change,
		ChangePercent:    &changePercent,
		Open:             &open,
		High:             &high,
		Low:              &low,
		PrevClose:        &prevClose,
		Volume:           &volume,
		Amount:           amount,
		FetchedAt:        now,
		NormalizedAt:     now,
		Source:           p.Name(),
		SourceVersion:    pytdxVersion,
		RawVolume:        &rawVolumeCopy,
		RawVolumeUnit:    "hands",
		RawAmount:        rawAmount,
		RawAmountUnit:    rawAmountUnit,
		RawAmountField:   rawAmountField,
		VolumeUnit:       "shares",
		AmountUnit:       amountUnit,
		AdjustmentBasis:  model.AdjustmentNone,
		QualityStatus:    quality,
		QualityReasons:   reasons,
		NormalizationVer: normalize.Version,
	}, nil
}

func (p *Pytdx) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	symbol, err := pytdxDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	wireSymbol := pytdxWireSymbol(symbol)
	switch adjustment {
	case model.AdjustmentNone, model.AdjustmentForward, model.AdjustmentBackward:
	default:
		return nil, fmt.Errorf("unsupported Pytdx adjustment %q", adjustment)
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil {
		return nil, fmt.Errorf("invalid Pytdx start date %q", startDate)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil {
		return nil, fmt.Errorf("invalid Pytdx end date %q", endDate)
	}
	if start.After(end) {
		return nil, errors.New("Pytdx start date must not be after end date")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	if end.Format("2006-01-02") > today {
		return nil, errors.New("Pytdx end date must not be after today in Asia/Shanghai")
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 2000 {
		return nil, errors.New("Pytdx daily-bar limit must not exceed 2000")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	client, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close()

	all := make(protocol.Klines, 0, 8000)
	historyComplete := false
	startDateReached := false
	for page := 0; page < pytdxMaxHistoryPage; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := p.waitForRateLimit(ctx); err != nil {
			return nil, err
		}
		offset := uint16(page * pytdxPageSize)
		response, requestErr := client.GetKlineDay(wireSymbol, offset, pytdxPageSize)
		if requestErr != nil {
			return nil, fmt.Errorf("Pytdx daily K-line page %d: %w", page+1, requestErr)
		}
		if response == nil || len(response.List) == 0 {
			historyComplete = true
			break
		}
		// TDX 的 start 是相对于最新数据的偏移；合并时去重检查并按交易日重新排序。
		all = append(append(protocol.Klines(nil), response.List...), all...)
		oldest := response.List[0].Time.In(loc).Format("2006-01-02")
		for _, item := range response.List {
			itemDate := item.Time.In(loc).Format("2006-01-02")
			if itemDate < oldest {
				oldest = itemDate
			}
		}
		if adjustment != model.AdjustmentBackward && oldest <= start.Format("2006-01-02") {
			startDateReached = true
			break
		}
		if len(response.List) < pytdxPageSize {
			historyComplete = true
			break
		}
	}
	if adjustment == model.AdjustmentBackward && !historyComplete {
		return nil, fmt.Errorf("Pytdx history exceeded %d pages; refusing to return incomplete backward-adjusted data", pytdxMaxHistoryPage)
	}
	if adjustment != model.AdjustmentBackward && !historyComplete && !startDateReached {
		return nil, fmt.Errorf("Pytdx history scan reached safety limit of %d pages before requested start date", pytdxMaxHistoryPage)
	}
	if len(all) == 0 {
		return nil, errors.New("Pytdx returned no daily K-line rows")
	}
	sort.Slice(all, func(i, j int) bool { return all[i].Time.Before(all[j].Time) })
	for i := 1; i < len(all); i++ {
		if all[i].Time.In(loc).Format("2006-01-02") == all[i-1].Time.In(loc).Format("2006-01-02") {
			return nil, fmt.Errorf("Pytdx returned duplicate daily bars on %s", all[i].Time.In(loc).Format("2006-01-02"))
		}
	}

	adjusted := all
	var adjustmentMethod string
	switch adjustment {
	case model.AdjustmentNone:
		adjustmentMethod = "pytdx_raw_ohlc_no_adjustment"
	case model.AdjustmentForward, model.AdjustmentBackward:
		if err := p.waitForRateLimit(ctx); err != nil {
			return nil, err
		}
		gbbq, gbbqErr := client.GetGbbq(wireSymbol)
		if gbbqErr != nil {
			return nil, fmt.Errorf("Pytdx ex-rights/ex-dividend data required for %q adjustment: %w", adjustment, gbbqErr)
		}
		events := make(protocol.XRXDs, 0)
		if gbbq != nil {
			for _, event := range gbbq.List {
				if event != nil && event.IsXRXD() {
					events = append(events, event.XRXD())
				}
			}
		}
		factors := events.Pre(all).Factors()
		if adjustment == model.AdjustmentForward {
			adjusted = protocol.ApplyQFQ(all, factors)
			adjustmentMethod = "pytdx_qfq"
		} else {
			adjusted = protocol.ApplyHFQ(all, factors)
			adjustmentMethod = "pytdx_hfq"
		}
	}

	fetchedAt := time.Now().UTC()
	out := make([]model.DailyBar, 0, min(limit, len(adjusted)))
	for i, item := range adjusted {
		tradeDate := item.Time.In(loc).Format("2006-01-02")
		if tradeDate < start.Format("2006-01-02") || tradeDate > end.Format("2006-01-02") {
			continue
		}
		open := item.Open.Float64()
		high := item.High.Float64()
		low := item.Low.Float64()
		closePrice := item.Close.Float64()
		if !pytdxPositiveFinite(open) || !pytdxPositiveFinite(high) ||
			!pytdxPositiveFinite(low) || !pytdxPositiveFinite(closePrice) ||
			low > high || open < low || open > high || closePrice < low || closePrice > high {
			return nil, fmt.Errorf("Pytdx OHLC invariant failed on %s", tradeDate)
		}
		rawVolume := float64(item.Volume)
		if !pytdxNonNegativeFinite(rawVolume) || math.Trunc(rawVolume) != rawVolume {
			return nil, fmt.Errorf("Pytdx contains invalid volume on %s", tradeDate)
		}
		volume, err := normalize.HandsToShares(rawVolume)
		if err != nil {
			return nil, fmt.Errorf("normalize Pytdx volume on %s: %w", tradeDate, err)
		}
		amountValue := item.Amount.Float64()
		if !pytdxNonNegativeFinite(amountValue) {
			return nil, fmt.Errorf("Pytdx contains invalid amount on %s", tradeDate)
		}
		var changePercent *float64
		prevClose := item.Last.Float64()
		if !pytdxPositiveFinite(prevClose) && i > 0 {
			prevClose = adjusted[i-1].Close.Float64()
		}
		quality := model.QualityValid
		var reasons []string
		if pytdxPositiveFinite(prevClose) {
			value := (closePrice - prevClose) / prevClose * 100
			changePercent = &value
		} else {
			quality = model.QualityDegraded
			reasons = append(reasons, "provider_change_percent_unavailable_for_first_bar")
		}
		rawVolumeCopy := rawVolume
		rawAmountCopy := amountValue
		out = append(out, model.DailyBar{
			Symbol:           symbol,
			TradeDate:        tradeDate,
			Period:           "1d",
			Open:             open,
			High:             high,
			Low:              low,
			Close:            closePrice,
			Volume:           &volume,
			Amount:           &amountValue,
			ChangePercent:    changePercent,
			AdjustmentBasis:  adjustment,
			AdjustmentMethod: adjustmentMethod,
			Source:           p.Name(),
			SourceVersion:    pytdxVersion,
			FetchedAt:        fetchedAt,
			NormalizedAt:     time.Now().UTC(),
			RawVolume:        &rawVolumeCopy,
			RawVolumeUnit:    "hands",
			RawAmount:        &rawAmountCopy,
			RawAmountUnit:    "CNY",
			RawAmountField:   "amount",
			VolumeUnit:       "shares",
			AmountUnit:       "CNY",
			QualityStatus:    quality,
			QualityReasons:   reasons,
			NormalizationVer: normalize.Version,
		})
	}
	return normalizeProviderDailyBars(symbol, start.Format("2006-01-02"), end.Format("2006-01-02"), limit, out, 0)
}

func pytdxDomesticSymbol(raw string) (string, error) {
	symbol, err := normalize.NormalizeSymbol(raw)
	if err != nil {
		return "", err
	}
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") {
		return "", fmt.Errorf("Pytdx adapter only accepts domestic SH/SZ/BJ equities, got %q", raw)
	}
	wireSymbol := pytdxWireSymbol(symbol)
	if wireSymbol == "" || !protocol.IsStock(wireSymbol) {
		return "", fmt.Errorf("Pytdx adapter only accepts A-share equity codes, not index or unsupported security %q", raw)
	}
	return symbol, nil
}

// pytdxWireSymbol 将规范化证券代码转换为通达信要求的交易所前缀格式。
// 必须保留显式交易所后缀，不能仅凭 6 位数字重新推断交易所。
func pytdxWireSymbol(symbol string) string {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ""
	}
	return strings.ToLower(parts[1]) + parts[0]
}

func pytdxPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func pytdxNonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
