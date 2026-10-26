package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const (
	tushareStockBasicVersion   = "tushare-stock-basic-v1"
	tushareCalendarVersion     = "tushare-trade-calendar-v1"
	tushareFundamentalsVersion = "tushare-fina-indicator-v1"
	tushareValuationVersion    = "tushare-daily-basic-v1"
)

// tushareTable 为 Tushare Pro 的通用字段/行响应提供按列名读取能力。
type tushareTable struct {
	positions map[string]int
	rows      [][]json.RawMessage
}

func (t *tushareTable) cell(row []json.RawMessage, field string) (json.RawMessage, error) {
	index, ok := t.positions[field]
	if !ok {
		return nil, fmt.Errorf("Tushare 响应缺少字段 %q", field)
	}
	if index < 0 || index >= len(row) {
		return nil, fmt.Errorf("Tushare 响应行缺少字段 %q 的值", field)
	}
	return bytes.TrimSpace(row[index]), nil
}

func (t *tushareTable) stringValue(row []json.RawMessage, field string) (string, error) {
	raw, err := t.cell(row, field)
	if err != nil {
		return "", err
	}
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return "", nil
	}
	return tushareString(raw)
}

func (t *tushareTable) optionalFloat(row []json.RawMessage, field string) (*float64, error) {
	raw, err := t.cell(row, field)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	var emptyString string
	if json.Unmarshal(raw, &emptyString) == nil && strings.TrimSpace(emptyString) == "" {
		return nil, nil
	}
	value, err := tushareRawFloat(raw)
	if err != nil {
		return nil, fmt.Errorf("Tushare 字段 %s 不是有效数值", field)
	}
	return &value, nil
}

func (p *Tushare) fetchCapabilityTable(ctx context.Context, apiName string, params map[string]string, fields string) (*tushareTable, error) {
	if p == nil || p.client == nil {
		return nil, errors.New("Tushare HTTP client 未配置")
	}
	if strings.TrimSpace(p.token) == "" {
		return nil, errors.New("Tushare Token 未配置")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := p.waitForRateLimit(ctx); err != nil {
		return nil, err
	}
	payload := map[string]any{
		"api_name": apiName,
		"token":    p.token,
		"params":   params,
		"fields":   fields,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("编码 Tushare %s 请求失败: %w", apiName, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("创建 Tushare %s 请求失败: %w", apiName, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "MarketDataService/1.0")
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Tushare %s 请求失败: %w", apiName, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("Tushare %s 返回 HTTP 状态码 %d", apiName, resp.StatusCode)
	}
	raw, err := readBoundedBody(resp.Body, maxProviderPayloadBytes)
	if err != nil {
		return nil, fmt.Errorf("读取 Tushare %s 响应失败: %w", apiName, err)
	}
	var envelope struct {
		Code *int    `json:"code"`
		Msg  string  `json:"msg"`
		Data *struct {
			Fields []string            `json:"fields"`
			Items  [][]json.RawMessage `json:"items"`
		} `json:"data"`
	}
	if err := decodeSingleJSON(raw, &envelope); err != nil {
		return nil, fmt.Errorf("解析 Tushare %s 响应失败: %w", apiName, err)
	}
	if envelope.Code == nil {
		return nil, fmt.Errorf("Tushare %s 响应缺少 code", apiName)
	}
	if *envelope.Code != 0 {
		message := strings.TrimSpace(envelope.Msg)
		if message == "" {
			message = "上游未提供错误描述"
		}
		if len(message) > 240 {
			message = message[:240]
		}
		return nil, fmt.Errorf("Tushare %s 错误 code=%d: %s", apiName, *envelope.Code, message)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("Tushare %s 响应缺少 data", apiName)
	}
	positions := make(map[string]int, len(envelope.Data.Fields))
	for index, field := range envelope.Data.Fields {
		field = strings.TrimSpace(field)
		if field == "" {
			return nil, fmt.Errorf("Tushare %s 响应含空字段名", apiName)
		}
		if _, duplicate := positions[field]; duplicate {
			return nil, fmt.Errorf("Tushare %s 响应含重复字段 %q", apiName, field)
		}
		positions[field] = index
	}
	return &tushareTable{positions: positions, rows: envelope.Data.Items}, nil
}

func tushareCompactDate(isoDate string) (string, error) {
	value := strings.TrimSpace(isoDate)
	if parsed, err := time.Parse("2006-01-02", value); err == nil && parsed.Format("2006-01-02") == value {
		return parsed.Format("20060102"), nil
	}
	if parsed, err := time.Parse("20060102", value); err == nil && parsed.Format("20060102") == value {
		return value, nil
	}
	return "", fmt.Errorf("无效日期 %q，应为 YYYY-MM-DD", isoDate)
}

func tushareISODate(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", errors.New("上游日期为空")
	}
	if parsed, err := time.Parse("20060102", value); err == nil && parsed.Format("20060102") == value {
		return parsed.Format("2006-01-02"), nil
	}
	if parsed, err := time.Parse("2006-01-02", value); err == nil && parsed.Format("2006-01-02") == value {
		return value, nil
	}
	return "", fmt.Errorf("上游日期 %q 格式无效", value)
}

func tushareExchangeCode(symbol string) (string, error) {
	canonical, err := normalize.NormalizeSymbol(symbol)
	if err != nil {
		return "", err
	}
	parts := strings.Split(canonical, ".")
	if len(parts) != 2 || len(parts[0]) != 6 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "BJ") {
		return "", fmt.Errorf("Tushare 仅支持带 SH/SZ/BJ 后缀的六位 A 股代码: %q", symbol)
	}
	for _, digit := range parts[0] {
		if digit < '0' || digit > '9' {
			return "", fmt.Errorf("Tushare 股票代码必须为六位数字: %q", symbol)
		}
	}
	return canonical, nil
}

