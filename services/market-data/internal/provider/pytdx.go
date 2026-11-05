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

	"github.com/Kasign/stock_data_provider/services/market-data/internal/model"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/normalize"
	"github.com/Kasign/stock_data_provider/services/market-data/internal/validation"
)

const (
	pytdxVersion        = "pytdx-injoyai-v0.0.86"
	pytdxPageSize       = 800
	pytdxMaxHistoryPage = 40
)

// Pytdx 使用通达信协议的 Go 客户端获取国内 A 股行情和日 K。
// 使用原生 Go 客户端，不在生产请求路径中启动 Python。
type Pytdx struct {
	timeout            time.Duration
	dailyClientFactory func(context.Context) (pytdxDailyClient, error)
	rateMu             sync.Mutex
	lastCall           time.Time
	hosts              []string
	hostIndex          int
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

// pytdxDailyClient 仅抽象日线复权链路依赖的方法，生产环境仍使用原生 tdx.Client。
type pytdxDailyClient interface {
	Close() error
	GetKlineDay(code string, start, count uint16) (*protocol.KlineResp, error)
	GetGbbq(code string) (*protocol.GbbqResp, error)
}

func (p *Pytdx) dailyClient(ctx context.Context) (pytdxDailyClient, error) {
	if p.dailyClientFactory != nil {
		return p.dailyClientFactory(ctx)
	}
	return p.dial(ctx)
}

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
	found, err := pytdxFindUniqueQuote(response, symbol)
	if err != nil {
		code := "invalid_quote_identity"
		if strings.Contains(err.Error(), "duplicate records") {
			code = "duplicate_identity"
		}
		addPytdxValidationIssue(ctx, 0, "symbol", "Code/Exchange", code, symbol, "tdx_quote_records", pytdxQuoteIdentityPreview(response), err.Error(), validation.SeverityError)
		return nil, err
	}
	if found == nil {
		code, message := "missing_quote", "Go 通达信协议响应未返回报价记录"
		actual := "empty"
		if len(response) > 0 {
			code, message = "identity_mismatch", "Go 通达信协议返回的证券代码或交易所与请求不一致"
			actual = pytdxQuoteIdentityPreview(response)
		}
		addPytdxValidationIssue(ctx, 0, "symbol", "Code/Exchange", code, symbol, "tdx_quote_records", actual, message, validation.SeverityError)
		return nil, fmt.Errorf("Pytdx quote response did not contain requested symbol %s", symbol)
	}
	if found.Kline == nil {
		addPytdxValidationIssue(ctx, 0, "quote", "Kline", "required_data_missing", "quote K-line values", "nil", "", "Go 通达信协议报价记录缺少行情字段", validation.SeverityError)
		return nil, fmt.Errorf("Pytdx quote response for %s has no K-line values", symbol)
	}
	bar := found.Kline
	return buildPytdxQuote(ctx, symbol, bar.Close.Float64(), bar.Last.Float64(), bar.Open.Float64(), bar.High.Float64(), bar.Low.Float64(), float64(bar.Volume), bar.Amount.Float64())
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

	client, err := p.dailyClient(ctx)
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
			tradeDate := all[i].Time.In(loc).Format("2006-01-02")
			addPytdxValidationIssue(ctx, i, "trade_date", "Time", "duplicate_trade_date", "one daily bar per trade date", "date", tradeDate, "Go 通达信协议响应包含重复交易日", validation.SeverityError)
			return nil, fmt.Errorf("Pytdx returned duplicate daily bars on %s", tradeDate)
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

	adjustmentRefDate := ""
	switch adjustment {
	case model.AdjustmentForward:
		adjustmentRefDate = all[len(all)-1].Time.In(loc).Format("2006-01-02")
	case model.AdjustmentBackward:
		adjustmentRefDate = all[0].Time.In(loc).Format("2006-01-02")
	}

	fetchedAt := time.Now().UTC()
	out := make([]model.DailyBar, 0, min(limit, len(adjusted)))
	for i, item := range adjusted {
		if item.Time.IsZero() {
			addPytdxValidationIssue(ctx, i, "trade_date", "Time", "invalid_date", "valid exchange timestamp", "zero_time", "", "Go 通达信协议响应缺少有效交易时间", validation.SeverityError)
			return nil, fmt.Errorf("Pytdx returned an invalid timestamp at row %d", i)
		}
		tradeDate := item.Time.In(loc).Format("2006-01-02")
		if tradeDate < start.Format("2006-01-02") || tradeDate > end.Format("2006-01-02") {
			continue
		}
		open := item.Open.Float64()
		high := item.High.Float64()
		low := item.Low.Float64()
		closePrice := item.Close.Float64()
		rawVolume := float64(item.Volume)
		amountValue := item.Amount.Float64()
		amountInfo, err := validatePytdxDailyBarValues(ctx, i, tradeDate, open, high, low, closePrice, rawVolume, amountValue)
		if err != nil {
			return nil, err
		}
		volume, err := normalize.HandsToShares(rawVolume)
		if err != nil {
			addPytdxValidationIssue(ctx, i, "volume", "Volume", "normalization_error", "convertible non-negative integer number of hands", "number", fmt.Sprintf("%g", rawVolume), "Go 通达信协议成交量无法转换为标准股数", validation.SeverityError)
			return nil, fmt.Errorf("normalize Pytdx volume on %s: %w", tradeDate, err)
		}
		var changePercent *float64
		prevClose := item.Last.Float64()
		if !pytdxPositiveFinite(prevClose) && i > 0 {
			prevClose = adjusted[i-1].Close.Float64()
		}
		quality := amountInfo.QualityStatus
		reasons := append([]string(nil), amountInfo.QualityReasons...)
		if pytdxPositiveFinite(prevClose) {
			value := (closePrice - prevClose) / prevClose * 100
			changePercent = &value
		} else {
			quality = model.QualityDegraded
			reasons = append(reasons, "provider_change_percent_unavailable_for_first_bar")
			addPytdxValidationIssue(ctx, i, "change_percent", "Last", "previous_close_unavailable", "positive previous close", "number", fmt.Sprintf("%g", item.Last.Float64()), "Go 通达信协议未提供可用于计算涨跌幅的前收盘价", validation.SeverityWarning)
		}
		rawVolumeCopy := rawVolume
		out = append(out, model.DailyBar{
			Symbol:            symbol,
			TradeDate:         tradeDate,
			Period:            "1d",
			Open:              open,
			High:              high,
			Low:               low,
			Close:             closePrice,
			Volume:            &volume,
			Amount:            amountInfo.Amount,
			ChangePercent:     changePercent,
			AdjustmentBasis:   adjustment,
			AdjustmentMethod:  adjustmentMethod,
			AdjustmentRefDate: adjustmentRefDate,
			Source:            p.Name(),
			SourceVersion:     pytdxVersion,
			FetchedAt:         fetchedAt,
			NormalizedAt:      time.Now().UTC(),
			RawVolume:         &rawVolumeCopy,
			RawVolumeUnit:     "hands",
			RawAmount:         amountInfo.RawAmount,
			RawAmountUnit:     amountInfo.RawAmountUnit,
			RawAmountField:    amountInfo.RawAmountField,
			VolumeUnit:        "shares",
			AmountUnit:        amountInfo.AmountUnit,
			QualityStatus:     quality,
			QualityReasons:    reasons,
			NormalizationVer:  normalize.Version,
		}))
	}
	return normalizeProviderDailyBarsWithReport(ctx, symbol, start.Format("2006-01-02"), end.Format("2006-01-02"), limit, out, 0)
}

