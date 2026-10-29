package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const (
	tushareStockBasicVersion   = "tushare-stock-basic-v1"
	tushareCalendarVersion     = "tushare-trade-calendar-v1"
	tushareFundamentalsVersion = "tushare-fina-indicator-v1"
	tushareValuationVersion    = "tushare-daily-basic-v2"
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
		Code *int   `json:"code"`
		Msg  string `json:"msg"`
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
	// tushareExchangeCode 校验格式和后缀，不等同于普通 A 股证券范围校验。
	// 防止 ETF、基金及其他非股票证券通过六位代码检查后被错误标为“主板股票”。
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tushare stock_basic 仅支持 A 股普通股票，证券 %s 不在支持范围内", symbol)
	}
	table, err := p.fetchCapabilityTable(ctx, "stock_basic", map[string]string{"ts_code": symbol}, "ts_code,name,industry,market,exchange,list_status,list_date,delist_date")
	if err != nil {
		return nil, err
	}
	if len(table.rows) == 0 {
		return nil, fmt.Errorf("Tushare stock_basic 未返回 %s", symbol)
	}
	var selected []json.RawMessage
	for index, row := range table.rows {
		code, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		canonical, normErr := normalize.NormalizeSymbol(code)
		if normErr != nil || canonical != symbol {
			return nil, fmt.Errorf("Tushare stock_basic 返回证券代码不匹配: requested=%s returned=%q", symbol, code)
		}
		if selected != nil {
			return nil, fmt.Errorf("Tushare stock_basic 第 %d 行重复返回证券 %s", index, symbol)
		}
		selected = row
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
	securityType, securityLabel := 0, ""
	switch strings.TrimSpace(market) {
	case "创业板":
		securityType, securityLabel = 2, "创业板股票"
	case "科创板":
		securityType, securityLabel = 3, "科创板股票"
	case "北交所":
		securityType, securityLabel = 4, "北交所股票"
	case "主板":
		securityType, securityLabel = 1, "主板股票"
	default:
		return nil, fmt.Errorf("Tushare stock_basic 返回未知或缺失的市场分类 %q，拒绝默认归类为主板", market)
	}
	now := time.Now().UTC()
	return &model.StockBasic{
		Symbol: symbol, Name: name, IPODate: listDate, OutDate: delistDate,
		SecurityType: securityType, SecurityTypeLabel: securityLabel,
		ListingStatus: listingStatus, ListingStatusLabel: listingLabel,
		Industry: industry, Source: p.Name(), SourceVersion: tushareStockBasicVersion,
		FetchedAt: now, QualityStatus: model.QualityDegraded,
		QualityReasons:   []string{"tushare_stock_basic_does_not_include_current_valuation_metrics", "provider_exact_snapshot_time_not_exposed"},
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
type tushareStatementMetricDefinition struct {
	field    string
	key      string
	name     string
	unit     string
	required bool
}

type tushareStatementDefinition struct {
	apiName  string
	category string
	metrics  []tushareStatementMetricDefinition
}

// tushareStatementDefinitions 按 Tushare 官方字段契约映射三张 A 股合并财务报表；不把未知字段或缺失值补零。
func tushareStatementDefinitions() []tushareStatementDefinition {
	return []tushareStatementDefinition{
		{
			apiName: "income", category: "income_statement",
			metrics: []tushareStatementMetricDefinition{
				{field: "basic_eps", key: "basic_eps", name: "基本每股收益", unit: "CNY/share"},
				{field: "diluted_eps", key: "diluted_eps", name: "稀释每股收益", unit: "CNY/share"},
				{field: "prem_earned", key: "earned_premiums", name: "已赚保费", unit: "CNY"},
				{field: "comm_income", key: "commission_income", name: "手续费及佣金收入", unit: "CNY"},
				{field: "n_oth_income", key: "net_other_income", name: "其他收益", unit: "CNY"},
				{field: "n_oth_b_income", key: "net_other_business_income", name: "其他业务净收益", unit: "CNY"},
				{field: "oth_b_income", key: "other_business_income", name: "其他业务收入", unit: "CNY"},
				{field: "fv_value_chg_gain", key: "fair_value_change_gain", name: "公允价值变动收益", unit: "CNY"},
				{field: "ass_invest_income", key: "associate_investment_income", name: "对联营企业和合营企业的投资收益", unit: "CNY"},
				{field: "forex_gain", key: "foreign_exchange_gain", name: "汇兑收益", unit: "CNY"},
				{field: "int_exp", key: "interest_expenses", name: "利息支出", unit: "CNY"},
				{field: "comm_exp", key: "commission_expenses", name: "手续费及佣金支出", unit: "CNY"},
				{field: "biz_tax_surchg", key: "business_tax_surcharges", name: "营业税金及附加", unit: "CNY"},
				{field: "total_revenue", key: "total_revenue", required: true, name: "营业总收入", unit: "CNY"},
				{field: "revenue", key: "revenue", name: "营业收入", unit: "CNY"},
				{field: "int_income", key: "interest_income", name: "利息收入", unit: "CNY"},
				{field: "n_commis_income", key: "net_commission_income", name: "手续费及佣金净收入", unit: "CNY"},
				{field: "invest_income", key: "investment_income", name: "投资净收益", unit: "CNY"},
				{field: "total_cogs", key: "total_cogs", name: "营业总成本", unit: "CNY"},
				{field: "oper_cost", key: "operating_cost", name: "营业成本", unit: "CNY"},
				{field: "sell_exp", key: "selling_expenses", name: "销售费用", unit: "CNY"},
				{field: "admin_exp", key: "administrative_expenses", name: "管理费用", unit: "CNY"},
				{field: "fin_exp", key: "financial_expenses", name: "财务费用", unit: "CNY"},
				{field: "assets_impair_loss", key: "asset_impairment_loss", name: "资产减值损失", unit: "CNY"},
				{field: "operate_profit", key: "operating_profit", name: "营业利润", unit: "CNY"},
				{field: "non_oper_income", key: "non_operating_income", name: "营业外收入", unit: "CNY"},
				{field: "non_oper_exp", key: "non_operating_expenses", name: "营业外支出", unit: "CNY"},
				{field: "total_profit", key: "total_profit", required: true, name: "利润总额", unit: "CNY"},
				{field: "income_tax", key: "income_tax", name: "所得税费用", unit: "CNY"},
				{field: "n_income", key: "net_income", required: true, name: "净利润（含少数股东损益）", unit: "CNY"},
				{field: "n_income_attr_p", key: "parent_net_income", required: true, name: "归属于母公司股东的净利润", unit: "CNY"},
				{field: "minority_gain", key: "minority_profit", name: "少数股东损益", unit: "CNY"},
				{field: "oth_compr_income", key: "other_comprehensive_income", name: "其他综合收益", unit: "CNY"},
				{field: "t_compr_income", key: "total_comprehensive_income", name: "综合收益总额", unit: "CNY"},
				{field: "compr_inc_attr_p", key: "parent_comprehensive_income", name: "归属于母公司股东的综合收益总额", unit: "CNY"},
				{field: "ebit", key: "ebit", name: "息税前利润", unit: "CNY"},
				{field: "ebitda", key: "ebitda", name: "息税折旧摊销前利润", unit: "CNY"},
				{field: "rd_exp", key: "rd_expenses", name: "研发费用", unit: "CNY"},
				{field: "fin_exp_int_exp", key: "financial_interest_expenses", name: "财务费用中的利息费用", unit: "CNY"},
				{field: "fin_exp_int_inc", key: "financial_interest_income", name: "财务费用中的利息收入", unit: "CNY"},
			},
		},
		{
			apiName: "balancesheet", category: "balance_sheet",
			metrics: []tushareStatementMetricDefinition{
				{field: "total_share", key: "total_shares", name: "期末总股本", unit: "shares"},
				{field: "special_rese", key: "special_reserve", name: "专项储备", unit: "CNY"},
				{field: "receiv_financing", key: "receivables_financing", name: "应收款项融资", unit: "CNY"},
				{field: "accounts_receiv_bill", key: "notes_and_accounts_receivable", name: "应收票据及应收账款", unit: "CNY"},
				{field: "accounts_pay", key: "notes_and_accounts_payable", name: "应付票据及应付账款", unit: "CNY"},
				{field: "oth_rcv_total", key: "total_other_receivables", name: "其他应收款合计", unit: "CNY"},
				{field: "fix_assets_total", key: "total_fixed_assets", name: "固定资产合计", unit: "CNY"},
				{field: "use_right_assets", key: "right_of_use_assets", name: "使用权资产", unit: "CNY"},
				{field: "lease_liab", key: "lease_liabilities", name: "租赁负债", unit: "CNY"},
				{field: "defer_tax_liab", key: "deferred_tax_liabilities", name: "递延所得税负债", unit: "CNY"},
				{field: "oth_cur_liab", key: "other_current_liabilities", name: "其他流动负债", unit: "CNY"},
				{field: "debt_invest", key: "debt_investments", name: "债权投资", unit: "CNY"},
				{field: "oth_debt_invest", key: "other_debt_investments", name: "其他债权投资", unit: "CNY"},
				{field: "cap_rese", key: "capital_reserve", name: "资本公积金", unit: "CNY"},
				{field: "surplus_rese", key: "surplus_reserve", name: "盈余公积金", unit: "CNY"},
				{field: "undistr_porfit", key: "undistributed_profit", name: "未分配利润", unit: "CNY"},
				{field: "money_cap", key: "cash_and_bank", name: "货币资金", unit: "CNY"},
				{field: "trad_asset", key: "trading_financial_assets", name: "交易性金融资产", unit: "CNY"},
				{field: "notes_receiv", key: "notes_receivable", name: "应收票据", unit: "CNY"},
				{field: "accounts_receiv", key: "accounts_receivable", name: "应收账款", unit: "CNY"},
				{field: "oth_receiv", key: "other_receivables", name: "其他应收款", unit: "CNY"},
				{field: "prepayment", key: "prepayments", name: "预付款项", unit: "CNY"},
				{field: "div_receiv", key: "dividends_receivable", name: "应收股利", unit: "CNY"},
				{field: "int_receiv", key: "interest_receivable", name: "应收利息", unit: "CNY"},
				{field: "inventories", key: "inventories", name: "存货", unit: "CNY"},
				{field: "oth_cur_assets", key: "other_current_assets", name: "其他流动资产", unit: "CNY"},
				{field: "total_cur_assets", key: "total_current_assets", name: "流动资产合计", unit: "CNY"},
				{field: "lt_eqt_invest", key: "long_term_equity_investments", name: "长期股权投资", unit: "CNY"},
				{field: "fix_assets", key: "fixed_assets", name: "固定资产", unit: "CNY"},
				{field: "cip", key: "construction_in_progress", name: "在建工程", unit: "CNY"},
				{field: "const_materials", key: "construction_materials", name: "工程物资", unit: "CNY"},
				{field: "intan_assets", key: "intangible_assets", name: "无形资产", unit: "CNY"},
				{field: "r_and_d", key: "research_and_development_assets", name: "研发支出", unit: "CNY"},
				{field: "goodwill", key: "goodwill", name: "商誉", unit: "CNY"},
				{field: "defer_tax_assets", key: "deferred_tax_assets", name: "递延所得税资产", unit: "CNY"},
				{field: "total_nca", key: "total_non_current_assets", name: "非流动资产合计", unit: "CNY"},
				{field: "total_assets", key: "total_assets", required: true, name: "资产总计", unit: "CNY"},
				{field: "st_borr", key: "short_term_borrowings", name: "短期借款", unit: "CNY"},
				{field: "notes_payable", key: "notes_payable", name: "应付票据", unit: "CNY"},
				{field: "acct_payable", key: "accounts_payable", name: "应付账款", unit: "CNY"},
				{field: "payroll_payable", key: "payroll_payable", name: "应付职工薪酬", unit: "CNY"},
				{field: "taxes_payable", key: "taxes_payable", name: "应交税费", unit: "CNY"},
				{field: "div_payable", key: "dividends_payable", name: "应付股利", unit: "CNY"},
				{field: "oth_payable", key: "other_payables", name: "其他应付款", unit: "CNY"},
				{field: "non_cur_liab_due_1y", key: "non_current_liabilities_due_within_one_year", name: "一年内到期的非流动负债", unit: "CNY"},
				{field: "total_cur_liab", key: "total_current_liabilities", name: "流动负债合计", unit: "CNY"},
				{field: "lt_borr", key: "long_term_borrowings", name: "长期借款", unit: "CNY"},
				{field: "bond_payable", key: "bonds_payable", name: "应付债券", unit: "CNY"},
				{field: "total_ncl", key: "total_non_current_liabilities", name: "非流动负债合计", unit: "CNY"},
				{field: "total_liab", key: "total_liabilities", required: true, name: "负债合计", unit: "CNY"},
				{field: "treasury_share", key: "treasury_shares", name: "库存股", unit: "CNY"},
				{field: "minority_int", key: "minority_interest", name: "少数股东权益", unit: "CNY"},
				{field: "total_hldr_eqy_exc_min_int", key: "equity_excluding_minority", name: "股东权益合计（不含少数股东权益）", unit: "CNY"},
				{field: "total_hldr_eqy_inc_min_int", key: "equity_including_minority", required: true, name: "股东权益合计（含少数股东权益）", unit: "CNY"},
				{field: "total_liab_hldr_eqy", key: "total_liabilities_and_equity", name: "负债和股东权益总计", unit: "CNY"},
				{field: "contract_assets", key: "contract_assets", name: "合同资产", unit: "CNY"},
				{field: "contract_liab", key: "contract_liabilities", name: "合同负债", unit: "CNY"},
			},
		},
		{
			apiName: "cashflow", category: "cash_flow_statement",
			metrics: []tushareStatementMetricDefinition{
				{field: "net_profit", key: "net_profit", name: "净利润", unit: "CNY"},
				{field: "recp_tax_rends", key: "cash_received_tax_refunds", name: "收到的税费返还", unit: "CNY"},
				{field: "n_recp_disp_fiolta", key: "net_cash_received_from_long_lived_asset_disposals", name: "处置固定资产等长期资产收回的现金净额", unit: "CNY"},
				{field: "n_recp_disp_sobu", key: "net_cash_received_from_subsidiary_disposals", name: "处置子公司及其他营业单位收到的现金净额", unit: "CNY"},
				{field: "oth_cash_recp_ral_fnc_act", key: "other_financing_cash_received", name: "收到其他与筹资活动有关的现金", unit: "CNY"},
				{field: "oth_cashpay_ral_fnc_act", key: "other_financing_cash_paid", name: "支付其他与筹资活动有关的现金", unit: "CNY"},
				{field: "incl_dvd_profit_paid_sc_ms", key: "dividends_paid_to_minority_shareholders", name: "支付给少数股东的股利利润", unit: "CNY"},
				{field: "prov_depr_assets", key: "asset_impairment_provision", name: "资产减值准备", unit: "CNY"},
				{field: "depr_fa_coga_dpba", key: "fixed_asset_depreciation", name: "固定资产等折旧", unit: "CNY"},
				{field: "amort_intang_assets", key: "intangible_asset_amortization", name: "无形资产摊销", unit: "CNY"},
				{field: "use_right_asset_dep", key: "right_of_use_asset_depreciation", name: "使用权资产折旧", unit: "CNY"},
				{field: "im_n_incr_cash_equ", key: "indirect_net_increase_cash_equivalents", name: "现金及现金等价物净增加额（间接法）", unit: "CNY"},
				{field: "credit_impa_loss", key: "credit_impairment_loss", name: "信用减值损失", unit: "CNY"},
				{field: "finan_exp", key: "financial_expenses", name: "财务费用", unit: "CNY"},
				{field: "c_fr_sale_sg", key: "cash_received_from_sales", name: "销售商品、提供劳务收到的现金", unit: "CNY"},
				{field: "c_fr_oth_operate_a", key: "cash_received_from_other_operating_activities", name: "收到其他与经营活动有关的现金", unit: "CNY"},
				{field: "c_inf_fr_operate_a", key: "total_cash_inflows_operating", name: "经营活动现金流入小计", unit: "CNY"},
				{field: "c_paid_goods_s", key: "cash_paid_for_goods_and_services", name: "购买商品、接受劳务支付的现金", unit: "CNY"},
				{field: "c_paid_to_for_empl", key: "cash_paid_to_employees", name: "支付给职工以及为职工支付的现金", unit: "CNY"},
				{field: "c_paid_for_taxes", key: "cash_paid_for_taxes", name: "支付的各项税费", unit: "CNY"},
				{field: "oth_cash_pay_oper_act", key: "cash_paid_for_other_operating_activities", name: "支付其他与经营活动有关的现金", unit: "CNY"},
				{field: "st_cash_out_act", key: "total_cash_outflows_operating", name: "经营活动现金流出小计", unit: "CNY"},
				{field: "n_cashflow_act", key: "net_operating_cash_flow", required: true, name: "经营活动产生的现金流量净额", unit: "CNY"},
				{field: "oth_recp_ral_inv_act", key: "cash_received_from_other_investing_activities", name: "收到其他与投资活动有关的现金", unit: "CNY"},
				{field: "c_disp_withdrwl_invest", key: "cash_received_from_investment_recovery", name: "收回投资收到的现金", unit: "CNY"},
				{field: "c_recp_return_invest", key: "cash_received_from_investment_returns", name: "取得投资收益收到的现金", unit: "CNY"},
				{field: "stot_inflows_inv_act", key: "total_cash_inflows_investing", name: "投资活动现金流入小计", unit: "CNY"},
				{field: "c_pay_acq_const_fiolta", key: "cash_paid_for_long_term_assets", name: "购建固定资产等支付的现金", unit: "CNY"},
				{field: "c_paid_invest", key: "cash_paid_for_investments", name: "投资支付的现金", unit: "CNY"},
				{field: "oth_pay_ral_inv_act", key: "cash_paid_for_other_investing_activities", name: "支付其他与投资活动有关的现金", unit: "CNY"},
				{field: "stot_out_inv_act", key: "total_cash_outflows_investing", name: "投资活动现金流出小计", unit: "CNY"},
				{field: "n_cashflow_inv_act", key: "net_investing_cash_flow", required: true, name: "投资活动产生的现金流量净额", unit: "CNY"},
				{field: "c_recp_borrow", key: "cash_received_from_borrowings", name: "取得借款收到的现金", unit: "CNY"},
				{field: "stot_cash_in_fnc_act", key: "total_cash_inflows_financing", name: "筹资活动现金流入小计", unit: "CNY"},
				{field: "c_prepay_amt_borr", key: "cash_paid_for_debt_repayment", name: "偿还债务支付的现金", unit: "CNY"},
				{field: "c_pay_dist_dpcp_int_exp", key: "cash_paid_for_dividends_profit_interest", name: "分配股利、利润或偿付利息支付的现金", unit: "CNY"},
				{field: "stot_cashout_fnc_act", key: "total_cash_outflows_financing", name: "筹资活动现金流出小计", unit: "CNY"},
				{field: "n_cash_flows_fnc_act", key: "net_financing_cash_flow", required: true, name: "筹资活动产生的现金流量净额", unit: "CNY"},
				{field: "eff_fx_flu_cash", key: "effect_of_fx_changes_on_cash", name: "汇率变动对现金的影响", unit: "CNY"},
				{field: "n_incr_cash_cash_equ", key: "net_increase_cash_equivalents", name: "现金及现金等价物净增加额", unit: "CNY"},
				{field: "c_cash_equ_beg_period", key: "beginning_cash_and_equivalents", name: "期初现金及现金等价物余额", unit: "CNY"},
				{field: "c_cash_equ_end_period", key: "ending_cash_and_equivalents", name: "期末现金及现金等价物余额", unit: "CNY"},
				{field: "free_cashflow", key: "free_cash_flow", name: "企业自由现金流量", unit: "CNY"},
				{field: "im_net_cashflow_oper_act", key: "indirect_net_operating_cash_flow", name: "经营活动现金流量净额（间接法）", unit: "CNY"},
			},
		},
	}
}

// tushareStatementReportType 返回合并报表的查询类型：一季报本身是单季度值，后续季度才取单季合并类型。
func tushareStatementReportType(apiName, periodDate string) string {
	if apiName != "income" && apiName != "cashflow" {
		return "1"
	}
	if strings.HasSuffix(periodDate, "-03-31") {
		return "1"
	}
	return "2"
}

func (p *Tushare) fetchStatementSection(ctx context.Context, symbol, periodCompact, periodDate string, definition tushareStatementDefinition) (model.FundamentalSection, error) {
	fields := []string{"ts_code", "ann_date", "f_ann_date", "end_date", "report_type"}
	for _, metric := range definition.metrics {
		fields = append(fields, metric.field)
	}
	// 利润表/现金流量表：Q1 使用 report_type=1，Q2-Q4 使用 report_type=2 的单季合并报表；资产负债表各季度使用 report_type=1 的期末合并报表。
	reportType := tushareStatementReportType(definition.apiName, periodDate)
	table, err := p.fetchCapabilityTable(ctx, definition.apiName, map[string]string{
		"ts_code": symbol, "period": periodCompact, "report_type": reportType,
	}, strings.Join(fields, ","))
	if err != nil {
		return model.FundamentalSection{}, err
	}

	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(loc).Format("2006-01-02")
	var selected []json.RawMessage
	selectedPublicationDate := ""
	for rowIndex, row := range table.rows {
		rawCode, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return model.FundamentalSection{}, readErr
		}
		rowSymbol, symbolErr := normalize.NormalizeSymbol(rawCode)
		if symbolErr != nil || rowSymbol != symbol {
			return model.FundamentalSection{}, fmt.Errorf("Tushare %s 第 %d 行证券代码不匹配: %q", definition.apiName, rowIndex, rawCode)
		}
		rawEndDate, readErr := table.stringValue(row, "end_date")
		if readErr != nil {
			return model.FundamentalSection{}, readErr
		}
		endDate, dateErr := tushareISODate(rawEndDate)
		if dateErr != nil {
			return model.FundamentalSection{}, fmt.Errorf("Tushare %s 第 %d 行报告期无效: %q", definition.apiName, rowIndex, rawEndDate)
		}
		if endDate != periodDate {
			continue
		}
		rowReportType, readErr := table.stringValue(row, "report_type")
		if readErr != nil {
			return model.FundamentalSection{}, readErr
		}
		if strings.TrimSpace(rowReportType) != reportType {
			continue
		}

		publicationRaw, readErr := table.stringValue(row, "f_ann_date")
		if readErr != nil {
			return model.FundamentalSection{}, readErr
		}
		if strings.TrimSpace(publicationRaw) == "" {
			publicationRaw, readErr = table.stringValue(row, "ann_date")
			if readErr != nil {
				return model.FundamentalSection{}, readErr
			}
		}
		if strings.TrimSpace(publicationRaw) == "" {
			continue
		}
		publicationDate, dateErr := tushareISODate(publicationRaw)
		if dateErr != nil || publicationDate < periodDate || publicationDate > today {
			continue
		}
		if selected == nil || publicationDate > selectedPublicationDate {
			selected = row
			selectedPublicationDate = publicationDate
		}
	}
	if selected == nil {
		return model.FundamentalSection{
			Category: definition.category, ReportDate: periodDate, Metrics: []model.FundamentalMetric{},
			QualityStatus:  model.QualityUnknown,
			QualityReasons: []string{"no_statement_row_with_confirmed_publication_date_for_requested_period"},
		}, nil
	}

	section := model.FundamentalSection{
		Category: definition.category, ReportDate: periodDate, PublicationDate: selectedPublicationDate,
		Metrics:       make([]model.FundamentalMetric, 0, len(definition.metrics)),
		QualityStatus: model.QualityValid,
	}
	requiredMissingCount := 0
	for _, metric := range definition.metrics {
		value, valueErr := table.optionalFloat(selected, metric.field)
		if valueErr != nil || value == nil || math.IsNaN(*value) || math.IsInf(*value, 0) {
			// 非关键明细科目允许按行业/报表结构缺失；只记录关键汇总科目的缺失为质量问题。
			if metric.required {
				requiredMissingCount++
				reason := "required_statement_metric_missing_or_invalid:" + metric.field
				section.QualityReasons = appendTushareReason(section.QualityReasons, reason)
			}
			continue
		}
		section.Metrics = append(section.Metrics, model.FundamentalMetric{
			Key: metric.key, Name: metric.name, Value: *value, Unit: metric.unit, SourceField: metric.field,
		})
	}
	section.HasData = len(section.Metrics) > 0
	if requiredMissingCount > 0 {
		section.QualityStatus = model.QualityDegraded
		section.QualityReasons = appendTushareReason(section.QualityReasons, "one_or_more_required_statement_fields_missing_or_invalid")
	}
	if !section.HasData {
		section.QualityStatus = model.QualityDegraded
		section.QualityReasons = appendTushareReason(section.QualityReasons, "no_numeric_statement_fields_available")
	}
	return section, nil
}

func (p *Tushare) FetchFundamentals(ctx context.Context, rawSymbol string, year, quarter int) (*model.FundamentalsSnapshot, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tushare fundamentals only support common A-share equities, got %q", rawSymbol)
	}
	period, err := tushareQuarterEnd(year, quarter)
	if err != nil {
		return nil, err
	}
	periodCompact := period.Format("20060102")
	fields := "ts_code,ann_date,end_date,eps,dt_eps,roe,roe_waa,roe_dt,roa,netprofit_margin,grossprofit_margin,assets_turn,inv_turn,ar_turn,ca_turn,fa_turn,netprofit_yoy,or_yoy,assets_yoy,eqt_yoy,current_ratio,quick_ratio,cash_ratio,debt_to_assets,ocf_to_or,ocf_to_profit,ocf_to_debt,dupont_roe,dupont_asset_turn,dupont_pnitoni,dupont_tax_burden,dupont_intburden"
	table, indicatorErr := p.fetchCapabilityTable(ctx, "fina_indicator", map[string]string{
		"ts_code": symbol, "period": periodCompact,
	}, fields)
	if indicatorErr != nil {
		// 指标端点权限或响应失败不应阻断独立的利润表、资产负债表和现金流量表端点。
		table = &tushareTable{positions: map[string]int{}, rows: [][]json.RawMessage{}}
	}
	var selected []json.RawMessage
	selectedAnnouncementDate := ""
	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		location = time.FixedZone("CST", 8*60*60)
	}
	today := time.Now().In(location).Format("2006-01-02")
	periodDate := period.Format("2006-01-02")
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
		if strings.TrimSpace(announcementRaw) == "" {
			// 无公告日期就无法证明该版本在何时公开；排除该行，避免把时点未知的财务数值用于分析。
			continue
		}
		announcementDate, dateErr := tushareISODate(announcementRaw)
		if dateErr != nil {
			return nil, fmt.Errorf("Tushare 财务公告日期无效: %w", dateErr)
		}
		// 只从报告期结束后、当前日期之前已公开的修订版本中选择最新记录。
		if announcementDate < periodDate || announcementDate > today {
			continue
		}
		if selected == nil || announcementDate > selectedAnnouncementDate {
			selected = row
			selectedAnnouncementDate = announcementDate
		}
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
		if selected == nil {
			allSectionsComplete = false
			sectionQuality := model.QualityUnknown
			sectionReasons := []string{"no_published_indicator_row_for_requested_period"}
			if indicatorErr != nil {
				sectionQuality = model.QualityDegraded
				sectionReasons = []string{"provider_fina_indicator_endpoint_unavailable"}
				snapshotReasons = appendTushareReason(snapshotReasons, "provider_fina_indicator_endpoint_unavailable")
			} else {
				snapshotReasons = appendTushareReason(snapshotReasons, "no_published_indicator_row_for_requested_period")
			}
			sections = append(sections, model.FundamentalSection{
				Category: definition.category, ReportDate: periodDate, Metrics: []model.FundamentalMetric{},
				HasData: false, QualityStatus: sectionQuality, QualityReasons: sectionReasons,
			})
			continue
		}
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
	// 将 Tushare 的三张财务报表作为独立分区追加。某个报表权限不足或缺失时，
	// 仅降级该分区，不能丢弃已经取得的六组财务指标。
	for _, statementDefinition := range tushareStatementDefinitions() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		section, statementErr := p.fetchStatementSection(
			ctx, symbol, periodCompact, periodDate, statementDefinition,
		)
		if statementErr != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			section = model.FundamentalSection{
				Category: statementDefinition.category, ReportDate: periodDate, Metrics: []model.FundamentalMetric{},
				QualityStatus:  model.QualityDegraded,
				QualityReasons: []string{"provider_statement_endpoint_unavailable"},
			}
			snapshotReasons = appendTushareReason(snapshotReasons, statementDefinition.category+"_provider_unavailable")
		}
		if section.HasData {
			hasData = true
		}
		if !section.HasData || section.QualityStatus != model.QualityValid {
			allSectionsComplete = false
			snapshotReasons = appendTushareReason(snapshotReasons, statementDefinition.category+"_incomplete_or_unavailable")
		}
		sections = append(sections, section)
	}
	if !hasData {
		return nil, fmt.Errorf("Tushare 在 %s 报告期没有可用财务指标或报表字段", period.Format("2006-01-02"))
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
		if !isTushareAShareSymbol(symbol) {
			return nil, fmt.Errorf("Tushare daily_basic only supports common A-share equities, got %q", raw)
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
	}, "ts_code,trade_date,pe,pe_ttm,pb,ps_ttm,turnover_rate,turnover_rate_f,volume_ratio,dv_ratio,dv_ttm")
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
	snapshotReasons := []string{"provider_exact_snapshot_time_not_exposed", "provider_pcf_ttm_not_supported_by_tushare_daily_basic", "provider_pe_mrq_not_supported_by_tushare_daily_basic"}
	for index, row := range table.rows {
		rawSymbol, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		if isTushareConvertibleBondCode(rawSymbol) {
			base.ExcludedNonAShareCount++
			base.QualityReasons = appendTushareReason(base.QualityReasons, "provider_non_a_share_security_rows_excluded")
			continue
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
		peStatic, readErr := table.optionalFloat(row, "pe")
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
		turnoverRate, readErr := table.optionalFloat(row, "turnover_rate")
		if readErr != nil {
			return nil, readErr
		}
		floatTurnoverRate, readErr := table.optionalFloat(row, "turnover_rate_f")
		if readErr != nil {
			return nil, readErr
		}
		volumeRatio, readErr := table.optionalFloat(row, "volume_ratio")
		if readErr != nil {
			return nil, readErr
		}
		dividendYield, readErr := table.optionalFloat(row, "dv_ratio")
		if readErr != nil {
			return nil, readErr
		}
		dividendYieldTTM, readErr := table.optionalFloat(row, "dv_ttm")
		if readErr != nil {
			return nil, readErr
		}
		itemReasons := []string{"provider_exact_snapshot_time_not_exposed", "provider_pcf_ttm_not_supported_by_tushare_daily_basic", "provider_pe_mrq_not_supported_by_tushare_daily_basic"}
		if peStatic == nil || peTTM == nil || pb == nil || psTTM == nil ||
			turnoverRate == nil || floatTurnoverRate == nil || volumeRatio == nil ||
			dividendYield == nil || dividendYieldTTM == nil {
			itemReasons = appendTushareReason(itemReasons, "one_or_more_daily_basic_metrics_missing")
		}
		bySymbol[symbol] = values{item: model.ValuationItem{
			Symbol: symbol, Code: strings.Split(symbol, ".")[0],
			PETTM: peTTM, PEMRQ: nil, PEStatic: peStatic, PBMRQ: pb, PSTTM: psTTM, PCFTTM: nil,
			TurnoverRatePercent: turnoverRate, FloatTurnoverRatePercent: floatTurnoverRate,
			VolumeRatio: volumeRatio, DividendYieldPercent: dividendYield, DividendYieldTTMPercent: dividendYieldTTM,
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

// isTushareAShareSymbol excludes B shares, funds and non-equity instruments from A-share event feeds.
func isTushareAShareSymbol(symbol string) bool {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 || len(parts[0]) != 6 {
		return false
	}
	code := parts[0]
	switch parts[1] {
	case "SH":
		for _, prefix := range []string{"600", "601", "603", "605", "688", "689"} {
			if strings.HasPrefix(code, prefix) {
				return true
			}
		}
	case "SZ":
		for _, prefix := range []string{"000", "001", "002", "003", "300", "301"} {
			if strings.HasPrefix(code, prefix) {
				return true
			}
		}
	case "BJ":
		return strings.HasPrefix(code, "4") || strings.HasPrefix(code, "8") || strings.HasPrefix(code, "92")
	}
	return false
}

func tushareOptionalInt(table *tushareTable, row []json.RawMessage, field string) (*int, error) {
	value, err := table.optionalFloat(row, field)
	if err != nil || value == nil {
		return nil, err
	}
	if math.Trunc(*value) != *value || *value < 0 || *value > 10000000 {
		return nil, fmt.Errorf("Tushare 字段 %s 不是有效非负整数", field)
	}
	result := int(*value)
	return &result, nil
}

// FetchLimitPool supplements Eastmoney/THS limit-up, limit-down and broken-board pools with Tushare limit_list_d.
func (p *Tushare) FetchLimitPool(ctx context.Context, rawPoolType, rawTradeDate string, limit int) (*model.LimitPoolSnapshot, error) {
	poolType := strings.ToLower(strings.TrimSpace(rawPoolType))
	if poolType == "" {
		poolType = "up"
	}
	limitType := ""
	switch poolType {
	case "up":
		limitType = "U"
	case "down":
		limitType = "D"
	case "broken":
		limitType = "Z"
	default:
		return nil, fmt.Errorf("Tushare 不支持涨跌停池类型 %q；可选 up/down/broken", rawPoolType)
	}
	if limit <= 0 {
		// 与 HTTP/Service 层默认值保持一致，避免直接调用 Provider 时悄悄只返回 30 条。
		limit = 100
	}
	if limit > 100 {
		return nil, errors.New("Tushare 涨跌停池单页条数不能超过 100")
	}
	tradeDate := strings.TrimSpace(rawTradeDate)
	if tradeDate == "" {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.FixedZone("CST", 8*60*60)
		}
		tradeDate = time.Now().In(loc).Format("2006-01-02")
	}
	compactDate, err := tushareCompactDate(tradeDate)
	if err != nil {
		return nil, err
	}
	parsedDate, _ := time.Parse("20060102", compactDate)
	tradeDate = parsedDate.Format("2006-01-02")
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if tradeDate > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("Tushare 涨跌停池交易日期不能晚于中国市场当前日期")
	}
	table, err := p.fetchCapabilityTable(ctx, "limit_list_d", map[string]string{
		"trade_date": compactDate, "limit_type": limitType,
	}, "trade_date,ts_code,industry,name,close,pct_chg,amount,limit_amount,float_mv,total_mv,turnover_ratio,fd_amount,first_time,last_time,open_times,up_stat,limit_times,limit")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	allItems := make([]model.LimitPoolItem, 0, len(table.rows))
	reasons := []string{"provider_amount_units_not_normalized", "provider_exact_snapshot_time_not_exposed"}
	totalIsLowerBound := len(table.rows) >= 2500
	if totalIsLowerBound {
		reasons = appendTushareReason(reasons, "provider_response_may_be_capped_at_2500_rows")
		reasons = appendTushareReason(reasons, "provider_limit_pool_total_is_lower_bound")
	}
	excludedNonAShareCount := 0
	seenSymbols := make(map[string]struct{}, len(table.rows))
	for index, row := range table.rows {
		rowDateRaw, readErr := table.stringValue(row, "trade_date")
		if readErr != nil {
			return nil, readErr
		}
		rowDate, dateErr := tushareISODate(rowDateRaw)
		if dateErr != nil || rowDate != tradeDate {
			return nil, fmt.Errorf("Tushare 涨跌停池日期不匹配: requested=%s returned=%q", tradeDate, rowDateRaw)
		}
		rowLimitType, readErr := table.stringValue(row, "limit")
		if readErr != nil || rowLimitType != limitType {
			return nil, fmt.Errorf("Tushare 涨跌停池类型不匹配: requested=%s returned=%q", limitType, rowLimitType)
		}
		rawSymbol, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		symbol, symbolErr := normalize.NormalizeSymbol(rawSymbol)
		if symbolErr != nil {
			return nil, fmt.Errorf("Tushare 涨跌停池第 %d 行证券代码无效: %q", index, rawSymbol)
		}
		if !isTushareAShareSymbol(symbol) {
			excludedNonAShareCount++
			reasons = appendTushareReason(reasons, "provider_non_a_share_security_rows_excluded")
			continue
		}
		if _, duplicate := seenSymbols[symbol]; duplicate {
			return nil, fmt.Errorf("Tushare 涨跌停池第 %d 行重复证券 %s", index, symbol)
		}
		seenSymbols[symbol] = struct{}{}
		name, readErr := table.stringValue(row, "name")
		if readErr != nil || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("Tushare 涨跌停池第 %d 行缺少证券名称", index)
		}
		change, readErr := table.optionalFloat(row, "pct_chg")
		if readErr != nil || change == nil {
			return nil, fmt.Errorf("Tushare 涨跌停池第 %d 行缺少有效涨跌幅", index)
		}
		closePrice, readErr := table.optionalFloat(row, "close")
		if readErr != nil {
			return nil, readErr
		}
		if closePrice != nil && *closePrice <= 0 {
			closePrice = nil
		}
		industry, readErr := table.stringValue(row, "industry")
		if readErr != nil {
			return nil, readErr
		}
		firstTime, readErr := table.stringValue(row, "first_time")
		if readErr != nil {
			return nil, readErr
		}
		lastTime, readErr := table.stringValue(row, "last_time")
		if readErr != nil {
			return nil, readErr
		}
		amount, readErr := table.optionalFloat(row, "amount")
		if readErr != nil {
			return nil, readErr
		}
		sealMoney, readErr := table.optionalFloat(row, "fd_amount")
		if readErr != nil {
			return nil, readErr
		}
		turnoverRatio, readErr := table.optionalFloat(row, "turnover_ratio")
		if readErr != nil {
			return nil, readErr
		}
		openTimes, readErr := tushareOptionalInt(table, row, "open_times")
		if readErr != nil {
			return nil, readErr
		}
		var consecutive *int
		if poolType == "up" {
			consecutive, readErr = tushareOptionalInt(table, row, "limit_times")
			if readErr != nil {
				return nil, readErr
			}
		}
		itemReasons := []string{"provider_amount_units_not_normalized", "provider_exact_snapshot_time_not_exposed"}
		if poolType == "up" && firstTime == "" {
			itemReasons = appendTushareReason(itemReasons, "provider_first_limit_time_missing")
		}
		if poolType != "broken" && turnoverRatio == nil {
			itemReasons = appendTushareReason(itemReasons, "provider_turnover_ratio_missing")
		}
		allItems = append(allItems, model.LimitPoolItem{
			Symbol: symbol, Code: strings.Split(symbol, ".")[0], Name: name,
			LastPrice: closePrice, ChangePercent: change,
			FirstTouchTime: firstTime, LastTouchTime: lastTime,
			ConsecutiveBoards: consecutive, OpenTimes: openTimes, TurnoverRatioPercent: turnoverRatio,
			Turnover: amount, SealMoney: sealMoney, ProviderAmountUnit: "provider_defined", Industry: industry,
			Source: p.Name(), SourceVersion: "tushare-limit-list-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: itemReasons,
		})
	}
	providerReportDate := ""
	if len(table.rows) > 0 {
		// 原始结果行已逐条核对 trade_date；只有确实收到该日期的源记录时才填报表日期。
		providerReportDate = tradeDate
	}
	if len(allItems) == 0 {
		return &model.LimitPoolSnapshot{
			PoolType: poolType, TradeDate: tradeDate, ProviderReportDate: providerReportDate,
			Total: 0, ProviderReturned: len(table.rows), ExcludedNonAShareCount: excludedNonAShareCount,
			TotalIsLowerBound: totalIsLowerBound, HasData: false, Items: []model.LimitPoolItem{},
			Source: p.Name(), SourceVersion: "tushare-limit-list-v1", FetchedAt: now,
			QualityStatus:  model.QualityUnknown,
			QualityReasons: append(reasons, "empty_limit_pool_cannot_confirm_zero_events_or_unavailable_source"),
		}, nil
	}
	total := len(allItems)
	if len(allItems) > limit {
		allItems = allItems[:limit]
	}
	return &model.LimitPoolSnapshot{
		PoolType: poolType, TradeDate: tradeDate, ProviderReportDate: providerReportDate,
		Total: total, ProviderReturned: len(table.rows), ExcludedNonAShareCount: excludedNonAShareCount,
		TotalIsLowerBound: totalIsLowerBound, HasData: true, Items: allItems,
		Source: p.Name(), SourceVersion: "tushare-limit-list-v1", FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: reasons,
	}, nil
}

func isTushareConvertibleBondCode(raw string) bool {
	code := strings.ToUpper(strings.TrimSpace(raw))
	parts := strings.Split(code, ".")
	if len(parts) != 2 || len(parts[0]) != 6 {
		return false
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return false
		}
	}
	switch {
	case strings.HasPrefix(code, "110"), strings.HasPrefix(code, "111"),
		strings.HasPrefix(code, "113"), strings.HasPrefix(code, "118"),
		strings.HasPrefix(code, "123"), strings.HasPrefix(code, "127"),
		strings.HasPrefix(code, "128"), strings.HasPrefix(code, "129"):
		return true
	default:
		return false
	}
}

