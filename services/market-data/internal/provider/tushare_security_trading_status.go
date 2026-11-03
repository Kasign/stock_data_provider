package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/model"
	"github.com/ZhuLinsen/daily_stock_analysis/services/market-data/internal/normalize"
)

const tushareSecurityTradingStatusVersion = "tushare-stock-st-namechange-suspend-d-trade-cal-v2"

// FetchSecurityTradingStatus reconstructs date-effective security status using source records.
// It intentionally does not substitute the current stock name for a missing historical name row.
func (p *Tushare) FetchSecurityTradingStatus(ctx context.Context, rawSymbol, rawTradeDate string) (*model.SecurityTradingStatusSnapshot, error) {
	symbol, err := tushareExchangeCode(rawSymbol)
	if err != nil {
		return nil, err
	}
	if !isTushareAShareSymbol(symbol) {
		return nil, fmt.Errorf("Tushare historical security status only supports common A-share equities, got %q", rawSymbol)
	}
	compactDate, err := tushareCompactDate(rawTradeDate)
	if err != nil {
		return nil, err
	}
	tradeDate, err := time.Parse("20060102", compactDate)
	if err != nil {
		return nil, err
	}
	shanghai, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		shanghai = time.FixedZone("CST", 8*60*60)
	}
	date := tradeDate.Format("2006-01-02")
	if date > time.Now().In(shanghai).Format("2006-01-02") {
		return nil, errors.New("Tushare security-status trade date must not be in the future")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	listingDate, err := p.fetchSecurityListingDate(ctx, symbol)
	if err != nil {
		return nil, fmt.Errorf("Tushare stock_basic for historical listing date: %w", err)
	}
	exchange, err := tushareCalendarExchange(symbol)
	if err != nil {
		return nil, err
	}
	// Tushare trade_cal 不接受 BSE 参数，官方文档说明北交所交易日历参考沪深交易所。
	// 这里仅借用相同的市场交易日历；上市交易日序号仍由下方 BSE 分支保守标记为未知。
	calendarExchange := exchange
	if exchange == "BSE" {
		calendarExchange = "SSE"
	}
	tradingDay, err := p.fetchSecurityTradingDay(ctx, calendarExchange, compactDate)
	if err != nil {
		return nil, fmt.Errorf("Tushare trade_cal for %s: %w", date, err)
	}
	name, nameErr := p.fetchEffectiveSecurityName(ctx, symbol, compactDate)
	suspension, suspensionTiming, suspensionType, suspensionReasons, suspensionErr := p.fetchSuspensionOnDate(ctx, symbol, compactDate, tradingDay)

	result := &model.SecurityTradingStatusSnapshot{
		Symbol:            symbol,
		TradeDate:         date,
		RiskWarningStatus: model.RiskWarningUnknown,
		DelistingStatus:   model.DelistingStatusUnknown,
		ListingDate:       listingDate,
		IsTradingDay:      tradingDay,
		TradingDayKnown:   true,
		SuspensionStatus:  suspension,
		SuspensionTiming:  suspensionTiming,
		SuspensionType:    suspensionType,
		Source:            p.Name(),
		SourceVersion:     tushareSecurityTradingStatusVersion,
		FetchedAt:         time.Now().UTC(),
		QualityStatus:     model.QualityDegraded,
		QualityReasons:    []string{"provider_exact_snapshot_time_not_exposed"},
	}

	historicalRiskStatus := model.RiskWarningUnknown
	historicalNameSuggestsDelisting := false
	if nameErr != nil {
		result.RiskWarningStatus = model.RiskWarningUnknown
		result.QualityStatus = model.QualityUnknown
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "historical_security_name_unavailable")
	} else {
		result.SecurityName = name.Name
		result.NameEffectiveFrom = name.StartDate
		result.NameEffectiveTo = name.EndDate
		historicalNameSuggestsDelisting = strings.Contains(name.Name, "退市") || strings.Contains(name.Name, "摘牌")
		historicalRiskStatus = riskWarningStatusFromHistoricalName(name.Name)
		result.DelistingStatus = delistingStatusFromHistoricalName(name.Name)
		if historicalRiskStatus == model.RiskWarningUnknown || result.DelistingStatus == model.DelistingStatusUnknown {
			result.QualityStatus = model.QualityUnknown
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "historical_security_name_does_not_prove_risk_warning_or_delisting_status")
		}
	}

	// stock_st 是按交易日提供的风险警示板名单；名称前缀只作为一致性校验，不作为权威状态回退。
	authoritativeRiskStatus := model.RiskWarningUnknown
	var riskStatusErr error
	if !tradingDay {
		// stock_st 是交易日名单；无交易日的空响应不能证明该证券属于普通状态。
		riskStatusErr = errors.New("Tushare stock_st 只能确认交易日的风险警示状态")
	} else {
		authoritativeRiskStatus, riskStatusErr = p.fetchHistoricalRiskWarningStatus(ctx, symbol, compactDate)
	}
	if riskStatusErr != nil {
		result.RiskWarningStatus = model.RiskWarningUnknown
		result.QualityStatus = model.QualityUnknown
		if !tradingDay {
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "authoritative_st_list_requires_trading_date")
		} else {
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "authoritative_historical_risk_warning_status_unavailable")
		}
	} else if result.DelistingStatus == model.DelistingStatusDelistingRisk ||
		result.DelistingStatus == model.DelistingStatusDelistingPeriod ||
		(historicalNameSuggestsDelisting && authoritativeRiskStatus == model.RiskWarningNormal) {
		// 名称出现退市线索时，空 ST 名单不能证明普通状态；特殊涨跌幅制度仍需独立事件证据。
		result.RiskWarningStatus = model.RiskWarningUnknown
		result.QualityStatus = model.QualityUnknown
		reason := "risk_warning_status_suppressed_for_delisting_price_limit_regime"
		if historicalNameSuggestsDelisting && result.DelistingStatus == model.DelistingStatusUnknown {
			reason = "risk_warning_status_not_assumed_normal_for_possible_delisting_name"
		}
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, reason)
	} else if historicalRiskStatus != model.RiskWarningUnknown && historicalRiskStatus != authoritativeRiskStatus {
		result.RiskWarningStatus = model.RiskWarningUnknown
		result.QualityStatus = model.QualityUnknown
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "historical_name_and_authoritative_st_list_conflict")
	} else {
		result.RiskWarningStatus = authoritativeRiskStatus
	}
	if strings.TrimSpace(listingDate) == "" {
		result.ListingStatusKnown = false
		result.QualityStatus = model.QualityUnknown
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_date_missing")
	} else {
		listingDateValue, parseErr := time.Parse("2006-01-02", listingDate)
		if parseErr != nil || listingDateValue.Format("2006-01-02") != listingDate || listingDateValue.After(tradeDate) {
			result.ListingStatusKnown = false
			result.QualityStatus = model.QualityUnknown
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_date_invalid_or_after_requested_trade_date")
		} else if exchange == "BSE" {
			// Tushare trade_cal documents SSE/SZSE calendars, but does not provide a
			// sufficiently verified BSE listing-session count. Do not approximate it.
			result.ListingStatusKnown = false
			result.QualityStatus = model.QualityUnknown
			result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_trading_day_number_not_verified_for_beijing_exchange")
		} else {
			count, countErr := p.countListingTradingDays(ctx, exchange, listingDateValue, tradeDate)
			if countErr != nil || count < 1 {
				result.ListingStatusKnown = false
				result.QualityStatus = model.QualityUnknown
				result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "listing_trading_day_number_unavailable")
			} else {
				result.ListingTradingDayNumber = count
				result.ListingStatusKnown = true
			}
		}
	}
	if suspensionErr != nil {
		result.SuspensionStatus = model.SuspensionStatusUnknown
		result.SuspensionTiming = ""
		result.SuspensionType = ""
		result.QualityStatus = model.QualityUnknown
		result.QualityReasons = appendUniqueSecurityStatusReason(result.QualityReasons, "historical_suspension_status_unavailable")
	} else {
		result.QualityReasons = append(result.QualityReasons, suspensionReasons...)
		if suspension == model.SuspensionStatusUnknown {
			result.QualityStatus = model.QualityUnknown
		}
	}
	result.QualityReasons = dedupeSecurityStatusReasons(result.QualityReasons)
	return result, nil
}