func buildPytdxQuote(ctx context.Context, symbol string, current, prevClose, open, high, low, rawVolume, amountValue float64) (*model.Quote, error) {
	prices := []struct {
		field         string
		providerField string
		value         float64
	}{
		{field: "current_price", providerField: "Close", value: current},
		{field: "prev_close", providerField: "Last", value: prevClose},
		{field: "open", providerField: "Open", value: open},
		{field: "high", providerField: "High", value: high},
		{field: "low", providerField: "Low", value: low},
	}
	for _, item := range prices {
		if !pytdxPositiveFinite(item.value) {
			addPytdxValidationIssue(ctx, 0, item.field, item.providerField, "invalid_number", "finite positive price", "number", fmt.Sprintf("%g", item.value), "Go 通达信协议报价价格字段不是正有限数值", validation.SeverityError)
			return nil, fmt.Errorf("Pytdx quote %s is not a positive finite number", item.field)
		}
	}
	if low > high || open < low || open > high || current < low || current > high {
		addPytdxValidationIssue(ctx, 0, "ohlc", "Open/High/Low/Close", "ohlc_invariant_violation", "low <= open/current <= high", "number", fmt.Sprintf("open=%g; high=%g; low=%g; current=%g", open, high, low, current), "Go 通达信协议报价 OHLC 区间关系异常", validation.SeverityError)
		return nil, errors.New("Pytdx quote violates OHLC invariants")
	}
	if !pytdxNonNegativeFinite(rawVolume) || math.Trunc(rawVolume) != rawVolume {
		addPytdxValidationIssue(ctx, 0, "volume", "Volume", "invalid_number", "finite non-negative integer number of hands", "number", fmt.Sprintf("%g", rawVolume), "Go 通达信协议报价成交量必须是非负整数手", validation.SeverityError)
		return nil, errors.New("Pytdx quote contains an invalid volume in hands")
	}
	volume, err := normalize.HandsToShares(rawVolume)
	if err != nil {
		addPytdxValidationIssue(ctx, 0, "volume", "Volume", "normalization_error", "convertible non-negative integer number of hands", "number", fmt.Sprintf("%g", rawVolume), "Go 通达信协议报价成交量无法转换为标准股数", validation.SeverityError)
		return nil, fmt.Errorf("normalize Pytdx quote volume: %w", err)
	}

	change := current - prevClose
	changePercent := change / prevClose * 100
	now := time.Now().UTC()
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
		reasons = append(reasons, "amount_missing_or_invalid_in_quote")
		if !pytdxNonNegativeFinite(amountValue) {
			addPytdxValidationIssue(ctx, 0, "amount", "Amount", "invalid_optional_number", "finite non-negative amount in CNY", "number", fmt.Sprintf("%g", amountValue), "Go 通达信协议报价成交额不是非负有限数值，已置空并降级", validation.SeverityWarning)
		} else if rawVolume > 0 {
			addPytdxValidationIssue(ctx, 0, "amount", "Amount", "amount_missing_or_zero", "positive amount when volume is positive", "number", fmt.Sprintf("%g", amountValue), "Go 通达信协议报价成交额为零但成交量非零，已置空并降级", validation.SeverityWarning)
		}
	}
	rawVolumeCopy := rawVolume
	quote := &model.Quote{
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
		Source:           "pytdx",
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
	}
	report, validationErr := normalize.ValidateQuoteWithReport(quote)
	validation.AddContextReport(ctx, report)
	if validationErr != nil {
		return nil, fmt.Errorf("Pytdx quote data contract: %w", validationErr)
	}
	return quote, nil
}