// FetchStockBasic 通过 stock_basic 读取证券身份与上市状态。
// Tushare 此接口不提供当前估值，因此不会在这里推算 PE/PB 或市值。
func (p *Tushare) FetchStockBasic(ctx context.Context, rawSymbol string) (*model.StockBasic, error) {
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	table, err := p.fetchCapabilityTable(ctx, "stock_basic", map[string]string{"ts_code": symbol}, "ts_code,name,industry,market,exchange,list_status,list_date,delist_date")
	if err != nil {
		return nil, err
	}
	if len(table.rows) == 0 {
		return nil, fmt.Errorf("Tushare stock_basic 未返回 %s", symbol)
	}
	var selected []json.RawMessage
	for _, row := range table.rows {
		code, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		canonical, normErr := normalize.NormalizeSymbol(code)
		if normErr != nil || canonical != symbol {
			return nil, fmt.Errorf("Tushare stock_basic 返回证券代码不匹配: requested=%s returned=%q", symbol, code)
		}
		selected = row
		break
	}
	if selected == nil {
		return nil, fmt.Errorf("Tushare stock_basic 未找到请求证券 %s", symbol)
	}
	name, err := table.stringValue(selected, "name")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("Tushare stock_basic 的 %s 缺少证券名称", symbol)
	}
	industry, err := table.stringValue(selected, "industry")
	if err != nil {
		return nil, err
	}
	market, err := table.stringValue(selected, "market")
	if err != nil {
		return nil, err
	}
	status, err := table.stringValue(selected, "list_status")
	if err != nil {
		return nil, err
	}
	listDateRaw, err := table.stringValue(selected, "list_date")
	if err != nil {
		return nil, err
	}
	delistDateRaw, err := table.stringValue(selected, "delist_date")
	if err != nil {
		return nil, err
	}
	listDate, delistDate := "", ""
	if strings.TrimSpace(listDateRaw) != "" {
		listDate, err = tushareISODate(listDateRaw)
		if err != nil {
			return nil, fmt.Errorf("Tushare stock_basic 上市日期无效: %w", err)
		}
	}
	if strings.TrimSpace(delistDateRaw) != "" {
		delistDate, err = tushareISODate(delistDateRaw)
		if err != nil {
			return nil, fmt.Errorf("Tushare stock_basic 退市日期无效: %w", err)
		}
	}
	listingStatus, listingLabel := 0, "非上市/已退市"
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "L":
		listingStatus, listingLabel = 1, "上市"
	case "D":
		listingStatus, listingLabel = 0, "退市"
	case "P":
		listingStatus, listingLabel = 0, "暂停上市"
	default:
		return nil, fmt.Errorf("Tushare stock_basic 返回未知上市状态 %q", status)
	}
	securityType, securityLabel := 1, "A股"
	switch strings.TrimSpace(market) {
	case "创业板":
		securityType, securityLabel = 2, "创业板股票"
	case "科创板":
		securityType, securityLabel = 3, "科创板股票"
	case "北交所":
		securityType, securityLabel = 4, "北交所股票"
	case "主板":
		securityType, securityLabel = 1, "主板股票"
	}
	now := time.Now().UTC()
	return &model.StockBasic{
		Symbol: symbol, Name: name, IPODate: listDate, OutDate: delistDate,
		SecurityType: securityType, SecurityTypeLabel: securityLabel,
		ListingStatus: listingStatus, ListingStatusLabel: listingLabel,
		Industry: industry, Source: p.Name(), SourceVersion: tushareStockBasicVersion,
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons: []string{"tushare_stock_basic_does_not_include_current_valuation_metrics", "provider_exact_snapshot_time_not_exposed"},
		NormalizationVer: normalize.Version,
	}, nil
}