// fetchSecurityListingDate 显式查询 L/D/P 状态，避免 Tushare stock_basic 默认只返回 L 状态。
// 历史证券已退市时仍需其原始上市日期；任何身份冲突、状态错配或异常市场分类都会被拒绝。
func (p *Tushare) fetchSecurityListingDate(ctx context.Context, symbol string) (string, error) {
	for _, listStatus := range []string{"L", "D", "P"} {
		table, err := p.fetchCapabilityTable(ctx, "stock_basic", map[string]string{
			"ts_code": symbol, "list_status": listStatus,
		}, "ts_code,list_status,list_date,market")
		if err != nil {
			return "", err
		}
		if len(table.rows) == 0 {
			continue
		}
		if len(table.rows) != 1 {
			return "", fmt.Errorf("Tushare stock_basic 状态 %s 对 %s 返回 %d 行，无法唯一定位证券", listStatus, symbol, len(table.rows))
		}
		row := table.rows[0]
		code, err := table.stringValue(row, "ts_code")
		if err != nil {
			return "", err
		}
		canonical, err := normalize.NormalizeSymbol(code)
		if err != nil || canonical != symbol {
			return "", fmt.Errorf("Tushare stock_basic 返回证券代码不匹配: requested=%s returned=%q", symbol, code)
		}
		returnedStatus, err := table.stringValue(row, "list_status")
		if err != nil {
			return "", err
		}
		if strings.ToUpper(strings.TrimSpace(returnedStatus)) != listStatus {
			return "", fmt.Errorf("Tushare stock_basic 返回状态 %q 与请求状态 %q 不匹配", returnedStatus, listStatus)
		}
		market, err := table.stringValue(row, "market")
		if err != nil {
			return "", err
		}
		switch strings.TrimSpace(market) {
		case "主板", "创业板", "科创板", "北交所":
		default:
			return "", fmt.Errorf("Tushare stock_basic 返回未知或缺失的 A 股市场分类 %q", market)
		}
		listDateRaw, err := table.stringValue(row, "list_date")
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(listDateRaw) == "" {
			return "", nil
		}
		return tushareISODate(listDateRaw)
	}
	return "", fmt.Errorf("Tushare stock_basic 未在 L/D/P 状态目录中找到 %s", symbol)
}