// FetchDragonTiger returns one page of top_list rows and keeps unverified currency fields out of *_CNY values.
func (p *Tushare) FetchDragonTiger(ctx context.Context, rawTradeDate string, page, pageSize int) (*model.DragonTigerSnapshot, error) {
	tradeDate := strings.TrimSpace(rawTradeDate)
	if tradeDate == "" {
		loc, err := time.LoadLocation("Asia/Shanghai")
		if err != nil {
			loc = time.FixedZone("CST", 8*60*60)
		}
		tradeDate = time.Now().In(loc).Format("2006-01-02")
	}
	compactDate, err := tushareCompactDate(tradeDate)
	if err != nil {
		return nil, err
	}
	parsedDate, _ := time.Parse("20060102", compactDate)
	tradeDate = parsedDate.Format("2006-01-02")
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if tradeDate > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("Tushare 龙虎榜交易日期不能晚于中国市场当前日期")
	}
	if page < 1 || page > 1000 {
		return nil, errors.New("Tushare 龙虎榜 page 必须在 1..1000")
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, errors.New("Tushare 龙虎榜 page_size 必须在 1..100")
	}
	table, err := p.fetchCapabilityTable(ctx, "top_list", map[string]string{"trade_date": compactDate},
		"trade_date,ts_code,name,close,pct_change,turnover_rate,amount,l_sell,l_buy,l_amount,net_amount,net_rate,amount_rate,reason")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	total := len(table.rows)
	totalIsLowerBound := total >= 10000
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	base := &model.DragonTigerSnapshot{
		TradeDate: tradeDate, ProviderReportDate: "", Page: page, PageSize: pageSize,
		Total: total, TotalIsLowerBound: totalIsLowerBound, Pages: pages,
		SourceReturned: 0, ExcludedNonAShareCount: 0,
		HasData: false, HasMore: false, Items: []model.DragonTigerItem{},
		Source: p.Name(), SourceVersion: "tushare-top-list-v1", FetchedAt: now,
		QualityStatus:  model.QualityUnknown,
		QualityReasons: []string{"provider_amount_units_not_normalized", "provider_exact_snapshot_time_not_exposed"},
	}
	if totalIsLowerBound {
		base.QualityReasons = appendTushareReason(base.QualityReasons, "provider_response_may_be_capped_at_10000_rows")
		base.QualityReasons = appendTushareReason(base.QualityReasons, "provider_dragon_tiger_total_is_lower_bound")
	}
	if total == 0 {
		base.QualityReasons = appendTushareReason(base.QualityReasons, "empty_dragon_tiger_report_cannot_confirm_zero_events_or_unavailable_source")
		return base, nil
	}
	if page > pages {
		return nil, fmt.Errorf("Tushare 龙虎榜请求页 %d 超出总页数 %d", page, pages)
	}
	start := (page - 1) * pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	rows := table.rows[start:end]
	base.SourceReturned = len(rows)
	base.HasMore = page < pages
	base.Items = make([]model.DragonTigerItem, 0, len(rows))
	seenSymbols := make(map[string]struct{}, len(rows))
	for index, row := range rows {
		rowDateRaw, readErr := table.stringValue(row, "trade_date")
		if readErr != nil {
			return nil, readErr
		}
		rowDate, dateErr := tushareISODate(rowDateRaw)
		if dateErr != nil || rowDate != tradeDate {
			return nil, fmt.Errorf("Tushare 龙虎榜日期不匹配: requested=%s returned=%q", tradeDate, rowDateRaw)
		}
		rawSymbol, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		symbol, symbolErr := normalize.NormalizeSymbol(rawSymbol)
		if symbolErr != nil {
			return nil, fmt.Errorf("Tushare 龙虎榜第 %d 行证券代码无效: %q", start+index, rawSymbol)
		}
		if !isTushareAShareSymbol(symbol) {
			base.ExcludedNonAShareCount++
			base.QualityReasons = appendTushareReason(base.QualityReasons, "provider_non_a_share_security_rows_excluded")
			continue
		}
		if _, duplicate := seenSymbols[symbol]; duplicate {
			return nil, fmt.Errorf("Tushare 龙虎榜第 %d 行重复证券 %s", start+index, symbol)
		}
		seenSymbols[symbol] = struct{}{}
		name, readErr := table.stringValue(row, "name")
		if readErr != nil || strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("Tushare 龙虎榜第 %d 行缺少证券名称", start+index)
		}
		closePrice, readErr := table.optionalFloat(row, "close")
		if readErr != nil {
			return nil, readErr
		}
		if closePrice != nil && *closePrice <= 0 {
			closePrice = nil
		}
		change, readErr := table.optionalFloat(row, "pct_change")
		if readErr != nil {
			return nil, readErr
		}
		buy, readErr := table.optionalFloat(row, "l_buy")
		if readErr != nil {
			return nil, readErr
		}
		sell, readErr := table.optionalFloat(row, "l_sell")
		if readErr != nil {
			return nil, readErr
		}
		deal, readErr := table.optionalFloat(row, "l_amount")
		if readErr != nil {
			return nil, readErr
		}
		net, readErr := table.optionalFloat(row, "net_amount")
		if readErr != nil {
			return nil, readErr
		}
		netRate, readErr := table.optionalFloat(row, "net_rate")
		if readErr != nil {
			return nil, readErr
		}
		amountRate, readErr := table.optionalFloat(row, "amount_rate")
		if readErr != nil {
			return nil, readErr
		}
		reason, readErr := table.stringValue(row, "reason")
		if readErr != nil {
			return nil, readErr
		}
		itemReasons := []string{"provider_amount_units_not_normalized", "provider_exact_snapshot_time_not_exposed"}
		base.Items = append(base.Items, model.DragonTigerItem{
			Symbol: symbol, Code: strings.Split(symbol, ".")[0], Name: name, TradeDate: tradeDate,
			ChangePercent: change, ClosePrice: closePrice,
			ProviderBuyValue: buy, ProviderSellValue: sell, ProviderDealValue: deal, ProviderNetValue: net,
			ProviderNetRate: netRate, ProviderAmountUnit: "provider_defined",
			DealAmountRatio: amountRate, Reason: strings.TrimSpace(reason),
			Source: p.Name(), SourceVersion: "tushare-top-list-v1", FetchedAt: now,
			QualityStatus: model.QualityDegraded, QualityReasons: itemReasons,
		})
	}
	// 只有实际收到且逐条核对过交易日的源记录，才能证明 ProviderReportDate。
	base.ProviderReportDate = tradeDate
	base.Returned = len(base.Items)
	if base.Returned == 0 {
		base.QualityStatus = model.QualityUnknown
		return base, nil
	}
	base.HasData = true
	base.QualityStatus = model.QualityDegraded
	return base, nil
}