// FetchTradingCalendar returns a complete inclusive calendar range. Missing or duplicated dates are treated as provider errors.
func (p *Tushare) FetchTradingCalendar(ctx context.Context, startDate, endDate string) (*model.TradingCalendarSnapshot, error) {
	startCompact, err := tushareCompactDate(startDate)
	if err != nil {
		return nil, err
	}
	endCompact, err := tushareCompactDate(endDate)
	if err != nil {
		return nil, err
	}
	start, _ := time.Parse("20060102", startCompact)
	end, _ := time.Parse("20060102", endCompact)
	if start.After(end) {
		return nil, errors.New("Tushare 交易日历开始日期不能晚于结束日期")
	}
	totalDays := int(end.Sub(start)/(24*time.Hour)) + 1
	if totalDays > 3660 {
		return nil, errors.New("Tushare 交易日历区间不能超过 3660 个自然日")
	}
	table, err := p.fetchCapabilityTable(ctx, "trade_cal", map[string]string{
		"exchange": "SSE", "start_date": startCompact, "end_date": endCompact,
	}, "exchange,cal_date,is_open")
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]bool, len(table.rows))
	for index, row := range table.rows {
		dateRaw, readErr := table.stringValue(row, "cal_date")
		if readErr != nil {
			return nil, fmt.Errorf("Tushare 交易日历第 %d 行: %w", index, readErr)
		}
		date, dateErr := tushareISODate(dateRaw)
		if dateErr != nil {
			return nil, fmt.Errorf("Tushare 交易日历第 %d 行: %w", index, dateErr)
		}
		if date < start.Format("2006-01-02") || date > end.Format("2006-01-02") {
			return nil, fmt.Errorf("Tushare 交易日历返回了范围外日期 %s", date)
		}
		if _, exists := byDate[date]; exists {
			return nil, fmt.Errorf("Tushare 交易日历返回重复日期 %s", date)
		}
		openRaw, readErr := table.stringValue(row, "is_open")
		if readErr != nil {
			return nil, readErr
		}
		switch strings.TrimSpace(openRaw) {
		case "0":
			byDate[date] = false
		case "1":
			byDate[date] = true
		default:
			return nil, fmt.Errorf("Tushare 交易日历 %s 的 is_open 无效: %q", date, openRaw)
		}
	}
	items := make([]model.TradingCalendarDay, 0, totalDays)
	tradingDays := 0
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		iso := date.Format("2006-01-02")
		isTradingDay, ok := byDate[iso]
		if !ok {
			return nil, fmt.Errorf("Tushare 交易日历缺少自然日 %s，拒绝将不完整数据视为完整日历", iso)
		}
		if isTradingDay {
			tradingDays++
		}
		items = append(items, model.TradingCalendarDay{Date: iso, IsTradingDay: isTradingDay})
	}
	return &model.TradingCalendarSnapshot{
		Market: "cn_a_share", StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
		TotalDays: totalDays, TradingDays: tradingDays, Items: items,
		Source: p.Name(), SourceVersion: tushareCalendarVersion, FetchedAt: time.Now().UTC(),
		QualityStatus: model.QualityValid,
	}, nil
}

