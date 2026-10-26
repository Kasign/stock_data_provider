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

const (
	hithinkVersion          = "hithink-finance-rest-v1"
	hithinkMaxPagesPerRange = 40
	hithinkMaxRowsPerCall   = 20000
)

// Hithink 对接同花顺官方金融数据 REST API。接口需要 API Key，默认不开启。
// 认证只通过 X-api-key Header 发送，不将 Key 放入 URL、日志或错误文本。
type Hithink struct {
	client  *http.Client
	baseURL string
	apiKey  string

	rateMu   sync.Mutex
	lastCall time.Time
}

func NewHithinkConfigured(timeout time.Duration, baseURL, apiKey string) *Hithink {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://fuyao.aicubes.cn"
	}
	return &Hithink{
		client:  &http.Client{Timeout: timeout},
		baseURL: strings.TrimRight(strings.TrimSpace(baseURL), "/"),
		apiKey:  strings.TrimSpace(apiKey),
	}
}

func (p *Hithink) Name() string { return "hithink" }

// SearchStocks 可按中文名称、代码或 THSCode 查询 A 股证券目录；不返回或推算行情价格。
func (p *Hithink) SearchStocks(ctx context.Context, queryText string, limit int) (*model.StockSearchSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺证券检索接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	queryText = strings.TrimSpace(queryText)
	if queryText == "" {
		return nil, errors.New("同花顺证券检索词不能为空")
	}
	if limit <= 0 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := url.Values{}
	query.Set("q", queryText)
	query.Set("asset_type", "a-share")
	query.Set("limit", strconv.Itoa(limit))
	raw, err := p.get(ctx, "/api/meta/tickers/search", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺证券检索请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Item []struct {
			THSCode  string `json:"thscode"`
			Ticker   string `json:"ticker"`
			Name     string `json:"name"`
			Exchange string `json:"exchange"`
			AssetType string `json:"asset_type"`
		} `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺证券目录失败: %w", err)
	}
	now := time.Now().UTC()
	snapshot := &model.StockSearchSnapshot{
		Query: queryText, Items: make([]model.StockSearchItem, 0, len(data.Item)),
		Source: p.Name(), SourceVersion: hithinkVersion,
		FetchedAt: now, QualityStatus: model.QualityValid,
	}
	seen := make(map[string]struct{}, len(data.Item))
	for _, item := range data.Item {
		if !strings.EqualFold(strings.TrimSpace(item.AssetType), "a-share") {
			continue
		}
		symbol, symbolErr := hithinkDomesticSymbol(item.THSCode)
		if symbolErr != nil {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = append(snapshot.QualityReasons, "one_or_more_provider_symbols_were_invalid")
			continue
		}
		if _, exists := seen[symbol]; exists {
			continue
		}
		seen[symbol] = struct{}{}
		code := strings.TrimSpace(item.Ticker)
		if code == "" {
			code = strings.Split(symbol, ".")[0]
		}
		exchange := strings.ToUpper(strings.TrimSpace(item.Exchange))
		if exchange == "" {
			parts := strings.Split(symbol, ".")
			if len(parts) == 2 {
				exchange = parts[1]
			}
		}
		snapshot.Items = append(snapshot.Items, model.StockSearchItem{
			Symbol: symbol, Code: code, Name: strings.TrimSpace(item.Name), Exchange: exchange,
		})
	}
	snapshot.Total = len(snapshot.Items)
	if snapshot.Total == 0 {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = append(snapshot.QualityReasons, "no_a_share_matches")
	}
	return snapshot, nil
}

type hithinkIndexDefinition struct {
	Symbol string
	Code   string
	Name   string
}

var hithinkCoreIndexDefinitions = []hithinkIndexDefinition{
	{Symbol: "000001.SH", Code: "000001", Name: "上证指数"},
	{Symbol: "399001.SZ", Code: "399001", Name: "深证成指"},
	{Symbol: "399006.SZ", Code: "399006", Name: "创业板指"},
	{Symbol: "000300.SH", Code: "000300", Name: "沪深300"},
	{Symbol: "000016.SH", Code: "000016", Name: "上证50"},
	{Symbol: "000905.SH", Code: "000905", Name: "中证500"},
	{Symbol: "000688.SH", Code: "000688", Name: "科创50"},
}

// FetchIndexQuotes 查询固定白名单核心指数，绝不发送无 thscodes 的全市场请求。
func (p *Hithink) FetchIndexQuotes(ctx context.Context) (*model.IndexQuoteSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺指数行情接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	symbols := make([]string, 0, len(hithinkCoreIndexDefinitions))
	allowed := make(map[string]hithinkIndexDefinition, len(hithinkCoreIndexDefinitions))
	for _, definition := range hithinkCoreIndexDefinitions {
		symbols = append(symbols, definition.Symbol)
		allowed[definition.Symbol] = definition
	}
	query := url.Values{}
	query.Set("thscodes", strings.Join(symbols, ","))
	raw, err := p.get(ctx, "/api/a-share-index/prices/snapshot", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺指数行情请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp *int64 `json:"timestamp"`
		Item []struct {
			THSCode       string   `json:"thscode"`
			Name          string   `json:"name"`
			LastPrice     *float64 `json:"last_price"`
			PriceChange   *float64 `json:"price_change"`
			ChangeRatio   *float64 `json:"price_change_ratio_pct"`
			OpenPrice     *float64 `json:"open_price"`
			HighPrice     *float64 `json:"high_price"`
			LowPrice      *float64 `json:"low_price"`
			PreviousPrice *float64 `json:"prev_price"`
			Turnover      *float64 `json:"turnover"`
		} `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺指数行情失败: %w", err)
	}
	if len(data.Item) == 0 {
		return nil, errors.New("同花顺指数行情未返回任何记录")
	}
	now := time.Now().UTC()
	snapshot := &model.IndexQuoteSnapshot{
		Items: make([]model.MarketIndexQuote, 0, len(hithinkCoreIndexDefinitions)),
		Source: p.Name(), SourceVersion: hithinkVersion, FetchedAt: now, QualityStatus: model.QualityValid,
	}
	var quoteTime *time.Time
	quoteTimeSemantics := ""
	if data.Timestamp != nil && *data.Timestamp > 0 {
		value := time.UnixMilli(*data.Timestamp).UTC()
		if !value.After(now.Add(5*time.Minute)) {
			quoteTime = &value
			quoteTimeSemantics = "provider_snapshot_time"
		}
	}
	if quoteTime == nil {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = append(snapshot.QualityReasons, "provider_snapshot_timestamp_missing_or_invalid")
	}
	bySymbol := make(map[string]model.MarketIndexQuote, len(hithinkCoreIndexDefinitions))
	seen := make(map[string]struct{}, len(hithinkCoreIndexDefinitions))
	for _, item := range data.Item {
		symbol := strings.ToUpper(strings.TrimSpace(item.THSCode))
		definition, ok := allowed[symbol]
		if !ok {
			continue
		}
		if _, duplicate := seen[symbol]; duplicate {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "duplicate_index_row_dropped")
			continue
		}
		seen[symbol] = struct{}{}
		if item.LastPrice == nil || item.PreviousPrice == nil || item.OpenPrice == nil || item.HighPrice == nil || item.LowPrice == nil ||
			!hithinkPositiveFinite(*item.LastPrice) || !hithinkPositiveFinite(*item.PreviousPrice) ||
			!hithinkPositiveFinite(*item.OpenPrice) || !hithinkPositiveFinite(*item.HighPrice) || !hithinkPositiveFinite(*item.LowPrice) ||
			*item.LowPrice > *item.HighPrice || *item.OpenPrice < *item.LowPrice || *item.OpenPrice > *item.HighPrice ||
			*item.LastPrice < *item.LowPrice || *item.LastPrice > *item.HighPrice {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "one_or_more_index_rows_have_invalid_required_prices")
			continue
		}
		change := *item.LastPrice - *item.PreviousPrice
		if item.PriceChange != nil && hithinkFinite(*item.PriceChange) {
			change = *item.PriceChange
		}
		changePercent := change / *item.PreviousPrice * 100
		if item.ChangeRatio != nil && hithinkFinite(*item.ChangeRatio) {
			changePercent = *item.ChangeRatio
		}
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = definition.Name
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "provider_index_name_missing_used_allowlisted_name")
		}
		var amount *float64
		amountUnit := "unknown"
		reasons := []string{}
		if quoteTime == nil {
			reasons = append(reasons, "provider_snapshot_timestamp_missing_or_invalid")
		}
		if item.Turnover != nil && hithinkNonNegativeFinite(*item.Turnover) {
			value := *item.Turnover
			amount = &value
			amountUnit = "CNY"
		} else {
			reasons = append(reasons, "provider_amount_missing_or_invalid")
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "provider_amount_missing_or_invalid")
		}
		itemQuality := model.QualityValid
		if len(reasons) > 0 {
			itemQuality = model.QualityDegraded
			snapshot.QualityStatus = model.QualityDegraded
		}
		bySymbol[symbol] = model.MarketIndexQuote{
			Symbol: symbol, Code: definition.Code, Name: name,
			LatestPrice: item.LastPrice, Change: &change, ChangePercent: &changePercent,
			Open: item.OpenPrice, High: item.HighPrice, Low: item.LowPrice, PrevClose: item.PreviousPrice,
			Amount: amount, AmountUnit: amountUnit, QuoteTime: quoteTime, QuoteTimeSemantics: quoteTimeSemantics,
			Source: p.Name(), SourceVersion: hithinkVersion, FetchedAt: now,
			QualityStatus: itemQuality, QualityReasons: reasons,
		}
	}
	for _, definition := range hithinkCoreIndexDefinitions {
		item, ok := bySymbol[definition.Symbol]
		if !ok {
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "one_or_more_allowlisted_indices_missing")
			continue
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	if len(snapshot.Items) == 0 {
		return nil, errors.New("同花顺指数行情未返回可用的核心指数记录")
	}
	return snapshot, nil
}