func parseTusharePublishedTime(raw string) *time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02 15:04:05", "2006/01/02 15:04:05", "2006-01-02 15:04"} {
		var parsed time.Time
		if layout == time.RFC3339Nano {
			parsed, err = time.Parse(layout, value)
		} else {
			parsed, err = time.ParseInLocation(layout, value, loc)
		}
		if err == nil {
			parsed = parsed.UTC()
			return &parsed
		}
	}
	return nil
}

// FetchStockAnnouncements uses the separately permissioned anns_d endpoint and locally paginates its bounded result set.
func (p *Tushare) FetchStockAnnouncements(ctx context.Context, rawSymbol, rawStartDate, rawEndDate string, page, pageSize int) (*model.StockAnnouncementSnapshot, error) {
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tushare anns_d 仅支持普通 A 股股票，证券 %s 不在支持范围内", symbol)
	}
	startDate, endDate := strings.TrimSpace(rawStartDate), strings.TrimSpace(rawEndDate)
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
	startDate, endDate = start.Format("2006-01-02"), end.Format("2006-01-02")
	if start.After(end) || end.Sub(start) > 365*24*time.Hour {
		return nil, errors.New("Tushare 公告日期范围无效或超过 366 天")
	}
	if page < 1 || page > 1000 {
		return nil, errors.New("Tushare 公告 page 必须在 1..1000")
	}
	if pageSize < 1 || pageSize > 100 {
		return nil, errors.New("Tushare 公告 page_size 必须在 1..100")
	}
	table, err := p.fetchCapabilityTable(ctx, "anns_d", map[string]string{
		"ts_code": symbol, "start_date": startCompact, "end_date": endCompact,
	}, "ann_date,ts_code,name,title,url,rec_time")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	allItems := make([]model.StockAnnouncement, 0, len(table.rows))
	seen := make(map[string]struct{}, len(table.rows))
	snapshotReasons := []string{}
	for index, row := range table.rows {
		rawCode, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		rowSymbol, symbolErr := normalize.NormalizeSymbol(rawCode)
		if symbolErr != nil || rowSymbol != symbol {
			return nil, fmt.Errorf("Tushare 公告第 %d 行股票代码不匹配: %q", index, rawCode)
		}
		dateRaw, readErr := table.stringValue(row, "ann_date")
		if readErr != nil {
			return nil, readErr
		}
		annDate, dateErr := tushareISODate(dateRaw)
		if dateErr != nil {
			return nil, fmt.Errorf("Tushare 公告第 %d 行日期无效: %w", index, dateErr)
		}
		if annDate < startDate || annDate > endDate {
			return nil, fmt.Errorf("Tushare 公告返回范围外日期 %s", annDate)
		}
		title, readErr := table.stringValue(row, "title")
		if readErr != nil {
			return nil, readErr
		}
		if strings.TrimSpace(title) == "" {
			return nil, fmt.Errorf("Tushare 公告第 %d 行缺少标题", index)
		}
		sourceURL, readErr := table.stringValue(row, "url")
		if readErr != nil {
			return nil, readErr
		}
		parsedURL, urlErr := url.Parse(strings.TrimSpace(sourceURL))
		if urlErr != nil || (parsedURL.Scheme != "https" && parsedURL.Scheme != "http") || parsedURL.Host == "" {
			return nil, fmt.Errorf("Tushare 公告第 %d 行来源 URL 无效", index)
		}
		stockName, readErr := table.stringValue(row, "name")
		if readErr != nil {
			return nil, readErr
		}
		displayTime, readErr := table.stringValue(row, "rec_time")
		if readErr != nil {
			return nil, readErr
		}
		publishedAt := parseTusharePublishedTime(displayTime)
		hashInput := symbol + "|" + annDate + "|" + strings.TrimSpace(title) + "|" + strings.TrimSpace(sourceURL)
		hash := sha256.Sum256([]byte(hashInput))
		articleID := "tushare-" + hex.EncodeToString(hash[:12])
		if _, duplicate := seen[articleID]; duplicate {
			snapshotReasons = appendTushareReason(snapshotReasons, "provider_duplicate_announcement_rows_deduplicated")
			continue
		}
		seen[articleID] = struct{}{}
		itemQuality := model.QualityValid
		itemReasons := []string{}
		if publishedAt == nil {
			itemQuality = model.QualityDegraded
			itemReasons = append(itemReasons, "provider_publish_time_missing_or_unparseable")
			snapshotReasons = appendTushareReason(snapshotReasons, "provider_publish_time_missing_or_unparseable")
		}
		allItems = append(allItems, model.StockAnnouncement{
			ArticleID: articleID, Symbol: symbol, StockName: stockName, Title: strings.TrimSpace(title),
			AnnouncementDate: annDate, PublishedAt: publishedAt, DisplayTime: displayTime,
			SourceURL: strings.TrimSpace(sourceURL), Source: p.Name(), SourceVersion: "tushare-anns-d-v1",
			FetchedAt: now, QualityStatus: itemQuality, QualityReasons: itemReasons,
		})
	}
	if len(table.rows) >= 2000 {
		snapshotReasons = appendTushareReason(snapshotReasons, "provider_response_may_be_capped_at_2000_rows")
	}
	sort.SliceStable(allItems, func(i, j int) bool {
		if allItems[i].AnnouncementDate != allItems[j].AnnouncementDate {
			return allItems[i].AnnouncementDate > allItems[j].AnnouncementDate
		}
		if allItems[i].PublishedAt != nil && allItems[j].PublishedAt != nil {
			return allItems[i].PublishedAt.After(*allItems[j].PublishedAt)
		}
		return false
	})
	total := len(allItems)
	pages := 0
	if total > 0 {
		pages = (total + pageSize - 1) / pageSize
	}
	if total == 0 {
		return &model.StockAnnouncementSnapshot{
			Symbol: symbol, StartDate: startDate, EndDate: endDate, Page: page, PageSize: pageSize,
			Total: 0, Pages: 0, Returned: 0, HasData: false, HasMore: false,
			Items: []model.StockAnnouncement{}, Source: p.Name(), SourceVersion: "tushare-anns-d-v1",
			FetchedAt: now, QualityStatus: model.QualityUnknown,
			QualityReasons: appendTushareReason(snapshotReasons, "empty_announcement_query_cannot_confirm_no_announcements_or_unavailable_source"),
		}, nil
	}
	if page > pages {
		return nil, fmt.Errorf("Tushare 公告请求页 %d 超出总页数 %d", page, pages)
	}
	startIndex := (page - 1) * pageSize
	endIndex := startIndex + pageSize
	if endIndex > total {
		endIndex = total
	}
	items := allItems[startIndex:endIndex]
	quality := model.QualityValid
	if len(snapshotReasons) > 0 {
		quality = model.QualityDegraded
	}
	for _, item := range items {
		if item.QualityStatus != model.QualityValid {
			quality = model.QualityDegraded
		}
	}
	return &model.StockAnnouncementSnapshot{
		Symbol: symbol, StartDate: startDate, EndDate: endDate, Page: page, PageSize: pageSize,
		Total: total, Pages: pages, Returned: len(items), HasData: true, HasMore: page < pages,
		Items: items, Source: p.Name(), SourceVersion: "tushare-anns-d-v1",
		FetchedAt: now, QualityStatus: quality, QualityReasons: snapshotReasons,
	}, nil
}