func pytdxQuoteIdentityPreview(response []*protocol.Quote) string {
	for _, item := range response {
		if item == nil {
			continue
		}
		preview := strings.TrimSpace(item.Code + "." + item.Exchange.String())
		if len(preview) > 160 {
			return preview[:160]
		}
		return preview
	}
	return ""
}

func addPytdxValidationIssue(ctx context.Context, rowIndex int, field, providerField, code, expected, actualType, actualPreview, message string, severity validation.Severity) {
	row := rowIndex
	report := validation.NewReport(1)
	report.Add(validation.FieldIssue{
		Stage: validation.StageValidate, Field: field, ProviderField: providerField,
		Code: code, Expected: expected, ActualType: actualType, ActualPreview: actualPreview,
		Severity: severity, Message: message, RowIndex: &row,
	})
	validation.AddContextReport(ctx, report)
}

func validatePytdxDailyBarValues(ctx context.Context, rowIndex int, tradeDate string, open, high, low, closePrice, rawVolume, rawAmount float64) (pytdxDailyAmountInfo, error) {
	prices := []struct {
		field string
		value float64
	}{
		{field: "open", value: open},
		{field: "high", value: high},
		{field: "low", value: low},
		{field: "close", value: closePrice},
	}
	for _, item := range prices {
		if !pytdxPositiveFinite(item.value) {
			addPytdxValidationIssue(ctx, rowIndex, item.field, strings.Title(item.field), "invalid_number", "finite positive price", "number", fmt.Sprintf("%g", item.value), "Go 通达信协议日 K 价格字段不是正有限数值", validation.SeverityError)
			return pytdxDailyAmountInfo{}, fmt.Errorf("Pytdx %s price is invalid on %s", item.field, tradeDate)
		}
	}
	if low > high || open < low || open > high || closePrice < low || closePrice > high {
		addPytdxValidationIssue(ctx, rowIndex, "ohlc", "Open/High/Low/Close", "ohlc_invariant_violation", "low <= open/close <= high", "number", "", "Go 通达信协议日 K OHLC 区间关系异常", validation.SeverityError)
		return pytdxDailyAmountInfo{}, fmt.Errorf("Pytdx OHLC invariant failed on %s", tradeDate)
	}
	if !pytdxNonNegativeFinite(rawVolume) || math.Trunc(rawVolume) != rawVolume {
		addPytdxValidationIssue(ctx, rowIndex, "volume", "Volume", "invalid_number", "finite non-negative integer number of hands", "number", fmt.Sprintf("%g", rawVolume), "Go 通达信协议日 K 成交量必须是非负整数手", validation.SeverityError)
		return pytdxDailyAmountInfo{}, fmt.Errorf("Pytdx contains invalid volume on %s", tradeDate)
	}
	amountInfo, err := normalizePytdxDailyAmount(rawAmount, rawVolume)
	if err != nil {
		addPytdxValidationIssue(ctx, rowIndex, "amount", "Amount", "invalid_number", "finite non-negative amount in CNY", "number", fmt.Sprintf("%g", rawAmount), "Go 通达信协议日 K 成交额不是非负有限数值", validation.SeverityError)
		return pytdxDailyAmountInfo{}, fmt.Errorf("Pytdx contains invalid amount on %s: %w", tradeDate, err)
	}
	if amountInfo.Amount == nil && amountInfo.QualityStatus == model.QualityDegraded {
		addPytdxValidationIssue(ctx, rowIndex, "amount", "Amount/Volume", "amount_volume_mismatch", "amount and volume should be jointly present or zero", "number", fmt.Sprintf("amount=%g; volume_hands=%g", rawAmount, rawVolume), "成交额与成交量零值关系不一致，成交额已置空并降级", validation.SeverityWarning)
	}
	return amountInfo, nil
}