// fetchHistoricalRiskWarningStatus 用 Tushare stock_st 的指定交易日名单确认 ST 状态。
// 该接口历史数据从 2000-01-01 开始；历史范围外或请求失败时返回错误而不是推断为正常。
func (p *Tushare) fetchHistoricalRiskWarningStatus(ctx context.Context, symbol, compactDate string) (model.RiskWarningStatus, error) {
	if compactDate < "20000101" {
		return model.RiskWarningUnknown, errors.New("Tushare stock_st 历史数据从 20000101 开始")
	}
	table, err := p.fetchCapabilityTable(ctx, "stock_st", map[string]string{
		"ts_code": symbol, "trade_date": compactDate,
	}, "ts_code,name,trade_date,type,type_name")
	if err != nil {
		return model.RiskWarningUnknown, err
	}
	// 即使名单为空，也必须确认响应提供了该接口的完整字段结构；不把异常空包当作“非 ST”。
	for _, field := range []string{"ts_code", "name", "trade_date", "type", "type_name"} {
		if _, ok := table.positions[field]; !ok {
			return model.RiskWarningUnknown, fmt.Errorf("Tushare stock_st 空间或记录响应缺少必需字段 %q", field)
		}
	}
	if len(table.rows) == 0 {
		return model.RiskWarningNormal, nil
	}
	if len(table.rows) != 1 {
		return model.RiskWarningUnknown, fmt.Errorf("Tushare stock_st 对 %s/%s 返回重复记录 %d 行", symbol, compactDate, len(table.rows))
	}
	row := table.rows[0]
	code, err := table.stringValue(row, "ts_code")
	if err != nil {
		return model.RiskWarningUnknown, err
	}
	canonical, err := normalize.NormalizeSymbol(code)
	if err != nil || canonical != symbol {
		return model.RiskWarningUnknown, fmt.Errorf("Tushare stock_st 返回证券代码不匹配: requested=%s returned=%q", symbol, code)
	}
	dateRaw, err := table.stringValue(row, "trade_date")
	if err != nil {
		return model.RiskWarningUnknown, err
	}
	rowDate, err := tushareCompactDate(dateRaw)
	if err != nil || rowDate != compactDate {
		return model.RiskWarningUnknown, fmt.Errorf("Tushare stock_st 返回日期不匹配: requested=%s returned=%q", compactDate, dateRaw)
	}
	statusType, err := table.stringValue(row, "type")
	if err != nil {
		return model.RiskWarningUnknown, err
	}
	if !strings.EqualFold(strings.TrimSpace(statusType), "ST") {
		return model.RiskWarningUnknown, fmt.Errorf("Tushare stock_st 返回未知类型 %q", statusType)
	}
	return model.RiskWarningApplied, nil
}