func tushareQuarterEnd(year, quarter int) (time.Time, error) {
	if year < 2007 || year > time.Now().Year() || quarter < 1 || quarter > 4 {
		return time.Time{}, errors.New("Tushare 财报年份或季度无效")
	}
	return time.Date(year, time.Month(quarter*3+1), 0, 0, 0, 0, 0, time.UTC), nil
}

type tushareMetricDefinition struct {
	field string
	key   string
	name  string
	unit  string
}

func tushareMetric(field, name, unit string) tushareMetricDefinition {
	return tushareMetricDefinition{field: field, key: field, name: name, unit: unit}
}

func tushareFundamentalDefinitions() []struct {
	category string
	metrics  []tushareMetricDefinition
} {
	return []struct {
		category string
		metrics  []tushareMetricDefinition
	}{
		{"profitability", []tushareMetricDefinition{
			tushareMetric("eps", "基本每股收益", "CNY/share"),
			tushareMetric("dt_eps", "稀释每股收益", "CNY/share"),
			tushareMetric("roe", "净资产收益率", "%"),
			tushareMetric("roe_waa", "加权净资产收益率", "%"),
			tushareMetric("roe_dt", "扣非净资产收益率", "%"),
			tushareMetric("roa", "总资产报酬率", "%"),
			tushareMetric("netprofit_margin", "销售净利率", "%"),
			tushareMetric("grossprofit_margin", "销售毛利率", "%"),
		}},
		{"operating", []tushareMetricDefinition{
			tushareMetric("assets_turn", "总资产周转率", "times"),
			tushareMetric("inv_turn", "存货周转率", "times"),
			tushareMetric("ar_turn", "应收账款周转率", "times"),
			tushareMetric("ca_turn", "流动资产周转率", "times"),
			tushareMetric("fa_turn", "固定资产周转率", "times"),
		}},
		{"growth", []tushareMetricDefinition{
			tushareMetric("netprofit_yoy", "净利润同比增长率", "%"),
			tushareMetric("or_yoy", "营业收入同比增长率", "%"),
			tushareMetric("assets_yoy", "总资产同比增长率", "%"),
			tushareMetric("eqt_yoy", "股东权益同比增长率", "%"),
		}},
		{"solvency", []tushareMetricDefinition{
			tushareMetric("current_ratio", "流动比率", "ratio"),
			tushareMetric("quick_ratio", "速动比率", "ratio"),
			tushareMetric("cash_ratio", "现金比率", "ratio"),
			tushareMetric("debt_to_assets", "资产负债率", "%"),
		}},
		{"cash_flow", []tushareMetricDefinition{
			tushareMetric("ocf_to_or", "经营现金流/营业收入", "%"),
			tushareMetric("ocf_to_profit", "经营现金流/净利润", "%"),
			tushareMetric("ocf_to_debt", "经营现金流/负债", "%"),
		}},
		{"dupont", []tushareMetricDefinition{
			tushareMetric("dupont_roe", "杜邦净资产收益率", "%"),
			tushareMetric("dupont_asset_turn", "杜邦总资产周转率", "times"),
			tushareMetric("dupont_pnitoni", "杜邦权益乘数", "ratio"),
			tushareMetric("dupont_tax_burden", "杜邦税负系数", "ratio"),
			tushareMetric("dupont_intburden", "杜邦利息负担系数", "ratio"),
		}},
	}
}