const tushareIndexCatalogVersion = "tushare-index-basic-v1"
const tushareIndexConstituentVersion = "tushare-index-weight-v1"
const tushareIndexDailyVersion = "tushare-index-daily-v1"

func canonicalTushareIndexSymbol(raw string) (string, error) {
	value := strings.ToUpper(strings.TrimSpace(raw))
	parts := strings.Split(value, ".")
	if len(parts) != 2 || (parts[1] != "SH" && parts[1] != "SZ" && parts[1] != "CSI") || len(parts[0]) != 6 {
		return "", fmt.Errorf("Tushare 指数代码必须为六位代码加 .SH/.SZ/.CSI 后缀: %q", raw)
	}
	for _, r := range parts[0] {
		if r < '0' || r > '9' {
			return "", fmt.Errorf("Tushare 指数代码必须是六位数字: %q", raw)
		}
	}
	return value, nil
}

// FetchIndexCatalog reads Tushare's official index directory using a distinct tag.
func (p *Tushare) FetchIndexCatalog(ctx context.Context, rawTag string) (*model.IndexCatalogSnapshot, error) {
	tag := strings.ToLower(strings.TrimSpace(rawTag))
	if tag == "" {
		tag = "cn_concept"
	}
	if tag != "tushare" {
		return nil, fmt.Errorf("Tushare 指数目录仅支持 tag=tushare，收到 %q", rawTag)
	}
	table, err := p.fetchCapabilityTable(ctx, "index_basic", map[string]string{},
		"ts_code,name,market,publisher,category,base_date,list_date,exp_date")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	items := make([]model.IndexCatalogItem, 0, len(table.rows))
	seen := make(map[string]struct{}, len(table.rows))
	reasons := []string{"provider_index_catalog_has_no_precise_snapshot_timestamp"}
	for index, row := range table.rows {
		rawSymbol, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		symbol, symbolErr := canonicalTushareIndexSymbol(rawSymbol)
		if symbolErr != nil {
			reasons = appendTushareReason(reasons, "provider_index_catalog_contains_unmapped_codes")
			continue
		}
		name, readErr := table.stringValue(row, "name")
		if readErr != nil {
			return nil, readErr
		}
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("Tushare 指数目录第 %d 行缺少名称", index)
		}
		if _, duplicate := seen[symbol]; duplicate {
			return nil, fmt.Errorf("Tushare 指数目录返回重复代码 %s", symbol)
		}
		seen[symbol] = struct{}{}
		items = append(items, model.IndexCatalogItem{Symbol: symbol, Name: name, Tag: tag})
	}
	if len(table.rows) >= 5000 {
		reasons = appendTushareReason(reasons, "provider_index_catalog_may_be_truncated_by_upstream_result_limit")
	}
	if len(items) == 0 {
		return &model.IndexCatalogSnapshot{
			Tag: tag, Items: []model.IndexCatalogItem{}, Total: 0, Source: p.Name(),
			SourceVersion: tushareIndexCatalogVersion, FetchedAt: now, QualityStatus: model.QualityUnknown,
			QualityReasons: appendTushareReason(reasons, "empty_index_catalog_cannot_confirm_no_indexes_or_unavailable_source"),
		}, nil
	}
	return &model.IndexCatalogSnapshot{
		Tag: tag, Items: items, Total: len(items), Source: p.Name(),
		SourceVersion: tushareIndexCatalogVersion, FetchedAt: now, QualityStatus: model.QualityDegraded, QualityReasons: reasons,
	}, nil
}