type effectiveTushareSecurityName struct {
	Name      string
	StartDate string
	EndDate   string
}

func (p *Tushare) fetchEffectiveSecurityName(ctx context.Context, symbol, compactDate string) (effectiveTushareSecurityName, error) {
	table, err := p.fetchCapabilityTable(ctx, "namechange", map[string]string{"ts_code": symbol},
		"ts_code,name,start_date,end_date,ann_date,change_reason")
	if err != nil {
		return effectiveTushareSecurityName{}, err
	}
	if len(table.rows) == 0 {
		return effectiveTushareSecurityName{}, errors.New("Tushare namechange returned no historical name rows")
	}
	var selected *effectiveTushareSecurityName
	for index, row := range table.rows {
		code, err := table.stringValue(row, "ts_code")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		canonical, err := normalize.NormalizeSymbol(code)
		if err != nil || canonical != symbol {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d returned a different or invalid symbol %q", index, code)
		}
		name, err := table.stringValue(row, "name")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		if strings.TrimSpace(name) == "" {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d has an empty name", index)
		}
		startRaw, err := table.stringValue(row, "start_date")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		startDate, err := tushareISODate(startRaw)
		if err != nil {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d has invalid start_date: %w", index, err)
		}
		endRaw, err := table.stringValue(row, "end_date")
		if err != nil {
			return effectiveTushareSecurityName{}, err
		}
		endDate := ""
		if strings.TrimSpace(endRaw) != "" {
			endDate, err = tushareISODate(endRaw)
			if err != nil {
				return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d has invalid end_date: %w", index, err)
			}
			if endDate < startDate {
				return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange row %d ends before it starts", index)
			}
		}
		if startDate > compactDate || (endDate != "" && endDate < compactDate) {
			continue
		}
		candidate := &effectiveTushareSecurityName{Name: strings.TrimSpace(name), StartDate: startDate, EndDate: endDate}
		if selected != nil {
			return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange has overlapping names effective on %s for %s", compactDate, symbol)
		}
		selected = candidate
	}
	if selected == nil {
		return effectiveTushareSecurityName{}, fmt.Errorf("Tushare namechange has no name effective on %s for %s", compactDate, symbol)
	}
	return *selected, nil
}

func riskWarningStatusFromHistoricalName(name string) model.RiskWarningStatus {
	normalized := strings.ToUpper(strings.TrimSpace(name))
	for _, prefix := range []string{"*ST", "S*ST", "SST", "ST"} {
		if strings.HasPrefix(normalized, prefix) {
			return model.RiskWarningApplied
		}
	}
	// Delisting-period names follow a separate price-limit regime; do not mistake them for normal stocks.
	if normalized == "" || strings.HasPrefix(normalized, "PT") ||
		strings.Contains(normalized, "退市") || strings.Contains(normalized, "摘牌") {
		return model.RiskWarningUnknown
	}
	return model.RiskWarningNormal
}

func delistingStatusFromHistoricalName(name string) model.DelistingStatus {
	// 历史名称（包括“退市整理”等字样）不是交易所事件记录，不能单独作为涨跌幅规则输入。
	// 在接入可核验的退市/重新上市/退市整理事件源前，统一返回 unknown，避免把名称线索当成确定状态。
	_ = name
	return model.DelistingStatusUnknown
}