// FetchFundamentals reads one explicit report period and preserves the publication date separately.
func (p *Tushare) FetchFundamentals(ctx context.Context, rawSymbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	period, err := tushareQuarterEnd(year, quarter)
	if err != nil {
		return nil, err
	}
	periodCompact := period.Format("20060102")
	fields := "ts_code,ann_date,end_date,eps,dt_eps,roe,roe_waa,roe_dt,roa,netprofit_margin,grossprofit_margin,assets_turn,inv_turn,ar_turn,ca_turn,fa_turn,netprofit_yoy,or_yoy,assets_yoy,eqt_yoy,current_ratio,quick_ratio,cash_ratio,debt_to_assets,ocf_to_or,ocf_to_profit,ocf_to_debt,dupont_roe,dupont_asset_turn,dupont_pnitoni,dupont_tax_burden,dupont_intburden"
	table, err := p.fetchCapabilityTable(ctx, "fina_indicator", map[string]string{
		"ts_code": symbol, "period": periodCompact,
	}, fields)
	if err != nil {
		return nil, err
	}
	var selected []json.RawMessage
	selectedAnnouncementDate := ""
	for index, row := range table.rows {
		code, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		canonical, normErr := normalize.NormalizeSymbol(code)
		if normErr != nil || canonical != symbol {
			return nil, fmt.Errorf("Tushare 财务指标第 %d 行代码不匹配: requested=%s returned=%q", index, symbol, code)
		}
		endDateRaw, readErr := table.stringValue(row, "end_date")
		if readErr != nil {
			return nil, readErr
		}
		endDate, dateErr := tushareISODate(endDateRaw)
		if dateErr != nil || endDate != period.Format("2006-01-02") {
			continue
		}
		announcementRaw, readErr := table.stringValue(row, "ann_date")
		if readErr != nil {
			return nil, readErr
		}
		announcementDate := ""
		if strings.TrimSpace(announcementRaw) != "" {
			announcementDate, dateErr = tushareISODate(announcementRaw)
			if dateErr != nil {
				return nil, fmt.Errorf("Tushare 财务公告日期无效: %w", dateErr)
			}
		}
		if selected == nil || announcementDate > selectedAnnouncementDate {
			selected = row
			selectedAnnouncementDate = announcementDate
		}
	}
	if selected == nil {
		return nil, fmt.Errorf("Tushare fina_indicator 未返回 %s 报告期 %s 的记录", symbol, period.Format("2006-01-02"))
	}
	now := time.Now().UTC()
	sections := make([]model.FundamentalSection, 0, 6)
	snapshotReasons := []string{}
	allSectionsComplete := selectedAnnouncementDate != ""
	if selectedAnnouncementDate == "" {
		snapshotReasons = appendTushareReason(snapshotReasons, "provider_publication_date_missing")
	}
	hasData := false
	for _, definition := range tushareFundamentalDefinitions() {
		metrics := make([]model.FundamentalMetric, 0, len(definition.metrics))
		sectionReasons := []string{}
		for _, metricDefinition := range definition.metrics {
			value, readErr := table.optionalFloat(selected, metricDefinition.field)
			if readErr != nil {
				return nil, readErr
			}
			if value == nil {
				sectionReasons = appendTushareReason(sectionReasons, "provider_metric_missing:"+metricDefinition.field)
				continue
			}
			metrics = append(metrics, model.FundamentalMetric{
				Key: metricDefinition.key, Name: metricDefinition.name, Value: *value,
				Unit: metricDefinition.unit, SourceField: metricDefinition.field,
			})
		}
		sectionHasData := len(metrics) > 0
		sectionStatus := model.QualityUnknown
		if sectionHasData {
			hasData = true
			sectionStatus = model.QualityValid
			if len(metrics) != len(definition.metrics) || selectedAnnouncementDate == "" {
				sectionStatus = model.QualityDegraded
				allSectionsComplete = false
			}
		} else {
			allSectionsComplete = false
			sectionReasons = appendTushareReason(sectionReasons, "provider_section_has_no_values")
		}
		if len(sectionReasons) > 0 {
			allSectionsComplete = false
			for _, reason := range sectionReasons {
				snapshotReasons = appendTushareReason(snapshotReasons, definition.category+":"+reason)
			}
		}
		sections = append(sections, model.FundamentalSection{
			Category: definition.category, ReportDate: period.Format("2006-01-02"),
			PublicationDate: selectedAnnouncementDate, HasData: sectionHasData, Metrics: metrics,
			QualityStatus: sectionStatus, QualityReasons: sectionReasons,
		})
	}
	if !hasData {
		return nil, fmt.Errorf("Tushare fina_indicator 在 %s 报告期没有可用指标", period.Format("2006-01-02"))
	}
	status := model.QualityDegraded
	if allSectionsComplete {
		status = model.QualityValid
	}
	return &model.FundamentalsSnapshot{
		Symbol: symbol, Year: year, Quarter: quarter, HasData: true, Sections: sections,
		Source: p.Name(), SourceVersion: tushareFundamentalsVersion, FetchedAt: now,
		QualityStatus: status, QualityReasons: snapshotReasons,
	}, nil
}

func appendTushareReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func tushareCalendarOpen(table *tushareTable, row []json.RawMessage) (bool, error) {
	value, err := table.stringValue(row, "is_open")
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(value) {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("Tushare is_open 值无效: %q", value)
	}
}

func (p *Tushare) latestValuationTradeDate(ctx context.Context) (string, error) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	now := time.Now().In(loc)
	reference := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	// daily_basic 通常在收盘后更新；18:00 前只选此前已完成的交易日。
	if now.Hour() < 18 {
		reference = reference.AddDate(0, 0, -1)
	}
	start := reference.AddDate(0, 0, -45)
	table, err := p.fetchCapabilityTable(ctx, "trade_cal", map[string]string{
		"exchange": "SSE", "start_date": start.Format("20060102"), "end_date": reference.Format("20060102"),
	}, "exchange,cal_date,is_open")
	if err != nil {
		return "", err
	}
	latest := ""
	for _, row := range table.rows {
		dateRaw, readErr := table.stringValue(row, "cal_date")
		if readErr != nil {
			return "", readErr
		}
		date, dateErr := tushareISODate(dateRaw)
		if dateErr != nil {
			return "", dateErr
		}
		isOpen, openErr := tushareCalendarOpen(table, row)
		if openErr != nil {
			return "", openErr
		}
		if !isOpen || date > reference.Format("2006-01-02") {
			continue
		}
		if date > latest {
			latest = date
		}
	}
	if latest == "" {
		return "", errors.New("Tushare 最近 45 个自然日内没有找到已完成的交易日")
	}
	return latest, nil
}