type tushareWeightedConstituent struct {
	symbol   string
	weight   *float64
	excluded bool
}

// FetchIndexConstituents returns the most recent monthly weight snapshot available in four months.
func (p *Tushare) FetchIndexConstituents(ctx context.Context, rawIndexSymbol string) (*model.IndexConstituentSnapshot, error) {
	return p.fetchIndexConstituents(ctx, rawIndexSymbol, "")
}

// FetchIndexConstituentsAsOf returns the latest Tushare index-weight report on or before asOfDate.
// It does not claim an effective-date interval for membership changes.
func (p *Tushare) FetchIndexConstituentsAsOf(ctx context.Context, rawIndexSymbol, asOfDate string) (*model.IndexConstituentSnapshot, error) {
	return p.fetchIndexConstituents(ctx, rawIndexSymbol, asOfDate)
}

func (p *Tushare) fetchIndexConstituents(ctx context.Context, rawIndexSymbol, rawAsOfDate string) (*model.IndexConstituentSnapshot, error) {
	indexSymbol, err := canonicalTushareIndexSymbol(rawIndexSymbol)
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	nowLocal := time.Now().In(loc)
	today := nowLocal.Format("2006-01-02")
	requestedAsOfDate := strings.TrimSpace(rawAsOfDate)
	anchorDate := nowLocal
	lookbackMonths := 4
	reasons := []string{"provider_constituent_date_is_latest_available_weight_snapshot_not_intraday"}
	var asOfTime time.Time
	if requestedAsOfDate != "" {
		var parseErr error
		asOfTime, parseErr = time.ParseInLocation("2006-01-02", requestedAsOfDate, loc)
		if parseErr != nil || asOfTime.Format("2006-01-02") != requestedAsOfDate {
			return nil, fmt.Errorf("Tushare 指数成分 as_of 日期必须使用 YYYY-MM-DD")
		}
		if requestedAsOfDate > today {
			return nil, fmt.Errorf("Tushare 指数成分 as_of 日期不能晚于当前日期")
		}
		anchorDate = asOfTime
		lookbackMonths = 120
		reasons = []string{"provider_as_of_uses_latest_index_weight_report_not_effective_membership_interval"}
	}
	monthStart := time.Date(anchorDate.Year(), anchorDate.Month(), 1, 0, 0, 0, 0, loc)

	// index_weight is monthly data. Query one month at a time from newest to
	// oldest; a single multi-month request can hit upstream result limits and
	// silently omit members from the newest snapshot. Historical lookups keep
	// each month isolated and never accept a report after the requested date.
	latestDate := ""
	latest := make([]tushareWeightedConstituent, 0)
	foundMonth := false
	monthlyResultMayBeTruncated := false
	for monthOffset := 0; monthOffset < lookbackMonths; monthOffset++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		windowStart := monthStart.AddDate(0, -monthOffset, 0)
		windowEnd := windowStart.AddDate(0, 1, -1)
		if requestedAsOfDate != "" && monthOffset == 0 && asOfTime.Before(windowEnd) {
			windowEnd = asOfTime
		}
		startCompact := windowStart.Format("20060102")
		endCompact := windowEnd.Format("20060102")
		table, fetchErr := p.fetchCapabilityTable(ctx, "index_weight", map[string]string{
			"index_code": indexSymbol, "start_date": startCompact, "end_date": endCompact,
		}, "index_code,con_code,trade_date,weight")
		if fetchErr != nil {
			return nil, fetchErr
		}
		if len(table.rows) >= 5000 {
			monthlyResultMayBeTruncated = true
			reasons = appendTushareReason(reasons, "provider_monthly_index_weight_result_is_large_and_may_be_truncated")
		}

		monthLatestDate := ""
		monthLatest := make([]tushareWeightedConstituent, 0, len(table.rows))
		for index, row := range table.rows {
			codeRaw, readErr := table.stringValue(row, "index_code")
			if readErr != nil {
				return nil, readErr
			}
			code, symbolErr := canonicalTushareIndexSymbol(codeRaw)
			if symbolErr != nil || code != indexSymbol {
				return nil, fmt.Errorf("Tushare 指数成分第 %d 行指数代码不匹配", index)
			}
			dateRaw, readErr := table.stringValue(row, "trade_date")
			if readErr != nil {
				return nil, readErr
			}
			date, dateErr := tushareISODate(dateRaw)
			if dateErr != nil || date < windowStart.Format("2006-01-02") || date > windowEnd.Format("2006-01-02") || date > today {
				return nil, fmt.Errorf("Tushare 指数成分第 %d 行日期无效或超出查询月份", index)
			}
			constituentRaw, readErr := table.stringValue(row, "con_code")
			if readErr != nil {
				return nil, readErr
			}
			constituent, symbolErr := normalize.NormalizeSymbol(constituentRaw)
			if symbolErr != nil || !isTushareAShareSymbol(constituent) {
				// 先按源端日期选择最新报告日，再记录不支持的证券；不能让过滤操作把报告日倒退。
				if date > monthLatestDate {
					monthLatestDate = date
					monthLatest = monthLatest[:0]
				}
				if date == monthLatestDate {
					monthLatest = append(monthLatest, tushareWeightedConstituent{excluded: true})
				}
				continue
			}
			weight, readErr := table.optionalFloat(row, "weight")
			if readErr != nil {
				return nil, readErr
			}
			if weight != nil && (*weight < 0 || math.IsNaN(*weight) || math.IsInf(*weight, 0)) {
				return nil, fmt.Errorf("Tushare 指数成分第 %d 行权重无效", index)
			}
			if date > monthLatestDate {
				monthLatestDate = date
				monthLatest = monthLatest[:0]
			}
			if date == monthLatestDate {
				monthLatest = append(monthLatest, tushareWeightedConstituent{symbol: constituent, weight: weight})
			}
		}
		if len(monthLatest) > 0 {
			latestDate = monthLatestDate
			latest = monthLatest
			foundMonth = true
			break
		}
	}

	now := time.Now().UTC()
	if !foundMonth || latestDate == "" || len(latest) == 0 {
		noSnapshotReason := "provider_has_no_constituent_snapshot_in_recent_four_months"
		if requestedAsOfDate != "" {
			noSnapshotReason = "provider_has_no_index_weight_snapshot_on_or_before_requested_as_of_within_120_months"
		}
		return &model.IndexConstituentSnapshot{
			IndexSymbol: indexSymbol, AsOfDate: requestedAsOfDate, Items: []model.IndexConstituent{}, Total: 0, Source: p.Name(),
			SourceVersion: tushareIndexConstituentVersion, FetchedAt: now, QualityStatus: model.QualityUnknown,
			QualityReasons: appendTushareReason(reasons, noSnapshotReason),
		}, nil
	}
	supportedCount := 0
	for _, entry := range latest {
		if !entry.excluded {
			supportedCount++
		}
	}
	if supportedCount == 0 {
		// 最新源报告日有记录，但没有受支持的 A 股成分；保留日期与覆盖计数，交由服务层继续回退。
		reasons = appendTushareReason(reasons, "provider_unsupported_constituent_rows_excluded")
		reasons = appendTushareReason(reasons, "provider_latest_constituent_snapshot_has_no_supported_a_share_members")
		return &model.IndexConstituentSnapshot{
			IndexSymbol: indexSymbol, AsOfDate: requestedAsOfDate, ProviderReportDate: latestDate, Items: []model.IndexConstituent{}, Total: 0,
			ProviderReturned: len(latest), ExcludedCount: len(latest),
			TotalIsLowerBound: monthlyResultMayBeTruncated, Source: p.Name(),
			SourceVersion: tushareIndexConstituentVersion, FetchedAt: now, QualityStatus: model.QualityUnknown,
			QualityReasons: reasons,
		}, nil
	}
	// index_weight gives constituent codes but not names; enrich in one stock_basic call.
	basicParams := map[string]string{"list_status": "L"}
	if requestedAsOfDate != "" {
		// 历史成分可能包含已退市证券；省略 list_status 以查询完整证券目录。
		basicParams = map[string]string{}
	}
	basic, err := p.fetchCapabilityTable(ctx, "stock_basic", basicParams, "ts_code,name")
	if err != nil {
		return nil, fmt.Errorf("Tushare 指数成分名称补充失败: %w", err)
	}
	names := make(map[string]string, len(basic.rows))
	for index, row := range basic.rows {
		rawSymbol, readErr := basic.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		symbol, symbolErr := normalize.NormalizeSymbol(rawSymbol)
		if symbolErr != nil {
			continue
		}
		name, readErr := basic.stringValue(row, "name")
		if readErr != nil {
			return nil, readErr
		}
		if strings.TrimSpace(name) != "" {
			if _, duplicate := names[symbol]; duplicate {
				return nil, fmt.Errorf("Tushare 股票目录第 %d 行出现重复代码 %s", index, symbol)
			}
			names[symbol] = name
		}
	}
	items := make([]model.IndexConstituent, 0, len(latest))
	seen := make(map[string]struct{}, len(latest))
	excludedCount := 0
	for _, entry := range latest {
		if entry.excluded {
			excludedCount++
			reasons = appendTushareReason(reasons, "provider_unsupported_constituent_rows_excluded")
			continue
		}
		if _, duplicate := seen[entry.symbol]; duplicate {
			return nil, fmt.Errorf("Tushare 指数成分 %s 在最新快照中重复", entry.symbol)
		}
		seen[entry.symbol] = struct{}{}
		name, ok := names[entry.symbol]
		if !ok {
			excludedCount++
			reasons = appendTushareReason(reasons, "one_or_more_constituent_names_missing_from_current_stock_directory")
			continue
		}
		items = append(items, model.IndexConstituent{Symbol: entry.symbol, Code: strings.Split(entry.symbol, ".")[0], Name: name, WeightPercent: entry.weight})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Symbol < items[j].Symbol })
	if len(items) == 0 {
		return nil, errors.New("Tushare 指数成分有代码但无法从股票目录解析任何名称")
	}
	return &model.IndexConstituentSnapshot{
		IndexSymbol: indexSymbol, AsOfDate: requestedAsOfDate, ProviderReportDate: latestDate, Items: items, Total: len(items),
		ProviderReturned: len(latest), ExcludedCount: excludedCount,
		TotalIsLowerBound: monthlyResultMayBeTruncated, Source: p.Name(),
		SourceVersion: tushareIndexConstituentVersion, FetchedAt: now,
		QualityStatus: model.QualityDegraded, QualityReasons: reasons,
	}, nil
}