func appendUniqueHithinkReason(reasons []string, reason string) []string {
	for _, item := range reasons {
		if item == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

// FetchTradingCalendar 调用同花顺滚动一年日历。上游只列开市日期，非交易日由适配器推断并降级。
func (p *Hithink) FetchTradingCalendar(ctx context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺交易日历接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil {
		return nil, fmt.Errorf("无效的同花顺交易日历起始日期 %q", startDate)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil {
		return nil, fmt.Errorf("无效的同花顺交易日历结束日期 %q", endDate)
	}
	if start.After(end) {
		return nil, errors.New("同花顺交易日历起始日期不能晚于结束日期")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	nowCN := time.Now().In(loc)
	todayDate := nowCN.Format("2006-01-02")
	firstSupportedDate := nowCN.AddDate(-1, 0, 0).Format("2006-01-02")
	if startDate < firstSupportedDate || endDate > todayDate {
		return nil, errors.New("同花顺交易日历仅覆盖 Asia/Shanghai 最近一年至今日；该区间应回退到支持完整日期范围的数据源")
	}
	if int(end.Sub(start)/(24*time.Hour))+1 > 3660 {
		return nil, errors.New("同花顺交易日历请求区间不能超过 3660 天")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	raw, err := p.get(ctx, "/api/a-share/calendar/trading-days", nil)
	if err != nil {
		return nil, fmt.Errorf("同花顺交易日历请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp *int64 `json:"timestamp"`
		Item []struct {
			DateMS *int64 `json:"date_ms"`
			Date string `json:"date"`
		} `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺交易日历失败: %w", err)
	}
	if len(data.Item) == 0 {
		return nil, errors.New("同花顺交易日历返回空的交易日列表")
	}
	tradingDates := make(map[string]struct{}, len(data.Item))
	for index, item := range data.Item {
		dateText := strings.TrimSpace(item.Date)
		var parsed time.Time
		if dateText != "" {
			parsed, err = time.Parse("20060102", dateText)
			if err != nil {
				parsed, err = time.Parse("2006-01-02", dateText)
			}
		} else if item.DateMS != nil && *item.DateMS > 0 {
			parsed = time.UnixMilli(*item.DateMS).In(loc)
		} else {
			return nil, fmt.Errorf("同花顺交易日历第 %d 项缺少有效日期", index)
		}
		if err != nil {
			return nil, fmt.Errorf("同花顺交易日历第 %d 项日期无效", index)
		}
		date := parsed.In(loc).Format("2006-01-02")
		if date < firstSupportedDate || date > todayDate {
			return nil, fmt.Errorf("同花顺交易日历返回了超出滚动一年范围的日期 %s", date)
		}
		if _, duplicate := tradingDates[date]; duplicate {
			return nil, fmt.Errorf("同花顺交易日历返回重复日期 %s", date)
		}
		tradingDates[date] = struct{}{}
	}
	startLocal := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc)
	endLocal := time.Date(end.Year(), end.Month(), end.Day(), 0, 0, 0, 0, loc)
	totalDays := int(endLocal.Sub(startLocal)/(24*time.Hour)) + 1
	items := make([]model.TradingCalendarDay, 0, totalDays)
	tradingDays := 0
	for current := startLocal; !current.After(endLocal); current = current.AddDate(0, 0, 1) {
		date := current.Format("2006-01-02")
		_, isTradingDay := tradingDates[date]
		if isTradingDay {
			tradingDays++
		}
		items = append(items, model.TradingCalendarDay{Date: date, IsTradingDay: isTradingDay})
	}
	snapshot := &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: startDate, EndDate: endDate,
		TotalDays: totalDays, TradingDays: tradingDays, Items: items,
		Source: p.Name(), SourceVersion: hithinkVersion, FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_returns_trading_days_only_non_trading_dates_inferred"},
	}
	if data.Timestamp == nil || *data.Timestamp <= 0 {
		snapshot.QualityReasons = append(snapshot.QualityReasons, "provider_snapshot_timestamp_missing")
	}
	return snapshot, nil
}

// FetchLimitPool maps the THS limit-up pool. The API does not expose separate down-limit or broken-board pools.
func (p *Hithink) FetchLimitPool(ctx context.Context, poolType, tradeDate string, limit int) (*model.LimitPoolSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺涨停池接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	poolType = strings.ToLower(strings.TrimSpace(poolType))
	if poolType != "up" {
		return nil, fmt.Errorf("同花顺当前只支持涨停池 up；%q 应回退到其他支持 down/broken 的数据源", poolType)
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		return nil, errors.New("同花顺涨停池单页条数不能超过 100")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	rawDate := strings.TrimSpace(tradeDate)
	if rawDate == "" {
		rawDate = time.Now().In(loc).Format("2006-01-02")
	}
	date, err := time.Parse("2006-01-02", rawDate)
	if err != nil || date.Format("2006-01-02") != rawDate {
		return nil, fmt.Errorf("无效的同花顺涨停池交易日 %q，应为 YYYY-MM-DD", tradeDate)
	}
	if rawDate > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("同花顺涨停池交易日不能晚于今日")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	dateMS := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, loc).UnixMilli()
	query := url.Values{
		"date_ms": []string{strconv.FormatInt(dateMS, 10)},
		"page": []string{"1"},
		"size": []string{strconv.Itoa(limit)},
		"sort_field": []string{"limit_up_time"},
		"sort_dir": []string{"asc"},
	}
	raw, err := p.get(ctx, "/api/a-share/special-data/limit-up-pool", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺涨停池请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp *int64 `json:"timestamp"`
		Pagination *struct {
			Total *int `json:"total"`
			Pages *int `json:"pages"`
			Size *int `json:"size"`
			Page *int `json:"page"`
		} `json:"pagination"`
		Item []map[string]json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺涨停池失败: %w", err)
	}
	if data.Pagination == nil || data.Pagination.Total == nil || *data.Pagination.Total < 0 {
		return nil, errors.New("同花顺涨停池响应缺少有效 pagination.total")
	}
	total := *data.Pagination.Total
	if len(data.Item) > limit || total < len(data.Item) {
		return nil, fmt.Errorf("同花顺涨停池分页计数不一致: total=%d items=%d limit=%d", total, len(data.Item), limit)
	}
	now := time.Now().UTC()
	reportDate := rawDate
	if data.Timestamp != nil && *data.Timestamp > 0 {
		sourceDate := time.UnixMilli(*data.Timestamp).In(loc).Format("2006-01-02")
		if sourceDate == rawDate {
			reportDate = sourceDate
		}
	}
	snapshot := &model.LimitPoolSnapshot{
		PoolType: "up", TradeDate: rawDate, ProviderReportDate: reportDate, Total: total,
		HasData: len(data.Item) > 0, Items: make([]model.LimitPoolItem, 0, len(data.Item)),
		Source: p.Name(), SourceVersion: hithinkVersion + "-limit-up-pool-v1",
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_amount_units_not_explicitly_documented", "limit_up_time_mapped_to_first_touch_time"},
	}
	if data.Timestamp == nil || *data.Timestamp <= 0 || time.UnixMilli(*data.Timestamp).After(now.Add(5*time.Minute)) {
		snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "provider_snapshot_timestamp_missing_or_invalid")
	}
	if total == 0 && len(data.Item) == 0 {
		snapshot.HasData = false
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "empty_limit_up_pool_cannot_distinguish_no_events_from_unavailable_or_non_trading_day")
		return snapshot, nil
	}
	if total > 0 && len(data.Item) == 0 {
		return nil, fmt.Errorf("同花顺涨停池 total=%d 但当前页没有返回数据", total)
	}
	for index, row := range data.Item {
		rawSymbol := hithinkFieldString(row, "thscode")
		symbol, symbolErr := hithinkDomesticSymbol(rawSymbol)
		if symbolErr != nil {
			return nil, fmt.Errorf("同花顺涨停池第 %d 行证券代码无效: %w", index, symbolErr)
		}
		code := hithinkFieldString(row, "ticker")
		if code == "" {
			code = strings.Split(symbol, ".")[0]
		}
		if code != strings.Split(symbol, ".")[0] {
			return nil, fmt.Errorf("同花顺涨停池第 %d 行代码不匹配: symbol=%s ticker=%s", index, symbol, code)
		}
		name := hithinkFieldString(row, "name")
		if name == "" {
			return nil, fmt.Errorf("同花顺涨停池第 %d 行缺少证券名称", index)
		}
		change := hithinkFieldNumber(row, "price_change_ratio_pct")
		if change == nil {
			return nil, fmt.Errorf("同花顺涨停池第 %d 行缺少有效涨跌幅", index)
		}
		var consecutive *int
		if value := hithinkFieldNumber(row, "continue_day_cnt"); value != nil &&
			*value >= 0 && math.Trunc(*value) == *value && *value <= 1000 {
			v := int(*value)
			consecutive = &v
		}
		firstTouch := hithinkFieldString(row, "limit_up_time")
		var sealMoney, maxSealMoney *float64
		if value := hithinkFieldNumber(row, "seal_money"); value != nil && *value >= 0 {
			sealMoney = value
		}
		if value := hithinkFieldNumber(row, "max_seal_money"); value != nil && *value >= 0 {
			maxSealMoney = value
		}
		itemReasons := []string{"provider_amount_unit_not_explicitly_documented"}
		if data.Timestamp == nil || *data.Timestamp <= 0 {
			itemReasons = append(itemReasons, "provider_snapshot_timestamp_missing_or_invalid")
		}
		if firstTouch == "" {
			itemReasons = append(itemReasons, "provider_limit_up_time_missing")
		}
		snapshot.Items = append(snapshot.Items, model.LimitPoolItem{
			Symbol: symbol, Code: code, Name: name, ChangePercent: change,
			FirstTouchTime: firstTouch, ConsecutiveBoards: consecutive,
			SealMoney: sealMoney, MaxSealMoney: maxSealMoney, ProviderAmountUnit: "provider_defined",
			Source: p.Name(), SourceVersion: snapshot.SourceVersion, FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: itemReasons,
		})
	}
	return snapshot, nil
}

// FetchDragonTiger maps the THS A-share stock rows into the shared paginated response.
// Provider-specific money values are preserved separately with unit=provider_defined; *_CNY fields remain unset.
func (p *Hithink) FetchDragonTiger(ctx context.Context, tradeDate string, page, pageSize int) (*model.DragonTigerSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺龙虎榜接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	if page < 1 || page > 1000 {
		return nil, errors.New("同花顺龙虎榜 page 必须在 1..1000")
	}
	if pageSize < 1 || pageSize > 500 {
		return nil, errors.New("同花顺龙虎榜 page_size 必须在 1..500")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	rawDate := strings.TrimSpace(tradeDate)
	if rawDate != "" {
		date, parseErr := time.Parse("2006-01-02", rawDate)
		if parseErr != nil || date.Format("2006-01-02") != rawDate {
			return nil, fmt.Errorf("无效的同花顺龙虎榜交易日 %q，应为 YYYY-MM-DD", tradeDate)
		}
		today := time.Now().In(loc).Format("2006-01-02")
		firstSupported := time.Now().In(loc).AddDate(-1, 0, 0).Format("2006-01-02")
		if rawDate < firstSupported || rawDate > today {
			return nil, errors.New("同花顺龙虎榜显式交易日需位于最近一年且不能晚于今日")
		}
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := url.Values{"board_type": []string{"all"}}
	if rawDate != "" {
		query.Set("date", rawDate)
	}
	raw, err := p.get(ctx, "/api/a-share/special-data/dragon-tiger-list", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺龙虎榜请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp *int64 `json:"timestamp"`
		BoardType string `json:"board_type"`
		TradeDate string `json:"trade_date"`
		Count *int `json:"count"`
		StockCount *int `json:"stock_count"`
		StockItems []map[string]json.RawMessage `json:"stock_items"`
	} 
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺龙虎榜失败: %w", err)
	}
	responseDate := strings.TrimSpace(data.TradeDate)
	if responseDate == "" {
		return nil, errors.New("同花顺龙虎榜响应缺少 trade_date")
	}
	parsedResponseDate, err := time.Parse("2006-01-02", responseDate)
	if err != nil || parsedResponseDate.Format("2006-01-02") != responseDate {
		return nil, fmt.Errorf("同花顺龙虎榜返回了无效交易日 %q", responseDate)
	}
	if rawDate != "" && rawDate != responseDate {
		return nil, fmt.Errorf("同花顺龙虎榜交易日不匹配: requested=%s returned=%s", rawDate, responseDate)
	}
	total := len(data.StockItems)
	if data.StockCount != nil {
		if *data.StockCount < 0 || *data.StockCount != len(data.StockItems) {
			return nil, fmt.Errorf("同花顺龙虎榜 stock_count 与 stock_items 不一致: declared=%d items=%d", *data.StockCount, len(data.StockItems))
		}
		total = *data.StockCount
	}
	now := time.Now().UTC()
	sourceVersion := hithinkVersion + "-dragon-tiger-v1"
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	snapshot := &model.DragonTigerSnapshot{
		TradeDate: responseDate, ProviderReportDate: responseDate,
		Page: page, PageSize: pageSize, Total: total, Pages: pages,
		SourceReturned: len(data.StockItems), Items: []model.DragonTigerItem{},
		Source: p.Name(), SourceVersion: sourceVersion, FetchedAt: now,
		QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"provider_amount_units_not_explicitly_documented", "hot_money_detail_not_represented_in_shared_stock_pagination"},
	}
	if data.Timestamp == nil || *data.Timestamp <= 0 || time.UnixMilli(*data.Timestamp).After(now.Add(5*time.Minute)) {
		snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "provider_snapshot_timestamp_missing_or_invalid")
	}
	if total == 0 {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "empty_dragon_tiger_report_cannot_distinguish_no_events_from_unavailable_or_not_yet_published")
		return snapshot, nil
	}
	if pages == 0 || page > pages {
		return nil, fmt.Errorf("同花顺龙虎榜请求页 %d 超出总页数 %d", page, pages)
	}
	startIndex := (page - 1) * pageSize
	endIndex := min(startIndex+pageSize, total)
	if startIndex >= len(data.StockItems) {
		return nil, fmt.Errorf("同花顺龙虎榜请求页 %d 超出了当前返回数据", page)
	}
	snapshot.HasMore = page < pages
	snapshot.HasData = true
	for index, row := range data.StockItems[startIndex:endIndex] {
		rawSymbol := hithinkFieldString(row, "thscode")
		symbol, symbolErr := hithinkDomesticSymbol(rawSymbol)
		if symbolErr != nil {
			return nil, fmt.Errorf("同花顺龙虎榜第 %d 行证券代码无效: %w", startIndex+index, symbolErr)
		}
		code := hithinkFieldString(row, "ticker")
		if code == "" {
			code = strings.Split(symbol, ".")[0]
		}
		if code != strings.Split(symbol, ".")[0] {
			return nil, fmt.Errorf("同花顺龙虎榜第 %d 行代码不匹配", startIndex+index)
		}
		name := hithinkFieldString(row, "name")
		if name == "" {
			return nil, fmt.Errorf("同花顺龙虎榜第 %d 行缺少证券名称", startIndex+index)
		}
		change := hithinkFieldNumber(row, "change")
		buy := hithinkFieldNumber(row, "buy_value")
		sell := hithinkFieldNumber(row, "sell_value")
		net := hithinkFieldNumber(row, "net_value")
		netRate := hithinkFieldNumber(row, "net_rate")
		orgNet := hithinkFieldNumber(row, "org_net_value")
		hotNet := hithinkFieldNumber(row, "hot_money_net_value")
		hotRank := hithinkFieldInteger(row, "hot_rank")
		rangeDays := hithinkFieldInteger(row, "range_days")
		if buy != nil && *buy < 0 || sell != nil && *sell < 0 {
			return nil, fmt.Errorf("同花顺龙虎榜第 %d 行买入/卖出金额不能为负", startIndex+index)
		}
		var concepts []string
		if rawConcepts, ok := row["concept_list"]; ok && len(rawConcepts) > 0 {
			_ = json.Unmarshal(rawConcepts, &concepts)
		}
		itemReasons := []string{"provider_amount_unit_not_explicitly_documented"}
		if data.Timestamp == nil || *data.Timestamp <= 0 {
			itemReasons = append(itemReasons, "provider_snapshot_timestamp_missing_or_invalid")
		}
		item := model.DragonTigerItem{
			Symbol: symbol, Code: code, Name: name, TradeDate: responseDate,
			ChangePercent: change, Reason: hithinkFieldString(row, "limit_reason"),
			ProviderBuyValue: buy, ProviderSellValue: sell, ProviderNetValue: net, ProviderNetRate: netRate,
			ProviderOrgNetValue: orgNet, ProviderHotMoneyNetValue: hotNet, ProviderAmountUnit: "provider_defined",
			HotRank: hotRank, RangeDays: rangeDays, Concepts: concepts,
			Source: p.Name(), SourceVersion: sourceVersion, FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: itemReasons,
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	snapshot.Returned = len(snapshot.Items)
	return snapshot, nil
}

func hithinkFieldString(row map[string]json.RawMessage, key string) string {
	var value string
	if json.Unmarshal(row[key], &value) == nil {
		return strings.TrimSpace(value)
	}
	return ""
}

func hithinkFieldNumber(row map[string]json.RawMessage, key string) *float64 {
	value, ok := hithinkParseFundamentalValue(row[key])
	if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}
	return &value
}

func hithinkFieldInteger(row map[string]json.RawMessage, key string) *int {
	value := hithinkFieldNumber(row, key)
	if value == nil || math.Trunc(*value) != *value || *value < 0 || *value > 1000000 {
		return nil
	}
	result := int(*value)
	return &result
}

func (p *Hithink) FetchValuations(ctx context.Context, rawSymbols []string) (*model.ValuationSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺估值接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	if len(rawSymbols) == 0 || len(rawSymbols) > 100 {
		return nil, errors.New("同花顺估值接口每次需要 1..100 个 A 股证券代码")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	symbols := make([]string, 0, len(rawSymbols))
	seen := make(map[string]struct{}, len(rawSymbols))
	for _, rawSymbol := range rawSymbols {
		symbol, err := hithinkDomesticSymbol(rawSymbol)
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[symbol]; duplicate {
			continue
		}
		seen[symbol] = struct{}{}
		symbols = append(symbols, symbol)
	}
	if len(symbols) == 0 {
		return nil, errors.New("同花顺估值接口没有有效证券代码")
	}
	query := url.Values{}
	query.Set("thscodes", strings.Join(symbols, ","))
	raw, err := p.get(ctx, "/api/a-share/valuations/snapshot", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺批量估值请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp *int64                      `json:"timestamp"`
		Total     *int                         `json:"total"`
		Item      []map[string]json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺批量估值失败: %w", err)
	}
	if data.Total != nil && (*data.Total < 0 || *data.Total != len(data.Item)) {
		return nil, fmt.Errorf("同花顺估值响应 total 与 item 数量不一致: total=%d items=%d", *data.Total, len(data.Item))
	}
	now := time.Now().UTC()
	snapshot := &model.ValuationSnapshot{
		RequestedSymbols: append([]string(nil), symbols...),
		Items: make([]model.ValuationItem, 0, len(data.Item)),
		Source: p.Name(), SourceVersion: hithinkVersion + "-valuation-snapshot-v1",
		FetchedAt: now, QualityStatus: model.QualityValid,
	}
	if data.Timestamp != nil && *data.Timestamp > 0 {
		timestamp := time.UnixMilli(*data.Timestamp).UTC()
		if !timestamp.After(now.Add(5*time.Minute)) && !timestamp.Before(now.AddDate(-10, 0, 0)) {
			snapshot.ProviderTimestamp = &timestamp
		}
	}
	if snapshot.ProviderTimestamp == nil {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = append(snapshot.QualityReasons, "provider_valuation_timestamp_missing_or_invalid")
	}
	requested := make(map[string]struct{}, len(symbols))
	for _, symbol := range symbols {
		requested[symbol] = struct{}{}
	}
	bySymbol := make(map[string]model.ValuationItem, len(data.Item))
	for rowIndex, row := range data.Item {
		rawSymbol := hithinkFieldString(row, "thscode")
		symbol, symbolErr := hithinkDomesticSymbol(rawSymbol)
		if symbolErr != nil {
			return nil, fmt.Errorf("同花顺估值第 %d 行证券代码无效: %w", rowIndex, symbolErr)
		}
		if _, ok := requested[symbol]; !ok {
			return nil, fmt.Errorf("同花顺估值响应包含未请求代码 %s", symbol)
		}
		if _, duplicate := bySymbol[symbol]; duplicate {
			return nil, fmt.Errorf("同花顺估值响应包含重复代码 %s", symbol)
		}
		code := hithinkFieldString(row, "ticker")
		if code == "" {
			code = strings.Split(symbol, ".")[0]
		}
		if code != strings.Split(symbol, ".")[0] {
			return nil, fmt.Errorf("同花顺估值第 %d 行 ticker 与 thscode 不一致", rowIndex)
		}
		name := hithinkFieldString(row, "name")
		itemReasons := make([]string, 0, 2)
		if name == "" {
			itemReasons = append(itemReasons, "provider_stock_name_missing")
		}
		pet := hithinkFieldNumber(row, "pe_ttm")
		pem := hithinkFieldNumber(row, "pe_mrq")
		pbm := hithinkFieldNumber(row, "pb_mrq")
		pst := hithinkFieldNumber(row, "ps_ttm")
		pcft := hithinkFieldNumber(row, "pcf_ttm")
		if pet == nil || pem == nil || pbm == nil || pst == nil || pcft == nil {
			itemReasons = append(itemReasons, "one_or_more_valuation_metrics_missing_or_invalid")
		}
		itemQuality := model.QualityValid
		if len(itemReasons) > 0 {
			itemQuality = model.QualityDegraded
			snapshot.QualityStatus = model.QualityDegraded
		}
		bySymbol[symbol] = model.ValuationItem{
			Symbol: symbol, Code: code, Name: name,
			PETTM: pet, PEMRQ: pem, PBMRQ: pbm, PSTTM: pst, PCFTTM: pcft,
			Source: p.Name(), SourceVersion: snapshot.SourceVersion, FetchedAt: now,
			QualityStatus: itemQuality, QualityReasons: itemReasons,
		}
	}
	for _, symbol := range symbols {
		item, ok := bySymbol[symbol]
		if !ok {
			snapshot.MissingSymbols = append(snapshot.MissingSymbols, symbol)
			snapshot.QualityStatus = model.QualityDegraded
			snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "one_or_more_requested_symbols_missing_from_provider_response")
			continue
		}
		snapshot.Items = append(snapshot.Items, item)
	}
	snapshot.Total = len(snapshot.Items)
	if snapshot.Total == 0 {
		snapshot.QualityStatus = model.QualityUnknown
		snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "provider_returned_no_valuation_rows")
	}
	if data.Total == nil {
		snapshot.QualityStatus = model.QualityDegraded
		snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, "provider_total_missing")
	}
	return snapshot, nil
}

func (p *Hithink) FetchQuote(ctx context.Context, rawSymbol string) (*model.Quote, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺行情接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	symbol, err := hithinkDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := url.Values{}
	query.Set("thscodes", symbol)
	raw, err := p.get(ctx, "/api/a-share/prices/snapshot", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺行情请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		Timestamp *int64 `json:"timestamp"`
		Item      []struct {
			THSCode       string   `json:"thscode"`
			LastPrice     *float64 `json:"last_price"`
			PriceChange   *float64 `json:"price_change"`
			ChangeRatio   *float64 `json:"price_change_ratio_pct"`
			OpenPrice     *float64 `json:"open_price"`
			HighPrice     *float64 `json:"high_price"`
			LowPrice      *float64 `json:"low_price"`
			PreviousPrice *float64 `json:"prev_price"`
			Volume        *float64 `json:"volume"`
			Turnover      *float64 `json:"turnover"`
		} `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺行情数据失败: %w", err)
	}
	var item *struct {
		THSCode       string   `json:"thscode"`
		LastPrice     *float64 `json:"last_price"`
		PriceChange   *float64 `json:"price_change"`
		ChangeRatio   *float64 `json:"price_change_ratio_pct"`
		OpenPrice     *float64 `json:"open_price"`
		HighPrice     *float64 `json:"high_price"`
		LowPrice      *float64 `json:"low_price"`
		PreviousPrice *float64 `json:"prev_price"`
		Volume        *float64 `json:"volume"`
		Turnover      *float64 `json:"turnover"`
	}
	for i := range data.Item {
		if strings.EqualFold(strings.TrimSpace(data.Item[i].THSCode), symbol) {
			item = &data.Item[i]
			break
		}
	}
	if item == nil {
		return nil, fmt.Errorf("同花顺行情响应未包含请求证券 %s", symbol)
	}
	if item.LastPrice == nil || !hithinkPositiveFinite(*item.LastPrice) ||
		item.PreviousPrice == nil || !hithinkPositiveFinite(*item.PreviousPrice) ||
		item.OpenPrice == nil || item.HighPrice == nil || item.LowPrice == nil ||
		!hithinkPositiveFinite(*item.OpenPrice) || !hithinkPositiveFinite(*item.HighPrice) || !hithinkPositiveFinite(*item.LowPrice) ||
		*item.LowPrice > *item.HighPrice || *item.OpenPrice < *item.LowPrice || *item.OpenPrice > *item.HighPrice ||
		*item.LastPrice < *item.LowPrice || *item.LastPrice > *item.HighPrice {
		return nil, errors.New("同花顺行情 OHLC/价格字段不满足数据契约")
	}

	change := *item.LastPrice - *item.PreviousPrice
	if item.PriceChange != nil && hithinkFinite(*item.PriceChange) {
		change = *item.PriceChange
	}
	changePercent := change / *item.PreviousPrice * 100
	if item.ChangeRatio != nil && hithinkFinite(*item.ChangeRatio) {
		changePercent = *item.ChangeRatio
	}
	now := time.Now().UTC()
	var volume *int64
	var rawVolume *float64
	var amount, rawAmount *float64
	quality := model.QualityDegraded
	reasons := []string{"provider_quote_timestamp_not_available_for_symbol_query"}
	if item.Volume != nil && hithinkNonNegativeFinite(*item.Volume) && math.Trunc(*item.Volume) == *item.Volume && *item.Volume <= float64(math.MaxInt64) {
		v := int64(*item.Volume)
		volume = &v
		vRaw := *item.Volume
		rawVolume = &vRaw
	} else {
		reasons = append(reasons, "volume_missing_or_invalid")
	}
	if item.Turnover != nil && hithinkNonNegativeFinite(*item.Turnover) {
		v := *item.Turnover
		amount, rawAmount = &v, &v
	} else {
		reasons = append(reasons, "amount_missing_or_invalid")
	}
	qualityReason := []string(nil)
	if len(reasons) > 0 {
		qualityReason = append(qualityReason, reasons...)
	}
	volumeUnit, amountUnit := "shares", "CNY"
	rawAmountUnit, rawAmountField := "CNY", "turnover"
	if amount == nil {
		amountUnit, rawAmountUnit, rawAmountField = "unknown", "", ""
	}
	return &model.Quote{
		Symbol: symbol,
		CurrentPrice: item.LastPrice,
		Change: &change,
		ChangePercent: &changePercent,
		Open: item.OpenPrice,
		High: item.HighPrice,
		Low: item.LowPrice,
		PrevClose: item.PreviousPrice,
		Volume: volume,
		Amount: amount,
		FetchedAt: now,
		NormalizedAt: now,
		Source: p.Name(),
		SourceVersion: hithinkVersion,
		RawVolume: rawVolume,
		RawVolumeUnit: volumeUnit,
		RawAmount: rawAmount,
		RawAmountUnit: rawAmountUnit,
		RawAmountField: rawAmountField,
		VolumeUnit: volumeUnit,
		AmountUnit: amountUnit,
		AdjustmentBasis: model.AdjustmentNone,
		QualityStatus: quality,
		QualityReasons: qualityReason,
		NormalizationVer: normalize.Version,
	}, nil
}

func (p *Hithink) FetchDailyBars(ctx context.Context, rawSymbol, startDate, endDate string, limit int, adjustment model.AdjustmentBasis) ([]model.DailyBar, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺历史 K 线接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	symbol, err := hithinkDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	start, err := time.Parse("2006-01-02", strings.TrimSpace(startDate))
	if err != nil {
		return nil, fmt.Errorf("无效的同花顺日 K 起始日期 %q", startDate)
	}
	end, err := time.Parse("2006-01-02", strings.TrimSpace(endDate))
	if err != nil {
		return nil, fmt.Errorf("无效的同花顺日 K 结束日期 %q", endDate)
	}
	if start.After(end) {
		return nil, errors.New("同花顺日 K 起始日期不能晚于结束日期")
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if end.Format("2006-01-02") > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("同花顺日 K 结束日期不能晚于 Asia/Shanghai 今日")
	}
	if limit <= 0 {
		limit = 120
	}
	if limit > 2000 {
		return nil, errors.New("同花顺日 K 条数上限为 2000")
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
		return nil, fmt.Errorf("不支持的同花顺复权口径 %q", adjustment)
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	all := make([]model.DailyBar, 0, min(limit, 2000))
	segmentStart := start
	for !segmentStart.After(end) {
		segmentEnd := segmentStart.AddDate(9, 0, 0)
		if segmentEnd.After(end) {
			segmentEnd = end
		}
		segmentBars, err := p.fetchDailySegment(ctx, symbol, segmentStart, segmentEnd, adjust, loc)
		if err != nil {
			return nil, err
		}
		all = append(all, segmentBars...)
		if len(all) > hithinkMaxRowsPerCall {
			return nil, fmt.Errorf("同花顺日 K 结果超过安全上限 %d 行", hithinkMaxRowsPerCall)
		}
		segmentStart = segmentEnd.AddDate(0, 0, 1)
	}
	if len(all) == 0 {
		return nil, errors.New("同花顺未返回请求区间的日 K")
	}
	sort.Slice(all, func(i, j int) bool { return all[i].TradeDate < all[j].TradeDate })
	for i := 1; i < len(all); i++ {
		if all[i].TradeDate == all[i-1].TradeDate {
			return nil, fmt.Errorf("同花顺返回重复交易日 %s", all[i].TradeDate)
		}
	}
	for i := range all {
		if all[i].ChangePercent == nil && i > 0 && all[i-1].Close > 0 {
			v := (all[i].Close - all[i-1].Close) / all[i-1].Close * 100
			all[i].ChangePercent = &v
		}
		if all[i].ChangePercent == nil {
			all[i].QualityStatus = model.QualityDegraded
			all[i].QualityReasons = append(all[i].QualityReasons, "change_percent_unavailable_for_first_bar")
		}
	}
	return normalizeProviderDailyBars(symbol, start.Format("2006-01-02"), end.Format("2006-01-02"), limit, all, 0)
}

// FetchFundamentals 使用同花顺财务指标端点读取单个报告期的能力指标。
// 官方响应不包含可确认的披露时间，也未为每个指标提供单位说明；因此缺失项不补零，完整性/时间可用性不满足要求时保持 degraded。
func (p *Hithink) FetchFundamentals(ctx context.Context, rawSymbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("同花顺 HTTP client 未配置")
	}
	if p.apiKey == "" {
		return nil, errors.New("同花顺财务指标接口需要配置 MARKET_DATA_HITHINK_API_KEY")
	}
	symbol, err := hithinkDomesticSymbol(rawSymbol)
	if err != nil {
		return nil, err
	}
	nowYear := time.Now().Year()
	if year < 2000 || year > nowYear {
		return nil, fmt.Errorf("同花顺财务指标年份必须在 2000..%d", nowYear)
	}
	if quarter < 1 || quarter > 4 {
		return nil, errors.New("同花顺财务指标季度必须为 1..4")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	report := fmt.Sprintf("%d-%d", year, quarter)
	query := url.Values{}
	query.Set("thscode", symbol)
	query.Set("report", report)
	raw, err := p.get(ctx, "/api/a-share/financials/indicators", query)
	if err != nil {
		return nil, fmt.Errorf("同花顺财务指标请求失败: %w", err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return nil, err
	}
	var data struct {
		THSCode string `json:"thscode"`
		Report  string `json:"report"`
		Abilities []struct {
			Ability string `json:"ability"`
			Indicators []struct {
				IndexID string `json:"index_id"`
				Value json.RawMessage `json:"value"`
			} `json:"indicators"`
		} `json:"abilities"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return nil, fmt.Errorf("解析同花顺财务指标失败: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(data.THSCode), symbol) {
		return nil, fmt.Errorf("同花顺财务响应证券代码不匹配: requested=%s returned=%q", symbol, data.THSCode)
	}
	if strings.TrimSpace(data.Report) != report {
		return nil, fmt.Errorf("同花顺财务响应报告期不匹配: requested=%s returned=%q", report, data.Report)
	}

	reportDate := time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC).Format("2006-01-02")
	snapshot := &model.FundamentalsSnapshot{
		Symbol: symbol, Year: year, Quarter: quarter,
		Sections: make([]model.FundamentalSection, 0, len(data.Abilities)),
		Source: p.Name(), SourceVersion: hithinkVersion,
		FetchedAt: time.Now().UTC(), QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"publication_date_not_exposed_by_provider", "indicator_units_not_exposed_by_provider"},
	}
	for _, ability := range data.Abilities {
		category := hithinkFundamentalCategory(ability.Ability)
		section := model.FundamentalSection{
			Category: category, ReportDate: reportDate,
			Metrics: make([]model.FundamentalMetric, 0, len(ability.Indicators)),
			QualityStatus: model.QualityDegraded,
			QualityReasons: []string{"publication_date_not_exposed_by_provider", "metric_unit_not_exposed_by_provider"},
		}
		invalidValueCount := 0
		for _, item := range ability.Indicators {
			indexID := strings.TrimSpace(item.IndexID)
			if indexID == "" {
				invalidValueCount++
				continue
			}
			value, ok := hithinkParseFundamentalValue(item.Value)
			if !ok {
				invalidValueCount++
				continue
			}
			section.Metrics = append(section.Metrics, model.FundamentalMetric{
				Key: indexID, Name: indexID, Value: value,
				Unit: "provider_defined", SourceField: indexID,
			})
		}
		section.HasData = len(section.Metrics) > 0
		if invalidValueCount > 0 {
			section.QualityReasons = append(section.QualityReasons, "one_or_more_metrics_missing_or_invalid")
		}
		if !section.HasData {
			section.QualityReasons = append(section.QualityReasons, "no_numeric_metrics_available")
		} else {
			snapshot.HasData = true
		}
		snapshot.Sections = append(snapshot.Sections, section)
	}
	// 报表接口按年度/季度返回多期数据，按报告期末精确匹配，不把相邻期间误当作目标季度。
	statementRequests := []struct {
		path     string
		category string
	}{
		{path: "/api/a-share/financials/income-statements", category: "income_statement"},
		{path: "/api/a-share/financials/balance-sheets", category: "balance_sheet"},
		{path: "/api/a-share/financials/cash-flow-statements", category: "cash_flow_statement"},
	}
	for _, request := range statementRequests {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		section, statementErr := p.fetchFinancialStatementSection(ctx, symbol, year, quarter, reportDate, request.path, request.category)
		if statementErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			snapshot.QualityReasons = appendUniqueHithinkReason(snapshot.QualityReasons, request.category+"_provider_unavailable")
			section = model.FundamentalSection{
				Category: request.category, ReportDate: reportDate,
				Metrics: []model.FundamentalMetric{}, QualityStatus: model.QualityDegraded,
				QualityReasons: []string{"provider_statement_unavailable_or_target_period_missing"},
			}
		}
		if section.HasData {
			snapshot.HasData = true
		}
		snapshot.Sections = append(snapshot.Sections, section)
	}
	if !snapshot.HasData {
		snapshot.QualityReasons = append(snapshot.QualityReasons, "no_numeric_metrics_for_requested_report")
	}
	return snapshot, nil
}

func hithinkParseFundamentalValue(raw json.RawMessage) (float64, bool) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return 0, false
	}
	var textValue string
	if json.Unmarshal(raw, &textValue) == nil {
		textValue = strings.TrimSpace(textValue)
		if textValue == "" {
			return 0, false
		}
		value, err := strconv.ParseFloat(textValue, 64)
		return value, err == nil && hithinkFinite(value)
	}
	value, err := strconv.ParseFloat(trimmed, 64)
	return value, err == nil && hithinkFinite(value)
}

func hithinkFundamentalCategory(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "growth":
		return "growth"
	case "profitability":
		return "profitability"
	case "solvency":
		return "solvency"
	case "operation":
		return "operating"
	case "cash-flow":
		return "cash_flow"
	default:
		value := strings.ToLower(strings.TrimSpace(raw))
		value = strings.ReplaceAll(value, "-", "_")
		if value == "" {
			return "other"
		}
		return value
	}
}

type hithinkStatementField struct {
	Key  string
	Name string
	Unit string
}

var hithinkStatementFields = map[string][]hithinkStatementField{
	"income_statement": {
		{Key: "basic_eps", Name: "基本每股收益", Unit: "CNY/share"},
		{Key: "operating_income", Name: "营业收入", Unit: "CNY"},
		{Key: "operating_costs", Name: "营业成本", Unit: "CNY"},
		{Key: "operating_expenses", Name: "营业支出", Unit: "CNY"},
		{Key: "operating_profit", Name: "营业利润", Unit: "CNY"},
		{Key: "profit_total", Name: "利润总额", Unit: "CNY"},
		{Key: "net_profit", Name: "净利润", Unit: "CNY"},
		{Key: "parent_holder_net_profit", Name: "归属于母公司所有者的净利润", Unit: "CNY"},
		{Key: "income_tax_expense", Name: "所得税费用", Unit: "CNY"},
		{Key: "interest_expenses", Name: "利息支出", Unit: "CNY"},
		{Key: "manage_fee", Name: "管理费用", Unit: "CNY"},
		{Key: "sales_fee", Name: "销售费用", Unit: "CNY"},
		{Key: "research_and_development_expenses", Name: "研发费用", Unit: "CNY"},
	},
	"balance_sheet": {
		{Key: "total_current_assets", Name: "流动资产合计", Unit: "CNY"},
		{Key: "non_current_nets_total", Name: "非流动资产净值合计", Unit: "CNY"},
		{Key: "assets_total", Name: "资产总计", Unit: "CNY"},
		{Key: "total_debt", Name: "负债合计", Unit: "CNY"},
		{Key: "holder_equity_total", Name: "所有者权益合计", Unit: "CNY"},
		{Key: "cash", Name: "货币资金", Unit: "CNY"},
		{Key: "accounts_receivable", Name: "应收账款", Unit: "CNY"},
	},
	"cash_flow_statement": {
		{Key: "act_cash_flow_net", Name: "经营活动产生的现金流量净额", Unit: "CNY"},
		{Key: "invest_cash_flow_net", Name: "投资活动产生的现金流量净额", Unit: "CNY"},
		{Key: "financing_cash_flow_net", Name: "筹资活动产生的现金流量净额", Unit: "CNY"},
		{Key: "cash_equivalents_net_addition", Name: "现金及现金等价物净增加额", Unit: "CNY"},
		{Key: "pay_dividends_profits_interest_cash", Name: "分配股利、利润或偿付利息支付的现金", Unit: "CNY"},
		{Key: "pay_fixed_assets_etc_cash", Name: "购建固定资产等支付的现金", Unit: "CNY"},
	},
}

func (p *Hithink) fetchFinancialStatementSection(ctx context.Context, symbol string, year, quarter int, targetReportDate, endpoint, category string) (model.FundamentalSection, error) {
	period := "quarterly"
	if quarter == 4 {
		period = "annual"
	}
	query := url.Values{}
	query.Set("thscode", symbol)
	query.Set("period", period)
	query.Set("limit", "20")
	raw, err := p.get(ctx, endpoint, query)
	if err != nil {
		return model.FundamentalSection{}, fmt.Errorf("同花顺 %s 请求失败: %w", category, err)
	}
	dataRaw, err := hithinkResponseData(raw)
	if err != nil {
		return model.FundamentalSection{}, err
	}
	var data struct {
		Item []map[string]json.RawMessage `json:"item"`
	}
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return model.FundamentalSection{}, fmt.Errorf("解析同花顺 %s 失败: %w", category, err)
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	var selected map[string]json.RawMessage
	for _, row := range data.Item {
		var rowSymbol string
		if err := json.Unmarshal(row["thscode"], &rowSymbol); err != nil || !strings.EqualFold(strings.TrimSpace(rowSymbol), symbol) {
			continue
		}
		endValue, ok := hithinkParseFundamentalValue(row["period_end_ms"])
		if !ok || endValue <= 0 || endValue > float64(math.MaxInt64) {
			continue
		}
		rowDate := time.UnixMilli(int64(endValue)).In(loc).Format("2006-01-02")
		if rowDate == targetReportDate {
			selected = row
			break
		}
	}
	if selected == nil {
		return model.FundamentalSection{}, fmt.Errorf("同花顺 %s 未返回报告期末为 %s 的记录", category, targetReportDate)
	}
	section := model.FundamentalSection{
		Category: category, ReportDate: targetReportDate,
		Metrics: make([]model.FundamentalMetric, 0, len(hithinkStatementFields[category])),
		QualityStatus: model.QualityValid,
	}
	reportDateMS, ok := hithinkParseFundamentalValue(selected["report_date_ms"])
	if ok && reportDateMS > 0 && reportDateMS <= float64(math.MaxInt64) {
		section.PublicationDate = time.UnixMilli(int64(reportDateMS)).In(loc).Format("2006-01-02")
	} else {
		section.QualityStatus = model.QualityDegraded
		section.QualityReasons = append(section.QualityReasons, "publication_date_missing_or_invalid")
	}
	missingMetrics := 0
	for _, field := range hithinkStatementFields[category] {
		value, exists := hithinkParseFundamentalValue(selected[field.Key])
		if !exists {
			missingMetrics++
			continue
		}
		section.Metrics = append(section.Metrics, model.FundamentalMetric{
			Key: field.Key, Name: field.Name, Value: value, Unit: field.Unit, SourceField: field.Key,
		})
	}
	section.HasData = len(section.Metrics) > 0
	if missingMetrics > 0 {
		section.QualityStatus = model.QualityDegraded
		section.QualityReasons = append(section.QualityReasons, "one_or_more_statement_fields_missing_or_invalid")
	}
	if !section.HasData {
		section.QualityStatus = model.QualityDegraded
		section.QualityReasons = append(section.QualityReasons, "no_numeric_statement_fields_available")
	}
	return section, nil
}

func (p *Hithink) fetchDailySegment(ctx context.Context, symbol string, start, end time.Time, adjust string, loc *time.Location) ([]model.DailyBar, error) {
	startMS := time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, loc).UnixMilli()
	endMS := time.Date(end.Year(), end.Month(), end.Day(), 23, 59, 59, int(time.Second-time.Nanosecond), loc).UnixMilli()
	offset := 0
	out := make([]model.DailyBar, 0, 1000)
	seen := make(map[string]struct{})
	for page := 0; page < hithinkMaxPagesPerRange; page++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		query := url.Values{}
		query.Set("thscode", symbol)
		query.Set("interval", "1d")
		query.Set("start", strconv.FormatInt(startMS, 10))
		query.Set("end", strconv.FormatInt(endMS, 10))
		query.Set("adjust", adjust)
		query.Set("offset", strconv.Itoa(offset))
		raw, err := p.get(ctx, "/api/a-share/prices/historical", query)
		if err != nil {
			return nil, fmt.Errorf("同花顺历史 K 线区间 %s..%s 请求失败: %w", start.Format("2006-01-02"), end.Format("2006-01-02"), err)
		}
		dataRaw, err := hithinkResponseData(raw)
		if err != nil {
			return nil, err
		}
		var data struct {
			Timestamp *int64 `json:"timestamp"`
			Item []struct {
				DateMS      *int64   `json:"date_ms"`
				OpenPrice   *float64 `json:"open_price"`
				HighPrice   *float64 `json:"high_price"`
				LowPrice    *float64 `json:"low_price"`
				ClosePrice  *float64 `json:"close_price"`
				Volume      *float64 `json:"volume"`
				Turnover    *float64 `json:"turnover"`
			} `json:"item"`
		}
		if err := json.Unmarshal(dataRaw, &data); err != nil {
			return nil, fmt.Errorf("解析同花顺历史 K 线失败: %w", err)
		}
		if len(data.Item) == 0 {
			return out, nil
		}
		fetchedAt := time.Now().UTC()
		for rowIndex, item := range data.Item {
			if item.DateMS == nil || item.OpenPrice == nil || item.HighPrice == nil || item.LowPrice == nil || item.ClosePrice == nil {
				return nil, fmt.Errorf("同花顺日 K 第 %d 行缺少日期或 OHLC", rowIndex)
			}
			tradeDate := time.UnixMilli(*item.DateMS).In(loc).Format("2006-01-02")
			if tradeDate < start.Format("2006-01-02") || tradeDate > end.Format("2006-01-02") {
				return nil, fmt.Errorf("同花顺日 K 日期 %s 超出请求区间", tradeDate)
			}
			if _, duplicate := seen[tradeDate]; duplicate {
				return nil, fmt.Errorf("同花顺分页返回重复交易日 %s", tradeDate)
			}
			seen[tradeDate] = struct{}{}
			open, high, low, closePrice := *item.OpenPrice, *item.HighPrice, *item.LowPrice, *item.ClosePrice
			if !hithinkPositiveFinite(open) || !hithinkPositiveFinite(high) || !hithinkPositiveFinite(low) || !hithinkPositiveFinite(closePrice) ||
				low > high || open < low || open > high || closePrice < low || closePrice > high {
				return nil, fmt.Errorf("同花顺日 K %s 的 OHLC 不满足数据约束", tradeDate)
			}
			var volume *int64
			var rawVolume *float64
			quality := model.QualityValid
			var reasons []string
			if item.Volume == nil || !hithinkNonNegativeFinite(*item.Volume) || math.Trunc(*item.Volume) != *item.Volume || *item.Volume > float64(math.MaxInt64) {
				quality = model.QualityDegraded
				reasons = append(reasons, "volume_missing_or_invalid")
			} else {
				v := int64(*item.Volume)
				volume = &v
				raw := *item.Volume
				rawVolume = &raw
			}
			var amount *float64
			var rawAmount *float64
			amountUnit, rawAmountUnit, rawAmountField := "unknown", "", ""
			if item.Turnover == nil || !hithinkNonNegativeFinite(*item.Turnover) {
				quality = model.QualityDegraded
				reasons = append(reasons, "amount_missing_or_invalid")
			} else {
				v := *item.Turnover
				amount, rawAmount = &v, &v
				amountUnit, rawAmountUnit, rawAmountField = "CNY", "CNY", "turnover"
			}
			out = append(out, model.DailyBar{
				Symbol: symbol, TradeDate: tradeDate, Period: "1d",
				Open: open, High: high, Low: low, Close: closePrice,
				Volume: volume, Amount: amount,
				AdjustmentBasis: model.AdjustmentBasis(hithinkAdjustmentBasis(adjust)),
				AdjustmentMethod: "hithink_api_" + adjust,
				Source: p.Name(), SourceVersion: hithinkVersion,
				FetchedAt: fetchedAt, NormalizedAt: time.Now().UTC(),
				RawVolume: rawVolume, RawVolumeUnit: "shares",
				RawAmount: rawAmount, RawAmountUnit: rawAmountUnit, RawAmountField: rawAmountField,
				VolumeUnit: "shares", AmountUnit: amountUnit,
				QualityStatus: quality, QualityReasons: reasons, NormalizationVer: normalize.Version,
			})
			if len(out) > hithinkMaxRowsPerCall {
				return nil, fmt.Errorf("同花顺分页结果超过安全上限 %d 行", hithinkMaxRowsPerCall)
			}
		}
		offset += len(data.Item)
	}
	return nil, fmt.Errorf("同花顺在 %d 页内未结束分页，拒绝返回可能不完整的 K 线", hithinkMaxPagesPerRange)
}

func (p *Hithink) get(ctx context.Context, path string, query url.Values) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := p.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	endpoint := p.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	req.Header.Set("X-api-key", p.apiKey)
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = readBoundedBody(resp.Body, 4096)
		return nil, fmt.Errorf("HTTP 状态码 %d", resp.StatusCode)
	}
	return readBoundedBody(resp.Body, maxProviderPayloadBytes)
}

func (p *Hithink) waitForRateLimit(ctx context.Context) error {
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

func hithinkResponseData(raw []byte) (json.RawMessage, error) {
	var envelope struct {
		Code      *int            `json:"code"`
		Message   string          `json:"message"`
		RequestID string          `json:"request_id"`
		Data      json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("解析同花顺响应信封失败: %w", err)
	}
	if envelope.Code == nil {
		return nil, errors.New("同花顺响应缺少 code")
	}
	if *envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Message)
		if message == "" {
			message = "unspecified provider error"
		}
		if envelope.RequestID != "" {
			return nil, fmt.Errorf("同花顺业务错误 code=%d request_id=%s: %s", *envelope.Code, envelope.RequestID, message)
		}
		return nil, fmt.Errorf("同花顺业务错误 code=%d: %s", *envelope.Code, message)
	}
	if len(strings.TrimSpace(string(envelope.Data))) == 0 || strings.TrimSpace(string(envelope.Data)) == "null" {
		return nil, errors.New("同花顺成功响应缺少 data")
	}
	return envelope.Data, nil
}

func hithinkDomesticSymbol(raw string) (string, error) {
	symbol, err := normalize.NormalizeSymbol(raw)
	if err != nil {
		return "", err
	}
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") {
		return "", fmt.Errorf("同花顺 A 股适配器只支持 SH/SZ/BJ 证券，收到 %q", raw)
	}
	return symbol, nil
}

func hithinkPositiveFinite(value float64) bool {
	return value > 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hithinkNonNegativeFinite(value float64) bool {
	return value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hithinkFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func hithinkAdjustmentBasis(adjust string) string {
	switch adjust {
	case "forward":
		return string(model.AdjustmentForward)
	case "backward":
		return string(model.AdjustmentBackward)
	default:
		return string(model.AdjustmentNone)
	}
}