func (p *Tushare) fetchSecurityTradingDay(ctx context.Context, exchange, compactDate string) (bool, error) {
	table, err := p.fetchCapabilityTable(ctx, "trade_cal", map[string]string{
		"exchange": exchange, "start_date": compactDate, "end_date": compactDate,
	}, "exchange,cal_date,is_open")
	if err != nil {
		return false, err
	}
	if len(table.rows) != 1 {
		return false, fmt.Errorf("expected one calendar row for %s/%s, received %d", exchange, compactDate, len(table.rows))
	}
	exchangeValue, err := table.stringValue(table.rows[0], "exchange")
	if err != nil || !strings.EqualFold(strings.TrimSpace(exchangeValue), exchange) {
		return false, fmt.Errorf("calendar exchange does not match requested exchange %s", exchange)
	}
	date, err := table.stringValue(table.rows[0], "cal_date")
	if err != nil {
		return false, err
	}
	normalizedDate, err := tushareCompactDate(date)
	if err != nil || normalizedDate != compactDate {
		return false, fmt.Errorf("calendar row date does not match requested date %s", compactDate)
	}
	open, err := table.stringValue(table.rows[0], "is_open")
	if err != nil {
		return false, err
	}
	switch strings.TrimSpace(open) {
	case "0":
		return false, nil
	case "1":
		return true, nil
	default:
		return false, fmt.Errorf("calendar is_open has invalid value %q", open)
	}
}

func (p *Tushare) countListingTradingDays(ctx context.Context, exchange string, listingDate, tradeDate time.Time) (int, error) {
	if listingDate.After(tradeDate) {
		return 0, errors.New("listing date is after requested trade date")
	}
	const chunkDays = 3500
	count := 0
	for start := listingDate; !start.After(tradeDate); {
		end := start.AddDate(0, 0, chunkDays-1)
		if end.After(tradeDate) {
			end = tradeDate
		}
		startCompact := start.Format("20060102")
		endCompact := end.Format("20060102")
		table, err := p.fetchCapabilityTable(ctx, "trade_cal", map[string]string{
			"exchange": exchange, "start_date": startCompact, "end_date": endCompact,
		}, "exchange,cal_date,is_open")
		if err != nil {
			return 0, err
		}
		expectedDays := int(end.Sub(start)/(24*time.Hour)) + 1
		if len(table.rows) != expectedDays {
			return 0, fmt.Errorf("listing calendar coverage mismatch for %s..%s: expected %d dates, received %d", startCompact, endCompact, expectedDays, len(table.rows))
		}
		seen := make(map[string]struct{}, len(table.rows))
		for index, row := range table.rows {
			exchangeValue, err := table.stringValue(row, "exchange")
			if err != nil || !strings.EqualFold(strings.TrimSpace(exchangeValue), exchange) {
				return 0, fmt.Errorf("listing calendar row %d has mismatched exchange %q", index, exchangeValue)
			}
			dateRaw, err := table.stringValue(row, "cal_date")
			if err != nil {
				return 0, fmt.Errorf("listing calendar row %d: %w", index, err)
			}
			date, err := tushareISODate(dateRaw)
			if err != nil {
				return 0, fmt.Errorf("listing calendar row %d: %w", index, err)
			}
			if date < start.Format("2006-01-02") || date > end.Format("2006-01-02") {
				return 0, fmt.Errorf("listing calendar returned out-of-range date %s", date)
			}
			if _, duplicate := seen[date]; duplicate {
				return 0, fmt.Errorf("listing calendar returned duplicate date %s", date)
			}
			seen[date] = struct{}{}
			open, err := table.stringValue(row, "is_open")
			if err != nil {
				return 0, fmt.Errorf("listing calendar row %s has no is_open value", date)
			}
			switch strings.TrimSpace(open) {
			case "1":
				count++
			case "0":
			default:
				return 0, fmt.Errorf("listing calendar row %s has invalid is_open value %q", date, open)
			}
		}
		if len(seen) != expectedDays {
			return 0, fmt.Errorf("listing calendar has incomplete date coverage for %s..%s", startCompact, endCompact)
		}
		start = end.AddDate(0, 0, 1)
	}
	return count, nil
}

func tushareCalendarExchange(symbol string) (string, error) {
	parts := strings.Split(symbol, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid canonical A-share symbol %q", symbol)
	}
	switch parts[1] {
	case "SH":
		return "SSE", nil
	case "SZ":
		return "SZSE", nil
	case "BJ":
		return "BSE", nil
	default:
		return "", fmt.Errorf("unsupported exchange for calendar query: %s", symbol)
	}
}