// FetchIndexDailyBars 读取未复权指数 OHLC；成交量保留手口径，成交额按千元转成人民币元。
func (p *Tushare) FetchIndexDailyBars(ctx context.Context, rawIndexSymbol, rawStartDate, rawEndDate string) (*model.IndexDailyBarsSnapshot, error) {
	indexSymbol, err := canonicalTushareIndexSymbol(rawIndexSymbol)
	if err != nil {
		return nil, err
	}
	startCompact, err := tushareCompactDate(rawStartDate)
	if err != nil {
		return nil, err
	}
	endCompact, err := tushareCompactDate(rawEndDate)
	if err != nil {
		return nil, err
	}
	start, _ := time.Parse("20060102", startCompact)
	end, _ := time.Parse("20060102", endCompact)
	if start.After(end) || end.After(start.AddDate(10, 0, 0)) {
		return nil, errors.New("Tushare 指数日 K 日期范围无效或超过 10 年")
	}
	loc, locErr := time.LoadLocation("Asia/Shanghai")
	if locErr != nil {
		loc = time.FixedZone("CST", 8*60*60)
	}
	if end.Format("2006-01-02") > time.Now().In(loc).Format("2006-01-02") {
		return nil, errors.New("Tushare 指数日 K end_date 不能晚于中国市场当前日期")
	}
	table, err := p.fetchCapabilityTable(ctx, "index_daily", map[string]string{
		"ts_code": indexSymbol, "start_date": startCompact, "end_date": endCompact,
	}, "ts_code,trade_date,open,high,low,close,pct_chg,vol,amount")
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	items := make([]model.IndexDailyBar, 0, len(table.rows))
	seen := make(map[string]struct{}, len(table.rows))
	snapshotReasons := []string{}
	for index, row := range table.rows {
		rawCode, readErr := table.stringValue(row, "ts_code")
		if readErr != nil {
			return nil, readErr
		}
		code, symbolErr := canonicalTushareIndexSymbol(rawCode)
		if symbolErr != nil || code != indexSymbol {
			return nil, fmt.Errorf("Tushare 指数日 K 第 %d 行指数代码不匹配", index)
		}
		dateRaw, readErr := table.stringValue(row, "trade_date")
		if readErr != nil {
			return nil, readErr
		}
		date, dateErr := tushareISODate(dateRaw)
		if dateErr != nil || date < start.Format("2006-01-02") || date > end.Format("2006-01-02") {
			return nil, fmt.Errorf("Tushare 指数日 K 第 %d 行交易日无效或超出范围", index)
		}
		if _, duplicate := seen[date]; duplicate {
			return nil, fmt.Errorf("Tushare 指数日 K 存在重复交易日 %s", date)
		}
		seen[date] = struct{}{}
		requiredPrice := func(field string) (float64, error) {
			value, fieldErr := table.optionalFloat(row, field)
			if fieldErr != nil {
				return 0, fieldErr
			}
			if value == nil || *value <= 0 || math.IsNaN(*value) || math.IsInf(*value, 0) {
				return 0, fmt.Errorf("Tushare 指数日 K %s 缺少有效的 %s", date, field)
			}
			return *value, nil
		}
		openPrice, fieldErr := requiredPrice("open")
		if fieldErr != nil {
			return nil, fieldErr
		}
		highPrice, fieldErr := requiredPrice("high")
		if fieldErr != nil {
			return nil, fieldErr
		}
		lowPrice, fieldErr := requiredPrice("low")
		if fieldErr != nil {
			return nil, fieldErr
		}
		closePrice, fieldErr := requiredPrice("close")
		if fieldErr != nil {
			return nil, fieldErr
		}
		if lowPrice > highPrice || openPrice < lowPrice || openPrice > highPrice || closePrice < lowPrice || closePrice > highPrice {
			return nil, fmt.Errorf("Tushare 指数日 K %s 的 OHLC 不满足数据契约", date)
		}
		rowReasons := []string{"provider_snapshot_timestamp_not_exposed"}
		var volume *int64
		rawVolume, fieldErr := table.optionalFloat(row, "vol")
		if fieldErr != nil {
			return nil, fieldErr
		}
		if rawVolume == nil || *rawVolume < 0 || math.Trunc(*rawVolume) != *rawVolume || *rawVolume >= float64(math.MaxInt64) {
			rowReasons = appendTushareReason(rowReasons, "provider_index_volume_missing_or_invalid")
		} else {
			value := int64(*rawVolume)
			volume = &value
		}
		var amount *float64
		rawAmount, fieldErr := table.optionalFloat(row, "amount")
		if fieldErr != nil {
			return nil, fieldErr
		}
		if rawAmount == nil || *rawAmount < 0 || math.IsNaN(*rawAmount) || math.IsInf(*rawAmount, 0) || *rawAmount > math.MaxFloat64/1000 {
			rowReasons = appendTushareReason(rowReasons, "provider_index_amount_missing_or_invalid")
		} else {
			value := *rawAmount * 1000
			amount = &value
		}
		change, fieldErr := table.optionalFloat(row, "pct_chg")
		if fieldErr != nil {
			return nil, fieldErr
		}
		for _, reason := range rowReasons {
			snapshotReasons = appendTushareReason(snapshotReasons, reason)
		}
		barQuality := model.QualityValid
		if len(rowReasons) > 0 {
			barQuality = model.QualityDegraded
		}
		volumeUnit, amountUnit := "hands", "CNY"
		if volume == nil {
			volumeUnit = "unknown"
		}
		if amount == nil {
			amountUnit = "unknown"
		}
		items = append(items, model.IndexDailyBar{
			IndexSymbol: indexSymbol, TradeDate: date, Open: openPrice, High: highPrice, Low: lowPrice, Close: closePrice,
			ChangePercent: change, Volume: volume, Amount: amount, VolumeUnit: volumeUnit, AmountUnit: amountUnit,
			Source: p.Name(), SourceVersion: tushareIndexDailyVersion, FetchedAt: now,
			QualityStatus: barQuality, QualityReasons: rowReasons,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].TradeDate < items[j].TradeDate })
	if len(items) == 0 {
		return &model.IndexDailyBarsSnapshot{
			IndexSymbol: indexSymbol, StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
			Items: []model.IndexDailyBar{}, Total: 0, Source: p.Name(), SourceVersion: tushareIndexDailyVersion,
			FetchedAt: now, QualityStatus: model.QualityUnknown,
			QualityReasons: []string{"empty_index_daily_series_cannot_confirm_unavailable_or_non_trading_range"},
		}, nil
	}
	quality := model.QualityValid
	if len(snapshotReasons) > 0 || len(table.rows) >= 8000 {
		quality = model.QualityDegraded
	}
	if len(table.rows) >= 8000 {
		snapshotReasons = appendTushareReason(snapshotReasons, "provider_index_daily_response_may_be_capped_at_8000_rows")
	}
	return &model.IndexDailyBarsSnapshot{
		IndexSymbol: indexSymbol, StartDate: start.Format("2006-01-02"), EndDate: end.Format("2006-01-02"),
		Items: items, Total: len(items), Source: p.Name(), SourceVersion: tushareIndexDailyVersion,
		FetchedAt: now, QualityStatus: quality, QualityReasons: snapshotReasons,
	}, nil
}