// pytdxDailyAmountInfo 在成交额与成交量明显矛盾时保留原始记录，将归一化成交额置空。
type pytdxDailyAmountInfo struct {
	Amount         *float64
	RawAmount      *float64
	AmountUnit     string
	RawAmountUnit  string
	RawAmountField string
	QualityStatus  model.QualityStatus
	QualityReasons []string
}

func normalizePytdxDailyAmount(rawAmount, rawVolume float64) (pytdxDailyAmountInfo, error) {
	if !pytdxNonNegativeFinite(rawAmount) {
		return pytdxDailyAmountInfo{}, errors.New("Pytdx raw amount must be finite and non-negative")
	}
	if !pytdxNonNegativeFinite(rawVolume) || math.Trunc(rawVolume) != rawVolume {
		return pytdxDailyAmountInfo{}, errors.New("Pytdx raw volume must be a finite non-negative integer number of hands")
	}

	rawCopy := rawAmount
	result := pytdxDailyAmountInfo{
		RawAmount: &rawCopy, AmountUnit: "CNY", RawAmountUnit: "CNY",
		RawAmountField: "amount", QualityStatus: model.QualityValid,
	}
	if (rawVolume == 0 && rawAmount > 0) || (rawVolume > 0 && rawAmount == 0) {
		result.AmountUnit = "unknown"
		result.QualityStatus = model.QualityDegraded
		result.QualityReasons = []string{"amount_volume_zero_mismatch_amount_omitted"}
		return result, nil
	}
	result.Amount = &rawCopy
	return result, nil
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
	if !isTushareAShareSymbol(symbol) {
		return "", fmt.Errorf("Pytdx adapter only accepts common A-share equities, got %q", raw)
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

// pytdxFindUniqueQuote 按证券代码和交易所定位报价；重复身份即拒绝，不能静默使用首条。
func pytdxFindUniqueQuote(response []*protocol.Quote, symbol string) (*protocol.Quote, error) {
	var found *protocol.Quote
	for _, item := range response {
		if !pytdxQuoteIdentityMatchesSymbol(item, symbol) {
			continue
		}
		if found != nil {
			return nil, fmt.Errorf("Pytdx quote response contains duplicate records for requested symbol %s", symbol)
		}
		found = item
	}
	return found, nil
}

func pytdxQuoteIdentityMatchesSymbol(item *protocol.Quote, symbol string) bool {
	if item == nil {
		return false
	}
	parts := strings.Split(symbol, ".")
	return len(parts) == 2 &&
		strings.TrimSpace(item.Code) == parts[0] &&
		strings.EqualFold(item.Exchange.String(), parts[1])
}

func pytdxQuoteMatchesSymbol(item *protocol.Quote, symbol string) bool {
	return item != nil && item.Kline != nil && pytdxQuoteIdentityMatchesSymbol(item, symbol)
}

func pytdxPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func pytdxNonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}