// FetchValuations batches daily_basic requests on the last completed trading day.
// Exact intraday timestamps and PCF TTM are not available from this endpoint, so the result is explicitly degraded.
func (p *Tushare) FetchValuations(ctx context.Context, rawSymbols []string) (*model.ValuationSnapshot, error) {
	if len(rawSymbols) == 0 || len(rawSymbols) > 100 {
		return nil, errors.New("Tushare 估值查询仅支持 1..100 个股票代码")
	}
	symbols := make([]string, 0, len(rawSymbols))
	seen := make(map[string]struct{}, len(rawSymbols))
	for _, raw := range rawSymbols {
		symbol, err := tushareExchangeCode(raw)
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
		return nil, errors.New("Tushare 估值请求没有有效代码")
	}
	tradeDate, err := p.latestValuationTradeDate(ctx)
	if err != nil {
		return nil, fmt.Errorf("选择 Tushare 估值交易日失败: %w", err)
	}
	compactDate, err := tushareCompactDate(tradeDate)
	if err != nil {
		return nil, err
	}
	table, err := p.fetchCapabilityTable(ctx, "daily_basic", map[string]string{
		"ts_code": strings.Join(symbols, ","), "trade_date": compactDate,
	}, "ts_code,trade_date,pe,pe_ttm,pb,ps_ttm")
	if err != nil {
		return nil, err
	}
	if len(table.rows) == 0 {
		return nil, fmt.Errorf("Tushare daily_basic 在交易日 %s 没有返回任何请求股票的估值", tradeDate)
	}
	type values struct {
		item model.ValuationItem
	}
	bySymbol := make(map[string]values, len(table.rows))
	now := time.Now().UTC()
	snapshotReasons := []string{"provider_exact_snapshot_time_not_exposed", "provider_pcf_ttm_not_supported_by_tushare_daily_basic"}
	for index, row := range table.rows {
		rawSymbol, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		symbol, symbolErr := normalize.NormalizeSymbol(rawSymbol)
		if symbolErr != nil {
			return nil, fmt.Errorf("Tushare daily_basic 第 %d 行代码无效: %q", index, rawSymbol)
		}
		if _, requested := seen[symbol]; !requested {
			return nil, fmt.Errorf("Tushare daily_basic 返回了未请求的证券 %s", symbol)
		}
		if _, duplicate := bySymbol[symbol]; duplicate {
			return nil, fmt.Errorf("Tushare daily_basic 返回重复证券 %s", symbol)
		}
		rowDateRaw, readErr := table.stringValue(row, "trade_date")
		if readErr != nil {
			return nil, readErr
		}
		rowDate, dateErr := tushareISODate(rowDateRaw)
		if dateErr != nil || rowDate != tradeDate {
			return nil, fmt.Errorf("Tushare daily_basic 日期不匹配: requested=%s returned=%q", tradeDate, rowDateRaw)
		}
		pe, readErr := table.optionalFloat(row, "pe")
		if readErr != nil {
			return nil, readErr
		}
		peTTM, readErr := table.optionalFloat(row, "pe_ttm")
		if readErr != nil {
			return nil, readErr
		}
		pb, readErr := table.optionalFloat(row, "pb")
		if readErr != nil {
			return nil, readErr
		}
		psTTM, readErr := table.optionalFloat(row, "ps_ttm")
		if readErr != nil {
			return nil, readErr
		}
		itemReasons := []string{"provider_exact_snapshot_time_not_exposed", "provider_pcf_ttm_not_supported_by_tushare_daily_basic"}
		if pe == nil || peTTM == nil || pb == nil || psTTM == nil {
			itemReasons = appendTushareReason(itemReasons, "one_or_more_valuation_metrics_missing")
		}
		bySymbol[symbol] = values{item: model.ValuationItem{
			Symbol: symbol, Code: strings.Split(symbol, ".")[0],
			PETTM: peTTM, PEMRQ: pe, PBMRQ: pb, PSTTM: psTTM, PCFTTM: nil,
			Source: p.Name(), SourceVersion: tushareValuationVersion, FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: itemReasons,
		}}
	}
	items := make([]model.ValuationItem, 0, len(bySymbol))
	missing := make([]string, 0, len(symbols)-len(bySymbol))
	for _, symbol := range symbols {
		if value, ok := bySymbol[symbol]; ok {
			items = append(items, value.item)
		} else {
			missing = append(missing, symbol)
		}
	}
	if len(missing) > 0 {
		snapshotReasons = appendTushareReason(snapshotReasons, "one_or_more_requested_symbols_missing_from_provider_response")
	}
	status := model.QualityDegraded
	return &model.ValuationSnapshot{
		RequestedSymbols: symbols, Total: len(items), Items: items, MissingSymbols: missing,
		ProviderReportDate: tradeDate, ProviderTimestamp: nil, Source: p.Name(),
		SourceVersion: tushareValuationVersion, FetchedAt: now,
		QualityStatus: status, QualityReasons: snapshotReasons,
	}, nil
}