func suspensionStatusFromTushareRows(tradingDay bool, rows []struct {
	status string
	timing string
}) (model.SuspensionStatus, string, string, error) {
	if !tradingDay {
		if len(rows) > 0 {
			return model.SuspensionStatusUnknown, "", "", errors.New("Tushare returned suspension records for a non-trading date")
		}
		return model.SuspensionStatusNotTradingDay, "", "", nil
	}
	if len(rows) == 0 {
		return model.SuspensionStatusNoEvent, "", "", nil
	}
	seenS, seenR := false, false
	timings := []string{}
	for _, row := range rows {
		switch strings.TrimSpace(row.status) {
		case "S":
			seenS = true
		case "R":
			seenR = true
		default:
			return model.SuspensionStatusUnknown, "", "", fmt.Errorf("invalid Tushare suspend_type %q", row.status)
		}
		timing := strings.TrimSpace(row.timing)
		if timing != "" {
			timings = append(timings, timing)
		}
	}
	if seenS && seenR {
		if len(timings) > 0 {
			return model.SuspensionStatusIntraday, strings.Join(dedupeSecurityStatusReasons(timings), ","), "S,R", nil
		}
		return model.SuspensionStatusUnknown, "", "S,R", errors.New("conflicting all-day suspension and resumption records")
	}
	if seenS && len(timings) > 0 {
		return model.SuspensionStatusIntraday, strings.Join(dedupeSecurityStatusReasons(timings), ","), "S", nil
	}
	if seenS {
		return model.SuspensionStatusSuspended, "", "S", nil
	}
	if seenR {
		return model.SuspensionStatusResumed, strings.Join(dedupeSecurityStatusReasons(timings), ","), "R", nil
	}
	return model.SuspensionStatusUnknown, "", "", errors.New("Tushare suspend_d rows did not describe a known state")
}

func (p *Tushare) fetchSuspensionOnDate(ctx context.Context, symbol, compactDate string, tradingDay bool) (model.SuspensionStatus, string, string, []string, error) {
	table, err := p.fetchCapabilityTable(ctx, "suspend_d", map[string]string{
		"ts_code": symbol, "trade_date": compactDate,
	}, "ts_code,trade_date,suspend_timing,suspend_type")
	if err != nil {
		return model.SuspensionStatusUnknown, "", "", nil, err
	}
	rows := make([]struct {
		status string
		timing string
	}, 0, len(table.rows))
	for index, row := range table.rows {
		code, err := table.stringValue(row, "ts_code")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		canonical, err := normalize.NormalizeSymbol(code)
		if err != nil || canonical != symbol {
			return model.SuspensionStatusUnknown, "", "", nil, fmt.Errorf("Tushare suspend_d row %d has a mismatched symbol %q", index, code)
		}
		dateRaw, err := table.stringValue(row, "trade_date")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		date, err := tushareCompactDate(dateRaw)
		if err != nil || date != compactDate {
			return model.SuspensionStatusUnknown, "", "", nil, fmt.Errorf("Tushare suspend_d row %d has a mismatched date %q", index, dateRaw)
		}
		status, err := table.stringValue(row, "suspend_type")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		timing, err := table.stringValue(row, "suspend_timing")
		if err != nil {
			return model.SuspensionStatusUnknown, "", "", nil, err
		}
		rows = append(rows, struct {
			status string
			timing string
		}{status: status, timing: timing})
	}
	status, timing, suspendType, stateErr := suspensionStatusFromTushareRows(tradingDay, rows)
	reasons := []string{}
	if stateErr != nil {
		reasons = append(reasons, "suspension_state_not_proven_by_provider_response")
	}
	return status, timing, suspendType, reasons, stateErr
}

func appendUniqueSecurityStatusReason(reasons []string, reason string) []string {
	for _, existing := range reasons {
		if existing == reason {
			return reasons
		}
	}
	return append(reasons, reason)
}

func dedupeSecurityStatusReasons(reasons []string) []string {
	out := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		out = appendUniqueSecurityStatusReason(out, reason)
	}
	return out
}
